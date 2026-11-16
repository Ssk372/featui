package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ssk372/featui/pkg/autotrack"
	"github.com/Ssk372/featui/pkg/config"
	"github.com/Ssk372/featui/pkg/model"
	"github.com/Ssk372/featui/pkg/steam"
)

type screen int

const (
	ScreenAppID screen = iota
	ScreenLoading
	ScreenReady
	ScreenError
)

type filterMode int

const (
	FilterAll filterMode = iota
	FilterPending
	FilterCompleted
)

func (f filterMode) String() string {
	switch f {
	case FilterPending:
		return "pendientes"
	case FilterCompleted:
		return "completados"
	default:
		return "todos"
	}
}

type recentItem struct {
	AppID     int
	Title     string
	Completed int
	Total     int
}

// Model es el estado completo de la TUI.
type Model struct {
	Screen screen
	Config config.Config
	Game   model.GameProgress
	AppID  int
	Cursor int
	Err    error

	StatusMsg string
	Warning   string

	Store    Store
	Steam    SteamAPI
	Notifier Notifier
	Sound    SoundPlayer

	// SoundWarning avisa de un sound_file inexistente o de la ausencia de
	// reproductor. Es informativo y no impide usar FeatUI.
	SoundWarning string

	Input        textinput.Model
	Search       textinput.Model
	Searching    bool
	Filter       filterMode
	ConfirmReset bool
	ConfirmSync  bool
	ShowHelp     bool

	Recents      []recentItem
	RecentCursor int

	// Reference es el estado real del jugador en Steam, solo para el badge
	// informativo. Nunca se mezcla con el progreso local.
	Reference map[string]bool

	// Auto-track: se consulta Steam mientras la pantalla de logros está
	// abierta y solo se marcan transiciones false→true.
	Tracker     *autotrack.Tracker
	AutoActive  bool
	AutoWarning string
	LastPoll    time.Time
	PollErrors  int

	width  int
	height int
	seq    int
}

// NewModel construye la TUI y, si hay juegos recientes, abre el último.
func NewModel(cfg config.Config, store Store, api SteamAPI, notif Notifier, snd SoundPlayer) Model {
	input := textinput.New()
	input.Placeholder = "49520 o https://store.steampowered.com/app/49520"
	input.CharLimit = 200
	input.Width = 46
	input.Focus()

	search := textinput.New()
	search.Placeholder = "buscar logro..."
	search.CharLimit = 120
	search.Width = 46

	m := Model{
		Screen:   ScreenAppID,
		Config:   cfg,
		Store:    store,
		Steam:    api,
		Notifier: notif,
		Sound:    snd,
		Input:    input,
		Search:   search,
		Filter:   FilterAll,
		Tracker:  autotrack.New(),
	}
	if snd != nil {
		m.SoundWarning = snd.Warning()
	}

	m.loadRecents()
	if len(m.Recents) > 0 {
		m.Screen = ScreenLoading
		m.AppID = m.Recents[0].AppID
		m.StatusMsg = fmt.Sprintf("Cargando AppID %d...", m.AppID)
		m.seq = m.nextSeq()
	}

	return m
}

// Init arranca la carga del último juego o muestra la pantalla de entrada.
func (m Model) Init() tea.Cmd {
	if m.Screen == ScreenLoading && m.AppID > 0 {
		return m.openGameCmd(m.AppID, false, m.seq)
	}
	return textinput.Blink
}

func (m *Model) nextSeq() int {
	m.seq++
	return m.seq
}

func (m *Model) loadRecents() {
	items := make([]recentItem, 0, len(m.Config.RecentAppIDs))
	for _, id := range m.Config.RecentAppIDs {
		item := recentItem{AppID: id}
		if progress, err := m.Store.LoadProgress(id); err == nil {
			item.Title = progress.Title
			item.Completed = progress.CompletedCount()
			item.Total = progress.Total()
		}
		items = append(items, item)
	}

	m.Recents = items
	if m.RecentCursor >= len(items) {
		m.RecentCursor = 0
	}
}

// unlockedSet convierte el snapshot de Steam en el mapa que usa el badge.
func unlockedSet(raw map[string]steam.PlayerAchievement) map[string]bool {
	out := make(map[string]bool, len(raw))
	for name, state := range raw {
		if state.Unlocked {
			out[name] = true
		}
	}
	return out
}

// toStates adapta el snapshot de Steam al tracker.
func toStates(raw map[string]steam.PlayerAchievement) map[string]autotrack.State {
	out := make(map[string]autotrack.State, len(raw))
	for name, state := range raw {
		out[name] = autotrack.State{
			Unlocked:   state.Unlocked,
			UnlockedAt: state.UnlockTime,
		}
	}
	return out
}
