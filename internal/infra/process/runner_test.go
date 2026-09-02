package process_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/GO-ai-orchestrator-cli/internal/infra/process"
)

var _ port.AgentRunner = (*process.Runner)(nil)

func TestAvailable(t *testing.T) {
	t.Parallel()

	runner := process.NewRunner("echo", "definitely-not-installed-codex")

	if err := runner.Available(agent.KindClaude); err != nil {
		t.Fatalf("echo is installed: %v", err)
	}
	if err := runner.Available(agent.KindCodex); err == nil {
		t.Fatal("a missing executable must be reported before dispatching")
	}
	if err := runner.Available("gemini"); !errors.Is(err, agent.ErrUnknownKind) {
		t.Fatalf("want ErrUnknownKind, got %v", err)
	}
}

func TestRunStreamsOutputToTheSink(t *testing.T) {
	t.Parallel()

	runner := process.NewRunner("echo", "echo")
	sink := &strings.Builder{}

	result, err := runner.Run(context.Background(), agent.Invocation{
		Assignment: agent.Assignment{Role: agent.RoleArchitect, Kind: agent.KindClaude},
		WorkDir:    t.TempDir(),
		Prompt:     "plan the work",
	}, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("want exit 0, got %d", result.ExitCode)
	}
	// echo receives the CLI flags plus the prompt, so the transcript proves
	// both the argument construction and the streaming.
	if !strings.Contains(sink.String(), "--permission-mode acceptEdits --print plan the work") {
		t.Fatalf("unexpected transcript: %q", sink.String())
	}
}

func TestRunReportsNonZeroExitAsResult(t *testing.T) {
	t.Parallel()

	runner := process.NewRunner("false", "false")

	result, err := runner.Run(context.Background(), agent.Invocation{
		Assignment: agent.Assignment{Role: agent.RoleBuilder, Kind: agent.KindCodex},
		WorkDir:    t.TempDir(),
		Prompt:     "implement",
	}, io.Discard)
	if err != nil {
		t.Fatalf("a nonzero exit is a result, not a transport error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("want a nonzero exit code")
	}
}

func TestRunRejectsIncompleteInvocations(t *testing.T) {
	t.Parallel()

	runner := process.NewRunner("echo", "echo")
	_, err := runner.Run(context.Background(), agent.Invocation{
		Assignment: agent.Assignment{Kind: agent.KindClaude},
		WorkDir:    t.TempDir(),
	}, io.Discard)
	if err == nil {
		t.Fatal("an empty prompt must be rejected")
	}
}
