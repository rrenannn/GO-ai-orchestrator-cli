// Package validation models the objective evidence a task passed: the
// commands the plan declared, run by the orchestrator itself instead of
// being taken on the word of an agent.
package validation

import (
	"strconv"
	"strings"
	"time"
)

// Result is the outcome of one validation command.
type Result struct {
	Command  string
	ExitCode int
	Output   string
	Duration time.Duration
	Err      error
}

// Passed reports whether the command succeeded.
func (r Result) Passed() bool { return r.Err == nil && r.ExitCode == 0 }

// Report is the evidence gathered for one task.
type Report struct {
	TaskID  string
	Results []Result
}

// NewReport builds a report for a task.
func NewReport(taskID string, results []Result) Report {
	return Report{TaskID: taskID, Results: results}
}

// Empty reports whether the task declared no validation at all.
func (r Report) Empty() bool { return len(r.Results) == 0 }

// Passed reports whether every command succeeded. An empty report passes:
// a task without declared validation is not evidence of failure.
func (r Report) Passed() bool {
	return len(r.Failures()) == 0
}

// Failures returns only the commands that did not pass.
func (r Report) Failures() []Result {
	failures := make([]Result, 0, len(r.Results))
	for _, result := range r.Results {
		if !result.Passed() {
			failures = append(failures, result)
		}
	}
	return failures
}

// Summary renders a one-line verdict for logs and terminal output.
func (r Report) Summary() string {
	if r.Empty() {
		return "nenhum comando de validação declarado"
	}
	failures := r.Failures()
	if len(failures) == 0 {
		return plural(len(r.Results), "comando") + " de validação passou"
	}

	names := make([]string, 0, len(failures))
	for _, failure := range failures {
		names = append(names, failure.Command)
	}
	return plural(len(failures), "comando") + " falhou: " + strings.Join(names, ", ")
}

func plural(total int, noun string) string {
	if total == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(total) + " " + noun + "s"
}
