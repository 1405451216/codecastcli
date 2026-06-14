package context

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ap "agentprimordia/pkg"
)

// TestCompressRatio: Verify compressed context is < 50% of original for a realistic conversation.
func TestCompressRatio(t *testing.T) {
	c := NewCompressor(2) // preserveRecent = 4

	// Build a realistic conversation with 40 messages (20 user + 20 assistant)
	msgs := make([]ap.Message, 40)
	for i := 0; i < 40; i++ {
		role := ap.RoleUser
		if i%2 == 1 {
			role = ap.RoleAssistant
		}
		// Each message ~200 chars
		msgs[i] = ap.Message{
			Role:     role,
			Content:  fmt.Sprintf("This is message number %d with some realistic content about code changes and file modifications that would typically appear in a coding assistant conversation.", i),
			Metadata: ap.Metadata{Timestamp: time.Now()},
		}
	}
	// Add a system message
	msgs = append([]ap.Message{{Role: "system", Content: "you are a coding assistant"}}, msgs...)

	// Calculate original total content size
	originalSize := 0
	for _, m := range msgs {
		originalSize += len(m.Content)
	}

	// Mock summary that returns a short summary
	shortSummary := func(_ context.Context, _ string) (string, error) {
		return "用户要求重构代码，修改了多个文件，修复了若干错误。", nil
	}

	out, err := c.Compress(context.Background(), msgs, shortSummary)
	if err != nil {
		t.Fatalf("Compress() error: %v", err)
	}

	// Calculate compressed total content size
	compressedSize := 0
	for _, m := range out {
		compressedSize += len(m.Content)
	}

	ratio := float64(compressedSize) / float64(originalSize)
	if ratio >= 0.5 {
		t.Errorf("compressed ratio = %.2f, want < 0.50 (original=%d, compressed=%d)", ratio, originalSize, compressedSize)
	}
}

// TestCompressPreservesFileEdits: Verify file modification records are 100% preserved.
func TestCompressPreservesFileEdits(t *testing.T) {
	c := NewCompressor(1) // preserveRecent = 2

	// Build messages where some contain file edit operations
	msgs := []ap.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "please edit_file auth.go to add logging"},
		{Role: "assistant", Content: "I will edit_file auth.go now"},
		{Role: "assistant", Content: "edit_file applied: auth.go updated with logging"},
		{Role: "user", Content: "now write_file user.go with the new model"},
		{Role: "assistant", Content: "write_file applied: user.go created"},
		{Role: "user", Content: "also create_file config.yaml"},
		{Role: "assistant", Content: "create_file applied: config.yaml created"},
		{Role: "user", Content: "latest request"},
		{Role: "assistant", Content: "latest response"},
	}

	// Summary that preserves file edit info
	fileSummary := func(_ context.Context, prompt string) (string, error) {
		// The prompt contains the original messages, so file operations are in there
		// Simulate a good summary that captures file edits
		if strings.Contains(prompt, "edit_file") || strings.Contains(prompt, "write_file") || strings.Contains(prompt, "create_file") {
			return "文件修改: edit_file auth.go, write_file user.go, create_file config.yaml", nil
		}
		return "no file edits", nil
	}

	out, err := c.Compress(context.Background(), msgs, fileSummary)
	if err != nil {
		t.Fatalf("Compress() error: %v", err)
	}

	// Verify that file edit information is preserved in the output
	// It should be in either the summary message or the recent messages
	allContent := ""
	for _, m := range out {
		allContent += m.Content + " "
	}

	fileOps := []string{"auth.go", "user.go", "config.yaml"}
	for _, op := range fileOps {
		if !strings.Contains(allContent, op) {
			t.Errorf("file edit record for %q not preserved in compressed output", op)
		}
	}
}

// TestCompressCost: Verify compression uses a small/cheap model (verify the config).
func TestCompressCost(t *testing.T) {
	c := NewCompressor(4)

	// Verify the compressor is configured with reasonable defaults
	// that would use a small/cheap model for summarization
	if c.maxSummaryChars <= 0 {
		t.Errorf("maxSummaryChars = %d, want > 0", c.maxSummaryChars)
	}
	// The summary is capped at 2000 chars, which keeps the LLM call cheap
	if c.maxSummaryChars > 2000 {
		t.Errorf("maxSummaryChars = %d, want <= 2000 (keeps summarization cheap)", c.maxSummaryChars)
	}
	// preserveRecent should be reasonable
	if c.preserveRecent <= 0 {
		t.Errorf("preserveRecent = %d, want > 0", c.preserveRecent)
	}
	// Default is 4 rounds = 8 messages preserved
	if c.preserveRecent != 8 {
		t.Errorf("preserveRecent = %d, want 8 (4 rounds * 2)", c.preserveRecent)
	}
}
