package usecase

import (
	"fmt"

	"github.com/GO-ai-orchestrator-cli/internal/app/port"
)

// InitializeInput is the request of the Initialize use case.
type InitializeInput struct {
	ProjectDir string
	Force      bool
}

// InitializeOutput reports what happened to each managed file.
type InitializeOutput struct {
	Files []port.InstalledFile
}

// Initialize installs the agent instruction files and the shared
// orchestration artifacts into a target project.
type Initialize struct {
	scaffolder port.Scaffolder
}

// NewInitialize wires the use case.
func NewInitialize(scaffolder port.Scaffolder) *Initialize {
	return &Initialize{scaffolder: scaffolder}
}

// Execute runs the use case.
func (u *Initialize) Execute(input InitializeInput) (InitializeOutput, error) {
	files, err := u.scaffolder.Install(input.ProjectDir, input.Force)
	if err != nil {
		return InitializeOutput{}, fmt.Errorf("install orchestration files: %w", err)
	}
	return InitializeOutput{Files: files}, nil
}
