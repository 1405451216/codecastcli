package provider

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
)

// CreateProvider 根据配置创建 LLM Provider
func CreateProvider(cfg *config.Config) (ap.Provider, error) {
	llmCfg := ap.Config{
		APIKey: cfg.APIKey,
		Model:  cfg.Model,
	}
	if cfg.BaseURL != "" {
		llmCfg.BaseURL = cfg.BaseURL
	}

	var p ap.Provider
	var err error

	switch cfg.Provider {
	case "openai":
		p, err = ap.NewOpenAIProvider(llmCfg)
	case "anthropic":
		p, err = ap.NewAnthropicProvider(llmCfg)
	case "gemini":
		p, err = ap.NewGeminiProvider(llmCfg)
	case "ollama":
		if llmCfg.BaseURL == "" {
			llmCfg.BaseURL = "http://localhost:11434"
		}
		p, err = ap.NewOllamaProvider(llmCfg)
	case "azure":
		return nil, fmt.Errorf("azure provider 需要额外配置，请使用 AgentPrimordia 直接配置 AzureConfig")
	case "cohere":
		p, err = ap.NewCohereProvider(llmCfg)
	case "mistral":
		p, err = ap.NewMistralProvider(llmCfg)
	case "deepseek":
		if llmCfg.BaseURL == "" {
			llmCfg.BaseURL = "https://api.deepseek.com"
		}
		p, err = ap.NewOpenAIProvider(llmCfg) // DeepSeek 兼容 OpenAI API
	case "qwen":
		if llmCfg.BaseURL == "" {
			llmCfg.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
		p, err = ap.NewOpenAIProvider(llmCfg) // 通义千问兼容 OpenAI API
	case "glm":
		if llmCfg.BaseURL == "" {
			llmCfg.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
		}
		p, err = ap.NewOpenAIProvider(llmCfg) // 智谱 GLM 兼容 OpenAI API
	default:
		return nil, fmt.Errorf("不支持的 provider: %s", cfg.Provider)
	}

	if err != nil {
		return nil, wrapProviderError(cfg.Provider, err)
	}

	// 验证连通性（30 秒超时）
	if verifyErr := verifyConnectivity(p, 30*time.Second); verifyErr != nil {
		return nil, wrapProviderError(cfg.Provider, verifyErr)
	}

	return p, nil
}

// wrapProviderError 包装 Provider 错误，添加用户友好的提示
func wrapProviderError(providerName string, err error) error {
	hint := "API Key 无效或网络不可达，请检查: 1) API Key 是否正确 2) 网络连接是否正常 3) Base URL 是否正确"
	if isNetworkError(err) {
		hint = "网络连接失败，请检查: 1) 网络连接是否正常 2) Base URL 是否正确 3) 是否需要配置代理"
	}
	return fmt.Errorf("创建 %s Provider 失败: %w\n💡 %s", ProviderDisplayName(providerName), err, hint)
}

// isNetworkError 判断是否为网络相关错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// 检查 net.Error 接口
	var netErr net.Error
	if strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "no such host") ||
		strings.Contains(err.Error(), "TLS") ||
		strings.Contains(err.Error(), "certificate") ||
		strings.Contains(err.Error(), "dial tcp") ||
		strings.Contains(err.Error(), "i/o timeout") ||
		strings.Contains(err.Error(), "network") ||
		strings.Contains(err.Error(), "DNS") {
		return true
	}
	// 也检查是否实现了 net.Error 接口
	if _, ok := err.(net.Error); ok {
		return true
	}
	_ = netErr // 避免未使用变量警告
	return false
}

// verifyConnectivity 验证 Provider 连通性（带超时）
func verifyConnectivity(p ap.Provider, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 尝试简单的连通性验证调用
	ch := make(chan error, 1)
	go func() {
		// Provider 接口可能没有 Ping 方法，尝试通过创建简单请求验证
		// 如果 Provider 不支持验证，则跳过
		if verifier, ok := p.(interface{ VerifyConnectivity(context.Context) error }); ok {
			ch <- verifier.VerifyConnectivity(ctx)
			return
		}
		ch <- nil
	}()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return fmt.Errorf("连接超时 (%v)，请检查网络和 Base URL 配置", timeout)
	}
}

// CreateProviderFromEnv 从环境变量创建 Provider（用于快速测试）
func CreateProviderFromEnv() (ap.Provider, error) {
	cfg := config.Load()
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("未设置 API Key")
	}
	return CreateProvider(cfg)
}

// ListSupportedProviders 返回支持的 Provider 列表
func ListSupportedProviders() []string {
	return []string{
		"openai",
		"anthropic",
		"gemini",
		"ollama",
		"azure",
		"cohere",
		"mistral",
		"deepseek",
		"qwen",
		"glm",
	}
}

// ProviderDisplayName 返回 Provider 的显示名称
func ProviderDisplayName(provider string) string {
	names := map[string]string{
		"openai":    "OpenAI",
		"anthropic": "Anthropic (Claude)",
		"gemini":    "Google Gemini",
		"ollama":    "Ollama (本地)",
		"azure":     "Azure OpenAI",
		"cohere":    "Cohere",
		"mistral":   "Mistral AI",
		"deepseek":  "DeepSeek",
		"qwen":      "通义千问 (Qwen)",
		"glm":       "智谱 GLM",
	}
	if name, ok := names[provider]; ok {
		return name
	}
	return provider
}
