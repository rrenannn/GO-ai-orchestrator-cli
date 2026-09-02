package gitrepo_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/gitrepo"
)

var _ port.Workspace = (*gitrepo.Repository)(nil)

// repository creates a real git repository, because the point of this adapter
// is exactly how git behaves.
func repository(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, arguments := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "forge@example.com"},
		{"config", "user.name", "forge"},
		{"config", "commit.gpgsign", "false"},
	} {
		command := exec.Command("git", arguments...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
	return dir
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsRepository(t *testing.T) {
	t.Parallel()

	repo := gitrepo.New("")
	ctx := context.Background()

	inside, err := repo.IsRepository(ctx, repository(t))
	if err != nil || !inside {
		t.Fatalf("want inside a repository, got %v (err=%v)", inside, err)
	}

	outside, err := repo.IsRepository(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("being outside a repository is an answer, not an error: %v", err)
	}
	if outside {
		t.Fatal("a plain directory is not a repository")
	}
}

func TestDiffBeforeAndAfterTheFirstCommit(t *testing.T) {
	t.Parallel()

	dir := repository(t)
	repo := gitrepo.New("")
	ctx := context.Background()

	// Nothing tracked yet: git diff has nothing to say about an untracked file.
	write(t, dir, "main.go", "package main\n")
	if _, err := repo.Diff(ctx, dir); err != nil {
		t.Fatalf("a repository without HEAD must not fail: %v", err)
	}

	if _, err := repo.Commit(ctx, dir, "first"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	write(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	diff, err := repo.Diff(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(diff, "func main()") || !strings.Contains(diff, "diff --git") {
		t.Fatalf("unexpected diff:\n%s", diff)
	}
}

func TestDiffOutsideARepositoryIsEmpty(t *testing.T) {
	t.Parallel()

	diff, err := gitrepo.New("").Diff(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != "" {
		t.Fatalf("want an empty diff, got %q", diff)
	}
}

func TestHasChangesAndCommit(t *testing.T) {
	t.Parallel()

	dir := repository(t)
	repo := gitrepo.New("")
	ctx := context.Background()

	write(t, dir, "main.go", "package main\n")
	dirty, err := repo.HasChanges(ctx, dir)
	if err != nil || !dirty {
		t.Fatalf("a new file is a change: %v (err=%v)", dirty, err)
	}

	hash, err := repo.Commit(ctx, dir, "feat(T1): add main\n\nAprovado pelo revisor.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hash) < 6 {
		t.Fatalf("want a short hash, got %q", hash)
	}

	clean, err := repo.HasChanges(ctx, dir)
	if err != nil || clean {
		t.Fatalf("the tree must be clean after committing: %v (err=%v)", clean, err)
	}

	log := exec.Command("git", "log", "-1", "--pretty=%H%n%s")
	log.Dir = dir
	output, err := log.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(output), hash) {
		t.Fatalf("the returned hash must be the commit: %q vs %s", hash, output)
	}
	if !strings.Contains(string(output), "feat(T1): add main") {
		t.Fatalf("unexpected commit subject:\n%s", output)
	}
}

func TestCommitWithNothingToCommitFails(t *testing.T) {
	t.Parallel()

	dir := repository(t)
	repo := gitrepo.New("")

	if _, err := repo.Commit(context.Background(), dir, "empty"); err == nil {
		t.Fatal("git refuses an empty commit and the adapter must report it")
	}
}
