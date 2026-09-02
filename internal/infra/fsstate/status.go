package fsstate

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

const statusHeader = `# Workflow State
# Managed by the orchestrator and by the agents.
# Supported phases: planning, implementing, reviewing, fixing, approved, completed
`

// parseStatus reads the key=value pairs of STATUS.md into a domain state.
func parseStatus(path string) (workflow.State, error) {
	file, err := os.Open(path)
	if err != nil {
		return workflow.State{}, err
	}
	defer file.Close()

	fields := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return workflow.State{}, fmt.Errorf("read %s: %w", path, err)
	}

	phase, err := workflow.ParsePhase(fields["phase"])
	if err != nil {
		return workflow.State{}, fmt.Errorf("%s: %w", path, err)
	}
	state := workflow.State{Phase: phase, TaskID: fields["task_id"]}
	if err := state.Validate(); err != nil {
		return workflow.State{}, fmt.Errorf("%s: %w", path, err)
	}
	return state, nil
}

// renderStatus serializes a state back to the shared file format.
func renderStatus(state workflow.State) []byte {
	var builder strings.Builder
	builder.WriteString(statusHeader)
	fmt.Fprintf(&builder, "phase=%s\n", state.Phase)
	fmt.Fprintf(&builder, "task_id=%s\n", state.TaskID)
	return []byte(builder.String())
}
