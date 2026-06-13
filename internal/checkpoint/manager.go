package checkpoint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Config holds configuration for the checkpoint Manager.
type Config struct {
	Enabled   bool
	AutoStash bool // default true
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:   true,
		AutoStash: true,
	}
}

// Manager manages automatic git checkpoints for a project.
type Manager struct {
	projectRoot string
	enabled     bool
	autoStash   bool
}

// NewManager creates a new checkpoint Manager for the given project root.
func NewManager(projectRoot string, cfg Config) *Manager {
	return &Manager{
		projectRoot: projectRoot,
		enabled:     cfg.Enabled,
		autoStash:   cfg.AutoStash,
	}
}

// IsGitRepo checks whether the projectRoot directory contains a .git entry.
func (m *Manager) IsGitRepo() bool {
	gitPath := filepath.Join(m.projectRoot, ".git")
	_, err := os.Stat(gitPath)
	return err == nil
}

// CreateCheckpoint stages all changes and commits them with a "codecast:" prefixed message.
// If the directory is not a git repo or the commit fails, it silently returns nil.
func (m *Manager) CreateCheckpoint(message string) error {
	if !m.IsGitRepo() {
		return nil
	}

	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = m.projectRoot
	if err := addCmd.Run(); err != nil {
		return nil
	}

	commitMsg := fmt.Sprintf("codecast: %s", message)
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = m.projectRoot
	if err := commitCmd.Run(); err != nil {
		return nil
	}

	return nil
}

// CreateStash pushes a git stash with a "codecast:" prefixed message.
// Silently returns nil on failure.
func (m *Manager) CreateStash(message string) error {
	if !m.IsGitRepo() {
		return nil
	}

	stashMsg := fmt.Sprintf("codecast: %s", message)
	stashCmd := exec.Command("git", "stash", "push", "-m", stashMsg)
	stashCmd.Dir = m.projectRoot
	if err := stashCmd.Run(); err != nil {
		return nil
	}

	return nil
}

// RestoreCheckpoint undoes the last checkpoint commit with a soft reset.
// Silently returns nil on failure.
func (m *Manager) RestoreCheckpoint() error {
	if !m.IsGitRepo() {
		return nil
	}

	resetCmd := exec.Command("git", "reset", "--soft", "HEAD~1")
	resetCmd.Dir = m.projectRoot
	if err := resetCmd.Run(); err != nil {
		return nil
	}

	return nil
}

// PopStash pops the latest stash entry.
// Silently returns nil on failure.
func (m *Manager) PopStash() error {
	if !m.IsGitRepo() {
		return nil
	}

	popCmd := exec.Command("git", "stash", "pop")
	popCmd.Dir = m.projectRoot
	if err := popCmd.Run(); err != nil {
		return nil
	}

	return nil
}

// fileModifyingTools is the set of tool names that trigger auto-checkpointing.
var fileModifyingTools = map[string]bool{
	"edit_file":   true,
	"write_file":  true,
	"delete_file": true,
}

// AutoCheckpoint creates a checkpoint or stash before file-modifying tool
// invocations when the manager is enabled. If autoStash is true it uses
// CreateStash; otherwise it uses CreateCheckpoint.
func (m *Manager) AutoCheckpoint(toolName, args string) error {
	if !m.enabled {
		return nil
	}
	if !fileModifyingTools[toolName] {
		return nil
	}

	message := fmt.Sprintf("%s %s", toolName, args)
	if m.autoStash {
		return m.CreateStash(message)
	}
	return m.CreateCheckpoint(message)
}

