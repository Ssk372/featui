package tui

import (
	"context"

	"github.com/Ssk372/featui/pkg/model"
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
	FetchPlayerAchievements(ctx context.Context, steamID string, appID int) (map[string]bool, error)
}
