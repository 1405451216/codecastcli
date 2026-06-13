package promptab

import (
	"strings"
	"testing"
)

// TestSearchThenEditHasPhases 验证 search-then-edit 变体含两阶段工作流。
func TestSearchThenEditHasPhases(t *testing.T) {
	v := searchThenEditVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	for _, marker := range []string{
		"Phase 1", "Phase 2",
		"Triage", "Edit",
		"NEVER",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("search-then-edit missing: %q", marker)
		}
	}
}

// TestFormatLockedHasRepairPrompt 验证 format-locked 变体含约束词 + repair prompt。
func TestFormatLockedHasRepairPrompt(t *testing.T) {
	v := formatLockedVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	// Aider 标志性约束词
	for _, marker := range []string{
		"MUST", "NEVER", "ONLY EVER", "ALWAYS",
		"repair prompt",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("format-locked missing: %q", marker)
		}
	}
}

// TestArchitectEditHasTwoPhases 验证 architect-edit 变体含双 Agent 拆分。
func TestArchitectEditHasTwoPhases(t *testing.T) {
	v := architectEditVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	for _, marker := range []string{
		"Plan-Agent", "Edit-Agent",
		"Plan 阶段", "Edit 阶段",
		"为什么",  // Plan 强调 why
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("architect-edit missing: %q", marker)
		}
	}
}

// TestShellOnlyHasOneLiners 验证 shell-only 变体含 one-liner 约束。
func TestShellOnlyHasOneLiners(t *testing.T) {
	v := shellOnlyVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	for _, marker := range []string{
		"one-liner", "1-3",
		"占位符",
		"NEVER",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("shell-only missing: %q", marker)
		}
	}
}

// TestLazyModeForbidsTODO 验证 lazy-mode 变体严禁 TODO。
func TestLazyModeForbidsTODO(t *testing.T) {
	v := lazyModeVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	// 反 TODO、反占位、反伪代码
	for _, marker := range []string{
		"TODO", "占位", "伪代码", "完整实现",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("lazy-mode missing: %q", marker)
		}
	}
}

// TestOvereagerModeForbidsScopeCreep 验证 overeager-mode 变体严控 scope。
func TestOvereagerModeForbidsScopeCreep(t *testing.T) {
	v := overeagerModeVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	for _, marker := range []string{
		"顺手", "scope",
		"严格", "绝不",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("overeager-mode missing: %q", marker)
		}
	}
}
