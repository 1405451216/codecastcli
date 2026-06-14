package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/undo"
)

// newTestDeleteTool 构造一个使用临时 backup 目录的 DeleteFileTool，
// 这样测试不会污染 ~/.codecast/backups，且每个测试的备份相互隔离。
func newTestDeleteTool(t *testing.T) *DeleteFileTool {
	t.Helper()
	mgr := undo.NewManager(t.TempDir())
	return NewDeleteFileTool(mgr)
}

func runDelete(t *testing.T, tool *DeleteFileTool, filePath string) (*ap.ToolResult, error) {
	t.Helper()
	args, _ := json.Marshal(deleteFileParams{FilePath: filePath})
	return tool.Execute(context.Background(), args)
}

func TestDeleteFile_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "victim.txt")
	content := []byte("content to be deleted")
	if err := os.WriteFile(tmpFile, content, 0o644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := newTestDeleteTool(t)
	result, err := runDelete(t, tool, tmpFile)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}
	if !strings.Contains(result.Content, "已删除") {
		t.Errorf("结果缺少 '已删除': %s", result.Content)
	}

	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("文件应该已被删除，但 stat 仍可见: err=%v", err)
	}
}

func TestDeleteFile_BackupCreated(t *testing.T) {
	backupRoot := t.TempDir()
	mgr := undo.NewManager(backupRoot)
	tool := NewDeleteFileTool(mgr)

	tmpFile := filepath.Join(t.TempDir(), "backupme.txt")
	original := []byte("original payload")
	if err := os.WriteFile(tmpFile, original, 0o644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	if _, err := runDelete(t, tool, tmpFile); err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}

	// 检查 backupRoot/.codecast/backups 下应至少有一个 .bak 文件
	backups := mgr.ListBackups()
	if len(backups) == 0 {
		t.Fatalf("期望至少一个备份被创建")
	}

	// 找到 backupme 对应的备份，验证内容匹配
	var found bool
	for _, b := range backups {
		if !strings.Contains(filepath.Base(b.BackupPath), "backupme") {
			continue
		}
		data, err := os.ReadFile(b.BackupPath)
		if err != nil {
			t.Fatalf("读取备份失败: %v", err)
		}
		if string(data) != string(original) {
			t.Errorf("备份内容不匹配\n期望: %q\n得到: %q", original, data)
		}
		found = true
		break
	}
	if !found {
		t.Errorf("未找到 backupme 的备份；所有备份: %+v", backups)
	}
}

func TestDeleteFile_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	missing := filepath.Join(tmpDir, "does_not_exist.txt")

	tool := newTestDeleteTool(t)
	result, err := runDelete(t, tool, missing)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("期望错误结果（文件不存在），但得到成功: %s", result.Content)
	}
	if !strings.Contains(result.Content, "文件不存在") {
		t.Errorf("错误信息应包含 '文件不存在': %s", result.Content)
	}
}

func TestDeleteFile_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "subdir_to_delete")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}

	tool := newTestDeleteTool(t)
	result, err := runDelete(t, tool, target)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("期望错误结果（目标是目录），但得到成功: %s", result.Content)
	}
	if !strings.Contains(result.Content, "不支持删除目录") {
		t.Errorf("错误信息应说明不支持删除目录: %s", result.Content)
	}

	// 目录应仍然存在
	if _, err := os.Stat(target); err != nil {
		t.Errorf("目录不应被删除，但 stat 失败: %v", err)
	}
}

func TestDeleteFile_RejectsParentTraversal(t *testing.T) {
	tool := newTestDeleteTool(t)

	// 包含 ".." 段
	result, err := runDelete(t, tool, "../outside.txt")
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("期望错误结果（含 .. 的路径），但得到成功: %s", result.Content)
	}
	if !strings.Contains(result.Content, "路径不安全") {
		t.Errorf("错误信息应说明路径不安全: %s", result.Content)
	}

	// 嵌入式 .. 段
	result2, err := runDelete(t, tool, "subdir/../../escape.txt")
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result2.IsError {
		t.Errorf("期望错误结果（嵌入 .. 的路径），但得到成功: %s", result2.Content)
	}
}

func TestDeleteFile_EmptyFilePath(t *testing.T) {
	tool := newTestDeleteTool(t)
	result, err := runDelete(t, tool, "")
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("期望错误结果（空路径），但得到成功: %s", result.Content)
	}
}

func TestDeleteFile_ToolInterface(t *testing.T) {
	tool := NewDeleteFileTool(nil) // nil mgr 应回退到 cwd
	if tool.Name() != "delete_file" {
		t.Errorf("Name() = %q, 期望 %q", tool.Name(), "delete_file")
	}
	if tool.Description() == "" {
		t.Error("Description() 不应为空")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters() 不应为 nil")
	}
}

func TestDeleteFile_ImplementsToolInterface(t *testing.T) {
	// 验证 DeleteFileTool 实现了 ap.Tool 接口
	var _ ap.Tool = NewDeleteFileTool(nil)
}
