package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// StartInput is the request of the Start use case.
type StartInput struct {
	Cycle       CycleInput
	Requirement string
}

// Start records a new feature request and drives the workflow from planning.
type Start struct {
	store port.StateStore
	cycle *Cycle
}

// NewStart wires the use case.
func NewStart(store port.StateStore, cycle *Cycle) *Start {
	return &Start{store: store, cycle: cycle}
}

// Execute runs the use case.
func (u *Start) Execute(ctx context.Context, input StartInput) (CycleOutput, error) {
	requirement := strings.TrimSpace(input.Requirement)
	if requirement == "" {
		return CycleOutput{}, ErrEmptyRequirement
	}
	if err := ensureInitialized(u.store, input.Cycle.ProjectDir); err != nil {
		return CycleOutput{}, err
	}

	observer := input.Cycle.withDefaults().Observer
	if input.Cycle.DryRun {
		observer.Publish(event.Notice{
			Level:   event.LevelInfo,
			Message: "simulação: registraria o pedido e começaria o planejamento",
		})
		return CycleOutput{}, nil
	}

	if err := u.store.SaveRequest(input.Cycle.ProjectDir, requirement); err != nil {
		return CycleOutput{}, fmt.Errorf("write feature request: %w", err)
	}

	planning := workflow.State{Phase: workflow.PhasePlanning}
	if err := u.store.SaveState(input.Cycle.ProjectDir, planning); err != nil {
		return CycleOutput{}, fmt.Errorf("reset workflow state: %w", err)
	}

	input.Cycle.Requirement = requirement
	return u.cycle.Execute(ctx, input.Cycle)
}
