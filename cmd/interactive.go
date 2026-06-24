package cmd

// interactive.go: 交互式 REPL 核心入口。
//
// Phase 5.2 拆分：原有的 2250 行已按职责拆分到多个文件：
//   - interactive_session.go  — 会话管理（导出、列表、恢复）
//   - interactive_files.go    — @file 引用展开、语言检测
//   - interactive_handlers.go — 斜杠命令处理器（/rules, /model, /plan 等）
//   - interactive_git.go      — Git 命令（/review, /blame, /diff, /history）
//   - interactive_commands.go — 配置/成本/会话/插件等命令处理器

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/splash"
	"codecast/cli/internal/tui"
	"codecast/cli/internal/ui"

	"github.com/c-bata/go-prompt"
	"github.com/fatih/color"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// quitFlag 标记 REPL 是否已收到退出信号。
// Critical 修复：使用 atomic.Bool 保护并发访问
var quitFlag atomic.Bool

// promptExit 是 go-prompt 优雅退出信号，由 executor 在 /quit 时
// panic 抛出，由 runGoPromptREPL 的 recover 捕获后正常返回。
// 这样 runInteractive 的 defer codecastAgent.Close() 能正常执行，
// 避免 MCP 子进程孤儿、SQLite 连接未关闭、成本数据丢失等问题。
type promptExit struct{}

// commandSuggestions 是斜杠命令的补全建议。
var commandSuggestions []prompt.Suggest

// getCommandSuggestions 返回当前注册表的补全列表
func getCommandSuggestions() []prompt.Suggest {
	r := activeCommandRegistry()
	if r == nil {
		return nil
	}
	return r.Suggestions()
}

// activeCommandRegistry 返回当前活跃的命令注册表
var globalCommandRegistry *CommandRegistry

func activeCommandRegistry() *CommandRegistry {
	if globalCommandRegistry != nil {
		return globalCommandRegistry
	}
	r := NewCommandRegistry()
	RegisterBuiltinCommands(r)
	return r
}

// 全局 Agent 引用，供 executor 和 completer 使用
var codecastAgent *agent.CodecastAgent

// 全局 FileCompleter，供 @file 补全和展开使用
var fileCompleter *tui.FileCompleter

// Task 1.6: Ctrl+C 中断当前请求支持。
var (
	processingCancel context.CancelFunc = func() {}
	processingMu     sync.Mutex
)

// acquireProcessingCtx 创建一个可取消的 ctx 并注册为当前处理的 ctx。
func acquireProcessingCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	processingMu.Lock()
	processingCancel = cancel
	processingMu.Unlock()
	return ctx, cancel
}

// setupSignalHandler 安装 SIGINT handler（Task 1.6）。
func setupSignalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for range sigCh {
			processingMu.Lock()
			cancel := processingCancel
			processingMu.Unlock()

			if codecastAgent != nil && codecastAgent.IsProcessing() {
				cancel()
				fmt.Println("\n⚠ 已中断当前请求")
			}
		}
	}()
}

// runInteractive 启动交互式 REPL。
func runInteractive() error {
	setupSignalHandler()
	cfg := config.Load()

	if v := viper.GetString("permission_mode"); v != "" {
		cfg.PermissionMode = v
	}
	if v := viper.GetStringSlice("scopes"); len(v) > 0 {
		cfg.Scopes = v
	}
	if viper.GetBool("safe_mode") {
		cfg.SafeMode = true
	}

	// 初始化命令注册表
	globalCommandRegistry = NewCommandRegistry()
	RegisterBuiltinCommands(globalCommandRegistry)
	commandSuggestions = globalCommandRegistry.Suggestions()

	// 启动 splash（阻塞式）：~2.5 秒后自动结束，保证 splash 完全退出后再进入主流程
	s := splash.DefaultSplash()
	splashDone := make(chan struct{})
	go func() {
		s.Run()      // 内部是 RunAsync + 等待 done
		close(splashDone)
	}()
	go func() {
		time.Sleep(2500 * time.Millisecond)
		s.Finish()
	}()
	<-splashDone
	time.Sleep(50 * time.Millisecond) // 给 splash 最后一帧渲染缓冲

	ui.PrintHelp()
	fmt.Println()

	// ───── 启动初始化：Provider → Model → Key（Ollama 等本地模型跳过）→ 权限模式 ─────
	needsSetup := cfg.APIKey == ""

	if needsSetup {
		// 1) Provider
		color.Cyan("「步骤 1/3」选择 Provider")
		cfg.Provider = chooseProvider(cfg.Provider)

		// 2) Model（根据 Provider 显示推荐模型）
		fmt.Println()
		color.Cyan("「步骤 2/3」选择模型")
		cfg.Model = chooseModel(cfg.Provider, cfg.Model)

		// 3) API Key（本地模型不需要）
		fmt.Println()
		if providerNeedsAPIKey(cfg.Provider) {
			color.Cyan("「步骤 3/3」输入 API Key")
			for {
				fmt.Print("API Key (隐藏输入): ")
				apiKey, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Println()
				if err != nil {
					color.Red("  读取失败: %v", err)
					continue
				}
				trimmed := strings.TrimSpace(string(apiKey))
				if trimmed == "" {
					color.Red("  API Key 不能为空，请重试")
					continue
				}
				cfg.APIKey = trimmed
				break
			}
			if err := config.Save(cfg); err != nil {
				color.Yellow("⚠ 保存配置失败: %v", err)
			} else {
				color.Green("✓ 配置已保存")
			}
		} else {
			color.Green("✓ %s 是本地模型，不需要 API Key", cfg.Provider)
			if err := config.Save(cfg); err != nil {
				color.Yellow("⚠ 保存配置失败: %v", err)
			}
		}

		// 4) 权限模式
		fmt.Println()
		color.Cyan("「附加」选择权限模式")
		if cfg.PermissionMode == "" {
			cfg.PermissionMode = choosePermissionMode("suggest")
			if err := config.Save(cfg); err != nil {
				color.Yellow("⚠ 保存配置失败: %v", err)
			}
		}
		fmt.Println()
	} else {
		// 已有配置，简洁显示
		color.Cyan("当前配置:")
		color.White("  Provider:   %s", cfg.Provider)
		color.White("  Model:      %s", cfg.Model)
		color.White("  API Key:    %s", cfg.MaskedAPIKey())
		if cfg.PermissionMode != "" {
			color.White("  权限模式:  %s", cfg.PermissionMode)
		}
		color.White("  提示: 使用 /model /config /rules 可调整")
		fmt.Println()
	}

	// 判断是否需要恢复会话
	var resumeSessionID string
	if viper.GetString("resume") != "" {
		resumeSessionID = viper.GetString("resume")
	} else if viper.GetBool("continue") {
		lastID, err := readLastSession()
		if err != nil || lastID == "" {
			color.Yellow("⚠️  没有找到最近的会话记录")
		} else {
			resumeSessionID = lastID
		}
	}

	// 初始化 Agent
	var err error
	if resumeSessionID != "" {
		codecastAgent, err = agent.NewWithSession(cfg, resumeSessionID)
	} else {
		codecastAgent, err = agent.New(cfg)
	}
	if err != nil {
		color.Red("初始化 Agent 失败: %v", err)
		color.Yellow("💡 请检查以下配置:")
		color.White("  1) API Key 是否正确 — 使用 /config set api_key <key> 重新设置")
		color.White("  2) Provider 是否可用 — 当前: %s", cfg.Provider)
		color.White("  3) 网络连接是否正常 — 确认可以访问 API 端点")
		color.White("  4) 模型是否正确 — 当前: %s，使用 /model 切换", cfg.Model)
		return fmt.Errorf("初始化 Agent 失败: %w", err)
	}
	defer codecastAgent.Close()

	if saveErr := saveLastSession(codecastAgent.GetSessionID()); saveErr != nil {
		_ = saveErr
	}

	if resumeSessionID != "" {
		displayResumeInfo(resumeSessionID)
	}

	color.Green("✓ Agent 已启动")
	color.White("  Provider: %s", cfg.Provider)
	color.White("  Model:    %s", cfg.Model)
	color.White("  Session:  %s", codecastAgent.GetSessionID())

	// 初始化 FileCompleter
	workDir, _ := os.Getwd()
	fileCompleter = tui.NewFileCompleter(workDir)
	if idx := codecastAgent.GetIndexer(); idx != nil {
		fileCompleter.SetIndexer(idx)
	}

	// 显示权限模式
	permMgr := codecastAgent.PermMgr()
	if permMgr != nil {
		color.White("  权限模式: %s", permMgr.ModeName())
		if cfg.SafeMode {
			color.Yellow("  安全模式: 已启用（Shell/Web 已禁用）")
		}
		if hitlMgr := permMgr.HitlManager(); hitlMgr != nil {
			go hitlInterruptWatcher(hitlMgr, permMgr)
		}
	}

	// 显示 MCP 启动告警
	if warnings := codecastAgent.GetMCPWarnings(); len(warnings) > 0 {
		fmt.Println()
		color.Yellow("⚠ MCP 启动时出现 %d 个警告：", len(warnings))
		for _, w := range warnings {
			color.Yellow("  - [%s] %s", w.Server, w.Err)
		}
	}

	fmt.Println()

	if err := runGoPromptREPL(); err != nil {
		color.Yellow("go-prompt 初始化失败，回退到基础输入模式: %v", err)
		runBufioREPL()
	}

	color.Yellow("再见！")
	return nil
}

// runGoPromptREPL 使用 go-prompt 运行 REPL
func runGoPromptREPL() (err error) {
	history := loadHistory()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("go-prompt 内部异常: %v", r)
		}
	}()

	p := prompt.New(
		executor,
		completer,
		prompt.OptionPrefix("❯ "),
		prompt.OptionTitle("Codecast CLI"),
		prompt.OptionHistory(history),
		prompt.OptionPrefixTextColor(prompt.Cyan),
		prompt.OptionSelectedDescriptionTextColor(prompt.White),
		prompt.OptionSelectedSuggestionBGColor(prompt.DarkGray),
		prompt.OptionSuggestionBGColor(prompt.DarkGray),
		prompt.OptionDescriptionBGColor(prompt.Black),
	)
	p.Run()
	if quitFlag.Load() {
		return nil
	}
	return nil
}

// runBufioREPL 回退到 bufio.NewReader 的基础 REPL
func runBufioREPL() {
	reader := bufio.NewReader(os.Stdin)
	running := true

	for running {
		fmt.Print(color.CyanString("❯ "))
		input, err := reader.ReadString('\n')
		if err != nil {
			if err.Error() != "EOF" {
				color.Red("读取输入错误: %v", err)
			}
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		for strings.HasSuffix(input, "\\") {
			input = input[:len(input)-1]
			fmt.Print(color.CyanString("… "))
			nextLine, err := reader.ReadString('\n')
			if err != nil {
				if err.Error() != "EOF" {
					color.Red("读取输入错误: %v", err)
				}
				break
			}
			input += "\n" + strings.TrimSpace(nextLine)
		}

		if handleSpecialCommand(input, codecastAgent, &running) {
			if !running {
				quitFlag.Store(true)
			}
			continue
		}

		expanded := expandFileReferences(input)
		ctx, cancel := acquireProcessingCtx()
		if err := codecastAgent.StreamProcess(ctx, expanded); err != nil {
			color.Red("处理失败: %v", err)
			color.Yellow("💡 如果持续失败，请尝试: 1) /model 切换模型 2) 检查网络连接 3) /clear 清除上下文后重试")
		}
		cancel()
	}
}

// executor 是 go-prompt 的执行回调
func executor(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}
	appendHistory(input)

	running := true
	if handleSpecialCommand(input, codecastAgent, &running) {
		if !running {
			quitFlag.Store(true)
		}
		return
	}

	expanded := expandFileReferences(input)
	ctx, cancel := acquireProcessingCtx()
	defer cancel()
	if err := codecastAgent.StreamProcess(ctx, expanded); err != nil {
		color.Red("处理失败: %v", err)
		color.Yellow("💡 如果持续失败，请尝试: 1) /model 切换模型 2) 检查网络连接 3) /clear 清除上下文后重试")
	}
}

// completer 是 go-prompt 的自动补全回调
func completer(d prompt.Document) []prompt.Suggest {
	text := d.TextBeforeCursor()
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "/") {
		return prompt.FilterHasPrefix(commandSuggestions, text, true)
	}
	if strings.Contains(text, "@") {
		return fileRefCompleter(text)
	}
	return nil
}

// fileRefCompleter 为 @path 提供文件路径补全建议
func fileRefCompleter(text string) []prompt.Suggest {
	lastAt := strings.LastIndex(text, "@")
	if lastAt < 0 {
		return nil
	}
	prefix := text[lastAt+1:]
	if fileCompleter == nil {
		return nil
	}
	matches := fileCompleter.Complete(prefix)
	if len(matches) == 0 {
		return nil
	}
	suggestions := make([]prompt.Suggest, 0, len(matches))
	for _, m := range matches {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        text[:lastAt+1] + m,
			Description: filepath.Ext(m),
		})
	}
	return suggestions
}

// getHistoryFilePath 返回历史记录文件路径
func getHistoryFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".codecast")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "history"), nil
}

// ─────────────────────────────────────────────────────────────────────
// 启动引导辅助：Provider / Model / 权限模式
// ─────────────────────────────────────────────────────────────────────

// providerTable：所有支持的 Provider 与推荐模型（截至 2026 年 6 月最新，已联网验证）
var providerTable = map[string][]string{
	"openai":    {"gpt-5.4", "gpt-5.4-pro", "gpt-5.5-instant"},
	"anthropic": {"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-3-5-20241022"},
	"gemini":    {"gemini-3-flash", "gemini-3-pro"},
	"deepseek":  {"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-v3"},
	"qwen":      {"qwen3.7-max", "qwen3.7-plus"},
	"glm":       {"glm-5.2", "glm-5v-turbo"},
	"mimo":      {"mimo-v2.5-pro"},
	"ollama":    {"qwen3:32b", "qwen3:14b", "deepseek-r1:14b", "llama3.3:70b"},
	"cohere":    {"command-r-plus"},
	"mistral":   {"mistral-large-latest"},
	"local":     {"local-default"},
}

// providerOrder：Provider 显示顺序
var providerOrder = []string{
	"openai", "anthropic", "gemini", "deepseek",
	"qwen", "glm", "ollama", "cohere", "mistral", "local",
}

// providerDescriptions：每个 Provider 的简短说明
var providerDescriptions = map[string]string{
	"openai":    "OpenAI（GPT 系列）",
	"anthropic": "Anthropic Claude（强代码能力）",
	"gemini":    "Google Gemini",
	"deepseek":  "DeepSeek（高性价比推理）",
	"qwen":      "阿里通义千问 Qwen",
	"glm":       "智谱 GLM",
	"ollama":    "Ollama 本地模型（无需 API Key）",
	"cohere":    "Cohere",
	"mistral":   "Mistral AI",
	"local":     "本地兼容接口（OpenAI 兼容协议）",
}

// providerNeedsAPIKey：该 Provider 是否需要 API Key
func providerNeedsAPIKey(provider string) bool {
	switch provider {
	case "ollama", "local":
		return false
	}
	return true
}

// chooseProvider：让用户从列表选 Provider，支持默认值（回车即采用）
func chooseProvider(defaultVal string) string {
	reader := bufio.NewReader(os.Stdin)
	for i, name := range providerOrder {
		desc := providerDescriptions[name]
		mark := ""
		if defaultVal != "" && strings.EqualFold(name, defaultVal) {
			mark = color.YellowString("  ★")
		}
		fmt.Printf("  %2d) %-12s %s%s\n", i+1, name, desc, mark)
	}

	prompt := color.CyanString("选择编号 [%s]: ", defaultVal)
	for {
		fmt.Print(prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			// stdin 不可读（比如管道），直接用默认值
			return defaultVal
		}
		line = strings.TrimSpace(line)

		// 回车 → 使用默认值
		if line == "" && defaultVal != "" {
			return defaultVal
		}

		// 数字选择
		var idx int
		if _, err := fmt.Sscanf(line, "%d", &idx); err == nil {
			if idx >= 1 && idx <= len(providerOrder) {
				return providerOrder[idx-1]
			}
		}

		// 直接输入 provider 名称
		lower := strings.ToLower(line)
		for _, name := range providerOrder {
			if lower == name {
				return name
			}
		}

		color.Red("  无效输入，请输入 1-%d 或 Provider 名称", len(providerOrder))
	}
}

// chooseModel：根据 Provider 显示推荐模型，用户选择或手动输入
func chooseModel(provider, defaultVal string) string {
	models := providerTable[provider]
	reader := bufio.NewReader(os.Stdin)

	for i, m := range models {
		mark := ""
		if defaultVal != "" && m == defaultVal {
			mark = color.YellowString("  ★")
		}
		fmt.Printf("  %2d) %s%s\n", i+1, m, mark)
	}

	prompt := color.CyanString("选择编号或直接输入模型名 [%s]: ", defaultVal)
	for {
		fmt.Print(prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			return defaultVal
		}
		line = strings.TrimSpace(line)

		// 回车 → 使用默认值
		if line == "" && defaultVal != "" {
			return defaultVal
		}

		// 数字选择
		var idx int
		if _, err := fmt.Sscanf(line, "%d", &idx); err == nil {
			if idx >= 1 && idx <= len(models) {
				return models[idx-1]
			}
		}

		// 自定义模型名
		if line != "" {
			return line
		}

		color.Red("  无效输入，请输入 1-%d 或模型名称", len(models))
	}
}

// choosePermissionMode：选择权限模式
func choosePermissionMode(defaultVal string) string {
	modes := []struct {
		name string
		desc string
	}{
		{"suggest", "每项操作前都需要确认（最安全）"},
		{"auto-edit", "自动应用文件编辑，运行命令前需要确认"},
		{"full-auto", "自动应用所有变更（本地项目推荐）"},
	}
	reader := bufio.NewReader(os.Stdin)

	for i, m := range modes {
		mark := ""
		if defaultVal != "" && m.name == defaultVal {
			mark = color.YellowString("  ★")
		}
		fmt.Printf("  %d) %-12s %s%s\n", i+1, m.name, m.desc, mark)
	}

	prompt := color.CyanString("选择编号 [%s]: ", defaultVal)
	for {
		fmt.Print(prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			return defaultVal
		}
		line = strings.TrimSpace(line)

		if line == "" && defaultVal != "" {
			return defaultVal
		}

		var idx int
		if _, err := fmt.Sscanf(line, "%d", &idx); err == nil {
			if idx >= 1 && idx <= len(modes) {
				return modes[idx-1].name
			}
		}

		lower := strings.ToLower(line)
		for _, m := range modes {
			if lower == m.name {
				return m.name
			}
		}

		color.Red("  无效输入，请输入 1-%d 或模式名称", len(modes))
	}
}

const maxHistory = 1000

// loadHistory 从文件加载历史记录
func loadHistory() []string {
	path, err := getHistoryFilePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var history []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			history = append(history, line)
		}
	}
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	return history
}

// appendHistory 追加一条历史记录到文件
func appendHistory(input string) {
	path, err := getHistoryFilePath()
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(input + "\n")
}

// handleSpecialCommand 处理斜杠命令的入口。
func handleSpecialCommand(input string, ag *agent.CodecastAgent, running *bool) bool {
	if input == "/quit" || input == "/q" || input == "/exit" {
		*running = false
		return true
	}
	if activeCommandRegistry().Dispatch(input, ag) {
		return true
	}
	return false
}

// hitlInterruptWatcher 监控 HITL 中断请求并在终端渲染确认提示。
func hitlInterruptWatcher(hitlMgr *permission.HITLManagerWrapper, permMgr *permission.Manager) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		req := hitlMgr.PendingRequest()
		if req == nil {
			continue
		}
		resp, _ := permission.HandleInterrupt(permMgr, req)
		_ = resp
		hitlMgr.SendResponse(resp)
	}
}
