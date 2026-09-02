// Package scaffold installs the agent instruction files and the shared
// orchestration artifacts into a target project. Templates are embedded so the
// binary works from anywhere, with no companion directory.
package scaffold

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GO-ai-orchestrator-cli/internal/app/port"
)

//go:embed assets
var assets embed.FS

// managed maps an embedded template to its destination inside a project.
// The embedded tree avoids dot-directories; the mapping restores them.
var managed = []struct {
	source      string
	destination string
}{
	{"assets/CLAUDE.md", "CLAUDE.md"},
	{"assets/AGENTS.md", "AGENTS.md"},
	{"assets/agent/REQUEST.md", ".agent/REQUEST.md"},
	{"assets/agent/PLAN.md", ".agent/PLAN.md"},
	{"assets/agent/TASKS.json", ".agent/TASKS.json"},
	{"assets/agent/REVIEW.md", ".agent/REVIEW.md"},
	{"assets/agent/STATUS.md", ".agent/STATUS.md"},
}

// Installer writes the managed files into a project.
type Installer struct{}

// NewInstaller returns the embedded-template installer.
func NewInstaller() *Installer { return &Installer{} }

// Install copies every managed file, preserving existing ones unless forced.
func (i *Installer) Install(projectDir string, force bool) ([]port.InstalledFile, error) {
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return nil, err
	}

	results := make([]port.InstalledFile, 0, len(managed))
	for _, file := range managed {
		target := filepath.Join(projectDir, file.destination)

		if !force {
			if _, err := os.Stat(target); err == nil {
				results = append(results, port.InstalledFile{Path: file.destination, Action: port.FilePreserved})
				continue
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}

		content, err := assets.ReadFile(file.source)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", file.source, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", target, err)
		}
		results = append(results, port.InstalledFile{Path: file.destination, Action: port.FileInstalled})
	}
	return results, nil
}
