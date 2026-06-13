package cmd

import (
	"context"
	"fmt"

	"codecast/cli/internal/config"
	"codecast/cli/internal/pool"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootPoolCmd 是 `codecast pool` 根命令。
//
// 在 v0.2.0 中，pool 子命令已迁移到交互模式的 `/pool` 斜杠命令。
var rootPoolCmd = &cobra.Command{
	Use:   "pool",
	Short: "管理 Agent Pool（已迁移: 在交互模式中使用 /pool）",
	Long: `⚠️  codecast pool 子命令已在 v0.2.0 中迁移到交互模式。

请在交互模式（运行 codecast 后）中直接使用 /pool 斜杠命令：

  /pool              — 查看 Pool 状态
  /pool status       — 同上
  /pool run <task>   — 提交并行任务`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow("⚠️  `codecast pool` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		color.Cyan("请在交互模式中使用 /pool 斜杠命令：")
		color.White("  /pool              — 查看 Pool 状态")
		color.White("  /pool run <task>   — 提交并行任务")
		fmt.Println()
		color.White("直接运行 `codecast` 进入交互模式，然后输入 /pool 即可。")
	},
}

var poolConcurrency int

// ============== 可复用函数 ==============

// poolRunStatus 显示 Pool 状态
func poolRunStatus() {
	color.Cyan("Agent Pool 状态:")
	color.White("  最大并发: %d", poolConcurrency)
}

// poolRunRun 提交并行任务
func poolRunRun(tasks []string) error {
	cfg := config.Load()
	mgr, err := pool.NewManager(cfg, poolConcurrency)
	if err != nil {
		return err
	}
	defer mgr.Close()

	// 转换为 TaskDefinition
	taskDefs := make([]pool.TaskDefinition, len(tasks))
	for i, t := range tasks {
		taskDefs[i] = pool.TaskDefinition{
			ID:    fmt.Sprintf("task-%d", i+1),
			Title: fmt.Sprintf("Task %d", i+1),
			Prompt: t,
		}
	}

	color.Yellow("正在提交 %d 个并行任务...", len(taskDefs))
	if _, err := mgr.DispatchTasks(context.Background(), taskDefs); err != nil {
		return err
	}
	color.Green("✓ 任务已提交")
	return nil
}

func init() {
	// 仅保留 rootPoolCmd，不再注册子命令
	rootCmd.AddCommand(rootPoolCmd)
	rootPoolCmd.Flags().IntVar(&poolConcurrency, "concurrency", 5, "最大并发数")
}
