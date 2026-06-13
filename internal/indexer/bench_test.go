package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// createBenchDir creates a temp directory with n source files for benchmarking.
func createBenchDir(b *testing.B, n int) string {
	b.Helper()
	tmpDir := b.TempDir()

	exts := []string{".go", ".py", ".js", ".ts", ".md"}
	contents := map[string]string{
		".go": "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"hello\") }\n",
		".py": "import os\ndef hello():\n    print('hello')\n",
		".js": "const fs = require('fs');\nmodule.exports = {};\n",
		".ts": "export function greet(name: string): string { return `Hello ${name}`; }\n",
		".md": "# Document\n\nSome content here.\n",
	}

	for i := 0; i < n; i++ {
		ext := exts[i%len(exts)]
		name := fmt.Sprintf("file_%03d%s", i, ext)
		dir := filepath.Join(tmpDir, fmt.Sprintf("dir_%03d", i/5))
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatalf("MkdirAll() error: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents[ext]), 0644); err != nil {
			b.Fatalf("WriteFile() error: %v", err)
		}
	}

	return tmpDir
}

// BenchmarkIndexerBuild benchmarks building index on a temp dir with 50 files.
func BenchmarkIndexerBuild(b *testing.B) {
	tmpDir := createBenchDir(b, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := NewIndexer(tmpDir)
		if err := idx.Build(); err != nil {
			b.Fatalf("Build() error: %v", err)
		}
	}
}

// BenchmarkIndexerGetFileTree benchmarks getting file tree from a pre-built index.
func BenchmarkIndexerGetFileTree(b *testing.B) {
	tmpDir := createBenchDir(b, 50)
	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		b.Fatalf("Build() error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.GetFileTree()
	}
}

// BenchmarkIndexerSearchFiles benchmarks file search on a pre-built index.
func BenchmarkIndexerSearchFiles(b *testing.B) {
	tmpDir := createBenchDir(b, 50)
	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		b.Fatalf("Build() error: %v", err)
	}

	queries := []string{".go", "file_001", "dir_00", "nonexistent"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.SearchFiles(queries[i%len(queries)])
	}
}
