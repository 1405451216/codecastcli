package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestMultiEditAtomicRollback: Create 5 temp files, edit all 5, make 1 fail
// (permission denied on its temp file), verify all 5 are rolled back.
func TestMultiEditAtomicRollback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 5 files with known content
	files := make([]string, 5)
	origContents := make([]string, 5)
	for i := 0; i < 5; i++ {
		files[i] = filepath.Join(tmpDir, "file"+string(rune('A'+i))+".txt")
		origContents[i] = "original_content_" + string(rune('A'+i)) + "\n"
		writeTestFileME(t, files[i], origContents[i])
	}

	// Make fileC's directory entry read-only so the rename step fails
	// We make the file itself read-only so the tmp write or rename fails
	failFile := files[2] // fileC
	if err := os.Chmod(failFile, 0444); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	// Also make the directory non-writable so .tmp file creation fails
	if err := os.Chmod(tmpDir, 0555); err != nil {
		t.Fatalf("chmod dir failed: %v", err)
	}
	defer os.Chmod(tmpDir, 0755) // restore for cleanup

	tool := NewMultiEditTool()
	edits := make([]editOperation, 5)
	for i := 0; i < 5; i++ {
		edits[i] = editOperation{
			FilePath:  files[i],
			OldString: "original_content_" + string(rune('A'+i)),
			NewString: "modified_content_" + string(rune('A'+i)),
		}
	}
	args, _ := json.Marshal(multiEditParams{Edits: edits})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	// Should fail because one file can't be written
	if !result.IsError {
		t.Fatal("expected error result due to permission denied, but got success")
	}

	// Restore permissions so we can read files
	os.Chmod(tmpDir, 0755)
	os.Chmod(failFile, 0644)

	// Verify ALL 5 files are rolled back to original content
	for i := 0; i < 5; i++ {
		got := readTestFileME(t, files[i])
		if got != origContents[i] {
			t.Errorf("file[%d] not rolled back: got %q, want %q", i, got, origContents[i])
		}
	}
}

// TestMultiEditPartialFailure: 3 succeed in preflight, 1 fails,
// verify the 3 successful ones are also rolled back (preflight fails = no writes at all).
func TestMultiEditPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()

	fileA := filepath.Join(tmpDir, "a.txt")
	fileB := filepath.Join(tmpDir, "b.txt")
	fileC := filepath.Join(tmpDir, "c.txt")
	fileD := filepath.Join(tmpDir, "d.txt")

	writeTestFileME(t, fileA, "alpha\n")
	writeTestFileME(t, fileB, "beta\n")
	writeTestFileME(t, fileC, "gamma\n")
	writeTestFileME(t, fileD, "delta\n")

	tool := NewMultiEditTool()
	args, _ := json.Marshal(multiEditParams{
		Edits: []editOperation{
			{FilePath: fileA, OldString: "alpha", NewString: "ALPHA"},
			{FilePath: fileB, OldString: "beta", NewString: "BETA"},
			{FilePath: fileC, OldString: "gamma", NewString: "GAMMA"},
			{FilePath: fileD, OldString: "DOES_NOT_EXIST", NewString: "DELTA"}, // fails preflight
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result (preflight failure), but got success")
	}

	// Verify all 4 files are unchanged (preflight failure = no writes)
	for _, f := range []struct {
		path, orig string
	}{
		{fileA, "alpha\n"},
		{fileB, "beta\n"},
		{fileC, "gamma\n"},
		{fileD, "delta\n"},
	} {
		got := readTestFileME(t, f.path)
		if got != f.orig {
			t.Errorf("file %s not preserved: got %q, want %q", f.path, got, f.orig)
		}
	}
}

// TestMultiEditAllSuccess: 5 files all succeed, verify all changes applied.
func TestMultiEditAllSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	files := make([]string, 5)
	for i := 0; i < 5; i++ {
		files[i] = filepath.Join(tmpDir, "f"+string(rune('0'+i))+".txt")
		writeTestFileME(t, files[i], "old_"+string(rune('0'+i))+"\n")
	}

	tool := NewMultiEditTool()
	edits := make([]editOperation, 5)
	for i := 0; i < 5; i++ {
		edits[i] = editOperation{
			FilePath:  files[i],
			OldString: "old_" + string(rune('0'+i)),
			NewString: "new_" + string(rune('0'+i)),
		}
	}
	args, _ := json.Marshal(multiEditParams{Edits: edits})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, but got error: %s", result.Content)
	}

	// Verify all 5 files have the new content
	for i := 0; i < 5; i++ {
		got := readTestFileME(t, files[i])
		want := "new_" + string(rune('0'+i)) + "\n"
		if got != want {
			t.Errorf("file[%d] mismatch: got %q, want %q", i, got, want)
		}
	}

	// Verify no leftover .tmp files
	for _, f := range files {
		if _, err := os.Stat(f + ".tmp"); err == nil {
			t.Errorf("temp file %s.tmp should have been cleaned up", f)
		}
	}
}
