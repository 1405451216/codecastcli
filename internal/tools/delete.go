// Package tools provides custom file-system tools for the agent.
//
// delete.go implements the delete_file tool, which removes a single file
// after backing it up via the undo manager so the operation can be undone.
// F-04 (delete_file tool): Task 1.1 of the IMPROVEMENT-PLAN — safe file
// deletion with pre-operation backup.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/undo"
	"codecast/cli/internal/util"
)

// DeleteFileTool 删除指定文件，操作前自动备份到 undo 管理器
type DeleteFileTool struct {
	undoMgr *undo.Manager
}

// NewDeleteFileTool 创建 DeleteFileTool 实例。
// 传入 nil 会回退到当前工作目录下的默认 undo 管理器，
// 方便测试和直接调用。
func NewDeleteFileTool(mgr *undo.Manager) *DeleteFileTool {
	if mgr == nil {
		dir, err := os.Getwd()
		if err != nil {
			dir = "."
		}
		mgr = undo.NewManager(dir)
	}
	return &DeleteFileTool{undoMgr: mgr}
}

// Name 返回工具名称
func (t *DeleteFileTool) Name() string {
	return "delete_file"
}

// Description 返回工具描述
func (t *DeleteFileTool) Description() string {
	return "删除指定文件，操作前自动备份到 undo 管理器。"
}

// deleteFileParams 定义 delete_file 工具的参数
type deleteFileParams struct {
	FilePath string `json:"file_path"`
}

// Parameters 返回工具参数的 JSON Schema
func (t *DeleteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": {
					"type": "string",
					"description": "要删除的文件路径"
				}
			},
			"required": ["file_path"]
		}`)
}

// Execute 执行 delete_file 工具
func (t *DeleteFileTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params deleteFileParams
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}

	if params.FilePath == "" {
		return ap.NewToolErrorResult("file_path 不能为空"), nil
	}

	// 路径安全：拒绝包含 ".." 的相对路径段，防止逃逸出项目根
	// 同时拒绝以分隔符开头的根路径（如 "/" 或 "C:\"）
	if util.HasUnsafePathSegment(params.FilePath) {
		return ap.NewToolErrorResult(fmt.Sprintf("路径不安全: %q 含 \"..\" 段或指向根目录", params.FilePath)), nil
	}

	// 检查文件状态
	info, err := os.Stat(params.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ap.NewToolErrorResult(fmt.Sprintf("文件不存在: %s", params.FilePath)), nil
		}
		return ap.NewToolErrorResult(fmt.Sprintf("获取文件信息失败: %v", err)), nil
	}

	// 不允许删除目录
	if info.IsDir() {
		return ap.NewToolErrorResult("delete_file 不支持删除目录，请用 shell 命令"), nil
	}

	// 备份（失败仅警告，仍尝试删除 — 与 buildUndoHook 行为一致）
	backupWarn := ""
	if t.undoMgr != nil {
		if berr := t.undoMgr.Backup(params.FilePath); berr != nil {
			backupWarn = fmt.Sprintf("警告: 备份失败，仍将删除: %v\n", berr)
		}
	}

	// 删除文件
	if err := os.Remove(params.FilePath); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("删除文件失败: %v", err)), nil
	}

	// 成功结果
	msg := fmt.Sprintf("已删除: %s (%d 字节)", params.FilePath, info.Size())
	if backupWarn != "" {
		msg = backupWarn + msg
	}
	return ap.NewToolResult(msg), nil
}
