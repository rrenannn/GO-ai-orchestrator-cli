package fsstate

import (
	"fmt"
	"strings"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/validation"
)

// renderValidation serializes the evidence gathered for a task. Both agents
// read this file: the reviewer to judge, the builder to fix.
func renderValidation(report validation.Report) []byte {
	var builder strings.Builder

	builder.WriteString("# Validation\n\n")
	builder.WriteString("Written by forge, not by an agent: these commands were executed by the\n")
	builder.WriteString("orchestrator itself.\n\n")
	fmt.Fprintf(&builder, "Task: %s\n", orNone(report.TaskID))

	verdict := "CHANGES REQUESTED"
	if report.Passed() {
		verdict = "PASSED"
	}
	fmt.Fprintf(&builder, "Verdict: %s\n\n", verdict)

	if report.Empty() {
		builder.WriteString("The task declared no validation commands.\n")
		return []byte(builder.String())
	}

	for _, result := range report.Results {
		status := "ok"
		switch {
		case result.Err != nil:
			status = "could not run: " + result.Err.Error()
		case result.ExitCode != 0:
			status = fmt.Sprintf("exit %d", result.ExitCode)
		}

		fmt.Fprintf(&builder, "## `%s`\n\n", result.Command)
		fmt.Fprintf(&builder, "- status: %s\n", status)
		fmt.Fprintf(&builder, "- duration: %s\n\n", result.Duration.Round(time.Millisecond))

		if result.Output != "" {
			builder.WriteString("```\n")
			builder.WriteString(result.Output)
			builder.WriteString("\n```\n\n")
		}
	}
	return []byte(builder.String())
}

func orNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
