package workflow

import (
	"fmt"
	"strings"
)

// State is the persisted position of a project inside the state machine.
type State struct {
	Phase  Phase
	TaskID string
}

// NewState builds a validated state.
func NewState(phase Phase, taskID string) (State, error) {
	state := State{Phase: phase, TaskID: strings.TrimSpace(taskID)}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

// Validate enforces the invariants of a persisted state.
func (s State) Validate() error {
	if !s.Phase.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownPhase, s.Phase)
	}
	if s.Phase.RequiresTask() && s.TaskID == "" {
		return fmt.Errorf("%w: %s", ErrMissingTask, s.Phase)
	}
	return nil
}

// String renders the state for logs and terminal output.
func (s State) String() string {
	if s.TaskID == "" {
		return s.Phase.String()
	}
	return fmt.Sprintf("%s[%s]", s.Phase, s.TaskID)
}
