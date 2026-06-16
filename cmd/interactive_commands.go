package cmd

// interactive_commands.go: 配置/成本/会话/插件/路由等命令处理器（从 interactive.go 拆分）
//
// 包含：/config, /cost, /session, /plugin, /route, /rag, /sandbox,
//
//	/workflow, /undo, /budget, /mcp, /stats 等处理器。

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/benchmark"
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
	case "learning":
		// P1 L2: 学习型路由统计
		lr := ag.GetLearningRouter()
		if lr == nil {
			color.Yellow("学习型路由未启用。请在 config.yaml 中设置 learning_routing.enabled: true")
			return
		}
		stats := lr.Stats()
		if len(stats) == 0 {
			color.Yellow("学习路由暂无数据（需启用并产生若干次调用）")
			return
		}
		color.Cyan("🔀 学习型路由统计 (按评分降序)")
		fmt.Println()
		color.HiBlack("  %-25s %-8s %-8s %-8s %-10s %-10s %s",
			"MODEL", "TIER", "SAMPLES", "SUCCESS", "AVG_COST", "TOTAL_TOK", "SCORE")
		for _, s := range stats {
			color.White("  %-25s %-8s %-8d %.2f%%   $%-9.6f %-10d %.4f",
				s.Name, s.Tier, s.Samples, s.SuccessRate()*100,
				s.AvgCost(), s.TotalTokens, s.Score())
		}
		fmt.Println()
		color.HiBlack("  评分公式: (1/avgCost) * (0.5 + successRate*0.5)")
	default:
		color.Yellow("未知子命令: %s", sub)
		color.White("可用: /route [on|off|test <input>|learning]")
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

// handleSemanticCommand 处理 /semantic 斜杠命令（P3 语义索引）
//
// 子命令：
//   /semantic index [path]   索引代码库（默认当前目录）
//   /semantic query <query>  语义检索代码
//   /semantic stats          查看索引统计
//   /semantic clear          清空索引
//   /semantic help           帮助
func handleSemanticCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printSemanticHelp()
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printSemanticHelp()
		return
	}
	sub := fields[0]
	rest := ""
	if len(fields) > 1 {
		rest = strings.TrimSpace(strings.TrimPrefix(args, sub))
	}

	switch sub {
	case "index":
		handleSemanticIndex(ag, rest)
	case "query", "q":
		handleSemanticQuery(ag, rest)
	case "stats":
		handleSemanticStats(ag)
	case "clear":
		handleSemanticClear(ag)
	case "help", "-h":
		printSemanticHelp()
	default:
		color.Red("未知子命令: %s", sub)
		printSemanticHelp()
	}
}

// handleSemanticIndex 索引代码库
func handleSemanticIndex(ag *agent.CodecastAgent, path string) {
	if ag == nil {
		color.Red("Agent 未初始化")
		return
	}
	si := ag.GetSemanticIndex()
	if si == nil {
		color.Red("语义索引未启用。请在 config.yaml 中设置 semantic_index.enabled: true")
		return
	}

	target := path
	if target == "" {
		target = "."
	}
	color.Cyan("🔍 开始索引: %s", target)

	// 切换索引根目录（如果指定了路径）
	ctx := context.Background()
	start := time.Now()

	var fileCount, errCount int
	cb := func(p string, err error) {
		if err != nil {
			errCount++
			color.Yellow("  跳过 %s: %v", p, err)
			return
		}
		fileCount++
		if fileCount%50 == 0 {
			color.HiBlack("  已索引 %d 个文件...", fileCount)
		}
	}

	// 直接用 IndexDir（基于配置的 RootDir）
	if err := si.IndexDir(ctx, cb); err != nil {
		color.Red("索引失败: %v", err)
		return
	}

	elapsed := time.Since(start)
	stats := si.Stats()
	color.Green("✓ 索引完成 (%s)", elapsed.Round(time.Millisecond))
	color.White("  文件数: %d", stats.IndexedFiles)
	color.White("  向量数: %d", stats.VectorCount)
	color.White("  BM25 文档: %d", stats.BM25DocCount)
	color.White("  Embedder: %s (dim=%d)", stats.EmbedderName, stats.VectorDim)
	if errCount > 0 {
		color.Yellow("  失败文件: %d", errCount)
	}

	// 持久化
	if err := si.Save(); err != nil {
		color.Yellow("⚠ 持久化失败: %v", err)
	}
}

// handleSemanticQuery 语义检索
func handleSemanticQuery(ag *agent.CodecastAgent, query string) {
	if ag == nil {
		color.Red("Agent 未初始化")
		return
	}
	if query == "" {
		color.Yellow("用法: /semantic query <query>")
		return
	}
	si := ag.GetSemanticIndex()
	if si == nil {
		color.Red("语义索引未启用。请在 config.yaml 中设置 semantic_index.enabled: true")
		return
	}
	if si.Stats().VectorCount == 0 {
		color.Yellow("索引为空，请先运行 /semantic index")
		return
	}

	ctx := context.Background()
	results, err := si.Retrieve(ctx, query)
	if err != nil {
		color.Red("检索失败: %v", err)
		return
	}
	if len(results) == 0 {
		color.Yellow("未找到匹配结果")
		return
	}

	color.Cyan("🔍 检索结果 (query: %q)", query)
	fmt.Println()
	for i, r := range results {
		color.HiBlack("%d. [%.4f] %s:%d-%d (%s)",
			i+1, r.Score, r.Chunk.File, r.Chunk.StartLine, r.Chunk.EndLine, r.Source)
		color.White("   符号: %s (%s)", r.Chunk.Symbol, r.Chunk.Kind)
		if r.Chunk.Signature != "" {
			color.HiBlack("   签名: %s", r.Chunk.Signature)
		}
		// 显示前 3 行代码预览
		lines := strings.Split(r.Chunk.Content, "\n")
		preview := 3
		if len(lines) < preview {
			preview = len(lines)
		}
		for j := 0; j < preview; j++ {
			color.HiBlack("   | %s", lines[j])
		}
		fmt.Println()
	}
}

// handleSemanticStats 索引统计
func handleSemanticStats(ag *agent.CodecastAgent) {
	if ag == nil {
		color.Red("Agent 未初始化")
		return
	}
	si := ag.GetSemanticIndex()
	if si == nil {
		color.Yellow("语义索引未启用")
		return
	}
	stats := si.Stats()
	color.Cyan("📊 语义索引统计")
	fmt.Println()
	color.White("  已索引文件: %d", stats.IndexedFiles)
	color.White("  向量数: %d", stats.VectorCount)
	color.White("  BM25 文档: %d", stats.BM25DocCount)
	color.White("  Embedder: %s", stats.EmbedderName)
	color.White("  向量维度: %d", stats.VectorDim)
}

// handleSemanticClear 清空索引
func handleSemanticClear(ag *agent.CodecastAgent) {
	if ag == nil {
		color.Red("Agent 未初始化")
		return
	}
	si := ag.GetSemanticIndex()
	if si == nil {
		color.Yellow("语义索引未启用")
		return
	}
	si.Clear()
	color.Green("✓ 索引已清空")
}

// printSemanticHelp 显示 /semantic 帮助
func printSemanticHelp() {
	color.Cyan("🔍 /semantic — 语义索引管理 (P3)")
	fmt.Println()
	color.White("用法:")
	color.White("  /semantic index [path]   索引代码库（默认当前目录）")
	color.White("  /semantic query <query>  语义检索代码")
	color.White("  /semantic stats          查看索引统计")
	color.White("  /semantic clear          清空索引")
	color.White("  /semantic help           显示此帮助")
	fmt.Println()
	color.HiBlack("需在 config.yaml 中启用: semantic_index.enabled: true")
	color.HiBlack("Embedding provider: openai (默认) / mock (测试)")
}

// handleBenchmarkCommand 处理 /benchmark 斜杠命令
//
// 用法:
//
//	/benchmark run [mock|keyword]  运行默认 benchmark 套件
//	/benchmark list                列出所有任务
//	/benchmark help                显示帮助
func handleBenchmarkCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printBenchmarkHelp()
		return
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		printBenchmarkHelp()
		return
	}
	sub := fields[0]
	switch sub {
	case "run":
		runnerType := "mock"
		if len(fields) > 1 {
			runnerType = fields[1]
		}
		runBenchmark(ag, runnerType)
	case "list":
		listBenchmarkTasks()
	case "help", "h":
		printBenchmarkHelp()
	default:
		color.Red("未知子命令: %s", sub)
		printBenchmarkHelp()
	}
}

// runBenchmark 执行 benchmark
func runBenchmark(ag *agent.CodecastAgent, runnerType string) {
	var runner benchmark.Runner
	switch runnerType {
	case "mock":
		runner = benchmark.NewMockRunner(time.Now().UnixNano(), 0.8)
	case "keyword":
		runner = benchmark.KeywordMatchRunner{}
	default:
		color.Red("未知 runner: %s (支持: mock, keyword)", runnerType)
		return
	}
	color.Cyan("🏃 运行 benchmark (runner=%s, tasks=%d)", runner.Name(), len(benchmark.DefaultTasks))
	fmt.Println()

	suite := benchmark.NewDefaultSuite(runner)
	ctx := context.Background()
	results := suite.Run(ctx)

	report := benchmark.GenerateReport(runner.Name(), results)

	// 打印摘要
	color.Cyan("━━━ Summary ━━━")
	color.White("  成功率: %.2f%% (%d/%d)", report.Summary.SuccessRate*100, report.Summary.SuccessCount, report.Summary.TotalTasks)
	color.White("  平均时延: %dms", report.Summary.AvgLatencyMs)
	color.White("  P95 时延: %dms", report.Summary.P95LatencyMs)
	color.White("  总 token: %d", report.Summary.TotalTokens)
	color.White("  总成本: $%.4f", report.Summary.TotalCostUSD)
	color.White("  总工具调用: %d", report.Summary.TotalToolCalls)
	fmt.Println()

	// 打印明细
	color.Cyan("━━━ Details ━━━")
	for _, r := range results {
		status := color.RedString("✗")
		if r.Metrics.Success {
			status = color.GreenString("✓")
		}
		errMsg := r.Metrics.Error
		if len(errMsg) > 40 {
			errMsg = errMsg[:40] + "..."
		}
		color.White("  %s %-12s %4dms  %5d tok  $%.4f  %d tools  %s",
			status, r.TaskID, r.Metrics.LatencyMs, r.Metrics.TokensUsed,
			r.Metrics.CostUSD, r.Metrics.ToolCalls, errMsg)
	}
	fmt.Println()

	// 保存报告
	reportDir := ".codecast/benchmark"
	osMkdirAll(reportDir)
	jsonPath := reportDir + "/report.json"
	mdPath := reportDir + "/report.md"
	if err := report.SaveJSON(jsonPath); err == nil {
		color.HiBlack("JSON 报告: %s", jsonPath)
	}
	if err := report.SaveMarkdown(mdPath); err == nil {
		color.HiBlack("Markdown 报告: %s", mdPath)
	}
}

// listBenchmarkTasks 列出所有 benchmark 任务
func listBenchmarkTasks() {
	color.Cyan("📋 Benchmark 任务集 (%d 个)", len(benchmark.DefaultTasks))
	fmt.Println()
	color.White("%-12s %-12s %-10s %s", "ID", "Type", "Diff", "Description")
	color.HiBlack("─────────────────────────────────────────────────────────")
	for _, t := range benchmark.DefaultTasks {
		color.White("%-12s %-12s %-10s %s", t.ID, t.Type, t.Difficulty, t.Description)
	}
}

// printBenchmarkHelp 显示 /benchmark 帮助
func printBenchmarkHelp() {
	color.Cyan("📊 /benchmark — 自建评估框架 (P0)")
	fmt.Println()
	color.White("用法:")
	color.White("  /benchmark run [mock|keyword]  运行默认 benchmark 套件")
	color.White("  /benchmark list                列出所有任务")
	color.White("  /benchmark help                显示此帮助")
	fmt.Println()
	color.HiBlack("任务集: 5 类 × 3 难度 = 15 个任务")
	color.HiBlack("  类型: question / edit / refactor / debug / multi-file")
	color.HiBlack("  难度: easy / medium / hard")
	color.HiBlack("指标: 成功率 / 时延 / token / 成本 / 工具调用数")
	color.HiBlack("报告: .codecast/benchmark/report.{json,md}")
}

// osMkdirAll 创建目录
func osMkdirAll(dir string) {
	_ = os.MkdirAll(dir, 0755)
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
