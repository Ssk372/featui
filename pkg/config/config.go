// Package config gestiona la configuración del usuario en XDG_CONFIG_HOME.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Ssk372/featui/internal/fsutil"
)

const (
	// DefaultLocale es el idioma con el que se piden los logros a Steam.
	DefaultLocale = "spanish"
	// DefaultMaxVisibleCount es cuántos logros se muestran de golpe.
	DefaultMaxVisibleCount = 15
	// MaxRecentAppIDs es cuántos juegos recientes se recuerdan.
	MaxRecentAppIDs = 5
	// EnvAPIKey permite sobreescribir la API key sin tocar config.json.
	EnvAPIKey = "FEATUI_STEAM_API_KEY"
)

var steamIDPattern = regexp.MustCompile(`^\d{17}$`)

// Config es la configuración persistente de FeatUI.
type Config struct {
	SteamAPIKey     string `json:"steam_api_key"`
	SteamID64       string `json:"steam_id_64,omitempty"`
	Locale          string `json:"locale"`
	MaxVisibleCount int    `json:"max_visible_count"`
	Notify          bool   `json:"notify"`
	WatchProfile    bool   `json:"watch_profile"`
	RecentAppIDs    []int  `json:"recent_app_ids"`
}

// fileConfig refleja config.json tal cual, incluida la clave legada
// "default_app_id" que existía cuando solo se soportaba Borderlands 2.
type fileConfig struct {
	SteamAPIKey     string `json:"steam_api_key"`
	SteamID64       string `json:"steam_id_64"`
	Locale          string `json:"locale"`
	MaxVisibleCount int    `json:"max_visible_count"`
	Notify          *bool  `json:"notify"`
	WatchProfile    *bool  `json:"watch_profile"`
	RecentAppIDs    []int  `json:"recent_app_ids"`
	DefaultAppID    int    `json:"default_app_id"`
}

// Dir devuelve el directorio de configuración de FeatUI.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo localizar el directorio de configuración: %w", err)
	}
	return filepath.Join(base, "featui"), nil
}

// Path devuelve la ruta de config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load lee la configuración, aplica valores por defecto y migra el formato
// antiguo. Si el archivo no existe, lo crea con una plantilla.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}

	cfg := defaults()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if saveErr := cfg.Save(); saveErr != nil {
			return cfg, fmt.Errorf("no se pudo crear %s: %w", path, saveErr)
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("no se pudo leer %s: %w", path, err)
	}

	var raw fileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, fmt.Errorf("config.json inválido (%s): %w", path, err)
	}

	changed := cfg.apply(raw)
	changed = cfg.normalize() || changed
	if changed {
		if err := cfg.Save(); err != nil {
			return cfg, fmt.Errorf("no se pudo actualizar %s: %w", path, err)
		}
	}
	return cfg, nil
}

func defaults() Config {
	return Config{
		Locale:          DefaultLocale,
		MaxVisibleCount: DefaultMaxVisibleCount,
		Notify:          true,
		WatchProfile:    false,
		RecentAppIDs:    []int{},
	}
}

func (c *Config) apply(raw fileConfig) bool {
	changed := false

	if raw.SteamAPIKey != "" {
		c.SteamAPIKey = strings.TrimSpace(raw.SteamAPIKey)
	}
	if raw.SteamID64 != "" {
		c.SteamID64 = strings.TrimSpace(raw.SteamID64)
	}
	if raw.Locale != "" {
		c.Locale = raw.Locale
	}
	if raw.MaxVisibleCount > 0 {
		c.MaxVisibleCount = raw.MaxVisibleCount
	}
	if raw.Notify != nil {
		c.Notify = *raw.Notify
	}
	if raw.WatchProfile != nil {
		c.WatchProfile = *raw.WatchProfile
	}
	if len(raw.RecentAppIDs) > 0 {
		c.RecentAppIDs = append([]int(nil), raw.RecentAppIDs...)
	}

	// Migración desde la versión monojuego.
	if raw.DefaultAppID > 0 && len(c.RecentAppIDs) == 0 {
		c.RecentAppIDs = []int{raw.DefaultAppID}
	}
	if raw.DefaultAppID != 0 {
		changed = true // reescribir sin la clave legada
	}

	return changed
}

// normalize corrige valores imposibles y devuelve true si hubo cambios.
func (c *Config) normalize() bool {
	changed := false

	if c.Locale == "" {
		c.Locale = DefaultLocale
		changed = true
	}
	if c.MaxVisibleCount <= 0 {
		c.MaxVisibleCount = DefaultMaxVisibleCount
		changed = true
	}
	if c.SteamID64 != "" && !ValidSteamID(c.SteamID64) {
		c.SteamID64 = ""
		changed = true
	}

	seen := make(map[int]bool, len(c.RecentAppIDs))
	out := make([]int, 0, MaxRecentAppIDs)
	for _, id := range c.RecentAppIDs {
		if id <= 0 || seen[id] {
			changed = true
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == MaxRecentAppIDs {
			break
		}
	}
	if len(out) != len(c.RecentAppIDs) {
		changed = true
	}
	c.RecentAppIDs = out

	return changed
}

// Save escribe la configuración de forma atómica con permisos 0600.
func (c Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsutil.WriteFileAtomic(path, data, 0o600)
}

// APIKey devuelve la clave efectiva: primero la variable de entorno y, si no,
// la del archivo de configuración. La variable nunca se escribe en disco.
func (c Config) APIKey() string {
	if v := strings.TrimSpace(os.Getenv(EnvAPIKey)); v != "" {
		return v
	}
	return strings.TrimSpace(c.SteamAPIKey)
}

// HasAPIKey indica si hay alguna clave configurada.
func (c Config) HasAPIKey() bool { return c.APIKey() != "" }

// AddRecentAppID coloca id al principio de la lista de recientes, sin duplicados
// y recortando a MaxRecentAppIDs. Devuelve true si la lista cambió.
func (c *Config) AddRecentAppID(id int) bool {
	if id <= 0 {
		return false
	}

	out := make([]int, 0, MaxRecentAppIDs)
	out = append(out, id)
	for _, existing := range c.RecentAppIDs {
		if existing != id {
			out = append(out, existing)
		}
	}
	if len(out) > MaxRecentAppIDs {
		out = out[:MaxRecentAppIDs]
	}

	changed := len(out) != len(c.RecentAppIDs)
	if !changed {
		for i := range out {
			if out[i] != c.RecentAppIDs[i] {
				changed = true
				break
			}
		}
	}

	c.RecentAppIDs = out
	return changed
}

// ValidSteamID comprueba que el identificador tenga el formato de un SteamID64.
func ValidSteamID(id string) bool {
	return steamIDPattern.MatchString(strings.TrimSpace(id))
}
