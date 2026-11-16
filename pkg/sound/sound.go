// Package sound reproduce un aviso sonoro al desbloquear logros.
//
// El objetivo es ser portable: en Linux usa PipeWire/PulseAudio/Canberra y en
// macOS/Windows cae a ffplay, mpv o al reproductor del sistema. Si no hay
// ningún reproductor o ningún archivo disponible, genera un chime WAV incluido
// en la caché para no depender de assets externos.
package sound

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// playTimeout evita que un reproductor colgado bloquee la TUI.
const playTimeout = 3 * time.Second

// Decisión de diseño: en Linux se prefiere un reproductor nativo del entorno
// (PipeWire, PulseAudio, Canberra) y luego los genéricos. En macOS y Windows se
// usan los reproductores multiplataforma o el del propio sistema.
var (
	goos       = runtime.GOOS
	lookPath   = exec.LookPath
	fileExists = func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	}
)

type backend struct {
	name string
	args func(file string) []string
}

// Play reproduce un archivo de audio con un timeout corto. Es seguro llamarlo
// desde varias goroutines: si ya hay un sonido sonando, se ignora el nuevo para
// no solapar avisos.
func (p *Player) Play() error {
	if p == nil || p.backend.name == "" || p.file == "" {
		return errors.New("no hay reproductor de sonido disponible")
	}
	if !p.mu.TryLock() {
		return nil
	}
	defer p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), playTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.backend.name, p.backend.args(p.file)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("el reproductor %s no terminó a tiempo", p.backend.name)
		}
		return fmt.Errorf("no se pudo reproducir el sonido con %s: %w", p.backend.name, err)
	}
	return nil
}

// Player es el reproductor resuelto para este equipo.
type Player struct {
	backend backend
	file    string
	warning string
	mu      sync.Mutex
}

// New resuelve el reproductor y el archivo de sonido.
//
// Prioridad del archivo:
//  1. customFile si está definido y existe.
//  2. El sonido del tema del sistema para el SO actual.
//  3. Un chime WAV generado en la caché de FeatUI.
//
// Nunca devuelve error: si no hay manera de sonar, Warning() lo explica y el
// resto de FeatUI sigue funcionando.
func New(customFile string) *Player {
	p := &Player{}

	if found, ok := resolveBackend(); ok {
		p.backend = found
	}

	if custom := expandHome(strings.TrimSpace(customFile)); custom != "" {
		switch {
		case fileExists(custom):
			p.file = custom
		default:
			p.warning = fmt.Sprintf("sound_file no existe (%s); usando el sonido por defecto", custom)
		}
	}

	if p.file == "" {
		if def, ok := systemDefaultFile(); ok {
			p.file = def
		}
	}

	if p.file == "" {
		chime, err := ensureChime()
		switch {
		case err != nil:
			p.warning = "no se encontró ningún sonido y no se pudo generar el chime: " + err.Error()
		default:
			p.file = chime
		}
	}

	if p.backend.name == "" && p.warning == "" {
		p.warning = "no hay ningún reproductor de audio disponible; los avisos sonoros están desactivados"
	}
	return p
}

// Available indica si hay reproductor y archivo listos.
func (p *Player) Available() bool {
	return p != nil && p.backend.name != "" && p.file != ""
}

// PlayerName devuelve el reproductor detectado (vacío si no hay).
func (p *Player) PlayerName() string {
	if p == nil {
		return ""
	}
	return p.backend.name
}

// File devuelve el archivo que se reproducirá.
func (p *Player) File() string {
	if p == nil {
		return ""
	}
	return p.file
}

// Warning devuelve un aviso si algo no está disponible, o "" si todo bien.
func (p *Player) Warning() string {
	if p == nil {
		return ""
	}
	return p.warning
}

// resolveBackend elige el primer reproductor instalado para el SO actual.
func resolveBackend() (backend, bool) {
	for _, candidate := range candidates(goos) {
		if _, err := lookPath(candidate.name); err == nil {
			return candidate, true
		}
	}
	return backend{}, false
}

func candidates(osName string) []backend {
	generic := []backend{
		{name: "ffplay", args: func(f string) []string {
			return []string{"-nodisp", "-autoexit", "-loglevel", "quiet", f}
		}},
		{name: "mpv", args: func(f string) []string {
			return []string{"--no-video", "--really-quiet", f}
		}},
	}

	switch osName {
	case "linux":
		return append([]backend{
			{name: "pw-play", args: func(f string) []string { return []string{f} }},
			{name: "paplay", args: func(f string) []string { return []string{f} }},
			{name: "canberra-gtk-play", args: func(f string) []string { return []string{"-f", f} }},
		}, generic...)
	case "darwin":
		return append([]backend{
			{name: "afplay", args: func(f string) []string { return []string{f} }},
		}, generic...)
	case "windows":
		return append(append([]backend(nil), generic...), backend{
			name: "powershell",
			args: func(f string) []string {
				script := fmt.Sprintf("(New-Object Media.SoundPlayer '%s').PlaySync()", strings.ReplaceAll(f, "'", "''"))
				return []string{"-NoProfile", "-NonInteractive", "-Command", script}
			},
		})
	default:
		return generic
	}
}

// systemDefaultFile devuelve el aviso del tema del sistema si existe.
func systemDefaultFile() (string, bool) {
	for _, path := range defaultFiles(goos) {
		if fileExists(path) {
			return path, true
		}
	}
	return "", false
}

func defaultFiles(osName string) []string {
	switch osName {
	case "linux":
		return []string{
			"/usr/share/sounds/freedesktop/stereo/complete.oga",
			"/usr/share/sounds/freedesktop/stereo/bell.oga",
			"/usr/share/sounds/freedesktop/stereo/message.oga",
			"/usr/share/sounds/sound-icons/prompt.wav",
		}
	case "darwin":
		return []string{
			"/System/Library/Sounds/Glass.aiff",
			"/System/Library/Sounds/Ping.aiff",
			"/System/Library/Sounds/Pop.aiff",
		}
	case "windows":
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return []string{
			filepath.Join(root, "Media", "Windows Notify System Generic.wav"),
			filepath.Join(root, "Media", "chimes.wav"),
			filepath.Join(root, "Media", "Windows Ding.wav"),
		}
	default:
		return nil
	}
}

func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func cacheDir() string {
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "featui")
	}
	return filepath.Join(os.TempDir(), "featui")
}

// ensureChime genera (una sola vez) un WAV corto de dos notas.
func ensureChime() (string, error) {
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "notify.wav")
	if fileExists(path) {
		return path, nil
	}

	data := chimeWAV()
	tmp, err := os.CreateTemp(dir, ".chime-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	return path, nil
}

// chimeWAV construye un WAV PCM mono 16 bits con dos notas y un envelope suave
// para evitar clics. No depende de archivos ni licencias externas.
func chimeWAV() []byte {
	const (
		rate     = 44100
		duration = 0.45
		channels = 1
		bits     = 16
	)
	total := int(rate * duration)
	samples := make([]int16, total)

	for i := 0; i < total; i++ {
		t := float64(i) / rate
		var value float64

		// Primera nota (E6) y segunda nota (A6) con solape breve.
		if t < 0.28 {
			value += 0.55 * math.Sin(2*math.Pi*1318.51*t) * envelope(t, 0.28)
		}
		if t >= 0.12 {
			local := t - 0.12
			value += 0.45 * math.Sin(2*math.Pi*1760*local) * envelope(local, 0.33)
		}

		if value > 1 {
			value = 1
		}
		if value < -1 {
			value = -1
		}
		samples[i] = int16(value * 0.85 * math.MaxInt16)
	}

	dataSize := len(samples) * 2
	buf := make([]byte, 44+dataSize)

	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // tamaño del bloque fmt
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:24], channels)
	binary.LittleEndian.PutUint32(buf[24:28], rate)
	binary.LittleEndian.PutUint32(buf[28:32], rate*channels*bits/8) // byte rate
	binary.LittleEndian.PutUint16(buf[32:34], channels*bits/8)      // block align
	binary.LittleEndian.PutUint16(buf[34:36], bits)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))

	for i, sample := range samples {
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(sample))
	}
	return buf
}

// envelope aplica un ataque rápido y una caída exponencial.
func envelope(t, length float64) float64 {
	if t < 0 || length <= 0 {
		return 0
	}
	attack := 0.006
	if t < attack {
		return t / attack
	}
	return math.Exp(-5 * (t - attack) / length)
}
