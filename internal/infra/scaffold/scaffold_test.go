package scaffold_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/fsstate"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/scaffold"
)

var _ port.Scaffolder = (*scaffold.Installer)(nil)

func TestInstallWritesTheManagedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files, err := scaffold.NewInstaller().Install(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 8 {
		t.Fatalf("want 8 managed files, got %d", len(files))
	}
	for _, file := range files {
		if file.Action != port.FileInstalled {
			t.Fatalf("%s: want installed, got %s", file.Path, file.Action)
		}
		if _, err := os.Stat(filepath.Join(dir, file.Path)); err != nil {
			t.Fatalf("%s was not written: %v", file.Path, err)
		}
	}

	// The installed state must be loadable by the store: the scaffold and the
	// state machine share one contract.
	state, err := fsstate.NewStore().LoadState(dir)
	if err != nil {
		t.Fatalf("installed state is not loadable: %v", err)
	}
	if state.Phase.String() != "planning" {
		t.Fatalf("a fresh project starts in planning, got %s", state.Phase)
	}
}

func TestInstallPreservesExistingFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	custom := []byte("# my own instructions\n")
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), custom, 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := scaffold.NewInstaller().Install(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files[0].Path != "CLAUDE.md" || files[0].Action != port.FilePreserved {
		t.Fatalf("want CLAUDE.md preserved, got %+v", files[0])
	}
	raw, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(custom) {
		t.Fatal("an existing instruction file must not be overwritten")
	}
}

func TestInstallForceOverwrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := scaffold.NewInstaller().Install(dir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if files[0].Action != port.FileInstalled {
		t.Fatalf("force must overwrite, got %s", files[0].Action)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "stale" {
		t.Fatal("the file was not replaced")
	}
}
