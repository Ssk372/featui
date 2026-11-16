package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ssk372/featui/pkg/autotrack"
	"github.com/Ssk372/featui/pkg/config"
	"github.com/Ssk372/featui/pkg/model"
	"github.com/Ssk372/featui/pkg/steam"
	"github.com/Ssk372/featui/pkg/storage"
)

type fakeSteam struct {
	schema []model.Achievement
	title  string
	err    error
	player func() (map[string]steam.PlayerAchievement, error)
}

func (f fakeSteam) FetchGameSchema(context.Context, int, string) ([]model.Achievement, string, error) {
	return f.schema, f.title, f.err
}

func (f fakeSteam) FetchPlayerAchievements(context.Context, string, int) (map[string]steam.PlayerAchievement, error) {
	if f.player != nil {
		return f.player()
	}
	return nil, errors.New("no usado en tests")
}

type fakeNotifier struct {
	names []string
}

func (f *fakeNotifier) SendAchievementNotification(_, name, _, _, _ string) error {
	f.names = append(f.names, name)
	return nil
}

type fakeSound struct {
	plays int
}

func (f *fakeSound) Play() error {
	f.plays++
	return nil
}

func (f *fakeSound) Warning() string { return "" }

// runCmd ejecuta un comando y, si es un batch, sus comandos internos.
func runCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, inner := range batch {
			runCmd(t, inner)
		}
	}
}

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func newTestStore(t *testing.T) *storage.Storage {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))

	store, err := storage.New()
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return store
}

// Regresión del bug que borraba el progreso local cuando Steam fallaba.
func TestOpenGameNeverOverwritesLocalProgressOnSchemaFailure(t *testing.T) {
	store := newTestStore(t)
	unlocked := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	local := model.GameProgress{
		AppID: 49520,
		Title: "Borderlands 2",
		Achievements: []model.Achievement{
			{APIName: "A", DisplayName: "A", Completed: true, UnlockedAt: &unlocked, Source: model.SourceManual},
		},
	}
	if err := store.SaveProgress(local); err != nil {
		t.Fatal(err)
	}

	m := Model{
		Store:  store,
		Steam:  fakeSteam{err: errors.New("sin red")},
		Config: config.Config{Locale: "spanish"},
	}

	msg := m.openGameCmd(49520, true, 1)()
	ready, ok := msg.(gameReadyMsg)
	if !ok {
		t.Fatalf("mensaje = %T, quiero gameReadyMsg", msg)
	}
	if ready.Progress.CompletedCount() != 1 {
		t.Fatalf("se perdió el progreso en memoria: %+v", ready.Progress)
	}
	if ready.Warning == "" {
		t.Fatal("debería avisar de que se usa el progreso local")
	}

	reloaded, err := store.LoadProgress(49520)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CompletedCount() != 1 {
		t.Fatal("el progreso local se sobrescribió en disco")
	}
	if reloaded.Achievements[0].UnlockedAt == nil {
		t.Fatal("se perdió unlocked_at en disco")
	}
}

func TestOpenGameMergesSchemaAndPreservesAudit(t *testing.T) {
	store := newTestStore(t)
	unlocked := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
	local := model.GameProgress{
		AppID: 49520,
		Achievements: []model.Achievement{
			{APIName: "A", DisplayName: "A", Completed: true, UnlockedAt: &unlocked, Source: model.SourceManual},
		},
	}
	if err := store.SaveProgress(local); err != nil {
		t.Fatal(err)
	}

	api := fakeSteam{
		title: "Borderlands",
		schema: []model.Achievement{
			{APIName: "A", DisplayName: "A actualizado"},
			{APIName: "B", DisplayName: "B", Completed: true, Source: "steam"},
		},
	}
	m := Model{Store: store, Steam: api, Config: config.Config{Locale: "spanish"}}

	msg := m.openGameCmd(49520, true, 1)()
	ready, ok := msg.(gameReadyMsg)
	if !ok {
		t.Fatalf("mensaje = %T, quiero gameReadyMsg", msg)
	}
	if ready.Warning != "" {
		t.Fatalf("warning inesperado: %s", ready.Warning)
	}
	if ready.Progress.Total() != 2 {
		t.Fatalf("total = %d", ready.Progress.Total())
	}
	if !ready.Progress.Achievements[0].Completed || ready.Progress.Achievements[0].UnlockedAt == nil {
		t.Fatal("el estado local de A debe conservarse")
	}
	if b := ready.Progress.Achievements[1]; b.Completed || b.Source != "" || b.UnlockedAt != nil {
		t.Fatalf("B debe entrar pendiente: %+v", b)
	}

	reloaded, err := store.LoadProgress(49520)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Title != "Borderlands" || reloaded.CompletedCount() != 1 {
		t.Fatalf("no se guardó el merge: %+v", reloaded)
	}
}

func TestOpenGameUsesCacheWhenSteamFails(t *testing.T) {
	store := newTestStore(t)
	cached := storage.CachedSchema{
		AppID:     49520,
		Locale:    "spanish",
		Title:     "Borderlands",
		FetchedAt: time.Now(),
		Achievements: []model.Achievement{
			{APIName: "A", DisplayName: "A"},
		},
	}
	if err := store.SaveSchema(49520, "spanish", cached); err != nil {
		t.Fatal(err)
	}

	m := Model{Store: store, Steam: fakeSteam{err: errors.New("sin red")}, Config: config.Config{Locale: "spanish"}}
	msg := m.openGameCmd(49520, true, 1)()

	ready, ok := msg.(gameReadyMsg)
	if !ok {
		t.Fatalf("mensaje = %T, quiero gameReadyMsg", msg)
	}
	if ready.Progress.Total() != 1 {
		t.Fatalf("debería usar la caché, total = %d", ready.Progress.Total())
	}
	// Sin progreso local, la caché no debe escribirse como progreso "completado".
	if ready.Progress.CompletedCount() != 0 {
		t.Fatal("la caché no debe marcar logros")
	}
}

func TestOpenGameWithoutLocalAndSteamError(t *testing.T) {
	store := newTestStore(t)
	m := Model{Store: store, Steam: fakeSteam{err: errors.New("boom")}, Config: config.Config{}}

	msg := m.openGameCmd(999, true, 1)()
	if _, ok := msg.(gameErrMsg); !ok {
		t.Fatalf("mensaje = %T, quiero gameErrMsg", msg)
	}
}

func TestTogglePersistsAuditFields(t *testing.T) {
	store := newTestStore(t)
	game := model.GameProgress{
		AppID: 49520,
		Title: "Borderlands 2",
		Achievements: []model.Achievement{
			{APIName: "A", DisplayName: "A"},
			{APIName: "B", DisplayName: "B"},
		},
	}
	if err := store.SaveProgress(game); err != nil {
		t.Fatal(err)
	}

	m := Model{
		Screen: ScreenReady,
		Store:  store,
		Config: config.Config{},
		Game:   game,
		Cursor: 0,
		Filter: FilterAll,
	}

	m.toggleCurrent()

	saved, err := store.LoadProgress(49520)
	if err != nil {
		t.Fatal(err)
	}
	ach := saved.Achievements[0]
	if !ach.Completed {
		t.Fatal("el logro no se marcó en disco")
	}
	if ach.UnlockedAt == nil {
		t.Fatal("falta unlocked_at")
	}
	if ach.Source != model.SourceManual {
		t.Fatalf("source = %q", ach.Source)
	}
	if saved.LastUpdated.IsZero() {
		t.Fatal("falta last_updated")
	}

	// Desmarcar debe limpiar la auditoría.
	m.toggleCurrent()
	saved, err = store.LoadProgress(49520)
	if err != nil {
		t.Fatal(err)
	}
	ach = saved.Achievements[0]
	if ach.Completed || ach.UnlockedAt != nil || ach.Source != "" {
		t.Fatalf("al desmarcar debe limpiarse todo: %+v", ach)
	}
}

func TestVisibleIndexesFilterAndAccentInsensitiveSearch(t *testing.T) {
	m := Model{
		Game: model.GameProgress{
			Achievements: []model.Achievement{
				{DisplayName: "Matadragones"},
				{DisplayName: "Dragón", Completed: true},
				{DisplayName: "Otra cosa"},
			},
		},
		Filter: FilterAll,
		Search: textinput.New(),
	}

	if got := len(m.visibleIndexes()); got != 3 {
		t.Fatalf("filtro todos = %d, quiero 3", got)
	}

	m.Filter = FilterPending
	if got := len(m.visibleIndexes()); got != 2 {
		t.Fatalf("filtro pendientes = %d, quiero 2", got)
	}

	m.Filter = FilterCompleted
	if got := len(m.visibleIndexes()); got != 1 {
		t.Fatalf("filtro completados = %d, quiero 1", got)
	}

	m.Filter = FilterAll
	m.Search.SetValue("matadragón")
	indexes := m.visibleIndexes()
	if len(indexes) != 1 || indexes[0] != 0 {
		t.Fatalf("la búsqueda con acento falló: %v", indexes)
	}
}

func TestStaleGameReadyIsIgnored(t *testing.T) {
	m := Model{Screen: ScreenAppID, seq: 2}

	updated, _ := m.Update(gameReadyMsg{Seq: 1, Progress: model.GameProgress{AppID: 1}})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update devolvió %T", updated)
	}
	if got.Screen != ScreenAppID {
		t.Fatal("una respuesta obsoleta no debe cambiar de pantalla")
	}
}

func readyAutoModel(t *testing.T) (Model, *storage.Storage) {
	t.Helper()
	store := newTestStore(t)
	game := model.GameProgress{
		AppID: 49520,
		Title: "Juego",
		Achievements: []model.Achievement{
			{APIName: "A", DisplayName: "A"},
			{APIName: "B", DisplayName: "B"},
		},
	}
	if err := store.SaveProgress(game); err != nil {
		t.Fatal(err)
	}

	m := Model{
		Screen:  ScreenReady,
		AppID:   49520,
		Game:    game,
		Store:   store,
		Config:  config.Config{AutoTrack: true, SteamID64: "76561198000000000", PollInterval: 30},
		Tracker: autotrack.New(),
		seq:     1,
	}
	return m, store
}

func TestReferenceSeedsBaselineWithoutUnlocking(t *testing.T) {
	m, store := readyAutoModel(t)
	states := map[string]steam.PlayerAchievement{
		"A": {Unlocked: true, UnlockTime: time.Unix(1700000000, 0)},
		"B": {},
	}

	updated, cmd := m.Update(referenceMsg{Seq: 1, AppID: 49520, States: states})
	got := updated.(Model)

	if got.Game.CompletedCount() != 0 {
		t.Fatalf("el baseline no debe desbloquear nada: %+v", got.Game.Achievements)
	}
	if !got.AutoActive {
		t.Fatal("el auto-track debería quedar activo")
	}
	if !got.Reference["A"] {
		t.Fatal("el badge de Steam debería reflejar A")
	}
	if got.LastPoll.IsZero() {
		t.Fatal("debería registrarse la hora de la última sincronización")
	}
	if cmd == nil {
		t.Fatal("debería programarse el siguiente sondeo")
	}

	saved, err := store.LoadProgress(49520)
	if err != nil {
		t.Fatal(err)
	}
	if saved.CompletedCount() != 0 {
		t.Fatal("el baseline no debe persistir logros como completados")
	}
}

func TestPollResultUnlocksTransitionWithSteamTime(t *testing.T) {
	m, store := readyAutoModel(t)
	m.AutoActive = true
	m.Tracker.Diff(map[string]autotrack.State{
		"A": {Unlocked: false},
		"B": {Unlocked: false},
	})

	unlockAt := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	states := map[string]steam.PlayerAchievement{
		"A": {Unlocked: true, UnlockTime: unlockAt},
		"B": {Unlocked: false},
	}

	updated, cmd := m.Update(pollResultMsg{Seq: 1, AppID: 49520, States: states})
	got := updated.(Model)

	if got.Game.CompletedCount() != 1 {
		t.Fatalf("completados = %d, quiero 1", got.Game.CompletedCount())
	}
	ach := got.Game.Achievements[0]
	if !ach.Completed || ach.Source != model.SourceSteam {
		t.Fatalf("logro mal marcado: %+v", ach)
	}
	if ach.UnlockedAt == nil || !ach.UnlockedAt.Equal(unlockAt) {
		t.Fatalf("unlocked_at = %v, quiero %v", ach.UnlockedAt, unlockAt)
	}
	if cmd == nil {
		t.Fatal("debería programarse el siguiente sondeo")
	}

	saved, err := store.LoadProgress(49520)
	if err != nil {
		t.Fatal(err)
	}
	if saved.CompletedCount() != 1 || saved.Achievements[0].Source != model.SourceSteam {
		t.Fatalf("no se persistió el desbloqueo: %+v", saved.Achievements[0])
	}
}

func TestManualUnmarkSurvivesNextPoll(t *testing.T) {
	m, _ := readyAutoModel(t)
	m.AutoActive = true
	unlockAt := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	states := map[string]steam.PlayerAchievement{
		"A": {Unlocked: true, UnlockTime: unlockAt},
		"B": {},
	}

	// Baseline pendiente y luego transición.
	m.Tracker.Diff(map[string]autotrack.State{"A": {Unlocked: false}, "B": {Unlocked: false}})
	updated, _ := m.Update(pollResultMsg{Seq: 1, AppID: 49520, States: states})
	m = updated.(Model)
	if m.Game.CompletedCount() != 1 {
		t.Fatal("el primer sondeo debía desbloquear A")
	}

	// Desmarque manual.
	m.toggleCurrent()
	if m.Game.CompletedCount() != 0 {
		t.Fatal("el desmarque manual falló")
	}

	// Steam sigue diciendo que A está desbloqueado: no debe revertirse.
	updated, _ = m.Update(pollResultMsg{Seq: 1, AppID: 49520, States: states})
	m = updated.(Model)
	if m.Game.CompletedCount() != 0 {
		t.Fatalf("el sondeo revirtió el desmarque manual: %+v", m.Game.Achievements)
	}
}

func TestPollDelayBacksOff(t *testing.T) {
	m := Model{Config: config.Config{PollInterval: 30}}
	tests := []struct {
		errors int
		want   time.Duration
	}{
		{errors: 0, want: 30 * time.Second},
		{errors: 1, want: 60 * time.Second},
		{errors: 2, want: 120 * time.Second},
		{errors: 3, want: 5 * time.Minute},
		{errors: 20, want: 5 * time.Minute},
	}
	for _, tt := range tests {
		m.PollErrors = tt.errors
		if got := m.pollDelay(); got != tt.want {
			t.Fatalf("%d errores → %v, quiero %v", tt.errors, got, tt.want)
		}
	}

	if got := (Model{Config: config.Config{PollInterval: 300}}).pollDelay(); got != 5*time.Minute {
		t.Fatalf("el backoff debe quedar acotado a 5 min, got %v", got)
	}
}

func TestPrivateProfileDisablesAutoTrack(t *testing.T) {
	m, _ := readyAutoModel(t)
	updated, cmd := m.Update(referenceMsg{Seq: 1, AppID: 49520, Err: steam.ErrPrivateProfile})
	got := updated.(Model)

	if got.AutoActive {
		t.Fatal("un perfil privado debe desactivar el auto-track")
	}
	if !strings.Contains(got.AutoWarning, "público") {
		t.Fatalf("aviso inesperado: %q", got.AutoWarning)
	}
	if cmd != nil {
		t.Fatal("no debe seguir sondeando un perfil privado")
	}
}

func TestSyncKeyConfirmsAndAdoptsSteamUnlocks(t *testing.T) {
	m, store := readyAutoModel(t)
	unlockAt := time.Date(2026, 11, 10, 22, 0, 0, 0, time.UTC)
	m.Steam = fakeSteam{player: func() (map[string]steam.PlayerAchievement, error) {
		return map[string]steam.PlayerAchievement{
			"A": {Unlocked: true, UnlockTime: unlockAt},
			"B": {Unlocked: false},
		}, nil
	}}

	updated, _ := m.Update(keyMsg("S"))
	m = updated.(Model)
	if !m.ConfirmSync {
		t.Fatal("S debe pedir confirmación")
	}

	updated, _ = m.Update(keyMsg("n"))
	m = updated.(Model)
	if m.ConfirmSync {
		t.Fatal("n debe cancelar la sincronización")
	}
	if m.Game.CompletedCount() != 0 {
		t.Fatal("cancelar no debe marcar nada")
	}

	updated, _ = m.Update(keyMsg("S"))
	m = updated.(Model)
	updated, cmd := m.Update(keyMsg("y"))
	m = updated.(Model)
	if m.ConfirmSync {
		t.Fatal("la confirmación debe cerrarse")
	}
	if cmd == nil {
		t.Fatal("y debe lanzar la consulta a Steam")
	}

	msg := cmd()
	syncMsg, ok := msg.(syncDoneMsg)
	if !ok {
		t.Fatalf("mensaje = %T, quiero syncDoneMsg", msg)
	}

	updated, _ = m.Update(syncMsg)
	m = updated.(Model)
	if m.Game.CompletedCount() != 1 {
		t.Fatalf("debería adoptar solo A, completados = %d", m.Game.CompletedCount())
	}
	ach := m.Game.Achievements[0]
	if ach.Source != model.SourceSteam || ach.UnlockedAt == nil || !ach.UnlockedAt.Equal(unlockAt) {
		t.Fatalf("adopción incorrecta: %+v", ach)
	}

	saved, err := store.LoadProgress(49520)
	if err != nil {
		t.Fatal(err)
	}
	if saved.CompletedCount() != 1 {
		t.Fatal("la adopción no se persistió")
	}
}

func TestManualToggleNotifiesAndPlaysSound(t *testing.T) {
	store := newTestStore(t)
	game := model.GameProgress{
		AppID:        1,
		Title:        "Juego",
		Achievements: []model.Achievement{{APIName: "A", DisplayName: "A"}},
	}
	if err := store.SaveProgress(game); err != nil {
		t.Fatal(err)
	}

	notif := &fakeNotifier{}
	snd := &fakeSound{}
	m := Model{
		Screen:   ScreenReady,
		AppID:    1,
		Game:     game,
		Store:    store,
		Config:   config.Config{Notify: true, Sound: true},
		Notifier: notif,
		Sound:    snd,
	}

	runCmd(t, m.toggleCurrent())

	if len(notif.names) != 1 || notif.names[0] != "A" {
		t.Fatalf("notificaciones = %v", notif.names)
	}
	if snd.plays != 1 {
		t.Fatalf("reproducciones = %d, quiero 1", snd.plays)
	}
}

func TestMultipleUnlocksUseSingleSummaryNotification(t *testing.T) {
	notif := &fakeNotifier{}
	snd := &fakeSound{}
	m := Model{
		Config:   config.Config{Notify: true, Sound: true},
		Notifier: notif,
		Sound:    snd,
	}

	cmd := m.achievementFeedback([]model.Achievement{
		{DisplayName: "A"},
		{DisplayName: "B"},
	})
	runCmd(t, cmd)

	if len(notif.names) != 1 || !strings.Contains(notif.names[0], "2 logros") {
		t.Fatalf("debería haber una sola notificación resumen: %v", notif.names)
	}
	if snd.plays != 1 {
		t.Fatalf("debería sonar una sola vez: %d", snd.plays)
	}
}

func TestStalePollResultIsIgnored(t *testing.T) {
	m, _ := readyAutoModel(t)
	m.AutoActive = true
	m.seq = 2
	m.Tracker.Diff(map[string]autotrack.State{"A": {Unlocked: false}, "B": {Unlocked: false}})

	updated, _ := m.Update(pollResultMsg{
		Seq:   1, // de una carga anterior
		AppID: 49520,
		States: map[string]steam.PlayerAchievement{
			"A": {Unlocked: true, UnlockTime: time.Now()},
			"B": {},
		},
	})
	got := updated.(Model)
	if got.Game.CompletedCount() != 0 {
		t.Fatal("un resultado de un sondeo viejo no debe aplicarse")
	}
}
