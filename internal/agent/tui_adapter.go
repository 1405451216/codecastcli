package agent

import (
	"context"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/tui"
)

// TUIAdapter 将 CodecastAgent 适配为 tui.AgentRunner 接口。
// 它直接调用底层 CapabilityAgent.StreamRun，将 ap.StreamEvent 转换为 tui.StreamEvent，
// 绕过 StreamProcess（StreamProcess 写 stdout，不适合 TUI 场景）。
type TUIAdapter struct {
	agent *CodecastAgent
}

// NewTUIAdapter 创建 CodecastAgent 的 TUI 适配器
func NewTUIAdapter(ag *CodecastAgent) *TUIAdapter {
	return &TUIAdapter{agent: ag}
}

// StreamRun 实现 tui.AgentRunner 接口。
// 调用底层 CapabilityAgent.StreamRun，将 ap.StreamEvent 转换为 tui.StreamEvent。
func (a *TUIAdapter) StreamRun(ctx context.Context, userInput string) (<-chan tui.StreamEvent, error) {
	// 注入压缩上下文（与 StreamProcess 一致）
	injected := a.agent.InjectCompressedContext(userInput)
	msg := ap.UserMessage(injected)

	// 通过底层 CapabilityAgent.StreamRun 获取流式事件
	capAgent := a.agent.CapabilityAgent()
	streamCh, err := capAgent.StreamRun(ctx, msg)
	if err != nil {
		return nil, err
	}

	// 转换通道：ap.StreamEvent → tui.StreamEvent
	out := make(chan tui.StreamEvent, 64)
	go func() {
		defer close(out)
		for evt := range streamCh {
			out <- convertAPStreamEvent(evt)
		}
	}()

	return out, nil
}

// convertAPStreamEvent 将 ap.StreamEvent 转换为 tui.StreamEvent
func convertAPStreamEvent(evt ap.StreamEvent) tui.StreamEvent {
	switch evt.Type {
	case ap.StreamEventToken:
		return tui.StreamEvent{Type: "token", Content: evt.Content}
	case ap.StreamEventThought:
		return tui.StreamEvent{Type: "token", Content: evt.Content}
	case ap.StreamEventToolCall:
		name, detail := formatAPToolCall(evt)
		return tui.StreamEvent{Type: "tool_call", Content: name, Detail: detail}
	case ap.StreamEventToolResult:
		return tui.StreamEvent{Type: "tool_result", Content: evt.Content}
	case ap.StreamEventComplete:
		return tui.StreamEvent{Type: "complete"}
	case ap.StreamEventError:
		return tui.StreamEvent{Type: "error", Content: evt.Content}
	default:
		return tui.StreamEvent{Type: "token", Content: evt.Content}
	}
}

// formatAPToolCallFromEvent 从 ap.StreamEvent 中提取工具调用信息
func formatAPToolCall(evt ap.StreamEvent) (name, detail string) {
	if tc, ok := evt.Data.(*ap.ToolCall); ok && tc != nil {
		name = tc.Name
		detail = truncateToolArgs(tc.Args, 80)
		return
	}
	name = "unknown"
	detail = truncateToolArgs(evt.Content, 80)
	return
}

// truncateToolArgs 截断工具参数字符串
func truncateToolArgs(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// RunTUI 启动 Bubble Tea TUI 界面，使用 CodecastAgent 作为后端。
// 这是启动 TUI 模式的推荐入口，在 cmd/root.go 的 --tui 分支中调用。
func RunTUI(ag *CodecastAgent, model, permissionMode string) error {
	adapter := NewTUIAdapter(ag)
	return tui.RunWithAgentAndConfig(adapter, model, permissionMode)
}
