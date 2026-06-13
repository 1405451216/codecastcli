package cmd

// feedback.go: /fb 斜杠命令 — 主动反馈 A/B 评估闭环。
//
// 用法：
//   /fb y   — 上一轮输出有用，success=true
//   /fb n   — 上一轮输出有问题，success=false
//   /fb show — 显示当前未判定轮
//   /fb help — 帮助
//
// 为什么不阻塞 stdin 问 y/n：
//   go-prompt 在流式输出后会切回 cooked mode，但 /fb 触发时机在 prompt 阶段，
//   阻塞读 stdin 会卡死 REPL。改用显式斜杠命令更稳。
//
// 与 /undo 的关系：
//   /undo 撤销文件修改 → ag.GetABIntegration().ResolveSuccess(false)
//   /fb n       → ag.GetABIntegration().ResolveSuccess(false)
//   /fb y       → ag.GetABIntegration().ResolveSuccess(true)

import (
	"fmt"
	"strings"

	"codecast/cli/internal/agent"

	"github.com/fatih/color"
)

func handleFbCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" || args == "help" || args == "-h" {
		printFbHelp()
		return
	}

	ab := abOf(ag)
	if ab == nil {
		color.Yellow("A/B 收敛器未初始化（可能 agent 未运行）")
		return
	}

	switch args {
	case "y", "yes", "ok":
		if ab.ResolveSuccess(true) {
			color.Green("✓ 上一轮已记录为 success")
		} else {
			color.Yellow("当前没有待判定的轮（可能已成功结束或已反馈）")
		}
	case "n", "no", "bad":
		if ab.ResolveSuccess(false) {
			color.Green("✓ 上一轮已记录为 fail")
		} else {
			color.Yellow("当前没有待判定的轮（可能已成功结束或已反馈）")
		}
	case "show":
		printFbShow(ab)
	case "enable":
		ab.SetEnabled(true)
		color.Green("✓ A/B 收敛器已启用")
	case "disable":
		ab.SetEnabled(false)
		color.Yellow("✓ A/B 收敛器已禁用（不再记录 outcome）")
	default:
		color.Yellow("未知子命令: %s", args)
		printFbHelp()
	}
}

// printFbShow 显示当前轮的元数据。
func printFbShow(ab *agent.ABIntegration) {
	// 简单展示：让用户知道系统状态
	if !ab.Enabled() {
		color.Yellow("A/B 收敛器：禁用")
		return
	}
	c := ab.Converger()
	if c == nil {
		color.Yellow("A/B 收敛器：未初始化")
		return
	}
	cfg := c.Config()
	color.Cyan("A/B 收敛器")
	fmt.Printf("  epsilon       = %.2f\n", cfg.Epsilon)
	fmt.Printf("  min_samples   = %d\n", cfg.MinSamples)
	fmt.Printf("  state_path    = %s\n", cfg.StatePath)
}

func printFbHelp() {
	color.Cyan("📝 /fb — A/B 反馈命令")
	fmt.Println()
	color.White("用法:")
	color.White("  /fb y          上一轮输出有效（success=true）")
	color.White("  /fb n          上一轮输出有问题（success=false）")
	color.White("  /fb show       显示收敛器状态")
	color.White("  /fb enable     启用收敛器")
	color.White("  /fb disable    禁用收敛器（不再记录 outcome）")
	color.White("  /fb help       显示本帮助")
	fmt.Println()
	color.White("提示:")
	color.White("  - /undo 撤销文件修改会自动记为 fail")
	color.White("  - LLM 成功完成默认记为 success")
	color.White("  - 显式 /fb n 会修正误判")
}

// abOf 提取 agent 里的 ABIntegration 句柄。
// 单独函数便于 /ab 斜杠命令复用。
func abOf(ag *agent.CodecastAgent) *agent.ABIntegration {
	if ag == nil {
		return nil
	}
	return ag.GetABIntegration()
}
