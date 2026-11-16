package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#7D56F4")).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("#04B575")).Bold(true)
	itemStyle     = lipgloss.NewStyle().PaddingLeft(2)
	checkedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))
	descStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Italic(true)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75"))
	steamStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#66C0F4"))
	barFullStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))
	barEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3C3C3C"))
	confirmStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#E06C75")).Padding(0, 1)
)

// View implementa tea.Model.
func (m Model) View() string {
	switch m.Screen {
	case ScreenLoading:
		return m.viewLoading()
	case ScreenError:
		return m.viewError()
	case ScreenReady:
		return m.viewAchievements()
	default:
		return m.viewAppID()
	}
}

func (m Model) viewLoading() string {
	label := "⏳ Cargando..."
	if m.AppID > 0 {
		label = fmt.Sprintf("⏳ Cargando AppID %d...", m.AppID)
	}

	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(titleStyle.Render(label))
	b.WriteString("\n\n  ")
	b.WriteString(m.StatusMsg)
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("  [esc] volver · [ctrl+c] salir"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewError() string {
	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(titleStyle.Render(fmt.Sprintf("❌ No se pudo abrir el AppID %d", m.AppID)))
	b.WriteString("\n\n  ")
	if m.Err != nil {
		b.WriteString(warnStyle.Render(m.Err.Error()))
	}
	if !m.Config.HasAPIKey() {
		b.WriteString("\n\n  ")
		b.WriteString(warnStyle.Render("Falta la API key: exporta FEATUI_STEAM_API_KEY o edita ~/.config/featui/config.json"))
	}
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("  [r] reintentar · [esc] volver · [q] salir"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewAppID() string {
	var b strings.Builder

	b.WriteString("\n  ")
	b.WriteString(titleStyle.Render("🏆 FeatUI — vuelve a cazar tus logros"))
	b.WriteString("\n\n  AppID o URL de Steam:\n  ")
	b.WriteString(m.Input.View())
	b.WriteString("\n\n")

	if len(m.Recents) > 0 {
		b.WriteString(helpStyle.Render("  Recientes") + "\n")
		for i, recent := range m.Recents {
			name := recent.Title
			if name == "" {
				name = fmt.Sprintf("AppID %d", recent.AppID)
			}
			line := fmt.Sprintf("%d. %s", i+1, name)
			if recent.Total > 0 {
				line += fmt.Sprintf("  (%d/%d)", recent.Completed, recent.Total)
			}

			if i == m.RecentCursor && m.Input.Value() == "" {
				b.WriteString(selectedStyle.Render("❯ "+line) + "\n")
			} else {
				b.WriteString(itemStyle.Render("  "+line) + "\n")
			}
		}
		b.WriteString("\n")
	}

	if !m.Config.HasAPIKey() {
		b.WriteString("  ")
		b.WriteString(warnStyle.Render("⚠ Falta la API key. Exporta FEATUI_STEAM_API_KEY o edita ~/.config/featui/config.json"))
		b.WriteString("\n\n")
	}

	if m.Warning != "" {
		b.WriteString("  ")
		b.WriteString(warnStyle.Render("⚠ " + m.Warning))
		b.WriteString("\n\n")
	}
	if m.StatusMsg != "" {
		b.WriteString("  ")
		b.WriteString(statusStyle.Render(m.StatusMsg))
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("  [enter] abrir · [↑/↓] recientes · [esc] salir"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewAchievements() string {
	indexes := m.visibleIndexes()
	total := m.Game.Total()
	done := m.Game.CompletedCount()

	var b strings.Builder

	b.WriteString("\n  ")
	b.WriteString(titleStyle.Render("🏆 " + displayTitle(m.Game.Title, m.AppID)))
	b.WriteString("  ")
	if total > 0 && done == total {
		b.WriteString(checkedStyle.Render(fmt.Sprintf("%d/%d (100%%) ★", done, total)))
	} else {
		b.WriteString(fmt.Sprintf("%d/%d (%d%%)", done, total, percent(done, total)))
	}
	b.WriteString("\n  ")
	b.WriteString(renderProgressBar(done, total, m.barWidth()))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render(fmt.Sprintf("  filtro: %s · %d visibles", m.Filter, len(indexes))))
	if m.Searching {
		b.WriteString("  " + m.Search.View())
	} else if query := strings.TrimSpace(m.Search.Value()); query != "" {
		b.WriteString(helpStyle.Render("  búsqueda: " + query))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  auto: " + m.autoState()))
	b.WriteString("\n\n")

	if len(indexes) == 0 {
		b.WriteString(itemStyle.Render("(sin logros para este filtro)"))
		b.WriteString("\n")
	} else {
		maxVisible := m.maxVisible()
		start := m.Cursor - maxVisible/2
		if start < 0 {
			start = 0
		}
		if start+maxVisible > len(indexes) {
			start = len(indexes) - maxVisible
			if start < 0 {
				start = 0
			}
		}
		end := start + maxVisible
		if end > len(indexes) {
			end = len(indexes)
		}

		for position := start; position < end; position++ {
			ach := m.Game.Achievements[indexes[position]]

			cursor := " "
			if position == m.Cursor {
				cursor = "❯"
			}

			check := "[ ]"
			if ach.Completed {
				check = checkedStyle.Render("[x]")
			}

			name := truncate(ach.DisplayName, m.nameWidth())
			line := fmt.Sprintf("%s %s %s", cursor, check, name)
			if m.Reference[ach.APIName] {
				line += steamStyle.Render(" · steam")
			}

			if position == m.Cursor {
				b.WriteString(selectedStyle.Render(line) + "\n")
				if ach.Description != "" {
					b.WriteString(itemStyle.Render(descStyle.Render("   └─ "+truncate(ach.Description, m.descWidth()))) + "\n")
				}
				if ach.UnlockedAt != nil {
					detail := fmt.Sprintf("   └─ %s", ach.UnlockedAt.Format("2006-01-02 15:04"))
					if ach.Source != "" {
						detail += " · " + string(ach.Source)
					}
					b.WriteString(itemStyle.Render(descStyle.Render(detail)) + "\n")
				}
			} else {
				b.WriteString(itemStyle.Render(line) + "\n")
			}
		}
	}

	if m.Reference != nil {
		b.WriteString("\n" + helpStyle.Render("  · steam = ya lo tienes desbloqueado en Steam (solo referencia)"))
	}

	b.WriteString("\n" + statusStyle.Render(" Status: "+m.StatusMsg))
	if m.Warning != "" {
		b.WriteString("\n" + warnStyle.Render(" ⚠ "+m.Warning))
	}
	if m.AutoWarning != "" {
		b.WriteString("\n" + warnStyle.Render(" ⚠ "+m.AutoWarning))
	}
	if m.SoundWarning != "" {
		b.WriteString("\n" + warnStyle.Render(" ⚠ sound: "+m.SoundWarning))
	}

	if m.ConfirmReset {
		b.WriteString("\n\n  " + confirmStyle.Render(fmt.Sprintf(
			"¿Reiniciar los %d logros de %s?\nSe guardará un backup del progreso actual. [y/N]",
			total, displayTitle(m.Game.Title, m.AppID),
		)))
	}

	if m.ConfirmSync {
		b.WriteString("\n\n  " + confirmStyle.Render(fmt.Sprintf(
			"¿Marcar en local todos los logros que ya tienes desbloqueados en Steam?\n%s · no modifica nada en Steam. [y/N]",
			displayTitle(m.Game.Title, m.AppID),
		)))
	}

	b.WriteString("\n\n" + helpStyle.Render(m.helpLine()))
	b.WriteString("\n")

	if m.ShowHelp {
		b.WriteString("\n" + m.renderHelp())
	}

	return b.String()
}

func (m Model) helpLine() string {
	if m.Searching {
		return "  [escribe] filtrar · [↑/↓] mover · [enter/esc] cerrar búsqueda"
	}
	if m.width > 0 && m.width < 108 {
		return "  [↑/↓] mover · [espacio] marcar · [/] buscar · [f] filtro · [S] sincronizar · [?] ayuda · [q] salir"
	}
	return "  [↑/↓] mover · [espacio] marcar · [/] buscar · [f] filtro · [S] sync Steam · [R] reiniciar · [r] recargar · [esc] volver · [?] ayuda · [q] salir"
}

func (m Model) renderHelp() string {
	lines := []string{
		helpStyle.Render("Atajos"),
		"  ↑/↓, k/j      mover el cursor",
		"  espacio       marcar / desmarcar",
		"  /             buscar por nombre o descripción",
		"  f             alternar filtro: todos / pendientes / completados",
		"  g / G         ir al principio / final",
		"  S             sincronizar con Steam (pide confirmación)",
		"  R             reiniciar la partida (pide confirmación)",
		"  r             recargar el esquema desde Steam",
		"  esc           volver a la pantalla de AppID",
		"  ?             ocultar esta ayuda",
		"",
		"  " + m.autoState(),
	}
	return strings.Join(lines, "\n")
}

// autoState resume el estado del sondeo para la ayuda y la barra de estado.
func (m Model) autoState() string {
	if !m.Config.AutoTrack {
		return "desactivado (auto_track = false)"
	}
	if m.Config.SteamID64 == "" {
		return "no disponible: falta steam_id_64"
	}

	interval := shortDuration(time.Duration(m.Config.PollInterval) * time.Second)
	last := "sin datos"
	if !m.LastPoll.IsZero() {
		last = m.LastPoll.Format("15:04")
	}

	suffix := fmt.Sprintf(" · %s · última sync %s", interval, last)
	switch {
	case !m.AutoActive:
		return "inactivo" + suffix
	case len(m.AutoWarning) > 0:
		return "con problemas" + suffix
	default:
		return "activo" + suffix
	}
}

func displayTitle(title string, appID int) string {
	if strings.TrimSpace(title) == "" {
		return fmt.Sprintf("AppID %d", appID)
	}
	return title
}

func percent(done, total int) int {
	if total <= 0 {
		return 0
	}
	return done * 100 / total
}

func (m Model) barWidth() int {
	width := m.width - 12
	if width < 10 {
		width = 10
	}
	if width > 40 {
		width = 40
	}
	return width
}

func renderProgressBar(done, total, width int) string {
	if total <= 0 {
		return ""
	}
	if width < 10 {
		width = 10
	}

	filled := (done*width + total/2) / total
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}

	return barFullStyle.Render(strings.Repeat("█", filled)) +
		barEmptyStyle.Render(strings.Repeat("░", width-filled))
}

func (m Model) maxVisible() int {
	limit := m.Config.MaxVisibleCount
	if limit <= 0 {
		limit = 15
	}
	if m.height > 0 {
		available := m.height - 12
		if available < 3 {
			available = 3
		}
		if available < limit {
			limit = available
		}
	}
	return limit
}

func (m Model) nameWidth() int {
	if m.width <= 24 {
		return 0
	}
	return m.width - 16
}

func (m Model) descWidth() int {
	if m.width <= 20 {
		return 0
	}
	return m.width - 12
}

// truncate recorta por runas para no partir caracteres multibyte.
func truncate(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
