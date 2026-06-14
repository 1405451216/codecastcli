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
var quitFlag bool

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

	// 启动神经网络动画
	s := splash.DefaultSplash()
	go s.Run()
	fmt.Println("按任意键跳过动画...")
	_, _ = bufio.NewReader(os.Stdin).ReadByte()
	s.Finish()

	ui.PrintHelp()
	fmt.Println()

	// 检查 API Key
	if cfg.APIKey == "" {
		color.Yellow("⚠️  未配置 API Key")
		fmt.Print("请输入 API Key (输入将隐藏): ")
		apiKey, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("读取 API Key 失败: %w", err)
		}
		cfg.APIKey = strings.TrimSpace(string(apiKey))
		if cfg.APIKey == "" {
			return fmt.Errorf("API Key 不能为空")
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("保存配置失败: %w", err)
		}
		color.Green("✓ API Key 已保存")
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
	if quitFlag {
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
				quitFlag = true
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
			quitFlag = true
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
