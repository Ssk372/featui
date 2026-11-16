package sound

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func stubLookPath(t *testing.T, available ...string) {
	t.Helper()
	original := lookPath
	set := make(map[string]bool, len(available))
	for _, name := range available {
		set[name] = true
	}
	lookPath = func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { lookPath = original })
}

func stubExists(t *testing.T, existing ...string) {
	t.Helper()
	original := fileExists
	set := make(map[string]bool, len(existing))
	for _, path := range existing {
		set[path] = true
	}
	fileExists = func(path string) bool { return set[path] }
	t.Cleanup(func() { fileExists = original })
}

func TestResolveBackendPrefersNativeOnLinux(t *testing.T) {
	stubLookPath(t, "paplay", "ffplay", "mpv")
	goos = "linux"
	t.Cleanup(func() { goos = "linux" })

	got, ok := resolveBackend()
	if !ok || got.name != "paplay" {
		t.Fatalf("backend = %q ok=%v, quiero paplay", got.name, ok)
	}
}

func TestResolveBackendFallsBackToGeneric(t *testing.T) {
	stubLookPath(t, "mpv")
	goos = "darwin"
	t.Cleanup(func() { goos = "darwin" })

	// En macOS existen afplay y ffplay; sin ellos debe quedar mpv.
	got, ok := resolveBackend()
	if !ok || got.name != "mpv" {
		t.Fatalf("backend = %q ok=%v, quiero mpv", got.name, ok)
	}
}

func TestResolveBackendWithoutPlayers(t *testing.T) {
	stubLookPath(t)
	goos = "linux"
	t.Cleanup(func() { goos = "linux" })

	if got, ok := resolveBackend(); ok {
		t.Fatalf("no debería haber backend, got %q", got.name)
	}
}

func TestNewUsesCustomFileWhenItExists(t *testing.T) {
	stubLookPath(t, "pw-play")
	stubExists(t, "/tmp/mi-sonido.ogg")
	goos = "linux"
	t.Cleanup(func() { goos = "linux" })
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	p := New("/tmp/mi-sonido.ogg")
	if p.File() != "/tmp/mi-sonido.ogg" {
		t.Fatalf("file = %q", p.File())
	}
	if p.Warning() != "" {
		t.Fatalf("warning inesperado: %s", p.Warning())
	}
	if !p.Available() {
		t.Fatal("debería estar disponible")
	}
}

func TestNewWarnsAndFallsBackWhenCustomFileMissing(t *testing.T) {
	stubLookPath(t, "pw-play")
	stubExists(t, "/usr/share/sounds/freedesktop/stereo/complete.oga")
	goos = "linux"
	t.Cleanup(func() { goos = "linux" })
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	p := New("/no/existe.ogg")
	if p.File() != "/usr/share/sounds/freedesktop/stereo/complete.oga" {
		t.Fatalf("file = %q, quiero el del sistema", p.File())
	}
	if p.Warning() == "" {
		t.Fatal("debería avisar de que el archivo personalizado no existe")
	}
}

func TestNewGeneratesChimeWhenNothingExists(t *testing.T) {
	stubLookPath(t, "pw-play")
	stubExists(t) // ni archivo propio ni sonidos del sistema
	goos = "linux"
	t.Cleanup(func() { goos = "linux" })

	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	p := New("")
	want := filepath.Join(cache, "featui", "notify.wav")
	if p.File() != want {
		t.Fatalf("file = %q, quiero %q", p.File(), want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("no se generó el chime: %v", err)
	}
}

func TestNewWithoutPlayerWarns(t *testing.T) {
	stubLookPath(t)
	stubExists(t, "/usr/share/sounds/freedesktop/stereo/complete.oga")
	goos = "linux"
	t.Cleanup(func() { goos = "linux" })

	p := New("")
	if p.Available() {
		t.Fatal("no debería estar disponible sin reproductor")
	}
	if p.Warning() == "" {
		t.Fatal("debería avisar de que no hay reproductor")
	}
}

func TestChimeIsValidWAV(t *testing.T) {
	data := chimeWAV()
	if len(data) < 44 {
		t.Fatalf("WAV demasiado corto: %d bytes", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatal("cabecera RIFF/WAVE incorrecta")
	}
	if got := string(data[12:16]); got != "fmt " {
		t.Fatalf("bloque fmt = %q", got)
	}
	if format := binary.LittleEndian.Uint16(data[20:22]); format != 1 {
		t.Fatalf("formato = %d, quiero PCM", format)
	}
	if rate := binary.LittleEndian.Uint32(data[24:28]); rate != 44100 {
		t.Fatalf("sample rate = %d", rate)
	}
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	if int(dataSize)+44 != len(data) {
		t.Fatalf("data size = %d, len = %d", dataSize, len(data))
	}
	if string(data[36:40]) != "data" {
		t.Fatal("falta el bloque data")
	}
}

func TestPlayWithoutBackendFails(t *testing.T) {
	p := &Player{}
	if err := p.Play(); err == nil {
		t.Fatal("Play sin reproductor debe fallar")
	}
}

func TestPlayRunsBackend(t *testing.T) {
	p := &Player{
		backend: backend{name: "true", args: func(string) []string { return nil }},
		file:    "/tmp/x.wav",
	}
	if err := p.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}
}
