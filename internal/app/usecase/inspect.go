package usecase

import (
	"context"
	"fmt"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
)

// InspectInput is the request of the Inspect use case.
type InspectInput struct {
	ProjectDir string
}

// InspectOutput carries what the agents have changed so far.
type InspectOutput struct {
	Diff string
}

// Inspect reads the uncommitted work of a project, so the operator can see
// what the agents wrote without leaving the interface.
type Inspect struct {
	workspace port.Workspace
}

// NewInspect wires the use case.
func NewInspect(workspace port.Workspace) *Inspect {
	return &Inspect{workspace: workspace}
}

// Execute runs the use case.
func (u *Inspect) Execute(ctx context.Context, input InspectInput) (InspectOutput, error) {
	diff, err := u.workspace.Diff(ctx, input.ProjectDir)
	if err != nil {
		return InspectOutput{}, fmt.Errorf("read the diff: %w", err)
	}
	return InspectOutput{Diff: diff}, nil
}
