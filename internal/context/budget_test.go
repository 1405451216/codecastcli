package context

import "testing"

func TestNewTokenBudget_DefaultBudget(t *testing.T) {
	tb := NewTokenBudget("gpt-4o", 0)
	// 60% of 128000 = 76800
	if tb.MaxTokens != 76800 {
		t.Errorf("MaxTokens = %d, want 76800", tb.MaxTokens)
	}
	if tb.ReserveSystem != 2000 {
		t.Errorf("ReserveSystem = %d, want 2000", tb.ReserveSystem)
	}
	if tb.ReserveReply != 2000 {
		t.Errorf("ReserveReply = %d, want 2000", tb.ReserveReply)
	}
	// Available = 76800 - 2000 - 2000 = 72800
	if tb.Available != 72800 {
		t.Errorf("Available = %d, want 72800", tb.Available)
	}
}

func TestNewTokenBudget_KnownModelDetection(t *testing.T) {
	tests := []struct {
		model         string
		contextWindow int
		maxTokens     int
	}{
		{"gpt-4o", 128000, 76800},
		{"gpt-4o-mini", 128000, 76800},
		{"gpt-4-turbo", 128000, 76800},
		{"claude-3-opus", 200000, 120000},
		{"claude-3-sonnet", 200000, 120000},
		{"deepseek-chat", 64000, 38400},
		{"qwen-plus", 131072, 78643},
	}
	for _, tt := range tests {
		tb := NewTokenBudget(tt.model, 0)
		if tb.ContextWindow != tt.contextWindow {
			t.Errorf("NewTokenBudget(%q).ContextWindow = %d, want %d", tt.model, tb.ContextWindow, tt.contextWindow)
		}
		if tb.MaxTokens != tt.maxTokens {
			t.Errorf("NewTokenBudget(%q).MaxTokens = %d, want %d", tt.model, tb.MaxTokens, tt.maxTokens)
		}
	}
}

func TestNewTokenBudget_UnknownModelFallback(t *testing.T) {
	tb := NewTokenBudget("some-unknown-model", 0)
	if tb.ContextWindow != DefaultContextWindow {
		t.Errorf("ContextWindow = %d, want %d (DefaultContextWindow)", tb.ContextWindow, DefaultContextWindow)
	}
	// 60% of 128000 = 76800
	if tb.MaxTokens != 76800 {
		t.Errorf("MaxTokens = %d, want 76800", tb.MaxTokens)
	}
}

func TestNewTokenBudget_AvailableCalculation(t *testing.T) {
	tb := NewTokenBudget("gpt-4o", 0)
	// Available = MaxTokens - ReserveSystem - ReserveReply
	expectedAvailable := tb.MaxTokens - tb.ReserveSystem - tb.ReserveReply
	if tb.Available != expectedAvailable {
		t.Errorf("Available = %d, want %d", tb.Available, expectedAvailable)
	}
}

func TestNewTokenBudgetWithOverride(t *testing.T) {
	tb := NewTokenBudgetWithOverride("gpt-4o", 0, 50000)
	if tb.MaxTokens != 50000 {
		t.Errorf("MaxTokens = %d, want 50000 (override)", tb.MaxTokens)
	}
	// Available = 50000 - 2000 - 2000 = 46000
	if tb.Available != 46000 {
		t.Errorf("Available = %d, want 46000", tb.Available)
	}
}

func TestNewTokenBudgetWithOverride_Zero(t *testing.T) {
	tb := NewTokenBudgetWithOverride("gpt-4o", 0, 0)
	// contextBudget=0 means auto-detect from model
	if tb.MaxTokens != 76800 {
		t.Errorf("MaxTokens = %d, want 76800 (auto-detect)", tb.MaxTokens)
	}
}

func TestNewCompressConfig_UsesTokenBudget(t *testing.T) {
	cfg := NewCompressConfig("gpt-4o", "", 0, 0, 0)
	// Should use TokenBudget: 60% of 128000 = 76800
	if cfg.ContextWindow != 76800 {
		t.Errorf("ContextWindow = %d, want 76800", cfg.ContextWindow)
	}
	if cfg.SummaryModel != "" {
		t.Errorf("SummaryModel = %q, want empty", cfg.SummaryModel)
	}
}

func TestNewCompressConfig_WithOverride(t *testing.T) {
	cfg := NewCompressConfig("gpt-4o", "gpt-4o-mini", 50000, 0.8, 6)
	if cfg.ContextWindow != 50000 {
		t.Errorf("ContextWindow = %d, want 50000 (override)", cfg.ContextWindow)
	}
	if cfg.SummaryModel != "gpt-4o-mini" {
		t.Errorf("SummaryModel = %q, want %q", cfg.SummaryModel, "gpt-4o-mini")
	}
	if cfg.Threshold != 0.8 {
		t.Errorf("Threshold = %f, want 0.8", cfg.Threshold)
	}
	if cfg.PreserveRecent != 6 {
		t.Errorf("PreserveRecent = %d, want 6", cfg.PreserveRecent)
	}
}
