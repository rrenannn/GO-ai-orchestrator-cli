package tui

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// maxLines caps the in-memory transcript. The full one is on disk anyway.
const maxLines = 5000

// Messages exchanged inside the interface.
type (
	eventMsg    struct{ published event.Event }
	finishedMsg struct{ err error }
	tickMsg     time.Time
	fileMsg     struct {
		pane    pane
		content string
	}
)

// pane is one of the views of the right side.
type pane int

const (
	paneLive pane = iota
	panePlan
	paneReview
)

var paneNames = map[pane]string{paneLive: "Live", panePlan: "Plan", paneReview: "Review"}

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
)

type line struct {
	kind lineKind
	role agent.Role
	text string
}

// model is the whole state of the interface.
type model struct {
	session *Session
	spin    spinner.Model
	view    viewport.Model
	ready   bool
	width   int
	height  int

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

	lines    []line
	files    map[pane]string
	pane     pane
	follow   bool
	paused   bool
	finished bool
	runErr   error

	confirmQuit bool
}

func newModel(session *Session) *model {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = styleAccent

	return &model{
		session:   session,
		spin:      spin,
		files:     map[pane]string{},
		follow:    true,
		startedAt: time.Now(),
	}
}

// Init starts the spinner and the one-second clock that keeps timers honest.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, everySecond())
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

	case finishedMsg:
		m.finished = true
		m.active = false
		if typed.err != nil && m.runErr == nil {
			m.runErr = typed.err
			m.append(line{kind: lineFail, text: typed.err.Error()})
		}
		m.refresh()
		return m, nil

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
	if m.confirmQuit {
		switch key.String() {
		case "q", "ctrl+c", "y", "enter":
			return m, tea.Quit
		default:
			m.confirmQuit = false
			m.refresh()
			return m, nil
		}
	}

	switch key.String() {
	case "q", "ctrl+c", "esc":
		if m.finished {
			return m, tea.Quit
		}
		m.confirmQuit = true
		m.refresh()
		return m, nil

	case "tab", "right", "l":
		return m, m.selectPane((m.pane + 1) % 3)

	case "shift+tab", "left", "h":
		return m, m.selectPane((m.pane + 2) % 3)

	case "1":
		return m, m.selectPane(paneLive)
	case "2":
		return m, m.selectPane(panePlan)
	case "3":
		return m, m.selectPane(paneReview)

	case "p":
		if !m.finished {
			m.paused = m.session.togglePause()
			if m.paused {
				m.append(line{kind: lineInfo, text: "paused: the loop stops before the next dispatch"})
			} else {
				m.append(line{kind: lineInfo, text: "resumed"})
			}
			m.refresh()
		}
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
	}

	previous := m.view.YOffset
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(key)
	if m.view.YOffset != previous {
		m.follow = m.view.AtBottom()
	}
	return m, cmd
}

func (m *model) selectPane(target pane) tea.Cmd {
	m.pane = target
	m.refresh()
	if target == paneLive {
		return nil
	}
	return m.loadPane(target)
}

// loadPane reads an artifact from the project. Reading it fresh on demand
// keeps the interface honest: it shows what the agents actually wrote.
func (m *model) loadPane(target pane) tea.Cmd {
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
		m.append(line{kind: lineSystem, text: "run started in " + typed.ProjectDir})

	case event.BoardUpdated:
		m.state = typed.State
		m.board = typed.Board

	case event.AgentStarted:
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
			text: "── " + typed.Assignment.String() + " · phase " + typed.State.Phase.String() + " ──",
		})

	case event.AgentOutput:
		m.append(line{kind: lineAgent, role: typed.Assignment.Role, text: typed.Line})

	case event.AgentFinished:
		m.active = false
		m.append(line{
			kind: lineSystem,
			role: typed.Assignment.Role,
			text: typed.Assignment.String() + " finished in " + typed.Result.Duration.Round(time.Second).String(),
		})

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

	case event.Notice:
		kind := lineInfo
		if typed.Level == event.LevelWarn {
			kind = lineWarn
		}
		m.append(line{kind: kind, text: typed.Message})

	case event.RunFinished:
		m.finished = true
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
