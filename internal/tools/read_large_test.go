package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadLargeFileChunked: Create a 10MB temp file, read it,
// verify it's read successfully (the tool reads all lines via scanner).
func TestReadLargeFileChunked(t *testing.T) {
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.txt")

	// Create a ~10MB file with many lines
	f, err := os.Create(largeFile)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	line := strings.Repeat("x", 100) + "\n"
	targetSize := 10 * 1024 * 1024 // 10MB
	written := 0
	for written < targetSize {
		n, err := f.WriteString(line)
		if err != nil {
			t.Fatalf("write failed: %v", err)
		}
		written += n
	}
	f.Close()

	tool := NewReadFileTool()
	// Read a range to avoid the large-file truncation hint obscuring results
	args, _ := json.Marshal(readFileParams{
		FilePath:  largeFile,
		StartLine: intPtr(0),
		EndLine:   intPtr(10),
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}

	// Verify the first few lines have line numbers
	if !strings.Contains(result.Content, "   1│") {
		t.Errorf("output should contain line numbers, got: %s", result.Content[:min(200, len(result.Content))])
	}
}

// TestReadBinaryDetection: Create a binary file with null bytes,
// verify it's detected as binary.
func TestReadBinaryDetection(t *testing.T) {
	tmpDir := t.TempDir()
	binFile := filepath.Join(tmpDir, "binary.dat")

	// Create a file with null bytes embedded
	content := []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00, 0x41}
	if err := os.WriteFile(binFile, content, 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{FilePath: binFile})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError {
		t.Error("binary file should return an error result")
	}
	if !strings.Contains(result.Content, "二进制") {
		t.Errorf("error message should mention binary, got: %s", result.Content)
	}
}

// TestReadBinarySkip: Verify binary files return a message instead of garbled content.
func TestReadBinarySkip(t *testing.T) {
	tmpDir := t.TempDir()
	binFile := filepath.Join(tmpDir, "image.bin")

	// Create a file that starts with text but has null bytes later
	content := []byte("some text header\n\x00\x01\x02\x03more garbled stuff")
	if err := os.WriteFile(binFile, content, 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{FilePath: binFile})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError {
		t.Error("binary file should return an error result, not garbled content")
	}
	// Verify it returns the binary detection message, not the raw content
	if strings.Contains(result.Content, "some text header") {
		t.Errorf("should not return raw file content for binary files, got: %s", result.Content)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
