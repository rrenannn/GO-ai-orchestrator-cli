// Package fsstate implements the state store on top of the shared .agent
// files, the contract both agent CLIs already read and write.
package fsstate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// Layout names the artifacts inside a project.
const (
	AgentDir    = ".agent"
	StatusFile  = "STATUS.md"
	TasksFile   = "TASKS.json"
	RequestFile = "REQUEST.md"
)

// Store reads and writes the orchestration artifacts of a project.
type Store struct{}

// NewStore returns a filesystem-backed state store.
func NewStore() *Store { return &Store{} }

// IsInitialized reports whether the project carries a workflow state file.
func (s *Store) IsInitialized(projectDir string) (bool, error) {
	_, err := os.Stat(filepath.Join(projectDir, AgentDir, StatusFile))
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

// LoadState reads the current workflow position.
func (s *Store) LoadState(projectDir string) (workflow.State, error) {
	return parseStatus(filepath.Join(projectDir, AgentDir, StatusFile))
}

// SaveState writes the workflow position atomically.
func (s *Store) SaveState(projectDir string, state workflow.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(projectDir, AgentDir, StatusFile), renderStatus(state))
}

// LoadBoard reads the task board.
func (s *Store) LoadBoard(projectDir string) (task.Board, error) {
	return parseBoard(filepath.Join(projectDir, AgentDir, TasksFile))
}

// SaveRequest writes the feature request the architect plans from.
func (s *Store) SaveRequest(projectDir string, requirement string) error {
	content := fmt.Sprintf(`# Feature Request

## Objective

%s

## Requirements

- Derive the observable behaviors and constraints from the objective.

## Definition of Done

- Every task in TASKS.json is approved by the reviewer.
`, requirement)
	return writeFileAtomic(filepath.Join(projectDir, AgentDir, RequestFile), []byte(content))
}

// writeFileAtomic avoids leaving a half-written artifact behind when an agent
// or the operator interrupts the run.
func writeFileAtomic(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
