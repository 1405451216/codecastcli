package tui

import (
	"strings"
	"testing"
)

func TestNewStatusBar(t *testing.T) {
	sb := NewStatusBar()

	if sb.Model != "unknown" {
		t.Errorf("Model: got %q, want %q", sb.Model, "unknown")
	}
	if sb.TokenCount != 0 {
		t.Errorf("TokenCount: got %d, want 0", sb.TokenCount)
	}
	if sb.Budget != 0.0 {
		t.Errorf("Budget: got %f, want 0.0", sb.Budget)
	}
	if sb.Mode != "auto-edit" {
		t.Errorf("Mode: got %q, want %q", sb.Mode, "auto-edit")
	}
}

func TestStatusBarRender(t *testing.T) {
	sb := NewStatusBar()
	sb.UpdateModel("gpt-4o")
	sb.UpdateTokens(1500)
	sb.UpdateBudget(1.25)
	sb.UpdateMode("code")

	rendered := sb.Render(120)

	// Verify the rendered output contains key information
	if !strings.Contains(rendered, "gpt-4o") {
		t.Errorf("rendered output should contain model name 'gpt-4o', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "1.5K") {
		t.Errorf("rendered output should contain formatted token count '1.5K', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "$1.25") {
		t.Errorf("rendered output should contain budget '$1.25', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "code") {
		t.Errorf("rendered output should contain mode 'code', got:\n%s", rendered)
	}
}

func TestStatusBarUpdate(t *testing.T) {
	sb := NewStatusBar()

	// Test UpdateModel
	sb.UpdateModel("claude-opus-4")
	if sb.Model != "claude-opus-4" {
		t.Errorf("UpdateModel: got %q, want %q", sb.Model, "claude-opus-4")
	}

	// Test UpdateTokens
	sb.UpdateTokens(5000)
	if sb.TokenCount != 5000 {
		t.Errorf("UpdateTokens: got %d, want 5000", sb.TokenCount)
	}

	// Test UpdateBudget
	sb.UpdateBudget(9.99)
	if sb.Budget != 9.99 {
		t.Errorf("UpdateBudget: got %f, want 9.99", sb.Budget)
	}

	// Test UpdateMode
	sb.UpdateMode("chat")
	if sb.Mode != "chat" {
		t.Errorf("UpdateMode: got %q, want %q", sb.Mode, "chat")
	}
}
