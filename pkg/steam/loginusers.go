package steam

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	return detectSteamID(runtime.GOOS, home, os.Getenv)
}

// detectSteamID recorre las ubicaciones habituales de loginusers.vdf para el
// sistema indicado. Se separa de DetectLocalSteamID para poder probar las rutas
// de otros sistemas sin depender de la máquina que ejecuta los tests.
func detectSteamID(goos, home string, getenv func(string) string) (string, error) {
	for _, path := range steamConfigCandidates(goos, home, getenv) {
		id, err := parseLoginUsers(path)
		if err == nil && id != "" {
			return id, nil
		}
	}
	return "", errors.New("no se encontró una cuenta de Steam local")
}

// steamConfigCandidates devuelve las rutas donde Steam guarda loginusers.vdf,
// en orden de preferencia y sin duplicados. getenv permite inyectar el entorno
// en los tests.
func steamConfigCandidates(goos, home string, getenv func(string) string) []string {
	if home == "" && goos != "windows" {
		return nil
	}

	var candidates []string

	switch goos {
	case "windows":
		// En Windows de 64 bits Steam se instala por defecto en Program Files
		// (x86), incluso en equipos con Windows de 64 bits. Se usan las
		// variables de entorno y, solo si no existen, las rutas habituales.
		var bases []string
		for _, key := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if base := getenv(key); base != "" {
				bases = append(bases, base)
			}
		}
		if len(bases) == 0 {
			bases = []string{`C:\Program Files (x86)`, `C:\Program Files`}
		}
		for _, base := range bases {
			candidates = append(candidates, filepath.Join(base, "Steam", "config", "loginusers.vdf"))
		}
	case "darwin":
		candidates = append(candidates,
			filepath.Join(home, "Library", "Application Support", "Steam", "config", "loginusers.vdf"),
		)
	default:
		candidates = append(candidates,
			filepath.Join(home, ".local", "share", "Steam", "config", "loginusers.vdf"),
			filepath.Join(home, ".steam", "steam", "config", "loginusers.vdf"),
			filepath.Join(home, ".steam", "root", "config", "loginusers.vdf"),
			// Steam instalado como Flatpak (Steam Deck incluida).
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam", "config", "loginusers.vdf"),
		)
	}

	seen := make(map[string]bool, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, path := range candidates {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		unique = append(unique, path)
	}
	return unique
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
