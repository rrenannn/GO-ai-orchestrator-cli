package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// Fixed chrome heights, in terminal rows.
const (
	headerHeight = 4
	footerHeight = 2
)

// pipeline is the linear reading of the state machine. Fixing is a loop back
// into review, so it is drawn as a branch instead of a step of its own.
var pipeline = []workflow.Phase{
	workflow.PhasePlanning,
	workflow.PhaseImplementing,
	workflow.PhaseReviewing,
	workflow.PhaseApproved,
	workflow.PhaseCompleted,
}

var phaseLabels = map[workflow.Phase]string{
	workflow.PhasePlanning:     "plan",
	workflow.PhaseImplementing: "build",
	workflow.PhaseReviewing:    "review",
	workflow.PhaseApproved:     "approved",
	workflow.PhaseCompleted:    "done",
}

// View renders the whole interface.
func (m *model) View() string {
	if !m.ready {
		return styleMuted.Render("starting maestro…")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.renderTasks(), m.renderStream())
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), body, m.renderFooter())
}

func (m *model) bodyHeight() int {
	return max(m.height-headerHeight-footerHeight, 6)
}

func (m *model) leftWidth() int {
	return min(max(m.width*38/100, 26), 46)
}

func (m *model) rightWidth() int {
	return max(m.width-m.leftWidth(), 24)
}

func (m *model) viewportSize() (int, int) {
	return max(m.rightWidth()-4, 12), max(m.bodyHeight()-4, 3)
}

func (m *model) renderHeader() string {
	project := m.projectDir
	if project == "" {
		project = "no project"
	}
	title := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleApp.Render("maestro"),
		styleMuted.Render("  "+shorten(project, max(m.width/2, 20))),
	)
	clock := styleMuted.Render(format(m.elapsed()))
	gap := max(m.width-4-lipgloss.Width(title)-lipgloss.Width(clock), 1)

	head := title + strings.Repeat(" ", gap) + clock
	return stylePanel.Width(m.width - 2).Render(head + "\n" + m.renderPipeline())
}

// renderPipeline draws where the workflow is, at a glance.
func (m *model) renderPipeline() string {
	currentIndex := indexOf(pipeline, m.state.Phase)
	fixing := m.state.Phase == workflow.PhaseFixing
	if fixing {
		currentIndex = indexOf(pipeline, workflow.PhaseReviewing)
	}

	segments := make([]string, 0, len(pipeline))
	for index, phase := range pipeline {
		glyph, style := "○", styleMuted
		switch {
		case index < currentIndex:
			glyph, style = "✓", styleOK
		case index == currentIndex:
			glyph, style = "●", styleAccent
		}
		segments = append(segments, style.Render(glyph+" "+phaseLabels[phase]))
	}

	rendered := strings.Join(segments, styleMuted.Render(" → "))
	if fixing {
		rendered += styleWarn.Render("   ↺ fixing")
	}
	return rendered
}

// renderTasks is the left panel: what the request is and where each task is.
func (m *model) renderTasks() string {
	width := m.leftWidth()
	inner := max(width-4, 10)
	sections := []string{}

	if m.requirement != "" {
		sections = append(sections,
			styleTitle.Render("REQUEST"),
			lipgloss.NewStyle().Width(inner).Foreground(colorText).Render(m.requirement),
			"",
		)
	}

	sections = append(sections, styleTitle.Render("TASKS"))
	if len(m.board.Tasks) == 0 {
		sections = append(sections, styleMuted.Render("waiting for the plan…"))
	}
	for _, item := range m.board.Tasks {
		glyph, style := taskGlyph(item.Status)
		marker := " "
		if item.ID == m.state.TaskID {
			marker = styleAccent.Render("▸")
		}
		label := shorten(item.ID+" "+item.Objective, inner-3)
		if item.ID == m.state.TaskID {
			label = styleTitle.Render(label)
		}
		sections = append(sections, marker+style.Render(glyph)+" "+label)
	}

	sections = append(sections, "", styleTitle.Render("RUN"))
	sections = append(sections,
		styleMuted.Render(fmt.Sprintf("steps  %d/%d", m.steps, m.maxSteps)),
		styleMuted.Render(fmt.Sprintf("fixes  %d/%d", m.fixes, m.maxFixes)),
		styleMuted.Render("task   "+orDash(m.state.TaskID)),
	)
	if dropped := m.session.droppedLines(); dropped > 0 {
		sections = append(sections, styleWarn.Render(fmt.Sprintf("%d lines dropped", dropped)))
	}

	return stylePanel.Width(width - 2).Height(m.bodyHeight() - 2).Render(strings.Join(sections, "\n"))
}

// renderStream is the right panel: live transcript, plan or review.
func (m *model) renderStream() string {
	tabs := make([]string, 0, len(paneNames))
	for _, candidate := range []pane{paneLive, panePlan, paneReview} {
		style := styleTabOff
		if candidate == m.pane {
			style = styleTabOn
		}
		tabs = append(tabs, style.Render(paneNames[candidate]))
	}

	head := strings.Join(tabs, styleMuted.Render(" · "))
	if m.pane == paneLive && !m.follow {
		head += styleWarn.Render("   (paused scroll · G to follow)")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, head, "", m.view.View())
	return stylePanel.Width(m.rightWidth() - 2).Height(m.bodyHeight() - 2).Render(content)
}

// paneContent renders whatever the selected pane shows.
func (m *model) paneContent() string {
	width, _ := m.viewportSize()

	if m.pane != paneLive {
		content, ok := m.files[m.pane]
		if !ok {
			return styleMuted.Render("loading " + paneFiles[m.pane] + "…")
		}
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	if len(m.lines) == 0 {
		return styleMuted.Render("waiting for the first agent…")
	}

	wrap := lipgloss.NewStyle().Width(width)
	rendered := make([]string, 0, len(m.lines))
	for _, entry := range m.lines {
		rendered = append(rendered, wrap.Render(m.renderLine(entry)))
	}
	return strings.Join(rendered, "\n")
}

func (m *model) renderLine(entry line) string {
	switch entry.kind {
	case lineSystem:
		return roleStyle(entry.role).Render(entry.text)
	case lineInfo:
		return styleAccent.Render("→ ") + entry.text
	case lineWarn:
		return styleWarn.Render("! " + entry.text)
	case lineFail:
		return styleError.Render("✗ " + entry.text)
	default:
		return roleStyle(entry.role).Render("▏") + entry.text
	}
}

// renderFooter carries the status badge, the running agent and the keys.
func (m *model) renderFooter() string {
	badge, detail := m.status()

	keys := []string{
		styleKey.Render("tab") + styleMuted.Render(" panes"),
		styleKey.Render("p") + styleMuted.Render(" pause"),
		styleKey.Render("f") + styleMuted.Render(" follow"),
		styleKey.Render("r") + styleMuted.Render(" reload"),
		styleKey.Render("q") + styleMuted.Render(" quit"),
	}
	help := strings.Join(keys, styleMuted.Render(" · "))
	if m.confirmQuit {
		help = styleWarn.Render("stop the run and quit? q confirms · any other key cancels")
	}

	first := badge + " " + detail
	return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Render(first) + "\n" +
		lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Render(help)
}

func (m *model) status() (string, string) {
	switch {
	case m.runErr != nil:
		return styleBadgeErr.Render("FAILED"), styleMuted.Render(shorten(m.runErr.Error(), max(m.width-16, 20)))
	case m.finished:
		summary := fmt.Sprintf("%s · %d steps · %d fixes · %s", m.state.Phase, m.steps, m.fixes, format(m.elapsed()))
		if m.logPath != "" {
			summary += " · log " + m.logPath
		}
		return styleBadgeOK.Render("DONE"), styleMuted.Render(shorten(summary, max(m.width-10, 20)))
	case m.paused:
		return styleBadgePau.Render("PAUSED"), styleMuted.Render("the loop stops before the next dispatch")
	case m.active:
		detail := roleStyle(m.current.Role).Render(strings.ToUpper(string(m.current.Role))) +
			styleMuted.Render(" via "+string(m.current.Kind)+" · "+format(m.agentElapsed()))
		return styleBadgeRun.Render("RUNNING"), m.spin.View() + " " + detail
	default:
		return styleBadgeRun.Render("RUNNING"), styleMuted.Render("preparing the next dispatch…")
	}
}

func indexOf(phases []workflow.Phase, wanted workflow.Phase) int {
	for index, phase := range phases {
		if phase == wanted {
			return index
		}
	}
	return 0
}

func shorten(text string, width int) string {
	if width <= 1 || lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	return string(runes[:max(width-1, 1)]) + "…"
}

func format(duration time.Duration) string {
	return duration.String()
}

func orDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}
