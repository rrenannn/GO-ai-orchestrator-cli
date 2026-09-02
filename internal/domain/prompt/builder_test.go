package prompt_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/prompt"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

func TestBuildCoversEveryActivePhase(t *testing.T) {
	t.Parallel()

	builder := prompt.NewBuilder()
	current := task.Task{ID: "T1", Objective: "add rate limiting", Status: task.StatusPending}

	cases := map[workflow.Phase][]string{
		workflow.PhasePlanning:     {"ARCHITECT", prompt.PlanFile, prompt.TasksFile, "phase=implementing"},
		workflow.PhaseImplementing: {"BUILDER", "T1 - add rate limiting", "phase=reviewing"},
		workflow.PhaseReviewing:    {"REVIEWER", "APPROVED", "CHANGES REQUESTED", "phase=fixing"},
		workflow.PhaseFixing:       {"BUILDER", prompt.ReviewFile, "phase=reviewing"},
		workflow.PhaseApproved:     {"ARCHITECT", "next open task", "phase=completed"},
	}

	for phase, fragments := range cases {
		state := workflow.State{Phase: phase, TaskID: "T1"}
		if phase == workflow.PhasePlanning {
			state.TaskID = ""
		}

		text, err := builder.Build(state, current)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", phase, err)
		}
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				t.Fatalf("%s prompt is missing %q:\n%s", phase, fragment, text)
			}
		}
	}
}

func TestBuildFallsBackToTheTaskID(t *testing.T) {
	t.Parallel()

	state := workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T9"}
	text, err := prompt.NewBuilder().Build(state, task.Task{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "T9") {
		t.Fatalf("prompt must name the task:\n%s", text)
	}
}

func TestBuildRejectsTerminalPhase(t *testing.T) {
	t.Parallel()

	state := workflow.State{Phase: workflow.PhaseCompleted}
	if _, err := prompt.NewBuilder().Build(state, task.Task{}); !errors.Is(err, workflow.ErrTerminalPhase) {
		t.Fatalf("want ErrTerminalPhase, got %v", err)
	}
}
