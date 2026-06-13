package cmd

import (
	"fmt"

	"codecast/cli/internal/sandbox"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootSandboxCmd 是 `codecast sandbox` 根命令。
//
// 在 v0.2.0 中，sandbox 子命令已迁移到交互模式的 `/sandbox` 斜杠命令。
var rootSandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "沙箱隔离管理（已迁移: 在交互模式中使用 /sandbox）",
	Long: "⚠️  codecast sandbox 子命令已在 v0.2.0 中迁移到交互模式。\n" +
		"\n" +
		"请在交互模式（运行 codecast 后）中直接使用 /sandbox 斜杠命令：\n" +
		"\n" +
		"  /sandbox              - 查看沙箱状态\n" +
		"  /sandbox status       - 同上\n" +
		"  /sandbox build        - 构建沙箱镜像",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("⚠️  `codecast sandbox` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		fmt.Println("请在交互模式中使用 /sandbox 斜杠命令：")
		fmt.Println("  /sandbox              - 查看沙箱状态")
		fmt.Println("  /sandbox build        - 构建沙箱镜像")
		fmt.Println()
		fmt.Println("直接运行 `codecast` 进入交互模式，然后输入 /sandbox 即可。")
	},
}

// ============== 可复用函数 ==============

// sandboxRunStatus 查看沙箱状态
func sandboxRunStatus() {
	color.Cyan("沙箱状态:")
	if sandbox.IsDockerAvailable() {
		color.Green("  Docker: 可用")
	} else {
		color.Yellow("  Docker: 不可用（将使用直接执行模式）")
	}
}

// sandboxRunBuild 构建沙箱镜像
func sandboxRunBuild() error {
	color.Cyan("正在构建沙箱镜像...")
	if err := sandbox.BuildSandboxImage(nil); err != nil {
		color.Red("构建失败: %v", err)
		return err
	}
	color.Green("✓ 沙箱镜像构建成功")
	return nil
}

func init() {
	// 仅保留 rootSandboxCmd，不再注册子命令
	rootCmd.AddCommand(rootSandboxCmd)
}
