package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRenderer(t *testing.T) {
	r := NewRenderer()
	if r == nil {
		t.Fatal("NewRenderer() 返回 nil")
	}
	if r.renderer == nil {
		t.Error("Renderer.renderer 不应为 nil")
	}
	if r.width <= 0 {
		t.Errorf("Renderer.width 应为正数, 得到 %d", r.width)
	}
}

func TestRenderMarkdown(t *testing.T) {
	r := NewRenderer()

	tests := []struct {
		name     string
		input    string
		contains string // 渲染结果应包含的子串
	}{
		{
			name:     "标题",
			input:    "# Hello",
			contains: "Hello",
		},
		{
			name:     "粗体",
			input:    "**bold**",
			contains: "bold",
		},
		{
			name:     "空字符串",
			input:    "",
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.RenderMarkdown(tt.input)
			if tt.input == "" {
				// 空输入可能返回空或仅空白
				return
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("RenderMarkdown(%q) 结果应包含 %q, 得到 %q", tt.input, tt.contains, result)
			}
		})
	}
}

func TestRenderCodeBlock(t *testing.T) {
	r := NewRenderer()
	code := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}"
	result := r.RenderCodeBlock(code, "go")
	if !strings.Contains(result, "package") {
		t.Errorf("RenderCodeBlock 渲染 Go 代码结果应包含 'package', 得到 %q", result)
	}
	if !strings.Contains(result, "main") {
		t.Errorf("RenderCodeBlock 渲染 Go 代码结果应包含 'main', 得到 %q", result)
	}
}

func TestRenderDiff(t *testing.T) {
	r := NewRenderer()

	diffText := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
-removed line
+added line
 context line`

	result := r.RenderDiff(diffText)

	// 验证所有行都被渲染（非空行）
	lines := strings.Split(result, "\n")
	if len(lines) < 6 {
		t.Errorf("RenderDiff 应输出多行, 得到 %d 行", len(lines))
	}

	// 验证包含关键内容
	if !strings.Contains(result, "added line") {
		t.Error("RenderDiff 结果应包含 'added line'")
	}
	if !strings.Contains(result, "removed line") {
		t.Error("RenderDiff 结果应包含 'removed line'")
	}
	if !strings.Contains(result, "@@") {
		t.Error("RenderDiff 结果应包含 hunk 标记 '@@'")
	}
}

func TestStreamPrinter_WriteToken(t *testing.T) {
	var buf bytes.Buffer
	sp := &StreamPrinter{
		renderer:   NewRenderer(),
		writer:     &buf,
		liveRender: false, // 禁用实时渲染以便测试
	}

	sp.WriteToken("hello")
	sp.WriteToken(" ")
	sp.WriteToken("world")

	sp.mu.Lock()
	got := sp.buffer.String()
	sp.mu.Unlock()

	if got != "hello world" {
		t.Errorf("WriteToken 后 buffer 应为 'hello world', 得到 %q", got)
	}
}

func TestStreamPrinter_Flush(t *testing.T) {
	var buf bytes.Buffer
	sp := &StreamPrinter{
		renderer:   NewRenderer(),
		writer:     &buf,
		liveRender: false,
	}

	sp.WriteToken("test content")
	result := sp.Flush()

	if result == "" {
		t.Error("Flush 应返回渲染后的内容")
	}
	if !strings.Contains(result, "test") {
		t.Errorf("Flush 结果应包含 'test', 得到 %q", result)
	}

	// Flush 后 buffer 应被清空
	sp.mu.Lock()
	empty := sp.buffer.String()
	sp.mu.Unlock()
	if empty != "" {
		t.Errorf("Flush 后 buffer 应为空, 得到 %q", empty)
	}
}

func TestStreamPrinter_FlushEmpty(t *testing.T) {
	var buf bytes.Buffer
	sp := &StreamPrinter{
		renderer:   NewRenderer(),
		writer:     &buf,
		liveRender: false,
	}

	result := sp.Flush()
	if result != "" {
		t.Errorf("空 buffer Flush 应返回空字符串, 得到 %q", result)
	}
}

func TestStreamPrinter_Reset(t *testing.T) {
	var buf bytes.Buffer
	sp := &StreamPrinter{
		renderer:   NewRenderer(),
		writer:     &buf,
		liveRender: false,
	}

	sp.WriteToken("some content")
	sp.Reset()

	sp.mu.Lock()
	got := sp.buffer.String()
	sp.mu.Unlock()
	if got != "" {
		t.Errorf("Reset 后 buffer 应为空, 得到 %q", got)
	}
}

func TestNewSpinner(t *testing.T) {
	s := NewSpinner("loading")
	if s == nil {
		t.Fatal("NewSpinner() 返回 nil")
	}
	if s.message != "loading" {
		t.Errorf("Spinner.message 应为 'loading', 得到 %q", s.message)
	}
	if s.active {
		t.Error("新创建的 Spinner 不应为 active")
	}
	if len(s.frames) == 0 {
		t.Error("Spinner.frames 不应为空")
	}
}

func TestSpinner_UpdateMessage(t *testing.T) {
	s := NewSpinner("initial")
	s.UpdateMessage("updated")
	if s.message != "updated" {
		t.Errorf("UpdateMessage 后 message 应为 'updated', 得到 %q", s.message)
	}
}

func TestStyles(t *testing.T) {
	// 验证 Styles 常量可以正常渲染，不会 panic
	tests := []struct {
		name  string
		style func() string
	}{
		{"Header", func() string { return Styles.Header.Render("test") }},
		{"SubHeader", func() string { return Styles.SubHeader.Render("test") }},
		{"Success", func() string { return Styles.Success.Render("test") }},
		{"Warning", func() string { return Styles.Warning.Render("test") }},
		{"Error", func() string { return Styles.Error.Render("test") }},
		{"Info", func() string { return Styles.Info.Render("test") }},
		{"Dim", func() string { return Styles.Dim.Render("test") }},
		{"Bold", func() string { return Styles.Bold.Render("test") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.style()
			if result == "" {
				t.Errorf("Styles.%s.Render 返回空字符串", tt.name)
			}
		})
	}
}

func TestPrintFunctions_NoPanic(t *testing.T) {
	// 验证 Print 函数不会 panic
	funcs := []struct {
		name string
		fn   func()
	}{
		{"PrintHeader", func() { PrintHeader("test header") }},
		{"PrintSuccess", func() { PrintSuccess("test success") }},
		{"PrintWarning", func() { PrintWarning("test warning") }},
		{"PrintError", func() { PrintError("test error") }},
		{"PrintInfo", func() { PrintInfo("test info") }},
		{"PrintDim", func() { PrintDim("test dim") }},
	}

	for _, tt := range funcs {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s panicked: %v", tt.name, r)
				}
			}()
			tt.fn()
		})
	}
}
