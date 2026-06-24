package ui

import (
	"bytes"
	"testing"
	"time"
)

func TestRenderMarkdown(t *testing.T) {
	// 测试基本 Markdown 渲染
	content := "# Hello\n\nThis is **bold** text."
	result := RenderMarkdown(content)
	
	// 渲染结果不应为空
	if result == "" {
		t.Error("RenderMarkdown returned empty string")
	}
	
	// 如果 glamour 初始化成功，结果应包含渲染后的内容
	// 如果失败，会回退到原始内容
	if result != content && len(result) < len(content) {
		t.Errorf("RenderMarkdown result seems truncated: got %q", result)
	}
}

func TestRenderMarkdownEmpty(t *testing.T) {
	// 测试空内容
	result := RenderMarkdown("")
	if result == "" {
		// 空输入应返回空或渲染后的空字符串
		return
	}
}

func TestSpinnerLifecycle(t *testing.T) {
	// 测试 Spinner 的创建、启动、停止
	var buf bytes.Buffer
	spinner := NewSpinner(&buf)
	
	if spinner == nil {
		t.Fatal("NewSpinner returned nil")
	}
	
	if spinner.IsActive() {
		t.Error("new spinner should not be active")
	}
	
	// 启动 spinner
	spinner.Start("Loading...")
	
	// 等待一小段时间让 goroutine 启动
	time.Sleep(10 * time.Millisecond)
	
	if !spinner.IsActive() {
		t.Error("spinner should be active after Start")
	}
	
	// 更新消息
	spinner.UpdateMessage("Still loading...")
	
	// 停止 spinner
	spinner.Stop()
	
	// 等待 goroutine 退出
	time.Sleep(10 * time.Millisecond)
	
	if spinner.IsActive() {
		t.Error("spinner should not be active after Stop")
	}
}

func TestSpinnerDoubleStart(t *testing.T) {
	// 测试重复启动不应 panic
	var buf bytes.Buffer
	spinner := NewSpinner(&buf)
	
	spinner.Start("First")
	time.Sleep(5 * time.Millisecond)
	
	// 第二次启动应该被忽略（内部检查）
	spinner.Start("Second")
	time.Sleep(5 * time.Millisecond)
	
	spinner.Stop()
}

func TestSpinnerDoubleStop(t *testing.T) {
	// 测试重复停止不应 panic
	var buf bytes.Buffer
	spinner := NewSpinner(&buf)
	
	spinner.Start("Test")
	time.Sleep(5 * time.Millisecond)
	
	spinner.Stop()
	// 第二次停止应该安全
	spinner.Stop()
}

func TestSpinnerStopWithoutStart(t *testing.T) {
	// 测试未启动就停止不应 panic
	var buf bytes.Buffer
	spinner := NewSpinner(&buf)
	
	// 不应该 panic
	spinner.Stop()
}

func TestGlobalSpinnerFunctions(t *testing.T) {
	// 测试全局 spinner 函数
	// 这些函数操作 defaultSpinner 全局变量
	
	// 清理函数：确保测试后重置状态
	defer func() {
		if defaultSpinner != nil {
			defaultSpinner.Stop()
			defaultSpinner = nil
		}
	}()
	
	// StartSpinner 应该创建并启动 spinner
	StartSpinner("Test message")
	
	if defaultSpinner == nil {
		t.Error("StartSpinner should create defaultSpinner")
	}
	
	if !defaultSpinner.IsActive() {
		t.Error("defaultSpinner should be active")
	}
	
	// UpdateSpinnerMessage 应该更新消息
	UpdateSpinnerMessage("Updated message")
	
	// StopSpinner 应该停止并清空
	StopSpinner()
	
	if defaultSpinner != nil {
		t.Error("StopSpinner should set defaultSpinner to nil")
	}
	
	// UpdateSpinnerMessage 在 nil 时不应 panic
	UpdateSpinnerMessage("This should not panic")
	
	// StopSpinner 在 nil 时不应 panic
	StopSpinner()
}
