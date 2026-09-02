package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/GO-ai-orchestrator-cli/internal/domain/agent"
	"github.com/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/GO-ai-orchestrator-cli/internal/domain/workflow"
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

	built := newModel(NewSession())
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
		"maestro",
		"add rate limiting",
		"T1 add config loader",
		"T2 add rate limiter",
		"steps  2/12",
		"editing internal/http/handler.go",
		"RUNNING",
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

	_, done := screen(t,
		event.RunStarted{ProjectDir: "/tmp/demo", LogPath: "/tmp/log", MaxSteps: 12, MaxFixes: 2},
		event.RunFinished{State: workflow.State{Phase: workflow.PhaseCompleted, TaskID: "T3"}, Steps: 9, Fixes: 1, LogPath: "/tmp/log"},
	)
	if !strings.Contains(done, "DONE") || !strings.Contains(done, "9 steps") {
		t.Fatalf("a finished run must show its summary:\n%s", done)
	}

	_, failed := screen(t,
		event.RunStarted{ProjectDir: "/tmp/demo", MaxSteps: 12},
		event.RunFinished{State: workflow.State{Phase: workflow.PhaseFixing, TaskID: "T1"}, Err: context.DeadlineExceeded},
	)
	if !strings.Contains(failed, "FAILED") {
		t.Fatalf("a failed run must be unmistakable:\n%s", failed)
	}
}

func TestPauseKeyHoldsTheLoop(t *testing.T) {
	t.Parallel()

	built := newModel(NewSession())
	built.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	built.Update(eventMsg{event.AgentStarted{Assignment: architect, Step: 1, MaxSteps: 12}})

	built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !built.paused {
		t.Fatal("p must pause the run")
	}
	if !strings.Contains(built.View(), "PAUSED") {
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

func TestQuitAsksBeforeStoppingARun(t *testing.T) {
	t.Parallel()

	built := newModel(NewSession())
	built.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	_, cmd := built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("the first q must ask for confirmation, not quit")
	}
	if !strings.Contains(built.View(), "stop the run and quit?") {
		t.Fatalf("the confirmation must be visible:\n%s", built.View())
	}

	_, cmd = built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("the second q must quit")
	}

	// Any other key cancels the confirmation.
	built.confirmQuit = true
	built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if built.confirmQuit {
		t.Fatal("another key must cancel the confirmation")
	}
}

func TestFinishedRunQuitsWithoutConfirmation(t *testing.T) {
	t.Parallel()

	built := newModel(NewSession())
	built.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	built.Update(finishedMsg{})

	if _, cmd := built.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}); cmd == nil {
		t.Fatal("a finished run must quit on the first q")
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

	session := NewSession()
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
	if !strings.Contains(built.View(), "fixes  1/2") {
		t.Fatalf("the budget must be visible:\n%s", built.View())
	}

	built.Update(eventMsg{event.PhaseChanged{From: fixing, To: reviewing}})
	if built.fixes != 1 {
		t.Fatalf("leaving fixing must not spend another round, got %d", built.fixes)
	}
}
