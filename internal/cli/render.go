package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/app/event"
)

// PlainRenderer prints a run as a plain, line-oriented transcript. It is the
// fallback when the output is not a terminal (a pipe, a CI job) and what
// --plain selects explicitly.
type PlainRenderer struct {
	out io.Writer
}

// NewPlainRenderer returns a renderer writing to out.
func NewPlainRenderer(out io.Writer) *PlainRenderer { return &PlainRenderer{out: out} }

// Publish implements port.Observer.
func (r *PlainRenderer) Publish(published event.Event) {
	switch typed := published.(type) {
	case event.RunStarted:
		if typed.Requirement != "" {
			r.printf("request: %s", typed.Requirement)
		}
		r.printf("project: %s", typed.ProjectDir)
		r.printf("log:     %s", typed.LogPath)

	case event.AgentStarted:
		r.printf("[%d/%d] %s: dispatching %s",
			typed.Step, typed.MaxSteps, typed.State.Phase, typed.Assignment)

	case event.AgentOutput:
		fmt.Fprintf(r.out, "   %s\n", strings.TrimRight(typed.Line, "\r\n"))

	case event.AgentFinished:
		r.printf("%s finished in %s", typed.Assignment, typed.Result.Duration.Round(time.Second))

	case event.ValidationStarted:
		r.printf("validando %s: %s", typed.TaskID, strings.Join(typed.Commands, ", "))

	case event.ValidationOutput:
		fmt.Fprintf(r.out, "   %s\n", strings.TrimRight(typed.Line, "\r\n"))

	case event.ValidationFinished:
		if typed.Report.Passed() {
			r.printf("validação: %s", typed.Report.Summary())
			return
		}
		r.printf("validação: %s", typed.Report.Summary())
		for _, failure := range typed.Report.Failures() {
			if failure.Output != "" {
				fmt.Fprintf(r.out, "   %s\n", failure.Output)
			}
		}

	case event.PhaseChanged:
		r.printf("%s -> %s", typed.From.Phase, typed.To.Phase)

	case event.Notice:
		if typed.Level == event.LevelWarn {
			r.printf("warning: %s", typed.Message)
			return
		}
		r.printf("%s", typed.Message)

	case event.RunFinished:
		r.printf("phase=%s steps=%d fixes=%d", typed.State.Phase, typed.Steps, typed.Fixes)
		if typed.LogPath != "" {
			r.printf("log: %s", typed.LogPath)
		}
	}
}

func (r *PlainRenderer) printf(format string, args ...any) {
	fmt.Fprintf(r.out, "-> "+format+"\n", args...)
}
