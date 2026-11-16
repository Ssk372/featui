// Comando featui: rastreador local de logros de Steam para la terminal.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ssk372/featui/pkg/config"
	"github.com/Ssk372/featui/pkg/notifier"
	"github.com/Ssk372/featui/pkg/steam"
	"github.com/Ssk372/featui/pkg/storage"
	"github.com/Ssk372/featui/pkg/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error al cargar la configuración: %w", err)
	}

	// El SteamID64 solo se usa para el badge informativo de Steam. Si no está
	// configurado, se intenta detectar de la instalación local sin molestar.
	if cfg.SteamID64 == "" {
		if detected, err := steam.DetectLocalSteamID(); err == nil {
			cfg.SteamID64 = detected
			_ = cfg.Save()
		}
	}

	store, err := storage.New()
	if err != nil {
		return fmt.Errorf("error al inicializar el almacenamiento local: %w", err)
	}

	client := steam.NewClient(cfg.APIKey())
	notif := notifier.New("FeatUI")

	program := tea.NewProgram(tui.NewModel(cfg, store, client, notif), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("error ejecutando FeatUI: %w", err)
	}
	return nil
}
