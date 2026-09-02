// Package event describes everything an orchestration run reports while it
// happens. The use cases publish these; the delivery layer decides whether to
// print a line or repaint a panel. Neither side knows about the other.
package event

import (
	"time"

	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// Event is the sealed set of things a run can report.
type Event interface {
	isEvent()
}

// Level qualifies a free-form notice.
type Level string

const (
	// LevelInfo is normal progress.
	LevelInfo Level = "info"
	// LevelWarn is a recoverable problem.
	LevelWarn Level = "warn"
)

// RunStarted opens a run.
type RunStarted struct {
	ProjectDir  string
	Requirement string
	LogPath     string
	MaxSteps    int
	MaxFixes    int
	StartedAt   time.Time
	DryRun      bool
}

// BoardUpdated carries a fresh snapshot of the workflow position and tasks.
type BoardUpdated struct {
	State workflow.State
	Board task.Board
}

// AgentStarted announces a dispatch.
type AgentStarted struct {
	Assignment agent.Assignment
	State      workflow.State
	Step       int
	MaxSteps   int
	Prompt     string
}

// AgentOutput is one line of an agent transcript.
type AgentOutput struct {
	Assignment agent.Assignment
	Line       string
}

// AgentFinished closes a dispatch.
type AgentFinished struct {
	Assignment agent.Assignment
	Result     agent.Result
}

// PhaseChanged reports an accepted workflow transition.
type PhaseChanged struct {
	From workflow.State
	To   workflow.State
}

// Paused and Resumed report operator control over the loop.
type Paused struct{}

// Resumed reports the loop leaving a pause.
type Resumed struct{}

// Notice is a free-form message that has no richer shape.
type Notice struct {
	Level   Level
	Message string
}

// RunFinished closes a run, successfully or not.
type RunFinished struct {
	State   workflow.State
	Steps   int
	Fixes   int
	LogPath string
	Err     error
}

func (RunStarted) isEvent()    {}
func (BoardUpdated) isEvent()  {}
func (AgentStarted) isEvent()  {}
func (AgentOutput) isEvent()   {}
func (AgentFinished) isEvent() {}
func (PhaseChanged) isEvent()  {}
func (Paused) isEvent()        {}
func (Resumed) isEvent()       {}
func (Notice) isEvent()        {}
func (RunFinished) isEvent()   {}
