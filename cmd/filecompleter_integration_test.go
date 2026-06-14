package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codecast/cli/internal/tui"
)

func TestFileCompleterIntegration(t *testing.T) {
	// Create a temp directory with some files
	tmpDir := t.TempDir()

	// Create test files
	files := []string{
		"main.go",
		"utils.go",
		"src/app.py",
		"src/helper.py",
		"README.md",
	}
	for _, f := range files {
		fullPath := filepath.Join(tmpDir, f)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte("content of "+f), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Also create a file in a skipped directory (.git) — should not appear
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("git config"), 0644); err != nil {
		t.Fatalf("failed to create .git/config: %v", err)
	}

	fc := tui.NewFileCompleter(tmpDir)

	// Test: Complete with ".go" prefix should find Go files
	results := fc.Complete(".go")
	if len(results) == 0 {
		t.Error("expected .go completion to return results, got none")
	}
	for _, r := range results {
		if !strings.HasSuffix(r, ".go") {
			t.Errorf("expected .go suffix, got %q", r)
		}
	}

	// Test: Complete with "main" prefix should find main.go
	results = fc.Complete("main")
	found := false
	for _, r := range results {
		if r == "main.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected main.go in results for prefix 'main', got %v", results)
	}

	// Test: Complete with "src" prefix should find files under src/
	// Note: use "src" (no slash) because FileCompleter.Complete uses
	// strings.Contains on platform-native paths (backslash on Windows).
	results = fc.Complete("src")
	if len(results) == 0 {
		t.Error("expected src completion to return results, got none")
	}
	for _, r := range results {
		// Results use forward slashes (filepath.ToSlash in FileCompleter)
		if !strings.HasPrefix(r, "src/") {
			t.Errorf("expected src/ prefix, got %q", r)
		}
	}

	// Test: .git files should be excluded
	results = fc.Complete("config")
	for _, r := range results {
		if strings.Contains(r, ".git") {
			t.Errorf("expected .git files to be excluded, got %q", r)
		}
	}

	// Test: empty prefix returns nil
	results = fc.Complete("")
	if results != nil {
		t.Errorf("expected nil for empty prefix, got %v", results)
	}
}

func TestExpandFileReferencesIntegration(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.go")
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	fc := tui.NewFileCompleter(tmpDir)

	// Test: @hello.go should expand to a code block
	input := "please review @hello.go"
	expanded := fc.ExpandFileReferences(input)

	if !strings.Contains(expanded, "```go") {
		t.Error("expected expanded output to contain ```go code fence")
	}
	if !strings.Contains(expanded, "// File: hello.go") {
		t.Error("expected expanded output to contain file path comment")
	}
	if !strings.Contains(expanded, "package main") {
		t.Error("expected expanded output to contain file content")
	}
	if strings.Contains(expanded, "@hello.go") {
		t.Error("expected @hello.go reference to be replaced, not kept")
	}
}

func TestExpandFileReferencesMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	fc := tui.NewFileCompleter(tmpDir)

	// Test: referencing a non-existent file should keep the original reference
	input := "look at @nonexistent.go"
	expanded := fc.ExpandFileReferences(input)

	if !strings.Contains(expanded, "@nonexistent.go") {
		t.Error("expected missing file reference to be preserved as-is")
	}
}

func TestExpandFileReferencesTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "big.go")

	// Create a file larger than maxFileContentLen (4000 bytes)
	largeContent := strings.Repeat("// line of code\n", 300) // ~4500 bytes
	if err := os.WriteFile(largeFile, []byte(largeContent), 0644); err != nil {
		t.Fatalf("failed to create large test file: %v", err)
	}

	fc := tui.NewFileCompleter(tmpDir)

	input := "@big.go"
	expanded := fc.ExpandFileReferences(input)

	if !strings.Contains(expanded, "truncated") {
		t.Error("expected large file to be truncated with notice")
	}
}

// simulateBackslashContinuation simulates the backslash continuation logic
// from runBufioREPL. It takes a slice of lines (as if read one-by-one from
// stdin) and joins them according to the backslash continuation rule.
func simulateBackslashContinuation(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	result := strings.TrimSpace(lines[0])
	for i := 1; i < len(lines); i++ {
		if strings.HasSuffix(result, "\\") {
			result = result[:len(result)-1] + "\n" + strings.TrimSpace(lines[i])
		} else {
			result += "\n" + strings.TrimSpace(lines[i])
		}
	}
	return result
}

func TestMultiLineBackslashContinuation(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{
			name:     "single line no backslash",
			lines:    []string{"hello world"},
			expected: "hello world",
		},
		{
			name:     "two lines with backslash continuation",
			lines:    []string{"line1\\", "line2"},
			expected: "line1\nline2",
		},
		{
			name:     "three lines with backslash continuation",
			lines:    []string{"line1\\", "line2\\", "line3"},
			expected: "line1\nline2\nline3",
		},
		{
			name:     "backslash not at end is preserved",
			lines:    []string{"a\\b"},
			expected: "a\\b",
		},
		{
			name:     "mixed: continuation then normal",
			lines:    []string{"first\\", "second", "third"},
			expected: "first\nsecond\nthird",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := simulateBackslashContinuation(tt.lines)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
