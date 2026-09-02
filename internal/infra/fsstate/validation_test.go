package fsstate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/validation"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/infra/fsstate"
)

func readValidation(t *testing.T, dir string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, fsstate.AgentDir, fsstate.ValidationFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSaveValidationRecordsFailures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	report := validation.NewReport("T2", []validation.Result{
		{Command: "go build ./...", Duration: time.Second},
		{Command: "go test ./...", ExitCode: 1, Output: "FAIL github.com/x/y", Duration: 2 * time.Second},
	})

	if err := fsstate.NewStore().SaveValidation(dir, report); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := readValidation(t, dir)
	for _, fragment := range []string{"Task: T2", "CHANGES REQUESTED", "go test ./...", "exit 1", "FAIL github.com/x/y"} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("the artifact must contain %q:\n%s", fragment, content)
		}
	}
}

func TestSaveValidationRecordsSuccessAndEmptyReports(t *testing.T) {
	t.Parallel()

	passed := t.TempDir()
	report := validation.NewReport("T1", []validation.Result{{Command: "go test ./..."}})
	if err := fsstate.NewStore().SaveValidation(passed, report); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content := readValidation(t, passed); !strings.Contains(content, "Verdict: PASSED") {
		t.Fatalf("unexpected artifact:\n%s", content)
	}

	empty := t.TempDir()
	if err := fsstate.NewStore().SaveValidation(empty, validation.NewReport("T1", nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := readValidation(t, empty)
	if !strings.Contains(content, "Verdict: PASSED") || !strings.Contains(content, "no validation commands") {
		t.Fatalf("unexpected artifact:\n%s", content)
	}
}
