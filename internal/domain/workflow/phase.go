// Package workflow holds the orchestration state machine:
// which phase the work is in, who acts next, and which moves are legal.
package workflow

import (
	"fmt"
	"strings"

	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
)

// Phase is a step of the request lifecycle.
type Phase string

const (
	// PhasePlanning waits for the architect to produce the plan and the tasks.
	PhasePlanning Phase = "planning"
	// PhaseImplementing waits for the builder to implement the current task.
	PhaseImplementing Phase = "implementing"
	// PhaseReviewing waits for the reviewer verdict.
	PhaseReviewing Phase = "reviewing"
	// PhaseFixing waits for the builder to apply review findings.
	PhaseFixing Phase = "fixing"
	// PhaseApproved waits for the architect to pick the next task.
	PhaseApproved Phase = "approved"
	// PhaseCompleted is terminal: no task is left open.
	PhaseCompleted Phase = "completed"
)

// Phases lists every supported phase in lifecycle order.
func Phases() []Phase {
	return []Phase{
		PhasePlanning,
		PhaseImplementing,
		PhaseReviewing,
		PhaseFixing,
		PhaseApproved,
		PhaseCompleted,
	}
}

// ParsePhase converts persisted text into a phase.
func ParsePhase(value string) (Phase, error) {
	candidate := Phase(strings.ToLower(strings.TrimSpace(value)))
	if !candidate.Valid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownPhase, value)
	}
	return candidate, nil
}

// Valid reports whether the phase belongs to the state machine.
func (p Phase) Valid() bool {
	for _, known := range Phases() {
		if p == known {
			return true
		}
	}
	return false
}

// String implements fmt.Stringer.
func (p Phase) String() string { return string(p) }

// IsTerminal reports whether no further dispatch is expected.
func (p Phase) IsTerminal() bool { return p == PhaseCompleted }

// RequiresTask reports whether the phase is meaningless without a task id.
func (p Phase) RequiresTask() bool {
	switch p {
	case PhaseImplementing, PhaseReviewing, PhaseFixing, PhaseApproved:
		return true
	default:
		return false
	}
}

// AgentFor returns the agent responsible for moving the phase forward.
func AgentFor(phase Phase) (agent.Assignment, error) {
	switch phase {
	case PhasePlanning, PhaseApproved:
		return agent.Assignment{Role: agent.RoleArchitect, Kind: agent.KindClaude}, nil
	case PhaseImplementing, PhaseFixing:
		return agent.Assignment{Role: agent.RoleBuilder, Kind: agent.KindCodex}, nil
	case PhaseReviewing:
		return agent.Assignment{Role: agent.RoleReviewer, Kind: agent.KindClaude}, nil
	case PhaseCompleted:
		return agent.Assignment{}, ErrTerminalPhase
	default:
		return agent.Assignment{}, fmt.Errorf("%w: %q", ErrUnknownPhase, phase)
	}
}
