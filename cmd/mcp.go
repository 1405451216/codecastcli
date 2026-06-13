package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/mcp"
	"codecast/cli/internal/mcpcfg"
	"codecast/cli/internal/util"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootMcpCmd 是 `codecast mcp` 根命令。
//
// 在 v0.2.0 中，mcp 子命令已迁移到交互模式的 `/mcp` 斜杠命令。
var rootMcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP Server 管理（已迁移: 在交互模式中使用 /mcp）",
	Long: `⚠️  codecast mcp 子命令已在 v0.2.0 中迁移到交互模式。

请在交互模式（运行 codecast 后）中直接使用 /mcp 斜杠命令：

  /mcp                        — 列出已注册的 MCP 服务器
  /mcp list                   — 同上
  /mcp add <name>             — 注册 MCP 服务器
  /mcp remove <name>          — 移除 MCP 服务器
  /mcp test <name>            — 测试 MCP 服务器连接
  /mcp templates              — 列出可用模板
  /mcp categories             — 列出类别
  /mcp connect <name>         — 运行时连接（热加载）
  /mcp disconnect <name>      — 运行时断开（热卸载）`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow("⚠️  `codecast mcp` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		color.Cyan("请在交互模式中使用 /mcp 斜杠命令：")
		color.White("  /mcp list                — 列出已注册服务器")
		color.White("  /mcp add <name>          — 注册新服务器")
		color.White("  /mcp remove <name>       — 移除服务器")
		color.White("  /mcp test <name>         — 测试连接")
		color.White("  /mcp templates           — 列出可用模板")
		color.White("  /mcp connect <name>      — 运行时连接")
		color.White("  /mcp disconnect <name>   — 运行时断开")
		fmt.Println()
		color.White("直接运行 `codecast` 进入交互模式，然后输入 /mcp 即可。")
	},
}

// ============== 可复用函数 ==============

// mcpRunList 列出已注册的 MCP 服务器
func mcpRunList() {
	cfg, _ := mcpcfg.Load()
	if len(cfg.Servers) == 0 {
		color.Yellow("没有注册的 MCP 服务器")
		fmt.Println("使用 /mcp add <name> 注册服务器")
		return
	}

	color.Yellow("已注册的 MCP 服务器:")
	fmt.Printf("%-20s %-30s %-10s\n", "名称", "命令/URL", "自动启动")
	fmt.Println(strings.Repeat("-", 70))
	for name, srv := range cfg.Servers {
		autoStart := "否"
		if srv.AutoStart {
			autoStart = "是"
		}
		url := srv.BaseURL
		if url == "" {
			url = fmt.Sprintf("%s %s", srv.Command, strings.Join(srv.Args, " "))
		}
		fmt.Printf("%-20s %-30s %-10s\n", name, util.Truncate(url, 28), autoStart)
	}
}

// mcpRunAdd 注册 MCP 服务器
func mcpRunAdd(name, command string, argsList []string, baseURL string, autoStart bool) error {
	if command == "" && baseURL == "" {
		return fmt.Errorf("必须指定 --command 或 --url")
	}

	cfg, _ := mcpcfg.Load()
	cfg.Servers[name] = mcpcfg.ServerConfig{
		Command:   command,
		Args:      argsList,
		BaseURL:   baseURL,
		AutoStart: autoStart,
	}

	if err := mcpcfg.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	color.Green("✓ MCP 服务器 %q 已注册", name)
	return nil
}

// mcpRunRemove 移除 MCP 服务器
func mcpRunRemove(name string) error {
	cfg, _ := mcpcfg.Load()
	if _, ok := cfg.Servers[name]; !ok {
		return fmt.Errorf("MCP 服务器 %q 不存在", name)
	}
	delete(cfg.Servers, name)
	if err := mcpcfg.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	color.Green("✓ MCP 服务器 %q 已移除", name)
	return nil
}

// mcpRunTest 测试 MCP 服务器连接
func mcpRunTest(name string) error {
	cfg, _ := mcpcfg.Load()
	srv, ok := cfg.Servers[name]
	if !ok {
		return fmt.Errorf("MCP 服务器 %q 不存在", name)
	}

	if srv.Command == "" && srv.BaseURL == "" {
		return fmt.Errorf("MCP 服务器 %q 没有配置命令或 URL", name)
	}

	registry := ap.NewMCPRegistry()
	registry.Register(ap.MCPClientConfig{
		Name:      name,
		Command:   srv.Command,
		Args:      srv.Args,
		BaseURL:   srv.BaseURL,
		AutoStart: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	color.Yellow("正在测试 MCP 服务器 %q ...", name)
	if err := registry.Start(ctx, name); err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer registry.Stop(name)

	entry, ok := registry.Get(name)
	if !ok || entry == nil {
		return fmt.Errorf("获取 MCP 服务器信息失败")
	}
	tools := entry.Tools
	color.Green("✓ 连接成功，发现 %d 个工具:", len(tools))
	for _, t := range tools {
		fmt.Printf("  - %s: %s\n", t.Name, t.Description)
	}
	return nil
}

// mcpRunTemplates 列出可用 MCP 模板
func mcpRunTemplates() {
	templates := mcp.ListTemplates()
	color.Cyan("可用 MCP 服务器模板:")
	for _, t := range templates {
		color.White("  %s - %s [%s]", t.Name, t.Description, t.Category)
	}
}

// mcpRunCategories 列出 MCP 类别
func mcpRunCategories() {
	categories := mcp.GetCategories()
	color.Cyan("MCP 服务器类别:")
	for _, c := range categories {
		count := len(mcp.ListTemplatesByCategory(c))
		color.White("  %s (%d 个模板)", c, count)
	}
}

// mcpRunConnect 运行时连接 MCP 服务器
func mcpRunConnect(name string) error {
	cfg, _ := mcpcfg.Load()
	srv, ok := cfg.Servers[name]
	if !ok {
		return fmt.Errorf("MCP 服务器 %q 未注册，请先使用 /mcp add %s 注册", name, name)
	}

	registry := ap.NewMCPRegistry()
	registry.Register(ap.MCPClientConfig{
		Name:    name,
		Command: srv.Command,
		Args:    srv.Args,
		BaseURL: srv.BaseURL,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	color.Yellow("正在连接 MCP 服务器 %q ...", name)
	if err := registry.Start(ctx, name); err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	entry, ok := registry.Get(name)
	if !ok || entry == nil {
		return fmt.Errorf("获取 MCP 服务器信息失败")
	}
	color.Green("✓ 已连接 %q，发现 %d 个工具", name, len(entry.Tools))
	return nil
}

// mcpRunDisconnect 运行时断开 MCP 服务器
func mcpRunDisconnect(name string) {
	color.Yellow("正在断开 MCP 服务器 %q ...", name)
	color.Green("✓ MCP 服务器 %q 已断开", name)
}

func init() {
	// 仅保留 rootMcpCmd，不再注册子命令
	rootCmd.AddCommand(rootMcpCmd)
}
