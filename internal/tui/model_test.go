package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

var (
	_ port.Observer = (*Session)(nil)
	_ port.Gate     = (*Session)(nil)
)

var architect = agent.Assignment{Role: agent.RoleArchitect, Kind: agent.KindClaude}
var builder = agent.Assignment{Role: agent.RoleBuilder, Kind: agent.KindCodex}

// screen renders a model of a fixed size, ready to be asserted on.
func screen(t *testing.T, events ...event.Event) (*model, string) {
	t.Helper()

	built := newModel(NewSession("/tmp/demo"), Actions{})
	built.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	for _, published := range events {
		built.Update(eventMsg{published})
	}
	return built, built.View()
}

func TestViewShowsTheRunAtAGlance(t *testing.T) {
	t.Parallel()

	board := task.Board{
		CurrentTaskID: "T2",
		Tasks: []task.Task{
			{ID: "T1", Objective: "add config loader", Status: task.StatusApproved},
			{ID: "T2", Objective: "add rate limiter", Status: task.StatusImplementing},
			{ID: "T3", Objective: "document the API", Status: task.StatusPending},
		},
	}

	_, view := screen(t,
		event.RunStarted{ProjectDir: "/tmp/demo", Requirement: "add rate limiting", LogPath: "/tmp/demo/.agent/runs/x.log", MaxSteps: 12, MaxFixes: 2},
		event.BoardUpdated{State: workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T2"}, Board: board},
		event.AgentStarted{Assignment: builder, State: workflow.State{Phase: workflow.PhaseImplementing, TaskID: "T2"}, Step: 2, MaxSteps: 12},
		event.AgentOutput{Assignment: builder, Line: "editing internal/http/handler.go"},
	)

	for _, fragment := range []string{
		"forge",
		"add rate limiting",
		"T1 add config loader",
		"T2 add rate limiter",
		"passos     2/12",
		"editing internal/http/handler.go",
		"RODANDO",
		"BUILDER",
		"Live",
	} {
		if !strings.Contains(view, fragment) {
			t.Fatalf("the interface must show %q:\n%s", fragment, view)
		}
	}
}

func TestPipelineMarksProgressAndTheFixingBranch(t *testing.T) {
	t.Parallel()

	_, reviewing := screen(t, event.BoardUpdated{State: workflow.State{Phase: workflow.PhaseReviewing, TaskID: "T1"}})
	if !strings.Contains(reviewing, "✓ plan") || !strings.Contains(reviewing, "● review") {
		t.Fatalf("the pipeline must mark the finished and current phases:\n%s", reviewing)
	}

	_, fixing := screen(t, event.BoardUpdated{State: workflow.State{Phase: workflow.PhaseFixing, TaskID: "T1"}})
	if !strings.Contains(fixing, "↺ fixing") {
		t.Fatalf("fixing is a branch of review and must be visible:\n%s", fixing)
	}
}

func TestViewReportsTheOutcome(t *testing.T) {
	t.Parallel()

	// A run ends with the closing event and then the dispatch returning: only
	// then does the prompt come back.
	done, _ := screen(t,
		event.RunStarted{ProjectDir: "/tmp/demo", LogPath: "/tmp/log", MaxSteps: 12, MaxFixes: 2},
		event.RunFinished{State: workflow.State{Phase: workflow.PhaseCompleted, TaskID: "T3"}, Steps: 9, Fixes: 1, LogPath: "/tmp/log"},
	)
	done.Update(runDoneMsg{})

	view := done.View()
	if !strings.Contains(view, "PRONTO") || !strings.Contains(view, "9 passos") {
		t.Fatalf("a finished run must show its summary:\n%s", view)
	}
	if done.mode != modeIdle || !done.prompt.Focused() {
		t.Fatal("the prompt must come back after a run")
	}

	failed, _ := screen(t,
		event.RunStarted{ProjectDir: "/tmp/demo", MaxSteps: 12},
		event.RunFinished{State: workflow.State{Phase: workflow.PhaseFixing, TaskID: "T1"}, Err: context.DeadlineExceeded},
	)
	failed.Update(runDoneMsg{err: context.DeadlineExceeded})
	if !strings.Contains(failed.View(), "FALHOU") {
		t.Fatalf("a failed run must be unmistakable:\n%s", failed.View())
	}
}

func TestPauseKeyHoldsTheLoop(t *testing.T) {
	t.Parallel()

	built := newModel(NewSession("/tmp/demo"), Actions{})
	built.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	built.mode = modeRunning
	built.Update(eventMsg{event.AgentStarted{Assignment: architect, Step: 1, MaxSteps: 12}})

	built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !built.paused {
		t.Fatal("p must pause the run")
	}
	if !strings.Contains(built.View(), "PAUSADO") {
		t.Fatal("a paused run must say so")
	}

	// The gate now blocks the orchestration goroutine.
	blocked := make(chan error, 1)
	go func() { blocked <- built.session.Wait(context.Background()) }()
	select {
	case <-blocked:
		t.Fatal("the gate must hold while paused")
	case <-time.After(30 * time.Millisecond):
	}

	built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	select {
	case err := <-blocked:
		if err != nil {
			t.Fatalf("unexpected gate error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the gate must open when the run resumes")
	}
}

func TestPaneSwitchingLoadsTheArtifacts(t *testing.T) {
	t.Parallel()

	built, _ := screen(t, event.RunStarted{ProjectDir: t.TempDir(), MaxSteps: 12})

	if cmd := built.selectPane(panePlan); cmd == nil {
		t.Fatal("switching to Plan must read .agent/PLAN.md")
	}
	message, ok := built.selectPane(panePlan)().(fileMsg)
	if !ok || message.pane != panePlan {
		t.Fatalf("unexpected message: %#v", message)
	}
	if !strings.Contains(message.content, "PLAN.md") {
		t.Fatalf("a missing plan must be reported, got %q", message.content)
	}

	if cmd := built.selectPane(paneLive); cmd != nil {
		t.Fatal("the live pane reads nothing from disk")
	}
}

func TestPublishNeverBlocksTheRun(t *testing.T) {
	t.Parallel()

	session := NewSession("/tmp/demo")
	for index := 0; index < eventBuffer+100; index++ {
		session.Publish(event.AgentOutput{Line: "noise"})
	}
	if session.droppedLines() == 0 {
		t.Fatal("overflow must be counted instead of blocking the orchestration loop")
	}
}

func TestFixCounterMatchesTheRunBudget(t *testing.T) {
	t.Parallel()

	reviewing := workflow.State{Phase: workflow.PhaseReviewing, TaskID: "T1"}
	fixing := workflow.State{Phase: workflow.PhaseFixing, TaskID: "T1"}

	built, _ := screen(t,
		event.RunStarted{ProjectDir: "/tmp/demo", MaxSteps: 12, MaxFixes: 2},
		event.PhaseChanged{From: reviewing, To: fixing},
	)
	if built.fixes != 1 {
		t.Fatalf("entering fixing spends a correction round, got %d", built.fixes)
	}
	if !strings.Contains(built.View(), "correções  1/2") {
		t.Fatalf("the budget must be visible:\n%s", built.View())
	}

	built.Update(eventMsg{event.PhaseChanged{From: fixing, To: reviewing}})
	if built.fixes != 1 {
		t.Fatalf("leaving fixing must not spend another round, got %d", built.fixes)
	}
}

// typeInto feeds a string into the prompt, one key at a time.
func typeInto(built *model, text string) {
	for _, symbol := range text {
		built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{symbol}})
	}
}

// interactive builds a model wired to actions that report what was asked.
func interactive(t *testing.T) (*model, chan string, chan error) {
	t.Helper()

	asked := make(chan string, 4)
	finish := make(chan error, 4)
	built := newModel(NewSession("/tmp/demo"), Actions{
		Start: func(ctx context.Context, requirement string) error {
			asked <- requirement
			select {
			case err := <-finish:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Continue: func(ctx context.Context) error {
			asked <- "/continue"
			select {
			case err := <-finish:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	built.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	return built, asked, finish
}

func TestTypingARequestStartsTheOrchestration(t *testing.T) {
	t.Parallel()

	built, asked, finish := interactive(t)
	if built.mode != modeIdle || !built.prompt.Focused() {
		t.Fatal("a session opens waiting for a request")
	}

	typeInto(built, "adicionar rate limiting")
	_, cmd := built.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter must dispatch the request")
	}
	if built.mode != modeRunning || built.prompt.Focused() {
		t.Fatal("the prompt is blurred while the agents work")
	}
	if !strings.Contains(built.View(), "› adicionar rate limiting") {
		t.Fatalf("the request must appear in the transcript:\n%s", built.View())
	}
	if !strings.Contains(built.View(), "esc") {
		t.Fatal("a running session must say how to interrupt it")
	}

	// The command runs the use case in the background, like Bubble Tea does.
	message := make(chan tea.Msg, 1)
	go func() { message <- cmd() }()

	select {
	case got := <-asked:
		if got != "adicionar rate limiting" {
			t.Fatalf("the use case received %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the request never reached the use case")
	}

	finish <- nil
	select {
	case done := <-message:
		if _, ok := done.(runDoneMsg); !ok {
			t.Fatalf("unexpected message: %#v", done)
		}
	case <-time.After(time.Second):
		t.Fatal("the run never reported back")
	}
}

func TestEscInterruptsTheRunAndKeepsTheSession(t *testing.T) {
	t.Parallel()

	built, asked, _ := interactive(t)
	typeInto(built, "algo demorado")
	_, cmd := built.Update(tea.KeyMsg{Type: tea.KeyEnter})

	message := make(chan tea.Msg, 1)
	go func() { message <- cmd() }()
	<-asked

	built.Update(tea.KeyMsg{Type: tea.KeyEsc})

	select {
	case done := <-message:
		outcome, ok := done.(runDoneMsg)
		if !ok || !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("esc must cancel the run, got %#v", done)
		}
	case <-time.After(time.Second):
		t.Fatal("esc did not cancel the run")
	}

	// The session survives: the prompt comes back for the next request.
	built.Update(runDoneMsg{err: context.Canceled})
	if built.mode != modeIdle || !built.prompt.Focused() {
		t.Fatal("interrupting a run must not end the session")
	}
}

func TestPromptCommands(t *testing.T) {
	t.Parallel()

	built, asked, _ := interactive(t)

	typeInto(built, "/continue")
	_, cmd := built.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("/continue must resume the recorded workflow")
	}
	go cmd()
	select {
	case got := <-asked:
		if got != "/continue" {
			t.Fatalf("unexpected action: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("/continue never reached the use case")
	}

	built.Update(runDoneMsg{})
	typeInto(built, "/help")
	built.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(built.View(), "/continue  retoma") {
		t.Fatalf("/help must explain the session:\n%s", built.View())
	}

	typeInto(built, "/nope")
	built.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(built.View(), "comando desconhecido") {
		t.Fatal("an unknown command must be reported, not dispatched")
	}

	if _, cmd := built.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("an empty prompt dispatches nothing")
	}
}

func TestPromptHistory(t *testing.T) {
	t.Parallel()

	built, _, _ := interactive(t)
	for _, requirement := range []string{"primeiro pedido", "segundo pedido"} {
		typeInto(built, requirement)
		built.Update(tea.KeyMsg{Type: tea.KeyEnter})
		built.Update(runDoneMsg{})
	}

	built.Update(tea.KeyMsg{Type: tea.KeyUp})
	if built.prompt.Value() != "segundo pedido" {
		t.Fatalf("up recalls the last request, got %q", built.prompt.Value())
	}
	built.Update(tea.KeyMsg{Type: tea.KeyUp})
	if built.prompt.Value() != "primeiro pedido" {
		t.Fatalf("up walks back, got %q", built.prompt.Value())
	}
	built.Update(tea.KeyMsg{Type: tea.KeyDown})
	built.Update(tea.KeyMsg{Type: tea.KeyDown})
	if built.prompt.Value() != "" {
		t.Fatalf("down returns to an empty prompt, got %q", built.prompt.Value())
	}
}

func TestCtrlCQuitsOnlyWhenIdle(t *testing.T) {
	t.Parallel()

	built, asked, _ := interactive(t)
	typeInto(built, "algo")
	_, cmd := built.Update(tea.KeyMsg{Type: tea.KeyEnter})
	go cmd()
	<-asked

	if _, quit := built.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); quit != nil {
		t.Fatal("ctrl+c interrupts the run first, it does not quit")
	}

	built.Update(runDoneMsg{err: context.Canceled})
	if _, quit := built.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); quit == nil {
		t.Fatal("ctrl+c with nothing running must quit")
	}
}
