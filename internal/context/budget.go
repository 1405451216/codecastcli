package context

import "strings"

// ModelContextWindows maps model names to their context window sizes（截至 2026 年 6 月最新，已联网验证）
var ModelContextWindows = map[string]int{
	// OpenAI（GPT-5.4/5.5 为最新）
	"gpt-5.4":        200000,
	"gpt-5.4-pro":    200000,
	"gpt-5.5-instant": 200000,
	"o3":             200000,
	"o3-mini":        200000,
	"gpt-4o":         128000,
	"gpt-4o-mini":    128000,
	"gpt-4-turbo":    128000,
	"gpt-3.5-turbo":  16385,
	// Anthropic
	"claude-sonnet-4-20250514":  200000,
	"claude-opus-4-20250514":    200000,
	"claude-haiku-3-5-20241022": 200000,
	"claude-3-opus":             200000,
	"claude-3-sonnet":           200000,
	"claude-3-haiku":            200000,
	// Google（Gemini 3 系列最新，1M上下文）
	"gemini-3-flash":   1000000,
	"gemini-3-pro":     1000000,
	"gemini-2.5-pro":   1000000,
	"gemini-2.5-flash": 1000000,
	// DeepSeek（V4-Pro/V4-Flash 为 2026.6 最新，1M上下文；V3 将于 7 月下线）
	"deepseek-v4-pro":    1000000,
	"deepseek-v4-flash":  1000000,
	"deepseek-v3":        128000,
	"deepseek-r1":        128000,
	"deepseek-r1-zero":   128000,
	"deepseek-chat":      64000,
	"deepseek-reasoner":  64000,
	"deepseek-coder":     16000,
	// Qwen（3.7 系列为 2026.6 最新，1M上下文）
	"qwen3.7-max":  1000000,
	"qwen3.7-plus": 1000000,
	"qwen3-max":    131072,
	"qwen3-plus":   131072,
	"qwen3-turbo":  131072,
	"qwen-max":     32000,
	"qwen-plus":    131072,
	"qwen-turbo":   131072,
	// GLM（GLM-5.2 为 2026.6.13 最新，1M上下文）
	"glm-5.2":       1000000,
	"glm-5v-turbo":  256000,
	"glm-5-plus":    256000,
	"glm-5-flash":   256000,
	"glm-5-turbo":   256000,
	"glm-4-plus":    128000,
	"glm-4-flash":   128000,
	"glm-4":         128000,
	// MiMo（小米，1M上下文）
	"mimo-v2.5-pro": 1000000,
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
