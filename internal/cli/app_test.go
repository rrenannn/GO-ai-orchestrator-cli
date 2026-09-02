package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/usecase"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/cli"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/prompt"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/clock"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/fsstate"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/gitrepo"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/process"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/runlog"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/scaffold"
)

// newApp wires the real adapters, with `echo` standing in for the agent CLIs.
func newApp(t *testing.T) (*cli.App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	store := fsstate.NewStore()
	cycle := usecase.NewCycle(
		store,
		process.NewRunner("echo", "echo"),
		process.NewValidator(),
		runlog.NewLogger(),
		clock.New(),
		prompt.NewBuilder(),
	)
	container := cli.Container{
		Initialize: usecase.NewInitialize(scaffold.NewInstaller()),
		Start:      usecase.NewStart(store, cycle),
		Cycle:      cycle,
		Status:     usecase.NewStatus(store),
		Inspect:    usecase.NewInspect(gitrepo.New("")),
	}
	// interactive=false keeps the tests on the plain transcript.
	return cli.New(container, stdout, stderr, "test", false), stdout, stderr
}

func TestInitThenStatusThenDryRun(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	app, stdout, stderr := newApp(t)
	ctx := context.Background()

	if code := app.Run(ctx, []string{"init", project}); code != cli.ExitOK {
		t.Fatalf("init failed (%d): %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(project, ".agent", "STATUS.md")); err != nil {
		t.Fatalf("init must create the workflow state: %v", err)
	}

	stdout.Reset()
	if code := app.Run(ctx, []string{"status", project}); code != cli.ExitOK {
		t.Fatalf("status failed (%d): %s", code, stderr)
	}
	if !strings.Contains(stdout.String(), "Fase:    planning") {
		t.Fatalf("unexpected status output:\n%s", stdout)
	}

	stdout.Reset()
	if code := app.Run(ctx, []string{"cycle", "--dry-run", project}); code != cli.ExitOK {
		t.Fatalf("dry run failed (%d): %s", code, stderr)
	}
	if !strings.Contains(stdout.String(), "architect (claude)") {
		t.Fatalf("a planning dry run must announce the architect:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(project, ".agent", "runs")); !os.IsNotExist(err) {
		t.Fatal("a dry run must not open a run log")
	}
}

func TestStatusOnAnUninitializedProjectHints(t *testing.T) {
	t.Parallel()

	app, _, stderr := newApp(t)
	if code := app.Run(context.Background(), []string{"status", t.TempDir()}); code != cli.ExitError {
		t.Fatalf("want an error exit, got %d", code)
	}
	if !strings.Contains(stderr.String(), "forge init") {
		t.Fatalf("the error must hint at init:\n%s", stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()

	app, _, stderr := newApp(t)
	ctx := context.Background()

	if code := app.Run(ctx, nil); code != cli.ExitUsage {
		t.Fatalf("want usage exit, got %d", code)
	}
	if code := app.Run(ctx, []string{"deploy"}); code != cli.ExitUsage {
		t.Fatalf("an unknown command must exit with usage, got %d", code)
	}
	if !strings.Contains(stderr.String(), "comando desconhecido: deploy") {
		t.Fatalf("unexpected stderr:\n%s", stderr)
	}
	if code := app.Run(ctx, []string{"start"}); code != cli.ExitUsage {
		t.Fatalf("start without a requirement must exit with usage, got %d", code)
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	app, stdout, _ := newApp(t)
	if code := app.Run(context.Background(), []string{"version"}); code != cli.ExitOK {
		t.Fatalf("want ok, got %d", code)
	}
	if !strings.Contains(stdout.String(), "forge test") {
		t.Fatalf("unexpected version output: %s", stdout)
	}
}

func TestInteractiveSessionNeedsATerminal(t *testing.T) {
	t.Parallel()

	app, _, stderr := newApp(t)
	ctx := context.Background()

	// newApp builds the CLI with interactive=false, like a pipe or a CI job.
	if code := app.Run(ctx, nil); code != cli.ExitUsage {
		t.Fatalf("want a usage exit, got %d", code)
	}
	if !strings.Contains(stderr.String(), "--plain") {
		t.Fatalf("the error must point at the scriptable form:\n%s", stderr)
	}

	stderr.Reset()
	if code := app.Run(ctx, []string{"run", t.TempDir()}); code != cli.ExitUsage {
		t.Fatalf("want a usage exit, got %d", code)
	}
	if !strings.Contains(stderr.String(), "precisa de um terminal") {
		t.Fatalf("unexpected stderr:\n%s", stderr)
	}
}

func TestValidationFlagReachesTheUseCase(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	app, stdout, stderr := newApp(t)
	ctx := context.Background()

	if code := app.Run(ctx, []string{"init", project}); code != cli.ExitOK {
		t.Fatalf("init failed (%d): %s", code, stderr)
	}

	stdout.Reset()
	if code := app.Run(ctx, []string{"cycle", "--no-validate", "--dry-run", project}); code != cli.ExitOK {
		t.Fatalf("--no-validate must be accepted (%d): %s", code, stderr)
	}
	if !strings.Contains(stdout.String(), "architect (claude)") {
		t.Fatalf("unexpected output:\n%s", stdout)
	}
}
