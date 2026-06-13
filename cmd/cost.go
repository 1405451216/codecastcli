package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"codecast/cli/internal/cost"
	"codecast/cli/internal/ui"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootCostCmd 是 `codecast cost` 根命令。
//
// 在 v0.2.0 中，cost 子命令已迁移到交互模式的 `/cost` 斜杠命令。
// 保留此根命令仅用于显示迁移提示。
var rootCostCmd = &cobra.Command{
	Use:   "cost",
	Short: "成本追踪管理（已迁移: 在交互模式中使用 /cost）",
	Long: `⚠️  codecast cost 子命令已在 v0.2.0 中迁移到交互模式。

请在交互模式（运行 codecast 后）中直接使用 /cost 斜杠命令：

  /cost                   — 查看成本汇总
  /cost summary           — 查看成本汇总
  /cost daily [days]      — 查看每日成本统计（默认 7 天）
  /cost list [limit]      — 查看最近调用记录（默认 20 条）
  /cost clear             — 清空所有成本记录`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow("⚠️  `codecast cost` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		color.Cyan("请在交互模式中使用 /cost 斜杠命令：")
		color.White("  /cost                   — 查看成本汇总")
		color.White("  /cost daily [days]      — 查看每日成本统计")
		color.White("  /cost list [limit]      — 查看最近调用记录")
		color.White("  /cost clear             — 清空所有成本记录")
		fmt.Println()
		color.White("直接运行 `codecast` 进入交互模式，然后输入 /cost 即可。")
	},
}

// ============== 可复用函数（供 /cost 斜杠命令使用） ==============

// costRunSummary 显示成本汇总
func costRunSummary(jsonOut bool) error {
	tracker, err := cost.NewTracker()
	if err != nil {
		return err
	}
	defer tracker.Close()

	summary, err := tracker.Summary()
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	}

	color.Cyan("📊 成本汇总")
	fmt.Printf("  总调用次数: %d\n", summary.CallCount)
	fmt.Printf("  总 Token 数: %d (输入 %d / 输出 %d)\n",
		summary.TotalTokens, summary.TotalPromptTokens, summary.TotalCompTokens)
	fmt.Printf("  总费用: $%.4f (≈ ¥%.4f)\n", summary.TotalCostUSD, summary.TotalCostCNY)
	fmt.Println()

	if len(summary.ByModel) > 0 {
		color.Yellow("📈 按模型统计:")
		fmt.Printf("  %-30s %-10s %-8s %-12s %-12s\n", "模型", "调用次数", "Token", "费用(USD)", "费用(CNY)")
		fmt.Println(strings.Repeat("-", 80))
		for _, m := range summary.ByModel {
			fmt.Printf("  %-30s %-10d %-8d $%-11.4f ¥%-11.4f\n",
				m.Model, m.Calls, m.Tokens, m.CostUSD, m.CostCNY)
		}
	}
	return nil
}

// costRunDaily 显示每日成本统计
func costRunDaily(days int, jsonOut bool) error {
	tracker, err := cost.NewTracker()
	if err != nil {
		return err
	}
	defer tracker.Close()

	daily, err := tracker.DailySummary(days)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(daily)
	}

	color.Cyan("📅 最近 %d 天成本统计", days)
	fmt.Printf("  %-12s %-8s %-12s %-12s\n", "日期", "调用", "费用(USD)", "费用(CNY)")
	fmt.Println(strings.Repeat("-", 50))
	for _, d := range daily {
		fmt.Printf("  %-12s %-8d $%-11.4f ¥%-11.4f\n", d.Day, d.Calls, d.CostUSD, d.CostCNY)
	}
	return nil
}

// costRunList 显示最近调用记录
func costRunList(limit int, jsonOut bool) error {
	tracker, err := cost.NewTracker()
	if err != nil {
		return err
	}
	defer tracker.Close()

	records, err := tracker.RecentRecords(limit)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(records)
	}

	color.Cyan("📝 最近 %d 条调用记录", len(records))
	fmt.Printf("  %-5s %-20s %-10s %-8s %-8s %-10s %-10s %-19s\n",
		"ID", "模型", "Provider", "输入", "输出", "费用(USD)", "费用(CNY)", "时间")
	fmt.Println(strings.Repeat("-", 100))
	for _, r := range records {
		model := r.Model
		if len(model) > 20 {
			model = model[:17] + "..."
		}
		fmt.Printf("  %-5d %-20s %-10s %-8d %-8d $%-9.4f ¥%-9.4f %s\n",
			r.ID, model, r.Provider, r.PromptTokens, r.CompletionTokens,
			r.CostUSD, r.CostCNY, r.Timestamp.Format("2006-01-02 15:04"))
	}
	return nil
}

// costRunClear 清空所有成本记录
func costRunClear() error {
	tracker, err := cost.NewTracker()
	if err != nil {
		return err
	}
	defer tracker.Close()

	if err := tracker.Clear(); err != nil {
		return err
	}
	ui.PrintSuccess("已清空所有成本记录")
	return nil
}

// costParseDaysArg 解析 days 参数（默认 7）
func costParseDaysArg(s string) int {
	if d, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && d > 0 {
		return d
	}
	return 7
}

// costParseLimitArg 解析 limit 参数（默认 20）
func costParseLimitArg(s string) int {
	if l, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && l > 0 {
		return l
	}
	return 20
}

func init() {
	// 仅保留 rootCostCmd，不再注册子命令
	rootCmd.AddCommand(rootCostCmd)
}
