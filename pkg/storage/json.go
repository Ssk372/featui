// Package storage guarda el progreso local y la caché de Steam en XDG.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Ssk372/featui/internal/fsutil"
	"github.com/Ssk372/featui/pkg/model"
)

// Storage encapsula los directorios de datos y caché de FeatUI.
type Storage struct {
	dataDir  string
	cacheDir string
}

// CachedSchema es el esquema de logros guardado en caché.
type CachedSchema struct {
	AppID        int                 `json:"app_id"`
	Locale       string              `json:"locale"`
	Title        string              `json:"title"`
	FetchedAt    time.Time           `json:"fetched_at"`
	Achievements []model.Achievement `json:"achievements"`
}

// New crea (si hace falta) los directorios de datos y caché.
//
// El progreso va a <datos>/featui/progress y la caché a <caché>/featui. En
// Linux respeta XDG_DATA_HOME; en macOS usa ~/Library/Application Support y en
// Windows %LocalAppData%.
func New() (*Storage, error) {
	dataHome, err := dataHome()
	if err != nil {
		return nil, err
	}

	cacheHome, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("no se pudo localizar el directorio de caché: %w", err)
	}

	s := &Storage{
		dataDir:  filepath.Join(dataHome, "featui", "progress"),
		cacheDir: filepath.Join(cacheHome, "featui"),
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return nil, err
	}
	return s, nil
}

// dataHome resuelve el directorio base de datos según el sistema operativo.
func dataHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo localizar el home del usuario: %w", err)
	}
	return dataHomeFor(runtime.GOOS, home), nil
}

// dataHomeFor es la parte pura de dataHome, fácil de testear.
func dataHomeFor(osName, home string) string {
	switch osName {
	case "windows":
		if local := os.Getenv("LocalAppData"); local != "" {
			return local
		}
		return filepath.Join(home, "AppData", "Local")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return xdg
		}
		return filepath.Join(home, ".local", "share")
	}
}

// DataDir devuelve el directorio donde viven los progresos.
func (s *Storage) DataDir() string { return s.dataDir }

// CacheDir devuelve el directorio de caché.
func (s *Storage) CacheDir() string { return s.cacheDir }

// ProgressPath devuelve la ruta del JSON de progreso de un juego.
func (s *Storage) ProgressPath(appID int) string {
	return filepath.Join(s.dataDir, fmt.Sprintf("%d.json", appID))
}

func (s *Storage) schemaPath(appID int, locale string) string {
	return filepath.Join(s.cacheDir, fmt.Sprintf("schema_%d_%s.json", appID, sanitizeLocale(locale)))
}

func sanitizeLocale(locale string) string {
	if locale == "" {
		return "default"
	}
	out := make([]rune, 0, len(locale))
	for _, r := range locale {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

// LoadProgress lee el progreso local. Devuelve os.ErrNotExist envuelto si el
// juego todavía no tiene archivo, para que el llamante pueda distinguirlo de un
// archivo corrupto.
func (s *Storage) LoadProgress(appID int) (model.GameProgress, error) {
	path := s.ProgressPath(appID)
	data, err := os.ReadFile(path)
	if err != nil {
		return model.GameProgress{}, err
	}

	var progress model.GameProgress
	if err := json.Unmarshal(data, &progress); err != nil {
		return model.GameProgress{}, fmt.Errorf("progreso local ilegible (%s): %w", path, err)
	}
	if progress.AppID == 0 {
		progress.AppID = appID
	}
	return progress, nil
}

// SaveProgress escribe el progreso de forma atómica y conserva una copia del
// archivo anterior en <appid>.json.bak antes de sobrescribir.
func (s *Storage) SaveProgress(progress model.GameProgress) error {
	if progress.AppID <= 0 {
		return errors.New("no se puede guardar un progreso sin AppID")
	}

	path := s.ProgressPath(progress.AppID)
	if _, err := os.Stat(path); err == nil {
		// Un fallo al hacer la copia no debe impedir guardar el progreso.
		_ = fsutil.CopyFile(path, path+".bak")
	}

	data, err := json.MarshalIndent(progress, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.WriteFileAtomic(path, data, 0o644)
}

// ResetProgress deja todos los logros del juego pendientes y devuelve una copia
// del progreso anterior en el .bak.
func (s *Storage) ResetProgress(appID int) (model.GameProgress, string, error) {
	progress, err := s.LoadProgress(appID)
	if err != nil {
		return model.GameProgress{}, "", err
	}

	for i := range progress.Achievements {
		progress.Achievements[i].Completed = false
		progress.Achievements[i].UnlockedAt = nil
		progress.Achievements[i].Source = ""
	}
	progress.LastUpdated = time.Now()

	if err := s.SaveProgress(progress); err != nil {
		return model.GameProgress{}, "", err
	}
	return progress, s.ProgressPath(appID) + ".bak", nil
}

// LoadSchema lee el esquema cacheado. El booleano indica si existía caché.
func (s *Storage) LoadSchema(appID int, locale string) (CachedSchema, bool, error) {
	path := s.schemaPath(appID, locale)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return CachedSchema{}, false, nil
	}
	if err != nil {
		return CachedSchema{}, false, err
	}

	var cached CachedSchema
	if err := json.Unmarshal(data, &cached); err != nil {
		return CachedSchema{}, false, fmt.Errorf("caché de logros ilegible (%s): %w", path, err)
	}
	if len(cached.Achievements) == 0 {
		return CachedSchema{}, false, nil
	}
	return cached, true, nil
}

// SaveSchema guarda el esquema en caché de forma atómica.
func (s *Storage) SaveSchema(appID int, locale string, cached CachedSchema) error {
	if appID <= 0 {
		return errors.New("no se puede cachear un esquema sin AppID")
	}
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.WriteFileAtomic(s.schemaPath(appID, locale), data, 0o644)
}
