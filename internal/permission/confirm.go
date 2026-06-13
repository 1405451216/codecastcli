package permission

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// UserAction 用户在确认提示中的操作
type UserAction int

const (
	ActionAllow       UserAction = iota // 允许执行
	ActionDeny                          // 拒绝执行
	ActionAlwaysAllow                   // 始终允许（加入白名单）
	ActionEditArgs                      // 修改参数后重新提交
)

// ConfirmResult 确认结果
type ConfirmResult struct {
	Action       UserAction
	ModifiedArgs string // 仅 ActionEditArgs 时有效
}

// ConfirmPrompt 在终端显示确认提示并等待用户输入。
//
// F-03 设计说明：go-prompt 在调用 executor 回调期间会把终端切回 cooked mode
//（见 c-bata/go-prompt prompt.go 的 setUp/tearDown）。所以 bufio.NewReader
// 直接读 stdin 是安全的 —— 此时 go-prompt 不会争抢输入。
//
// 关键点：
//  1. 用 ANSI 颜色把 permission prompt 视觉上与普通提示区分，避免用户混淆
//  2. 用 \r\n 立即 flush 终端，确保用户看到提示再回答
//  3. **无超时**：用户可能在思考；不要自动 deny（F-11 路线要求全人工）
//  4. EOF / 错误时默认 deny（最保守）
func ConfirmPrompt(toolName, args string) ConfirmResult {
	// \033[1;33m = 亮黄粗体；\033[0m = 重置 — 明显区别于普通 cyan prompt
	const header = "\033[1;33m"
	const reset = "\033[0m"

	// 立即 flush，确保用户先看到提示
	out := os.Stdout
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s┌── ⚠ 权限请求 ─────────────────────────%s\n", header, reset)
	fmt.Fprintf(out, "%s│  工具: %s%s\n", header, toolName, reset)
	if args != "" {
		displayArgs := args
		if len(displayArgs) > 200 {
			displayArgs = displayArgs[:200] + "..."
		}
		fmt.Fprintf(out, "%s│  参数: %s%s\n", header, displayArgs, reset)
	}
	fmt.Fprintf(out, "%s└────────────────────────────────────────%s\n", header, reset)
	fmt.Fprintf(out, "%s允许执行? [y]es / [n]o / [a]lways / [e]dit > %s", header, reset)
	out.Sync() // 确保用户看到提示再开始读

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		// EOF 或 I/O 错误 — 保守地拒绝
		return ConfirmResult{Action: ActionDeny}
	}
	input = strings.TrimSpace(strings.ToLower(input))

	switch input {
	case "y", "yes":
		return ConfirmResult{Action: ActionAllow}
	case "n", "no", "":
		return ConfirmResult{Action: ActionDeny}
	case "a", "always", "always-allow":
		return ConfirmResult{Action: ActionAlwaysAllow}
	case "e", "edit", "edit-args":
		fmt.Fprintf(out, "%s请输入修改后的参数 (留空取消): %s", header, reset)
		out.Sync()
		modified, err := reader.ReadString('\n')
		if err != nil {
			return ConfirmResult{Action: ActionDeny}
		}
		modified = strings.TrimSpace(modified)
		if modified == "" {
			return ConfirmResult{Action: ActionDeny}
		}
		return ConfirmResult{Action: ActionEditArgs, ModifiedArgs: modified}
	default:
		return ConfirmResult{Action: ActionDeny}
	}
}

// HandleInterrupt 处理中断请求，在终端显示确认提示并返回响应
func HandleInterrupt(mgr *Manager, req *InterruptRequest) (*HumanResponse, bool) {
	toolName := ""
	args := ""
	if req.Data != nil {
		if t, ok := req.Data["tool"].(string); ok {
			toolName = t
		}
		if a, ok := req.Data["args"].(string); ok {
			args = a
		}
	}

	if mgr.IsDenied(toolName) {
		return &HumanResponse{Approved: false}, false
	}

	result := ConfirmPrompt(toolName, args)

	switch result.Action {
	case ActionAllow:
		return &HumanResponse{Approved: true}, false
	case ActionDeny:
		return &HumanResponse{Approved: false}, false
	case ActionAlwaysAllow:
		mgr.AddAutoAllow(toolName)
		return &HumanResponse{Approved: true}, true
	case ActionEditArgs:
		return &HumanResponse{
			Approved: true,
			Modified: map[string]any{"args": result.ModifiedArgs},
		}, false
	default:
		return &HumanResponse{Approved: false}, false
	}
}
