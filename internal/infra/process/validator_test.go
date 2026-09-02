package process_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/process"
)

var _ port.Validator = (*process.Validator)(nil)

func TestValidateRunsEveryCommandAndKeepsGoingAfterAFailure(t *testing.T) {
	t.Parallel()

	sink := &strings.Builder{}
	results, err := process.NewValidator().Validate(
		context.Background(),
		t.TempDir(),
		[]string{"echo primeiro", "exit 3", "echo terceiro"},
		0,
		sink,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	if !results[0].Passed() || results[0].Output != "primeiro" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].Passed() || results[1].ExitCode != 3 {
		t.Fatalf("a failing command must be reported, not hidden: %+v", results[1])
	}
	if !results[2].Passed() {
		t.Fatal("a failure must not stop the commands that follow it")
	}
	if !strings.Contains(sink.String(), "terceiro") {
		t.Fatalf("output must be streamed: %q", sink.String())
	}
}

func TestValidateRunsInTheProjectDirectory(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	results, err := process.NewValidator().Validate(
		context.Background(),
		project,
		[]string{"pwd"},
		0,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// macOS reports /private/var for /var, so compare the tail.
	if !strings.HasSuffix(results[0].Output, strings.TrimPrefix(project, "/private")) {
		t.Fatalf("commands must run in the project: %q vs %q", results[0].Output, project)
	}
}

func TestValidateSkipsBlankCommands(t *testing.T) {
	t.Parallel()

	results, err := process.NewValidator().Validate(
		context.Background(),
		t.TempDir(),
		[]string{"   ", "true"},
		0,
		io.Discard,
	)
	if err != nil || len(results) != 1 {
		t.Fatalf("want a single result, got %+v (err=%v)", results, err)
	}
}

func TestValidateHonorsTheTimeout(t *testing.T) {
	t.Parallel()

	started := time.Now()
	results, err := process.NewValidator().Validate(
		context.Background(),
		t.TempDir(),
		[]string{"sleep 30"},
		150*time.Millisecond,
		io.Discard,
	)
	if err != nil {
		t.Fatalf("a timeout is a failed command, not a transport error: %v", err)
	}
	if results[0].Passed() {
		t.Fatal("a command killed by the timeout must not pass")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("the timeout did not kill the command: took %s", elapsed)
	}
}

func TestValidateStopsWhenTheRunIsCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := process.NewValidator().Validate(ctx, t.TempDir(), []string{"true"}, 0, io.Discard)
	if err == nil {
		t.Fatal("a cancelled run must not keep validating")
	}
}
