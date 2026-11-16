package tui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/Ssk372/featui/pkg/config"
	"github.com/Ssk372/featui/pkg/model"
	"github.com/Ssk372/featui/pkg/storage"
)

type fakeSteam struct {
	schema []model.Achievement
	title  string
	err    error
}

func (f fakeSteam) FetchGameSchema(context.Context, int, string) ([]model.Achievement, string, error) {
	return f.schema, f.title, f.err
}

func (f fakeSteam) FetchPlayerAchievements(context.Context, string, int) (map[string]bool, error) {
	return nil, errors.New("no usado en tests")
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
