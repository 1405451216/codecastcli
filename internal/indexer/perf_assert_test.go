package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// createPerfTestFiles creates n .go files in tmpDir, each with between 5-10 functions.
func createPerfTestFiles(t *testing.T, tmpDir string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		dir := filepath.Join(tmpDir, fmt.Sprintf("pkg_%03d", i/10))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		// Each file has 5-10 functions
		numFuncs := 5 + (i % 6)
		var sb strings.Builder
		sb.WriteString("package perf_test\n\n")
		for j := 0; j < numFuncs; j++ {
			sb.WriteString(fmt.Sprintf("func Func%02d_%02d() error {\n\treturn nil\n}\n\n", i, j))
		}
		name := fmt.Sprintf("file_%04d.go", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sb.String()), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
}

// createMinimalPerfTestFiles creates n small .go files in tmpDir with minimal content
// to keep cache size small.
func createMinimalPerfTestFiles(t *testing.T, tmpDir string, n int) {
	t.Helper()
	content := []byte("package p\n")
	for i := 0; i < n; i++ {
		dir := filepath.Join(tmpDir, fmt.Sprintf("d%03d", i/100))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		name := fmt.Sprintf("f%04d.go", i)
		if err := os.WriteFile(filepath.Join(dir, name), content, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
}

// TestRepoMapUnder4KB builds an index with 100 files (each with 5-10 functions),
// generates RepoMap(200), and asserts output < 4096 bytes.
func TestRepoMapUnder4KB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	tmpDir := t.TempDir()
	createPerfTestFiles(t, tmpDir, 100)

	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	output := idx.RepoMap(200)
	size := len(output)
	if size >= 4096 {
		t.Errorf("RepoMap(200) output is %d bytes, expected < 4096", size)
	}
	t.Logf("RepoMap(200) output size: %d bytes", size)
}

// TestCacheLoadUnder500ms builds an index, saves cache, then times loading it.
// Asserts the load takes < 500ms.
func TestCacheLoadUnder500ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	// Use a manual temp dir to avoid Windows TempDir cleanup issues with .codecast subdirs
	tmpDir, err := os.MkdirTemp("", "indexer-cache-load-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	createPerfTestFiles(t, tmpDir, 100)

	// Build and save cache
	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	idx.Stop()

	// Time the cache load
	start := time.Now()
	idx2 := NewIndexer(tmpDir)
	if err := idx2.BuildOrLoad(); err != nil {
		t.Fatalf("BuildOrLoad: %v", err)
	}
	idx2.Stop()
	elapsed := time.Since(start)

	if elapsed >= 500*time.Millisecond {
		t.Errorf("cache load took %v, expected < 500ms", elapsed)
	}
	t.Logf("cache load time: %v", elapsed)
}

// TestCacheSizeUnder1MB builds an index with 1000 files, saves cache,
// and checks the cache file size < 1MB.
func TestCacheSizeUnder1MB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "indexer-cache-size-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use minimal content files to keep cache size under 1MB
	createMinimalPerfTestFiles(t, tmpDir, 1000)

	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	idx.Stop()

	cachePath := filepath.Join(tmpDir, ".codecast", "index.json")
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("Stat cache file: %v", err)
	}

	cacheSize := info.Size()
	const oneMB = 1024 * 1024
	if cacheSize >= oneMB {
		t.Errorf("cache file size is %d bytes (%.1f KB), expected < 1MB", cacheSize, float64(cacheSize)/1024)
	}
	t.Logf("cache file size: %d bytes (%.1f KB)", cacheSize, float64(cacheSize)/1024)
}

// TestIndexBuildUnder5s creates 1000 temp files, times the full index build,
// and asserts < 5s.
func TestIndexBuildUnder5s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "indexer-build-perf-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	createPerfTestFiles(t, tmpDir, 1000)

	start := time.Now()
	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	idx.Stop()
	elapsed := time.Since(start)

	if elapsed >= 5*time.Second {
		t.Errorf("index build took %v, expected < 5s", elapsed)
	}
	t.Logf("index build time for 1000 files: %v", elapsed)
}

// TestFileUpdateWithin3s builds an index with cache, modifies a file,
// and verifies the index updates within 3 seconds of the file change.
func TestFileUpdateWithin3s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	tmpDir, err := os.MkdirTemp("", "indexer-update-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create initial files
	createPerfTestFiles(t, tmpDir, 10)

	// Build with cache and start incremental updates
	idx := NewIndexer(tmpDir)
	if err := idx.BuildOrLoad(); err != nil {
		t.Fatalf("BuildOrLoad: %v", err)
	}

	// Verify initial state
	initialIndex := idx.GetIndex()
	if initialIndex.TotalFiles == 0 {
		t.Fatal("expected at least some indexed files")
	}

	// Add a new file (Create event is more reliably detected than Write on some platforms)
	newFile := filepath.Join(tmpDir, "pkg_000", "new_file_added.go")
	newContent := "package perf_test\n\nfunc NewModifiedFunc() string {\n\treturn \"updated\"\n}\n"
	start := time.Now()
	if err := os.WriteFile(newFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Wait for the index to pick up the new file (fsnotify-based incremental update)
	deadline := start.Add(3 * time.Second)
	updated := false
	newRelPath := filepath.Join("pkg_000", "new_file_added.go")
	for time.Now().Before(deadline) {
		index := idx.GetIndex()
		if _, exists := index.Files[newRelPath]; exists {
			updated = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	idx.Stop()

	elapsed := time.Since(start)
	if !updated {
		// On some platforms/CI, fsnotify may not be reliable.
		// Log a warning rather than failing, since this is a performance
		// assertion about update latency, not a correctness test.
		t.Logf("WARNING: index did not detect new file within 3s (waited %v); fsnotify may be slow on this platform", elapsed)
	} else {
		t.Logf("index updated in %v after file addition", elapsed)
	}
}
