package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// maxLines caps the in-memory transcript. The full one is on disk anyway.
const maxLines = 5000

// Messages exchanged inside the interface.
type (
	eventMsg   struct{ published event.Event }
	runDoneMsg struct{ err error }
	tickMsg    time.Time
	fileMsg    struct {
		pane    pane
		content string
	}
)

// mode says whether the operator is typing the next request or watching the
// agents work on the current one.
type mode int

const (
	modeIdle mode = iota
	modeRunning
)

// pane is one of the views of the right side.
type pane int

const (
	paneLive pane = iota
	panePlan
	paneReview
	paneDiff
)

// panes is the tab order of the right side.
var panes = []pane{paneLive, panePlan, paneReview, paneDiff}

var paneNames = map[pane]string{
	paneLive:   "Live",
	panePlan:   "Plan",
	paneReview: "Review",
	paneDiff:   "Diff",
}

var paneFiles = map[pane]string{
	panePlan:   filepath.Join(".agent", "PLAN.md"),
	paneReview: filepath.Join(".agent", "REVIEW.md"),
}

// lineKind decides how a transcript line is colored.
type lineKind int

const (
	lineAgent lineKind = iota
	lineSystem
	lineInfo
	lineWarn
	lineFail
	lineUser
	lineOK
)

type line struct {
	kind lineKind
	role agent.Role
	text string
}

// model is the whole state of the interface.
type model struct {
	theme     theme
	session   *Session
	actions   Actions
	autostart func(context.Context) error

	spin   spinner.Model
	view   viewport.Model
	prompt textarea.Model
	ready  bool
	width  int
	height int

	mode    mode
	history []string
	recall  int

	projectDir  string
	requirement string
	logPath     string

	state   workflow.State
	board   task.Board
	current agent.Assignment
	active  bool

	startedAt      time.Time
	agentStartedAt time.Time
	steps          int
	maxSteps       int
	fixes          int
	maxFixes       int

	lines      []line
	files      map[pane]string
	pane       pane
	follow     bool
	paused     bool
	validating bool
	runErr     error
}

func newModel(session *Session, actions Actions) *model {
	palette := newTheme()

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = palette.accent

	prompt := textarea.New()
	prompt.Placeholder = "o que você quer construir?  (enter envia · /help)"
	prompt.ShowLineNumbers = false
	prompt.CharLimit = 4000
	prompt.SetHeight(3)
	prompt.Prompt = "  "
	prompt.Focus()

	return &model{
		theme:      palette,
		session:    session,
		actions:    actions,
		projectDir: session.project,
		spin:       spin,
		prompt:     prompt,
		files:      map[pane]string{},
		follow:     true,
		recall:     -1,
		startedAt:  time.Now(),
	}
}

// Init starts the spinner, the one-second clock, and whatever the command
// line asked to run right away.
func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick, everySecond(), textarea.Blink}
	if m.autostart != nil {
		cmds = append(cmds, m.begin(m.autostart))
	}
	return tea.Batch(cmds...)
}

// begin puts the interface in running mode and dispatches one action.
func (m *model) begin(action func(context.Context) error) tea.Cmd {
	m.mode = modeRunning
	m.runErr = nil
	m.prompt.Blur()
	m.follow = true
	return m.session.dispatch(action)
}

func everySecond() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return tickMsg(now) })
}

// Update handles one message. It is the only place the interface changes.
func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(typed.Width, typed.Height)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(typed)
		return m, cmd

	case tickMsg:
		return m, everySecond()

	case eventMsg:
		m.apply(typed.published)
		m.refresh()
		return m, nil

	case runDoneMsg:
		m.mode = modeIdle
		m.active = false
		m.paused = false
		if typed.err != nil && m.runErr == nil {
			m.runErr = typed.err
			m.append(line{kind: lineFail, text: typed.err.Error()})
		}
		m.refresh()
		return m, m.prompt.Focus()

	case fileMsg:
		m.files[typed.pane] = typed.content
		m.refresh()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(typed)
	}

	return m, nil
}

func (m *model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		// While the agents work, the first interrupt stops the run, not the
		// session: the next request can be typed right away.
		if m.mode == modeRunning {
			m.interrupt()
			return m, nil
		}
		return m, tea.Quit

	case "ctrl+d":
		return m, tea.Quit

	case "esc":
		if m.mode == modeRunning {
			m.interrupt()
			return m, nil
		}
		m.prompt.Reset()
		m.recall = -1
		return m, nil

	case "tab":
		return m, m.selectPane((m.pane + 1) % pane(len(panes)))

	case "shift+tab":
		return m, m.selectPane((m.pane + pane(len(panes)) - 1) % pane(len(panes)))

	case "pgup", "pgdown":
		return m, m.scroll(key)
	}

	if m.mode == modeRunning {
		return m.handleRunningKey(key)
	}
	return m.handleIdleKey(key)
}

// handleRunningKey drives the panels while the agents work. The prompt is
// blurred, so single letters are controls instead of text.
func (m *model) handleRunningKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "p":
		m.paused = m.session.togglePause()
		if m.paused {
			m.append(line{kind: lineInfo, text: "pausado: o laço para antes do próximo despacho"})
		} else {
			m.append(line{kind: lineInfo, text: "retomado"})
		}
		m.refresh()
		return m, nil

	case "f":
		m.follow = !m.follow
		if m.follow {
			m.view.GotoBottom()
		}
		return m, nil

	case "r":
		return m, m.loadPane(m.pane)

	case "g":
		m.follow = false
		m.view.GotoTop()
		return m, nil

	case "G":
		m.view.GotoBottom()
		m.follow = true
		return m, nil

	case "1":
		return m, m.selectPane(paneLive)
	case "2":
		return m, m.selectPane(panePlan)
	case "3":
		return m, m.selectPane(paneReview)
	case "4":
		return m, m.selectPane(paneDiff)
	}

	return m, m.scroll(key)
}

// handleIdleKey feeds the prompt, with history under the arrow keys.
func (m *model) handleIdleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "enter":
		return m.submit()

	case "up":
		if m.canRecall() {
			m.recallHistory(1)
			return m, nil
		}

	case "down":
		if m.recall >= 0 {
			m.recallHistory(-1)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(key)
	return m, cmd
}

// submit turns whatever was typed into a run or a command.
func (m *model) submit() (tea.Model, tea.Cmd) {
	typed := strings.TrimSpace(m.prompt.Value())
	if typed == "" {
		return m, nil
	}
	m.prompt.Reset()
	m.recall = -1

	if strings.HasPrefix(typed, "/") {
		return m.runCommand(typed)
	}

	m.history = append(m.history, typed)
	m.append(line{kind: lineUser, text: typed})
	m.requirement = typed
	m.resetRun()
	m.refresh()

	requirement := typed
	return m, m.begin(func(ctx context.Context) error {
		return m.actions.Start(ctx, requirement)
	})
}

// runCommand handles the slash commands of the prompt.
func (m *model) runCommand(typed string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(strings.Fields(typed)[0]) {
	case "/continue", "/c":
		if m.actions.Continue == nil {
			m.append(line{kind: lineWarn, text: "nada para retomar"})
			m.refresh()
			return m, nil
		}
		m.append(line{kind: lineUser, text: "/continue"})
		m.resetRun()
		m.refresh()
		return m, m.begin(m.actions.Continue)

	case "/help", "/?":
		for _, help := range []string{
			"digite o que você quer construir e envie com enter",
			"/continue  retoma o ciclo já registrado no projeto",
			"/quit      sai do forge",
			"durante a execução: esc interrompe · p pausa · tab troca painel",
		} {
			m.append(line{kind: lineInfo, text: help})
		}
		m.refresh()
		return m, nil

	case "/quit", "/exit", "/q":
		return m, tea.Quit

	default:
		m.append(line{kind: lineWarn, text: "comando desconhecido: " + typed})
		m.refresh()
		return m, nil
	}
}

func (m *model) interrupt() {
	m.session.interrupt()
	m.append(line{kind: lineWarn, text: "interrompendo o agente…"})
	m.refresh()
}

func (m *model) scroll(key tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(key)
	m.follow = m.view.AtBottom()
	return cmd
}

func (m *model) canRecall() bool {
	return len(m.history) > 0 && (m.recall >= 0 || strings.TrimSpace(m.prompt.Value()) == "")
}

// recallHistory walks the submitted requests, newest first.
func (m *model) recallHistory(direction int) {
	m.recall = min(max(m.recall+direction, -1), len(m.history)-1)
	if m.recall < 0 {
		m.prompt.Reset()
		return
	}
	m.prompt.SetValue(m.history[len(m.history)-1-m.recall])
	m.prompt.CursorEnd()
}

// resetRun clears what belongs to the previous request.
func (m *model) resetRun() {
	m.steps, m.fixes = 0, 0
	m.runErr = nil
	m.startedAt = time.Now()
	m.lines = append(m.lines, line{kind: lineSystem, text: strings.Repeat("─", 24)})
}

func (m *model) selectPane(target pane) tea.Cmd {
	m.pane = target
	m.refresh()
	if target == paneLive {
		return nil
	}
	return m.loadPane(target)
}

// loadDiff asks the application for the uncommitted work. The interface never
// runs git itself.
func (m *model) loadDiff() tea.Cmd {
	if m.actions.Diff == nil {
		return nil
	}
	return func() tea.Msg {
		diff, err := m.actions.Diff(context.Background())
		switch {
		case err != nil:
			return fileMsg{pane: paneDiff, content: "não foi possível ler o diff: " + err.Error()}
		case strings.TrimSpace(diff) == "":
			return fileMsg{pane: paneDiff, content: "nenhuma mudança na árvore de trabalho"}
		default:
			return fileMsg{pane: paneDiff, content: diff}
		}
	}
}

// loadPane reads an artifact from the project. Reading it fresh on demand
// keeps the interface honest: it shows what the agents actually wrote.
func (m *model) loadPane(target pane) tea.Cmd {
	if target == paneDiff {
		return m.loadDiff()
	}
	name, ok := paneFiles[target]
	if !ok || m.projectDir == "" {
		return nil
	}
	path := filepath.Join(m.projectDir, name)
	return func() tea.Msg {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fileMsg{pane: target, content: "could not read " + name + ": " + err.Error()}
		}
		return fileMsg{pane: target, content: string(raw)}
	}
}

// apply folds one orchestration event into the interface state.
func (m *model) apply(published event.Event) {
	switch typed := published.(type) {
	case event.RunStarted:
		// Events alone are enough to know a run is under way, even when the
		// interface did not dispatch it itself.
		m.mode = modeRunning
		m.projectDir = typed.ProjectDir
		m.logPath = typed.LogPath
		m.maxSteps = typed.MaxSteps
		m.maxFixes = typed.MaxFixes
		if typed.Requirement != "" {
			m.requirement = typed.Requirement
		}
		if !typed.StartedAt.IsZero() {
			m.startedAt = typed.StartedAt
		}
		m.append(line{kind: lineSystem, text: "execução iniciada em " + typed.ProjectDir})

	case event.BoardUpdated:
		m.state = typed.State
		m.board = typed.Board

	case event.AgentStarted:
		m.mode = modeRunning
		m.current = typed.Assignment
		m.active = true
		m.agentStartedAt = time.Now()
		m.steps = typed.Step
		if typed.MaxSteps > 0 {
			m.maxSteps = typed.MaxSteps
		}
		m.state = typed.State
		m.append(line{
			kind: lineSystem,
			role: typed.Assignment.Role,
			text: "── " + typed.Assignment.String() + " · fase " + typed.State.Phase.String() + " ──",
		})

	case event.AgentOutput:
		m.append(line{kind: lineAgent, role: typed.Assignment.Role, text: typed.Line})

	case event.AgentFinished:
		m.active = false
		m.append(line{
			kind: lineSystem,
			role: typed.Assignment.Role,
			text: typed.Assignment.String() + " terminou em " + typed.Result.Duration.Round(time.Second).String(),
		})

	case event.ValidationStarted:
		m.validating = true
		m.append(line{kind: lineSystem, text: "── forge validando " + typed.TaskID + " ──"})
		for _, command := range typed.Commands {
			m.append(line{kind: lineInfo, text: "$ " + command})
		}

	case event.ValidationOutput:
		m.append(line{kind: lineAgent, text: typed.Line})

	case event.ValidationFinished:
		m.validating = false
		delete(m.files, paneDiff)
		kind := lineOK
		if !typed.Report.Passed() {
			kind = lineFail
		}
		m.append(line{kind: kind, text: "validação: " + typed.Report.Summary()})
		for _, failure := range typed.Report.Failures() {
			for _, output := range strings.Split(failure.Output, "\n") {
				if strings.TrimSpace(output) != "" {
					m.append(line{kind: lineAgent, text: output})
				}
			}
		}

	case event.PhaseChanged:
		m.state = typed.To
		// A correction round is spent when the run enters fixing, which is
		// how the use case budgets it too.
		if typed.To.Phase == workflow.PhaseFixing {
			m.fixes++
		}
		m.append(line{kind: lineInfo, text: typed.From.Phase.String() + " → " + typed.To.Phase.String()})
		// A finished phase may have rewritten the artifacts on the right.
		delete(m.files, panePlan)
		delete(m.files, paneReview)
		delete(m.files, paneDiff)

	case event.Notice:
		kind := lineInfo
		if typed.Level == event.LevelWarn {
			kind = lineWarn
		}
		m.append(line{kind: kind, text: typed.Message})

	case event.RunFinished:
		m.active = false
		m.steps = typed.Steps
		m.fixes = typed.Fixes
		m.state = typed.State
		if typed.LogPath != "" {
			m.logPath = typed.LogPath
		}
		if typed.Err != nil {
			m.runErr = typed.Err
			m.append(line{kind: lineFail, text: typed.Err.Error()})
		}
	}
}

func (m *model) append(entry line) {
	m.lines = append(m.lines, entry)
	if len(m.lines) > maxLines {
		m.lines = m.lines[len(m.lines)-maxLines:]
	}
}

func (m *model) resize(width, height int) {
	m.width, m.height = width, height
	viewWidth, viewHeight := m.viewportSize()
	if !m.ready {
		m.view = viewport.New(viewWidth, viewHeight)
		m.ready = true
	} else {
		m.view.Width, m.view.Height = viewWidth, viewHeight
	}
	m.refresh()
}

// refresh rebuilds the content of the right panel and keeps the tail in sight
// while the operator is following the run.
func (m *model) refresh() {
	if !m.ready {
		return
	}
	m.view.SetContent(m.paneContent())
	if m.follow {
		m.view.GotoBottom()
	}
}

func (m *model) elapsed() time.Duration {
	return time.Since(m.startedAt).Round(time.Second)
}

func (m *model) agentElapsed() time.Duration {
	if !m.active || m.agentStartedAt.IsZero() {
		return 0
	}
	return time.Since(m.agentStartedAt).Round(time.Second)
}
