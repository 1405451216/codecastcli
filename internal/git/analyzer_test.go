package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary git repository for testing.
// Returns the repo path and a cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "git-analyzer-test-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s", strings.Join(args, " "), string(out))
		}
	}

	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")

	// Create an initial commit so the repo is not empty
	testFile := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit("add", "hello.txt")
	runGit("commit", "-m", "initial commit")

	cleanup := func() { os.RemoveAll(dir) }
	return dir, cleanup
}

func TestNewAnalyzer(t *testing.T) {
	a := NewAnalyzer(".")
	if a == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
	if a.repoPath != "." {
		t.Errorf("repoPath = %q, want .", a.repoPath)
	}
}

func TestIsGitRepo_InsideRepo(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)
	if !a.IsGitRepo() {
		t.Error("IsGitRepo() = false inside a git repo, want true")
	}
}

func TestIsGitRepo_OutsideRepo(t *testing.T) {
	dir, err := os.MkdirTemp("", "non-git-dir-")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	a := NewAnalyzer(dir)
	if a.IsGitRepo() {
		t.Error("IsGitRepo() = true outside a git repo, want false")
	}
}

func TestRecentChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)
	output, err := a.RecentChanges(5)
	if err != nil {
		t.Fatalf("RecentChanges error: %v", err)
	}
	if !strings.Contains(output, "initial commit") {
		t.Errorf("RecentChanges output = %q, want to contain 'initial commit'", output)
	}
}

func TestCurrentBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)
	branch, err := a.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch error: %v", err)
	}
	// After git init the default branch may be "master" or "main"
	if branch == "" {
		t.Error("CurrentBranch returned empty string")
	}
}

func TestGitCommand(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)

	// Test a basic git command via the unexported git() method indirectly
	output, err := a.RecentChanges(1)
	if err != nil {
		t.Fatalf("git command failed: %v", err)
	}
	if output == "" {
		t.Error("expected non-empty output from git log")
	}
}

func TestStagedAndUnstagedDiff(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)

	// No changes yet
	staged, err := a.StagedDiff()
	if err != nil {
		t.Fatalf("StagedDiff error: %v", err)
	}
	unstaged, err := a.UnstagedDiff()
	if err != nil {
		t.Fatalf("UnstagedDiff error: %v", err)
	}
	if staged != "" {
		t.Errorf("StagedDiff = %q, want empty with no staged changes", staged)
	}
	if unstaged != "" {
		t.Errorf("UnstagedDiff = %q, want empty with no unstaged changes", unstaged)
	}

	// Modify file and check unstaged diff
	testFile := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("modified content\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	unstaged, err = a.UnstagedDiff()
	if err != nil {
		t.Fatalf("UnstagedDiff after modify error: %v", err)
	}
	if unstaged == "" {
		t.Error("UnstagedDiff empty after modifying file, want non-empty")
	}
}

func TestFileHistory(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)
	output, err := a.FileHistory("hello.txt", 5)
	if err != nil {
		t.Fatalf("FileHistory error: %v", err)
	}
	if !strings.Contains(output, "initial commit") {
		t.Errorf("FileHistory output = %q, want to contain 'initial commit'", output)
	}
}

func TestBlameFile(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)
	output, err := a.BlameFile("hello.txt")
	if err != nil {
		t.Fatalf("BlameFile error: %v", err)
	}
	if output == "" {
		t.Error("BlameFile returned empty output")
	}
}

func TestBlameContext(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	a := NewAnalyzer(dir)
	output, err := a.BlameContext("hello.txt", 1, 1)
	if err != nil {
		t.Fatalf("BlameContext error: %v", err)
	}
	if output == "" {
		t.Error("BlameContext returned empty output")
	}
}
