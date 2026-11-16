// Package model define las estructuras de datos del rastreador de logros.
package model

import "time"

// UnlockSource indica de dónde salió el marcado de un logro.
type UnlockSource string

const (
	// SourceManual lo marcó el usuario desde la TUI.
	SourceManual UnlockSource = "manual"
	// SourceWatcher lo marcó una detección automática (reservado).
	SourceWatcher UnlockSource = "watcher"
)

// Achievement es un logro con su estado local.
type Achievement struct {
	APIName     string       `json:"api_name"`
	DisplayName string       `json:"display_name"`
	Description string       `json:"description"`
	IconURL     string       `json:"icon_url,omitempty"`
	IconGrayURL string       `json:"icon_gray_url,omitempty"`
	Hidden      bool         `json:"hidden,omitempty"`
	Completed   bool         `json:"completed"`
	UnlockedAt  *time.Time   `json:"unlocked_at,omitempty"`
	Source      UnlockSource `json:"source,omitempty"`
}

// GameProgress es el progreso local de un juego concreto.
type GameProgress struct {
	AppID        int           `json:"app_id"`
	Title        string        `json:"title"`
	Achievements []Achievement `json:"achievements"`
	LastUpdated  time.Time     `json:"last_updated"`
}

// Total devuelve el número de logros del juego.
func (g GameProgress) Total() int { return len(g.Achievements) }

// CompletedCount devuelve cuántos logros están marcados.
func (g GameProgress) CompletedCount() int {
	done := 0
	for _, a := range g.Achievements {
		if a.Completed {
			done++
		}
	}
	return done
}
