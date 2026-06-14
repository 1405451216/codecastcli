package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandFileReferences(t *testing.T) {
	// Create a temp directory with a file
	tmpDir := t.TempDir()
	testContent := "package main\n\nfunc main() {}"
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	fc := NewFileCompleter(tmpDir)

	input := "请解释 @main.go 的代码"
	result := fc.ExpandFileReferences(input)

	// Verify the @main.go reference was expanded
	if !containsPlain(result, "```go") {
		t.Errorf("expanded output should contain '```go' code block, got:\n%s", result)
	}
	if !containsPlain(result, "// File: main.go") {
		t.Errorf("expanded output should contain '// File: main.go', got:\n%s", result)
	}
	if !containsPlain(result, "package main") {
		t.Errorf("expanded output should contain file content 'package main', got:\n%s", result)
	}
	if !containsPlain(result, "```") {
		t.Errorf("expanded output should contain closing '```', got:\n%s", result)
	}
}

func TestExpandFileReferencesMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileCompleter(tmpDir)

	input := "请解释 @nonexistent.go 的代码"
	result := fc.ExpandFileReferences(input)

	// When file doesn't exist, the original @reference should be preserved
	if !containsPlain(result, "@nonexistent.go") {
		t.Errorf("missing file: original reference should be preserved, got:\n%s", result)
	}
}

func TestExpandFileReferencesNoRefs(t *testing.T) {
	tmpDir := t.TempDir()
	fc := NewFileCompleter(tmpDir)

	input := "这段代码没有文件引用"
	result := fc.ExpandFileReferences(input)

	if result != input {
		t.Errorf("input without @refs should be unchanged, got:\n%s", result)
	}
}

func TestDetectLanguageFromExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".py", "python"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".jsx", "javascript"},
		{".rs", "rust"},
		{".java", "java"},
		{".kt", "kotlin"},
		{".c", "c"},
		{".cpp", "cpp"},
		{".cs", "csharp"},
		{".rb", "ruby"},
		{".php", "php"},
		{".swift", "swift"},
		{".scala", "scala"},
		{".sql", "sql"},
		{".sh", "shell"},
		{".yaml", "yaml"},
		{".yml", "yaml"},
		{".json", "json"},
		{".xml", "xml"},
		{".html", "html"},
		{".css", "css"},
		{".md", "markdown"},
		{".toml", "toml"},
		{".lua", "lua"},
		{".zig", "zig"},
		{".ex", "elixir"},
		{".erl", "erlang"},
		{".hs", "haskell"},
		{".ml", "ocaml"},
		{".vim", "vim"},
		{".xyz", ""}, // unknown extension
		{".GO", "go"}, // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := detectLanguageFromExt(tt.ext)
			if got != tt.want {
				t.Errorf("detectLanguageFromExt(%q) = %q, want %q", tt.ext, got, tt.want)
			}
		})
	}
}

// containsPlain checks if s contains substr without relying on ANSI escape handling.
func containsPlain(s, substr string) bool {
	return len(s) >= len(substr) && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
