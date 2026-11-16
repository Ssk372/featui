package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Ssk372/featui/pkg/model"
)

func setup(t *testing.T) *Storage {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))

	store, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return store
}

func sampleProgress() model.GameProgress {
	unlocked := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	return model.GameProgress{
		AppID: 49520,
		Title: "Borderlands 2",
		Achievements: []model.Achievement{
			{APIName: "Achievement_1", DisplayName: "Uno", Completed: true, UnlockedAt: &unlocked, Source: model.SourceManual},
			{APIName: "Achievement_2", DisplayName: "Dos"},
		},
		LastUpdated: unlocked,
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	store := setup(t)
	progress := sampleProgress()

	if err := store.SaveProgress(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	got, err := store.LoadProgress(progress.AppID)
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}
	if !reflect.DeepEqual(got, progress) {
		t.Fatalf("roundtrip distinto:\n got %+v\nwant %+v", got, progress)
	}

	if _, err := os.Stat(store.ProgressPath(progress.AppID)); err != nil {
		t.Fatalf("no existe el archivo de progreso: %v", err)
	}
}

func TestSaveCreatesBackupOfPreviousVersion(t *testing.T) {
	store := setup(t)
	progress := sampleProgress()

	if err := store.SaveProgress(progress); err != nil {
		t.Fatal(err)
	}

	progress.Achievements[1].Completed = true
	if err := store.SaveProgress(progress); err != nil {
		t.Fatal(err)
	}

	backupPath := store.ProgressPath(progress.AppID) + ".bak"
	raw, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("no se creó el .bak: %v", err)
	}

	var backup model.GameProgress
	if err := json.Unmarshal(raw, &backup); err != nil {
		t.Fatalf(".bak ilegible: %v", err)
	}
	if backup.Achievements[1].Completed {
		t.Fatal("el .bak debe contener la versión anterior (sin el segundo logro)")
	}
}

func TestLoadCorruptFileDoesNotLoseItOnSave(t *testing.T) {
	store := setup(t)
	path := store.ProgressPath(49520)
	if err := os.WriteFile(path, []byte("{esto no es json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := store.LoadProgress(49520); err == nil {
		t.Fatal("LoadProgress debía fallar con un JSON corrupto")
	}

	progress := sampleProgress()
	if err := store.SaveProgress(progress); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	raw, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("el archivo corrupto debería conservarse en .bak: %v", err)
	}
	if !strings.Contains(string(raw), "esto no es json") {
		t.Fatal("el .bak no contiene el archivo corrupto original")
	}
}

func TestResetProgress(t *testing.T) {
	store := setup(t)
	progress := sampleProgress()
	if err := store.SaveProgress(progress); err != nil {
		t.Fatal(err)
	}

	reset, backup, err := store.ResetProgress(progress.AppID)
	if err != nil {
		t.Fatalf("ResetProgress: %v", err)
	}
	if reset.CompletedCount() != 0 {
		t.Fatalf("quedaron %d logros completados", reset.CompletedCount())
	}
	for _, ach := range reset.Achievements {
		if ach.UnlockedAt != nil || ach.Source != "" {
			t.Fatalf("el logro %s conserva auditoría tras el reset", ach.APIName)
		}
	}
	if backup == "" {
		t.Fatal("ResetProgress debe devolver la ruta del backup")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("no existe el backup %s: %v", backup, err)
	}

	reloaded, err := store.LoadProgress(progress.AppID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CompletedCount() != 0 {
		t.Fatal("el reset no se persistió")
	}
}

func TestSchemaCache(t *testing.T) {
	store := setup(t)

	if _, ok, err := store.LoadSchema(49520, "spanish"); err != nil || ok {
		t.Fatalf("caché vacía: ok=%v err=%v", ok, err)
	}

	cached := CachedSchema{
		AppID:     49520,
		Locale:    "spanish",
		Title:     "Borderlands 2",
		FetchedAt: time.Now(),
		Achievements: []model.Achievement{
			{APIName: "Achievement_1", DisplayName: "Uno"},
		},
	}
	if err := store.SaveSchema(49520, "spanish", cached); err != nil {
		t.Fatalf("SaveSchema: %v", err)
	}

	got, ok, err := store.LoadSchema(49520, "spanish")
	if err != nil || !ok {
		t.Fatalf("LoadSchema: ok=%v err=%v", ok, err)
	}
	if got.Title != cached.Title || len(got.Achievements) != 1 {
		t.Fatalf("caché distinta: %+v", got)
	}

	// Un esquema sin logros no debe considerarse caché válida.
	empty := CachedSchema{AppID: 1, Locale: "spanish", FetchedAt: time.Now()}
	if err := store.SaveSchema(1, "spanish", empty); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.LoadSchema(1, "spanish"); ok {
		t.Fatal("una caché sin logros no debe usarse")
	}
}

func TestPathsRespectXDG(t *testing.T) {
	store := setup(t)
	if !strings.Contains(store.DataDir(), "data") {
		t.Fatalf("el directorio de datos no respeta XDG_DATA_HOME: %s", store.DataDir())
	}
	if !strings.Contains(store.CacheDir(), "cache") {
		t.Fatalf("el directorio de caché no respeta XDG_CACHE_HOME: %s", store.CacheDir())
	}
}
