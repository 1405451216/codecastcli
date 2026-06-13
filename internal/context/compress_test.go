package context

import "testing"

func TestNewTokenBudget(t *testing.T) {
	tb := NewTokenBudget("gpt-4o", 500)
	if tb.ContextWindow != 128000 {
		t.Errorf("ContextWindow = %d, want 128000", tb.ContextWindow)
	}
	if tb.SystemPrompt != 500 {
		t.Errorf("SystemPrompt = %d, want 500", tb.SystemPrompt)
	}
	if tb.Reserved != 128000/4 {
		t.Errorf("Reserved = %d, want %d", tb.Reserved, 128000/4)
	}
	if tb.Used != 0 {
		t.Errorf("Used = %d, want 0", tb.Used)
	}
}

func TestNewTokenBudget_UnknownModel(t *testing.T) {
	tb := NewTokenBudget("unknown-model-xyz", 200)
	if tb.ContextWindow != DefaultContextWindow {
		t.Errorf("ContextWindow = %d, want %d (DefaultContextWindow)", tb.ContextWindow, DefaultContextWindow)
	}
}

func TestNewTokenBudget_PrefixMatch(t *testing.T) {
	// "gpt-4o-2024-05-13" should match "gpt-4o" prefix
	tb := NewTokenBudget("gpt-4o-2024-05-13", 100)
	if tb.ContextWindow != 128000 {
		t.Errorf("ContextWindow = %d, want 128000 (prefix match gpt-4o)", tb.ContextWindow)
	}

	// "claude-3-opus-20240229" should match "claude-3-opus" prefix
	tb2 := NewTokenBudget("claude-3-opus-20240229", 100)
	if tb2.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000 (prefix match claude-3-opus)", tb2.ContextWindow)
	}
}

func TestTokenBudget_Available(t *testing.T) {
	tb := &TokenBudget{
		ContextWindow: 100000,
		SystemPrompt:  5000,
		Reserved:      25000,
		Used:          10000,
	}
	want := 100000 - 5000 - 25000 - 10000 // 60000
	if got := tb.Available(); got != want {
		t.Errorf("Available() = %d, want %d", got, want)
	}
}

func TestTokenBudget_Available_Negative(t *testing.T) {
	tb := &TokenBudget{
		ContextWindow: 10000,
		SystemPrompt:  5000,
		Reserved:      3000,
		Used:          5000,
	}
	if got := tb.Available(); got != 0 {
		t.Errorf("Available() = %d, want 0 (clamped from negative)", got)
	}
}

func TestTokenBudget_ShouldCompress(t *testing.T) {
	tb := &TokenBudget{
		ContextWindow: 100000,
		SystemPrompt:  5000,
		Reserved:      25000,
		Used:          0,
	}
	// totalBudget = 100000 - 5000 - 25000 = 70000
	// threshold 0.7 => need Used >= 49000 to compress

	tb.Used = 48000
	if tb.ShouldCompress(0.7) {
		t.Errorf("ShouldCompress(0.7) = true with Used=48000, want false")
	}

	tb.Used = 49000
	if !tb.ShouldCompress(0.7) {
		t.Errorf("ShouldCompress(0.7) = false with Used=49000, want true")
	}

	tb.Used = 60000
	if !tb.ShouldCompress(0.7) {
		t.Errorf("ShouldCompress(0.7) = false with Used=60000, want true")
	}
}

func TestTokenBudget_ShouldCompress_ZeroBudget(t *testing.T) {
	tb := &TokenBudget{
		ContextWindow: 5000,
		SystemPrompt:  5000,
		Reserved:      1000,
		Used:          0,
	}
	// totalBudget = 5000 - 5000 - 1000 = -1000 <= 0
	if !tb.ShouldCompress(0.7) {
		t.Errorf("ShouldCompress() = false with zero/negative budget, want true")
	}
}

func TestGetContextWindow(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-4", 8192},
		{"gpt-4o", 128000},
		{"claude-3-sonnet", 200000},
		{"glm-4", 128000},
		{"deepseek-chat", 64000},
		{"qwen-plus", 131072},
		{"totally-unknown", DefaultContextWindow},
	}
	for _, tt := range tests {
		got := GetContextWindow(tt.model)
		if got != tt.want {
			t.Errorf("GetContextWindow(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestDefaultCompressConfig(t *testing.T) {
	cfg := DefaultCompressConfig()
	if !cfg.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if cfg.Threshold != 0.7 {
		t.Errorf("Threshold = %f, want 0.7", cfg.Threshold)
	}
	if cfg.PreserveRecent != 4 {
		t.Errorf("PreserveRecent = %d, want 4", cfg.PreserveRecent)
	}
	if cfg.SummaryModel != "" {
		t.Errorf("SummaryModel = %q, want empty string", cfg.SummaryModel)
	}
	if cfg.ContextWindow != 0 {
		t.Errorf("ContextWindow = %d, want 0", cfg.ContextWindow)
	}
}

func TestNewCompressConfigFromYAML(t *testing.T) {
	// Custom values
	cfg := NewCompressConfigFromYAML(true, 0.8, "gpt-4o-mini", 64000)
	if !cfg.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if cfg.Threshold != 0.8 {
		t.Errorf("Threshold = %f, want 0.8", cfg.Threshold)
	}
	if cfg.SummaryModel != "gpt-4o-mini" {
		t.Errorf("SummaryModel = %q, want %q", cfg.SummaryModel, "gpt-4o-mini")
	}
	if cfg.ContextWindow != 64000 {
		t.Errorf("ContextWindow = %d, want 64000", cfg.ContextWindow)
	}
	// PreserveRecent should keep default
	if cfg.PreserveRecent != 4 {
		t.Errorf("PreserveRecent = %d, want 4 (default)", cfg.PreserveRecent)
	}
}

func TestNewCompressConfigFromYAML_BoundaryConditions(t *testing.T) {
	// enabled=false should keep default true (since code uses `if enabled`)
	cfg := NewCompressConfigFromYAML(false, 0.8, "model", 1000)
	if !cfg.Enabled {
		t.Errorf("Enabled = false when passing false, want true (default kept since `if enabled` skips false)")
	}

	// threshold=0 should not override default (condition: threshold > 0)
	cfg2 := NewCompressConfigFromYAML(true, 0, "", 0)
	if cfg2.Threshold != 0.7 {
		t.Errorf("Threshold = %f when passing 0, want 0.7 (default kept)", cfg2.Threshold)
	}

	// threshold > 1.0 should not override default
	cfg3 := NewCompressConfigFromYAML(true, 1.5, "", 0)
	if cfg3.Threshold != 0.7 {
		t.Errorf("Threshold = %f when passing 1.5, want 0.7 (default kept)", cfg3.Threshold)
	}

	// threshold = 1.0 should be accepted
	cfg4 := NewCompressConfigFromYAML(true, 1.0, "", 0)
	if cfg4.Threshold != 1.0 {
		t.Errorf("Threshold = %f when passing 1.0, want 1.0", cfg4.Threshold)
	}

	// empty summaryModel should not override default
	cfg5 := NewCompressConfigFromYAML(true, 0.5, "", 0)
	if cfg5.SummaryModel != "" {
		t.Errorf("SummaryModel = %q when passing empty, want empty", cfg5.SummaryModel)
	}

	// contextWindow=0 should not override default
	cfg6 := NewCompressConfigFromYAML(true, 0.5, "m", 0)
	if cfg6.ContextWindow != 0 {
		t.Errorf("ContextWindow = %d when passing 0, want 0 (default kept)", cfg6.ContextWindow)
	}
}
