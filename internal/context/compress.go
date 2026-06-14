package context

import (
	"fmt"
)

// CompressConfig holds the configuration for context compression
type CompressConfig struct {
	Enabled        bool    `yaml:"enabled"`
	Threshold      float64 `yaml:"threshold"`        // 0.0-1.0, default 0.7
	SummaryModel   string  `yaml:"summary_model"`    // model for summarization
	ContextWindow  int     `yaml:"context_window"`   // override context window
	PreserveRecent int     `yaml:"preserve_recent"`  // number of recent messages to preserve
}

// NewCompressConfig creates CompressConfig from model name and optional overrides.
// It uses TokenBudget to calculate ContextWindow from the model if not explicitly set.
func NewCompressConfig(model string, summaryModel string, contextBudget int, threshold float64, preserveRecent int) CompressConfig {
	cfg := CompressConfig{
		Enabled:        true,
		Threshold:      0.7,
		PreserveRecent: 4,
	}

	// Use TokenBudget to calculate context window from model
	tb := NewTokenBudget(model, 0)
	if contextBudget > 0 {
		cfg.ContextWindow = contextBudget
	} else {
		cfg.ContextWindow = tb.MaxTokens
	}

	if threshold > 0 && threshold <= 1.0 {
		cfg.Threshold = threshold
	}
	if summaryModel != "" {
		cfg.SummaryModel = summaryModel
	}
	if preserveRecent > 0 {
		cfg.PreserveRecent = preserveRecent
	}
	return cfg
}

// DefaultCompressConfig returns the default compression config
func DefaultCompressConfig() CompressConfig {
	return CompressConfig{
		Enabled:        true,
		Threshold:      0.7,
		PreserveRecent: 4,
	}
}

// NewCompressConfigFromYAML creates CompressConfig from YAML config values
func NewCompressConfigFromYAML(enabled bool, threshold float64, summaryModel string, contextWindow int) CompressConfig {
	cfg := DefaultCompressConfig()
	if enabled {
		cfg.Enabled = enabled
	}
	if threshold > 0 && threshold <= 1.0 {
		cfg.Threshold = threshold
	}
	if summaryModel != "" {
		cfg.SummaryModel = summaryModel
	}
	if contextWindow > 0 {
		cfg.ContextWindow = contextWindow
	}
	return cfg
}

// String returns a human-readable representation of CompressConfig
func (c CompressConfig) String() string {
	return fmt.Sprintf("CompressConfig{enabled=%v, threshold=%.2f, summary_model=%q, context_window=%d, preserve_recent=%d}",
		c.Enabled, c.Threshold, c.SummaryModel, c.ContextWindow, c.PreserveRecent)
}
