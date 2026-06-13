package provider

import (
	"strings"
	"testing"

	"codecast/cli/internal/config"
)

func TestCreateProvider_ValidConfig(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantErr  bool
	}{
		{"openai", "openai", false},
		{"anthropic", "anthropic", false},
		{"gemini", "gemini", false},
		{"ollama", "ollama", false},
		{"cohere", "cohere", false},
		{"mistral", "mistral", false},
		{"deepseek", "deepseek", false},
		{"qwen", "qwen", false},
		{"glm", "glm", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				APIKey:   "test-key",
				Model:    "test-model",
				Provider: tt.provider,
			}
			p, err := CreateProvider(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateProvider(%q) error = %v, wantErr %v", tt.provider, err, tt.wantErr)
				return
			}
			if !tt.wantErr && p == nil {
				t.Errorf("CreateProvider(%q) 返回 nil provider", tt.provider)
			}
		})
	}
}

func TestCreateProvider_Azure(t *testing.T) {
	cfg := &config.Config{
		APIKey:   "test-key",
		Model:    "test-model",
		Provider: "azure",
	}
	_, err := CreateProvider(cfg)
	if err == nil {
		t.Error("CreateProvider(azure) 应返回错误")
	}
	if !strings.Contains(err.Error(), "azure") {
		t.Errorf("azure 错误消息应包含 'azure', 得到 %v", err)
	}
}

func TestCreateProvider_EmptyProvider(t *testing.T) {
	cfg := &config.Config{
		APIKey:   "test-key",
		Model:    "test-model",
		Provider: "",
	}
	_, err := CreateProvider(cfg)
	if err == nil {
		t.Error("CreateProvider 空字符串 provider 应返回错误")
	}
}

func TestCreateProvider_UnsupportedProvider(t *testing.T) {
	cfg := &config.Config{
		APIKey:   "test-key",
		Model:    "test-model",
		Provider: "nonexistent",
	}
	_, err := CreateProvider(cfg)
	if err == nil {
		t.Error("CreateProvider 不支持的 provider 应返回错误")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("错误消息应包含 provider 名称, 得到 %v", err)
	}
}

func TestCreateProvider_OllamaDefaultBaseURL(t *testing.T) {
	cfg := &config.Config{
		APIKey:   "",
		Model:    "llama3",
		Provider: "ollama",
	}
	p, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider(ollama) 不应返回错误: %v", err)
	}
	if p == nil {
		t.Error("CreateProvider(ollama) 返回 nil provider")
	}
}

func TestCreateProvider_DeepSeekDefaultBaseURL(t *testing.T) {
	cfg := &config.Config{
		APIKey:   "test-key",
		Model:    "deepseek-chat",
		Provider: "deepseek",
	}
	p, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider(deepseek) 不应返回错误: %v", err)
	}
	if p == nil {
		t.Error("CreateProvider(deepseek) 返回 nil provider")
	}
}

func TestListSupportedProviders(t *testing.T) {
	providers := ListSupportedProviders()

	expected := []string{"openai", "anthropic", "gemini", "ollama", "azure", "cohere", "mistral", "deepseek", "qwen", "glm"}
	if len(providers) != len(expected) {
		t.Errorf("ListSupportedProviders 返回 %d 个 provider, 期望 %d 个", len(providers), len(expected))
	}

	for _, p := range expected {
		found := false
		for _, got := range providers {
			if got == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListSupportedProviders 缺少 %q", p)
		}
	}
}

func TestProviderDisplayName(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"openai", "OpenAI"},
		{"anthropic", "Anthropic (Claude)"},
		{"gemini", "Google Gemini"},
		{"ollama", "Ollama (本地)"},
		{"azure", "Azure OpenAI"},
		{"cohere", "Cohere"},
		{"mistral", "Mistral AI"},
		{"deepseek", "DeepSeek"},
		{"qwen", "通义千问 (Qwen)"},
		{"glm", "智谱 GLM"},
		{"unknown", "unknown"}, // 未知 provider 返回原值
		{"", ""},               // 空字符串返回空
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := ProviderDisplayName(tt.provider)
			if got != tt.want {
				t.Errorf("ProviderDisplayName(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}
