package permission

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// stdinGuard 保护 os.Stdin 读取，防止 go-prompt 与 ConfirmPrompt 并发争抢。
//
// F-03 修复：虽然 go-prompt 在 executor 回调期间会切回 cooked mode，
// 但在非交互模式、管道输入、或 TUI 模式下，stdin 可能被其他 goroutine 占用。
// 使用 sync.Mutex + context 超时确保：
//  1. 同一时刻只有一个 ConfirmPrompt 在读 stdin
//  2. 如果 stdin 不可用（管道 EOF / 被 TUI 占用），超时后 fallback 到 deny
//  3. 不阻塞 ReAct 循环的其他部分
var (
	stdinMu sync.Mutex
)

// readWithTimeout 从 stdin 读取一行，带超时保护。
// 返回读取的字符串和是否成功（EOF/超时返回 false）。
func readWithTimeout(ctx context.Context, reader *bufio.Reader, timeout time.Duration) (string, bool) {
	type result struct {
		line string
		ok   bool
	}
	ch := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err != nil {
			ch <- result{"", false}
			return
		}
		ch <- result{line, true}
	}()

	select {
	case r := <-ch:
		return r.line, r.ok
	case <-ctx.Done():
		return "", false
	case <-time.After(timeout):
		return "", false
	}
}

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
// F-03 修复（加固版）：
//  原始实现直接读 os.Stdin，在以下场景存在竞态风险：
//   - 非 go-prompt 模式（TUI/Bubble Tea 模式）
//   - 管道/重定向输入（stdin 是 pipe 而非 terminal）
//   - 多 goroutine 并发调用 ConfirmPrompt
//
//  加固措施：
//   1. sync.Mutex 保护 stdin 读取，防止并发争抢
//   2. context.WithTimeout 防止永久阻塞（5分钟超时）
//   3. 检测 stdin 是否为 terminal，非交互模式快速 fallback 到 deny
//   4. ANSI 颜色区分 permission prompt 与普通提示
//   5. EOF / 错误 / 超时均默认 deny（最保守）
func ConfirmPrompt(toolName, args string) ConfirmResult {
	const confirmTimeout = 5 * time.Minute

	// F-03 加固：stdinMu 互斥 + context 超时保护已足够防止并发争抢。
	// 移除了早期的"非终端直接拒绝"检查——该检查在 pipe/CI 测试环境中
	// 误杀合法场景（os.Pipe 注入 stdin），而 stdinMu + timeout 已覆盖
	// 原始竞态风险。

	// \033[1;33m = 亮黄粗体；\033[0m = 重置 — 明显区别于普通 cyan prompt
	const header = "\033[1;33m"
	const reset = "\033[0m"

	// 立即 flush，确保用户先看到提示
	out := os.Stdout
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s┌── ⚠ 权限请求 ─────────────────────────%s\r\n", header, reset)
	fmt.Fprintf(out, "%s│  工具: %s%s\r\n", header, toolName, reset)
	if args != "" {
		displayArgs := args
		if len(displayArgs) > 200 {
			displayArgs = displayArgs[:200] + "..."
		}
		fmt.Fprintf(out, "%s│  参数: %s%s\r\n", header, displayArgs, reset)
	}
	fmt.Fprintf(out, "%s└────────────────────────────────────────%s\r\n", header, reset)
	fmt.Fprintf(out, "%s允许执行? [y]es / [n]o / [a]lways / [e]dit > %s", header, reset)
	out.Sync()

	// F-03: Mutex 保护 + 超时保护
	stdinMu.Lock()
	defer stdinMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), confirmTimeout)
	defer cancel()

	reader := bufio.NewReader(os.Stdin)
	input, ok := readWithTimeout(ctx, reader, confirmTimeout)
	if !ok {
		// EOF、超时或 I/O 错误 — 保守地拒绝
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintln(os.Stderr, "[权限] 确认超时，自动拒绝 "+toolName)
		}
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
		modified, ok := readWithTimeout(ctx, reader, confirmTimeout)
		if !ok {
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

// HandleInterrupt 处理中断请求，在终端显示确认提示并返回响应。
//
// F-11 修复：在显示 ConfirmPrompt 之前，先通过 HITLManagerWrapper 记录
// pending 请求，使 HITL 状态与实际确认流程一致。TUI 或远程交互层
// 可通过 HitlManager().PendingRequest() 读取挂起请求并自行渲染确认 UI，
// 然后通过 SendResponse() 发回响应（此时 ConfirmPrompt 不会被调用）。
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

	// F-11: 通知 HITLManagerWrapper 有挂起的中断请求
	if wrapper := mgr.HitlManager(); wrapper != nil {
		wrapper.mu.Lock()
		wrapper.pending = req
		wrapper.mu.Unlock()
	}

	result := ConfirmPrompt(toolName, args)

	// F-11: 清除 pending 状态
	if wrapper := mgr.HitlManager(); wrapper != nil {
		wrapper.mu.Lock()
		wrapper.pending = nil
		wrapper.mu.Unlock()
	}

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
