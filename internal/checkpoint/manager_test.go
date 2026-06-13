package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitRepo(t *testing.T) {
	// Non-git directory should return false
	tmpDir := t.TempDir()
	m := NewManager(tmpDir, DefaultConfig())
	if m.IsGitRepo() {
		t.Errorf("IsGitRepo() = true for non-git directory, want false")
	}

	// Create .git entry, should return true
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	if !m.IsGitRepo() {
		t.Errorf("IsGitRepo() = false after creating .git, want true")
	}
}

func TestIsGitRepo_File(t *testing.T) {
	// A .git file (submodule file) should also count
	tmpDir := t.TempDir()
	gitFile := filepath.Join(tmpDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: ../.git/modules/sub"), 0o644); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}
	m := NewManager(tmpDir, DefaultConfig())
	if !m.IsGitRepo() {
		t.Errorf("IsGitRepo() = false when .git is a file, want true")
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Errorf("DefaultConfig().Enabled = false, want true")
	}
	if !cfg.AutoStash {
		t.Errorf("DefaultConfig().AutoStash = false, want true")
	}
}

func TestNewManager(t *testing.T) {
	cfg := Config{Enabled: true, AutoStash: false}
	m := NewManager("/some/path", cfg)
	if m.projectRoot != "/some/path" {
		t.Errorf("projectRoot = %q, want %q", m.projectRoot, "/some/path")
	}
	if !m.enabled {
		t.Errorf("enabled = false, want true")
	}
	if m.autoStash {
		t.Errorf("autoStash = true, want false")
	}
}

func TestAutoCheckpoint_Disabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{Enabled: false, AutoStash: false}
	m := NewManager(tmpDir, cfg)

	// Should return nil even for file-modifying tools when disabled
	err := m.AutoCheckpoint("edit_file", "somefile.txt")
	if err != nil {
		t.Errorf("AutoCheckpoint() returned error when disabled: %v", err)
	}
}

func TestAutoCheckpoint_NonFileModifyingTool(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{Enabled: true, AutoStash: false}
	m := NewManager(tmpDir, cfg)

	// Non-file-modifying tool should be a no-op
	err := m.AutoCheckpoint("read_file", "somefile.txt")
	if err != nil {
		t.Errorf("AutoCheckpoint() returned error for non-file-modifying tool: %v", err)
	}
}

func TestAutoCheckpoint_FileModifyingTools(t *testing.T) {
	tools := []string{"edit_file", "write_file", "delete_file"}
	for _, tool := range tools {
		tmpDir := t.TempDir()
		cfg := Config{Enabled: true, AutoStash: false}
		m := NewManager(tmpDir, cfg)

		// Not a git repo, so all operations silently return nil
		err := m.AutoCheckpoint(tool, "test.txt")
		if err != nil {
			t.Errorf("AutoCheckpoint(%q) returned error: %v", tool, err)
		}
	}
}

func TestAutoCheckpoint_AutoStashTrue(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{Enabled: true, AutoStash: true}
	m := NewManager(tmpDir, cfg)

	// Not a git repo, silently nil — but the code path goes through CreateStash
	err := m.AutoCheckpoint("edit_file", "test.txt")
	if err != nil {
		t.Errorf("AutoCheckpoint with autoStash=true returned error: %v", err)
	}
}

func TestAutoCheckpoint_AutoStashFalse(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{Enabled: true, AutoStash: false}
	m := NewManager(tmpDir, cfg)

	// Not a git repo, silently nil — code path goes through CreateCheckpoint
	err := m.AutoCheckpoint("write_file", "test.txt")
	if err != nil {
		t.Errorf("AutoCheckpoint with autoStash=false returned error: %v", err)
	}
}

func TestCreateCheckpoint_NonGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir, DefaultConfig())

	err := m.CreateCheckpoint("test message")
	if err != nil {
		t.Errorf("CreateCheckpoint on non-git dir returned error: %v", err)
	}
}

func TestCreateStash_NonGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir, DefaultConfig())

	err := m.CreateStash("test message")
	if err != nil {
		t.Errorf("CreateStash on non-git dir returned error: %v", err)
	}
}

func TestRestoreCheckpoint_NonGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir, DefaultConfig())

	err := m.RestoreCheckpoint()
	if err != nil {
		t.Errorf("RestoreCheckpoint on non-git dir returned error: %v", err)
	}
}

func TestPopStash_NonGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir, DefaultConfig())

	err := m.PopStash()
	if err != nil {
		t.Errorf("PopStash on non-git dir returned error: %v", err)
	}
}
