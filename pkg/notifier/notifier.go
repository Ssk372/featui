// Package notifier envía notificaciones de escritorio con notify-send.
package notifier

import (
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxIconBytes = 2 << 20 // 2 MiB

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// Notifier envía notificaciones y cachea los iconos descargados.
type Notifier struct {
	AppName  string
	cacheDir string
	http     *http.Client
}

// New crea un notifier. Los iconos se guardan en XDG_CACHE_HOME/featui/icons.
func New(appName string) *Notifier {
	cacheDir := filepath.Join(os.TempDir(), "featui_icons")
	if base, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(base, "featui", "icons")
	}
	_ = os.MkdirAll(cacheDir, 0o755)

	return &Notifier{
		AppName:  appName,
		cacheDir: cacheDir,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// SendAchievementNotification descarga el icono si hace falta y lanza
// notify-send. cacheKey debe ser estable (el APIName del logro).
func (n *Notifier) SendAchievementNotification(gameTitle, achievementName, description, iconURL, cacheKey string) error {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return fmt.Errorf("notify-send no está disponible: %w", err)
	}

	summary := fmt.Sprintf("🏆 ¡Logro Desbloqueado! - %s", gameTitle)
	body := fmt.Sprintf("<b>%s</b>", achievementName)
	if description != "" {
		body += fmt.Sprintf("\n<i>%s</i>", description)
	}

	args := []string{"-a", n.AppName, "-u", "normal"}
	if iconPath := n.downloadIcon(iconURL, cacheKey); iconPath != "" {
		args = append(args, "-i", iconPath)
	}
	args = append(args, summary, body)

	return exec.Command("notify-send", args...).Run()
}

func (n *Notifier) downloadIcon(url, cacheKey string) string {
	if url == "" {
		return ""
	}

	name := sanitize(cacheKey)
	if name == "" {
		name = fmt.Sprintf("%x", sha1.Sum([]byte(url))) //nolint:gosec // solo para nombrar un archivo
	}
	path := filepath.Join(n.cacheDir, name+".png")

	if _, err := os.Stat(path); err == nil {
		return path
	}

	resp, err := n.http.Get(url)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	tmp, err := os.CreateTemp(n.cacheDir, "icon-*")
	if err != nil {
		return ""
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, maxIconBytes)); err != nil {
		_ = tmp.Close()
		return ""
	}
	if err := tmp.Close(); err != nil {
		return ""
	}
	if err := os.Rename(tmpName, path); err != nil {
		return ""
	}
	return path
}

func sanitize(value string) string {
	value = strings.TrimSpace(unsafeFilenameChars.ReplaceAllString(value, "_"))
	value = strings.Trim(value, "._-")
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}
