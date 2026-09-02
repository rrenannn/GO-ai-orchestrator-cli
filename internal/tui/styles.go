package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/GO-ai-orchestrator-cli/internal/domain/task"
)

// Palette. Adaptive colors keep the interface readable on light and dark
// terminals without asking the operator to configure anything.
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

var (
	styleApp      = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	styleMuted    = lipgloss.NewStyle().Foreground(colorMuted)
	styleOK       = lipgloss.NewStyle().Foreground(colorOK)
	styleWarn     = lipgloss.NewStyle().Foreground(colorWarn)
	styleError    = lipgloss.NewStyle().Foreground(colorError)
	styleAccent   = lipgloss.NewStyle().Foreground(colorAccent)
	stylePanel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder).Padding(0, 1)
	styleTabOn    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Underline(true)
	styleTabOff   = lipgloss.NewStyle().Foreground(colorMuted)
	styleKey      = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	styleBadgeRun = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorAccent).Padding(0, 1)
	styleBadgeOK  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorOK).Padding(0, 1)
	styleBadgeErr = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorError).Padding(0, 1)
	styleBadgePau = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1F2328")).Background(colorWarn).Padding(0, 1)
)

// roleStyle colors an agent by the role it plays.
func roleStyle(role agent.Role) lipgloss.Style {
	switch role {
	case agent.RoleArchitect:
		return lipgloss.NewStyle().Bold(true).Foreground(colorArchitect)
	case agent.RoleBuilder:
		return lipgloss.NewStyle().Bold(true).Foreground(colorBuilder)
	case agent.RoleReviewer:
		return lipgloss.NewStyle().Bold(true).Foreground(colorReviewer)
	default:
		return styleMuted
	}
}

// taskGlyph renders the status of a task as a single character.
func taskGlyph(status task.Status) (string, lipgloss.Style) {
	switch status {
	case task.StatusApproved:
		return "✓", styleOK
	case task.StatusImplementing, task.StatusReviewing:
		return "◐", styleAccent
	case task.StatusBlocked:
		return "⊘", styleError
	default:
		return "○", styleMuted
	}
}
