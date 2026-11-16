package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvAPIKey, "")
}

func TestLoadCreatesDefaultConfig(t *testing.T) {
	setup(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Locale != DefaultLocale {
		t.Fatalf("locale = %q, quiero %q", cfg.Locale, DefaultLocale)
	}
	if cfg.MaxVisibleCount != DefaultMaxVisibleCount {
		t.Fatalf("max_visible_count = %d", cfg.MaxVisibleCount)
	}
	if !cfg.Notify {
		t.Fatal("notify debería estar activo por defecto")
	}
	if !cfg.AutoTrack {
		t.Fatal("auto_track debería estar activo por defecto")
	}
	if cfg.PollInterval != DefaultPollInterval {
		t.Fatalf("poll_interval = %d, quiero %d", cfg.PollInterval, DefaultPollInterval)
	}
	if !cfg.Sound {
		t.Fatal("sound debería estar activo por defecto")
	}
	if cfg.SoundFile != "" {
		t.Fatalf("sound_file = %q, quiero vacío", cfg.SoundFile)
	}
	if len(cfg.RecentAppIDs) != 0 {
		t.Fatalf("recent_app_ids = %v, quiero vacío", cfg.RecentAppIDs)
	}

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no se creó config.json: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permisos = %v, quiero 0600", info.Mode().Perm())
	}
}

func TestLoadMigratesDefaultAppID(t *testing.T) {
	setup(t)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "steam_api_key": "file-key",
  "steam_id_64": "",
  "default_app_id": 49520,
  "max_visible_count": 15,
  "watch_profile": true
}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cfg.RecentAppIDs) != 1 || cfg.RecentAppIDs[0] != 49520 {
		t.Fatalf("recent_app_ids = %v, quiero [49520]", cfg.RecentAppIDs)
	}
	if cfg.SteamAPIKey != "file-key" {
		t.Fatalf("se perdió la API key: %q", cfg.SteamAPIKey)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "default_app_id") {
		t.Fatal("la clave legada debería haberse eliminado del archivo")
	}
	if strings.Contains(string(raw), "watch_profile") {
		t.Fatal("watch_profile debería haberse eliminado del archivo")
	}
	if !strings.Contains(string(raw), "auto_track") {
		t.Fatal("deberían escribirse las claves nuevas")
	}
}

func TestEnvOverridesAPIKeyWithoutPersistingIt(t *testing.T) {
	setup(t)
	t.Setenv(EnvAPIKey, "env-key")

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"steam_api_key":"file-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.APIKey(); got != "env-key" {
		t.Fatalf("APIKey() = %q, quiero la del entorno", got)
	}
	if cfg.SteamAPIKey != "file-key" {
		t.Fatalf("la config en memoria no debe mezclarse con el entorno: %q", cfg.SteamAPIKey)
	}

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "env-key") {
		t.Fatal("la API key del entorno nunca debe escribirse en disco")
	}
}

func TestAddRecentAppID(t *testing.T) {
	cfg := Config{}

	for _, id := range []int{1, 2, 3, 4, 5, 6} {
		cfg.AddRecentAppID(id)
	}
	want := []int{6, 5, 4, 3, 2}
	if len(cfg.RecentAppIDs) != len(want) {
		t.Fatalf("recent_app_ids = %v, quiero %v", cfg.RecentAppIDs, want)
	}
	for i := range want {
		if cfg.RecentAppIDs[i] != want[i] {
			t.Fatalf("recent_app_ids = %v, quiero %v", cfg.RecentAppIDs, want)
		}
	}

	if cfg.AddRecentAppID(6) {
		t.Fatal("repetir el primero no debe cambiar la lista")
	}
	if !cfg.AddRecentAppID(2) {
		t.Fatal("mover un AppID existente al principio debe contar como cambio")
	}
	if cfg.RecentAppIDs[0] != 2 {
		t.Fatalf("el AppID reciente debería ir primero: %v", cfg.RecentAppIDs)
	}
}

func TestNormalizeDropsInvalidSteamID(t *testing.T) {
	cfg := Config{SteamID64: "123", Locale: "", MaxVisibleCount: -1}
	if !cfg.normalize() {
		t.Fatal("normalize debía reportar cambios")
	}
	if cfg.SteamID64 != "" {
		t.Fatalf("steam_id_64 inválido debería limpiarse, got %q", cfg.SteamID64)
	}
	if cfg.Locale != DefaultLocale || cfg.MaxVisibleCount != DefaultMaxVisibleCount {
		t.Fatal("normalize no aplicó los valores por defecto")
	}
}

func TestNormalizeClampsPollInterval(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{in: 0, want: DefaultPollInterval},
		{in: -5, want: DefaultPollInterval},
		{in: 2, want: MinPollInterval},
		{in: 60, want: 60},
		{in: 9999, want: MaxPollInterval},
	}
	for _, tt := range tests {
		cfg := Config{PollInterval: tt.in}
		cfg.normalize()
		if cfg.PollInterval != tt.want {
			t.Fatalf("poll_interval %d → %d, quiero %d", tt.in, cfg.PollInterval, tt.want)
		}
	}
}

func TestNormalizeTrimsSoundFile(t *testing.T) {
	cfg := Config{SoundFile: "  /tmp/a.ogg  "}
	cfg.normalize()
	if cfg.SoundFile != "/tmp/a.ogg" {
		t.Fatalf("sound_file = %q", cfg.SoundFile)
	}
}

func TestValidSteamID(t *testing.T) {
	if !ValidSteamID("76561198000000000") {
		t.Fatal("un SteamID64 de 17 dígitos debería ser válido")
	}
	for _, invalid := range []string{"", "123", "7656119800000000a", "765611980000000000"} {
		if ValidSteamID(invalid) {
			t.Fatalf("%q no debería ser válido", invalid)
		}
	}
}
