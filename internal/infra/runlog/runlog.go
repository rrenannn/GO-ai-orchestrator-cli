// Package runlog stores the transcript of each orchestration run.
package runlog

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

// DirName is the directory holding run transcripts inside a project.
const DirName = ".agent/runs"

// Logger creates one append-only transcript per run.
type Logger struct{}

// NewLogger returns a filesystem-backed run logger.
func NewLogger() *Logger { return &Logger{} }

// Open creates the transcript file for a run and returns it with its path.
func (l *Logger) Open(projectDir string, startedAt time.Time) (io.WriteCloser, string, error) {
	directory := filepath.Join(projectDir, DirName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, "", err
	}
	path := filepath.Join(directory, startedAt.Format("20060102T150405")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", err
	}
	return file, path, nil
}
