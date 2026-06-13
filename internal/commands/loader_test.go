package commands

import (
	"strings"
	"testing"
)

func TestParseValidFrontmatter(t *testing.T) {
	doc := `---
name: explain
description: 解释代码
mode: ask
audience: senior
---

请解释 $ARGUMENTS
`
	spec, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if spec.Name != "explain" {
		t.Errorf("Name = %q, want explain", spec.Name)
	}
	if spec.Description != "解释代码" {
		t.Errorf("Description = %q", spec.Description)
	}
	if spec.Mode != "ask" {
		t.Errorf("Mode = %q, want ask", spec.Mode)
	}
	if spec.Audience != "senior" {
		t.Errorf("Audience = %q", spec.Audience)
	}
	if !strings.Contains(spec.Template, "请解释 $ARGUMENTS") {
		t.Errorf("Template missing body: %q", spec.Template)
	}
}

func TestParseEmptyFrontmatter(t *testing.T) {
	doc := `---
---

只有 body 的命令
`
	_, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse should succeed with empty frontmatter: %v", err)
	}
}

func TestParseMissingClosingDelimiter(t *testing.T) {
	doc := `---
name: broken
description: 没结束
没有结束符
`
	_, err := Parse(doc)
	if err == nil || !strings.Contains(err.Error(), "frontmatter not closed") {
		t.Errorf("want 'frontmatter not closed' error, got: %v", err)
	}
}

func TestParseUnexpectedContentBeforeFrontmatter(t *testing.T) {
	doc := `这是非法的前导内容
---
name: foo
---
body
`
	_, err := Parse(doc)
	if err == nil {
		t.Errorf("want error for content before frontmatter")
	}
}

func TestLoadFileValidation(t *testing.T) {
	// 缺 name
	doc := `---
description: 无名命令
---

body
`
	_, err := Parse(doc)
	// Parse 本身不校验必填，由 LoadFile 校验
	// 但先测试 Parse 不会失败
	if err != nil {
		t.Fatalf("Parse should succeed: %v", err)
	}
	// 模拟 LoadFile 校验
	spec, _ := Parse(doc)
	if spec.Name != "" {
		t.Errorf("Name should be empty, got %q", spec.Name)
	}
}

func TestRenderSimpleVariable(t *testing.T) {
	spec := &CommandSpec{
		Name:      "x",
		Audience:  "senior",
		Depth:     "detailed",
		Template:  "audience={{audience}} depth={{depth}}",
	}
	got := spec.Render(RenderInputs{Arguments: "foo.go"})
	want := "audience=senior depth=detailed"
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderWithFallback(t *testing.T) {
	spec := &CommandSpec{
		Name:     "x",
		Audience: "intermediate",
		Template: `audience={{audience}} focus={{focus | "all"}}`,
	}
	got := spec.Render(RenderInputs{Arguments: "foo.go"})
	want := `audience=intermediate focus=all`
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderArguments(t *testing.T) {
	spec := &CommandSpec{
		Name:     "review",
		Template: `target=$ARGUMENTS, arg0=$ARG0, arg1=$ARG1`,
	}
	got := spec.Render(RenderInputs{Arguments: "foo.go focus=security"})
	want := `target=foo.go focus=security, arg0=foo.go, arg1=focus=security`
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderKeyValueOverride(t *testing.T) {
	spec := &CommandSpec{
		Name:     "review",
		Audience: "intermediate",
		Template: `audience={{audience}} focus={{focus | "all"}}`,
	}
	got := spec.Render(RenderInputs{Arguments: "foo.go focus=security"})
	// Arguments 中的 focus=security 应覆盖 fallback
	want := `audience=intermediate focus=security`
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderDefaultsOverride(t *testing.T) {
	spec := &CommandSpec{
		Name:     "x",
		Audience: "intermediate",
		Template: `audience={{audience}}`,
	}
	got := spec.Render(RenderInputs{
		Arguments: "foo.go",
		Defaults:  map[string]string{"audience": "senior"},
	})
	want := `audience=senior`
	if got != want {
		t.Errorf("Defaults should override frontmatter: got %q, want %q", got, want)
	}
}

func TestRenderQuotedArgument(t *testing.T) {
	spec := &CommandSpec{
		Name:     "x",
		Template: `arg0=[$ARG0] arg1=[$ARG1]`,
	}
	got := spec.Render(RenderInputs{Arguments: `"path with space.go" focus=security`})
	want := `arg0=[path with space.go] arg1=[focus=security]`
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestLoadDir(t *testing.T) {
	specs, err := LoadDir("../../.codecast/commands")
	if err != nil {
		t.Skipf("LoadDir skipped (likely running outside repo): %v", err)
	}
	if len(specs) < 3 {
		t.Errorf("expected at least 3 commands, got %d", len(specs))
	}
	// 检查三个示例
	names := map[string]bool{}
	for _, s := range specs {
		names[s.Name] = true
	}
	for _, want := range []string{"explain", "review", "test"} {
		if !names[want] {
			t.Errorf("missing command %q in .codecast/commands", want)
		}
	}
}

func TestRenderRealExplainCommand(t *testing.T) {
	specs, err := LoadDir("../../.codecast/commands")
	if err != nil {
		t.Skipf("LoadDir skipped: %v", err)
	}
	var explain *CommandSpec
	for _, s := range specs {
		if s.Name == "explain" {
			explain = s
			break
		}
	}
	if explain == nil {
		t.Fatal("explain command not loaded")
	}
	rendered := explain.Render(RenderInputs{Arguments: "src/foo.go"})
	// 应当注入 Arguments
	if !strings.Contains(rendered, "src/foo.go") {
		t.Errorf("rendered prompt missing target: %q", rendered)
	}
	// 应当注入默认值
	if !strings.Contains(rendered, "intermediate") {
		t.Errorf("rendered prompt missing default audience: %q", rendered)
	}
}
