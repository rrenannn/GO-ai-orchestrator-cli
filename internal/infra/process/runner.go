// Package process dispatches agents by executing their CLIs as child
// processes, streaming their output to the console and to the run log.
package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
)

// Defaults for the executables backing each agent kind.
const (
	DefaultClaudeCommand = "claude"
	DefaultCodexCommand  = "codex"
)

// Runner executes agent CLIs. Argument construction is the only place that
// knows how each vendor CLI is invoked. Everything the child process writes
// goes to the sink given per run, never straight to the terminal: the
// delivery layer owns the screen.
type Runner struct {
	commands map[agent.Kind]commandSpec
}

type commandSpec struct {
	executable string
	arguments  func(executable string, invocation agent.Invocation) []string
}

// NewRunner builds a runner. Empty command names fall back to the defaults.
func NewRunner(claudeCommand, codexCommand string) *Runner {
	if claudeCommand == "" {
		claudeCommand = DefaultClaudeCommand
	}
	if codexCommand == "" {
		codexCommand = DefaultCodexCommand
	}
	return &Runner{
		commands: map[agent.Kind]commandSpec{
			agent.KindClaude: {
				executable: claudeCommand,
				arguments: func(_ string, invocation agent.Invocation) []string {
					return []string{"--permission-mode", "acceptEdits", "--print", invocation.Prompt}
				},
			},
			agent.KindCodex: {
				executable: codexCommand,
				arguments: func(_ string, invocation agent.Invocation) []string {
					return []string{"exec", "--cd", invocation.WorkDir, "--sandbox", "workspace-write", invocation.Prompt}
				},
			},
		},
	}
}

// Available reports a missing executable before any state is changed.
func (r *Runner) Available(kind agent.Kind) error {
	spec, ok := r.commands[kind]
	if !ok {
		return fmt.Errorf("%w: %s", agent.ErrUnknownKind, kind)
	}
	if _, err := exec.LookPath(spec.executable); err != nil {
		return fmt.Errorf("%s executable not found in PATH: %s", kind, spec.executable)
	}
	return nil
}

// Run executes the agent CLI and reports how it ended. A nonzero exit is a
// result, not a transport error; only a failure to run is an error.
func (r *Runner) Run(ctx context.Context, invocation agent.Invocation, sink io.Writer) (agent.Result, error) {
	if err := invocation.Validate(); err != nil {
		return agent.Result{}, err
	}
	spec, ok := r.commands[invocation.Assignment.Kind]
	if !ok {
		return agent.Result{}, fmt.Errorf("%w: %s", agent.ErrUnknownKind, invocation.Assignment.Kind)
	}

	if invocation.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, invocation.Timeout)
		defer cancel()
	}

	command := exec.CommandContext(ctx, spec.executable, spec.arguments(spec.executable, invocation)...)
	command.Dir = invocation.WorkDir
	command.Stdout = sink
	command.Stderr = sink

	started := time.Now()
	err := command.Run()
	result := agent.Result{Duration: time.Since(started)}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return result, nil
	case errors.As(err, &exitErr):
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	default:
		return result, fmt.Errorf("run %s: %w", spec.executable, err)
	}
}
