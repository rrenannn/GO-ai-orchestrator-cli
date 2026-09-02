// Package gitrepo reads and writes the git repository of a target project.
// It is the only place that knows the git command line.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// maxDiffBytes bounds what the interface is asked to render at once.
const maxDiffBytes = 512 << 10

// Repository runs git against a project directory.
type Repository struct {
	command string
}

// New returns a repository backed by the git executable. An empty name falls
// back to "git".
func New(command string) *Repository {
	if command == "" {
		command = "git"
	}
	return &Repository{command: command}
}

// IsRepository reports whether the directory is inside a git work tree.
func (r *Repository) IsRepository(ctx context.Context, projectDir string) (bool, error) {
	output, err := r.run(ctx, projectDir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		// git answers with an error outside a repository: that is an answer,
		// not a failure.
		return false, nil
	}
	return strings.TrimSpace(output) == "true", nil
}

// Diff returns the uncommitted work of the project: staged and unstaged
// together, which is what the agents have produced so far.
func (r *Repository) Diff(ctx context.Context, projectDir string) (string, error) {
	inside, err := r.IsRepository(ctx, projectDir)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", nil
	}

	// Before the first commit there is no HEAD to compare against.
	arguments := []string{"diff", "HEAD"}
	if _, err := r.run(ctx, projectDir, "rev-parse", "--verify", "HEAD"); err != nil {
		arguments = []string{"diff"}
	}

	diff, err := r.run(ctx, projectDir, arguments...)
	if err != nil {
		return "", err
	}
	return truncate(diff), nil
}

// HasChanges reports whether anything is left to commit.
func (r *Repository) HasChanges(ctx context.Context, projectDir string) (bool, error) {
	status, err := r.run(ctx, projectDir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(status) != "", nil
}

// Commit stages everything and commits it, returning the short hash. Hooks of
// the project are respected: a rejected commit is a real answer.
func (r *Repository) Commit(ctx context.Context, projectDir string, message string) (string, error) {
	if _, err := r.run(ctx, projectDir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := r.run(ctx, projectDir, "commit", "-m", message); err != nil {
		return "", err
	}

	hash, err := r.run(ctx, projectDir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(hash), nil
}

func (r *Repository) run(ctx context.Context, projectDir string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, r.command, arguments...)
	command.Dir = projectDir

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %s", arguments[0], firstLine(stderr.String(), exitErr.Error()))
		}
		return "", fmt.Errorf("git %s: %w", arguments[0], err)
	}
	return stdout.String(), nil
}

func truncate(diff string) string {
	if len(diff) <= maxDiffBytes {
		return diff
	}
	return diff[:maxDiffBytes] + "\n\n[diff truncado pelo forge]\n"
}

func firstLine(candidates ...string) string {
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if index := strings.IndexByte(trimmed, '\n'); index >= 0 {
			return trimmed[:index]
		}
		return trimmed
	}
	return "erro desconhecido"
}
