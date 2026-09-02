package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/prompt"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// Default limits keep a runaway loop bounded without human supervision.
const (
	DefaultMaxFixes = 2
	DefaultMaxSteps = 12
)

// CycleInput is the request of the Cycle use case. Observer and Gate are
// run-scoped collaborators: the delivery layer supplies the ones that match
// the presentation it chose for this run.
type CycleInput struct {
	ProjectDir   string
	Requirement  string
	DryRun       bool
	MaxFixes     int
	MaxSteps     int
	AgentTimeout time.Duration
	Observer     port.Observer
	Gate         port.Gate
}

func (i CycleInput) withDefaults() CycleInput {
	if i.MaxFixes <= 0 {
		i.MaxFixes = DefaultMaxFixes
	}
	if i.MaxSteps <= 0 {
		i.MaxSteps = DefaultMaxSteps
	}
	if i.Observer == nil {
		i.Observer = silentObserver{}
	}
	if i.Gate == nil {
		i.Gate = openGate{}
	}
	return i
}

type silentObserver struct{}

func (silentObserver) Publish(event.Event) {}

type openGate struct{}

func (openGate) Wait(context.Context) error { return nil }

// CycleOutput reports how the run ended.
type CycleOutput struct {
	FinalState workflow.State
	Steps      int
	Fixes      int
	LogPath    string
	Planned    *agent.Invocation // set on dry runs only
}

// Cycle drives the orchestration loop:
// architect plans, builder implements, reviewer approves or rejects,
// builder fixes, architect selects the next task, until nothing is open.
type Cycle struct {
	store   port.StateStore
	runner  port.AgentRunner
	logs    port.RunLog
	clock   port.Clock
	prompts prompt.Builder
}

// NewCycle wires the use case.
func NewCycle(
	store port.StateStore,
	runner port.AgentRunner,
	logs port.RunLog,
	clock port.Clock,
	prompts prompt.Builder,
) *Cycle {
	return &Cycle{store: store, runner: runner, logs: logs, clock: clock, prompts: prompts}
}

// Execute runs the loop until the workflow completes, a limit is reached,
// an agent fails, or the operator interrupts it.
func (u *Cycle) Execute(ctx context.Context, input CycleInput) (output CycleOutput, err error) {
	input = input.withDefaults()

	if err := ensureInitialized(u.store, input.ProjectDir); err != nil {
		return CycleOutput{}, err
	}

	state, err := u.store.LoadState(input.ProjectDir)
	if err != nil {
		return CycleOutput{}, fmt.Errorf("load workflow state: %w", err)
	}

	if input.DryRun {
		return u.preview(input, state)
	}

	sink, logPath, err := u.logs.Open(input.ProjectDir, u.clock.Now())
	if err != nil {
		return CycleOutput{}, fmt.Errorf("open run log: %w", err)
	}
	defer sink.Close()

	input.Observer.Publish(event.RunStarted{
		ProjectDir:  input.ProjectDir,
		Requirement: input.Requirement,
		LogPath:     logPath,
		MaxSteps:    input.MaxSteps,
		MaxFixes:    input.MaxFixes,
		StartedAt:   u.clock.Now(),
	})
	defer func() {
		input.Observer.Publish(event.RunFinished{
			State:   output.FinalState,
			Steps:   output.Steps,
			Fixes:   output.Fixes,
			LogPath: output.LogPath,
			Err:     err,
		})
	}()

	output = CycleOutput{FinalState: state, LogPath: logPath}
	u.publishBoard(input, state)

	// The correction budget is per task: a new task starts with a fresh one.
	fixedTaskID, fixesForTask := "", 0
	for {
		if err := ctx.Err(); err != nil {
			return output, err
		}
		if state.Phase.IsTerminal() {
			input.Observer.Publish(event.Notice{Level: event.LevelInfo, Message: "execução concluída: nenhuma tarefa aberta restante"})
			return output, nil
		}
		if err := input.Gate.Wait(ctx); err != nil {
			return output, err
		}

		invocation, err := u.invocationFor(input, state)
		if err != nil {
			return output, err
		}
		if state.Phase == workflow.PhaseFixing {
			if fixedTaskID != state.TaskID {
				fixedTaskID, fixesForTask = state.TaskID, 0
			}
			if fixesForTask >= input.MaxFixes {
				return output, fmt.Errorf("%w (%d) for task %s: review %s", ErrFixLimit, input.MaxFixes, state.TaskID, prompt.ReviewFile)
			}
			fixesForTask++
			output.Fixes++
		}
		if output.Steps >= input.MaxSteps {
			return output, fmt.Errorf("%w (%d): inspect %s", ErrStepLimit, input.MaxSteps, logPath)
		}
		output.Steps++

		next, err := u.dispatch(ctx, input, state, invocation, sink, output.Steps)
		output.FinalState = next
		if err != nil {
			return output, err
		}
		state = next
	}
}

// preview resolves the agent that would act next without changing anything.
func (u *Cycle) preview(input CycleInput, state workflow.State) (CycleOutput, error) {
	u.publishBoard(input, state)
	if state.Phase.IsTerminal() {
		input.Observer.Publish(event.Notice{Level: event.LevelInfo, Message: "simulação: a execução já está concluída"})
		return CycleOutput{FinalState: state}, nil
	}
	invocation, err := u.invocationFor(input, state)
	if err != nil {
		return CycleOutput{}, err
	}
	input.Observer.Publish(event.Notice{
		Level:   event.LevelInfo,
		Message: fmt.Sprintf("simulação: despacharia %s na fase %s", invocation.Assignment, state.Phase),
	})
	return CycleOutput{FinalState: state, Planned: &invocation}, nil
}

// dispatch runs one agent and returns the state it left behind.
func (u *Cycle) dispatch(
	ctx context.Context,
	input CycleInput,
	current workflow.State,
	invocation agent.Invocation,
	sink io.Writer,
	step int,
) (workflow.State, error) {
	input.Observer.Publish(event.AgentStarted{
		Assignment: invocation.Assignment,
		State:      current,
		Step:       step,
		MaxSteps:   input.MaxSteps,
		Prompt:     invocation.Prompt,
	})

	stream := newLineWriter(func(line string) {
		input.Observer.Publish(event.AgentOutput{Assignment: invocation.Assignment, Line: line})
	})

	result, err := u.runner.Run(ctx, invocation, io.MultiWriter(sink, stream))
	stream.Close()
	input.Observer.Publish(event.AgentFinished{Assignment: invocation.Assignment, Result: result})

	if err != nil {
		return current, fmt.Errorf("%w: %s: %w", ErrAgentFailed, invocation.Assignment, err)
	}
	if result.ExitCode != 0 {
		return current, fmt.Errorf("%w: %s exited with %d", ErrAgentFailed, invocation.Assignment, result.ExitCode)
	}

	reported, err := u.store.LoadState(input.ProjectDir)
	if err != nil {
		return current, fmt.Errorf("reload workflow state: %w", err)
	}

	next, err := workflow.Advance(current, reported)
	if err != nil {
		resolved, ok := u.resolveStall(input, current, reported, err)
		if !ok {
			return current, err
		}
		next = resolved
	}

	input.Observer.Publish(event.PhaseChanged{From: current, To: next})
	u.publishBoard(input, next)
	return next, nil
}

// resolveStall covers the one benign stall: the architect confirmed the last
// approval and left the workflow on approved because nothing is open. The run
// completes instead of failing, and the terminal state is persisted.
func (u *Cycle) resolveStall(input CycleInput, current, reported workflow.State, cause error) (workflow.State, bool) {
	if !errors.Is(cause, workflow.ErrPhaseNotAdvanced) || current.Phase != workflow.PhaseApproved {
		return workflow.State{}, false
	}

	board, err := u.store.LoadBoard(input.ProjectDir)
	if err != nil || board.HasOpen() {
		return workflow.State{}, false
	}

	final := workflow.State{Phase: workflow.PhaseCompleted, TaskID: reported.TaskID}
	if err := u.store.SaveState(input.ProjectDir, final); err != nil {
		input.Observer.Publish(event.Notice{
			Level:   event.LevelWarn,
			Message: fmt.Sprintf("não foi possível persistir o estado concluído: %v", err),
		})
	}
	return final, true
}

func (u *Cycle) invocationFor(input CycleInput, state workflow.State) (agent.Invocation, error) {
	assignment, err := workflow.AgentFor(state.Phase)
	if err != nil {
		return agent.Invocation{}, err
	}
	if err := u.runner.Available(assignment.Kind); err != nil {
		return agent.Invocation{}, err
	}

	current, err := u.currentTask(input.ProjectDir, state)
	if err != nil {
		return agent.Invocation{}, err
	}

	text, err := u.prompts.Build(state, current)
	if err != nil {
		return agent.Invocation{}, err
	}

	invocation := agent.Invocation{
		Assignment: assignment,
		WorkDir:    input.ProjectDir,
		Prompt:     text,
		Timeout:    input.AgentTimeout,
	}
	if err := invocation.Validate(); err != nil {
		return agent.Invocation{}, err
	}
	return invocation, nil
}

// currentTask enriches the prompt with the objective when the board knows it.
// A board that cannot be read yet (planning) is not an error.
func (u *Cycle) currentTask(projectDir string, state workflow.State) (task.Task, error) {
	if state.TaskID == "" {
		return task.Task{}, nil
	}
	board, err := u.store.LoadBoard(projectDir)
	if err != nil {
		return task.Task{}, fmt.Errorf("load task board: %w", err)
	}
	found, _ := board.Find(state.TaskID)
	return found, nil
}

// publishBoard gives the delivery layer a fresh view of the tasks. A board
// that cannot be read is not worth failing a run for.
func (u *Cycle) publishBoard(input CycleInput, state workflow.State) {
	board, err := u.store.LoadBoard(input.ProjectDir)
	if err != nil {
		return
	}
	input.Observer.Publish(event.BoardUpdated{State: state, Board: board})
}
