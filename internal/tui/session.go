// Package tui renders an orchestration run as a live terminal interface.
// It is a delivery adapter: it implements the Observer and Gate ports and
// knows nothing about how the workflow decides anything.
package tui

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GO-ai-orchestrator-cli/internal/app/event"
)

// eventBuffer absorbs bursts of agent output without ever blocking the run.
const eventBuffer = 8192

// Session bridges a running use case and the Bubble Tea program: it receives
// events from the orchestration goroutine and holds the pause switch.
type Session struct {
	events chan event.Event

	mu      sync.Mutex
	paused  bool
	resume  chan struct{}
	dropped int
	workErr error
}

// NewSession creates an idle session.
func NewSession() *Session {
	return &Session{
		events: make(chan event.Event, eventBuffer),
		resume: make(chan struct{}),
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

// Run paints the interface while work runs, and keeps it open afterwards so
// the operator can read the outcome. It returns the error of the work.
func (s *Session) Run(ctx context.Context, work func(context.Context) error) error {
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	program := tea.NewProgram(newModel(s), tea.WithAltScreen(), tea.WithContext(ctx))

	worked := make(chan struct{})
	go func() {
		defer close(worked)
		err := work(workCtx)

		s.mu.Lock()
		s.workErr = err
		s.mu.Unlock()
		close(s.events)
	}()

	// Forwarding through one channel keeps the last events of a run ahead of
	// the message that announces its end.
	go func() {
		for published := range s.events {
			program.Send(eventMsg{published})
		}
		s.mu.Lock()
		err := s.workErr
		s.mu.Unlock()
		program.Send(finishedMsg{err: err})
	}()

	_, runErr := program.Run()

	// Quitting the interface stops the run: the agent process is killed with
	// the context, and we wait for it to be reaped before returning.
	cancel()
	<-worked

	s.mu.Lock()
	workErr := s.workErr
	s.mu.Unlock()

	if workErr != nil {
		return workErr
	}
	return runErr
}
