package agent

// agent_hooks.go: Hook 函数（从 agent.go 拆分）
//
// Phase 5.2 拆分：将 Hook 构建函数从 agent.go 中分离，
// 使 agent.go 聚焦于 Agent 生命周期和核心方法。
//
// 包含：buildPermHook, applyToolPermissions, buildDiffPreviewHook,
//
//	buildUndoHook, buildCheckpointHook, jsonGetString

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/checkpoint"
	"codecast/cli/internal/diff"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/tui"
	"codecast/cli/internal/undo"
)

// buildPermHook 构建权限检查 Hook。
//
// F-11 修复：权限确认走 HITL 通道（InterruptRequest → HandleInterrupt → HumanResponse），
// 而非直接调用 ConfirmPrompt。这样 permission.Manager 的 HITLManagerWrapper
// 状态与实际确认流程一致，不再是无用代码。
//
// 流程：
//  1. IsDenied → 直接拒绝
//  2. ShouldApprove → 构造 InterruptRequest，调用 HandleInterrupt
//  3. HandleInterrupt 内部调用 ConfirmPrompt，返回 HumanResponse
//  4. 根据 HumanResponse 决定允许/拒绝/always-allow/edit-args
func buildPermHook(mgr *permission.Manager) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil {
			return nil
		}

		toolName := hctx.ToolCall.Name

		if mgr.IsDenied(toolName) {
			return fmt.Errorf("工具 %s 已被安全模式禁止", toolName)
		}

		if mgr.ShouldApprove(toolName) {
			// F-11: 通过 HITL 通道路由，而非直接调 ConfirmPrompt
			req := &permission.InterruptRequest{
				Reason: permission.InterruptToolConfirm,
				Data: map[string]any{
					"tool": toolName,
					"args": hctx.ToolCall.Args,
				},
			}
			resp, allowAlways := permission.HandleInterrupt(mgr, req)

			if !resp.Approved {
				return fmt.Errorf("用户拒绝执行工具 %s", toolName)
			}
			if allowAlways {
				// HandleInterrupt 内部已调 AddAutoAllow，无需重复
			}
			if resp.Modified != nil {
				if args, ok := resp.Modified["args"].(string); ok && args != "" {
					hctx.ToolCall.Args = args
				}
			}
		}

		return nil
	}
}

// applyToolPermissions 为注册表中的工具设置 AP 框架级别的 ToolPermission。
//
// 与 buildPermHook 形成双重保障：
//   - ToolPermission 在 executor.Execute() 内部执行（更底层）
//   - buildPermHook 在 ReAct 循环的 HookBeforeTool 阶段执行（更上层）
//
// F-11: ConfirmFunc 也走 HandleInterrupt，与 buildPermHook 保持一致。
func applyToolPermissions(registry *ap.ToolRegistry, mgr *permission.Manager) {
	for _, toolName := range registry.List() {
		if !mgr.ShouldApprove(toolName) {
			continue
		}
		perm := ap.ToolPermission{
			RequireConfirmation: true,
			ConfirmFunc: func(name string, args json.RawMessage) bool {
				req := &permission.InterruptRequest{
					Reason: permission.InterruptToolConfirm,
					Data: map[string]any{
						"tool": name,
						"args": string(args),
					},
				}
				resp, _ := permission.HandleInterrupt(mgr, req)
				return resp.Approved
			},
		}
		_ = registry.SetPermission(toolName, perm)
	}
}

// buildDiffPreviewHook 构建 Diff 预览 Hook
func buildDiffPreviewHook(prev *diff.Previewer) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil || prev == nil {
			return nil
		}

		toolName := hctx.ToolCall.Name
		switch toolName {
		case "edit_file", "write_file":
			var argsMap map[string]json.RawMessage
			if err := json.Unmarshal([]byte(hctx.ToolCall.Args), &argsMap); err != nil {
				return nil
			}
			filePath := jsonGetString(argsMap, "file_path")
			if filePath == "" {
				filePath = jsonGetString(argsMap, "path")
			}
			if filePath == "" {
				return nil
			}

			if toolName == "edit_file" {
				oldStr := jsonGetString(argsMap, "old_string")
				newStr := jsonGetString(argsMap, "new_string")
				if oldStr != "" {
					change := prev.PreviewEdit(filePath, oldStr, newStr)
					fmt.Println(tui.Styles.Warning.Render("即将修改文件: " + filePath))
					fmt.Println(tui.NewRenderer().RenderDiff(change.Diff))
				}
			} else if toolName == "write_file" {
				content := jsonGetString(argsMap, "content")
				_, err := os.Stat(filePath)
				exists := err == nil
				change := prev.PreviewWrite(filePath, content, exists)
				if exists {
					fmt.Println(tui.Styles.Warning.Render("即将覆盖文件: " + filePath))
				} else {
					fmt.Println(tui.Styles.Info.Render("即将创建文件: " + filePath))
				}
				fmt.Println(tui.NewRenderer().RenderDiff(change.Diff))
			}
		}

		return nil
	}
}

// jsonGetString 从 map[string]json.RawMessage 安全提取字符串值
func jsonGetString(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// buildUndoHook 构建 Undo 备份 Hook
func buildUndoHook(mgr *undo.Manager) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil || mgr == nil {
			return nil
		}
		toolName := hctx.ToolCall.Name
		if toolName == "edit_file" || toolName == "write_file" {
			var argsMap map[string]json.RawMessage
			if err := json.Unmarshal([]byte(hctx.ToolCall.Args), &argsMap); err != nil {
				return nil
			}
			filePath := jsonGetString(argsMap, "file_path")
			if filePath == "" {
				filePath = jsonGetString(argsMap, "path")
			}
			if filePath != "" {
				_ = mgr.Backup(filePath)
			}
		}
		return nil
	}
}

// buildCheckpointHook 构建 Git Checkpoint Hook
func buildCheckpointHook(mgr *checkpoint.Manager) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil || mgr == nil {
			return nil
		}
		toolName := hctx.ToolCall.Name
		if toolName != "edit_file" && toolName != "write_file" && toolName != "delete_file" {
			return nil
		}
		return mgr.AutoCheckpoint(toolName, hctx.ToolCall.Args)
	}
}
