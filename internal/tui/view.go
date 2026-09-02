package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// Fixed chrome heights, in terminal rows.
const (
	headerHeight = 4
	promptHeight = 5
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
		return m.theme.muted.Render("iniciando o forge…")
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.renderTasks(), m.renderStream())
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), body, m.renderPrompt(), m.renderFooter())
}

func (m *model) bodyHeight() int {
	return max(m.height-headerHeight-promptHeight-footerHeight, 6)
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
		project = "nenhum projeto"
	}
	title := lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.theme.app.Render("forge"),
		m.theme.muted.Render("  "+shorten(project, max(m.width/2, 20))),
	)
	clock := m.theme.muted.Render(format(m.elapsed()))
	gap := max(m.width-4-lipgloss.Width(title)-lipgloss.Width(clock), 1)

	head := title + strings.Repeat(" ", gap) + clock
	return m.theme.panel.Width(m.width - 2).Render(head + "\n" + m.renderPipeline())
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
		glyph, style := "○", m.theme.muted
		switch {
		case index < currentIndex:
			glyph, style = "✓", m.theme.ok
		case index == currentIndex:
			glyph, style = "●", m.theme.accent
		}
		segments = append(segments, style.Render(glyph+" "+phaseLabels[phase]))
	}

	rendered := strings.Join(segments, m.theme.muted.Render(" → "))
	if fixing {
		rendered += m.theme.warn.Render("   ↺ fixing")
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
			m.theme.title.Render("PEDIDO"),
			lipgloss.NewStyle().Width(inner).Foreground(colorText).Render(m.requirement),
			"",
		)
	}

	sections = append(sections, m.theme.title.Render("TAREFAS"))
	if len(m.board.Tasks) == 0 {
		sections = append(sections, m.theme.muted.Render("aguardando o plano…"))
	}
	for _, item := range m.board.Tasks {
		glyph, style := m.theme.glyph(item.Status)
		marker := " "
		if item.ID == m.state.TaskID {
			marker = m.theme.accent.Render("▸")
		}
		label := shorten(item.ID+" "+item.Objective, inner-3)
		if item.ID == m.state.TaskID {
			label = m.theme.title.Render(label)
		}
		sections = append(sections, marker+style.Render(glyph)+" "+label)
	}

	sections = append(sections, "", m.theme.title.Render("EXECUÇÃO"))
	sections = append(sections,
		m.theme.muted.Render(fmt.Sprintf("passos     %d/%d", m.steps, m.maxSteps)),
		m.theme.muted.Render(fmt.Sprintf("correções  %d/%d", m.fixes, m.maxFixes)),
		m.theme.muted.Render("tarefa     "+orDash(m.state.TaskID)),
	)
	if dropped := m.session.droppedLines(); dropped > 0 {
		sections = append(sections, m.theme.warn.Render(fmt.Sprintf("%d linhas descartadas", dropped)))
	}

	return m.theme.panel.Width(width - 2).Height(m.bodyHeight() - 2).Render(strings.Join(sections, "\n"))
}

// renderStream is the right panel: live transcript, plan or review.
func (m *model) renderStream() string {
	tabs := make([]string, 0, len(paneNames))
	for _, candidate := range []pane{paneLive, panePlan, paneReview} {
		style := m.theme.tabOff
		if candidate == m.pane {
			style = m.theme.tabOn
		}
		tabs = append(tabs, style.Render(paneNames[candidate]))
	}

	head := strings.Join(tabs, m.theme.muted.Render(" · "))
	if m.pane == paneLive && !m.follow {
		head += m.theme.warn.Render("   (rolagem parada · G para seguir)")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, head, "", m.view.View())
	return m.theme.panel.Width(m.rightWidth() - 2).Height(m.bodyHeight() - 2).Render(content)
}

// paneContent renders whatever the selected pane shows.
func (m *model) paneContent() string {
	width, _ := m.viewportSize()

	if m.pane != paneLive {
		content, ok := m.files[m.pane]
		if !ok {
			return m.theme.muted.Render("lendo " + paneFiles[m.pane] + "…")
		}
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	if len(m.lines) == 0 {
		return m.theme.muted.Render("descreva um pedido para começar")
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
		return m.theme.role(entry.role).Render(entry.text)
	case lineInfo:
		return m.theme.accent.Render("→ ") + entry.text
	case lineWarn:
		return m.theme.warn.Render("! " + entry.text)
	case lineFail:
		return m.theme.fail.Render("✗ " + entry.text)
	case lineUser:
		return m.theme.user.Render("› " + entry.text)
	default:
		return m.theme.role(entry.role).Render("▏") + entry.text
	}
}

// renderPrompt is where the operator says what they want built. It stays on
// screen during a run, showing how to interrupt it.
func (m *model) renderPrompt() string {
	box := m.theme.panel.Width(m.width - 2).Height(promptHeight - 2)

	if m.mode == modeRunning {
		hint := m.theme.muted.Render("os agentes estão trabalhando · ") + m.theme.key.Render("esc") + m.theme.muted.Render(" interrompe")
		return box.BorderForeground(colorBorder).Render(hint)
	}

	m.prompt.SetWidth(max(m.width-6, 20))
	return box.BorderForeground(colorAccent).Render(m.prompt.View())
}

// renderFooter carries the status badge, the running agent and the keys., the running agent and the keys.
func (m *model) renderFooter() string {
	badge, detail := m.status()

	keys := []string{
		m.theme.key.Render("enter") + m.theme.muted.Render(" enviar"),
		m.theme.key.Render("↑") + m.theme.muted.Render(" histórico"),
		m.theme.key.Render("/continue") + m.theme.muted.Render(" retomar"),
		m.theme.key.Render("tab") + m.theme.muted.Render(" painéis"),
		m.theme.key.Render("ctrl+c") + m.theme.muted.Render(" sair"),
	}
	if m.mode == modeRunning {
		keys = []string{
			m.theme.key.Render("esc") + m.theme.muted.Render(" interromper"),
			m.theme.key.Render("p") + m.theme.muted.Render(" pausar"),
			m.theme.key.Render("f") + m.theme.muted.Render(" seguir"),
			m.theme.key.Render("tab") + m.theme.muted.Render(" painéis"),
			m.theme.key.Render("r") + m.theme.muted.Render(" recarregar"),
		}
	}
	help := strings.Join(keys, m.theme.muted.Render(" · "))
	first := badge + " " + detail
	return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Render(first) + "\n" +
		lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Render(help)
}

func (m *model) status() (string, string) {
	switch {
	case m.runErr != nil && m.mode == modeIdle:
		return m.theme.badgeFailed.Render("FALHOU"), m.theme.muted.Render(shorten(m.runErr.Error(), max(m.width-16, 20)))
	case m.mode == modeIdle && m.steps > 0:
		summary := fmt.Sprintf("%s · %d passos · %d correções · %s", m.state.Phase, m.steps, m.fixes, format(m.elapsed()))
		if m.logPath != "" {
			summary += " · log " + m.logPath
		}
		return m.theme.badgeDone.Render("PRONTO"), m.theme.muted.Render(shorten(summary, max(m.width-12, 20)))
	case m.mode == modeIdle:
		return m.theme.badgeIdle.Render("FORGE"), m.theme.muted.Render("descreva o que você quer construir neste projeto")
	case m.paused:
		return m.theme.badgePaused.Render("PAUSADO"), m.theme.muted.Render("o laço para antes do próximo despacho")
	case m.active:
		detail := m.theme.role(m.current.Role).Render(strings.ToUpper(string(m.current.Role))) +
			m.theme.muted.Render(" via "+string(m.current.Kind)+" · "+format(m.agentElapsed()))
		return m.theme.badgeRunning.Render("RODANDO"), m.spin.View() + " " + detail
	default:
		return m.theme.badgeRunning.Render("RODANDO"), m.theme.muted.Render("preparando o próximo despacho…")
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
