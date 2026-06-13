package undo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	tmpDir := t.TempDir()
	mgr := NewManager(tmpDir)
	return mgr, tmpDir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create dir %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func TestBackup(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	src := filepath.Join(tmpDir, "example.txt")
	writeFile(t, src, "hello world")

	if err := mgr.Backup(src); err != nil {
		t.Fatalf("Backup() error: %v", err)
	}

	// Verify backup file exists in backupDir
	infos, err := os.ReadDir(mgr.backupDir)
	if err != nil {
		t.Fatalf("ReadDir backupDir: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 backup file, got %d", len(infos))
	}

	name := infos[0].Name()
	if !strings.HasSuffix(name, ".txt.bak") {
		t.Errorf("backup filename %q should end with .txt.bak", name)
	}
	if !strings.Contains(name, "example_") {
		t.Errorf("backup filename %q should contain 'example_'", name)
	}

	// Verify backup content
	data, err := os.ReadFile(filepath.Join(mgr.backupDir, name))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("backup content = %q, want %q", string(data), "hello world")
	}
}

func TestBackup_NonexistentFile(t *testing.T) {
	mgr, _ := setupTestManager(t)

	err := mgr.Backup("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestRestore(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	src := filepath.Join(tmpDir, "data.go")
	writeFile(t, src, "original content")

	if err := mgr.Backup(src); err != nil {
		t.Fatalf("Backup() error: %v", err)
	}

	// Modify the original file
	writeFile(t, src, "modified content")

	// Restore from backup
	restored, err := mgr.Restore(src)
	if err != nil {
		t.Fatalf("Restore() error: %v", err)
	}
	if !restored {
		t.Fatal("Restore() returned false, expected true")
	}

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "original content" {
		t.Errorf("restored content = %q, want %q", string(data), "original content")
	}
}

func TestRestore_NoBackup(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	src := filepath.Join(tmpDir, "nosuch.go")
	writeFile(t, src, "content")

	restored, err := mgr.Restore(src)
	if err != nil {
		t.Fatalf("Restore() error: %v", err)
	}
	if restored {
		t.Error("Restore() should return false when no backup exists")
	}
}

func TestRestore_MultipleBackups(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	src := filepath.Join(tmpDir, "file.txt")
	writeFile(t, src, "version1")
	if err := mgr.Backup(src); err != nil {
		t.Fatalf("Backup v1: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // ensure different timestamps

	writeFile(t, src, "version2")
	if err := mgr.Backup(src); err != nil {
		t.Fatalf("Backup v2: %v", err)
	}

	// Overwrite with something else
	writeFile(t, src, "destroyed")

	restored, err := mgr.Restore(src)
	if err != nil {
		t.Fatalf("Restore() error: %v", err)
	}
	if !restored {
		t.Fatal("Restore() returned false, expected true")
	}

	// Should restore the most recent backup (version2)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "version2" {
		t.Errorf("restored content = %q, want %q", string(data), "version2")
	}
}

func TestListBackups(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	src1 := filepath.Join(tmpDir, "a.txt")
	src2 := filepath.Join(tmpDir, "b.go")

	writeFile(t, src1, "aaa")
	writeFile(t, src2, "bbb")

	if err := mgr.Backup(src1); err != nil {
		t.Fatalf("Backup a.txt: %v", err)
	}
	if err := mgr.Backup(src2); err != nil {
		t.Fatalf("Backup b.go: %v", err)
	}

	entries := mgr.ListBackups()
	if len(entries) != 2 {
		t.Fatalf("ListBackups() returned %d entries, want 2", len(entries))
	}

	// Entries should be sorted newest first
	for i := 1; i < len(entries); i++ {
		if entries[i].Timestamp.After(entries[i-1].Timestamp) {
			t.Error("ListBackups() not sorted newest first")
		}
	}

	// Check that entries have reasonable fields
	for _, e := range entries {
		if e.OriginalPath == "" {
			t.Error("OriginalPath is empty")
		}
		if e.BackupPath == "" {
			t.Error("BackupPath is empty")
		}
		if e.Timestamp.IsZero() {
			t.Error("Timestamp is zero")
		}
	}
}

func TestListBackups_Empty(t *testing.T) {
	mgr, _ := setupTestManager(t)

	entries := mgr.ListBackups()
	if len(entries) != 0 {
		t.Errorf("ListBackups() on empty dir returned %d entries, want 0", len(entries))
	}
}

func TestCleanup_OldByName(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	// Manually create a backup with an old timestamp in the name
	oldName := "file_20200101_000000.txt.bak"
	oldPath := filepath.Join(mgr.backupDir, oldName)
	if err := os.MkdirAll(mgr.backupDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create a recent backup
	src := filepath.Join(tmpDir, "file.txt")
	writeFile(t, src, "new")
	if err := mgr.Backup(src); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	entries := mgr.ListBackups()
	for _, e := range entries {
		if filepath.Base(e.BackupPath) == oldName {
			t.Error("old backup should have been cleaned up")
		}
	}
}

func TestCleanup_KeepsRecent(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	src := filepath.Join(tmpDir, "recent.txt")
	writeFile(t, src, "recent content")
	if err := mgr.Backup(src); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	if err := mgr.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	entries := mgr.ListBackups()
	if len(entries) != 1 {
		t.Errorf("expected 1 backup after cleanup, got %d", len(entries))
	}
}

func TestBackup_MaxBackupsEnforced(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	mgr.maxBackups = 3

	src := filepath.Join(tmpDir, "limited.txt")
	for i := 0; i < 5; i++ {
		writeFile(t, src, fmt.Sprintf("content-%d", i))
		if err := mgr.Backup(src); err != nil {
			t.Fatalf("Backup %d: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond) // unique timestamps
	}

	entries := mgr.ListBackups()
	if len(entries) > 3 {
		t.Errorf("expected at most 3 backups, got %d", len(entries))
	}
}

func TestBackup_DifferentFiles(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)

	src1 := filepath.Join(tmpDir, "alpha.txt")
	src2 := filepath.Join(tmpDir, "beta.go")

	writeFile(t, src1, "alpha content")
	writeFile(t, src2, "beta content")

	if err := mgr.Backup(src1); err != nil {
		t.Fatalf("Backup alpha: %v", err)
	}
	if err := mgr.Backup(src2); err != nil {
		t.Fatalf("Backup beta: %v", err)
	}

	entries := mgr.ListBackups()
	if len(entries) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(entries))
	}

	// Restore alpha
	writeFile(t, src1, "overwritten")
	restored, err := mgr.Restore(src1)
	if err != nil {
		t.Fatalf("Restore alpha: %v", err)
	}
	if !restored {
		t.Fatal("Restore alpha returned false")
	}

	data, err := os.ReadFile(src1)
	if err != nil {
		t.Fatalf("read alpha: %v", err)
	}
	if string(data) != "alpha content" {
		t.Errorf("alpha content = %q, want %q", string(data), "alpha content")
	}
}
