package fsstate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rrenannn/GO-ai-orchestrator-cli/internal/domain/task"
)

// boardDocument is the on-disk shape of .agent/TASKS.json. It is a transport
// concern, kept out of the domain model on purpose.
type boardDocument struct {
	Version       int            `json:"version"`
	CurrentTaskID *string        `json:"currentTaskId"`
	Tasks         []taskDocument `json:"tasks"`
}

type taskDocument struct {
	ID                 string   `json:"id"`
	Objective          string   `json:"objective"`
	Status             string   `json:"status"`
	Files              []string `json:"files"`
	Notes              notes    `json:"notes"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Validation         []string `json:"validation"`
}

// notes accepts the original string form as well as the list form agents
// naturally produce for implementation details.
type notes string

func (n *notes) UnmarshalJSON(raw []byte) error {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		*n = notes(single)
		return nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return fmt.Errorf("must be a string or an array of strings")
	}
	*n = notes(strings.Join(list, "\n"))
	return nil
}

// parseBoard decodes the task board written by the agents. A board that does
// not exist yet is an empty board, not a failure: planning creates it.
func parseBoard(path string) (task.Board, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return task.Board{Version: 1}, nil
	}
	if err != nil {
		return task.Board{}, err
	}

	var document boardDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return task.Board{}, fmt.Errorf("%s: %w", path, err)
	}

	board := task.Board{Version: document.Version, Tasks: make([]task.Task, 0, len(document.Tasks))}
	if document.CurrentTaskID != nil {
		board.CurrentTaskID = *document.CurrentTaskID
	}
	for _, item := range document.Tasks {
		status := task.Status(item.Status)
		if item.Status == "" {
			status = task.StatusPending
		}
		board.Tasks = append(board.Tasks, task.Task{
			ID:                 item.ID,
			Objective:          item.Objective,
			Status:             status,
			Files:              item.Files,
			Notes:              string(item.Notes),
			AcceptanceCriteria: item.AcceptanceCriteria,
			Validation:         item.Validation,
		})
	}
	return board, nil
}
