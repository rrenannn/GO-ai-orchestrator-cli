package validation_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/validation"
)

func TestEmptyReportPasses(t *testing.T) {
	t.Parallel()

	// A task that declared no validation is not evidence of failure.
	report := validation.NewReport("T1", nil)
	if !report.Empty() || !report.Passed() {
		t.Fatalf("an empty report must pass: %+v", report)
	}
	if !strings.Contains(report.Summary(), "nenhum comando") {
		t.Fatalf("unexpected summary: %q", report.Summary())
	}
}

func TestReportPassesWhenEveryCommandSucceeds(t *testing.T) {
	t.Parallel()

	report := validation.NewReport("T1", []validation.Result{
		{Command: "go build ./...", Duration: time.Second},
		{Command: "go test ./...", Duration: 2 * time.Second},
	})
	if !report.Passed() || len(report.Failures()) != 0 {
		t.Fatalf("want a passing report, got %+v", report)
	}
	if !strings.Contains(report.Summary(), "2 comandos") {
		t.Fatalf("unexpected summary: %q", report.Summary())
	}
}

func TestReportFailsOnNonZeroExitAndOnUnrunnableCommands(t *testing.T) {
	t.Parallel()

	report := validation.NewReport("T1", []validation.Result{
		{Command: "go build ./..."},
		{Command: "go test ./...", ExitCode: 1},
		{Command: "make lint", Err: errors.New("executable not found")},
	})

	failures := report.Failures()
	if report.Passed() || len(failures) != 2 {
		t.Fatalf("want two failures, got %+v", failures)
	}
	if failures[0].Command != "go test ./..." || failures[1].Command != "make lint" {
		t.Fatalf("unexpected failures: %+v", failures)
	}
	summary := report.Summary()
	if !strings.Contains(summary, "go test ./...") || !strings.Contains(summary, "make lint") {
		t.Fatalf("the summary must name what failed: %q", summary)
	}
}
