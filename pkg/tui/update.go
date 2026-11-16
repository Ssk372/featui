package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ssk372/featui/pkg/appid"
	"github.com/Ssk372/featui/pkg/model"
	"github.com/Ssk372/featui/pkg/storage"
	"github.com/Ssk372/featui/pkg/watcher"
)

// SchemaTTL es cuánto tiempo se reutiliza el esquema cacheado antes de volver a
// preguntar a Steam.
const SchemaTTL = 24 * time.Hour

type gameReadyMsg struct {
	Seq      int
	Progress model.GameProgress
	Warning  string
}

type gameErrMsg struct {
	Seq   int
	AppID int
	Err   error
}

type referenceMsg struct {
	AppID     int
	Completed map[string]bool
	Err       error
}

type resetDoneMsg struct {
	AppID    int
	Progress model.GameProgress
	Backup   string
	Err      error
}

func joinWarning(existing, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	switch {
	case existing == "":
		return extra
	case extra == "":
		return existing
	default:
		return existing + " · " + extra
	}
}

// openGameCmd carga el progreso local y el esquema (caché o Steam) en un solo
// paso. La regla de oro: si no hay esquema, NUNCA se guarda nada.
func (m Model) openGameCmd(appIDValue int, force bool, seq int) tea.Cmd {
	store, api := m.Store, m.Steam
	locale := m.Config.Locale

	return func() tea.Msg {
		local, localErr := store.LoadProgress(appIDValue)
		localFound := localErr == nil
		warning := ""
		if localErr != nil && !errors.Is(localErr, os.ErrNotExist) {
			warning = localErr.Error()
		}

		cached, hasCache, cacheErr := store.LoadSchema(appIDValue, locale)
		if cacheErr != nil {
			warning = joinWarning(warning, cacheErr.Error())
		}

		schema := cached.Achievements
		title := cached.Title
		needFetch := force || !hasCache || time.Since(cached.FetchedAt) > SchemaTTL

		if needFetch {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			fresh, freshTitle, err := api.FetchGameSchema(ctx, appIDValue, locale)
			cancel()

			if err == nil && len(fresh) == 0 {
				err = fmt.Errorf("Steam no devolvió logros para el AppID %d", appIDValue)
			}

			switch {
			case err == nil:
				schema, title = fresh, freshTitle
				_ = store.SaveSchema(appIDValue, locale, storage.CachedSchema{
					AppID:        appIDValue,
					Locale:       locale,
					Title:        freshTitle,
					FetchedAt:    time.Now(),
					Achievements: fresh,
				})
			case hasCache:
				warning = joinWarning(warning, "sin conexión con Steam; usando caché")
			case localFound:
				warning = joinWarning(warning, "sin conexión con Steam; mostrando progreso local")
				return gameReadyMsg{Seq: seq, Progress: local, Warning: warning}
			default:
				return gameErrMsg{Seq: seq, AppID: appIDValue, Err: err}
			}
		}

		if len(schema) == 0 {
			if localFound {
				return gameReadyMsg{Seq: seq, Progress: local, Warning: warning}
			}
			return gameErrMsg{
				Seq:   seq,
				AppID: appIDValue,
				Err:   fmt.Errorf("Steam no devolvió logros para el AppID %d", appIDValue),
			}
		}

		merged := model.Merge(local, schema, title, time.Now())
		merged.AppID = appIDValue
		if err := store.SaveProgress(merged); err != nil {
			warning = joinWarning(warning, "no se pudo guardar el progreso: "+err.Error())
		}

		return gameReadyMsg{Seq: seq, Progress: merged, Warning: warning}
	}
}

// Update implementa tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		width := msg.Width - 16
		if width < 20 {
			width = 20
		}
		if width > 60 {
			width = 60
		}
		m.Input.Width = width
		m.Search.Width = width
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case gameReadyMsg:
		if msg.Seq != m.seq {
			return m, nil // respuesta obsoleta
		}
		m.stopWatcher()
		m.Game = msg.Progress
		m.AppID = msg.Progress.AppID
		m.Screen = ScreenReady
		m.Cursor = 0
		m.Filter = FilterAll
		m.ConfirmReset = false
		m.Searching = false
		m.Search.SetValue("")
		m.Search.Blur()
		m.Input.Blur()
		m.Warning = msg.Warning
		m.Err = nil
		m.Reference = nil
		m.Watcher = watcher.New(m.AppID)
		m.StatusMsg = fmt.Sprintf("%s · %d logros", m.Game.Title, m.Game.Total())

		if m.Config.AddRecentAppID(m.AppID) {
			_ = m.Config.Save()
		}
		m.loadRecents()

		cmds := make([]tea.Cmd, 0, 2)
		if cmd := m.referenceCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := m.armWatcher(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case gameErrMsg:
		if msg.Seq != m.seq {
			return m, nil
		}
		m.stopWatcher()
		m.Screen = ScreenError
		m.AppID = msg.AppID
		m.Err = msg.Err
		m.StatusMsg = ""
		return m, nil

	case referenceMsg:
		if msg.AppID != m.AppID || m.Screen != ScreenReady {
			return m, nil
		}
		if msg.Err == nil {
			m.Reference = msg.Completed
		}
		return m, nil

	case resetDoneMsg:
		if msg.AppID != m.AppID {
			return m, nil
		}
		if msg.Err != nil {
			m.Warning = msg.Err.Error()
			return m, nil
		}
		m.Game = msg.Progress
		m.ConfirmReset = false
		m.ClampCursor()
		m.StatusMsg = fmt.Sprintf("Partida reiniciada · backup en %s", msg.Backup)
		return m, nil

	case watcher.Msg:
		m.WatchActive = false
		if m.Screen == ScreenReady && msg.AppID == m.AppID {
			if progress, err := m.Store.LoadProgress(m.AppID); err == nil {
				m.Game = progress
				m.ClampCursor()
				m.StatusMsg = "Guardado detectado en el juego · progreso recargado"
			}
		}
		if cmd := m.armWatcher(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}

	// Mensajes de parpadeo del cursor de los inputs.
	var cmd tea.Cmd
	switch {
	case m.Screen == ScreenAppID:
		m.Input, cmd = m.Input.Update(msg)
	case m.Searching:
		m.Search, cmd = m.Search.Update(msg)
	}
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		m.shutdownWatcher()
		return m, tea.Quit
	}

	if m.ConfirmReset {
		switch key {
		case "y", "Y":
			store, appIDValue := m.Store, m.AppID
			m.ConfirmReset = false
			return m, func() tea.Msg {
				progress, backup, err := store.ResetProgress(appIDValue)
				return resetDoneMsg{AppID: appIDValue, Progress: progress, Backup: backup, Err: err}
			}
		case "n", "N", "esc":
			m.ConfirmReset = false
			m.StatusMsg = "Reinicio cancelado"
		}
		return m, nil
	}

	if m.Searching {
		switch key {
		case "esc":
			m.Searching = false
			m.Search.Blur()
			m.Search.SetValue("")
			m.Cursor = 0
			return m, nil
		case "enter", "tab":
			m.Searching = false
			m.Search.Blur()
			return m, nil
		case "up", "k":
			m.moveCursor(-1)
			return m, nil
		case "down", "j":
			m.moveCursor(1)
			return m, nil
		}

		var cmd tea.Cmd
		m.Search, cmd = m.Search.Update(msg)
		m.Cursor = 0
		return m, cmd
	}

	switch m.Screen {
	case ScreenAppID:
		return m.handleAppIDKey(key, msg)
	case ScreenReady:
		return m.handleReadyKey(key)
	case ScreenError:
		switch key {
		case "q", "esc":
			m.Screen = ScreenAppID
			m.Err = nil
			m.StatusMsg = ""
			m.Input.Focus()
			return m, textinput.Blink
		case "r", "enter":
			if m.AppID > 0 {
				// startGame ya incrementa la secuencia e ignora respuestas viejas.
				return m.startGame(m.AppID, true)
			}
		}
	case ScreenLoading:
		if key == "esc" {
			m.Screen = ScreenAppID
			m.AppID = 0
			m.StatusMsg = ""
			m.Input.Focus()
			return m, textinput.Blink
		}
	}

	return m, nil
}

func (m Model) handleAppIDKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.shutdownWatcher()
		return m, tea.Quit
	case "enter":
		value := strings.TrimSpace(m.Input.Value())
		if value != "" {
			id, err := appid.Parse(value)
			if err != nil {
				m.StatusMsg = err.Error()
				return m, nil
			}
			return m.startGame(id, false)
		}
		if len(m.Recents) > 0 {
			return m.startGame(m.Recents[m.RecentCursor].AppID, false)
		}
		m.StatusMsg = "Escribe un AppID o una URL de Steam"
		return m, nil
	case "up", "k":
		if m.Input.Value() == "" && len(m.Recents) > 0 {
			if m.RecentCursor > 0 {
				m.RecentCursor--
			}
			return m, nil
		}
	case "down", "j":
		if m.Input.Value() == "" && len(m.Recents) > 0 {
			if m.RecentCursor < len(m.Recents)-1 {
				m.RecentCursor++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	if m.Input.Value() == "" {
		m.RecentCursor = 0
	}
	return m, cmd
}

func (m Model) handleReadyKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		m.shutdownWatcher()
		return m, tea.Quit

	case "esc":
		m.stopWatcher()
		m.Screen = ScreenAppID
		m.AppID = 0
		m.Game = model.GameProgress{}
		m.Reference = nil
		m.Cursor = 0
		m.Warning = ""
		m.StatusMsg = ""
		m.Input.Focus()
		return m, textinput.Blink

	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-5)
	case "pgdown":
		m.moveCursor(5)
	case "home", "g":
		m.Cursor = 0
		m.ClampCursor()
	case "end", "G":
		m.Cursor = len(m.visibleIndexes()) - 1
		m.ClampCursor()

	case " ":
		return m, m.toggleCurrent()

	case "/":
		m.Searching = true
		m.Search.SetValue("")
		m.Search.Focus()
		m.Cursor = 0
		return m, textinput.Blink

	case "f":
		m.Filter = (m.Filter + 1) % 3
		m.Cursor = 0

	case "r":
		seq := m.nextSeq()
		m.Screen = ScreenLoading
		m.StatusMsg = "Actualizando desde Steam..."
		m.stopWatcher()
		return m, m.openGameCmd(m.AppID, true, seq)

	case "R":
		m.ConfirmReset = true

	case "?":
		m.ShowHelp = !m.ShowHelp
	}

	return m, nil
}

func (m Model) startGame(id int, force bool) (tea.Model, tea.Cmd) {
	seq := m.nextSeq()

	m.Screen = ScreenLoading
	m.AppID = id
	m.Err = nil
	m.Warning = ""
	m.StatusMsg = fmt.Sprintf("Cargando AppID %d...", id)
	m.Game = model.GameProgress{AppID: id}
	m.Cursor = 0
	m.Filter = FilterAll
	m.ConfirmReset = false
	m.Searching = false
	m.Search.SetValue("")
	m.Search.Blur()
	m.Input.SetValue("")
	m.Input.Blur()
	m.Reference = nil
	m.stopWatcher()

	return m, m.openGameCmd(id, force, seq)
}

func (m *Model) moveCursor(delta int) {
	total := len(m.visibleIndexes())
	if total == 0 {
		m.Cursor = 0
		return
	}
	m.Cursor += delta
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= total {
		m.Cursor = total - 1
	}
}

// ClampCursor mantiene el cursor dentro de la lista filtrada.
func (m *Model) ClampCursor() {
	total := len(m.visibleIndexes())
	switch {
	case total == 0:
		m.Cursor = 0
	case m.Cursor < 0:
		m.Cursor = 0
	case m.Cursor >= total:
		m.Cursor = total - 1
	}
}

// visibleIndexes devuelve los índices reales que pasan filtro y búsqueda.
func (m Model) visibleIndexes() []int {
	query := foldText(strings.TrimSpace(m.Search.Value()))
	indexes := make([]int, 0, len(m.Game.Achievements))

	for i, ach := range m.Game.Achievements {
		switch m.Filter {
		case FilterPending:
			if ach.Completed {
				continue
			}
		case FilterCompleted:
			if !ach.Completed {
				continue
			}
		}
		if query != "" {
			haystack := foldText(ach.DisplayName + " " + ach.Description + " " + ach.APIName)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		indexes = append(indexes, i)
	}
	return indexes
}

var accentFolder = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ü", "u", "Ñ", "n",
)

func foldText(value string) string {
	return accentFolder.Replace(strings.ToLower(value))
}

// toggleCurrent marca o desmarca el logro seleccionado y lo persiste.
func (m *Model) toggleCurrent() tea.Cmd {
	indexes := m.visibleIndexes()
	if len(indexes) == 0 || m.Cursor >= len(indexes) {
		return nil
	}

	ach := &m.Game.Achievements[indexes[m.Cursor]]
	ach.Completed = !ach.Completed

	now := time.Now()
	if ach.Completed {
		ach.UnlockedAt = &now
		ach.Source = model.SourceManual
	} else {
		ach.UnlockedAt = nil
		ach.Source = ""
	}
	m.Game.LastUpdated = now

	if err := m.Store.SaveProgress(m.Game); err != nil {
		m.Warning = "no se pudo guardar el progreso: " + err.Error()
	} else {
		m.Warning = ""
	}

	if ach.Completed {
		m.StatusMsg = "Logro marcado: " + ach.DisplayName
	} else {
		m.StatusMsg = "Logro desmarcado: " + ach.DisplayName
	}
	m.ClampCursor()

	if ach.Completed && m.Config.Notify && m.Notifier != nil {
		return m.notifyCmd(*ach)
	}
	return nil
}

func (m Model) notifyCmd(ach model.Achievement) tea.Cmd {
	notif, gameTitle := m.Notifier, m.Game.Title
	return func() tea.Msg {
		_ = notif.SendAchievementNotification(gameTitle, ach.DisplayName, ach.Description, ach.IconURL, ach.APIName)
		return nil
	}
}

func (m Model) referenceCmd() tea.Cmd {
	if m.Config.SteamID64 == "" || m.AppID <= 0 {
		return nil
	}
	api, steamID, appIDValue := m.Steam, m.Config.SteamID64, m.AppID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		completed, err := api.FetchPlayerAchievements(ctx, steamID, appIDValue)
		return referenceMsg{AppID: appIDValue, Completed: completed, Err: err}
	}
}

// armWatcher lanza un watcher nuevo, cancelando el anterior si lo hubiera.
func (m *Model) armWatcher() tea.Cmd {
	if !m.Config.WatchProfile || m.Watcher == nil || !m.Watcher.Available() {
		m.WatchActive = false
		return nil
	}

	m.stopWatcher()
	ctx, cancel := context.WithCancel(m.rootCtx)
	m.watchCancel = cancel
	m.WatchActive = true
	return m.Watcher.Start(ctx)
}

func (m *Model) stopWatcher() {
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
	m.WatchActive = false
}

func (m *Model) shutdownWatcher() {
	m.stopWatcher()
	if m.rootCancel != nil {
		m.rootCancel()
	}
}
