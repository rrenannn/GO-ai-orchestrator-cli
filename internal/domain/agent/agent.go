// Package agent describes the AI agents the orchestrator can dispatch.
// It is a leaf domain package: it knows nothing about workflows, files or processes.
package agent

import (
	"errors"
	"fmt"
	"time"
)

// ErrUnknownKind is returned when a kind has no known executor.
var ErrUnknownKind = errors.New("unknown agent kind")

// Kind identifies which CLI backs an agent.
type Kind string

const (
	// KindClaude is the Claude Code CLI.
	KindClaude Kind = "claude"
	// KindCodex is the Codex CLI.
	KindCodex Kind = "codex"
)

// Role is the responsibility an agent takes in the workflow.
type Role string

const (
	// RoleArchitect plans the work and selects the next task.
	RoleArchitect Role = "architect"
	// RoleBuilder implements tasks and applies review findings.
	RoleBuilder Role = "builder"
	// RoleReviewer approves or rejects an implementation.
	RoleReviewer Role = "reviewer"
)

// Assignment binds a role to the CLI that performs it.
type Assignment struct {
	Role Role
	Kind Kind
}

// String renders the assignment for logs and terminal output.
func (a Assignment) String() string {
	return fmt.Sprintf("%s (%s)", a.Role, a.Kind)
}

// Invocation is a single agent dispatch request.
type Invocation struct {
	Assignment Assignment
	WorkDir    string
	Prompt     string
	Timeout    time.Duration
}

// Validate guards the invariants an executor relies on.
func (i Invocation) Validate() error {
	switch {
	case i.WorkDir == "":
		return errors.New("agent invocation requires a working directory")
	case i.Prompt == "":
		return errors.New("agent invocation requires a prompt")
	case i.Assignment.Kind == "":
		return ErrUnknownKind
	default:
		return nil
	}
}

// Result reports how a dispatch ended.
type Result struct {
	ExitCode int
	Duration time.Duration
}
