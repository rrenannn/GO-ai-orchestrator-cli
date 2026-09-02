package workflow_test

import (
	"errors"
	"testing"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

func TestParsePhase(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		input string
		want  workflow.Phase
		fails bool
	}{
		"lowercase":  {input: "planning", want: workflow.PhasePlanning},
		"padded":     {input: "  reviewing\t", want: workflow.PhaseReviewing},
		"mixed case": {input: "Approved", want: workflow.PhaseApproved},
		"unknown":    {input: "shipping", fails: true},
		"empty":      {input: "", fails: true},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := workflow.ParsePhase(testCase.input)
			if testCase.fails {
				if !errors.Is(err, workflow.ErrUnknownPhase) {
					t.Fatalf("want ErrUnknownPhase, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("want %s, got %s", testCase.want, got)
			}
		})
	}
}

func TestStateValidateRequiresTask(t *testing.T) {
	t.Parallel()

	if _, err := workflow.NewState(workflow.PhaseImplementing, ""); !errors.Is(err, workflow.ErrMissingTask) {
		t.Fatalf("want ErrMissingTask, got %v", err)
	}
	if _, err := workflow.NewState(workflow.PhasePlanning, ""); err != nil {
		t.Fatalf("planning must not require a task: %v", err)
	}
}

func TestAdvanceFollowsTheStateMachine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		from    workflow.State
		to      workflow.State
		wantErr error
	}{
		{
			name: "planning starts the first task",
			from: workflow.State{Phase: workflow.PhasePlanning},
			to:   workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T1"},
		},
		{
			name: "implementation goes to review",
			from: workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T1"},
			to:   workflow.State{Phase: workflow.PhaseReviewing, TaskID: "T1"},
		},
		{
			name: "review rejects to fixing",
			from: workflow.State{Phase: workflow.PhaseReviewing, TaskID: "T1"},
			to:   workflow.State{Phase: workflow.PhaseFixing, TaskID: "T1"},
		},
		{
			name: "approval starts the next task",
			from: workflow.State{Phase: workflow.PhaseApproved, TaskID: "T1"},
			to:   workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T2"},
		},
		{
			name:    "review cannot skip to completed",
			from:    workflow.State{Phase: workflow.PhaseReviewing, TaskID: "T1"},
			to:      workflow.State{Phase: workflow.PhaseCompleted, TaskID: "T1"},
			wantErr: workflow.ErrInvalidTransition,
		},
		{
			name:    "builder cannot approve its own work",
			from:    workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T1"},
			to:      workflow.State{Phase: workflow.PhaseApproved, TaskID: "T1"},
			wantErr: workflow.ErrInvalidTransition,
		},
		{
			name:    "an idle agent stalls the workflow",
			from:    workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T1"},
			to:      workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T1"},
			wantErr: workflow.ErrPhaseNotAdvanced,
		},
		{
			name:    "reported state must be valid",
			from:    workflow.State{Phase: workflow.PhasePlanning},
			to:      workflow.State{Phase: workflow.PhaseImplementing},
			wantErr: workflow.ErrMissingTask,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			next, err := workflow.Advance(testCase.from, testCase.to)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("want %v, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if next != testCase.to {
				t.Fatalf("want %v, got %v", testCase.to, next)
			}
		})
	}
}

func TestAgentForRoutesEachPhase(t *testing.T) {
	t.Parallel()

	cases := map[workflow.Phase]agent.Assignment{
		workflow.PhasePlanning:     {Role: agent.RoleArchitect, Kind: agent.KindClaude},
		workflow.PhaseImplementing: {Role: agent.RoleBuilder, Kind: agent.KindCodex},
		workflow.PhaseReviewing:    {Role: agent.RoleReviewer, Kind: agent.KindClaude},
		workflow.PhaseFixing:       {Role: agent.RoleBuilder, Kind: agent.KindCodex},
		workflow.PhaseApproved:     {Role: agent.RoleArchitect, Kind: agent.KindClaude},
	}

	for phase, want := range cases {
		got, err := workflow.AgentFor(phase)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", phase, err)
		}
		if got != want {
			t.Fatalf("%s: want %v, got %v", phase, want, got)
		}
	}

	if _, err := workflow.AgentFor(workflow.PhaseCompleted); !errors.Is(err, workflow.ErrTerminalPhase) {
		t.Fatalf("want ErrTerminalPhase, got %v", err)
	}
}
