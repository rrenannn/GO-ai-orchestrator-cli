// Package task models the unit of work produced by the architect and
// consumed by the builder and the reviewer.
package task

import "strings"

// Status is the lifecycle state of a single task.
type Status string

const (
	// StatusPending has not been started yet.
	StatusPending Status = "pending"
	// StatusImplementing is being built right now.
	StatusImplementing Status = "implementing"
	// StatusReviewing waits for the reviewer verdict.
	StatusReviewing Status = "reviewing"
	// StatusApproved passed review and is done.
	StatusApproved Status = "approved"
	// StatusBlocked cannot progress without human input.
	StatusBlocked Status = "blocked"
)

// IsOpen reports whether the task still needs agent work.
func (s Status) IsOpen() bool {
	return s != StatusApproved && s != StatusBlocked
}

// Task is an atomic, independently reviewable piece of work.
type Task struct {
	ID                 string
	Objective          string
	Status             Status
	Files              []string
	Notes              string
	AcceptanceCriteria []string
	Validation         []string
}

// Summary renders a short, single-line description used in prompts.
func (t Task) Summary() string {
	objective := strings.TrimSpace(t.Objective)
	if objective == "" {
		return t.ID
	}
	return t.ID + " - " + objective
}

// Board is the aggregate holding every task of the current request.
type Board struct {
	Version       int
	CurrentTaskID string
	Tasks         []Task
}

// Find returns the task with the given ID.
func (b Board) Find(id string) (Task, bool) {
	for _, candidate := range b.Tasks {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Task{}, false
}

// Current returns the task the board points at.
func (b Board) Current() (Task, bool) {
	if b.CurrentTaskID == "" {
		return Task{}, false
	}
	return b.Find(b.CurrentTaskID)
}

// Open returns every task that still requires agent work.
func (b Board) Open() []Task {
	open := make([]Task, 0, len(b.Tasks))
	for _, candidate := range b.Tasks {
		if candidate.Status.IsOpen() {
			open = append(open, candidate)
		}
	}
	return open
}

// HasOpen reports whether any task still requires agent work.
func (b Board) HasOpen() bool {
	return len(b.Open()) > 0
}

// Counts summarizes the board by status, for status reporting.
func (b Board) Counts() map[Status]int {
	counts := make(map[Status]int, len(b.Tasks))
	for _, candidate := range b.Tasks {
		counts[candidate.Status]++
	}
	return counts
}
