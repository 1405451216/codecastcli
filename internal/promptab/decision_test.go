package promptab

import (
	"strings"
	"testing"
)

// TestDecisionTreeHasStepMarkers 验证 decision-tree 变体含 "Step N" 决策树标记。
func TestDecisionTreeHasStepMarkers(t *testing.T) {
	v := decisionTreeVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	// 至少含 4 个 Step N 标记（Step 0/1/2/3/4）
	count := 0
	for i := 0; i <= 5; i++ {
		marker := "## Step " + string(rune('0'+i))
		if strings.Contains(out, marker) {
			count++
		}
	}
	if count < 4 {
		t.Errorf("decision-tree should have >= 4 Step markers, found %d\n--- output ---\n%s", count, tail(out, 500))
	}
}

// TestSelfCheckHasChecklist 验证 self-check 变体含 5 步自检清单。
func TestSelfCheckHasChecklist(t *testing.T) {
	v := selfCheckVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	for _, marker := range []string{
		"## 自检 1：准确性",
		"## 自检 2：边界",
		"## 自检 3：可逆性",
		"## 自检 4：验证",
		"## 自检 5：诚实",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("self-check missing checklist item: %q", marker)
		}
	}
}

// TestScopeGuardHasBlacklist 验证 scope-guard 变体含黑名单。
func TestScopeGuardHasBlacklist(t *testing.T) {
	v := scopeGuardVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	for _, path := range []string{"/etc", "~/.ssh", "~/.aws", "~/.kube"} {
		if !strings.Contains(out, path) {
			t.Errorf("scope-guard missing blacklisted path: %q", path)
		}
	}
}

// TestMCPRouterHasDecisionTree 验证 mcp-router 变体含决策树。
func TestMCPRouterHasDecisionTree(t *testing.T) {
	v := mcpRouterVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice"})
	for _, marker := range []string{
		"### Step 1",
		"### Step 2",
		"### Step 3",
		"### Step 4",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("mcp-router missing decision step: %q", marker)
		}
	}
	// 核心原则：不要擅自为用户选 partner
	if !strings.Contains(out, "选 partner") {
		t.Errorf("mcp-router should warn against picking partners")
	}
}

func tail2(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
