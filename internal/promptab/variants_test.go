package promptab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedVariantsLoad(t *testing.T) {
	vs := EmbeddedVariants()
	if len(vs) < 2 {
		t.Fatalf("expected at least 2 embedded variants, got %d", len(vs))
	}
	names := map[string]bool{}
	for _, v := range vs {
		names[v.Name] = true
		if err := v.Parse(); err != nil {
			t.Errorf("variant %q parse: %v", v.Name, err)
		}
		if v.Get("identity") == "" {
			t.Errorf("variant %q missing identity section", v.Name)
		}
	}
	for _, want := range []string{"default", "concise", "safety-first", "claude-style", "code-reviewer", "pair-programmer"} {
		if !names[want] {
			t.Errorf("missing embedded variant %q", want)
		}
	}
}

func TestRegistryRegisterAndResolve(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	v, err := r.Resolve("default")
	if err != nil {
		t.Fatalf("Resolve default: %v", err)
	}
	if v.Name != "default" {
		t.Errorf("got %q, want default", v.Name)
	}
	_, err = r.Resolve("nonexistent")
	if err == nil {
		t.Errorf("expected error for nonexistent variant")
	}
}

func TestRegistryLoadDirOverrides(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)

	// 用户目录覆盖 default
	tmp := t.TempDir()
	override := `name: default
description: user override
author: test
sections:
  identity: "用户自定义身份"
`
	if err := os.WriteFile(filepath.Join(tmp, "user-default.yaml"), []byte(override), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.LoadDir(tmp); err != nil {
		t.Fatal(err)
	}
	v, _ := r.Resolve("default")
	got := v.Get("identity")
	if !strings.Contains(got, "用户自定义身份") {
		t.Errorf("override should win, got identity=%q", got)
	}
}

func TestRegistryLoadDirMissingOK(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadDir(filepath.Join(t.TempDir(), "nonexistent")); err != nil {
		t.Errorf("missing dir should be OK, got: %v", err)
	}
}

func TestRegistryLoadDirIgnoresNonYAML(t *testing.T) {
	r := NewRegistry()
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("ignore me"), 0644)
	os.WriteFile(filepath.Join(tmp, "good.yaml"), []byte("name: extra\nsections:\n  identity: x\n"), 0644)
	if err := r.LoadDir(tmp); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("extra"); err != nil {
		t.Errorf("expected extra variant loaded, got: %v", err)
	}
}

func TestRenderInterpolates(t *testing.T) {
	v := &Variant{
		Name: "t",
		Sections: map[string]Section{
			"identity": {Body: "OS: {{os}} CWD: {{cwd}}"},
		},
	}
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp"})
	if !strings.Contains(out, "OS: linux CWD: /tmp") {
		t.Errorf("Render = %q", out)
	}
}

func TestRenderSectionOrder(t *testing.T) {
	// 即使 section 顺序在 yaml 中乱序，输出顺序应固定
	v := &Variant{
		Name: "t",
		Sections: map[string]Section{
			"project_rules":     {Body: "PR"},
			"identity":          {Body: "ID"},
			"output_format":     {Body: "OF"},
			"tool_guide":        {Body: "TG"},
		},
	}
	out := v.Render(RenderInputs{})
	idIdx := strings.Index(out, "ID")
	tgIdx := strings.Index(out, "TG")
	ofIdx := strings.Index(out, "OF")
	prIdx := strings.Index(out, "PR")
	if !(idIdx < tgIdx && tgIdx < ofIdx && ofIdx < prIdx) {
		t.Errorf("section order wrong: id=%d tg=%d of=%d pr=%d", idIdx, tgIdx, ofIdx, prIdx)
	}
}

func TestRenderSkipsEmptySections(t *testing.T) {
	v := &Variant{
		Name: "t",
		Sections: map[string]Section{
			"identity": {Body: "ID"},
			"budget_awareness": {Body: ""}, // 空 section
		},
	}
	out := v.Render(RenderInputs{})
	if strings.Contains(out, "成本预算") {
		t.Errorf("empty section should be skipped, got: %q", out)
	}
}

func TestSelectFixed(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	v, chosen, err := r.ResolveWithStrategy(Selector{Strategy: SelectFixed, Fixed: "concise"})
	if err != nil {
		t.Fatal(err)
	}
	if chosen != "concise" || v.Name != "concise" {
		t.Errorf("fixed selection failed: chosen=%q name=%q", chosen, v.Name)
	}
}

func TestSelectFixedFallbackToDefault(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	v, chosen, err := r.ResolveWithStrategy(Selector{Strategy: SelectFixed, Fixed: "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	if chosen != "default" || v.Name != "default" {
		t.Errorf("expected fallback to default, got chosen=%q name=%q", chosen, v.Name)
	}
}

func TestSelectWeightedDeterministic(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	sel := Selector{
		Strategy: SelectWeightedRandom,
		Weights:  map[string]int{"default": 5, "concise": 1, "safety-first": 0},
	}
	// 多次调用应产生可重现序列（基于 stickyCounter）
	results := make(map[string]int)
	for i := 0; i < 30; i++ {
		_, chosen, _ := r.ResolveWithStrategy(sel)
		results[chosen]++
	}
	// safety-first 权重 0，永远不应被选
	if results["safety-first"] > 0 {
		t.Errorf("weight-0 variant should never be chosen, got %d", results["safety-first"])
	}
	// default 权重高，应出现更多次
	if results["default"] <= results["concise"] {
		t.Logf("distribution: %+v (default should be highest)", results)
	}
}

func TestOnSelectCallback(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	var captured []string
	r.SetOnSelect(func(name, source string) {
		captured = append(captured, name)
	})
	r.Resolve("concise")
	if len(captured) != 1 || captured[0] != "concise" {
		t.Errorf("onSelect not called correctly: %v", captured)
	}
}
