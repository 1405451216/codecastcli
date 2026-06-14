package context

import "strings"

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

// TokenBudget manages context token budget
type TokenBudget struct {
	// Budget allocation fields
	MaxTokens     int // Total budget (default: 60% of model context window)
	ReserveSystem int // System message reserve (default: 2000)
	ReserveReply  int // Reply reserve (default: 2000)
	Available     int // Available for conversation history

	// Legacy fields (backward compatibility)
	ContextWindow int
	SystemPrompt  int
	Reserved      int // reserved for response
	Used          int // currently used tokens
}

// NewTokenBudget creates a TokenBudget based on model name
func NewTokenBudget(model string, systemPromptTokens int) *TokenBudget {
	window := GetContextWindow(model)
	maxTokens := int(float64(window) * 0.6)
	reserveSystem := 2000
	reserveReply := 2000

	return &TokenBudget{
		MaxTokens:     maxTokens,
		ReserveSystem: reserveSystem,
		ReserveReply:  reserveReply,
		Available:     maxTokens - reserveSystem - reserveReply,

		ContextWindow: window,
		SystemPrompt:  systemPromptTokens,
		Reserved:      window / 4,
		Used:          0,
	}
}

// NewTokenBudgetWithOverride creates a TokenBudget, using contextBudget if > 0
// instead of auto-detecting from model.
func NewTokenBudgetWithOverride(model string, systemPromptTokens int, contextBudget int) *TokenBudget {
	tb := NewTokenBudget(model, systemPromptTokens)
	if contextBudget > 0 {
		tb.MaxTokens = contextBudget
		tb.Available = contextBudget - tb.ReserveSystem - tb.ReserveReply
	}
	return tb
}

// AvailableTokens returns the number of tokens available for conversation.
// This is the method version that also accounts for Used tokens.
func (tb *TokenBudget) AvailableTokens() int {
	available := tb.Available - tb.Used
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
