package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/validation"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// fakeStore is an in-memory stand-in for the .agent files.
type fakeStore struct {
	initialized bool
	state       workflow.State
	board       task.Board
	request     string
	saves       []workflow.State
	validations []validation.Report
	loadErr     error
}

func (s *fakeStore) IsInitialized(string) (bool, error) { return s.initialized, nil }

func (s *fakeStore) LoadState(string) (workflow.State, error) {
	if s.loadErr != nil {
		return workflow.State{}, s.loadErr
	}
	return s.state, nil
}

func (s *fakeStore) SaveState(_ string, state workflow.State) error {
	s.state = state
	s.saves = append(s.saves, state)
	return nil
}

func (s *fakeStore) LoadBoard(string) (task.Board, error) { return s.board, nil }

func (s *fakeStore) SaveRequest(_ string, requirement string) error {
	s.request = requirement
	return nil
}

func (s *fakeStore) SaveValidation(_ string, report validation.Report) error {
	s.validations = append(s.validations, report)
	return nil
}

// fakeValidator replays scripted validation outcomes.
type fakeValidator struct {
	results [][]validation.Result
	err     error
	calls   [][]string
}

func (v *fakeValidator) Validate(
	_ context.Context,
	_ string,
	commands []string,
	_ time.Duration,
	sink io.Writer,
) ([]validation.Result, error) {
	v.calls = append(v.calls, commands)
	if v.err != nil {
		return nil, v.err
	}
	if sink != nil {
		io.WriteString(sink, "validando\n")
	}
	if len(v.results) == 0 {
		return passing(commands), nil
	}

	current := v.results[0]
	v.results = v.results[1:]
	return current, nil
}

// passing is the default outcome: every command succeeded.
func passing(commands []string) []validation.Result {
	results := make([]validation.Result, 0, len(commands))
	for _, command := range commands {
		results = append(results, validation.Result{Command: command})
	}
	return results
}

// failing builds the outcome of a command that ran and failed.
func failing(command string) []validation.Result {
	return []validation.Result{{Command: command, ExitCode: 1, Output: "FAIL ./internal/http"}}
}

// step is one scripted agent turn: what it writes back and how it exits.
type step struct {
	writes   *workflow.State
	board    *task.Board
	exitCode int
	err      error
}

// fakeRunner replays scripted agent turns against the fake store.
type fakeRunner struct {
	store       *fakeStore
	script      []step
	calls       []agent.Invocation
	unavailable map[agent.Kind]error
}

func (r *fakeRunner) Available(kind agent.Kind) error { return r.unavailable[kind] }

func (r *fakeRunner) Run(_ context.Context, invocation agent.Invocation, sink io.Writer) (agent.Result, error) {
	r.calls = append(r.calls, invocation)
	if sink != nil {
		io.WriteString(sink, invocation.Prompt)
	}
	if len(r.script) == 0 {
		return agent.Result{}, errors.New("fakeRunner: no scripted turn left")
	}

	current := r.script[0]
	r.script = r.script[1:]
	if current.writes != nil {
		r.store.state = *current.writes
	}
	if current.board != nil {
		r.store.board = *current.board
	}
	return agent.Result{ExitCode: current.exitCode, Duration: time.Second}, current.err
}

func (r *fakeRunner) kinds() []agent.Kind {
	kinds := make([]agent.Kind, 0, len(r.calls))
	for _, call := range r.calls {
		kinds = append(kinds, call.Assignment.Kind)
	}
	return kinds
}

// fakeLogs keeps the transcript in memory.
type fakeLogs struct{ buffer bytes.Buffer }

func (l *fakeLogs) Open(string, time.Time) (io.WriteCloser, string, error) {
	return nopCloser{&l.buffer}, "/tmp/run.log", nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// fakeObserver records everything a run publishes.
type fakeObserver struct{ events []event.Event }

func (o *fakeObserver) Publish(published event.Event) {
	o.events = append(o.events, published)
}

// countOf reports how many events of a given type were published.
func countOf[T event.Event](o *fakeObserver) int {
	total := 0
	for _, published := range o.events {
		if _, ok := published.(T); ok {
			total++
		}
	}
	return total
}

// firstOf returns the first event of a given type.
func firstOf[T event.Event](o *fakeObserver) (T, bool) {
	for _, published := range o.events {
		if typed, ok := published.(T); ok {
			return typed, true
		}
	}
	var zero T
	return zero, false
}

// blockingGate pauses the loop until it is released.
type blockingGate struct{ release chan struct{} }

func newBlockingGate() *blockingGate {
	return &blockingGate{release: make(chan struct{})}
}

func (g *blockingGate) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.release:
		return nil
	}
}

// fixedClock keeps run log names deterministic.
type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }

// fakeScaffolder reports a fixed installation outcome.
type fakeScaffolder struct {
	files []port.InstalledFile
	err   error
	force bool
}

func (s *fakeScaffolder) Install(_ string, force bool) ([]port.InstalledFile, error) {
	s.force = force
	return s.files, s.err
}

func state(phase workflow.Phase, taskID string) *workflow.State {
	return &workflow.State{Phase: phase, TaskID: taskID}
}

func boardWith(statuses ...task.Status) *task.Board {
	built := task.Board{Version: 1}
	for index, status := range statuses {
		id := string(rune('A' + index))
		built.Tasks = append(built.Tasks, task.Task{
			ID:         "T" + id,
			Objective:  "task " + id,
			Status:     status,
			Validation: []string{"go test ./..."},
		})
	}
	return &built
}
