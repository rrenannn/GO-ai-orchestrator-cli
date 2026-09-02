package task_test

import (
	"testing"

	"github.com/GO-ai-orchestrator-cli/internal/domain/task"
)

func board() task.Board {
	return task.Board{
		Version:       1,
		CurrentTaskID: "T2",
		Tasks: []task.Task{
			{ID: "T1", Objective: "add config loader", Status: task.StatusApproved},
			{ID: "T2", Objective: "add rate limiter", Status: task.StatusImplementing},
			{ID: "T3", Objective: "document the API", Status: task.StatusPending},
			{ID: "T4", Objective: "migrate storage", Status: task.StatusBlocked},
		},
	}
}

func TestBoardOpenIgnoresApprovedAndBlocked(t *testing.T) {
	t.Parallel()

	open := board().Open()
	if len(open) != 2 {
		t.Fatalf("want 2 open tasks, got %d", len(open))
	}
	if open[0].ID != "T2" || open[1].ID != "T3" {
		t.Fatalf("unexpected open tasks: %v", open)
	}
}

func TestBoardCurrent(t *testing.T) {
	t.Parallel()

	current, found := board().Current()
	if !found || current.ID != "T2" {
		t.Fatalf("want T2, got %v (found=%v)", current.ID, found)
	}

	empty := task.Board{}
	if _, found := empty.Current(); found {
		t.Fatal("an empty board has no current task")
	}
}

func TestBoardHasOpen(t *testing.T) {
	t.Parallel()

	done := task.Board{Tasks: []task.Task{{ID: "T1", Status: task.StatusApproved}}}
	if done.HasOpen() {
		t.Fatal("a fully approved board has no open task")
	}
	if !board().HasOpen() {
		t.Fatal("board with pending tasks must report open work")
	}
}

func TestTaskSummary(t *testing.T) {
	t.Parallel()

	with := task.Task{ID: "T1", Objective: "add config loader"}
	if got := with.Summary(); got != "T1 - add config loader" {
		t.Fatalf("unexpected summary: %q", got)
	}
	without := task.Task{ID: "T1", Objective: "   "}
	if got := without.Summary(); got != "T1" {
		t.Fatalf("unexpected summary: %q", got)
	}
}
