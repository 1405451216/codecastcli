package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"
	"codecast/cli/internal/git"
	"codecast/cli/internal/hooks"
	"codecast/cli/internal/indexer"
	"codecast/cli/internal/model"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/rules"
	"codecast/cli/internal/session"
	"codecast/cli/internal/splash"
	"codecast/cli/internal/subagent"
	"codecast/cli/internal/tui"
	"codecast/cli/internal/ui"
	"codecast/cli/internal/vision"

	"github.com/c-bata/go-prompt"
	"github.com/fatih/color"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// quitFlag 标记 REPL 是否已收到退出信号。
//
// v0.2.0 之前使用 panic(promptExit{}) 作为退出信号，依赖 recover() 捕获。
// 这种控制流脆弱——任何第三方库（甚至 go-prompt 自身）抛出 panic 都会被
// 误判为正常退出。改用显式的 quitFlag + done channel。
var quitFlag bool

// commandSuggestions 是斜杠命令的补全建议。
// v0.2.0: 改由 CommandRegistry 提供单一来源，消除之前 /sandbox 重复条目。
// 这里保留为兼容变量，实际由 getCommandSuggestions() 返回 registry 的快照。
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
// 优先返回 runInteractive() 启动时初始化的 registry
var globalCommandRegistry *CommandRegistry

func activeCommandRegistry() *CommandRegistry {
	if globalCommandRegistry != nil {
		return globalCommandRegistry
	}
	// fallback: 创建一个并注册默认命令
	r := NewCommandRegistry()
	RegisterBuiltinCommands(r)
	return r
}

// 全局 Agent 引用，供 executor 和 completer 使用
var codecastAgent *agent.CodecastAgent

// 全局 FileCompleter，供 @file 补全和展开使用
var fileCompleter *tui.FileCompleter

// Task 1.6: Ctrl+C 中断当前请求支持。
//
// 设计要点：
//  1. 当前请求的可取消 cancel 用包级变量持有（processingCancel），
//     SIGINT handler 通过它取消正在运行的 StreamProcess。
//  2. executor / runBufioREPL 在调 StreamProcess 前，
//     创建一个新 ctx 并把它的 cancel 存到 processingCancel。
//  3. StreamProcess 退出时（成功 / 失败 / 被取消），
//     defer cancel() 会执行；下一次 REPL 循环会重建 ctx。
//  4. processingMu 保护 processingCancel 的读写：
//     - 写：REPL 主循环（创建新 ctx）
//     - 读：SIGINT handler goroutine
//     临界区很短（仅做指针赋值），mutex 开销可接受。
//
// 为什么不用 atomic.Pointer：atomic.Pointer 写需要 *context.CancelFunc 类型，
// context.CancelFunc 本身是 interface 转换，atomic 不支持任意 interface。
// 用 mutex + 指针更直接。
var (
	processingCancel context.CancelFunc = func() {}
	processingMu     sync.Mutex
)

// acquireProcessingCtx 创建一个可取消的 ctx 并把它注册为
// "当前正在处理的 ctx"，供 SIGINT handler 通过 processingCancel 取消。
// 调用方必须在 StreamProcess 结束后调用返回的 cancel（建议用 defer）。
//
// 关键：每次处理用户输入都应调用本函数，handler 才能精确取消
// 正在运行的那次请求；如果只在启动时创建一次 ctx，第一次 cancel
// 之后再创建的新 ctx 仍能跑。
func acquireProcessingCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	processingMu.Lock()
	processingCancel = cancel
	processingMu.Unlock()

	return ctx, cancel
}

// setupSignalHandler 安装 SIGINT handler（Task 1.6）。
//
// 行为：
//   - 处理中按 Ctrl+C：调用 processingCancel() 取消当前 ctx；
//     StreamProcess 看到 ctx.Done() 后返回错误，REPL 重新等待输入。
//   - 空闲时按 Ctrl+C：go-prompt 自带 handleSignals 已经会处理
//     （见 c-bata/go-prompt signal_posix.go:30），不需要我们干预。
//
// 边界：
//   - handler 内部只用 codecastAgent.IsProcessing() 与 processingCancel，
//     不调任何会阻塞的函数（避免在 signal handler 中死锁）。
//   - 不调 os.Exit — 让 go-prompt / runBufioREPL 自然清理（defer codecastAgent.Close）。
func setupSignalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for range sigCh {
			// 取当前 cancel（短临界区，handler 内不阻塞）
			processingMu.Lock()
			cancel := processingCancel
			processingMu.Unlock()

			if codecastAgent != nil && codecastAgent.IsProcessing() {
				// 正在处理 → 取消当前 ctx
				// 注意：defer cancel() 在 StreamProcess 退出时会被执行一次，
				// 这里再调一次是 no-op（context.CancelFunc 重复调用安全）。
				cancel()
				fmt.Println("\n⚠ 已中断当前请求")
			}
			// 空闲 → 不做任何事。go-prompt 的内置 handleSignals goroutine
			// 在 executor 返回后会重新监听 SIGINT，触发 exitCh → os.Exit(0)。
			// 唯一边界：bufio 路径下用户连续按 Ctrl+C — 我们这里消费了一次
			// 信号，go-prompt 不会消费；但 bufio 路径在 reader.ReadString('\n')
			// 阻塞时收到的 SIGINT 会让 read 返回 EINTR 错误，我们在 runBufioREPL
			// 看到 read error 后会自然 break。两条路径都能"退出"。
		}
	}()
}

// runInteractive 启动交互式 REPL。F-10：返回 error 而非 os.Exit，
// 由 cmd/root.go 统一处理退出码。
func runInteractive() error {
	// Task 1.6: 安装 SIGINT handler。
	// 注意 go-prompt 在 executor 回调期间会 stop 自己的 handleSignals goroutine
	// （见 c-bata/go-prompt prompt.go:79），所以 executor 期间 SIGINT
	// 不会让 go-prompt 主动 os.Exit — 我们的 signal.Notify 是唯一活跃 listener。
	// executor 返回后 go-prompt 重新启动 handleSignals，此时 SIGINT 走
	// go-prompt 自带逻辑 → exitCh → os.Exit(0)，即"空闲时按 Ctrl+C 退出"。
	setupSignalHandler()
	cfg := config.Load()

	// 从 viper 读取权限相关配置
	if v := viper.GetString("permission_mode"); v != "" {
		cfg.PermissionMode = v
	}
	if v := viper.GetStringSlice("scopes"); len(v) > 0 {
		cfg.Scopes = v
	}
	if viper.GetBool("safe_mode") {
		cfg.SafeMode = true
	}

	// 初始化命令注册表（v0.2.0+ 统一来源）
	globalCommandRegistry = NewCommandRegistry()
	RegisterBuiltinCommands(globalCommandRegistry)
	commandSuggestions = globalCommandRegistry.Suggestions()

	// 启动神经网络动画（后台运行，按任意键可跳过）
	s := splash.DefaultSplash()
	go s.Run()

	// 等待用户按键继续（更自然地退出动画）
	fmt.Println("按任意键跳过动画...")
	_, _ = bufio.NewReader(os.Stdin).ReadByte()

	// v0.2.0 修复: 用户按键后必须显式 Finish splash，
	// 否则 renderLoop 会在后台持续刷新终端，与 REPL 输出混在一起。
	// finishOnce 保护重复调用安全。
	s.Finish()

	ui.PrintHelp()
	fmt.Println()

	// 检查 API Key（F-10：用 return err 替代 os.Exit，
	// 让调用方（cmd/root.go）统一处理退出，避免在测试/包装时无法捕获）
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
		// 从 last_session 文件读取最近的会话 ID
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
		// F-10：改为返回 error 而非 os.Exit，便于测试和包装
		return fmt.Errorf("初始化 Agent 失败: %w", err)
	}
	defer codecastAgent.Close()

	// 保存当前会话 ID 到 last_session 文件
	if saveErr := saveLastSession(codecastAgent.GetSessionID()); saveErr != nil {
		// 保存失败不阻塞，仅静默忽略
		_ = saveErr
	}

	// 显示恢复信息
	if resumeSessionID != "" {
		displayResumeInfo(resumeSessionID)
	}

	color.Green("✓ Agent 已启动")
	color.White("  Provider: %s", cfg.Provider)
	color.White("  Model:    %s", cfg.Model)
	color.White("  Session:  %s", codecastAgent.GetSessionID())

	// 初始化 FileCompleter（使用当前工作目录作为根目录）
	workDir, _ := os.Getwd()
	fileCompleter = tui.NewFileCompleter(workDir)
	// 如果 Agent 有索引器，关联到 FileCompleter 以加速搜索
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

		// 启动 HITL 中断处理 goroutine。
		// 当 AP 框架的 HITLManager 触发 OnInterrupt 时，
		// 请求会存入 HITLManagerWrapper.pending，
		// 此 goroutine 检测到后渲染确认提示并发回用户响应。
		if hitlMgr := permMgr.HitlManager(); hitlMgr != nil {
			go hitlInterruptWatcher(hitlMgr, permMgr)
		}
	}

	// F-04：显示 MCP 启动告警（之前只 slog.Warn，用户看不见）
	if warnings := codecastAgent.GetMCPWarnings(); len(warnings) > 0 {
		fmt.Println()
		color.Yellow("⚠ MCP 启动时出现 %d 个警告：", len(warnings))
		for _, w := range warnings {
			color.Yellow("  - [%s] %s", w.Server, w.Err)
		}
	}

	fmt.Println()

	// 尝试使用 go-prompt REPL，失败则回退到 bufio
	if err := runGoPromptREPL(); err != nil {
		color.Yellow("go-prompt 初始化失败，回退到基础输入模式: %v", err)
		runBufioREPL()
	}

	color.Yellow("再见！")
	return nil
}

// runGoPromptREPL 使用 go-prompt 运行 REPL
func runGoPromptREPL() (err error) {
	// 加载历史记录
	history := loadHistory()

	// 仍保留 recover() 但仅处理意外的 panic（不再误判正常退出）
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
		return nil // 正常退出
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

		// 多行输入支持：以反斜杠结尾时继续读取下一行
		for strings.HasSuffix(input, "\\") {
			input = input[:len(input)-1] // 去掉末尾反斜杠
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

		// 处理特殊命令
		if handleSpecialCommand(input, codecastAgent, &running) {
			// v0.2.0: 与 go-prompt 路径保持一致，退出时设置 quitFlag
			if !running {
				quitFlag = true
			}
			continue
		}

		// 展开 @file 引用
		expanded := expandFileReferences(input)

		// Task 1.6: 每次处理创建新 ctx，让 SIGINT 能精确取消当前请求
		ctx, cancel := acquireProcessingCtx()
		if err := codecastAgent.StreamProcess(ctx, expanded); err != nil {
			color.Red("处理失败: %v", err)
			color.Yellow("💡 如果持续失败，请尝试: 1) /model 切换模型 2) 检查网络连接 3) /clear 清除上下文后重试")
		}
		cancel() // StreamProcess 内部可能已 defer cancel()，这里再调一次是 no-op
	}
}

// executor 是 go-prompt 的执行回调
func executor(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	// 保存历史记录（保存原始输入，不含文件展开内容）
	appendHistory(input)

	running := true
	// 处理特殊命令
	if handleSpecialCommand(input, codecastAgent, &running) {
		if !running {
			quitFlag = true // v0.2.0: 显式退出信号，不再 panic
		}
		return
	}

	// 展开 @file 引用
	expanded := expandFileReferences(input)

	// Task 1.6: 每次处理创建新 ctx，让 SIGINT 能精确取消当前请求
	// F-03：go-prompt 在 executor 回调期间处于 cooked mode，
	// permission.ConfirmPrompt 可直接读 stdin 而不会与 prompt 抢。
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

	// 只对 / 开头的命令提供补全
	if strings.HasPrefix(text, "/") {
		return prompt.FilterHasPrefix(commandSuggestions, text, true)
	}

	// @file 路径补全：当输入包含 @ 时，提供文件路径建议
	if strings.Contains(text, "@") {
		return fileRefCompleter(text)
	}

	return nil
}

// fileRefCompleter 为 @path 提供文件路径补全建议
func fileRefCompleter(text string) []prompt.Suggest {
	// 找到最后一个 @ 的位置
	lastAt := strings.LastIndex(text, "@")
	if lastAt < 0 {
		return nil
	}

	// 提取 @ 后面的路径前缀
	prefix := text[lastAt+1:]

	// 使用 FileCompleter 搜索匹配的文件
	if fileCompleter == nil {
		return nil
	}

	matches := fileCompleter.Complete(prefix)
	if len(matches) == 0 {
		return nil
	}

	suggestions := make([]prompt.Suggest, 0, len(matches))
	for _, m := range matches {
		// 构建补全文本：保留 @ 前的内容 + @ + 匹配的路径
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
	// 限制数量
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
//
// v0.2.0 重构：之前这里有 30+ 个 case + 30+ 个 HasPrefix 块，
// 维护两套平行逻辑（switch vs 字符串前缀）容易漂移。
//
// 现在：
//   1. /quit /q /exit 仍在本函数内处理（需要直接设置 *running）
//   2. 其他所有 /<cmd> 委托给 activeCommandRegistry().Dispatch()，
//      由 CommandRegistry 集中维护命令名/别名/handler
//   3. handler 通过闭包访问 handleXxxCommand 函数，签名一致
func handleSpecialCommand(input string, ag *agent.CodecastAgent, running *bool) bool {
	// === 退出命令需要直接修改 *running，不走 dispatch ===
	if input == "/quit" || input == "/q" || input == "/exit" {
		*running = false
		return true
	}

	// === 其他所有 /<cmd> 委托给注册表 ===
	// Dispatch 解析 /<name> 与 args，找到 handler 调用，
	// 并返回 handler 是否消费了此命令。
	if activeCommandRegistry().Dispatch(input, ag) {
		return true
	}

	// === 兼容：未注册的 /<cmd> 走原 Agent 流程 ===
	return false
}

func exportCurrentSession() {
	filename := fmt.Sprintf("codecast-session-%s.md", time.Now().Format("20060102-150405"))
	exportCurrentSessionTo(filename)
}

func exportCurrentSessionTo(filename string) {
	mgr, err := session.NewManager()
	if err != nil {
		color.Red("导出失败: %v", err)
		return
	}
	defer mgr.Close()

	sessions, err := mgr.List()
	if err != nil {
		color.Red("导出失败: %v", err)
		return
	}

	if len(sessions) == 0 {
		color.Yellow("没有可导出的会话记录")
		return
	}

	// 导出最近更新的会话
	sess := sessions[0]
	msgs, err := mgr.GetHistory(sess.SessionID, 1000)
	if err != nil {
		color.Red("导出失败: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 会话记录: %s\n\n", sess.SessionID))
	sb.WriteString(fmt.Sprintf("> 导出时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("## 用户 (%s)\n\n%s\n\n", msg.CreatedAt.Format("15:04:05"), msg.Content))
		case "assistant":
			sb.WriteString(fmt.Sprintf("## 助手 (%s)\n\n%s\n\n", msg.CreatedAt.Format("15:04:05"), msg.Content))
		default:
			sb.WriteString(fmt.Sprintf("## %s (%s)\n\n%s\n\n", msg.Role, msg.CreatedAt.Format("15:04:05"), msg.Content))
		}
	}

	if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
		color.Red("保存文件失败: %v", err)
		return
	}
	color.Green("✓ 会话已导出到 %s", filename)
}

func listSessions() {
	mgr, err := session.NewManager()
	if err != nil {
		color.Red("获取会话列表失败: %v", err)
		return
	}
	defer mgr.Close()

	sessions, err := mgr.List()
	if err != nil {
		color.Red("获取会话列表失败: %v", err)
		return
	}

	if len(sessions) == 0 {
		color.Yellow("暂无会话记录")
		return
	}

	color.Cyan("💬 会话列表 (%d)", len(sessions))
	for i, s := range sessions {
		fmt.Printf("  %d. %s (%d 条消息, 最后更新: %s)\n",
			i+1, s.SessionID, s.MessageCount, s.UpdatedAt.Format("01-02 15:04"))
	}
}

// lastSessionPath 返回 last_session 文件路径
func lastSessionPath() string {
	return filepath.Join(config.GetConfigDir(), "last_session")
}

// readLastSession 从 last_session 文件读取最近的会话 ID
func readLastSession() (string, error) {
	data, err := os.ReadFile(lastSessionPath())
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("last_session 文件为空")
	}
	return id, nil
}

// saveLastSession 将当前会话 ID 保存到 last_session 文件
func saveLastSession(sessionID string) error {
	configDir := config.GetConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(lastSessionPath(), []byte(sessionID+"\n"), 0644)
}

// displayResumeInfo 显示恢复会话的信息
func displayResumeInfo(sessionID string) {
	mgr, err := session.NewManager()
	if err != nil {
		color.Green("✓ 已恢复会话: %s", sessionID)
		return
	}
	defer mgr.Close()

	msgs, err := mgr.GetHistory(sessionID, 1000)
	if err != nil {
		color.Green("✓ 已恢复会话: %s", sessionID)
		return
	}

	color.Green("✓ 已恢复会话: %s (%d 条历史消息)", sessionID, len(msgs))

	// 显示最近 3 条消息摘要
	recentCount := 3
	if len(msgs) < recentCount {
		recentCount = len(msgs)
	}
	if recentCount > 0 {
		fmt.Println("最近消息:")
		// 取最后 recentCount 条
		start := len(msgs) - recentCount
		for i := start; i < len(msgs); i++ {
			msg := msgs[i]
			roleLabel := "用户"
			if msg.Role == "assistant" {
				roleLabel = "助手"
			}
			summary := truncateRunes(msg.Content, 50)
			fmt.Printf("  %d. [%s] %s\n", i-start+1, roleLabel, summary)
		}
	}
	fmt.Println()
}

// truncateRunes 按 rune 截断字符串到指定长度
func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}

// fileRefRe matches @path/to/file references in user input.
// The path must start with a non-whitespace, non-/, non-@ character after @.
var fileRefRe = regexp.MustCompile(`@([^\s/@][^\s]*)`)

// expandFileReferences expands @path/to/file references in user input
// by reading the file content and injecting it into the message.
// Uses FileCompleter.ExpandFileReferences when available for better
// language detection, truncation, and missing file handling.
func expandFileReferences(input string) string {
	if fileCompleter != nil {
		return fileCompleter.ExpandFileReferences(input)
	}
	// Fallback: inline regex replacement when FileCompleter is not initialized
	return fileRefRe.ReplaceAllStringFunc(input, func(match string) string {
		path := match[1:] // Remove @ prefix
		content, err := os.ReadFile(path)
		if err != nil {
			return match
		}
		ext := filepath.Ext(path)
		lang := detectLanguageFromExt(ext)
		truncated := truncateContent(string(content), 4000)
		return fmt.Sprintf("\n```%s\n// File: %s\n%s\n```\n", lang, path, truncated)
	})
}

// detectLanguageFromExt maps file extensions to code fence language identifiers.
func detectLanguageFromExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".js":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	case ".sql":
		return "sql"
	case ".sh":
		return "bash"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h":
		return "c"
	case ".hpp":
		return "cpp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".r":
		return "r"
	case ".lua":
		return "lua"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".xml":
		return "xml"
	case ".proto":
		return "protobuf"
	case ".dockerfile":
		return "dockerfile"
	case ".makefile":
		return "makefile"
	default:
		return ""
	}
}

// truncateContent truncates content to maxChars bytes, appending a notice if truncated.
func truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n... (truncated, file too large)"
}

// handleFilesCommand 处理 /files 命令，列出匹配 glob 模式的文件
func handleFilesCommand(args string, ag *agent.CodecastAgent) {
	pattern := strings.TrimSpace(args)
	if pattern == "" {
		pattern = "*"
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		color.Red("无效的 glob 模式: %v", err)
		return
	}

	if len(matches) == 0 {
		color.Yellow("没有匹配 %s 的文件", pattern)
		color.White("提示: 使用 @<path> 在消息中引用文件内容")
		return
	}

	color.Cyan("匹配 %s 的文件 (%d):", pattern, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.IsDir() {
			color.White("  📁 %s/", m)
		} else {
			color.White("  📄 %s (%s)", m, humanizeBytes(int(info.Size())))
		}
	}
	color.HiBlack("提示: 使用 @<path> 在消息中引用文件内容")
}

// hitlInterruptWatcher 监控 HITL 中断请求并在终端渲染确认提示。
//
// 工作流程：
//  1. AP 框架的 HITLManager 在工具执行前调用 OnInterrupt 回调
//  2. OnInterrupt 将 InterruptRequest 存入 HITLManagerWrapper.pending
//  3. 本 watcher 检测到 pending 请求后，调用 HandleInterrupt 渲染确认提示
//  4. 用户输入后，通过 SendResponse 将 HumanResponse 发回 HITL 通道
//  5. AP 框架的 RequestInterrupt 从通道读取响应，继续或中止工具执行
//
// 注意：当前 buildPermHook 已在 HookBeforeTool 阶段同步处理确认，
// 所以 HITL 通道路径主要在 AP 框架直接使用 WithHITL 注入时激活。
// 两条路径不会同时触发（Hook 先执行，如果 Hook 拒绝则工具不会到达 executor）。
func hitlInterruptWatcher(hitlMgr *permission.HITLManagerWrapper, permMgr *permission.Manager) {
	// 轮询间隔：100ms 足够快（用户感知 < 200ms），CPU 开销可忽略
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		req := hitlMgr.PendingRequest()
		if req == nil {
			continue
		}

		// 有挂起的中断请求，处理它
		resp, alwaysAllowed := permission.HandleInterrupt(permMgr, req)

		// 如果用户选择 always-allow，更新权限管理器
		if alwaysAllowed {
			// HandleInterrupt 内部已调用 AddAutoAllow
		}

		// 清除 pending 状态
		hitlMgr.SendResponse(resp)
	}
}

// handleModeCommand 处理 /mode 命令，切换权限模式
func handleModeCommand(args string, ag *agent.CodecastAgent) {
	mode := strings.TrimSpace(args)
	if mode == "" {
		// 显示当前模式
		permMgr := ag.PermMgr()
		if permMgr != nil {
			color.Cyan("当前权限模式: %s", permMgr.ModeName())
			fmt.Println("可选模式: suggest, auto-edit, full-auto")
		}
		return
	}

	permMgr := ag.PermMgr()
	if permMgr == nil {
		color.Red("权限管理器未初始化")
		return
	}

	newMode, err := permission.ParseApprovalMode(mode)
	if err != nil {
		color.Red("%v", err)
		return
	}

	// 更新权限管理器（F-02：使用 SetMode 保留 denyList/autoAllow，
	// 避免 /mode 静默清除 SafeMode 黑名单）
	permMgr.SetMode(newMode)
	color.Green("✓ 权限模式已切换为: %s", permMgr.ModeName())
}

// handleRulesCommand 处理 /rules 命令
func handleRulesCommand(args string, ag *agent.CodecastAgent) {
	switch strings.TrimSpace(args) {
	case "", "show":
		// 显示当前规则（含文件来源和大小）
		loader := rules.NewLoader(".")
		rs, err := loader.Load()
		if err != nil {
			color.Red("加载规则失败: %v", err)
			return
		}
		if rs.Merged == "" {
			color.Yellow("未找到项目规则")
			color.White("使用 /rules init 初始化项目配置")
			return
		}
		color.Cyan("已加载的规则:")
		// 显示各来源文件信息
		homeDir, _ := os.UserHomeDir()
		showRuleSource(filepath.Join(homeDir, ".codecast", "rules.md"), "全局", rs.Global)
		showRuleSource(".codecast/rules.md", "项目", rs.Project)
		showRuleSource(".codecast/rules.local.md", "本地", rs.Local)
		for _, sm := range rs.SubModules {
			showRuleSource(filepath.Join(".codecast", "rules", sm.Filename), "子模块", sm.Content)
		}
		totalSize := len(rs.Global) + len(rs.Project) + len(rs.Local)
		for _, sm := range rs.SubModules {
			totalSize += len(sm.Content)
		}
		sources := 0
		if rs.Global != "" {
			sources++
		}
		if rs.Project != "" {
			sources++
		}
		if rs.Local != "" {
			sources++
		}
		sources += len(rs.SubModules)
		color.HiBlack("总大小: %s | 来源: %d 个文件", humanizeBytes(totalSize), sources)
		fmt.Println("─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─")
		fmt.Println(rs.Merged)
	case "init":
		// 在交互模式内初始化项目规则
		if err := rules.InitProject("."); err != nil {
			if strings.Contains(err.Error(), "已存在") {
				color.Yellow("项目规则已存在: %v", err)
			} else {
				color.Red("初始化失败: %v", err)
			}
			return
		}
		color.Green("✓ 已创建 .codecast/rules.md 模板文件")
		color.White("  请根据项目需求编辑此文件。")
	case "reload":
		// 重新加载规则
		color.Yellow("规则将在下次对话时自动重新加载")
	default:
		color.Yellow("未知子命令: %s", args)
		color.White("可用: /rules [show|init|reload]")
	}
}

// showRuleSource 显示规则文件来源信息
func showRuleSource(path, level, content string) {
	if content == "" {
		return
	}
	color.Green("  ✓ %s (%s, %s)", path, level, humanizeBytes(len(content)))
}

// humanizeBytes 将字节数转为人类可读格式
func humanizeBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

// handleCompactCommand 处理 /compact 命令（Task 1.4: 摘要式压缩）
// 旧实现直接 ClearContext 丢失关键信息；现改为：
//   1. 调用 CodecastAgent.SummarizeContext（用 LLM 摘要旧消息）
//   2. 失败时降级到 ClearContext
func handleCompactCommand(args string, ag *agent.CodecastAgent) {
	if ag == nil {
		color.Red("Agent 未初始化")
		return
	}
	color.Cyan("正在摘要压缩上下文...")

	// 30 秒超时，避免 LLM 不可用时挂死
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ag.SummarizeContext(ctx); err != nil {
		color.Yellow("摘要失败，降级到清空: %v", err)
		ag.ClearContext()
		color.Yellow("✓ 上下文已清空（降级）")
		return
	}
	color.Green("✓ 上下文已摘要压缩")
}

// handlePlanCommand 处理 /plan 命令（DI-7: 真正连接 Orchestrator）
func handlePlanCommand(args string, ag *agent.CodecastAgent) {
	if args == "" {
		color.Yellow("用法: /plan <任务描述>")
		return
	}
	color.Cyan("正在规划任务...")

	cfg := config.Load()
	orchestrator, err := subagent.NewOrchestrator(cfg, nil, nil)
	if err != nil {
		color.Red("创建编排器失败: %v", err)
		color.White("回退到普通模式规划...")
		// 回退：直接用 Agent 做规划
		ctx := context.Background()
		planPrompt := fmt.Sprintf("请分析以下任务并制定执行计划（仅规划，不执行）：\n\n%s", args)
		if err := ag.StreamProcess(ctx, planPrompt); err != nil {
			color.Red("规划失败: %v", err)
		}
		return
	}

	ctx := context.Background()
	plan, err := orchestrator.PlanOnly(ctx, args)
	if err != nil {
		color.Red("规划失败: %v", err)
		return
	}

	tui.PrintHeader("执行计划")
	fmt.Println(ag.GetRenderer().RenderMarkdown(plan))
}

// handleDelegateCommand 处理 /delegate 命令（DI-7: 真正连接 Orchestrator）
// 支持 /delegate -v <task> 启用 DAG 可视化
func handleDelegateCommand(args string, ag *agent.CodecastAgent) {
	// 解析 -v 标志
	visualize := false
	task := args
	if strings.HasPrefix(args, "-v ") {
		visualize = true
		task = strings.TrimSpace(strings.TrimPrefix(args, "-v"))
	} else if strings.HasPrefix(args, "--visualize ") {
		visualize = true
		task = strings.TrimSpace(strings.TrimPrefix(args, "--visualize"))
	}

	if task == "" {
		color.Yellow("用法: /delegate [-v] <任务描述>")
		color.White("  -v, --visualize  显示 DAG 可视化")
		return
	}
	color.Cyan("正在使用 Plan+Execute 双 Agent 协作...")

	cfg := config.Load()
	orchestrator, err := subagent.NewOrchestrator(cfg, nil, nil)
	if err != nil {
		color.Red("创建编排器失败: %v", err)
		color.White("回退到普通模式执行...")
		ctx := context.Background()
		if err := ag.StreamProcess(ctx, task); err != nil {
			color.Red("执行失败: %v", err)
		}
		return
	}

	// 启用 DAG 可视化
	if visualize {
		orchestrator.SetVisualization(true)
	}

	spinner := tui.NewSpinner("规划中...")
	spinner.Start()

	// 如果启用可视化，在后台 goroutine 中定期渲染 DAGView
	var renderDone chan struct{}
	if visualize {
		renderDone = make(chan struct{})
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					dv := orchestrator.GetDAGView()
					if dv != nil {
						fmt.Print("\r\033[K") // 清除当前行
						fmt.Println(dv.Render(60))
					}
				case <-renderDone:
					return
				}
			}
		}()
	}

	ctx := context.Background()
	result, err := orchestrator.PlanAndExecute(ctx, task)
	spinner.Stop()

	// 停止后台渲染
	if renderDone != nil {
		close(renderDone)
	}

	// 渲染最终 DAG 状态
	if visualize {
		dv := orchestrator.GetDAGView()
		if dv != nil {
			fmt.Println(dv.Render(60))
		}
	}

	if err != nil {
		color.Red("执行失败: %v", err)
		return
	}

	// 显示规划结果
	if result.Plan != "" {
		tui.PrintHeader("规划结果")
		fmt.Println(ag.GetRenderer().RenderMarkdown(result.Plan))
	}

	// 显示执行结果
	if result.Execution != "" {
		tui.PrintHeader("执行结果")
		fmt.Println(ag.GetRenderer().RenderMarkdown(result.Execution))
	}

	// 显示摘要
	tui.PrintDim(result.Summary())
}

// handleHooksCommand 处理 /hooks 命令
func handleHooksCommand(args string, ag *agent.CodecastAgent) {
	hooksDir := filepath.Join(".codecast", "hooks")
	hm := hooks.NewHookManager(hooksDir)
	hookList := hm.List()
	if len(hookList) == 0 {
		color.Yellow("未配置任何钩子")
		color.White("在 .codecast/hooks/hooks.yaml 中配置钩子")
		return
	}
	color.Cyan("已配置的钩子:")
	for _, h := range hookList {
		status := "禁用"
		if h.Enabled {
			status = "启用"
		}
		color.White("  [%s] %s - %s (%s)", status, h.Name, h.Point, h.Command)
	}
}

// handleVisionCommand 处理 /vision 命令
func handleVisionCommand(args string, ag *agent.CodecastAgent) {
	if args == "" {
		color.Yellow("用法: /vision <图片路径>")
		return
	}
	imagePath := strings.TrimSpace(args)
	if !vision.IsImageFile(imagePath) {
		color.Red("不支持的图片格式，支持: jpg, png, gif, webp, bmp")
		return
	}
	color.Cyan("正在分析图片: %s", imagePath)

	// 使用 Agent 处理图片分析请求
	ctx := context.Background()
	analysisPrompt := fmt.Sprintf("请分析以下图片文件: %s\n描述图片内容，如果图片包含代码，请分析代码逻辑。", imagePath)
	if err := ag.StreamProcess(ctx, analysisPrompt); err != nil {
		color.Red("图片分析失败: %v", err)
	}
}

// handleScreenshotCommand 处理 /screenshot 命令
func handleScreenshotCommand(args string, ag *agent.CodecastAgent) {
	color.Cyan("正在截取屏幕截图...")
	capture := vision.NewScreenshotCapture()
	path, err := capture.Capture()
	if err != nil {
		color.Red("截图失败: %v", err)
		return
	}
	color.Green("截图已保存: %s", path)
	color.White("使用 /vision %s 分析截图", path)
}

// handlePoolCommand 处理 /pool 命令
func handlePoolCommand(args string, ag *agent.CodecastAgent) {
	color.Cyan("Agent Pool 状态:")
	color.White("  Pool 功能需要先初始化，使用 codecast pool 命令管理")
}

// handlePluginsCommand 处理 /plugins 命令
func handlePluginsCommand(args string, ag *agent.CodecastAgent) {
	color.Cyan("已加载的插件:")
	color.White("  使用 codecast plugin list 查看插件列表")
}

// handleIndexCommand 处理 /index 命令
func handleIndexCommand(args string, ag *agent.CodecastAgent) {
	idx := ag.GetIndexer()
	if idx == nil {
		color.Yellow("索引器未初始化")
		return
	}
	index := idx.GetIndex()
	color.Cyan("代码库索引:")
	color.White("  文件数: %d", index.TotalFiles)
	color.White("  总大小: %s", indexer.FormatSize(index.TotalSize))
	for lang, count := range index.Languages {
		color.White("  %s: %d 文件", lang, count)
	}
}

// handleModelCommand 处理 /model 命令（DI-4: 切换模型并重建 Provider）
func handleModelCommand(args string, ag *agent.CodecastAgent) {
	if args == "" {
		switcher := ag.GetModelSwitcher()
		if switcher != nil {
			color.Cyan("当前模型: %s (%s)", switcher.CurrentModel(), switcher.CurrentProvider())
			color.White("可用模型:")
			for _, m := range model.ListModels("") {
				active := ""
				if m.ID == switcher.CurrentModel() {
					active = " ← 当前"
				}
				color.White("  %s (%s) - $%.4f/1k%s", m.ID, m.Provider, m.CostPer1kIn, active)
			}
		}
		return
	}
	if err := ag.SwitchModel(strings.TrimSpace(args)); err != nil {
		color.Red("%v", err)
		return
	}
	switcher := ag.GetModelSwitcher()
	color.Green("✓ 模型已切换为: %s (%s)", switcher.CurrentModel(), switcher.CurrentProvider())
}

// handleConfigCommand 处理 /config 斜杠命令。
//
// 用法:
//   /config                       — 显示帮助与当前配置概览
//   /config list                  — 列出所有配置项
//   /config get <key>             — 读取单个配置项
//   /config set <key> <value>     — 设置单个配置项
//   /config wizard                — 启动交互式配置向导
//   /config providers             — 列出支持的 LLM Provider
//   /config init                  — 初始化配置文件
func handleConfigCommand(args string, ag *agent.CodecastAgent) {
	args = strings.TrimSpace(args)
	if args == "" {
		printConfigHelp()
		configList()
		return
	}

	// 拆分子命令与剩余参数
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
		// /config set <key> <value>  — value 可能包含空格
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

// printConfigHelp 打印 /config 斜杠命令的使用说明
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
		// 显示当前路由配置和状态
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

// countFileRefsForRoute 统计输入中 @file 引用的数量
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

// truncateForDisplay 截断字符串用于显示
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
	prompt := strings.TrimSpace(strings.TrimPrefix(args, sub))
	switch sub {
	case "pipeline":
		if err := workflowRunPipeline(prompt, "分析,开发,测试"); err != nil {
			color.Red("Pipeline 失败: %v", err)
		}
	case "parallel":
		if err := workflowRunParallel(prompt, "审查1,审查2,审查3"); err != nil {
			color.Red("Parallel 失败: %v", err)
		}
	case "handoff":
		if err := workflowRunHandoff(prompt, "分析,开发,测试"); err != nil {
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

// handleUndoCommand 处理 /undo 命令（F2: 撤销最近文件修改）
func handleUndoCommand(args string, ag *agent.CodecastAgent) {
	undoMgr := ag.GetUndoManager()
	if undoMgr == nil {
		color.Red("Undo 管理器未初始化")
		return
	}

	var restoredPath string
	if args == "" {
		// 无参数时回滚最近修改的文件
		backups := undoMgr.ListBackups()
		if len(backups) == 0 {
			color.Yellow("没有可撤销的文件修改")
			return
		}
		// 取最近的备份
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

	// A/B 反馈：撤销视为 fail 信号，让收敛器知道上一轮的变体"被拒绝"
	if restoredPath != "" && ag != nil {
		if ab := ag.GetABIntegration(); ab != nil {
			if ab.ResolveSuccess(false) {
				color.HiBlack("→ A/B: 上一轮已记为 fail（撤销联动）")
			}
		}
	}
}

// handleBudgetCommand 处理 /budget 命令（F8: 查看预算使用情况）
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

// handleMCPInteractiveCommand 处理 /mcp 交互命令（F7: 运行时 MCP 管理）
func handleMCPInteractiveCommand(args string, ag *agent.CodecastAgent) {
	subCmd := strings.TrimSpace(args)
	switch {
	case subCmd == "" || subCmd == "list":
		// 列出已连接的 MCP 服务器
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

func handleStatsFromRegistry(stats any) {
uiPrintStatsFromInteractive(stats)
}

func printAgentStats(stats any) {
switch s := stats.(type) {
case ap.AgentStats:
color.Yellow("📊 Agent 统计:")
fmt.Printf("  状态: %v\n", s.Status)
fmt.Printf("  当前轮数: %d\n", s.CurrentTurn)
fmt.Printf("  消息总数: %d\n", s.TotalMessages)
fmt.Printf("  工具调用: %v\n", s.ToolsCalled)
default:
color.Yellow("📊 Agent 统计: (类型不可用 %T)", stats)
}
}

func uiPrintStatsFromInteractive(stats any) {
printAgentStats(stats)
}

func printResumeHint() {
	color.Cyan("💡 提示: 启动时使用 --resume <id> 或 --continue 恢复会话")
}

// handleReviewCommand 处理 /review 命令 — AI 审查代码变更（含 blame 注入）
//
// 用法:
//
//	/review          — 审查 staged+unstaged 变更
//	/review main     — 审查与 main 分支的差异
//	/review --json   — 输出 JSON 格式审查结果
//	/review --pr     — 将审查结果发布为 GitHub PR 评论
func handleReviewCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	// Parse flags and branch argument
	outputJSON := strings.Contains(args, "--json")
	postPR := strings.Contains(args, "--pr")
	cleanArgs := strings.ReplaceAll(args, "--json", "")
	cleanArgs = strings.ReplaceAll(cleanArgs, "--pr", "")
	branch := strings.TrimSpace(cleanArgs)

	var diff string
	var err error

	if branch != "" {
		diff, err = analyzer.GetDiff(branch)
		if err != nil {
			color.Red("获取 diff 失败: %v", err)
			return
		}
	} else {
		// Get staged + unstaged changes
		staged, sErr := analyzer.StagedDiff()
		unstaged, uErr := analyzer.UnstagedDiff()
		if sErr != nil {
			color.Red("获取 staged diff 失败: %v", sErr)
			return
		}
		if uErr != nil {
			color.Red("获取 unstaged diff 失败: %v", uErr)
			return
		}
		diff = staged + unstaged
	}

	if diff == "" {
		color.Yellow("没有找到变更")
		return
	}

	// Collect blame information for each changed file
	blames := collectBlames(analyzer, diff)

	// Build structured review prompt with blame context
	prompt := git.FormatReviewPrompt(diff, blames)

	ctx, cancel := acquireProcessingCtx()
	defer cancel()

	if outputJSON || postPR {
		// Use ProcessWithResult to capture LLM output for structured parsing
		result, err := ag.ProcessWithResult(ctx, prompt)
		if err != nil {
			color.Red("审查失败: %v", err)
			return
		}

		reviewResult := git.ParseReviewOutput(result.Content)

		if outputJSON {
			jsonStr, err := reviewResult.ToJSON()
			if err != nil {
				color.Red("JSON 序列化失败: %v", err)
				return
			}
			fmt.Println(jsonStr)
		}

		if postPR {
			owner, repo, prNum, err := git.GetPRInfo(analyzer)
			if err != nil {
				color.Yellow("PR 信息获取失败: %v", err)
				color.White("以文本方式输出审查结果：")
				printReviewResultText(reviewResult)
				return
			}
			if err := git.PostPRComments(reviewResult.Findings, owner, repo, prNum); err != nil {
				color.Red("发布 PR 评论失败: %v", err)
			}
		}

		// If --json only (no --pr), also print a short summary
		if outputJSON && !postPR {
			printReviewSummary(reviewResult)
		}
	} else {
		// Default: stream the review output
		if err := ag.StreamProcess(ctx, prompt); err != nil {
			color.Red("审查失败: %v", err)
		}
	}
}

// collectBlames gathers blame summaries for each file in the diff.
func collectBlames(analyzer *git.Analyzer, diff string) map[string]string {
	files := git.ExtractChangedFiles(diff)
	if len(files) == 0 {
		return nil
	}

	blames := make(map[string]string, len(files))
	for _, file := range files {
		summary := analyzer.BlameSummary(file)
		blames[file] = summary
	}
	return blames
}

// printReviewResultText prints a ReviewResult as formatted text.
func printReviewResultText(r *git.ReviewResult) {
	color.Cyan("📋 审查结果")
	fmt.Printf("评分: %d/10\n", r.OverallScore)
	fmt.Printf("摘要: %s\n", r.Summary)
	if len(r.Findings) > 0 {
		fmt.Println()
		for i, f := range r.Findings {
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			sevColor := severityColor(f.Severity)
			sevColor("  [%d] %s | %s — %s", i+1, f.Severity, f.Category, loc)
			fmt.Printf("      %s\n", f.Message)
			if f.Suggestion != "" {
				fmt.Printf("      💡 %s\n", f.Suggestion)
			}
		}
	}
}

// printReviewSummary prints a brief summary after JSON output.
func printReviewSummary(r *git.ReviewResult) {
	color.Cyan("\n📋 审查摘要")
	fmt.Printf("  评分: %d/10 | 发现: %d 项\n", r.OverallScore, len(r.Findings))
	critical := 0
	for _, f := range r.Findings {
		if f.Severity == "critical" {
			critical++
		}
	}
	if critical > 0 {
		color.Red("  ⚠ %d 个严重问题", critical)
	}
}

// severityColor returns a color-print function based on severity level.
func severityColor(severity string) func(string, ...interface{}) {
	switch severity {
	case "critical":
		return color.Red
	case "warning":
		return color.Yellow
	default:
		return color.White
	}
}

// handleBlameCommand 处理 /blame 命令 — 查看 Git Blame 信息
func handleBlameCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	args = strings.TrimSpace(args)
	if args == "" {
		color.Yellow("用法: /blame <file> [line]")
		return
	}

	// Parse: /blame <file> [line] or /blame <file> [start-end]
	parts := strings.Fields(args)
	file := parts[0]

	var output string
	var err error

	if len(parts) >= 2 {
		// Parse line range: single line "10" or range "10-20"
		lineSpec := parts[1]
		if idx := strings.Index(lineSpec, "-"); idx > 0 {
			var start, end int
			if _, err := fmt.Sscanf(lineSpec, "%d-%d", &start, &end); err == nil && start > 0 && end >= start {
				output, err = analyzer.BlameContext(file, start, end)
			} else {
				color.Red("无效的行号范围: %s (格式: start-end)", lineSpec)
				return
			}
		} else {
			var line int
			if _, err := fmt.Sscanf(lineSpec, "%d", &line); err == nil && line > 0 {
				output, err = analyzer.BlameContext(file, line, line)
			} else {
				color.Red("无效的行号: %s", lineSpec)
				return
			}
		}
	} else {
		output, err = analyzer.BlameFile(file)
	}

	if err != nil {
		color.Red("获取 blame 信息失败: %v", err)
		return
	}

	if output == "" {
		color.Yellow("文件 %s 没有 blame 信息", file)
		return
	}

	color.Cyan("📝 Git Blame: %s", file)
	fmt.Println(output)
}

// handleHistoryCommand 处理 /history 命令 — 查看文件修改历史
func handleHistoryCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	file := strings.TrimSpace(args)
	if file == "" {
		color.Yellow("用法: /history <file>")
		return
	}

	output, err := analyzer.FileHistory(file, 20)
	if err != nil {
		color.Red("获取文件历史失败: %v", err)
		return
	}

	if output == "" {
		color.Yellow("文件 %s 没有提交历史", file)
		return
	}

	color.Cyan("📜 文件修改历史: %s", file)
	fmt.Println(output)
}

// handleDiffCommand 处理 /diff 命令 — 查看当前代码变更
func handleDiffCommand(args string, ag *agent.CodecastAgent) {
	analyzer := git.NewAnalyzer(".")
	if !analyzer.IsGitRepo() {
		color.Red("当前目录不是 Git 仓库")
		return
	}

	branch := strings.TrimSpace(args)

	if branch != "" {
		output, err := analyzer.GetDiff(branch)
		if err != nil {
			color.Red("获取 diff 失败: %v", err)
			return
		}
		if output == "" {
			color.Yellow("与 %s 分支没有差异", branch)
			return
		}
		color.Cyan("🔀 Diff: %s...HEAD", branch)
		fmt.Println(output)
		return
	}

	// Show staged + unstaged changes
	staged, sErr := analyzer.StagedDiff()
	if sErr != nil {
		color.Red("获取 staged diff 失败: %v", sErr)
		return
	}
	unstaged, uErr := analyzer.UnstagedDiff()
	if uErr != nil {
		color.Red("获取 unstaged diff 失败: %v", uErr)
		return
	}

	if staged == "" && unstaged == "" {
		color.Yellow("没有未提交的变更")
		return
	}

	if staged != "" {
		color.Cyan("📋 Staged Changes:")
		fmt.Println(staged)
	}
	if unstaged != "" {
		color.Cyan("📋 Unstaged Changes:")
		fmt.Println(unstaged)
	}
}
