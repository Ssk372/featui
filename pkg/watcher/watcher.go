// Package watcher vigila el archivo de guardado de juegos soportados para
// avisar a la TUI de que el juego escribió progreso nuevo.
//
// Importante: el watcher NUNCA marca logros. El formato de guardado es
// propietario de cada juego, así que solo puede avisar de que hubo un guardado;
// la TUI decide qué hacer con esa señal.
package watcher

import (
	"context"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// settleDelay es la espera tras detectar un cambio de mtime para que el juego
// termine de escribir (suele escribir varias veces seguidas).
const settleDelay = 2 * time.Second

// Msg avisa de que el juego escribió en su archivo de guardado.
type Msg struct {
	AppID int
}

// Watcher vigila un juego concreto.
type Watcher struct {
	appID int
	path  string
}

// adapter resuelve la ruta de guardado de un juego. Solo hay uno por ahora.
type adapter func() string

var adapters = map[int]adapter{
	49520: borderlands2ProfilePath, // Borderlands 2
}

// New crea un watcher para appID. Si el juego no tiene adaptador, Available
// devolverá false y Start no hará nada.
func New(appID int) *Watcher {
	w := &Watcher{appID: appID}
	if resolve, ok := adapters[appID]; ok {
		w.path = resolve()
	}
	return w
}

// Available indica si este juego tiene una ruta de guardado detectable.
func (w *Watcher) Available() bool { return w != nil && w.path != "" }

// Path devuelve la ruta vigilada (vacía si no hay).
func (w *Watcher) Path() string {
	if w == nil {
		return ""
	}
	return w.path
}

// Start devuelve un comando de Bubble Tea que espera a que el archivo cambie y
// emite un único Msg. El llamante debe volver a lanzarlo tras cada Msg.
func (w *Watcher) Start(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if !w.Available() {
			return nil
		}

		info, err := os.Stat(w.path)
		if err != nil {
			return nil
		}
		last := info.ModTime()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				info, err := os.Stat(w.path)
				if err != nil {
					continue
				}
				if !info.ModTime().After(last) {
					continue
				}
				last = info.ModTime()

				select {
				case <-ctx.Done():
					return nil
				case <-time.After(settleDelay):
				}
				return Msg{AppID: w.appID}
			}
		}
	}
}

// borderlands2ProfilePath busca el profile.bin tanto en la instalación nativa
// como en el prefix de Proton.
func borderlands2ProfilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	patterns := []string{
		filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata", "49520", "pfx", "drive_c", "users", "steamuser", "Documents", "My Games", "Borderlands 2", "WillowGame", "SaveData", "*", "profile.bin"),
		filepath.Join(home, ".steam", "steam", "steamapps", "compatdata", "49520", "pfx", "drive_c", "users", "steamuser", "Documents", "My Games", "Borderlands 2", "WillowGame", "SaveData", "*", "profile.bin"),
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}
