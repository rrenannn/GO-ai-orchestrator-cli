// Package usecase contains the application-specific orchestration rules.
// It depends on the domain and on port interfaces only.
package usecase

import "errors"

var (
	// ErrNotInitialized reports a project without orchestration artifacts.
	ErrNotInitialized = errors.New("project is not initialized")
	// ErrFixLimit reports too many correction rounds for a single task.
	ErrFixLimit = errors.New("correction limit reached")
	// ErrStepLimit reports too many agent dispatches in one run.
	ErrStepLimit = errors.New("dispatch limit reached")
	// ErrAgentFailed reports a nonzero exit from an agent CLI.
	ErrAgentFailed = errors.New("agent failed")
	// ErrEmptyRequirement reports a start command without a requirement.
	ErrEmptyRequirement = errors.New("requirement must not be empty")
)
