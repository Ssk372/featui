package steam

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

const twoUsersVDF = `"users"
{
	"76561197960287930"
	{
		"AccountName"		"secundaria"
		"PersonaName"		"Secundaria"
		"MostRecent"		"0"
		"Timestamp"		"1700000000"
	}
	"76561198000000001"
	{
		"AccountName"		"principal"
		"PersonaName"		"Principal"
		"MostRecent"		"1"
		"Timestamp"		"1700000100"
	}
}
`

func TestSteamConfigCandidatesPerOS(t *testing.T) {
	home := filepath.Join(t.TempDir(), "alguien")

	tests := []struct {
		name string
		goos string
		env  map[string]string
		want []string
	}{
		{
			name: "linux",
			goos: "linux",
			want: []string{
				filepath.Join(home, ".local", "share", "Steam", "config", "loginusers.vdf"),
				filepath.Join(home, ".steam", "steam", "config", "loginusers.vdf"),
				filepath.Join(home, ".steam", "root", "config", "loginusers.vdf"),
				filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam", "config", "loginusers.vdf"),
			},
		},
		{
			name: "macos",
			goos: "darwin",
			want: []string{
				filepath.Join(home, "Library", "Application Support", "Steam", "config", "loginusers.vdf"),
			},
		},
		{
			name: "windows",
			goos: "windows",
			env: map[string]string{
				"ProgramFiles(x86)": `D:\Program Files (x86)`,
				"ProgramFiles":      `D:\Program Files`,
			},
			want: []string{
				filepath.Join(`D:\Program Files (x86)`, "Steam", "config", "loginusers.vdf"),
				filepath.Join(`D:\Program Files`, "Steam", "config", "loginusers.vdf"),
			},
		},
		{
			name: "windows sin variables de entorno",
			goos: "windows",
			want: []string{
				filepath.Join(`C:\Program Files (x86)`, "Steam", "config", "loginusers.vdf"),
				filepath.Join(`C:\Program Files`, "Steam", "config", "loginusers.vdf"),
			},
		},
		{
			name: "windows con variables repetidas",
			goos: "windows",
			env: map[string]string{
				"ProgramFiles(x86)": `C:\Steam`,
				"ProgramFiles":      `C:\Steam`,
			},
			want: []string{
				filepath.Join(`C:\Steam`, "Steam", "config", "loginusers.vdf"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := steamConfigCandidates(tt.goos, home, envFrom(tt.env))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("candidatos = %q, quiero %q", got, tt.want)
			}
			for _, path := range got {
				if path == "" {
					t.Fatal("no debe haber rutas vacías")
				}
			}
		})
	}
}

func TestSteamConfigCandidatesWithoutHome(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		if got := steamConfigCandidates(goos, "", envFrom(nil)); len(got) != 0 {
			t.Fatalf("%s sin home devolvió %q", goos, got)
		}
	}
}

func TestDetectSteamIDPrefersMostRecent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".local", "share", "Steam", "config", "loginusers.vdf")
	writeFile(t, path, twoUsersVDF)

	got, err := detectSteamID("linux", home, envFrom(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got != "76561198000000001" {
		t.Fatalf("steamid = %q, quiero la cuenta MostRecent", got)
	}
}

func TestDetectSteamIDFallsBackToFirstUser(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".steam", "steam", "config", "loginusers.vdf")
	writeFile(t, path, `"users"
{
	"76561197960287930"
	{
		"AccountName"		"unica"
	}
}
`)

	got, err := detectSteamID("linux", home, envFrom(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got != "76561197960287930" {
		t.Fatalf("steamid = %q", got)
	}
}

func TestDetectSteamIDUsesFlatpakPath(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam", "config", "loginusers.vdf")
	writeFile(t, path, twoUsersVDF)

	got, err := detectSteamID("linux", home, envFrom(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got != "76561198000000001" {
		t.Fatalf("steamid = %q", got)
	}
}

func TestDetectSteamIDUsesMacOSPath(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "Library", "Application Support", "Steam", "config", "loginusers.vdf")
	writeFile(t, path, twoUsersVDF)

	got, err := detectSteamID("darwin", home, envFrom(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got != "76561198000000001" {
		t.Fatalf("steamid = %q", got)
	}
}

func TestDetectSteamIDUsesWindowsProgramFiles(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "Steam", "config", "loginusers.vdf")
	writeFile(t, path, twoUsersVDF)

	env := envFrom(map[string]string{
		"ProgramFiles(x86)": base,
		"ProgramFiles":      filepath.Join(base, "otro"),
	})
	got, err := detectSteamID("windows", `C:\Users\alguien`, env)
	if err != nil {
		t.Fatal(err)
	}
	if got != "76561198000000001" {
		t.Fatalf("steamid = %q", got)
	}
}

func TestDetectSteamIDMissingFile(t *testing.T) {
	if _, err := detectSteamID("linux", t.TempDir(), envFrom(nil)); err == nil {
		t.Fatal("sin loginusers.vdf debería devolver error")
	}
}

func TestDetectSteamIDIgnoresInvalidVDF(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".local", "share", "Steam", "config", "loginusers.vdf")
	writeFile(t, path, `"users" { }`)

	if _, err := detectSteamID("linux", home, envFrom(nil)); err == nil {
		t.Fatal("un VDF sin cuentas debería devolver error")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
