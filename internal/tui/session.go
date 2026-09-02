// Package tui renders an orchestration run as a live terminal interface where
// the operator types what they want built. It is a delivery adapter: it
// implements the Observer and Gate ports and knows nothing about how the
// workflow decides anything.
package tui

import (
	"context"
	"os"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/GO-ai-orchestrator-cli/internal/app/event"
)

// eventBuffer absorbs bursts of agent output without ever blocking a run.
const eventBuffer = 8192

// Actions are the use cases the interface can trigger. The interface decides
// when; the application decides what each one does.
type Actions struct {
	// Start records a new request and drives it from planning.
	Start func(ctx context.Context, requirement string) error
	// Continue resumes the workflow already recorded in the project.
	Continue func(ctx context.Context) error
}

// Session bridges the running use cases and the Bubble Tea program: it carries
// events one way, holds the pause switch, and hands out the context each run
// is cancelled with.
type Session struct {
	project string
	events  chan event.Event

	mu        sync.Mutex
	base      context.Context
	paused    bool
	resume    chan struct{}
	dropped   int
	cancelRun context.CancelFunc
	running   sync.WaitGroup
}

// settlePalette resolves the terminal colors once, before the program owns
// stdin, and caches the answer so nothing queries the terminal mid-render:
// a late reply to that query is read as if the operator had typed it.
//
// FORGE_BACKGROUND=dark|light skips the query altogether, for terminals
// that answer it badly.
func settlePalette() {
	lipgloss.SetColorProfile(lipgloss.ColorProfile())

	if declared, ok := os.LookupEnv("FORGE_BACKGROUND"); ok {
		lipgloss.SetHasDarkBackground(!strings.EqualFold(strings.TrimSpace(declared), "light"))
		return
	}
	lipgloss.SetHasDarkBackground(lipgloss.HasDarkBackground())
}

// NewSession creates an idle session for a project.
func NewSession(project string) *Session {
	return &Session{
		project: project,
		events:  make(chan event.Event, eventBuffer),
		resume:  make(chan struct{}),
		base:    context.Background(),
	}
}

// Publish implements port.Observer. It never blocks the orchestration loop:
// if the interface falls behind, transcript lines are dropped and counted.
func (s *Session) Publish(published event.Event) {
	select {
	case s.events <- published:
	default:
		s.mu.Lock()
		s.dropped++
		s.mu.Unlock()
	}
}

// Wait implements port.Gate: it holds the loop while the operator paused it.
func (s *Session) Wait(ctx context.Context) error {
	for {
		s.mu.Lock()
		paused, resume := s.paused, s.resume
		s.mu.Unlock()

		if !paused {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-resume:
		}
	}
}

// togglePause flips the pause switch and reports the new value.
func (s *Session) togglePause() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.paused = !s.paused
	if !s.paused {
		close(s.resume)
		s.resume = make(chan struct{})
	}
	return s.paused
}

// droppedLines reports how much transcript the interface could not keep up with.
func (s *Session) droppedLines() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

// dispatch starts one run in the background and reports its outcome back to
// the interface. Each run gets its own context, so interrupting a run leaves
// the session alive for the next request.
func (s *Session) dispatch(action func(context.Context) error) tea.Cmd {
	s.mu.Lock()
	ctx, cancel := context.WithCancel(s.base)
	s.cancelRun = cancel
	s.mu.Unlock()

	s.running.Add(1)
	return func() tea.Msg {
		defer s.running.Done()
		defer cancel()
		return runDoneMsg{err: action(ctx)}
	}
}

// interrupt cancels the run in flight, if any. The agent process dies with
// the context; the session stays up.
func (s *Session) interrupt() {
	s.mu.Lock()
	cancel := s.cancelRun
	s.cancelRun = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Run opens the interface and keeps it alive until the operator leaves.
// autostart, when set, is dispatched as soon as the interface is up, which is
// how `forge start "..."` and `forge cycle` enter the same session.
func (s *Session) Run(ctx context.Context, actions Actions, autostart func(context.Context) error) error {
	s.mu.Lock()
	s.base = ctx
	s.mu.Unlock()

	settlePalette()

	model := newModel(s, actions)
	model.autostart = autostart
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))

	closed := make(chan struct{})
	go func() {
		for {
			select {
			case published := <-s.events:
				program.Send(eventMsg{published})
			case <-closed:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	_, err := program.Run()

	// Leaving the interface stops whatever was running and waits for the
	// agent process to be reaped before the command returns.
	close(closed)
	s.interrupt()
	s.running.Wait()
	return err
}
