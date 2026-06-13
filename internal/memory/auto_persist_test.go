package memory

import (
	"strings"
	"testing"
)

func TestAutoPersister_LearnFromConversation(t *testing.T) {
	tmpDir := t.TempDir()
	ap := NewAutoPersister(tmpDir)

	// 测试代码风格偏好
	err := ap.LearnFromConversation("请使用tab缩进", "好的")
	if err != nil {
		t.Fatalf("LearnFromConversation 失败: %v", err)
	}

	rules := ap.GetAutoRules()
	if !strings.Contains(rules, "Tab 缩进") {
		t.Errorf("规则应包含 'Tab 缩进', 实际: %q", rules)
	}

	// 测试测试偏好
	err = ap.LearnFromConversation("写测试", "好的")
	if err != nil {
		t.Fatalf("LearnFromConversation 失败: %v", err)
	}

	rules = ap.GetAutoRules()
	if !strings.Contains(rules, "添加测试") {
		t.Errorf("规则应包含 '添加测试', 实际: %q", rules)
	}

	// 测试语言偏好
	err = ap.LearnFromConversation("用中文注释", "好的")
	if err != nil {
		t.Fatalf("LearnFromConversation 失败: %v", err)
	}

	rules = ap.GetAutoRules()
	if !strings.Contains(rules, "中文注释") {
		t.Errorf("规则应包含 '中文注释', 实际: %q", rules)
	}

	// 测试框架偏好
	err = ap.LearnFromConversation("使用gin", "好的")
	if err != nil {
		t.Fatalf("LearnFromConversation 失败: %v", err)
	}

	rules = ap.GetAutoRules()
	if !strings.Contains(rules, "Gin") {
		t.Errorf("规则应包含 'Gin', 实际: %q", rules)
	}

	// 测试禁止事项
	err = ap.LearnFromConversation("不要删除现有代码", "好的")
	if err != nil {
		t.Fatalf("LearnFromConversation 失败: %v", err)
	}

	rules = ap.GetAutoRules()
	if !strings.Contains(rules, "不要删除") {
		t.Errorf("规则应包含 '不要删除', 实际: %q", rules)
	}
}

func TestAutoPersister_GetAutoRules_NoRules(t *testing.T) {
	tmpDir := t.TempDir()
	ap := NewAutoPersister(tmpDir)

	rules := ap.GetAutoRules()
	if rules != "" {
		t.Errorf("初始状态应无规则, 实际: %q", rules)
	}
}

func TestAutoPersister_ClearAutoRules(t *testing.T) {
	tmpDir := t.TempDir()
	ap := NewAutoPersister(tmpDir)

	// 先添加规则
	err := ap.LearnFromConversation("使用空格", "好的")
	if err != nil {
		t.Fatalf("LearnFromConversation 失败: %v", err)
	}

	rules := ap.GetAutoRules()
	if rules == "" {
		t.Fatal("添加规则后应有规则内容")
	}

	// 清除规则
	err = ap.ClearAutoRules()
	if err != nil {
		t.Fatalf("ClearAutoRules 失败: %v", err)
	}

	rules = ap.GetAutoRules()
	if rules != "" {
		t.Errorf("清除后应无规则, 实际: %q", rules)
	}
}

func TestAutoPersister_Deduplication(t *testing.T) {
	tmpDir := t.TempDir()
	ap := NewAutoPersister(tmpDir)

	// 添加相同的学习点两次
	err := ap.LearnFromConversation("使用tab缩进", "好的")
	if err != nil {
		t.Fatalf("第一次 LearnFromConversation 失败: %v", err)
	}

	err = ap.LearnFromConversation("使用tab缩进", "好的")
	if err != nil {
		t.Fatalf("第二次 LearnFromConversation 失败: %v", err)
	}

	rules := ap.GetAutoRules()
	count := strings.Count(rules, "Tab 缩进")
	if count != 1 {
		t.Errorf("重复学习点应去重, 'Tab 缩进' 出现 %d 次, want 1", count)
	}
}

func TestAutoPersister_NoLearningsFromIrrelevant(t *testing.T) {
	tmpDir := t.TempDir()
	ap := NewAutoPersister(tmpDir)

	// 无关对话不应产生学习点
	err := ap.LearnFromConversation("你好", "你好！有什么可以帮你的？")
	if err != nil {
		t.Fatalf("LearnFromConversation 失败: %v", err)
	}

	rules := ap.GetAutoRules()
	if rules != "" {
		t.Errorf("无关对话不应产生规则, 实际: %q", rules)
	}
}
