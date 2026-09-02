// Package port declares the outbound interfaces the use cases depend on.
// Every implementation lives in internal/infra; the application layer never
// imports it back.
package port

import (
	"context"
	"io"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// StateStore persists the shared orchestration artifacts of a project.
type StateStore interface {
	// IsInitialized reports whether the project carries orchestration state.
	IsInitialized(projectDir string) (bool, error)
	// LoadState reads the current workflow position.
	LoadState(projectDir string) (workflow.State, error)
	// SaveState writes the workflow position.
	SaveState(projectDir string, state workflow.State) error
	// LoadBoard reads the task board.
	LoadBoard(projectDir string) (task.Board, error)
	// SaveRequest writes the feature request the architect will plan from.
	SaveRequest(projectDir string, requirement string) error
}

// AgentRunner dispatches an invocation to the CLI of an agent.
// Implementations stream everything the agent writes to sink.
type AgentRunner interface {
	Run(ctx context.Context, invocation agent.Invocation, sink io.Writer) (agent.Result, error)
	// Available reports a missing executable before any state is changed.
	Available(kind agent.Kind) error
}

// FileAction is what a scaffolder did with a single template file.
type FileAction string

const (
	// FileInstalled means the file was written.
	FileInstalled FileAction = "installed"
	// FilePreserved means an existing file was kept untouched.
	FilePreserved FileAction = "preserved"
)

// InstalledFile reports the outcome for one scaffolded file.
type InstalledFile struct {
	Path   string
	Action FileAction
}

// Scaffolder installs the agent instruction files into a project.
type Scaffolder interface {
	Install(projectDir string, force bool) ([]InstalledFile, error)
}

// RunLog opens the append-only transcript of one orchestration run.
type RunLog interface {
	Open(projectDir string, startedAt time.Time) (io.WriteCloser, string, error)
}

// Observer receives everything a run reports, as it happens. The plain
// renderer and the TUI are two implementations of this single port.
type Observer interface {
	Publish(event event.Event)
}

// Gate lets the operator hold the loop between dispatches. A nil gate means
// the run is never paused.
type Gate interface {
	// Wait blocks while the run is paused and returns when it may proceed,
	// or an error if the context ends first.
	Wait(ctx context.Context) error
}

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}
