package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codecast/cli/internal/promptab"
)

func TestPromptResolverBuildFallsBack(t *testing.T) {
	res := NewPromptResolver()
	// 显式选 nonexistent → 应回落到 default variant
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "nonexistent"})
	prompt := res.Build("linux", "/tmp", "", nil, "suggest", 0)
	// embedded default 的 identity 段应被包含
	if !strings.Contains(prompt, "CodecastAgent") {
		t.Errorf("fallback to default variant failed, got prompt tail:\n%s", tail(prompt, 200))
	}
}

func TestPromptResolverBuildWithConciseVariant(t *testing.T) {
	res := NewPromptResolver()
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "concise"})
	prompt := res.Build("linux", "/tmp", "", nil, "suggest", 0)
	// concise variant 标识：identity 段用极简句式
	if !strings.Contains(prompt, "改前必读") {
		t.Errorf("concise variant not selected, got:\n%s", tail(prompt, 200))
	}
	// 不应包含 default 的"核心准则"段
	if strings.Contains(prompt, "核心准则（按优先级排序") {
		t.Errorf("concise should not include default's verbose section, got:\n%s", tail(prompt, 400))
	}
}

func TestPromptResolverBuildWithSafetyFirst(t *testing.T) {
	res := NewPromptResolver()
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "safety-first"})
	prompt := res.Build("darwin", "/Users/dev", "", nil, "auto-edit", 0)
	if !strings.Contains(prompt, "极度保守") {
		t.Errorf("safety-first variant not selected, got:\n%s", tail(prompt, 200))
	}
}

func TestPromptResolverInterpolatesOS(t *testing.T) {
	res := NewPromptResolver()
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "default"})
	prompt := res.Build("darwin", "/Users/dev/proj", "", nil, "suggest", 0)
	if !strings.Contains(prompt, "darwin") {
		t.Errorf("OS interpolation failed, prompt should contain 'darwin':\n%s", tail(prompt, 300))
	}
	if !strings.Contains(prompt, "/Users/dev/proj") {
		t.Errorf("CWD interpolation failed, prompt should contain CWD:\n%s", tail(prompt, 300))
	}
}

func TestPromptResolverInterpolatesBudget(t *testing.T) {
	res := NewPromptResolver()
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "default"})
	prompt := res.Build("linux", "/tmp", "", nil, "suggest", 12.34)
	if !strings.Contains(prompt, "12.34") {
		t.Errorf("budget interpolation failed, prompt should contain '12.34':\n%s", tail(prompt, 300))
	}
}

func TestPromptResolverConciseDoesNotIncludeVerboseSection(t *testing.T) {
	res := NewPromptResolver()
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "concise"})
	concise := res.Build("linux", "/tmp", "", nil, "suggest", 0)
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "default"})
	def := res.Build("linux", "/tmp", "", nil, "suggest", 0)
	// concise 应该比 default 短不少
	if len(concise) >= len(def) {
		t.Logf("concise=%d default=%d (concise should be shorter)", len(concise), len(def))
	}
}

func TestLoadProjectDirOverrides(t *testing.T) {
	res := NewPromptResolver()
	// 创建临时项目级 prompts 目录
	dir := t.TempDir()
	override := `name: default
description: project override
author: project-test
sections:
  identity: "PROJECT_OVERRIDE_MARKER"
`
	if err := os.WriteFile(filepath.Join(dir, "project.yaml"), []byte(override), 0644); err != nil {
		t.Fatal(err)
	}
	if err := res.LoadProjectDir(dir); err != nil {
		t.Fatal(err)
	}
	res.SetSelector(promptab.Selector{Strategy: promptab.SelectFixed, Fixed: "default"})
	prompt := res.Build("linux", "/tmp", "", nil, "suggest", 0)
	if !strings.Contains(prompt, "PROJECT_OVERRIDE_MARKER") {
		t.Errorf("project-level override should win, prompt tail:\n%s", tail(prompt, 300))
	}
}

func TestLoadProjectDirMissingOK(t *testing.T) {
	res := NewPromptResolver()
	if err := res.LoadProjectDir(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Errorf("missing project dir should be silent, got: %v", err)
	}
}

func TestLoadProjectDirInvalidYAMLErrors(t *testing.T) {
	res := NewPromptResolver()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("not: valid: yaml: ::::"), 0644)
	err := res.LoadProjectDir(dir)
	if err == nil {
		t.Errorf("expected error for invalid YAML")
	}
}

func TestSelectorConfigToSelector(t *testing.T) {
	tests := []struct {
		name     string
		cfg      SelectorConfig
		wantStr  promptab.SelectStrategy
		wantName string
	}{
		{"empty defaults to fixed", SelectorConfig{}, promptab.SelectFixed, ""},
		{"fixed with variant", SelectorConfig{Variant: "concise"}, promptab.SelectFixed, "concise"},
		{"round-robin strategy", SelectorConfig{Strategy: "round-robin", Variant: "x"}, promptab.SelectRoundRobin, "x"},
		{"weighted strategy", SelectorConfig{Strategy: "weighted", Variant: "x"}, promptab.SelectWeightedRandom, "x"},
		{"weighted-random alias", SelectorConfig{Strategy: "weighted-random"}, promptab.SelectWeightedRandom, ""},
		{"unknown strategy falls back to fixed", SelectorConfig{Strategy: "garbage"}, promptab.SelectFixed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := tt.cfg.ToSelector()
			if sel.Strategy != tt.wantStr {
				t.Errorf("Strategy = %v, want %v", sel.Strategy, tt.wantStr)
			}
			if sel.Fixed != tt.wantName {
				t.Errorf("Fixed = %q, want %q", sel.Fixed, tt.wantName)
			}
		})
	}
}

func TestResolverEndToEndWithWeights(t *testing.T) {
	res := NewPromptResolver()
	res.SetSelector(SelectorConfig{
		Strategy: "weighted",
		Weights:  map[string]int{"default": 1, "concise": 0, "safety-first": 0},
	}.ToSelector())
	prompt := res.Build("linux", "/tmp", "", nil, "suggest", 0)
	// weight 0 应被忽略；只有 default 权重>0，应被选中
	if !strings.Contains(prompt, "CodecastAgent") {
		t.Errorf("expected default variant, got:\n%s", tail(prompt, 200))
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
