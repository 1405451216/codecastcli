package promptab

import (
	"strings"
	"testing"
)

// TestClaudeStylePreservesXML 验证 claude-style 变体的 XML 标签结构完整。
//
// Claude Fable 5 提示词的核心特征就是用 XML 标签分章节（如 <tool>、<good_response>）。
// 模板插值不应破坏这些标签结构。
func TestClaudeStylePreservesXML(t *testing.T) {
	v := claudeStyleVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{
		OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice",
		ProjectRules: "rule",
		FileTree:     "tree",
	})

	// 关键 XML 标签必须保留
	requiredTags := []string{
		"<product_information>",
		"</product_information>",
		"<core_principles",
		"</core_principles>",
		"<tool_usage>",
		"</tool_usage>",
		"<forbidden_behaviors>",
		"</forbidden_behaviors>",
		"<workflow>",
		"</workflow>",
		"<output_format>",
		"</output_format>",
	}
	for _, tag := range requiredTags {
		if !strings.Contains(out, tag) {
			t.Errorf("claude-style variant missing required XML tag: %q\noutput tail:\n%s", tag, tail(out, 300))
		}
	}
}

// TestCodeReviewerHasStructuredSections 验证 code-reviewer 变体含分级反馈结构。
func TestCodeReviewerHasStructuredSections(t *testing.T) {
	v := codeReviewerVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{
		OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice",
	})
	for _, marker := range []string{
		"## 总体评价",
		"## 🔴 关键问题",
		"## 🟡 改进建议",
		"## 🟢 亮点",
		"## 验证建议",
	} {
		if !strings.Contains(out, marker) {
			t.Errorf("code-reviewer missing section: %q", marker)
		}
	}
}

// TestPairProgrammerHasConversationalTone 验证 pair-programmer 变体含对话式注解。
func TestPairProgrammerHasConversationalTone(t *testing.T) {
	v := pairProgrammerVariant()
	_ = v.Parse()
	out := v.Render(RenderInputs{
		OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice",
	})
	// 至少包含一个对话式标记
	markers := []string{"💡", "✅", "🤔", "我", "我们", "你"}
	found := 0
	for _, m := range markers {
		if strings.Contains(out, m) {
			found++
		}
	}
	if found < 3 {
		t.Errorf("pair-programmer should have conversational tone, found %d/%d markers in:\n%s", found, len(markers), tail(out, 400))
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
