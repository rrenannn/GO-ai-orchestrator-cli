package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/GO-ai-orchestrator-cli/internal/cli"
	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/GO-ai-orchestrator-cli/internal/domain/workflow"
)

func TestPlainRendererPrintsTheWholeRun(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	renderer := cli.NewPlainRenderer(out)
	assignment := agent.Assignment{Role: agent.RoleBuilder, Kind: agent.KindCodex}
	implementing := workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T1"}

	renderer.Publish(event.RunStarted{ProjectDir: "/tmp/demo", Requirement: "add health check", LogPath: "/tmp/demo/.agent/runs/x.log"})
	renderer.Publish(event.AgentStarted{Assignment: assignment, State: implementing, Step: 2, MaxSteps: 12})
	renderer.Publish(event.AgentOutput{Assignment: assignment, Line: "go test ./... ok"})
	renderer.Publish(event.AgentFinished{Assignment: assignment, Result: agent.Result{Duration: 3 * time.Second}})
	renderer.Publish(event.PhaseChanged{From: implementing, To: workflow.State{Phase: workflow.PhaseReviewing, TaskID: "T1"}})
	renderer.Publish(event.Notice{Level: event.LevelWarn, Message: "state not persisted"})
	renderer.Publish(event.RunFinished{State: workflow.State{Phase: workflow.PhaseCompleted}, Steps: 6, Fixes: 1, LogPath: "/tmp/demo/.agent/runs/x.log"})

	for _, fragment := range []string{
		"request: add health check",
		"[2/12] implementing: dispatching builder (codex)",
		"go test ./... ok",
		"builder (codex) finished in 3s",
		"implementing -> reviewing",
		"warning: state not persisted",
		"phase=completed steps=6 fixes=1",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("the transcript must contain %q:\n%s", fragment, out)
		}
	}
}

func TestPlainRendererIgnoresUnknownEvents(t *testing.T) {
	t.Parallel()

	out := &bytes.Buffer{}
	renderer := cli.NewPlainRenderer(out)
	renderer.Publish(event.Paused{})
	renderer.Publish(event.Resumed{})
	renderer.Publish(event.RunFinished{Err: errors.New("boom")})

	if strings.Contains(out.String(), "boom") {
		t.Fatal("the failure is reported by the command, not twice by the transcript")
	}
}
