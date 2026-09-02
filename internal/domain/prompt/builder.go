// Package prompt turns a workflow position into the instruction text
// dispatched to an agent. It is domain policy: it decides what each role
// is allowed to do and which artifacts it must produce.
package prompt

import (
	"fmt"
	"strings"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/workflow"
)

// Artifacts are the shared files both agents read and write.
const (
	RequestFile = ".agent/REQUEST.md"
	PlanFile    = ".agent/PLAN.md"
	TasksFile   = ".agent/TASKS.json"
	ReviewFile  = ".agent/REVIEW.md"
	StatusFile  = ".agent/STATUS.md"
)

// Builder renders prompts for every phase of the workflow.
type Builder struct{}

// NewBuilder returns the default prompt builder.
func NewBuilder() Builder { return Builder{} }

// Build renders the instruction for the current state. The task is optional
// and only used to give the agent the objective it is working on.
func (Builder) Build(state workflow.State, current task.Task) (string, error) {
	subject := state.TaskID
	if current.ID == state.TaskID && strings.TrimSpace(current.Objective) != "" {
		subject = current.Summary()
	}

	switch state.Phase {
	case workflow.PhasePlanning:
		return join(
			"You are the ARCHITECT of this repository.",
			"Read CLAUDE.md, "+RequestFile+" and inspect the existing architecture before deciding anything.",
			"Do not implement production code in this step.",
			"Write the technical approach, the risks and the task order in "+PlanFile+".",
			"Write small, ordered, independently reviewable tasks in "+TasksFile+"; each task needs id, status, objective, files, notes, acceptanceCriteria and validation commands.",
			"Select the first task as currentTaskId and set "+StatusFile+" to phase=implementing with task_id set to that id.",
			"If the request needs no code change, explain why in "+PlanFile+" and set "+StatusFile+" to phase=completed.",
		), nil

	case workflow.PhaseImplementing:
		return join(
			"You are the BUILDER of this repository.",
			"Read AGENTS.md, "+PlanFile+", "+TasksFile+" and "+ReviewFile+".",
			fmt.Sprintf("Implement ONLY the current task: %s.", subject),
			"Follow the conventions already present in the repository and avoid unrelated refactors.",
			"Before finishing: format the changed code, run the task validation commands, inspect the git diff and verify every acceptance criterion.",
			fmt.Sprintf("Then set %s to phase=reviewing with task_id=%s.", StatusFile, state.TaskID),
		), nil

	case workflow.PhaseReviewing:
		return join(
			"You are the REVIEWER of this repository.",
			"Read CLAUDE.md, "+PlanFile+", "+TasksFile+", "+ReviewFile+", the current git diff and the validation output.",
			fmt.Sprintf("Review ONLY the task: %s.", subject),
			"Check correctness, architecture, regressions, concurrency, error handling, security, performance and tests.",
			"Never approve code just because it compiles.",
			"Record the verdict in "+ReviewFile+" as APPROVED or CHANGES REQUESTED, with actionable findings.",
			fmt.Sprintf("If approved: mark task %s as approved in %s and set %s to phase=approved with the same task_id.", state.TaskID, TasksFile, StatusFile),
			fmt.Sprintf("If rejected: set %s to phase=fixing with the same task_id.", StatusFile),
		), nil

	case workflow.PhaseFixing:
		return join(
			"You are the BUILDER of this repository.",
			"Read AGENTS.md and "+ReviewFile+".",
			fmt.Sprintf("Fix ONLY the review findings reported for the task: %s.", subject),
			"Do not start other tasks and do not refactor unrelated code.",
			"Rerun the task validation commands and inspect the git diff.",
			fmt.Sprintf("Then set %s to phase=reviewing with task_id=%s.", StatusFile, state.TaskID),
		), nil

	case workflow.PhaseApproved:
		return join(
			"You are the ARCHITECT of this repository.",
			"Read CLAUDE.md, "+PlanFile+", "+TasksFile+" and "+ReviewFile+".",
			fmt.Sprintf("Confirm task %s is marked approved in %s.", state.TaskID, TasksFile),
			"Do not implement code in this step.",
			fmt.Sprintf("Select the next open task, update currentTaskId and set %s to phase=implementing with its task_id.", StatusFile),
			fmt.Sprintf("If no open task remains, set %s to phase=completed keeping task_id=%s.", StatusFile, state.TaskID),
		), nil

	case workflow.PhaseCompleted:
		return "", workflow.ErrTerminalPhase

	default:
		return "", fmt.Errorf("%w: %q", workflow.ErrUnknownPhase, state.Phase)
	}
}

func join(lines ...string) string {
	return strings.Join(lines, "\n")
}
