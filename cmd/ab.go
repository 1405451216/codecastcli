package cmd

// ab.go: /ab 斜杠命令 — A/B 自动收敛管理。
//
// 子命令：
//   /ab                  显示收敛状态报告
//   /ab enable           开启自动收敛（写入 ~/.codecast/config.yaml）
//   /ab disable          关闭自动收敛
//   /ab reset            清空所有历史（慎用）
//   /ab suggest          显示建议的下一个变体 + 当前权重
//   /ab apply            把建议权重写入 config.yaml（需用户确认）
//   /ab epsilon <0-1>    设置探索率
//   /ab help             显示帮助

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codecast/cli/internal/ab"
	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"
	"codecast/cli/internal/promptab"

	"github.com/fatih/color"
)

const abStateFileName = "ab_state.json"

func abStatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codecast", abStateFileName)
}

// handleAbCommand 处理 /ab 斜杠命令
func handleAbCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	// 列出可用变体
	available := availableVariantNames()

	if args == "" {
		printAbReport(available)
		return
	}
	fields := strings.Fields(args)
	sub := fields[0]
	switch sub {
	case "enable":
		abEnable()
	case "disable":
		abDisable()
	case "reset":
		abReset()
	case "suggest":
		abSuggest(available)
	case "apply":
		abApply(available, ag)
	case "export":
		abExportPath := ""
		if len(fields) >= 2 {
			abExportPath = fields[1]
		}
		handleAbExport(abExportPath, ag)
	case "epsilon":
		if len(fields) < 2 {
			color.Yellow("用法: /ab epsilon <0-1>")
			return
		}
		abSetEpsilon(fields[1])
	case "help", "-h", "--help":
		printAbHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printAbHelp()
	}
}

func printAbHelp() {
	color.Cyan("🎯 /ab — A/B 自动收敛管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /ab                       显示当前收敛状态 + 推荐")
	color.White("  /ab enable                开启自动收敛（写入 config）")
	color.White("  /ab disable               关闭自动收敛")
	color.White("  /ab reset                 清空所有历史（慎用）")
	color.White("  /ab suggest               显示建议的下一个变体 + 当前权重")
	color.White("  /ab apply                 把建议权重写入 config.yaml（持久化）")
	color.White("  /ab export [path.html]    导出 HTML 报告（默认 ~/.codecast/reports/）")
	color.White("  /ab epsilon <0-1>         设置探索率（0=纯利用, 1=纯探索）")
	color.White("  /ab help                  显示本帮助")
	fmt.Println()
	color.White("工作原理: epsilon-greedy + 冷启动期 + Wilson 95% CI")
	color.White("  - 冷启动期（每变体采样 < MinSamples）：均匀轮转")
	color.White("  - 之后：以 epsilon 概率探索，(1-epsilon) 概率用当前最优")
	color.White("  - 评分：1/avg_cost * (0.5 + success_rate*0.5)")
	color.White("  - 显著性：CI 不重叠才算'显著更好'，避免 n 小时误判")
}

func abEnable() {
	cfg := config.Load()
	cfg.PromptStrategy = "weighted"
	if err := config.Save(cfg); err != nil {
		color.Red("✗ 写入 config 失败: %v", err)
		return
	}
	color.Green("✓ 自动收敛已开启（strategy=weighted）")
	color.HiBlack("提示: 配合 prompt_weights 与 cost by-variant 看效果")
}

func abDisable() {
	cfg := config.Load()
	cfg.PromptStrategy = "fixed"
	if err := config.Save(cfg); err != nil {
		color.Red("✗ 写入 config 失败: %v", err)
		return
	}
	color.Green("✓ 已切回 fixed 策略（自动收敛关闭）")
}

func abReset() {
	c, err := ab.Load(abStatePath())
	if err != nil {
		color.Red("✗ 加载状态失败: %v", err)
		return
	}
	c.Reset()
	if err := c.Save(); err != nil {
		color.Red("✗ 保存状态失败: %v", err)
		return
	}
	color.Green("✓ 已清空所有 A/B 收敛历史")
}

func abSuggest(available []string) {
	c, err := ab.Load(abStatePath())
	if err != nil {
		color.Red("✗ 加载状态失败: %v", err)
		return
	}
	sug := c.Suggest(available)
	color.Cyan("推荐下一个变体: %s", sug.NextVariant)
	fmt.Println()
	fmt.Println("建议权重:")
	for _, name := range available {
		w := sug.Weights[name]
		fmt.Printf("  %-15s %d\n", name, w)
	}
}

func abApply(available []string, ag *agent.CodecastAgent) {
	c, err := ab.Load(abStatePath())
	if err != nil {
		color.Red("✗ 加载状态失败: %v", err)
		return
	}
	weights := c.ComputeWeights(available)
	cfg := config.Load()
	cfg.PromptStrategy = "weighted"
	cfg.PromptWeights = weights
	if err := config.Save(cfg); err != nil {
		color.Red("✗ 写入 config 失败: %v", err)
		return
	}
	color.Green("✓ 已把建议权重写入 ~/.codecast/config.yaml")
	for name, w := range weights {
		fmt.Printf("  %-15s %d\n", name, w)
	}
	// 立即让 agent 看到新权重（无需重启）
	if ag != nil {
		if err := ag.RefreshConfig(); err != nil {
			color.Yellow("⚠ 即时刷新失败: %v（下次启动会生效）", err)
		} else {
			color.HiBlack("→ 已应用新权重到当前会话")
		}
	}
}

func abSetEpsilon(s string) {
	var eps float64
	if _, err := fmt.Sscanf(s, "%f", &eps); err != nil {
		color.Red("✗ 解析失败: %v", err)
		return
	}
	if eps < 0 || eps > 1 {
		color.Red("✗ epsilon 必须在 [0, 1] 范围")
		return
	}
	c, err := ab.Load(abStatePath())
	if err != nil {
		color.Red("✗ 加载状态失败: %v", err)
		return
	}
	c.Config().Epsilon = eps
	if err := c.Save(); err != nil {
		color.Red("✗ 保存状态失败: %v", err)
		return
	}
	color.Green("✓ epsilon = %.2f", eps)
}

func printAbReport(available []string) {
	c, err := ab.Load(abStatePath())
	if err != nil {
		color.Red("✗ 加载状态失败: %v", err)
		return
	}
	fmt.Println(c.Report(available))
}

// availableVariantNames 返回当前所有可用变体名
func availableVariantNames() []string {
	vs := promptab.EmbeddedVariants()
	names := make([]string, 0, len(vs))
	for _, v := range vs {
		names = append(names, v.Name)
	}
	// 加用户/项目级 prompts 目录里的（如果存在）
	// 简化：此处只列嵌入变体
	return names
}
