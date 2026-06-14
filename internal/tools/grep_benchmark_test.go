package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// create10KGoFiles creates a temp directory with 10,000 small .go files,
// each containing "func Test..." somewhere. Returns the temp dir path.
func create10KGoFiles(b *testing.B) string {
	b.Helper()
	tmpDir := b.TempDir()
	content := []byte("package fake\n\nfunc TestSomething(t *testing.T) {\n\tt.Log(\"hello\")\n}\n")
	for i := 0; i < 10000; i++ {
		dir := filepath.Join(tmpDir, fmt.Sprintf("pkg_%04d", i/100))
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatalf("MkdirAll: %v", err)
		}
		name := fmt.Sprintf("file_%04d_test.go", i)
		if err := os.WriteFile(filepath.Join(dir, name), content, 0644); err != nil {
			b.Fatalf("WriteFile: %v", err)
		}
	}
	return tmpDir
}

// BenchmarkGrep10KFiles benchmarks the grep tool searching "func Test" across 10,000 .go files.
// Asserts the benchmark shows < 2s for a single operation.
func BenchmarkGrep10KFiles(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping 10K file benchmark in short mode")
	}
	tmpDir := create10KGoFiles(b)

	tool := NewGrepSearchToolWithRG("") // force native path

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := tool.Execute(context.Background(), mustJSON(b, grepSearchParams{
			Pattern:    "func Test",
			Path:       tmpDir,
			MaxResults: 50,
		}))
		if err != nil {
			b.Fatalf("Execute: %v", err)
		}
		if result == nil || result.IsError {
			b.Fatalf("unexpected error result: %v", result)
		}
	}

	// Assert < 2s per operation
	if b.Elapsed()/time.Duration(b.N) > 2*time.Second {
		b.Fatalf("average per-operation time %v exceeds 2s threshold", b.Elapsed()/time.Duration(b.N))
	}
}

// TestGrepGitignoreExclusion verifies that .gen.go files are excluded from
// grep results when .gitignore contains "*.gen.go".
func TestGrepGitignoreExclusion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	root := t.TempDir()

	// Write .gitignore excluding generated files
	writeFile(t, filepath.Join(root, ".gitignore"), "*.gen.go\n")

	// Create regular .go files
	writeFile(t, filepath.Join(root, "handler.go"), "package main // FUNC_MARKER\n")
	writeFile(t, filepath.Join(root, "service.go"), "package main // FUNC_MARKER\n")

	// Create .gen.go files that should be excluded
	writeFile(t, filepath.Join(root, "types.gen.go"), "package main // FUNC_MARKER\n")
	writeFile(t, filepath.Join(root, "client.gen.go"), "package main // FUNC_MARKER\n")

	tool := NewGrepSearchToolWithRG("") // force native path
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern:    "FUNC_MARKER",
		Path:       root,
		MaxResults: 100,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	body := result.Content
	// Regular files should be present
	if !strings.Contains(body, "handler.go") {
		t.Errorf("expected handler.go in results, got:\n%s", body)
	}
	if !strings.Contains(body, "service.go") {
		t.Errorf("expected service.go in results, got:\n%s", body)
	}
	// .gen.go files should be excluded
	if strings.Contains(body, "types.gen.go") {
		t.Errorf("types.gen.go should have been excluded by .gitignore, got:\n%s", body)
	}
	if strings.Contains(body, "client.gen.go") {
		t.Errorf("client.gen.go should have been excluded by .gitignore, got:\n%s", body)
	}
}

// TestGrepNativeFallback verifies that when ripgrep is not available,
// the native Go implementation returns results correctly.
func TestGrepNativeFallback(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\nfunc Hello() {}\n")
	writeFile(t, filepath.Join(root, "util.go"), "package main\nfunc World() {}\n")

	// Use a non-existent ripgrep path to force native fallback
	tool := NewGrepSearchToolWithRG(filepath.Join(t.TempDir(), "nonexistent_rg_binary"))
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern:    "func",
		Path:       root,
		MaxResults: 50,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Hello") {
		t.Errorf("expected Hello in results, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "World") {
		t.Errorf("expected World in results, got:\n%s", result.Content)
	}
}
