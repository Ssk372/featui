// Package steam encapsula las llamadas de solo lectura a la API de Steam.
package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ssk372/featui/pkg/model"
)

var (
	// ErrInvalidAPIKey indica que Steam rechazó la clave (o falta).
	ErrInvalidAPIKey = errors.New("Steam rechazó la API key: revisa steam_api_key en config.json o la variable FEATUI_STEAM_API_KEY")
	// ErrNoStats indica que el juego no expone logros por API.
	ErrNoStats = errors.New("este juego no expone logros a través de la API de Steam")
	// ErrPrivateProfile indica que el perfil impide leer datos del jugador.
	ErrPrivateProfile = errors.New("el perfil de Steam no es público")
)

// Client es un cliente HTTP mínimo para la API pública de Steam.
type Client struct {
	apiKey    string
	baseURL   string
	http      *http.Client
	retryWait time.Duration
}

// NewClient crea un cliente. La clave vacía provoca ErrInvalidAPIKey al usarlo.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:    strings.TrimSpace(apiKey),
		baseURL:   "https://api.steampowered.com",
		http:      &http.Client{Timeout: 12 * time.Second},
		retryWait: 400 * time.Millisecond,
	}
}

// OwnedGame es un juego propiedad de la cuenta.
type OwnedGame struct {
	AppID           int
	Name            string
	PlaytimeForever int
	IconURL         string
}

// FetchGameSchema devuelve los logros de un juego (sin estado) y su nombre.
func (c *Client) FetchGameSchema(ctx context.Context, appID int, locale string) ([]model.Achievement, string, error) {
	if appID <= 0 {
		return nil, "", fmt.Errorf("AppID inválido: %d", appID)
	}

	params := url.Values{}
	params.Set("appid", strconv.Itoa(appID))
	if locale != "" {
		params.Set("l", locale)
	}

	var resp struct {
		Game struct {
			GameName           string `json:"gameName"`
			AvailableGameStats struct {
				Achievements []struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
					Description string `json:"description"`
					Icon        string `json:"icon"`
					IconGray    string `json:"icongray"`
					Hidden      int    `json:"hidden"`
				} `json:"achievements"`
			} `json:"availableGameStats"`
		} `json:"game"`
	}

	if err := c.get(ctx, "/ISteamUserStats/GetSchemaForGame/v2/", params, &resp); err != nil {
		return nil, "", err
	}

	raw := resp.Game.AvailableGameStats.Achievements
	if len(raw) == 0 {
		return nil, resp.Game.GameName, ErrNoStats
	}

	achievements := make([]model.Achievement, 0, len(raw))
	for _, item := range raw {
		achievements = append(achievements, model.Achievement{
			APIName:     item.Name,
			DisplayName: item.DisplayName,
			Description: item.Description,
			IconURL:     item.Icon,
			IconGrayURL: item.IconGray,
			Hidden:      item.Hidden != 0,
		})
	}

	return achievements, resp.Game.GameName, nil
}

// PlayerAchievement es el estado real de un logro en Steam.
type PlayerAchievement struct {
	Unlocked   bool
	UnlockTime time.Time
}

// FetchPlayerAchievements devuelve el estado real del jugador. FeatUI lo usa
// para el badge informativo y para detectar desbloqueos nuevos mientras la app
// está abierta. Nunca escribe nada en Steam.
func (c *Client) FetchPlayerAchievements(ctx context.Context, steamID string, appID int) (map[string]PlayerAchievement, error) {
	if !ValidSteamID(steamID) {
		return nil, fmt.Errorf("SteamID64 inválido: %q", steamID)
	}
	if appID <= 0 {
		return nil, fmt.Errorf("AppID inválido: %d", appID)
	}

	params := url.Values{}
	params.Set("steamid", steamID)
	params.Set("appid", strconv.Itoa(appID))

	var resp struct {
		PlayerStats struct {
			Success      bool   `json:"success"`
			Error        string `json:"error"`
			Achievements []struct {
				APIName    string `json:"apiname"`
				Achieved   int    `json:"achieved"`
				UnlockTime int64  `json:"unlocktime"`
			} `json:"achievements"`
		} `json:"playerstats"`
	}

	if err := c.get(ctx, "/ISteamUserStats/GetPlayerAchievements/v1/", params, &resp); err != nil {
		return nil, err
	}

	stats := resp.PlayerStats
	if !stats.Success {
		return nil, classifyPlayerStatsError(stats.Error)
	}

	completed := make(map[string]PlayerAchievement, len(stats.Achievements))
	for _, item := range stats.Achievements {
		state := PlayerAchievement{Unlocked: item.Achieved == 1}
		if state.Unlocked && item.UnlockTime > 0 {
			state.UnlockTime = time.Unix(item.UnlockTime, 0).UTC()
		}
		completed[item.APIName] = state
	}
	return completed, nil
}

// FetchOwnedGames devuelve la biblioteca del jugador. Requiere perfil público.
// Todavía no se usa en la TUI; queda listo para el selector de biblioteca.
func (c *Client) FetchOwnedGames(ctx context.Context, steamID string) ([]OwnedGame, error) {
	if !ValidSteamID(steamID) {
		return nil, fmt.Errorf("SteamID64 inválido: %q", steamID)
	}

	params := url.Values{}
	params.Set("steamid", steamID)
	params.Set("include_appinfo", "true")
	params.Set("include_played_free_games", "true")

	var resp struct {
		Response struct {
			GameCount int `json:"game_count"`
			Games     []struct {
				AppID           int    `json:"appid"`
				Name            string `json:"name"`
				PlaytimeForever int    `json:"playtime_forever"`
				IconHash        string `json:"img_icon_url"`
			} `json:"games"`
		} `json:"response"`
	}

	if err := c.get(ctx, "/IPlayerService/GetOwnedGames/v1/", params, &resp); err != nil {
		return nil, err
	}

	if resp.Response.GameCount == 0 && len(resp.Response.Games) == 0 {
		return nil, ErrPrivateProfile
	}

	games := make([]OwnedGame, 0, len(resp.Response.Games))
	for _, item := range resp.Response.Games {
		game := OwnedGame{
			AppID:           item.AppID,
			Name:            item.Name,
			PlaytimeForever: item.PlaytimeForever,
		}
		if item.IconHash != "" {
			game.IconURL = fmt.Sprintf("https://media.steampowered.com/steamcommunity/public/images/apps/%d/%s.jpg", item.AppID, item.IconHash)
		}
		games = append(games, game)
	}
	return games, nil
}

// ValidSteamID comprueba el formato de un SteamID64 (17 dígitos).
func ValidSteamID(id string) bool {
	id = strings.TrimSpace(id)
	if len(id) != 17 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func classifyPlayerStatsError(message string) error {
	msg := strings.ToLower(strings.TrimSpace(message))
	switch {
	case msg == "":
		return ErrNoStats
	case strings.Contains(msg, "not public"), strings.Contains(msg, "private"):
		return ErrPrivateProfile
	case strings.Contains(msg, "no stats"), strings.Contains(msg, "no achievements"):
		return ErrNoStats
	default:
		return fmt.Errorf("Steam: %s", strings.TrimSpace(message))
	}
}

// get ejecuta una petición GET con reintentos para 429 y 5xx.
func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	if c.apiKey == "" {
		return ErrInvalidAPIKey
	}

	query := url.Values{}
	for key, values := range params {
		query[key] = append([]string(nil), values...)
	}
	query.Set("key", c.apiKey)
	endpoint := c.baseURL + path + "?" + query.Encode()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := c.retryWait * time.Duration(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "featui (github.com/Ssk372/featui)")

		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = fmt.Errorf("no se pudo contactar con Steam: %w", err)
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("no se pudo leer la respuesta de Steam: %w", readErr)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
			return ErrInvalidAPIKey
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
			lastErr = fmt.Errorf("Steam respondió HTTP %d", resp.StatusCode)
			continue
		case resp.StatusCode != http.StatusOK:
			return fmt.Errorf("Steam respondió HTTP %d", resp.StatusCode)
		}

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("respuesta de Steam ilegible: %w", err)
		}
		return nil
	}

	return lastErr
}
