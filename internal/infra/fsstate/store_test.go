package fsstate_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/fsstate"
)

var _ port.StateStore = (*fsstate.Store)(nil)

func writeAgentFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, fsstate.AgentDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsInitialized(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := fsstate.NewStore()

	initialized, err := store.IsInitialized(dir)
	if err != nil || initialized {
		t.Fatalf("empty dir must not be initialized (err=%v)", err)
	}

	writeAgentFile(t, dir, fsstate.StatusFile, "phase=planning\ntask_id=\n")
	if initialized, err = store.IsInitialized(dir); err != nil || !initialized {
		t.Fatalf("want initialized, got %v (err=%v)", initialized, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := fsstate.NewStore()
	want := workflow.State{Phase: workflow.PhaseReviewing, TaskID: "T2"}

	if err := store.SaveState(dir, want); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := store.LoadState(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Fatalf("want %v, got %v", want, got)
	}

	raw, err := os.ReadFile(filepath.Join(dir, fsstate.AgentDir, fsstate.StatusFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "phase=reviewing") || !strings.Contains(string(raw), "task_id=T2") {
		t.Fatalf("unexpected status file:\n%s", raw)
	}
}

func TestLoadStateToleratesAgentFormatting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeAgentFile(t, dir, fsstate.StatusFile, "# comment\n\n  phase =  Implementing \ntask_id = T7 \nnoise\n")

	got, err := fsstate.NewStore().LoadState(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Phase != workflow.PhaseImplementing || got.TaskID != "T7" {
		t.Fatalf("unexpected state: %v", got)
	}
}

func TestLoadStateRejectsInconsistentFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeAgentFile(t, dir, fsstate.StatusFile, "phase=implementing\ntask_id=\n")
	if _, err := fsstate.NewStore().LoadState(dir); !errors.Is(err, workflow.ErrMissingTask) {
		t.Fatalf("want ErrMissingTask, got %v", err)
	}

	other := t.TempDir()
	writeAgentFile(t, other, fsstate.StatusFile, "phase=deploying\n")
	if _, err := fsstate.NewStore().LoadState(other); !errors.Is(err, workflow.ErrUnknownPhase) {
		t.Fatalf("want ErrUnknownPhase, got %v", err)
	}
}

func TestSaveStateRejectsInvalidState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := fsstate.NewStore().SaveState(dir, workflow.State{Phase: workflow.PhaseFixing})
	if !errors.Is(err, workflow.ErrMissingTask) {
		t.Fatalf("want ErrMissingTask, got %v", err)
	}
}

func TestLoadBoard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeAgentFile(t, dir, fsstate.TasksFile, `{
	  "version": 1,
	  "currentTaskId": "T1",
	  "tasks": [
	    {"id": "T1", "objective": "add loader", "status": "approved", "validation": ["go test ./..."]},
	    {"id": "T2", "objective": "add limiter", "status": ""}
	  ]
	}`)

	board, err := fsstate.NewStore().LoadBoard(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if board.CurrentTaskID != "T1" || len(board.Tasks) != 2 {
		t.Fatalf("unexpected board: %+v", board)
	}
	if board.Tasks[1].Status != task.StatusPending {
		t.Fatalf("a missing status defaults to pending, got %q", board.Tasks[1].Status)
	}
	if !board.HasOpen() {
		t.Fatal("T2 is still open")
	}
}

func TestLoadBoardAcceptsNotesAsList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeAgentFile(t, dir, fsstate.TasksFile, `{
	  "version": 1,
	  "currentTaskId": "T1",
	  "tasks": [{
	    "id": "T1",
	    "objective": "add loader",
	    "notes": ["create the loader", "cover error handling"]
	  }]
	}`)

	board, err := fsstate.NewStore().LoadBoard(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := board.Tasks[0].Notes, "create the loader\ncover error handling"; got != want {
		t.Fatalf("want notes %q, got %q", want, got)
	}
}

func TestLoadBoardBeforePlanning(t *testing.T) {
	t.Parallel()

	board, err := fsstate.NewStore().LoadBoard(t.TempDir())
	if err != nil {
		t.Fatalf("a missing board is not an error: %v", err)
	}
	if len(board.Tasks) != 0 {
		t.Fatalf("want an empty board, got %+v", board)
	}
}

func TestSaveRequest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := fsstate.NewStore().SaveRequest(dir, "add tenant rate limiting"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, fsstate.AgentDir, fsstate.RequestFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "add tenant rate limiting") {
		t.Fatalf("unexpected request file:\n%s", raw)
	}
}
