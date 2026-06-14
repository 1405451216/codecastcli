package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codecast/cli/internal/config"
	"codecast/cli/internal/diff"
	"codecast/cli/internal/indexer"
	automem "codecast/cli/internal/memory"
	"codecast/cli/internal/model"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/tui"
)

// TestBuildSystemPromptWithIndexer 测试 DI-2: 系统提示词注入文件树
func TestBuildSystemPromptWithIndexer(t *testing.T) {
	// 创建临时目录作为项目根
	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	idx := indexer.NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatal(err)
	}

	prompt := buildSystemPrompt("linux", tmpDir, "", idx, "suggest", 0)

	// 验证文件树被注入
	if !strings.Contains(prompt, "代码库结构") {
		t.Error("系统提示词应包含代码库结构部分")
	}
	if !strings.Contains(prompt, "main.go") {
		t.Error("系统提示词应包含 main.go 文件")
	}
	if !strings.Contains(prompt, "go") {
		t.Error("系统提示词应包含 Go 语言统计")
	}
}

// TestBuildSystemPromptWithoutIndexer 测试无索引器时的系统提示词
func TestBuildSystemPromptWithoutIndexer(t *testing.T) {
	prompt := buildSystemPrompt("linux", "/tmp", "测试规则", nil, "suggest", 0)

	if !strings.Contains(prompt, "操作系统: linux") {
		t.Error("应包含操作系统信息")
	}
	if !strings.Contains(prompt, "测试规则") {
		t.Error("应包含项目规则")
	}
	if strings.Contains(prompt, "代码库结构") {
		t.Error("无索引器时不应包含代码库结构")
	}
}

// TestBuildSystemPromptWithProjectRules 测试系统提示词包含项目规则
func TestBuildSystemPromptWithProjectRules(t *testing.T) {
	prompt := buildSystemPrompt("windows", "C:\\project", "使用 Tab 缩进", nil, "full-auto", 0)

	if !strings.Contains(prompt, "项目规则") {
		t.Error("应包含项目规则部分")
	}
	if !strings.Contains(prompt, "使用 Tab 缩进") {
		t.Error("应包含具体规则内容")
	}
}

// TestDiffPreviewHook 测试 DI-3: Diff 预览 Hook
func TestDiffPreviewHook(t *testing.T) {
	prev := diff.NewPreviewer()
	hook := buildDiffPreviewHook(prev)

	// 验证 hook 不 panic
	if hook == nil {
		t.Error("diff preview hook 不应为 nil")
	}
}

// TestJsonGetString 测试 jsonGetString 的各种输入场景。
// 这同时是 P0 漏洞修复的回归测试：验证标准库能正确处理
// \uXXXX、嵌套对象、控制字符、多行字符串、攻击载荷等。
func TestJsonGetString(t *testing.T) {
	tests := []struct {
		name     string
		jsonStr  string
		key      string
		expected string
	}{
		{
			name:     "basic string",
			jsonStr:  `{"key": "value"}`,
			key:      "key",
			expected: "value",
		},
		{
			name:     "empty object",
			jsonStr:  `{}`,
			key:      "key",
			expected: "",
		},
		{
			name:     "missing field",
			jsonStr:  `{"other": "x"}`,
			key:      "key",
			expected: "",
		},
		{
			name:     "nested object returns empty (not a string)",
			jsonStr:  `{"key": {"nested": "val"}}`,
			key:      "key",
			expected: "",
		},
		{
			name:     "unicode escape decoded (P0 漏洞修复验证)",
			jsonStr:  `{"key": "hello"}`,
			key:      "key",
			expected: "hello",
		},
		{
			name:     "control char \\n decoded",
			jsonStr:  `{"key": "line1\nline2"}`,
			key:      "key",
			expected: "line1\nline2",
		},
		{
			name:     "multiline string with embedded newlines",
			jsonStr:  `{"key": "a\nb\nc"}`,
			key:      "key",
			expected: "a\nb\nc",
		},
		{
			name:     "control char \\t decoded",
			jsonStr:  `{"key": "col1\tcol2"}`,
			key:      "key",
			expected: "col1\tcol2",
		},
		{
			name:     "control char \\b and \\f decoded",
			jsonStr:  `{"key": "a\bb\fcc"}`,
			key:      "key",
			expected: "a\bb\fcc",
		},
		{
			name:     "numeric value returns empty (not a string)",
			jsonStr:  `{"key": 123}`,
			key:      "key",
			expected: "",
		},
		{
			name:     "array value returns empty (not a string)",
			jsonStr:  `{"key": [1,2]}`,
			key:      "key",
			expected: "",
		},
		{
			name:     "boolean value returns empty (not a string)",
			jsonStr:  `{"key": true}`,
			key:      "key",
			expected: "",
		},
		{
			name:     "null value returns empty",
			jsonStr:  `{"key": null}`,
			key:      "key",
			expected: "",
		},
		{
			name:     "attack payload file_path",
			jsonStr:  `{"file_path": "/etc/passwd"}`,
			key:      "file_path",
			expected: "/etc/passwd",
		},
		{
			name:     "file_path via path alias",
			jsonStr:  `{"path": "/etc/passwd"}`,
			key:      "path",
			expected: "/etc/passwd",
		},
		{
			name:     "unicode in attack payload (was a bypass vector)",
			jsonStr:  `{"file_path": "\/etc\/passwd"}`,
			key:      "file_path",
			expected: "/etc/passwd",
		},
		{
			name:     "escaped quotes in value",
			jsonStr:  `{"key": "say \"hi\""}`,
			key:      "key",
			expected: `say "hi"`,
		},
		{
			name:     "escaped backslash",
			jsonStr:  `{"key": "a\\b"}`,
			key:      "key",
			expected: `a\b`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tt.jsonStr), &m); err != nil {
				// 整体 JSON 解析失败对 jsonGetString 来说视为空 map
				m = map[string]json.RawMessage{}
			}
			result := jsonGetString(m, tt.key)
			if result != tt.expected {
				t.Errorf("jsonGetString(%q, %q) = %q, want %q", tt.jsonStr, tt.key, result, tt.expected)
			}
		})
	}
}

// TestJsonGetString_InvalidJSON 验证非法 JSON 输入不会 panic，且返回空字符串
func TestJsonGetString_InvalidJSON(t *testing.T) {
	// 直接测试 jsonGetString 在 nil map 上的行为
	if got := jsonGetString(nil, "anything"); got != "" {
		t.Errorf("jsonGetString(nil, ...) = %q, want empty", got)
	}
}

// TestLoadProjectRulesWithAutoRules 测试 DI-5: 自动加载 auto_rules.md
func TestLoadProjectRulesWithAutoRules(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 .codecast 目录和 auto_rules.md
	codecastDir := filepath.Join(tmpDir, ".codecast")
	if err := os.MkdirAll(codecastDir, 0755); err != nil {
		t.Fatal(err)
	}
	autoRules := "- 代码风格: 使用 Tab 缩进"
	if err := os.WriteFile(filepath.Join(codecastDir, "auto_rules.md"), []byte(autoRules), 0644); err != nil {
		t.Fatal(err)
	}

	// 保存当前目录并切换
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	rules := loadProjectRules()
	if !strings.Contains(rules, "自动学习规则") {
		t.Error("应包含自动学习规则部分")
	}
	if !strings.Contains(rules, "Tab 缩进") {
		t.Error("应包含 auto_rules.md 的内容")
	}
}

// TestModelSwitcherIntegration 测试 DI-4: 模型切换器集成
func TestModelSwitcherIntegration(t *testing.T) {
	cfg := &config.Config{
		Model:    "gpt-4o",
		Provider: "openai",
		APIKey:   "test-key",
	}

	switcher := model.NewSwitcher(cfg)
	if switcher.CurrentModel() != "gpt-4o" {
		t.Errorf("初始模型应为 gpt-4o, got %s", switcher.CurrentModel())
	}

	// 切换到已知模型
	err := switcher.SwitchWithConfig("claude-sonnet-4-20250514", cfg)
	if err != nil {
		t.Fatalf("切换模型失败: %v", err)
	}

	if switcher.CurrentModel() != "claude-sonnet-4-20250514" {
		t.Errorf("切换后模型应为 claude-sonnet-4-20250514, got %s", switcher.CurrentModel())
	}
	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Error("配置中的模型也应更新")
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider 应更新为 anthropic, got %s", cfg.Provider)
	}

	// 切换到未知模型应失败
	err = switcher.Switch("unknown-model-xyz")
	if err == nil {
		t.Error("切换到未知模型应返回错误")
	}
}

// TestTUIRendererIntegration 测试 DI-1: TUI 渲染器集成
func TestTUIRendererIntegration(t *testing.T) {
	renderer := tui.NewRenderer()
	if renderer == nil {
		t.Fatal("渲染器不应为 nil")
	}

	// 测试 Markdown 渲染
	result := renderer.RenderMarkdown("# Hello\n\nThis is **bold** text.")
	if result == "" {
		t.Error("渲染结果不应为空")
	}

	// 测试代码块渲染
	codeResult := renderer.RenderCodeBlock("fmt.Println(\"hello\")", "go")
	if codeResult == "" {
		t.Error("代码块渲染结果不应为空")
	}

	// 测试 Diff 渲染
	diffResult := renderer.RenderDiff("--- a/test.go\n+++ b/test.go\n@@\n-old line\n+new line")
	if diffResult == "" {
		t.Error("Diff 渲染结果不应为空")
	}
}

// TestPermissionHookIntegration 测试权限 Hook 集成
func TestPermissionHookIntegration(t *testing.T) {
	mgr, err := permission.NewManagerFromString("suggest")
	if err != nil {
		t.Fatal(err)
	}
	hook := buildPermHook(mgr)
	if hook == nil {
		t.Error("权限 hook 不应为 nil")
	}

	// 测试安全模式
	mgr.AddDeny("shell_execute")
	if !mgr.IsDenied("shell_execute") {
		t.Error("shell_execute 应被禁止")
	}
}

// TestAutoMemoryIntegration 测试 DI-5: 自动记忆集成
func TestAutoMemoryIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	autoMem := automem.NewAutoPersister(tmpDir)

	// 学习用户偏好
	err := autoMem.LearnFromConversation("请使用tab缩进", "好的，我将使用 Tab 缩进")
	if err != nil {
		t.Fatalf("学习失败: %v", err)
	}

	// 验证规则已持久化
	rules := autoMem.GetAutoRules()
	if rules == "" {
		t.Error("自动规则不应为空")
	}
	if !strings.Contains(rules, "Tab") {
		t.Error("自动规则应包含 Tab 缩进偏好")
	}

	// 清除规则
	if err := autoMem.ClearAutoRules(); err != nil {
		t.Fatalf("清除规则失败: %v", err)
	}
}

// TestIndexerIntegration 测试索引器集成
func TestIndexerIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件
	goFile := filepath.Join(tmpDir, "main.go")
	os.WriteFile(goFile, []byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"hello\") }\n"), 0644)

	pyFile := filepath.Join(tmpDir, "app.py")
	os.WriteFile(pyFile, []byte("import os\ndef hello():\n    print('hello')\n"), 0644)

	idx := indexer.NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatal(err)
	}

	// 验证索引
	index := idx.GetIndex()
	if index.TotalFiles != 2 {
		t.Errorf("应有 2 个文件, got %d", index.TotalFiles)
	}

	// 验证语言检测
	if index.Languages["go"] != 1 {
		t.Error("应有 1 个 Go 文件")
	}
	if index.Languages["python"] != 1 {
		t.Error("应有 1 个 Python 文件")
	}

	// 验证文件树
	tree := idx.GetFileTree()
	if !strings.Contains(tree, "main.go") {
		t.Error("文件树应包含 main.go")
	}
	if !strings.Contains(tree, "app.py") {
		t.Error("文件树应包含 app.py")
	}

	// 验证搜索
	results := idx.SearchFiles("main")
	if len(results) != 1 {
		t.Errorf("搜索 main 应返回 1 个结果, got %d", len(results))
	}
}
