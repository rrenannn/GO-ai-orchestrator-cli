package workflow

import "fmt"

// transitions is the whole state machine:
//
//	planning ─▶ implementing ─▶ reviewing ─┬─▶ approved ─┬─▶ implementing (next task)
//	                 ▲                     │             └─▶ completed
//	                 └──── fixing ◀────────┘
var transitions = map[Phase][]Phase{
	PhasePlanning:     {PhaseImplementing, PhaseCompleted},
	PhaseImplementing: {PhaseReviewing},
	PhaseReviewing:    {PhaseApproved, PhaseFixing},
	PhaseFixing:       {PhaseReviewing},
	PhaseApproved:     {PhaseImplementing, PhaseCompleted},
	PhaseCompleted:    {},
}

// CanTransition reports whether moving from one phase to another is legal.
func CanTransition(from, to Phase) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Advance validates the move an agent reported and returns the next state.
// A reported phase equal to the current one means the agent did not do its job.
func Advance(current State, reported State) (State, error) {
	if err := reported.Validate(); err != nil {
		return State{}, err
	}
	if reported.Phase == current.Phase {
		return State{}, fmt.Errorf("%w: still %s", ErrPhaseNotAdvanced, current.Phase)
	}
	if !CanTransition(current.Phase, reported.Phase) {
		return State{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Phase, reported.Phase)
	}
	return reported, nil
}
