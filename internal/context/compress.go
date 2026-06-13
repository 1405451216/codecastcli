package context

import (
	"fmt"
	"strings"
)

// ModelContextWindows maps model names to their context window sizes
var ModelContextWindows = map[string]int{
	"gpt-4":             8192,
	"gpt-4-turbo":       128000,
	"gpt-4o":            128000,
	"gpt-4o-mini":       128000,
	"gpt-3.5-turbo":     16385,
	"claude-3-opus":     200000,
	"claude-3-sonnet":   200000,
	"claude-3-haiku":    200000,
	"claude-3.5-sonnet": 200000,
	"claude-4-sonnet":   200000,
	"glm-4":             128000,
	"glm-4-plus":        128000,
	"glm-4-flash":       128000,
	"deepseek-chat":     64000,
	"deepseek-coder":    16000,
	"qwen-max":          32000,
	"qwen-plus":         131072,
	"qwen-turbo":        131072,
}

// DefaultContextWindow is the fallback context window size
const DefaultContextWindow = 128000

// TokenBudget calculates how many tokens are available for the conversation
type TokenBudget struct {
	ContextWindow int
	SystemPrompt  int
	Reserved      int // reserved for response
	Used          int // currently used tokens
}

// NewTokenBudget creates a TokenBudget based on model name
func NewTokenBudget(model string, systemPromptTokens int) *TokenBudget {
	window := GetContextWindow(model)
	return &TokenBudget{
		ContextWindow: window,
		SystemPrompt:  systemPromptTokens,
		Reserved:      window / 4, // 25% reserved for response
		Used:          0,
	}
}

// Available returns the number of tokens available for conversation
func (tb *TokenBudget) Available() int {
	available := tb.ContextWindow - tb.SystemPrompt - tb.Reserved - tb.Used
	if available < 0 {
		return 0
	}
	return available
}

// ShouldCompress returns true if token usage exceeds the threshold
func (tb *TokenBudget) ShouldCompress(threshold float64) bool {
	totalBudget := tb.ContextWindow - tb.SystemPrompt - tb.Reserved
	if totalBudget <= 0 {
		return true
	}
	usage := float64(tb.Used) / float64(totalBudget)
	return usage >= threshold
}

// GetContextWindow returns the context window size for a model
func GetContextWindow(model string) int {
	// Try exact match first
	if w, ok := ModelContextWindows[model]; ok {
		return w
	}
	// Try longest prefix match (e.g., "gpt-4o-2024-05-13" matches "gpt-4o" not "gpt-4")
	bestPrefix := ""
	bestWindow := 0
	for prefix, w := range ModelContextWindows {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestWindow = w
		}
	}
	if bestWindow > 0 {
		return bestWindow
	}
	return DefaultContextWindow
}

// CompressConfig holds the configuration for context compression
type CompressConfig struct {
	Enabled        bool    `yaml:"enabled"`
	Threshold      float64 `yaml:"threshold"`        // 0.0-1.0, default 0.7
	SummaryModel   string  `yaml:"summary_model"`    // model for summarization
	ContextWindow  int     `yaml:"context_window"`   // override context window
	PreserveRecent int     `yaml:"preserve_recent"`  // number of recent messages to preserve
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
