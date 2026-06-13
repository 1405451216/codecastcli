package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	"codecast/cli/internal/diff"
	"codecast/cli/internal/indexer"
	"codecast/cli/internal/permission"
)

// BenchmarkNewAgent benchmarks agent creation time.
// This requires a valid API key; skip if unavailable.
func BenchmarkNewAgent(b *testing.B) {
	apiKey := os.Getenv("CODECAST_API_KEY")
	if apiKey == "" {
		b.Skip("CODECAST_API_KEY not set, skipping BenchmarkNewAgent")
	}

	cfg := &config.Config{
		APIKey:   apiKey,
		Model:    "gpt-4o",
		Provider: "openai",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent, err := New(cfg)
		if err != nil {
			b.Fatalf("New() error: %v", err)
		}
		agent.Close()
	}
}

// BenchmarkProcess benchmarks a mock Process call.
// This requires a valid API key; skip if unavailable.
func BenchmarkProcess(b *testing.B) {
	apiKey := os.Getenv("CODECAST_API_KEY")
	if apiKey == "" {
		b.Skip("CODECAST_API_KEY not set, skipping BenchmarkProcess")
	}

	cfg := &config.Config{
		APIKey:   apiKey,
		Model:    "gpt-4o",
		Provider: "openai",
	}

	agent, err := New(cfg)
	if err != nil {
		b.Fatalf("New() error: %v", err)
	}
	defer agent.Close()

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := agent.Process(ctx, "Say hello"); err != nil {
			b.Fatalf("Process() error: %v", err)
		}
	}
}

// BenchmarkBuildSystemPrompt benchmarks system prompt building without indexer.
func BenchmarkBuildSystemPrompt(b *testing.B) {
	b.Run("without_indexer", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buildSystemPrompt("linux", "/tmp/project", "use tabs for indentation", nil, "suggest", 0)
		}
	})

	b.Run("with_indexer", func(b *testing.B) {
		tmpDir := b.TempDir()
		for j := 0; j < 20; j++ {
			name := fmt.Sprintf("file_%d.go", j)
			content := fmt.Sprintf("package main\nfunc fn%d() {}\n", j)
			os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644)
		}

		idx := indexer.NewIndexer(tmpDir)
		if err := idx.Build(); err != nil {
			b.Fatalf("Build() error: %v", err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buildSystemPrompt("linux", tmpDir, "use tabs for indentation", idx, "auto-edit", 5.0)
		}
	})
}

// BenchmarkBuildPermHook benchmarks permission hook creation.
func BenchmarkBuildPermHook(b *testing.B) {
	mgr, err := permission.NewManagerFromString("suggest")
	if err != nil {
		b.Fatalf("NewManagerFromString() error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hook := buildPermHook(mgr)
		// Invoke the hook once with a nil ToolCall to exercise the fast path.
		hook(context.Background(), &ap.HookContext{})
	}
}

// BenchmarkBuildDiffPreviewHook benchmarks diff preview hook creation and invocation.
func BenchmarkBuildDiffPreviewHook(b *testing.B) {
	prev := diff.NewPreviewer()
	hook := buildDiffPreviewHook(prev)

	hctx := &ap.HookContext{
		ToolCall: &ap.ToolCall{
			Name: "edit_file",
			Args: `{"file_path": "/tmp/test.go", "old_string": "old", "new_string": "new"}`,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hook(context.Background(), hctx)
	}
}

// BenchmarkExtractJSONField benchmarks JSON field extraction.
func BenchmarkExtractJSONField(b *testing.B) {
	jsonStr := `{"file_path": "/very/long/path/to/some/file.go", "old_string": "line1\nline2\nline3", "new_string": "replaced1\nreplaced2"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractJSONField(jsonStr, "file_path")
		extractJSONField(jsonStr, "old_string")
		extractJSONField(jsonStr, "new_string")
	}
}

// BenchmarkLoadProjectRules benchmarks project rules loading.
func BenchmarkLoadProjectRules(b *testing.B) {
	tmpDir := b.TempDir()

	// Create .codecast directory with rules and auto_rules.
	codecastDir := filepath.Join(tmpDir, ".codecast")
	if err := os.MkdirAll(codecastDir, 0755); err != nil {
		b.Fatalf("MkdirAll() error: %v", err)
	}
	os.WriteFile(filepath.Join(codecastDir, "rules.md"), []byte("- Use tabs\n- Max line length 120\n"), 0644)
	os.WriteFile(filepath.Join(codecastDir, "auto_rules.md"), []byte("- Prefer edit_file over write_file\n"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loadProjectRules()
	}
}
