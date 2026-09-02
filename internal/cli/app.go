// Package cli is the delivery layer: it parses arguments, calls a single use
// case and renders the result. It holds no orchestration rule of its own.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/port"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/usecase"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/tui"
)

// Exit codes returned to the shell.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// Container carries the use cases the CLI can invoke.
type Container struct {
	Initialize *usecase.Initialize
	Start      *usecase.Start
	Cycle      *usecase.Cycle
	Status     *usecase.Status
	Inspect    *usecase.Inspect
}

// App dispatches a command line to a use case.
type App struct {
	container   Container
	stdout      io.Writer
	stderr      io.Writer
	version     string
	interactive bool
}

// New wires the delivery layer. When interactive is false - a pipe, a CI job,
// a test - every run falls back to the plain transcript.
func New(container Container, stdout, stderr io.Writer, version string, interactive bool) *App {
	return &App{container: container, stdout: stdout, stderr: stderr, version: version, interactive: interactive}
}

// Run executes one command and returns the process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	// No command opens the interactive session, the way forge is meant to
	// be used: you type what you want built and it orchestrates.
	if len(args) == 0 {
		return a.report(a.runInteractive(ctx, nil))
	}

	command, rest := args[0], args[1:]

	var err error
	switch command {
	case "run":
		err = a.runInteractive(ctx, rest)
	case "init":
		err = a.runInit(rest)
	case "start":
		err = a.runStart(ctx, rest)
	case "cycle":
		err = a.runCycle(ctx, rest)
	case "status":
		err = a.runStatus(rest)
	case "version", "--version", "-v":
		fmt.Fprintf(a.stdout, "forge %s\n", a.version)
	case "help", "--help", "-h":
		fmt.Fprint(a.stdout, usageText)
	default:
		fmt.Fprintf(a.stderr, "comando desconhecido: %s\n\n", command)
		fmt.Fprint(a.stderr, usageText)
		return ExitUsage
	}

	return a.report(err)
}

func (a *App) report(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(a.stdout, usageText)
		return ExitUsage
	case errors.Is(err, errUsage):
		fmt.Fprintf(a.stderr, "%v\n\n", err)
		fmt.Fprint(a.stderr, usageText)
		return ExitUsage
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(a.stderr, "interrompido")
		return ExitError
	default:
		fmt.Fprintf(a.stderr, "erro: %v\n", err)
		if errors.Is(err, usecase.ErrNotInitialized) {
			fmt.Fprintln(a.stderr, "dica: rode `forge init <projeto>` primeiro")
		}
		return ExitError
	}
}

var errUsage = errors.New("argumentos inválidos")

func (a *App) runInit(args []string) error {
	flags := a.newFlagSet("init")
	force := flags.Bool("force", false, "sobrescreve os arquivos gerenciados pelo forge")
	if err := flags.Parse(args); err != nil {
		return err
	}

	projectDir, err := projectArg(flags.Args(), 0)
	if err != nil {
		return err
	}

	output, err := a.container.Initialize.Execute(usecase.InitializeInput{ProjectDir: projectDir, Force: *force})
	if err != nil {
		return err
	}
	for _, file := range output.Files {
		fmt.Fprintf(a.stdout, "%-10s %s\n", file.Action, file.Path)
	}
	fmt.Fprintf(a.stdout, "\n%d arquivos prontos em %s\n", len(output.Files), projectDir)
	fmt.Fprintf(a.stdout, "próximo passo: forge start %s \"<requisito>\"\n", projectDir)
	return nil
}

func (a *App) runStart(ctx context.Context, args []string) error {
	flags := a.newFlagSet("start")
	run := bindRunFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	positional := flags.Args()
	var projectArgument, requirement string
	switch len(positional) {
	case 1:
		projectArgument, requirement = ".", positional[0]
	case 2:
		projectArgument, requirement = positional[0], positional[1]
	default:
		return fmt.Errorf("%w: start precisa de um requisito", errUsage)
	}

	projectDir, err := absolutePath(projectArgument)
	if err != nil {
		return err
	}
	run.input.ProjectDir = projectDir

	return a.execute(ctx, run, func(actions tui.Actions) func(context.Context) error {
		return func(ctx context.Context) error { return actions.Start(ctx, requirement) }
	})
}

func (a *App) runCycle(ctx context.Context, args []string) error {
	flags := a.newFlagSet("cycle")
	run := bindRunFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	projectDir, err := projectArg(flags.Args(), 0)
	if err != nil {
		return err
	}
	run.input.ProjectDir = projectDir

	return a.execute(ctx, run, func(actions tui.Actions) func(context.Context) error {
		return actions.Continue
	})
}

// execute runs a use case under the presentation the operator asked for.
func (a *App) execute(ctx context.Context, run *runFlags, autostart func(tui.Actions) func(context.Context) error) error {
	// The plain transcript is one-shot and closes with the outcome of the run.
	if run.plain || !a.interactive || run.input.DryRun {
		run.input.Observer = NewPlainRenderer(a.stdout)
		return autostart(a.actions(run))(ctx)
	}
	return a.session(ctx, run, autostart)
}

// session opens the interactive interface, optionally starting one run.
func (a *App) session(ctx context.Context, run *runFlags, autostart func(tui.Actions) func(context.Context) error) error {
	live := tui.NewSession(run.input.ProjectDir)
	run.input.Observer = live
	run.input.Gate = live

	actions := a.actions(run)
	var first func(context.Context) error
	if autostart != nil {
		first = autostart(actions)
	}
	return live.Run(ctx, actions, first)
}

// actions binds the interface vocabulary to the use cases.
func (a *App) actions(run *runFlags) tui.Actions {
	return tui.Actions{
		Start: func(ctx context.Context, requirement string) error {
			_, err := a.container.Start.Execute(ctx, usecase.StartInput{Cycle: run.input, Requirement: requirement})
			return err
		},
		Continue: func(ctx context.Context) error {
			_, err := a.container.Cycle.Execute(ctx, run.input)
			return err
		},
		Diff: func(ctx context.Context) (string, error) {
			output, err := a.container.Inspect.Execute(ctx, usecase.InspectInput{ProjectDir: run.input.ProjectDir})
			return output.Diff, err
		},
	}
}

// runInteractive opens a session with an empty prompt, preparing the project
// first so the operator never has to know about the .agent files.
func (a *App) runInteractive(ctx context.Context, args []string) error {
	flags := a.newFlagSet("run")
	run := bindRunFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}

	projectDir, err := projectArg(flags.Args(), 0)
	if err != nil {
		return err
	}
	run.input.ProjectDir = projectDir

	if !a.interactive {
		return fmt.Errorf("%w: o modo interativo precisa de um terminal; use `forge start %s \"<requisito>\" --plain`", errUsage, projectDir)
	}

	live := tui.NewSession(run.input.ProjectDir)
	run.input.Observer = live
	run.input.Gate = live

	if err := a.prepare(projectDir, live); err != nil {
		return err
	}
	return live.Run(ctx, a.actions(run), nil)
}

// prepare installs whatever the project is missing and says what it did.
func (a *App) prepare(projectDir string, live *tui.Session) error {
	output, err := a.container.Initialize.Execute(usecase.InitializeInput{ProjectDir: projectDir})
	if err != nil {
		return err
	}

	installed := make([]string, 0, len(output.Files))
	for _, file := range output.Files {
		if file.Action == port.FileInstalled {
			installed = append(installed, file.Path)
		}
	}
	live.Publish(event.Notice{Level: event.LevelInfo, Message: "projeto: " + projectDir})
	if len(installed) > 0 {
		live.Publish(event.Notice{
			Level:   event.LevelInfo,
			Message: "arquivos criados: " + strings.Join(installed, ", "),
		})
	}
	return nil
}

func (a *App) runStatus(args []string) error {
	flags := a.newFlagSet("status")
	if err := flags.Parse(args); err != nil {
		return err
	}

	projectDir, err := projectArg(flags.Args(), 0)
	if err != nil {
		return err
	}

	output, err := a.container.Status.Execute(usecase.StatusInput{ProjectDir: projectDir})
	if err != nil {
		return err
	}

	fmt.Fprintf(a.stdout, "Projeto: %s\n", projectDir)
	fmt.Fprintf(a.stdout, "Fase:    %s\n", output.State.Phase)
	fmt.Fprintf(a.stdout, "Tarefa:  %s\n", orNone(output.State.TaskID))
	if output.IsFinished {
		fmt.Fprintln(a.stdout, "Próximo: nada, o pedido está concluído")
	} else {
		fmt.Fprintf(a.stdout, "Próximo: %s -- %s\n", output.Next, describe(output.State.Phase))
	}

	if len(output.Board.Tasks) > 0 {
		fmt.Fprintf(a.stdout, "\nTarefas (%s):\n", summarizeCounts(output.Board.Counts()))
		for _, item := range output.Board.Tasks {
			marker := " "
			if item.ID == output.State.TaskID {
				marker = "*"
			}
			fmt.Fprintf(a.stdout, " %s %-10s %-13s %s\n", marker, item.ID, item.Status, item.Objective)
		}
	}
	return nil
}

func (a *App) newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	flags.Usage = func() { fmt.Fprint(a.stderr, usageText) }
	return flags
}

// runFlags separates what the use case needs from how it is presented.
type runFlags struct {
	input usecase.CycleInput
	plain bool
}

func bindRunFlags(flags *flag.FlagSet) *runFlags {
	run := &runFlags{}
	flags.BoolVar(&run.plain, "plain", false, "transcrição em texto, sem interface interativa")
	flags.BoolVar(&run.input.DryRun, "dry-run", false, "mostra o agente que rodaria, sem despachar")
	flags.IntVar(&run.input.MaxFixes, "max-fixes", usecase.DefaultMaxFixes, "rodadas de correção por tarefa")
	flags.IntVar(&run.input.MaxSteps, "max-steps", usecase.DefaultMaxSteps, "despachos de agente por execução")
	flags.DurationVar(&run.input.AgentTimeout, "timeout", 0, "timeout por agente, zero desliga")
	flags.BoolVar(&run.input.SkipValidation, "no-validate", false, "não roda os comandos de validação das tarefas")
	flags.BoolVar(&run.input.Commit, "commit", false, "commita cada tarefa aprovada pelo revisor")
	return run
}

func projectArg(positional []string, index int) (string, error) {
	if len(positional) > index+1 {
		return "", fmt.Errorf("%w: argumento inesperado %q", errUsage, positional[index+1])
	}
	if len(positional) <= index {
		return absolutePath(".")
	}
	return absolutePath(positional[index])
}

func absolutePath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolver %q: %w", value, err)
	}
	return absolute, nil
}

func describe(phase workflow.Phase) string {
	switch phase {
	case workflow.PhasePlanning:
		return "escrever PLAN.md e TASKS.json"
	case workflow.PhaseImplementing:
		return "implementar a tarefa atual e validá-la"
	case workflow.PhaseReviewing:
		return "revisar a implementação e registrar o veredito"
	case workflow.PhaseFixing:
		return "aplicar os achados da revisão"
	case workflow.PhaseApproved:
		return "selecionar a próxima tarefa aberta"
	default:
		return "nada"
	}
}

func summarizeCounts(counts map[task.Status]int) string {
	parts := make([]string, 0, len(counts))
	for status, total := range counts {
		parts = append(parts, fmt.Sprintf("%d %s", total, status))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func orNone(value string) string {
	if value == "" {
		return "nenhuma"
	}
	return value
}

var _ port.Observer = (*PlainRenderer)(nil)
