package workflow

import "errors"

var (
	// ErrUnknownPhase reports a phase value outside the state machine.
	ErrUnknownPhase = errors.New("unknown workflow phase")
	// ErrInvalidTransition reports a move the state machine forbids.
	ErrInvalidTransition = errors.New("invalid workflow transition")
	// ErrMissingTask reports a phase that requires a task without one.
	ErrMissingTask = errors.New("workflow phase requires a task id")
	// ErrPhaseNotAdvanced reports an agent that finished without moving the workflow.
	ErrPhaseNotAdvanced = errors.New("agent finished without advancing the workflow")
	// ErrTerminalPhase reports an attempt to dispatch work for a finished workflow.
	ErrTerminalPhase = errors.New("workflow already completed")
)
