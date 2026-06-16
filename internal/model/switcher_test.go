package model

import (
	"strings"
	"testing"

	"codecast/cli/internal/config"
)

func TestFindModel(t *testing.T) {
	// 测试已知模型（更新为 2026 年 6 月最新版本）
	tests := []struct {
		modelID  string
		found    bool
		name     string
		provider string
	}{
		{"claude-sonnet-4-20250514", true, "Claude Sonnet 4", "anthropic"},
		{"gpt-4o", true, "GPT-4o (Legacy)", "openai"},
		{"gemini-2.5-pro", true, "Gemini 2.5 Pro (Legacy)", "google"},
		{"deepseek-v4-pro", true, "DeepSeek V4 Pro", "deepseek"},
		{"qwen3.7-max", true, "Qwen 3.7 Max", "qwen"},
		{"glm-5.2", true, "GLM-5.2", "zhipu"},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			info := FindModel(tt.modelID)
			if !tt.found {
				if info != nil {
					t.Errorf("FindModel(%q) 应返回 nil", tt.modelID)
				}
				return
			}
			if info == nil {
				t.Fatalf("FindModel(%q) 返回 nil, want non-nil", tt.modelID)
			}
			if info.Name != tt.name {
				t.Errorf("Name = %q, want %q", info.Name, tt.name)
			}
			if info.Provider != tt.provider {
				t.Errorf("Provider = %q, want %q", info.Provider, tt.provider)
			}
		})
	}
}

func TestFindModel_Unknown(t *testing.T) {
	info := FindModel("nonexistent-model-xyz")
	if info != nil {
		t.Errorf("FindModel(未知模型) 应返回 nil, 实际: %+v", info)
	}
}

func TestListModels(t *testing.T) {
	// 列出所有模型
	all := ListModels("")
	if len(all) != len(KnownModels) {
		t.Errorf("ListModels('') 返回 %d 个模型, want %d", len(all), len(KnownModels))
	}

	// 按 provider 过滤
	openaiModels := ListModels("openai")
	for _, m := range openaiModels {
		if m.Provider != "openai" {
			t.Errorf("ListModels(openai) 返回了非 openai 模型: %s (%s)", m.Name, m.Provider)
		}
	}
	if len(openaiModels) == 0 {
		t.Error("ListModels(openai) 应返回至少一个模型")
	}

	// 按 anthropic 过滤
	anthropicModels := ListModels("anthropic")
	for _, m := range anthropicModels {
		if m.Provider != "anthropic" {
			t.Errorf("ListModels(anthropic) 返回了非 anthropic 模型: %s (%s)", m.Name, m.Provider)
		}
	}

	// 不存在的 provider
	none := ListModels("nonexistent")
	if len(none) != 0 {
		t.Errorf("ListModels(nonexistent) 应返回空, 实际: %d", len(none))
	}
}

func TestListProviders(t *testing.T) {
	providers := ListProviders()
	if len(providers) == 0 {
		t.Error("ListProviders() 应返回至少一个 provider")
	}

	// 验证无重复
	seen := make(map[string]bool)
	for _, p := range providers {
		if seen[p] {
			t.Errorf("ListProviders() 包含重复的 provider: %s", p)
		}
		seen[p] = true
	}

	// 验证已知 provider 存在
	expectedProviders := []string{"anthropic", "openai", "google", "deepseek", "qwen", "zhipu"}
	for _, ep := range expectedProviders {
		if !seen[ep] {
			t.Errorf("ListProviders() 缺少 provider: %s", ep)
		}
	}
}

func TestSwitcher_Switch(t *testing.T) {
	cfg := &config.Config{
		Model:    "gpt-4o",
		Provider: "openai",
	}
	s := NewSwitcher(cfg)

	if s.CurrentModel() != "gpt-4o" {
		t.Errorf("CurrentModel() = %q, want %q", s.CurrentModel(), "gpt-4o")
	}

	err := s.Switch("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("Switch() 失败: %v", err)
	}

	if s.CurrentModel() != "claude-sonnet-4-20250514" {
		t.Errorf("CurrentModel() = %q, want %q", s.CurrentModel(), "claude-sonnet-4-20250514")
	}
	if s.CurrentProvider() != "anthropic" {
		t.Errorf("CurrentProvider() = %q, want %q", s.CurrentProvider(), "anthropic")
	}
}

func TestSwitcher_Switch_Unknown(t *testing.T) {
	cfg := &config.Config{
		Model:    "gpt-4o",
		Provider: "openai",
	}
	s := NewSwitcher(cfg)

	err := s.Switch("nonexistent-model-xyz")
	if err == nil {
		t.Error("Switch(未知模型) 应返回错误")
	}

	// 确认模型未改变
	if s.CurrentModel() != "gpt-4o" {
		t.Errorf("切换失败后 CurrentModel() = %q, want %q", s.CurrentModel(), "gpt-4o")
	}
}

func TestGetModelInfo(t *testing.T) {
	info := GetModelInfo("gpt-4o")
	if !strings.Contains(info, "GPT-4o") {
		t.Errorf("GetModelInfo(gpt-4o) 应包含 'GPT-4o', 实际: %q", info)
	}
	if !strings.Contains(info, "openai") {
		t.Errorf("GetModelInfo(gpt-4o) 应包含 'openai', 实际: %q", info)
	}

	// 未知模型
	unknownInfo := GetModelInfo("nonexistent")
	if !strings.Contains(unknownInfo, "未知模型") {
		t.Errorf("GetModelInfo(未知模型) 应包含 '未知模型', 实际: %q", unknownInfo)
	}
}
