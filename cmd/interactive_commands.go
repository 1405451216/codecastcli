package cmd

// interactive_commands.go: 配置/成本/会话/插件/路由等命令处理器（从 interactive.go 拆分）
//
// 包含：/config, /cost, /session, /plugin, /route, /rag, /sandbox,
//
//	/workflow, /undo, /budget, /mcp, /stats 等处理器。

import (
	"fmt"
	"strings"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/git"

	"github.com/fatih/color"
)

// handleConfigCommand 处理 /config 斜杠命令
func handleConfigCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printConfigHelp()
		configList()
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printConfigHelp()
		return
	}
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}
	switch sub {
	case "list", "ls":
		configList()
	case "get":
		key := strings.TrimSpace(rest)
		if key == "" {
			color.Yellow("用法: /config get <key>")
			return
		}
		val, err := configGet(key)
		if err != nil {
			color.Red("%v", err)
			return
		}
		color.Cyan("%s = %s", key, val)
	case "set":
		kv := strings.SplitN(strings.TrimSpace(rest), " ", 2)
		if len(kv) != 2 {
			color.Yellow("用法: /config set <key> <value>")
			return
		}
		key := strings.TrimSpace(kv[0])
		value := kv[1]
		if err := configSet(key, value); err != nil {
			color.Red("%v", err)
			return
		}
		color.Green("✓ 已设置 %s", key)
	case "wizard":
		if err := configWizard(); err != nil {
			color.Red("%v", err)
		}
	case "providers":
		configProviders()
	case "init":
		if err := configInit(); err != nil {
			color.Red("%v", err)
		}
	case "help", "-h", "--help":
		printConfigHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printConfigHelp()
	}
}

func printConfigHelp() {
	color.Cyan("⚙️  /config — 配置管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /config                       查看帮助与当前配置")
	color.White("  /config list                  列出所有配置项")
	color.White("  /config get <key>             读取单个配置项")
	color.White("  /config set <key> <value>     设置单个配置项")
	color.White("  /config wizard                启动交互式配置向导")
	color.White("  /config providers             列出支持的 LLM Provider")
	color.White("  /config init                  初始化配置文件")
	fmt.Println()
	color.White("示例:")
	color.White("  /config set api_key sk-xxxx")
	color.White("  /config set provider openai")
	color.White("  /config set model gpt-4o")
	color.White("  /config get model")
	fmt.Println()
}

// handleCostCommand 处理 /cost 斜杠命令
func handleCostCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printCostHelp()
		if err := costRunSummary(false); err != nil {
			color.Red("查询失败: %v", err)
		}
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printCostHelp()
		return
	}
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}
	switch sub {
	case "summary", "sum":
		if err := costRunSummary(false); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "daily", "d":
		days := costParseDaysArg(rest)
		if err := costRunDaily(days, false); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "list", "ls":
		limit := costParseLimitArg(rest)
		if err := costRunList(limit, false); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "clear":
		if err := costRunClear(); err != nil {
			color.Red("清空失败: %v", err)
		}
	case "by-variant", "variant", "ab":
		if err := costRunByVariant(false); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "help", "-h":
		printCostHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printCostHelp()
	}
}

func printCostHelp() {
	color.Cyan("💰 /cost — 成本管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /cost                  查看成本汇总")
	color.White("  /cost daily [days]     查看每日成本（默认 7 天）")
	color.White("  /cost list [limit]     查看最近调用（默认 20 条）")
	color.White("  /cost by-variant       按 prompt 变体聚合（v0.3.0 A/B 分析）")
	color.White("  /cost clear            清空所有记录")
	fmt.Println()
}

// handleSessionCommand 处理 /session 斜杠命令
func handleSessionCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printSessionHelp()
		if err := sessionRunList(false); err != nil {
			color.Red("查询失败: %v", err)
		}
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printSessionHelp()
		return
	}
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}
	switch sub {
	case "list", "ls":
		if err := sessionRunList(false); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "show":
		if rest == "" {
			color.Yellow("用法: /session show <session-id>")
			return
		}
		if err := sessionRunShow(rest, false); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "delete", "rm":
		if rest == "" {
			color.Yellow("用法: /session delete <session-id>")
			return
		}
		if err := sessionRunDelete(rest); err != nil {
			color.Red("删除失败: %v", err)
		}
	case "export":
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			color.Yellow("用法: /session export <session-id> [output-file]")
			return
		}
		outputFile := ""
		if len(parts) >= 2 {
			outputFile = parts[1]
		}
		if err := sessionRunExport(parts[0], outputFile); err != nil {
			color.Red("导出失败: %v", err)
		}
	case "help", "-h":
		printSessionHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printSessionHelp()
	}
}

func printSessionHelp() {
	color.Cyan("💬 /session — 会话管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /session list                    列出所有会话")
	color.White("  /session show <id>               查看会话历史")
	color.White("  /session delete <id>             删除会话")
	color.White("  /session export <id> [file]      导出会话为 Markdown")
	fmt.Println()
}

// handlePluginCommand 处理 /plugin 斜杠命令
func handlePluginCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printPluginHelp()
		if err := pluginRunList(); err != nil {
			color.Red("查询失败: %v", err)
		}
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printPluginHelp()
		return
	}
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}
	switch sub {
	case "list", "ls":
		if err := pluginRunList(); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "install", "add":
		if rest == "" {
			color.Yellow("用法: /plugin install <name>")
			return
		}
		if err := pluginRunInstall(rest); err != nil {
			color.Red("安装失败: %v", err)
		}
	case "unload", "remove", "rm":
		if rest == "" {
			color.Yellow("用法: /plugin unload <name>")
			return
		}
		if err := pluginRunUnload(rest); err != nil {
			color.Red("卸载失败: %v", err)
		}
	case "available":
		pluginRunAvailable()
	case "help", "-h":
		printPluginHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printPluginHelp()
	}
}

func printPluginHelp() {
	color.Cyan("🧩 /plugin — 插件管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /plugin list                列出已安装的插件")
	color.White("  /plugin install <name>      安装插件")
	color.White("  /plugin unload <name>       卸载插件")
	color.White("  /plugin available           列出可用插件")
	fmt.Println()
}

// handleRouteCommand 处理 /route 命令 — 智能模型路由管理
func handleRouteCommand(args string, ag *agent.CodecastAgent) {
	router := ag.GetRouter()
	if router == nil {
		color.Red("路由器未初始化")
		return
	}
	args = strings.TrimSpace(args)
	if args == "" {
		cfg := router.Config()
		status := "禁用"
		if router.IsEnabled() {
			status = "启用"
		}
		color.Cyan("🔀 智能模型路由:")
		color.White("  状态:     %s", status)
		color.White("  简单模型: %s", cfg.SimpleModel)
		color.White("  中等模型: %s", cfg.MediumModel)
		color.White("  复杂模型: %s", cfg.ComplexModel)
		color.White("  当前模型: %s", ag.GetModelSwitcher().CurrentModel())
		fmt.Println()
		color.White("用法:")
		color.White("  /route          显示路由配置和状态")
		color.White("  /route on       启用智能路由")
		color.White("  /route off      禁用智能路由")
		color.White("  /route test <input>  测试输入的路由结果")
		return
	}
	fields := strings.Fields(args)
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}
	switch sub {
	case "on":
		router.SetEnabled(true)
		color.Green("✓ 智能路由已启用")
	case "off":
		router.SetEnabled(false)
		color.Yellow("✓ 智能路由已禁用")
	case "test":
		if rest == "" {
			color.Yellow("用法: /route test <input>")
			return
		}
		fileCount := countFileRefsForRoute(rest)
		routedModel := router.Route(rest, fileCount)
		currentModel := ag.GetModelSwitcher().CurrentModel()
		color.Cyan("路由测试结果:")
		color.White("  输入:     %s", truncateForDisplay(rest, 80))
		color.White("  文件引用: %d", fileCount)
		color.White("  当前模型: %s", currentModel)
		color.White("  路由模型: %s", routedModel)
		if routedModel != currentModel {
			color.Green("  → 会切换模型")
		} else {
			color.HiBlack("  → 无需切换")
		}
	default:
		color.Yellow("未知子命令: %s", sub)
		color.White("可用: /route [on|off|test <input>]")
	}
}

func countFileRefsForRoute(input string) int {
	count := 0
	inRef := false
	for i := 0; i < len(input); i++ {
		if input[i] == '@' && !inRef {
			inRef = true
			count++
		} else if input[i] == ' ' || input[i] == '\t' || input[i] == '\n' {
			inRef = false
		}
	}
	return count
}

func truncateForDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// handleRagCommand 处理 /rag 斜杠命令
func handleRagCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printRagHelp()
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printRagHelp()
		return
	}
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}
	switch sub {
	case "index":
		if rest == "" {
			color.Yellow("用法: /rag index <path>")
			return
		}
		if err := ragRunIndex(rest, false); err != nil {
			color.Red("索引失败: %v", err)
		}
	case "query", "q":
		if rest == "" {
			color.Yellow("用法: /rag query <query>")
			return
		}
		if err := ragRunQuery(rest, 3); err != nil {
			color.Red("查询失败: %v", err)
		}
	case "chat":
		if rest == "" {
			color.Yellow("用法: /rag chat <query>")
			return
		}
		if err := ragRunChat(rest); err != nil {
			color.Red("对话失败: %v", err)
		}
	case "help", "-h":
		printRagHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printRagHelp()
	}
}

func printRagHelp() {
	color.Cyan("📚 /rag — 知识库管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /rag index <path>        索引文档到知识库")
	color.White("  /rag query <query>       查询知识库")
	color.White("  /rag chat <query>        基于知识库对话")
	fmt.Println()
}

// handleSandboxCommand 处理 /sandbox 斜杠命令
func handleSandboxCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printSandboxHelp()
		sandboxRunStatus()
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printSandboxHelp()
		return
	}
	switch fields[0] {
	case "status":
		sandboxRunStatus()
	case "build":
		if err := sandboxRunBuild(); err != nil {
			color.Red("构建失败: %v", err)
		}
	case "help", "-h":
		printSandboxHelp()
	default:
		color.Yellow("未知子命令: %s", fields[0])
		printSandboxHelp()
	}
}

func printSandboxHelp() {
	color.Cyan("🏖️  /sandbox — 沙箱管理")
	fmt.Println()
	color.White("用法:")
	color.White("  /sandbox              查看沙箱状态")
	color.White("  /sandbox build        构建沙箱镜像")
	fmt.Println()
}

// handleWorkflowCommand 处理 /workflow 斜杠命令
func handleWorkflowCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printWorkflowHelp()
		return
	}
	fields := strings.Fields(args)
	if len(fields) < 2 {
		color.Yellow("用法: /workflow pipeline|parallel|handoff <prompt>")
		return
	}
	sub := fields[0]
	taskPrompt := strings.TrimSpace(strings.TrimPrefix(args, sub))
	switch sub {
	case "pipeline":
		if err := workflowRunPipeline(taskPrompt, "分析,开发,测试"); err != nil {
			color.Red("Pipeline 失败: %v", err)
		}
	case "parallel":
		if err := workflowRunParallel(taskPrompt, "审查1,审查2,审查3"); err != nil {
			color.Red("Parallel 失败: %v", err)
		}
	case "handoff":
		if err := workflowRunHandoff(taskPrompt, "分析,开发,测试"); err != nil {
			color.Red("Handoff 失败: %v", err)
		}
	case "help", "-h":
		printWorkflowHelp()
	default:
		color.Yellow("未知子命令: %s", sub)
		printWorkflowHelp()
	}
}

func printWorkflowHelp() {
	color.Cyan("🔄 /workflow — 多 Agent 工作流")
	fmt.Println()
	color.White("用法:")
	color.White("  /workflow pipeline <prompt>    Pipeline 顺序执行")
	color.White("  /workflow parallel <prompt>    Parallel 并行执行")
	color.White("  /workflow handoff <prompt>     Handoff 动态交接")
	fmt.Println()
}

// handleUndoCommand 处理 /undo 命令
func handleUndoCommand(args string, ag *agent.CodecastAgent) {
	undoMgr := ag.GetUndoManager()
	if undoMgr == nil {
		color.Red("Undo 管理器未初始化")
		return
	}
	var restoredPath string
	if args == "" {
		backups := undoMgr.ListBackups()
		if len(backups) == 0 {
			color.Yellow("没有可撤销的文件修改")
			return
		}
		mostRecent := backups[0]
		restored, err := ag.UndoLastFileChange(mostRecent.OriginalPath)
		if err != nil {
			color.Red("撤销失败: %v", err)
			return
		}
		if !restored {
			color.Yellow("无法恢复 %s", mostRecent.OriginalPath)
			return
		}
		restoredPath = mostRecent.OriginalPath
		color.Green("✓ 已撤销 %s 的最近修改", restoredPath)
	} else {
		filePath := strings.TrimSpace(args)
		restored, err := ag.UndoLastFileChange(filePath)
		if err != nil {
			color.Red("撤销失败: %v", err)
			return
		}
		if !restored {
			color.Yellow("未找到 %s 的备份", filePath)
			return
		}
		restoredPath = filePath
		color.Green("✓ 已撤销 %s 的最近修改", restoredPath)
	}
	if restoredPath != "" && ag != nil {
		if ab := ag.GetABIntegration(); ab != nil {
			if ab.ResolveSuccess(false) {
				color.HiBlack("→ A/B: 上一轮已记为 fail（撤销联动）")
			}
		}
	}
}

// handleBudgetCommand 处理 /budget 命令
func handleBudgetCommand(args string, ag *agent.CodecastAgent) {
	ctrl := ag.GetBudgetController()
	if ctrl == nil {
		color.Yellow("预算控制器未配置")
		color.White("在配置文件中设置 daily_budget_usd 或 session_budget_usd 启用预算控制")
		return
	}
	status := ctrl.Check()
	if status == nil {
		color.Yellow("无法获取预算状态")
		return
	}
	color.Cyan("预算使用情况:")
	if status.DailyRemainingUSD > 0 || status.DailySpendUSD > 0 {
		color.White("  日花费:   $%.4f (剩余 $%.4f, %.0f%%)", status.DailySpendUSD, status.DailyRemainingUSD, status.DailyPercent*100)
	}
	if status.SessionRemainingUSD > 0 || status.SessionSpendUSD > 0 {
		color.White("  会话花费: $%.4f (剩余 $%.4f, %.0f%%)", status.SessionSpendUSD, status.SessionRemainingUSD, status.SessionPercent*100)
	}
	if status.IsOverBudget {
		color.Red("  ⚠ 预算已超限!")
	}
}

// handleMCPInteractiveCommand 处理 /mcp 交互命令
func handleMCPInteractiveCommand(args string, ag *agent.CodecastAgent) {
	subCmd := strings.TrimSpace(args)
	switch {
	case subCmd == "" || subCmd == "list":
		color.Cyan("MCP 服务器管理:")
		color.White("  /mcp list              - 列出已注册服务器")
		color.White("  /mcp connect <name>    - 连接服务器")
		color.White("  /mcp disconnect <name> - 断开服务器")
		color.White("  /mcp tools <name>      - 列出服务器工具")
	case strings.HasPrefix(subCmd, "connect "):
		name := strings.TrimSpace(strings.TrimPrefix(subCmd, "connect "))
		color.Yellow("正在连接 MCP 服务器 %s ...", name)
		color.White("提示: 使用 codecast mcp connect %s 进行完整连接", name)
	case strings.HasPrefix(subCmd, "disconnect "):
		name := strings.TrimSpace(strings.TrimPrefix(subCmd, "disconnect "))
		color.Yellow("正在断开 MCP 服务器 %s ...", name)
		color.Green("✓ MCP 服务器 %s 已断开", name)
	case strings.HasPrefix(subCmd, "tools "):
		name := strings.TrimSpace(strings.TrimPrefix(subCmd, "tools "))
		color.Yellow("MCP 服务器 %s 的工具:", name)
		color.White("  使用 codecast mcp test %s 查看工具列表", name)
	default:
		color.Yellow("未知 MCP 子命令: %s", subCmd)
	}
}

// handlePromptCommand 处理 /prompt 命令
// 注：实际实现在 cmd/prompt.go 中，此处不重复声明

// 注：git 相关引用通过 interactive_git.go 中的 collectBlames 提供
var _ = git.NewAnalyzer // 确保 git 包被引用
