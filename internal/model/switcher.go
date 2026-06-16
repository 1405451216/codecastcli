package model

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"codecast/cli/internal/config"
	"codecast/cli/internal/provider"
)

// ModelInfo 模型信息
type ModelInfo struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Provider       string  `json:"provider"`
	ContextWindow  int     `json:"context_window"`
	MaxOutput      int     `json:"max_output"`
	SupportsVision bool    `json:"supports_vision"`
	CostPer1kIn    float64 `json:"cost_per_1k_input"`
	CostPer1kOut   float64 `json:"cost_per_1k_output"`
}

// KnownModels 已知模型列表（截至 2026 年 6 月最新版本，已联网验证）
var KnownModels = []ModelInfo{
	// Anthropic（Claude 4 系列仍为可用最新；Fable 5 于 6.13 因出口管制全球封禁，不可用）
	{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", ContextWindow: 200000, MaxOutput: 64000, SupportsVision: true, CostPer1kIn: 0.003, CostPer1kOut: 0.015},
	{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", Provider: "anthropic", ContextWindow: 200000, MaxOutput: 32000, SupportsVision: true, CostPer1kIn: 0.015, CostPer1kOut: 0.075},
	{ID: "claude-haiku-3-5-20241022", Name: "Claude Haiku 3.5", Provider: "anthropic", ContextWindow: 200000, MaxOutput: 8192, SupportsVision: true, CostPer1kIn: 0.001, CostPer1kOut: 0.005},
	// OpenAI（GPT-5.4/5.5 为截至 2026 年 6 月最新；o3/gpt-4o 保留兼容）
	{ID: "gpt-5.4", Name: "GPT-5.4", Provider: "openai", ContextWindow: 200000, MaxOutput: 64000, SupportsVision: true, CostPer1kIn: 0.005, CostPer1kOut: 0.02},
	{ID: "gpt-5.4-pro", Name: "GPT-5.4 Pro", Provider: "openai", ContextWindow: 200000, MaxOutput: 100000, SupportsVision: true, CostPer1kIn: 0.01, CostPer1kOut: 0.04},
	{ID: "gpt-5.5-instant", Name: "GPT-5.5 Instant", Provider: "openai", ContextWindow: 200000, MaxOutput: 32768, SupportsVision: true, CostPer1kIn: 0, CostPer1kOut: 0},
	{ID: "o3", Name: "o3 (Legacy)", Provider: "openai", ContextWindow: 200000, MaxOutput: 100000, SupportsVision: false, CostPer1kIn: 0.002, CostPer1kOut: 0.008},
	{ID: "o3-mini", Name: "o3 Mini (Legacy)", Provider: "openai", ContextWindow: 200000, MaxOutput: 50000, SupportsVision: false, CostPer1kIn: 0.001, CostPer1kOut: 0.004},
	{ID: "gpt-4o", Name: "GPT-4o (Legacy)", Provider: "openai", ContextWindow: 128000, MaxOutput: 16384, SupportsVision: true, CostPer1kIn: 0.005, CostPer1kOut: 0.015},
	{ID: "gpt-4o-mini", Name: "GPT-4o Mini (Legacy)", Provider: "openai", ContextWindow: 128000, MaxOutput: 16384, SupportsVision: true, CostPer1kIn: 0.00015, CostPer1kOut: 0.0006},
	// Google（Gemini 3 系列为截至 2026 年 6 月最新）
	{ID: "gemini-3-flash", Name: "Gemini 3 Flash", Provider: "google", ContextWindow: 1000000, MaxOutput: 8192, SupportsVision: true, CostPer1kIn: 0.00015, CostPer1kOut: 0.0006},
	{ID: "gemini-3-pro", Name: "Gemini 3 Pro", Provider: "google", ContextWindow: 1000000, MaxOutput: 65536, SupportsVision: true, CostPer1kIn: 0.00125, CostPer1kOut: 0.005},
	{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro (Legacy)", Provider: "google", ContextWindow: 1000000, MaxOutput: 65536, SupportsVision: true, CostPer1kIn: 0.00125, CostPer1kOut: 0.005},
	// DeepSeek（V4-Pro/V4-Flash 为截至 2026 年 6 月最新；V3/R1/R1-Zero 保留兼容）
	{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", Provider: "deepseek", ContextWindow: 1000000, MaxOutput: 32768, SupportsVision: false, CostPer1kIn: 0.0002, CostPer1kOut: 0.0008},
	{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", Provider: "deepseek", ContextWindow: 1000000, MaxOutput: 32768, SupportsVision: false, CostPer1kIn: 0.0001, CostPer1kOut: 0.0004},
	{ID: "deepseek-v3", Name: "DeepSeek V3 (Legacy)", Provider: "deepseek", ContextWindow: 128000, MaxOutput: 8192, SupportsVision: false, CostPer1kIn: 0.00014, CostPer1kOut: 0.00028},
	{ID: "deepseek-r1", Name: "DeepSeek R1 (Legacy)", Provider: "deepseek", ContextWindow: 128000, MaxOutput: 8192, SupportsVision: false, CostPer1kIn: 0.00055, CostPer1kOut: 0.00219},
	{ID: "deepseek-r1-zero", Name: "DeepSeek R1 Zero (Legacy)", Provider: "deepseek", ContextWindow: 128000, MaxOutput: 8192, SupportsVision: false, CostPer1kIn: 0.00055, CostPer1kOut: 0.00219},
	// Qwen（Qwen 3.7 系列为截至 2026 年 6 月最新，1M上下文）
	{ID: "qwen3.7-max", Name: "Qwen 3.7 Max", Provider: "qwen", ContextWindow: 1000000, MaxOutput: 32768, SupportsVision: false, CostPer1kIn: 0.002, CostPer1kOut: 0.008},
	{ID: "qwen3.7-plus", Name: "Qwen 3.7 Plus", Provider: "qwen", ContextWindow: 1000000, MaxOutput: 32768, SupportsVision: true, CostPer1kIn: 0.0008, CostPer1kOut: 0.0024},
	{ID: "qwen3-max", Name: "Qwen 3 Max (Legacy)", Provider: "qwen", ContextWindow: 131072, MaxOutput: 16384, SupportsVision: true, CostPer1kIn: 0.002, CostPer1kOut: 0.006},
	{ID: "qwen3-plus", Name: "Qwen 3 Plus (Legacy)", Provider: "qwen", ContextWindow: 131072, MaxOutput: 16384, SupportsVision: true, CostPer1kIn: 0.0004, CostPer1kOut: 0.0012},
	// GLM（GLM-5.2 为截至 2026 年 6 月最新，1M上下文）
	{ID: "glm-5.2", Name: "GLM-5.2", Provider: "zhipu", ContextWindow: 1000000, MaxOutput: 32768, SupportsVision: true, CostPer1kIn: 0.001, CostPer1kOut: 0.004},
	{ID: "glm-5v-turbo", Name: "GLM-5V Turbo", Provider: "zhipu", ContextWindow: 256000, MaxOutput: 16384, SupportsVision: true, CostPer1kIn: 0.0005, CostPer1kOut: 0.002},
	{ID: "glm-5-plus", Name: "GLM-5 Plus (Legacy)", Provider: "zhipu", ContextWindow: 256000, MaxOutput: 8192, SupportsVision: true, CostPer1kIn: 0.01, CostPer1kOut: 0.01},
	{ID: "glm-5-flash", Name: "GLM-5 Flash (Legacy)", Provider: "zhipu", ContextWindow: 256000, MaxOutput: 8192, SupportsVision: true, CostPer1kIn: 0.001, CostPer1kOut: 0.001},
	// MiMo（小米，截至 2026 年 6 月最新，编程性价比极高）
	{ID: "mimo-v2.5-pro", Name: "MiMo V2.5 Pro", Provider: "mimo", ContextWindow: 1000000, MaxOutput: 32768, SupportsVision: true, CostPer1kIn: 0.000003, CostPer1kOut: 0.000003},
}

// Switcher 模型切换器
type Switcher struct {
	currentModel    string
	currentProvider string
	mu              sync.RWMutex
}

// NewSwitcher 创建模型切换器
func NewSwitcher(cfg *config.Config) *Switcher {
	return &Switcher{
		currentModel:    cfg.Model,
		currentProvider: cfg.Provider,
	}
}

// Switch 切换模型
func (s *Switcher) Switch(modelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查找模型信息
	info := FindModel(modelID)
	if info == nil {
		return fmt.Errorf("未知模型: %s", modelID)
	}

	s.currentModel = info.ID
	s.currentProvider = info.Provider
	return nil
}

// SwitchWithConfig 切换模型并更新配置
func (s *Switcher) SwitchWithConfig(modelID string, cfg *config.Config) error {
	if err := s.Switch(modelID); err != nil {
		// 提供同 Provider 下的可用模型建议
		suggestion := suggestModelsForProvider(cfg.Provider)
		if suggestion != "" {
			return fmt.Errorf("%w\n💡 同 Provider (%s) 下的可用模型:\n%s", err, cfg.Provider, suggestion)
		}
		return err
	}

	cfg.Model = s.currentModel
	cfg.Provider = s.currentProvider
	return nil
}

// suggestModelsForProvider 返回指定 Provider 下的可用模型列表
func suggestModelsForProvider(providerName string) string {
	var models []string
	for _, m := range KnownModels {
		if m.Provider == providerName {
			models = append(models, fmt.Sprintf("  - %s (%s)", m.ID, m.Name))
		}
	}
	return strings.Join(models, "\n")
}

// CurrentModel 返回当前模型
func (s *Switcher) CurrentModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentModel
}

// CurrentProvider 返回当前 Provider
func (s *Switcher) CurrentProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentProvider
}

// FindModel 查找模型信息
func FindModel(modelID string) *ModelInfo {
	for i := range KnownModels {
		if KnownModels[i].ID == modelID {
			return &KnownModels[i]
		}
	}
	// 前缀匹配
	for i := range KnownModels {
		if strings.HasPrefix(modelID, KnownModels[i].ID) {
			return &KnownModels[i]
		}
	}
	return nil
}

// ListModels 列出可用模型
func ListModels(providerName string) []ModelInfo {
	var result []ModelInfo
	for _, m := range KnownModels {
		if providerName == "" || m.Provider == providerName {
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CostPer1kIn < result[j].CostPer1kIn
	})
	return result
}

// ListProviders 列出所有 Provider
func ListProviders() []string {
	seen := make(map[string]bool)
	var providers []string
	for _, m := range KnownModels {
		if !seen[m.Provider] {
			seen[m.Provider] = true
			providers = append(providers, m.Provider)
		}
	}
	return providers
}

// GetModelInfo 获取模型信息字符串
func GetModelInfo(modelID string) string {
	info := FindModel(modelID)
	if info == nil {
		return fmt.Sprintf("未知模型: %s", modelID)
	}
	return fmt.Sprintf("%s (%s) - 上下文: %d tokens, 视觉: %v, 输入: $%.4f/1k, 输出: $%.4f/1k",
		info.Name, info.Provider, info.ContextWindow, info.SupportsVision, info.CostPer1kIn, info.CostPer1kOut)
}

// ValidateProvider 验证 Provider 配置
func ValidateProvider(providerName string, cfg *config.Config) error {
	_, err := provider.CreateProvider(cfg)
	return err
}
