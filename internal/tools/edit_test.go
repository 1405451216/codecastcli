package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ap "agentprimordia/pkg"
)

func TestEditFile_SuccessfulSingleReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	args, _ := json.Marshal(editFileParams{
		FilePath:   tmpFile,
		OldString:  "line2",
		NewString:  "replaced",
		ReplaceAll: false,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Errorf("期望成功，但得到错误: %s", result.Content)
	}

	data, _ := os.ReadFile(tmpFile)
	got := string(data)
	expected := "line1\nreplaced\nline3\n"
	if got != expected {
		t.Errorf("文件内容不匹配\n期望: %q\n得到: %q", expected, got)
	}
}

func TestEditFile_MultipleMatchesWithoutReplaceAll(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "foo bar foo baz foo"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	args, _ := json.Marshal(editFileParams{
		FilePath:   tmpFile,
		OldString:  "foo",
		NewString:  "qux",
		ReplaceAll: false,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望错误结果（多处匹配但未设置 replace_all），但得到成功")
	}
}

func TestEditFile_MultipleMatchesWithReplaceAll(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "foo bar foo baz foo"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	args, _ := json.Marshal(editFileParams{
		FilePath:   tmpFile,
		OldString:  "foo",
		NewString:  "qux",
		ReplaceAll: true,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Errorf("期望成功，但得到错误: %s", result.Content)
	}

	data, _ := os.ReadFile(tmpFile)
	got := string(data)
	expected := "qux bar qux baz qux"
	if got != expected {
		t.Errorf("文件内容不匹配\n期望: %q\n得到: %q", expected, got)
	}
}

func TestEditFile_NoMatchFound(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	args, _ := json.Marshal(editFileParams{
		FilePath:   tmpFile,
		OldString:  "not_found",
		NewString:  "replacement",
		ReplaceAll: false,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望错误结果（未找到匹配），但得到成功")
	}
}

func TestEditFile_EmptyOldString(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	args, _ := json.Marshal(editFileParams{
		FilePath:   tmpFile,
		OldString:  "",
		NewString:  "replacement",
		ReplaceAll: false,
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望错误结果（old_string 为空），但得到成功")
	}
}

func TestEditFile_ToolInterface(t *testing.T) {
	tool := NewEditFileTool()
	if tool.Name() != "edit_file" {
		t.Errorf("Name() = %q, 期望 %q", tool.Name(), "edit_file")
	}
	if tool.Description() == "" {
		t.Error("Description() 不应为空")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters() 不应为 nil")
	}
}

func TestEditFile_ImplementsToolInterface(t *testing.T) {
	// 验证 EditFileTool 实现了 ap.Tool 接口
	var _ ap.Tool = NewEditFileTool()
}
