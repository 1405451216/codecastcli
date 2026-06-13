package cmd

import (
	"fmt"

	"codecast/cli/internal/rules"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化项目配置",
	Long: `在当前目录创建 .codecast/ 目录和 rules.md 模板文件。

创建的文件：
  .codecast/rules.md      - 项目规则（提交到版本控制）
  .codecast/rules.local.md - 本地规则（已添加到 .gitignore）
  .codecast/.gitignore     - 忽略本地规则文件`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	projectRoot := "."

	if err := rules.InitProject(projectRoot); err != nil {
		color.Red("初始化失败: %v", err)
		return err
	}

	color.Green("✓ 项目配置已初始化")
	fmt.Println()
	color.White("  创建的文件:")
	color.White("    .codecast/rules.md       - 项目规则")
	color.White("    .codecast/.gitignore      - 忽略本地规则")
	fmt.Println()
	color.White("  提示:")
	color.White("    编辑 .codecast/rules.md 添加项目规则")
	color.White("    使用 .codecast/rules.local.md 添加个人规则（不提交到版本控制）")
	return nil
}
