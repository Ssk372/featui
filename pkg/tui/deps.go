package tui

import (
	"context"

	"github.com/Ssk372/featui/pkg/model"
	"github.com/Ssk372/featui/pkg/steam"
	"github.com/Ssk372/featui/pkg/storage"
)

// Store es la parte de almacenamiento que usa la TUI.
type Store interface {
	LoadProgress(appID int) (model.GameProgress, error)
	SaveProgress(progress model.GameProgress) error
	ResetProgress(appID int) (model.GameProgress, string, error)
	LoadSchema(appID int, locale string) (storage.CachedSchema, bool, error)
	SaveSchema(appID int, locale string, cached storage.CachedSchema) error
}

// SteamAPI es la parte del cliente de Steam que usa la TUI.
type SteamAPI interface {
	FetchGameSchema(ctx context.Context, appID int, locale string) ([]model.Achievement, string, error)
	FetchPlayerAchievements(ctx context.Context, steamID string, appID int) (map[string]steam.PlayerAchievement, error)
}

// Notifier envía notificaciones de escritorio. La TUI solo depende de esta
// interfaz para poder añadir backends de macOS/Windows sin tocar la lógica.
type Notifier interface {
	SendAchievementNotification(gameTitle, achievementName, description, iconURL, cacheKey string) error
}

// SoundPlayer reproduce el aviso sonoro. Se ejecuta siempre en un tea.Cmd para
// no bloquear la interfaz.
type SoundPlayer interface {
	Play() error
	// Warning explica por qué el sonido no está disponible o qué fallback se
	// está usando. Cadena vacía si todo va bien.
	Warning() string
}
