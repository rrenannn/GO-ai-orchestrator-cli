package usecase

import (
	"fmt"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// StatusInput is the request of the Status use case.
type StatusInput struct {
	ProjectDir string
}

// StatusOutput is a snapshot of the workflow, ready to be presented.
type StatusOutput struct {
	State      workflow.State
	Next       agent.Assignment
	Board      task.Board
	OpenTasks  []task.Task
	IsFinished bool
}

// Status reports the current workflow position of a project.
type Status struct {
	store port.StateStore
}

// NewStatus wires the use case.
func NewStatus(store port.StateStore) *Status {
	return &Status{store: store}
}

// Execute runs the use case.
func (u *Status) Execute(input StatusInput) (StatusOutput, error) {
	if err := ensureInitialized(u.store, input.ProjectDir); err != nil {
		return StatusOutput{}, err
	}

	state, err := u.store.LoadState(input.ProjectDir)
	if err != nil {
		return StatusOutput{}, fmt.Errorf("load workflow state: %w", err)
	}

	board, err := u.store.LoadBoard(input.ProjectDir)
	if err != nil {
		return StatusOutput{}, fmt.Errorf("load task board: %w", err)
	}

	output := StatusOutput{
		State:      state,
		Board:      board,
		OpenTasks:  board.Open(),
		IsFinished: state.Phase.IsTerminal(),
	}
	if !output.IsFinished {
		if next, err := workflow.AgentFor(state.Phase); err == nil {
			output.Next = next
		}
	}
	return output, nil
}

func ensureInitialized(store port.StateStore, projectDir string) error {
	initialized, err := store.IsInitialized(projectDir)
	if err != nil {
		return fmt.Errorf("inspect project: %w", err)
	}
	if !initialized {
		return fmt.Errorf("%w: %s", ErrNotInitialized, projectDir)
	}
	return nil
}
