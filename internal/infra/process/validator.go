package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/validation"
)

// Shell runs the validation commands the plan declared. They are written for
// a shell, not for a process table, so they go through one.
const (
	Shell     = "sh"
	ShellFlag = "-c"
)

// maxCapturedOutput bounds what a report carries: enough tail to explain a
// failure, never enough to blow up the artifact the agents read next.
const maxCapturedOutput = 4 << 10

// Validator runs declared validation commands and reports what happened.
// It is deliberately dumb: it decides nothing about the workflow, it only
// produces evidence.
type Validator struct{}

// NewValidator returns the shell-backed validator.
func NewValidator() *Validator { return &Validator{} }

// Validate runs every command, in order, without stopping at the first
// failure: a report that covers all of them explains more than one that
// stops early. Output is streamed to sink and its tail kept in the result.
func (v *Validator) Validate(
	ctx context.Context,
	projectDir string,
	commands []string,
	timeout time.Duration,
	sink io.Writer,
) ([]validation.Result, error) {
	results := make([]validation.Result, 0, len(commands))

	for _, command := range commands {
		if strings.TrimSpace(command) == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return results, err
		}
		results = append(results, v.run(ctx, projectDir, command, timeout, sink))
	}
	return results, nil
}

func (v *Validator) run(
	ctx context.Context,
	projectDir string,
	command string,
	timeout time.Duration,
	sink io.Writer,
) validation.Result {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	tail := &tailWriter{limit: maxCapturedOutput}
	shell := exec.CommandContext(ctx, Shell, ShellFlag, command)
	shell.Dir = projectDir
	shell.Stdout = io.MultiWriter(sink, tail)
	shell.Stderr = shell.Stdout

	started := time.Now()
	err := shell.Run()
	result := validation.Result{
		Command:  command,
		Output:   tail.String(),
		Duration: time.Since(started),
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return result
	case errors.As(err, &exitErr):
		result.ExitCode = exitErr.ExitCode()
	default:
		result.Err = fmt.Errorf("run %q: %w", command, err)
	}
	return result
}

// tailWriter keeps only the last bytes written to it.
type tailWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (w *tailWriter) Write(chunk []byte) (int, error) {
	written := len(chunk)
	w.buffer.Write(chunk)

	if excess := w.buffer.Len() - w.limit; excess > 0 {
		w.buffer.Next(excess)
	}
	return written, nil
}

func (w *tailWriter) String() string {
	return strings.TrimSpace(w.buffer.String())
}
