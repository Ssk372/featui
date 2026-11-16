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
	"github.com/Ssk372/featui/pkg/autotrack"
	"github.com/Ssk372/featui/pkg/model"
	"github.com/Ssk372/featui/pkg/steam"
	"github.com/Ssk372/featui/pkg/storage"
)

// SchemaTTL es cuánto tiempo se reutiliza el esquema cacheado antes de volver a
// preguntar a Steam.
const SchemaTTL = 24 * time.Hour

// maxBackoff acota el sondeo cuando Steam falla repetidamente.
const maxBackoff = 5 * time.Minute

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
	Seq    int
	AppID  int
	States map[string]steam.PlayerAchievement
	Err    error
}

type pollTickMsg struct {
	AppID int
	Seq   int
}

type pollResultMsg struct {
	Seq    int
	AppID  int
	States map[string]steam.PlayerAchievement
	Err    error
}

type syncDoneMsg struct {
	Seq    int
	AppID  int
	States map[string]steam.PlayerAchievement
	Err    error
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
				warning = joinWarning(warning, err.Error()+" · usando caché")
			case localFound:
				warning = joinWarning(warning, err.Error()+" · mostrando progreso local")
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
		m.stopAuto()
		m.Game = msg.Progress
		m.AppID = msg.Progress.AppID
		m.Screen = ScreenReady
		m.Cursor = 0
		m.Filter = FilterAll
		m.ConfirmReset = false
		m.ConfirmSync = false
		m.Searching = false
		m.Search.SetValue("")
		m.Search.Blur()
		m.Input.Blur()
		m.Warning = msg.Warning
		m.AutoWarning = ""
		m.PollErrors = 0
		m.LastPoll = time.Time{}
		m.Err = nil
		m.Reference = nil
		m.Tracker = autotrack.New()
		m.StatusMsg = fmt.Sprintf("%s · %d logros", m.Game.Title, m.Game.Total())

		if m.Config.AddRecentAppID(m.AppID) {
			_ = m.Config.Save()
		}
		m.loadRecents()

		cmds := make([]tea.Cmd, 0, 1)
		if m.Config.AutoTrack {
			if m.Config.SteamID64 == "" {
				m.AutoWarning = "auto-track desactivado: falta steam_id_64 (perfil público de Steam)"
			} else if cmd := m.referenceCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else if cmd := m.referenceCmd(); cmd != nil {
			// Aunque el auto-track esté apagado, el badge informativo sigue.
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
		m.stopAuto()
		m.Screen = ScreenError
		m.AppID = msg.AppID
		m.Err = msg.Err
		m.StatusMsg = ""
		return m, nil

	case referenceMsg:
		if msg.Seq != m.seq || msg.AppID != m.AppID || m.Screen != ScreenReady {
			return m, nil
		}
		if msg.Err != nil {
			if !m.Config.AutoTrack {
				return m, nil
			}
			if autoDisabledBy(msg.Err) {
				m.AutoActive = false
				m.AutoWarning = autoWarningText(msg.Err)
				return m, nil
			}
			// Error transitorio: se mantiene el auto-track con backoff; el
			// primer sondeo correcto establecerá la línea base.
			m.PollErrors = 1
			m.AutoActive = true
			m.AutoWarning = "no se pudo consultar Steam: " + msg.Err.Error()
			return m, m.schedulePoll(m.AppID)
		}

		m.Reference = unlockedSet(msg.States)
		m.AutoWarning = ""
		if !m.Config.AutoTrack {
			return m, nil
		}
		m.Tracker.Diff(toStates(msg.States)) // solo línea base: no desbloquea nada
		m.LastPoll = time.Now()
		m.PollErrors = 0
		m.AutoActive = true
		return m, m.schedulePoll(m.AppID)

	case pollTickMsg:
		if msg.AppID != m.AppID || msg.Seq != m.seq || m.Screen != ScreenReady || !m.AutoActive {
			return m, nil
		}
		return m, m.pollCmd(msg.AppID)

	case pollResultMsg:
		if msg.Seq != m.seq || msg.AppID != m.AppID || m.Screen != ScreenReady || !m.AutoActive {
			return m, nil
		}
		if msg.Err != nil {
			m.PollErrors++
			if autoDisabledBy(msg.Err) {
				m.AutoActive = false
				m.AutoWarning = autoWarningText(msg.Err)
				return m, nil
			}
			if m.PollErrors >= 3 {
				m.AutoWarning = fmt.Sprintf(
					"auto-track con problemas; reintentando cada %s: %s",
					shortDuration(m.pollDelay()), msg.Err.Error(),
				)
			}
			return m, m.schedulePoll(m.AppID)
		}

		m.AutoWarning = ""
		m.PollErrors = 0
		m.LastPoll = time.Now()
		m.Reference = unlockedSet(msg.States)

		unlocks := m.Tracker.Diff(toStates(msg.States))
		next := m.schedulePoll(m.AppID)
		if len(unlocks) == 0 {
			return m, next
		}

		changed := m.applyUnlocks(unlocks, model.SourceSteam)
		if len(changed) == 0 {
			return m, next
		}
		return m, mergeCmds(next, m.achievementFeedback(changed))

	case syncDoneMsg:
		if msg.Seq != m.seq || msg.AppID != m.AppID || m.Screen != ScreenReady {
			return m, nil
		}
		if msg.Err != nil {
			m.AutoWarning = "no se pudo sincronizar con Steam: " + msg.Err.Error()
			m.StatusMsg = "Sincronización cancelada"
			return m, nil
		}

		m.AutoWarning = ""
		m.Reference = unlockedSet(msg.States)
		if m.Tracker != nil {
			// Mantiene la línea base al día para no repetir estos avisos.
			m.Tracker.Diff(toStates(msg.States))
		}

		changed := m.applyUnlocks(autotrack.Adopt(toStates(msg.States)), model.SourceSteam)
		switch {
		case len(changed) == 0:
			m.StatusMsg = "Todo al día: no hay logros nuevos en Steam"
		case len(changed) == 1:
			m.StatusMsg = "Sincronizado desde Steam: " + changed[0].DisplayName
		default:
			m.StatusMsg = fmt.Sprintf("Sincronizados %d logros desde Steam", len(changed))
		}
		return m, m.achievementFeedback(changed)

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
		m.shutdown()
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

	if m.ConfirmSync {
		switch key {
		case "y", "Y":
			m.ConfirmSync = false
			m.StatusMsg = "Consultando Steam..."
			return m, m.syncCmd()
		case "n", "N", "esc":
			m.ConfirmSync = false
			m.StatusMsg = "Sincronización cancelada"
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
		m.shutdown()
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
		m.shutdown()
		return m, tea.Quit

	case "esc":
		m.stopAuto()
		m.Screen = ScreenAppID
		m.AppID = 0
		m.Game = model.GameProgress{}
		m.Reference = nil
		m.Tracker = autotrack.New()
		m.AutoWarning = ""
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
		m.stopAuto()
		return m, m.openGameCmd(m.AppID, true, seq)

	case "S":
		if m.Config.SteamID64 == "" {
			m.StatusMsg = "Configura steam_id_64 para sincronizar con Steam"
			return m, nil
		}
		m.ConfirmSync = true
		m.ConfirmReset = false

	case "R":
		m.ConfirmReset = true
		m.ConfirmSync = false

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
	m.ConfirmSync = false
	m.Searching = false
	m.Search.SetValue("")
	m.Search.Blur()
	m.Input.SetValue("")
	m.Input.Blur()
	m.Reference = nil
	m.Tracker = autotrack.New()
	m.stopAuto()

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

	if ach.Completed {
		return m.achievementFeedback([]model.Achievement{*ach})
	}
	return nil
}

// applyUnlocks marca en el progreso local los logros que Steam desbloqueó.
// Devuelve solo los que cambiaron de verdad, para no notificar duplicados.
func (m *Model) applyUnlocks(unlocks []autotrack.Unlock, source model.UnlockSource) []model.Achievement {
	if len(unlocks) == 0 {
		return nil
	}

	positions := make(map[string]int, len(m.Game.Achievements))
	for i := range m.Game.Achievements {
		positions[m.Game.Achievements[i].APIName] = i
	}

	changed := make([]model.Achievement, 0, len(unlocks))
	for _, unlock := range unlocks {
		position, ok := positions[unlock.APIName]
		if !ok {
			continue
		}
		ach := &m.Game.Achievements[position]
		if ach.Completed {
			continue
		}

		at := unlock.UnlockedAt
		if at.IsZero() {
			at = time.Now()
		}
		ach.Completed = true
		ach.UnlockedAt = &at
		ach.Source = source
		changed = append(changed, *ach)
	}

	if len(changed) == 0 {
		return nil
	}

	m.Game.LastUpdated = time.Now()
	if err := m.Store.SaveProgress(m.Game); err != nil {
		m.Warning = "no se pudo guardar el progreso: " + err.Error()
	}
	m.ClampCursor()

	if len(changed) == 1 {
		m.StatusMsg = "Logro desbloqueado en Steam: " + changed[0].DisplayName
	} else {
		m.StatusMsg = fmt.Sprintf("%d logros desbloqueados en Steam", len(changed))
	}
	return changed
}

// achievementFeedback notifica y suena, respetando notify y sound por separado.
func (m Model) achievementFeedback(achievements []model.Achievement) tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2)

	if len(achievements) > 0 && m.Config.Notify && m.Notifier != nil {
		cmds = append(cmds, m.notifyCmd(achievements))
	}
	if m.Config.Sound && m.Sound != nil {
		player := m.Sound
		cmds = append(cmds, func() tea.Msg {
			_ = player.Play()
			return nil
		})
	}

	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

func (m Model) notifyCmd(achievements []model.Achievement) tea.Cmd {
	notif, gameTitle := m.Notifier, m.Game.Title
	items := append([]model.Achievement(nil), achievements...)

	return func() tea.Msg {
		if len(items) == 1 {
			ach := items[0]
			_ = notif.SendAchievementNotification(gameTitle, ach.DisplayName, ach.Description, ach.IconURL, ach.APIName)
			return nil
		}

		names := make([]string, 0, len(items))
		for _, ach := range items {
			names = append(names, ach.DisplayName)
		}
		first := items[0]
		summary := strings.Join(names, ", ")
		if len([]rune(summary)) > 350 {
			summary = string([]rune(summary)[:349]) + "…"
		}
		_ = notif.SendAchievementNotification(
			gameTitle,
			fmt.Sprintf("%d logros desbloqueados", len(items)),
			summary,
			first.IconURL,
			"summary-"+first.APIName,
		)
		return nil
	}
}

func (m Model) referenceCmd() tea.Cmd {
	if m.Config.SteamID64 == "" || m.AppID <= 0 {
		return nil
	}
	api, steamID, appIDValue, seq := m.Steam, m.Config.SteamID64, m.AppID, m.seq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		states, err := api.FetchPlayerAchievements(ctx, steamID, appIDValue)
		return referenceMsg{Seq: seq, AppID: appIDValue, States: states, Err: err}
	}
}

func (m Model) pollCmd(appIDValue int) tea.Cmd {
	api, steamID, seq := m.Steam, m.Config.SteamID64, m.seq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		states, err := api.FetchPlayerAchievements(ctx, steamID, appIDValue)
		return pollResultMsg{Seq: seq, AppID: appIDValue, States: states, Err: err}
	}
}

func (m Model) syncCmd() tea.Cmd {
	api, steamID, appIDValue, seq := m.Steam, m.Config.SteamID64, m.AppID, m.seq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		states, err := api.FetchPlayerAchievements(ctx, steamID, appIDValue)
		return syncDoneMsg{Seq: seq, AppID: appIDValue, States: states, Err: err}
	}
}

// schedulePoll encola el siguiente sondeo con backoff si Steam viene fallando.
func (m Model) schedulePoll(appIDValue int) tea.Cmd {
	if !m.AutoActive {
		return nil
	}
	seq := m.seq
	return tea.Tick(m.pollDelay(), func(time.Time) tea.Msg {
		return pollTickMsg{AppID: appIDValue, Seq: seq}
	})
}

func (m Model) pollDelay() time.Duration {
	base := time.Duration(m.Config.PollInterval) * time.Second
	if base <= 0 {
		base = 30 * time.Second
	}

	delay := base
	switch {
	case m.PollErrors <= 0:
		delay = base
	case m.PollErrors == 1:
		delay = base * 2
	case m.PollErrors == 2:
		delay = base * 4
	default:
		delay = maxBackoff
	}
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return delay
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d%time.Minute == 0:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	default:
		return d.Round(time.Second).String()
	}
}

func mergeCmds(first, second tea.Cmd) tea.Cmd {
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	default:
		return tea.Batch(first, second)
	}
}

// autoDisabledBy indica si el error es permanente para el auto-track.
func autoDisabledBy(err error) bool {
	return errors.Is(err, steam.ErrPrivateProfile) ||
		errors.Is(err, steam.ErrNoStats) ||
		errors.Is(err, steam.ErrInvalidAPIKey)
}

func autoWarningText(err error) string {
	switch {
	case errors.Is(err, steam.ErrPrivateProfile):
		return "auto-track desactivado: el perfil de Steam no es público"
	case errors.Is(err, steam.ErrNoStats):
		return "auto-track desactivado: este juego no expone logros por API"
	case errors.Is(err, steam.ErrInvalidAPIKey):
		return "auto-track desactivado: la API key de Steam no es válida"
	default:
		return "auto-track: " + err.Error()
	}
}

func (m *Model) stopAuto() {
	m.AutoActive = false
}

func (m *Model) shutdown() {
	m.stopAuto()
}
