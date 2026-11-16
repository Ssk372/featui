package steam

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := NewClient("test-key")
	client.baseURL = server.URL
	client.retryWait = time.Millisecond
	return client
}

func TestFetchGameSchema(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISteamUserStats/GetSchemaForGame/v2/" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("appid"); got != "49520" {
			t.Errorf("appid = %q", got)
		}
		if got := r.URL.Query().Get("l"); got != "spanish" {
			t.Errorf("l = %q", got)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("key = %q", got)
		}
		_, _ = w.Write([]byte(`{
			"game": {
				"gameName": "Borderlands",
				"availableGameStats": {
					"achievements": [
						{"name": "Achievement_1", "displayName": "La primera es gratis", "description": "Haz algo", "icon": "http://icon", "icongray": "http://gray", "hidden": 1}
					]
				}
			}
		}`))
	})

	achievements, title, err := client.FetchGameSchema(context.Background(), 49520, "spanish")
	if err != nil {
		t.Fatalf("FetchGameSchema: %v", err)
	}
	if title != "Borderlands" {
		t.Fatalf("title = %q", title)
	}
	if len(achievements) != 1 {
		t.Fatalf("logros = %d", len(achievements))
	}
	ach := achievements[0]
	if ach.APIName != "Achievement_1" || !ach.Hidden || ach.IconURL != "http://icon" || ach.Completed {
		t.Fatalf("logro mal parseado: %+v", ach)
	}
}

func TestFetchGameSchemaWithoutAchievements(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"game":{"gameName":"Sin logros","availableGameStats":{"achievements":[]}}}`))
	})

	if _, _, err := client.FetchGameSchema(context.Background(), 1, "spanish"); !errors.Is(err, ErrNoStats) {
		t.Fatalf("err = %v, quiero ErrNoStats", err)
	}
}

func TestFetchGameSchemaInvalidKey(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if _, _, err := client.FetchGameSchema(context.Background(), 49520, "spanish"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("err = %v, quiero ErrInvalidAPIKey", err)
	}
}

func TestClientWithoutAPIKeyFailsFast(t *testing.T) {
	client := NewClient("")
	if _, _, err := client.FetchGameSchema(context.Background(), 1, "spanish"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("err = %v, quiero ErrInvalidAPIKey", err)
	}
}

func TestRetriesServerErrors(t *testing.T) {
	var calls int32
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"game":{"gameName":"X","availableGameStats":{"achievements":[{"name":"A","displayName":"A"}]}}}`))
	})

	if _, _, err := client.FetchGameSchema(context.Background(), 1, "spanish"); err != nil {
		t.Fatalf("debería recuperarse tras un 500: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("llamadas = %d, quiero 2", got)
	}
}

func TestFetchPlayerAchievements(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISteamUserStats/GetPlayerAchievements/v1/" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("steamid"); got != "76561198000000000" {
			t.Errorf("steamid = %q", got)
		}
		_, _ = w.Write([]byte(`{"playerstats":{"success":true,"achievements":[
			{"apiname":"A","achieved":1},
			{"apiname":"B","achieved":0}
		]}}`))
	})

	completed, err := client.FetchPlayerAchievements(context.Background(), "76561198000000000", 49520)
	if err != nil {
		t.Fatalf("FetchPlayerAchievements: %v", err)
	}
	if !completed["A"] || completed["B"] {
		t.Fatalf("mapa incorrecto: %+v", completed)
	}
}

func TestFetchPlayerAchievementsPrivateProfile(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"playerstats":{"success":false,"error":"Profile is not public"}}`))
	})

	_, err := client.FetchPlayerAchievements(context.Background(), "76561198000000000", 49520)
	if !errors.Is(err, ErrPrivateProfile) {
		t.Fatalf("err = %v, quiero ErrPrivateProfile", err)
	}
}

func TestFetchOwnedGames(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/IPlayerService/GetOwnedGames/v1/" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("include_appinfo"); got != "true" {
			t.Errorf("include_appinfo = %q", got)
		}
		_, _ = w.Write([]byte(`{"response":{"game_count":1,"games":[
			{"appid":49520,"name":"Borderlands 2","playtime_forever":120,"img_icon_url":"abc123"}
		]}}`))
	})

	games, err := client.FetchOwnedGames(context.Background(), "76561198000000000")
	if err != nil {
		t.Fatalf("FetchOwnedGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("juegos = %d", len(games))
	}
	if games[0].AppID != 49520 || games[0].Name != "Borderlands 2" {
		t.Fatalf("juego mal parseado: %+v", games[0])
	}
	if !strings.Contains(games[0].IconURL, "/49520/abc123.jpg") {
		t.Fatalf("icono mal construido: %q", games[0].IconURL)
	}
}

func TestFetchOwnedGamesPrivateProfile(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"response":{}}`))
	})

	_, err := client.FetchOwnedGames(context.Background(), "76561198000000000")
	if !errors.Is(err, ErrPrivateProfile) {
		t.Fatalf("err = %v, quiero ErrPrivateProfile", err)
	}
}

func TestDetectLocalSteamIDParsesMostRecent(t *testing.T) {
	vdf := `"users"
{
	"76561198000000001"
	{
		"AccountName" "uno"
		"MostRecent" "0"
	}
	"76561198000000002"
	{
		"AccountName" "dos"
		"MostRecent" "1"
	}
}`
	path := t.TempDir() + "/loginusers.vdf"
	if err := os.WriteFile(path, []byte(vdf), 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := parseLoginUsers(path)
	if err != nil {
		t.Fatalf("parseLoginUsers: %v", err)
	}
	if id != "76561198000000002" {
		t.Fatalf("id = %q, quiero la cuenta MostRecent", id)
	}
}
