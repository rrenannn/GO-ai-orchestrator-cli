package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/GO-ai-orchestrator-cli/internal/app/usecase"
	"github.com/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/GO-ai-orchestrator-cli/internal/domain/workflow"
)

func TestStartRecordsTheRequestAndPlans(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseApproved, TaskID: "OLD"}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseImplementing, "TA"), board: boardWith(task.StatusImplementing)},
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	cycle, observer := newCycle(store, runner)

	output, err := usecase.NewStart(store, cycle).Execute(context.Background(), usecase.StartInput{
		Cycle:       usecase.CycleInput{ProjectDir: "/project", Observer: observer},
		Requirement: "  add tenant rate limiting  ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.request != "add tenant rate limiting" {
		t.Fatalf("the requirement must be trimmed and stored, got %q", store.request)
	}
	if len(store.saves) == 0 || store.saves[0].Phase != workflow.PhasePlanning {
		t.Fatalf("start must reset the workflow to planning, got %v", store.saves)
	}
	if output.FinalState.Phase != workflow.PhaseCompleted {
		t.Fatalf("want completed, got %s", output.FinalState.Phase)
	}
}

func TestStartRejectsAnEmptyRequirement(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true}
	cycle, observer := newCycle(store, &fakeRunner{store: store})

	_, err := usecase.NewStart(store, cycle).Execute(context.Background(), usecase.StartInput{
		Cycle:       usecase.CycleInput{ProjectDir: "/project", Observer: observer},
		Requirement: "   ",
	})
	if !errors.Is(err, usecase.ErrEmptyRequirement) {
		t.Fatalf("want ErrEmptyRequirement, got %v", err)
	}
}

func TestStartDryRunChangesNothing(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store}
	cycle, observer := newCycle(store, runner)

	if _, err := usecase.NewStart(store, cycle).Execute(context.Background(), usecase.StartInput{
		Cycle:       usecase.CycleInput{ProjectDir: "/project", DryRun: true, Observer: observer},
		Requirement: "add caching",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.request != "" || len(store.saves) != 0 || len(runner.calls) != 0 {
		t.Fatal("a dry run must not touch the project")
	}
}

func TestInitializeReportsEveryManagedFile(t *testing.T) {
	t.Parallel()

	scaffolder := &fakeScaffolder{files: []port.InstalledFile{
		{Path: "CLAUDE.md", Action: port.FileInstalled},
		{Path: "AGENTS.md", Action: port.FilePreserved},
	}}

	output, err := usecase.NewInitialize(scaffolder).Execute(usecase.InitializeInput{ProjectDir: "/project", Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output.Files) != 2 {
		t.Fatalf("want 2 files, got %d", len(output.Files))
	}
	if !scaffolder.force {
		t.Fatal("force must reach the scaffolder")
	}
}

func TestStatusSummarizesTheWorkflow(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		initialized: true,
		state:       workflow.State{Phase: workflow.PhaseReviewing, TaskID: "TA"},
		board:       *boardWith(task.StatusReviewing, task.StatusPending),
	}

	output, err := usecase.NewStatus(store).Execute(usecase.StatusInput{ProjectDir: "/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.IsFinished {
		t.Fatal("a reviewing workflow is not finished")
	}
	if output.Next.Kind != "claude" {
		t.Fatalf("the reviewer is Claude, got %s", output.Next.Kind)
	}
	if len(output.OpenTasks) != 2 {
		t.Fatalf("want 2 open tasks, got %d", len(output.OpenTasks))
	}
}
