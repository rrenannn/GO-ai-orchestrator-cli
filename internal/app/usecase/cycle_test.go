package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/usecase"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/prompt"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/validation"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

func newCycle(store *fakeStore, runner *fakeRunner) (*usecase.Cycle, *fakeObserver) {
	cycle, observer, _ := newValidatingCycle(store, runner, &fakeValidator{})
	return cycle, observer
}

// newValidatingCycle exposes the validator so a test can script its outcome.
func newValidatingCycle(store *fakeStore, runner *fakeRunner, validator *fakeValidator) (*usecase.Cycle, *fakeObserver, *fakeValidator) {
	observer := &fakeObserver{}
	cycle := usecase.NewCycle(store, runner, validator, &fakeLogs{}, fixedClock{}, prompt.NewBuilder())
	return cycle, observer, validator
}

// run executes a cycle with the observer the helper created.
func run(t *testing.T, cycle *usecase.Cycle, observer *fakeObserver, input usecase.CycleInput) (usecase.CycleOutput, error) {
	t.Helper()
	input.Observer = observer
	return cycle.Execute(context.Background(), input)
}

func TestCycleRunsTheFullWorkflow(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseImplementing, "TA"), board: boardWith(task.StatusImplementing)},
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	cycle, observer := newCycle(store, runner)

	output, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.FinalState.Phase != workflow.PhaseCompleted {
		t.Fatalf("want completed, got %s", output.FinalState.Phase)
	}
	if output.Steps != 4 {
		t.Fatalf("want 4 dispatches, got %d", output.Steps)
	}

	want := []agent.Kind{agent.KindClaude, agent.KindCodex, agent.KindClaude, agent.KindClaude}
	got := runner.kinds()
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("dispatch %d: want %s, got %s", index, want[index], got[index])
		}
	}
}

func TestCycleSendsRejectionsBackToTheBuilder(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseReviewing, TaskID: "TA"}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseFixing, "TA")},
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	cycle, observer := newCycle(store, runner)

	output, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Fixes != 1 {
		t.Fatalf("want 1 correction round, got %d", output.Fixes)
	}
	if runner.calls[1].Assignment.Kind != agent.KindCodex {
		t.Fatalf("a rejection must go back to Codex, got %s", runner.calls[1].Assignment.Kind)
	}
}

func TestCycleStopsAtTheCorrectionLimit(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseReviewing, TaskID: "TA"}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseFixing, "TA")},
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseFixing, "TA")},
	}}
	cycle, observer := newCycle(store, runner)

	_, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project", MaxFixes: 1})
	if !errors.Is(err, usecase.ErrFixLimit) {
		t.Fatalf("want ErrFixLimit, got %v", err)
	}
}

func TestCycleStopsAtTheDispatchLimit(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseImplementing, TaskID: "TA"}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseFixing, "TA")},
	}}
	cycle, observer := newCycle(store, runner)

	output, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project", MaxSteps: 1})
	if !errors.Is(err, usecase.ErrStepLimit) {
		t.Fatalf("want ErrStepLimit, got %v", err)
	}
	if output.Steps != 1 {
		t.Fatalf("want 1 dispatch before the limit, got %d", output.Steps)
	}
}

func TestCycleRejectsIllegalTransitions(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseImplementing, TaskID: "TA"}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseApproved, "TA")}, // the builder cannot approve itself
	}}
	cycle, observer := newCycle(store, runner)

	if _, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"}); !errors.Is(err, workflow.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
}

func TestCycleFailsWhenAnAgentExitsNonZero(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store, script: []step{{exitCode: 3}}}
	cycle, observer := newCycle(store, runner)

	if _, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"}); !errors.Is(err, usecase.ErrAgentFailed) {
		t.Fatalf("want ErrAgentFailed, got %v", err)
	}
}

func TestCycleCompletesWhenTheArchitectFindsNoOpenTask(t *testing.T) {
	t.Parallel()

	// The architect leaves the phase on approved because every task is done.
	store := &fakeStore{
		initialized: true,
		state:       workflow.State{Phase: workflow.PhaseApproved, TaskID: "TA"},
		board:       *boardWith(task.StatusApproved),
	}
	runner := &fakeRunner{store: store, script: []step{{}}}
	cycle, observer := newCycle(store, runner)

	output, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.FinalState.Phase != workflow.PhaseCompleted {
		t.Fatalf("want completed, got %s", output.FinalState.Phase)
	}
	if store.state.Phase != workflow.PhaseCompleted {
		t.Fatalf("the terminal state must be persisted, got %s", store.state.Phase)
	}
}

func TestCycleFailsWhenTheArchitectStallsWithOpenTasks(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		initialized: true,
		state:       workflow.State{Phase: workflow.PhaseApproved, TaskID: "TA"},
		board:       *boardWith(task.StatusApproved, task.StatusPending),
	}
	runner := &fakeRunner{store: store, script: []step{{}}}
	cycle, observer := newCycle(store, runner)

	if _, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"}); !errors.Is(err, workflow.ErrPhaseNotAdvanced) {
		t.Fatalf("want ErrPhaseNotAdvanced, got %v", err)
	}
}

func TestCycleDryRunDispatchesNothing(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseReviewing, TaskID: "TA"}}
	runner := &fakeRunner{store: store}
	cycle, observer := newCycle(store, runner)

	output, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project", DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("a dry run must not dispatch, got %d calls", len(runner.calls))
	}
	if output.Planned == nil || output.Planned.Assignment.Role != agent.RoleReviewer {
		t.Fatalf("want a planned reviewer invocation, got %+v", output.Planned)
	}
}

func TestCycleRequiresAnInitializedProject(t *testing.T) {
	t.Parallel()

	cycle, observer := newCycle(&fakeStore{}, &fakeRunner{})
	if _, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"}); !errors.Is(err, usecase.ErrNotInitialized) {
		t.Fatalf("want ErrNotInitialized, got %v", err)
	}
}

func TestCycleFailsFastOnAMissingExecutable(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	missing := errors.New("claude executable not found in PATH")
	runner := &fakeRunner{store: store, unavailable: map[agent.Kind]error{agent.KindClaude: missing}}
	cycle, observer := newCycle(store, runner)

	if _, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"}); !errors.Is(err, missing) {
		t.Fatalf("want the lookup error, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("nothing must be dispatched when the executable is missing")
	}
}

func TestCycleHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store, script: []step{{writes: state(workflow.PhaseImplementing, "TA")}}}
	cycle, observer := newCycle(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := cycle.Execute(ctx, usecase.CycleInput{ProjectDir: "/project", Observer: observer}); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestCycleGivesEachTaskItsOwnCorrectionBudget(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseReviewing, TaskID: "TA"}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseFixing, "TA")},                                          // TA rejected once
		{writes: state(workflow.PhaseReviewing, "TA")},                                       // builder fixed TA
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)}, // TA approved
		{writes: state(workflow.PhaseImplementing, "TB")},                                    // architect picks TB
		{writes: state(workflow.PhaseReviewing, "TB")},                                       // builder implements TB
		{writes: state(workflow.PhaseFixing, "TB")},                                          // TB rejected once
		{writes: state(workflow.PhaseReviewing, "TB")},                                       // builder fixed TB
		{writes: state(workflow.PhaseApproved, "TB")},                                        // TB approved
		{writes: state(workflow.PhaseCompleted, "TB")},
	}}
	cycle, observer := newCycle(store, runner)

	output, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project", MaxFixes: 1})
	if err != nil {
		t.Fatalf("one correction per task must fit in the budget: %v", err)
	}
	if output.Fixes != 2 {
		t.Fatalf("want 2 correction rounds in total, got %d", output.Fixes)
	}
}

func TestCycleReportsWhatItIsDoing(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseImplementing, "TA"), board: boardWith(task.StatusImplementing)},
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	cycle, observer := newCycle(store, runner)

	if _, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project", Requirement: "add health check"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	started, ok := firstOf[event.RunStarted](observer)
	if !ok || started.Requirement != "add health check" || started.LogPath == "" {
		t.Fatalf("the run must open with its requirement and log: %+v", started)
	}
	if got := countOf[event.AgentStarted](observer); got != 4 {
		t.Fatalf("want 4 dispatch announcements, got %d", got)
	}
	if got := countOf[event.AgentFinished](observer); got != 4 {
		t.Fatalf("want 4 dispatch completions, got %d", got)
	}
	if got := countOf[event.PhaseChanged](observer); got != 4 {
		t.Fatalf("want 4 transitions, got %d", got)
	}
	// The fake runner echoes the prompt, so the transcript must reach the UI.
	if countOf[event.AgentOutput](observer) == 0 {
		t.Fatal("agent output must be published line by line")
	}

	finished, ok := firstOf[event.RunFinished](observer)
	if !ok || finished.Err != nil || finished.State.Phase != workflow.PhaseCompleted {
		t.Fatalf("the run must close with its outcome: %+v", finished)
	}
}

func TestCycleReportsTheFailureThatEndedTheRun(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store, script: []step{{exitCode: 2}}}
	cycle, observer := newCycle(store, runner)

	if _, err := run(t, cycle, observer, usecase.CycleInput{ProjectDir: "/project"}); err == nil {
		t.Fatal("want an error")
	}
	finished, ok := firstOf[event.RunFinished](observer)
	if !ok || !errors.Is(finished.Err, usecase.ErrAgentFailed) {
		t.Fatalf("the closing event must carry the failure: %+v", finished)
	}
}

func TestCycleHoldsAtTheGateBeforeDispatching(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseImplementing, "TA")},
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	cycle, observer := newCycle(store, runner)
	gate := newBlockingGate()

	done := make(chan error, 1)
	go func() {
		_, err := cycle.Execute(context.Background(), usecase.CycleInput{
			ProjectDir: "/project",
			Observer:   observer,
			Gate:       gate,
		})
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("the run must wait at the gate, it returned %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(gate.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error after releasing the gate: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the run did not resume after the gate opened")
	}
}

func TestCycleGateCancellationStopsTheRun(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhasePlanning}}
	runner := &fakeRunner{store: store}
	cycle, observer := newCycle(store, runner)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := cycle.Execute(ctx, usecase.CycleInput{
		ProjectDir: "/project",
		Observer:   observer,
		Gate:       newBlockingGate(),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("nothing must be dispatched while paused")
	}
}

func TestValidationFailureGoesStraightBackToTheBuilder(t *testing.T) {
	t.Parallel()

	store := &fakeStore{initialized: true, state: workflow.State{Phase: workflow.PhaseImplementing, TaskID: "TA"}}
	store.board = *boardWith(task.StatusImplementing)
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseReviewing, "TA")}, // the builder claims it is done
		{writes: state(workflow.PhaseReviewing, "TA")}, // and fixes it after forge disagrees
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	validator := &fakeValidator{results: [][]validation.Result{failing("go test ./...")}}
	cycle, observer, _ := newValidatingCycle(store, runner, validator)

	output, err := cycle.Execute(context.Background(), usecase.CycleInput{ProjectDir: "/project", Observer: observer})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The reviewer is never asked about a failing build: builder, builder, reviewer, architect.
	kinds := runner.kinds()
	roles := make([]agent.Role, 0, len(runner.calls))
	for _, call := range runner.calls {
		roles = append(roles, call.Assignment.Role)
	}
	want := []agent.Role{agent.RoleBuilder, agent.RoleBuilder, agent.RoleReviewer, agent.RoleArchitect}
	if len(roles) != len(want) {
		t.Fatalf("want %v, got %v (%v)", want, roles, kinds)
	}
	for index := range want {
		if roles[index] != want[index] {
			t.Fatalf("dispatch %d: want %s, got %s", index, want[index], roles[index])
		}
	}
	if output.Fixes != 1 {
		t.Fatalf("a failed validation spends a correction round, got %d", output.Fixes)
	}

	// The failure is persisted so the builder can read what broke.
	if len(store.validations) != 2 || store.validations[0].Passed() {
		t.Fatalf("the evidence must be recorded: %+v", store.validations)
	}
	if len(store.saves) == 0 || store.saves[0].Phase != workflow.PhaseFixing {
		t.Fatalf("forge must move the workflow to fixing itself: %+v", store.saves)
	}
	if got := countOf[event.ValidationFinished](observer); got != 2 {
		t.Fatalf("want 2 validation verdicts published, got %d", got)
	}
}

func TestValidationPassesToTheReviewer(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		initialized: true,
		state:       workflow.State{Phase: workflow.PhaseImplementing, TaskID: "TA"},
		board:       *boardWith(task.StatusImplementing),
	}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	validator := &fakeValidator{}
	cycle, observer, _ := newValidatingCycle(store, runner, validator)

	if _, err := cycle.Execute(context.Background(), usecase.CycleInput{ProjectDir: "/project", Observer: observer}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(validator.calls) != 1 || validator.calls[0][0] != "go test ./..." {
		t.Fatalf("the declared command must be run: %+v", validator.calls)
	}
	if runner.calls[1].Assignment.Role != agent.RoleReviewer {
		t.Fatalf("a passing build goes to the reviewer, got %s", runner.calls[1].Assignment.Role)
	}
	if len(store.validations) != 1 || !store.validations[0].Passed() {
		t.Fatalf("the evidence must be recorded even when it passes: %+v", store.validations)
	}
}

func TestValidationIsSkippedWhenTheTaskDeclaresNoCommands(t *testing.T) {
	t.Parallel()

	board := task.Board{Tasks: []task.Task{{ID: "TA", Objective: "no tests", Status: task.StatusImplementing}}}
	store := &fakeStore{
		initialized: true,
		state:       workflow.State{Phase: workflow.PhaseImplementing, TaskID: "TA"},
		board:       board,
	}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	validator := &fakeValidator{}
	cycle, observer, _ := newValidatingCycle(store, runner, validator)

	if _, err := cycle.Execute(context.Background(), usecase.CycleInput{ProjectDir: "/project", Observer: observer}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(validator.calls) != 0 {
		t.Fatalf("nothing to run means nothing is run: %+v", validator.calls)
	}
	if runner.calls[1].Assignment.Role != agent.RoleReviewer {
		t.Fatal("the task must still reach the reviewer")
	}
	if len(store.validations) != 1 || !store.validations[0].Empty() {
		t.Fatalf("an empty report is still recorded: %+v", store.validations)
	}
}

func TestValidationCanBeTurnedOff(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		initialized: true,
		state:       workflow.State{Phase: workflow.PhaseImplementing, TaskID: "TA"},
		board:       *boardWith(task.StatusImplementing),
	}
	runner := &fakeRunner{store: store, script: []step{
		{writes: state(workflow.PhaseReviewing, "TA")},
		{writes: state(workflow.PhaseApproved, "TA"), board: boardWith(task.StatusApproved)},
		{writes: state(workflow.PhaseCompleted, "TA")},
	}}
	validator := &fakeValidator{results: [][]validation.Result{failing("go test ./...")}}
	cycle, observer, _ := newValidatingCycle(store, runner, validator)

	if _, err := cycle.Execute(context.Background(), usecase.CycleInput{
		ProjectDir:     "/project",
		Observer:       observer,
		SkipValidation: true,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(validator.calls) != 0 {
		t.Fatal("--no-validate must not run anything")
	}
	if runner.calls[1].Assignment.Role != agent.RoleReviewer {
		t.Fatal("without validation the workflow behaves as before")
	}
}

func TestValidationThatCannotRunStopsTheExecution(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		initialized: true,
		state:       workflow.State{Phase: workflow.PhaseImplementing, TaskID: "TA"},
		board:       *boardWith(task.StatusImplementing),
	}
	runner := &fakeRunner{store: store, script: []step{{writes: state(workflow.PhaseReviewing, "TA")}}}
	validator := &fakeValidator{err: errors.New("shell not found")}
	cycle, observer, _ := newValidatingCycle(store, runner, validator)

	_, err := cycle.Execute(context.Background(), usecase.CycleInput{ProjectDir: "/project", Observer: observer})
	if !errors.Is(err, usecase.ErrValidationFailed) {
		t.Fatalf("want ErrValidationFailed, got %v", err)
	}
}
