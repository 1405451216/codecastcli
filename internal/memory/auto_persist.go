package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AutoPersister 自动记忆持久化器
type AutoPersister struct {
	projectRoot string
	rulesDir    string
	maxEntries  int
	mu          sync.Mutex
}

// NewAutoPersister 创建自动持久化器
func NewAutoPersister(projectRoot string) *AutoPersister {
	return &AutoPersister{
		projectRoot: projectRoot,
		rulesDir:    filepath.Join(projectRoot, ".codecast"),
		maxEntries:  50,
	}
}

// LearnFromConversation 从对话中学习并持久化
func (ap *AutoPersister) LearnFromConversation(userMessage, assistantResponse string) error {
	// 提取学习点
	learnings := ap.extractLearnings(userMessage, assistantResponse)
	if len(learnings) == 0 {
		return nil
	}

	return ap.persistLearnings(learnings)
}

// extractLearnings 从对话中提取学习点
func (ap *AutoPersister) extractLearnings(userMsg, assistantResp string) []string {
	var learnings []string

	// 检测用户偏好
	lower := strings.ToLower(userMsg)

	// 代码风格偏好
	if strings.Contains(lower, "使用tab") || strings.Contains(lower, "用tab缩进") {
		learnings = append(learnings, "- 代码风格: 使用 Tab 缩进")
	}
	if strings.Contains(lower, "使用空格") || strings.Contains(lower, "2空格缩进") {
		learnings = append(learnings, "- 代码风格: 使用 2 空格缩进")
	}
	if strings.Contains(lower, "4空格缩进") {
		learnings = append(learnings, "- 代码风格: 使用 4 空格缩进")
	}

	// 测试偏好
	if strings.Contains(lower, "写测试") || strings.Contains(lower, "添加测试") {
		learnings = append(learnings, "- 测试: 修改代码后添加测试")
	}

	// 语言偏好
	if strings.Contains(lower, "用中文注释") || strings.Contains(lower, "中文注释") {
		learnings = append(learnings, "- 语言: 使用中文注释")
	}
	if strings.Contains(lower, "英文注释") || strings.Contains(lower, "用英文注释") {
		learnings = append(learnings, "- 语言: 使用英文注释")
	}

	// 框架偏好
	if strings.Contains(lower, "使用gin") {
		learnings = append(learnings, "- 框架: 使用 Gin Web Framework")
	}
	if strings.Contains(lower, "使用echo") {
		learnings = append(learnings, "- 框架: 使用 Echo Framework")
	}

	// 禁止事项
	if strings.Contains(lower, "不要删除") || strings.Contains(lower, "别删") {
		learnings = append(learnings, "- 禁止: 不要删除现有代码/测试")
	}
	if strings.Contains(lower, "不要修改") || strings.Contains(lower, "别改") {
		learnings = append(learnings, "- 禁止: 不要修改指定文件")
	}

	return learnings
}

// persistLearnings 持久化学习点
func (ap *AutoPersister) persistLearnings(learnings []string) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	if err := os.MkdirAll(ap.rulesDir, 0755); err != nil {
		return fmt.Errorf("创建规则目录失败: %w", err)
	}

	autoRulesPath := filepath.Join(ap.rulesDir, "auto_rules.md")

	// 读取已有规则
	existing := ""
	if data, err := os.ReadFile(autoRulesPath); err == nil {
		existing = string(data)
	}

	// 合并新学习点（去重）
	for _, learning := range learnings {
		if !strings.Contains(existing, learning) {
			existing += "\n" + learning
		}
	}

	// 写入文件
	header := fmt.Sprintf("# 自动学习规则 (生成于 %s)\n# 此文件由 Codecast 自动生成，请勿手动修改\n", time.Now().Format("2006-01-02"))
	content := header + existing

	return os.WriteFile(autoRulesPath, []byte(content), 0644)
}

// GetAutoRules 获取自动学习的规则
func (ap *AutoPersister) GetAutoRules() string {
	autoRulesPath := filepath.Join(ap.rulesDir, "auto_rules.md")
	data, err := os.ReadFile(autoRulesPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// ClearAutoRules 清除自动学习规则
func (ap *AutoPersister) ClearAutoRules() error {
	autoRulesPath := filepath.Join(ap.rulesDir, "auto_rules.md")
	return os.Remove(autoRulesPath)
}
