package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
)

// Palette. Adaptive colors keep the interface readable on light and dark
// terminals without asking the operator to configure anything. These are
// plain values: nothing here touches the terminal.
var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#5B3FD9", Dark: "#A98BFF"}
	colorText   = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6E6E6"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B8B8B"}
	colorBorder = lipgloss.AdaptiveColor{Light: "#D0D7DE", Dark: "#3A3A3A"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
	colorError  = lipgloss.AdaptiveColor{Light: "#B62324", Dark: "#F85149"}

	colorArchitect = lipgloss.AdaptiveColor{Light: "#5B3FD9", Dark: "#A98BFF"}
	colorBuilder   = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	colorReviewer  = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
)

// theme holds every style of the interface. It is built when the interface
// starts, never at package initialization: creating a lipgloss style makes it
// inspect the terminal, and commands without an interface must not do that.
type theme struct {
	app    lipgloss.Style
	title  lipgloss.Style
	muted  lipgloss.Style
	ok     lipgloss.Style
	warn   lipgloss.Style
	fail   lipgloss.Style
	accent lipgloss.Style
	user   lipgloss.Style
	key    lipgloss.Style

	panel  lipgloss.Style
	tabOn  lipgloss.Style
	tabOff lipgloss.Style

	badgeRunning lipgloss.Style
	badgeDone    lipgloss.Style
	badgeFailed  lipgloss.Style
	badgePaused  lipgloss.Style
	badgeIdle    lipgloss.Style

	architect lipgloss.Style
	builder   lipgloss.Style
	reviewer  lipgloss.Style
}

func newTheme() theme {
	badge := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	white := lipgloss.Color("#FFFFFF")

	return theme{
		app:    lipgloss.NewStyle().Bold(true).Foreground(colorAccent),
		title:  lipgloss.NewStyle().Bold(true).Foreground(colorText),
		muted:  lipgloss.NewStyle().Foreground(colorMuted),
		ok:     lipgloss.NewStyle().Foreground(colorOK),
		warn:   lipgloss.NewStyle().Foreground(colorWarn),
		fail:   lipgloss.NewStyle().Foreground(colorError),
		accent: lipgloss.NewStyle().Foreground(colorAccent),
		user:   lipgloss.NewStyle().Bold(true).Foreground(colorAccent),
		key:    lipgloss.NewStyle().Bold(true).Foreground(colorText),

		panel:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder).Padding(0, 1),
		tabOn:  lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Underline(true),
		tabOff: lipgloss.NewStyle().Foreground(colorMuted),

		badgeRunning: badge.Foreground(white).Background(colorAccent),
		badgeDone:    badge.Foreground(white).Background(colorOK),
		badgeFailed:  badge.Foreground(white).Background(colorError),
		badgePaused:  badge.Foreground(lipgloss.Color("#1F2328")).Background(colorWarn),
		badgeIdle:    badge.Foreground(colorText).Background(colorBorder),

		architect: lipgloss.NewStyle().Bold(true).Foreground(colorArchitect),
		builder:   lipgloss.NewStyle().Bold(true).Foreground(colorBuilder),
		reviewer:  lipgloss.NewStyle().Bold(true).Foreground(colorReviewer),
	}
}

// role colors an agent by the role it plays.
func (t theme) role(role agent.Role) lipgloss.Style {
	switch role {
	case agent.RoleArchitect:
		return t.architect
	case agent.RoleBuilder:
		return t.builder
	case agent.RoleReviewer:
		return t.reviewer
	default:
		return t.muted
	}
}

// diffLine colors one line of a unified diff.
func (t theme) diffLine(text string) string {
	switch {
	case strings.HasPrefix(text, "diff --git"), strings.HasPrefix(text, "index "):
		return t.title.Render(text)
	case strings.HasPrefix(text, "@@"):
		return t.accent.Render(text)
	case strings.HasPrefix(text, "+++"), strings.HasPrefix(text, "---"):
		return t.muted.Render(text)
	case strings.HasPrefix(text, "+"):
		return t.ok.Render(text)
	case strings.HasPrefix(text, "-"):
		return t.fail.Render(text)
	default:
		return text
	}
}

// glyph renders the status of a task as a single character.
func (t theme) glyph(status task.Status) (string, lipgloss.Style) {
	switch status {
	case task.StatusApproved:
		return "✓", t.ok
	case task.StatusImplementing, task.StatusReviewing:
		return "◐", t.accent
	case task.StatusBlocked:
		return "⊘", t.fail
	default:
		return "○", t.muted
	}
}
