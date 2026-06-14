package agent

import (
	"context"
	"fmt"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/tui"
	"codecast/cli/internal/util"

	"github.com/fatih/color"
)

// StreamProcess 以流式方式处理用户输入，实时输出 token（DI-6: 使用 tui.StreamPrinter）
func (a *CodecastAgent) StreamProcess(ctx context.Context, userInput string) error {
	// F8: 预算检查
	if a.budgetCtrl != nil {
		status := a.budgetCtrl.Check()
		if status != nil && status.IsOverBudget {
			return fmt.Errorf("预算已超限: 日花费 $%.4f (上限 $%.2f), 会话花费 $%.4f (上限 $%.2f)",
				status.DailySpendUSD, status.DailySpendUSD+status.DailyRemainingUSD,
				status.SessionSpendUSD, status.SessionSpendUSD+status.SessionRemainingUSD)
		}
	}

	msg := ap.UserMessage(userInput)
	// 任务感知路由：每轮根据用户输入 + 工具诉求重选变体
	a.selectVariantForInput(userInput, false)
	if a.ab != nil {
		a.ab.StartRound(a.currentVariant)
	}
	streamCh, err := a.agent.StreamRun(ctx, msg)
	if err != nil {
		return fmt.Errorf("启动流式输出失败: %w\n💡 请检查: 1) 模型 (%s) 是否正确 2) Provider (%s) 是否可用 3) API Key 是否有效 4) 网络连接是否正常", err, a.config.Model, a.config.Provider)
	}

	fmt.Println()

	// 使用 TUI Spinner 和 StreamPrinter
	spinner := tui.NewSpinner("思考中...")
	spinner.Start()
	streamPrinter := tui.NewStreamPrinter()

	var lastResp *ap.Response
	var tokenBuf strings.Builder
	firstToken := true

	for evt := range streamCh {
		switch evt.Type {
		case ap.StreamEventToken:
			if firstToken {
				spinner.Stop()
				firstToken = false
			}
			// 使用 StreamPrinter 逐 token 输出
			streamPrinter.WriteToken(evt.Content)
			tokenBuf.WriteString(evt.Content)

		case ap.StreamEventThought:
			spinner.UpdateMessage("思考中...")

		case ap.StreamEventToolCall:
			spinner.Stop()
			toolInfo := formatToolCall(evt)
			color.Yellow("\n⚙ 调用工具: %s", toolInfo)
			spinner = tui.NewSpinner("执行工具...")
			spinner.Start()

		case ap.StreamEventToolResult:
			spinner.Stop()
			foldedResult := foldResult(evt.Content, 5, 2)
			color.Green("✓ %s", foldedResult)

		case ap.StreamEventComplete:
			spinner.Stop()
			// 用 TUI 渲染器重新渲染完整响应
			if tokenBuf.Len() > 0 {
				fmt.Print("\r\033[J") // 清除当前行及以下
				fmt.Print(a.renderer.RenderMarkdown(tokenBuf.String()))
			}
			fmt.Println()
			if resp, ok := evt.Data.(*ap.Response); ok && resp != nil {
				lastResp = resp
				statsLine := fmt.Sprintf("[完成] 轮数=%d, 工具调用=%d, Token=%s",
					resp.Metrics.TotalTurns, resp.Metrics.TotalTools, formatTokenCount(resp.Usage.TotalTokens))
				color.HiBlack(statsLine)
			}

		case ap.StreamEventError:
			spinner.Stop()
			color.Red("\n✗ %s", evt.Content)
		}
	}
	fmt.Println()

	// 记录成本
	if lastResp != nil {
		a.recordCost(lastResp.Usage, "stream")

		// F1: 智能上下文管理 — 检测是否需要自动压缩
		a.checkAutoCompact(lastResp.Usage)
	}

	// 自动学习
	if mem := a.GetAutoMemory(); mem != nil {
		go mem.LearnFromConversation(userInput, tokenBuf.String())
	}

	return nil
}

// checkAutoCompact 检测上下文使用比例，自动触发压缩（F1）
func (a *CodecastAgent) checkAutoCompact(usage ap.AgentUsage) {
	if !a.config.AutoCompact {
		return
	}
	ratio := a.config.AutoCompactRatio
	if ratio <= 0 {
		ratio = 0.8 // 默认 80% 触发
	}

	budget := a.GetTokenBudget()
	if budget == nil {
		return
	}

	contextWindow := budget.ContextWindow
	if contextWindow <= 0 {
		return
	}

	usageRatio := float64(usage.TotalTokens) / float64(contextWindow)
	if usageRatio >= ratio {
		tui.PrintWarning(fmt.Sprintf("上下文使用率 %.0f%%，自动压缩中...", usageRatio*100))
		a.ClearContext()
		tui.PrintSuccess("上下文已自动压缩")
	}
}

// formatToolCall 格式化工具调用信息
func formatToolCall(evt ap.StreamEvent) string {
	if tc, ok := evt.Data.(*ap.ToolCall); ok && tc != nil {
		return fmt.Sprintf("%s(%s)", tc.Name, util.Truncate(tc.Args, 80))
	}
	return evt.Content
}

// foldResult 折叠长结果：超过 maxLines 行时，只显示 firstN 和 lastN 行
func foldResult(content string, maxLines int, showLines int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}

	var sb strings.Builder
	for i := 0; i < showLines; i++ {
		sb.WriteString(lines[i])
		if i < showLines-1 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString(fmt.Sprintf("\n  ... (共 %d 行)", len(lines)))
	for i := len(lines) - showLines; i < len(lines); i++ {
		sb.WriteString("\n")
		sb.WriteString(lines[i])
	}
	return sb.String()
}

// formatTokenCount 格式化 Token 数量，添加千分位逗号
func formatTokenCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(c)
	}
	return result.String()
}
