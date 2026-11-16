package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var (
	loginUserBlock = regexp.MustCompile(`(?ms)"(\d{17})"\s*\{([^}]*)\}`)
	mostRecentFlag = regexp.MustCompile(`"MostRecent"\s*"1"`)
)

// DetectLocalSteamID intenta leer el SteamID64 de la cuenta usada más
// recientemente, sin pedirle nada al usuario.
func DetectLocalSteamID() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(home, ".local", "share", "Steam", "config", "loginusers.vdf"),
		filepath.Join(home, ".steam", "steam", "config", "loginusers.vdf"),
		filepath.Join(home, ".steam", "root", "config", "loginusers.vdf"),
	}

	for _, path := range candidates {
		id, err := parseLoginUsers(path)
		if err == nil && id != "" {
			return id, nil
		}
	}
	return "", errors.New("no se encontró una cuenta de Steam local")
}

func parseLoginUsers(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	matches := loginUserBlock.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("loginusers.vdf sin cuentas (%s)", path)
	}

	for _, match := range matches {
		if mostRecentFlag.MatchString(match[2]) {
			return match[1], nil
		}
	}
	return matches[0][1], nil
}
