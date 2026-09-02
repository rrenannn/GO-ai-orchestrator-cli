// Command forge orchestrates an AI agent workflow over a target project:
// Claude architects and reviews, Codex implements.
//
// This file is the composition root: it is the only place where the concrete
// adapters of internal/infra are bound to the ports of internal/app.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/GO-ai-orchestrator-cli/internal/app/usecase"
	"github.com/GO-ai-orchestrator-cli/internal/cli"
	"github.com/GO-ai-orchestrator-cli/internal/domain/prompt"
	"github.com/GO-ai-orchestrator-cli/internal/infra/clock"
	"github.com/GO-ai-orchestrator-cli/internal/infra/fsstate"
	"github.com/GO-ai-orchestrator-cli/internal/infra/process"
	"github.com/GO-ai-orchestrator-cli/internal/infra/runlog"
	"github.com/GO-ai-orchestrator-cli/internal/infra/scaffold"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(run(ctx))
}

func run(ctx context.Context) int {
	store := fsstate.NewStore()
	runner := process.NewRunner(
		os.Getenv("FORGE_CLAUDE_CMD"),
		os.Getenv("FORGE_CODEX_CMD"),
	)
	cycle := usecase.NewCycle(store, runner, runlog.NewLogger(), clock.New(), prompt.NewBuilder())

	container := cli.Container{
		Initialize: usecase.NewInitialize(scaffold.NewInstaller()),
		Start:      usecase.NewStart(store, cycle),
		Cycle:      cycle,
		Status:     usecase.NewStatus(store),
	}

	return cli.New(container, os.Stdout, os.Stderr, version, isTerminal(os.Stdout)).Run(ctx, os.Args[1:])
}

// isTerminal reports whether the interface can take over the screen. A pipe,
// a redirect or a CI job gets the plain transcript instead.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
