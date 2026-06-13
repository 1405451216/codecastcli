package cmd

import (
	"fmt"

	"codecast/cli/internal/config"
	"codecast/cli/internal/plugin"

	ap "agentprimordia/pkg"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootPluginCmd 是 `codecast plugin` 根命令。
//
// 在 v0.2.0 中，plugin 子命令已迁移到交互模式的 `/plugin` 斜杠命令。
var rootPluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "管理社区插件（已迁移: 在交互模式中使用 /plugin）",
	Long: `⚠️  codecast plugin 子命令已在 v0.2.0 中迁移到交互模式。

请在交互模式（运行 codecast 后）中直接使用 /plugin 斜杠命令：

  /plugin                  — 列出已安装的插件
  /plugin list             — 同上
  /plugin install <name>   — 安装插件
  /plugin unload <name>    — 卸载插件
  /plugin available        — 列出可用插件`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow("⚠️  `codecast plugin` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		color.Cyan("请在交互模式中使用 /plugin 斜杠命令：")
		color.White("  /plugin list             — 列出已安装的插件")
		color.White("  /plugin install <name>   — 安装插件")
		color.White("  /plugin unload <name>    — 卸载插件")
		color.White("  /plugin available        — 列出可用插件")
		fmt.Println()
		color.White("直接运行 `codecast` 进入交互模式，然后输入 /plugin 即可。")
	},
}

var pluginInstallDir string

// getPluginsDir 返回插件安装目录
func getPluginsDir(cfg *config.Config) string {
	if pluginInstallDir != "" {
		return pluginInstallDir
	}
	return cfg.ProjectRoot
}

// ============== 可复用函数 ==============

// pluginRunList 列出已安装的插件
func pluginRunList() error {
	cfg := config.Load()
	pluginsDir := getPluginsDir(cfg)
	registry := ap.NewToolRegistry()
	mgr := plugin.NewManager(registry, pluginsDir)

	plugins := mgr.ListPlugins()
	if len(plugins) == 0 {
		color.Yellow("未安装任何插件")
		return nil
	}

	color.Cyan("已安装的插件:")
	for _, p := range plugins {
		fmt.Printf("  - %s (v%s) - %d 个工具\n", p.Name, p.Version, p.Count)
	}
	return nil
}

// pluginRunInstall 安装插件
func pluginRunInstall(name string) error {
	cfg := config.Load()
	pluginsDir := getPluginsDir(cfg)
	registry := ap.NewToolRegistry()
	mgr := plugin.NewManager(registry, pluginsDir)

	color.Yellow("正在安装插件 %q ...", name)
	if err := mgr.InstallPlugin(name); err != nil {
		return fmt.Errorf("安装失败: %w", err)
	}
	color.Green("✓ 插件 %q 安装成功", name)
	return nil
}

// pluginRunUnload 卸载插件
func pluginRunUnload(name string) error {
	cfg := config.Load()
	pluginsDir := getPluginsDir(cfg)
	registry := ap.NewToolRegistry()
	mgr := plugin.NewManager(registry, pluginsDir)

	if err := mgr.UnloadPlugin(name); err != nil {
		return fmt.Errorf("卸载失败: %w", err)
	}
	color.Green("✓ 插件 %q 已卸载", name)
	return nil
}

// pluginRunAvailable 列出可用插件
func pluginRunAvailable() {
	color.Cyan("可用插件列表:")
	// 这里可以扩展为远程搜索
	color.White("  使用 /plugin install <name> 安装插件")
}

func init() {
	// 仅保留 rootPluginCmd，不再注册子命令
	rootCmd.AddCommand(rootPluginCmd)
	rootPluginCmd.Flags().StringVar(&pluginInstallDir, "dir", "", "插件安装目录")
}
