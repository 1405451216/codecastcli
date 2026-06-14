package agent

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/budget"
	"codecast/cli/internal/checkpoint"
	"codecast/cli/internal/config"
	ctxpkg "codecast/cli/internal/context"
	"codecast/cli/internal/cost"
	"codecast/cli/internal/diff"
	"codecast/cli/internal/indexer"
	"codecast/cli/internal/lazy"
	automem "codecast/cli/internal/memory"
	"codecast/cli/internal/model"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/provider"
	"codecast/cli/internal/rules"
	"codecast/cli/internal/sandbox"
	sessionpkg "codecast/cli/internal/session"
	customtools "codecast/cli/internal/tools"
	"codecast/cli/internal/tui"
	"codecast/cli/internal/ui"
	"codecast/cli/internal/undo"
)

// CodecastAgent 封装了 AgentPrimordia 的 Agent，提供 CLI 交互功能
type CodecastAgent struct {
	agent         *ap.CapabilityAgent
	config        *config.Config
	memory        ap.Memory
	registry      *ap.ToolRegistry
	session       *ap.Session
	sessionID     string
	costTracker   *cost.Tracker
	mcpRegistry   *ap.MCPRegistry
	permMgr       *permission.Manager
	indexer       *indexer.Indexer
	modelSwitcher *model.Switcher
	diffPreview   *diff.Previewer
	renderer      *tui.Renderer
	// F2: Undo/Rollback
	undoMgr *undo.Manager
	// F4: Git Checkpoint
	checkpointMgr *checkpoint.Manager
	// F8: Budget Controller
	budgetCtrl *budget.Controller
	// F10: 懒加载模块（首次访问时才初始化）
	lazySandbox   *lazy.Value[*sandbox.Executor]
	lazyAutoMem   *lazy.Value[*automem.AutoPersister]
	// F04: 启动时累积的 MCP 警告，runInteractive 渲染时展示
	mcpWarnings []MCPWarning
	// F05: 共享的 SQLite 连接（与 session.Manager 共享），
	// 通过 GetSharedDB() 暴露给其他模块
	sharedDB *sql.DB
	// ABIntegrate: A/B 收敛器 + 时延追踪（A/B 评估闭环核心）
	ab *ABIntegration
	// currentVariant 当前轮次选中的变体名（每轮 Process 入口重算）。
	// system prompt 仍由 buildSystemPrompt 静态构建（避免破坏会话），
	// 但 ab.RecordOutcome 实际记录的 variant 用这个值，以反映路由决策。
	currentVariant string
	// routerPrompt 路由决策器（懒初始化：首次 SelectRouted 策略时构造）。
	routerPrompt *RouterCache
}

// newAgent 内部工厂函数
func newAgent(cfg *config.Config, sessionID string) (*CodecastAgent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	llmProvider, err := provider.CreateProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w\n💡 请检查: 1) API Key 是否正确 2) Provider 配置是否正确 3) 使用 /model 切换到其他模型", err)
	}

	registry, err := ap.DefaultToolkit(ap.ToolkitConfig{
		RootDir:     ".",
		EnableFS:    true,
		EnableShell: true,
		EnableWeb:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("创建工具集失败: %w", err)
	}

	editTool := customtools.NewEditFileTool()
	registry.Register(editTool)
	grepTool := customtools.NewGrepSearchTool()
	registry.Register(grepTool)
	globTool := customtools.NewGlobSearchTool()
	registry.Register(globTool)
	listTool := customtools.NewListFilesTool()
	registry.Register(listTool)
	// delete_file 接受 *undo.Manager 以共享 agent 的 undo 栈；
	// 这里传 nil，由 NewDeleteFileTool 内部回退到默认 undo manager。
	// 后续可改造为延迟注入（先注册占位、undoMgr 创建后绑定），但 v0.3
	// 暂保持简单 — delete_file 自带备份栈，agent Undo 命令暂不覆盖它。
	deleteTool := customtools.NewDeleteFileTool(nil)
	registry.Register(deleteTool)
	multiEditTool := customtools.NewMultiEditTool()
	registry.Register(multiEditTool)
	// 增强 read_file 覆盖 AP 默认（AP DefaultToolkit 注册的是
	// "filesystem" 多操作工具，而非同名 "read_file"，所以不会冲突）
	readTool := customtools.NewReadFileTool()
	registry.Register(readTool)

	mcpReg, mcpWarnings, _ := ConnectMCPServers(registry)
	// F-04：MCP 启动告警暂存到 agent 结构，由 runInteractive 渲染时显示
	_ = mcpWarnings

	memPath := filepath.Join(config.GetConfigDir(), "memory.db")
	memory, err := ap.NewSQLiteStore(memPath)
	if err != nil {
		memory, err = ap.WithInMemory()
		if err != nil {
			return nil, fmt.Errorf("初始化记忆存储失败: %w", err)
		}
	}

	// F-05：暴露底层 *sql.DB 供 session.Manager 共享，
	// 避免双开同一个 SQLite 文件导致的锁竞争。
	// 内存模式下 GetDB() 可能为 nil；只有磁盘模式才有意义。
	var sharedDB *sql.DB
	if memory != nil {
		sharedDB = memory.GetDB()
	}
	// 注入到 session 包的进程级共享槽，让 session.NewManager() 自动复用
	sessionpkg.SetSharedDB(sharedDB)

	// 权限管理器
	permMode := cfg.PermissionMode
	if permMode == "" {
		permMode = "suggest"
	}
	permMgr, err := permission.NewManagerFromString(permMode)
	if err != nil {
		return nil, fmt.Errorf("创建权限管理器失败: %w", err)
	}

	if cfg.SafeMode {
		permMgr.AddDeny("shell_execute")
		permMgr.AddDeny("web_request")
		permMgr.AddDeny("web_fetch")
	}

	// 创建 Diff 预览器
	diffPrev := diff.NewPreviewer()

	// 创建 Undo 管理器（F2）
	undoMgr := undo.NewManager(getCurrentDir())

	// 创建 Git Checkpoint 管理器（F4）
	cpCfg := checkpoint.DefaultConfig()
	cpCfg.Enabled = cfg.AutoCheckpoint
	cpCfg.AutoStash = cfg.AutoStash
	checkpointMgr := checkpoint.NewManager(getCurrentDir(), cpCfg)

	// 创建 Budget 控制器（F8）
	budgetCtrl := budget.NewController(budget.BudgetConfig{
		DailyLimitUSD:    cfg.DailyBudgetUSD,
		SessionLimitUSD:  cfg.SessionBudgetUSD,
		DailyTokenLimit:  cfg.DailyTokenLimit,
		SessionTokenLimit: cfg.SessionTokenLimit,
	})

	// 使用 Hooks 实现权限拦截 + Diff 预览 + Undo 备份 + Checkpoint
	hooks := ap.NewHookManager()
	hooks.Register(ap.HookBeforeTool, buildPermHook(permMgr))
	hooks.Register(ap.HookBeforeTool, buildDiffPreviewHook(diffPrev))
	hooks.Register(ap.HookBeforeTool, buildUndoHook(undoMgr))
	hooks.Register(ap.HookBeforeTool, buildCheckpointHook(checkpointMgr))

	// 加载项目规则（含自动学习规则）
	projectRules := loadProjectRules()

	// 创建代码库索引器（先构建，以便注入系统提示词）
	// F-07：带 spinner 的构建（替代静默同步），大仓库给用户视觉反馈
	idx := indexer.NewIndexer(getCurrentDir())
	ui.StartSpinner("正在构建代码库索引...")
	if err := idx.BuildWithCallback(func(p string) {
		// 回调每次可能非常频繁（每文件一次），所以仅每 N 个文件更新一次 spinner
		if idx.GetIndex() != nil && idx.GetIndex().TotalFiles%50 == 0 {
			ui.UpdateSpinnerMessage(fmt.Sprintf("索引中: %d 个文件...", idx.GetIndex().TotalFiles))
		}
	}); err != nil {
		// 索引失败不阻塞 Agent — 系统提示词退化为不含文件树
		fmt.Fprintf(os.Stderr, "⚠ 代码库索引失败: %v\n", err)
	}
	ui.StopSpinner()

	// 创建系统提示词（注入项目规则 + 代码库文件树）
	// 优先走 PromptResolver（支持 YAML 外部化变体 + A/B 策略），失败则回落到 buildSystemPrompt。
	resolver := DefaultResolver()
	// 加载项目级 .codecast/prompts/（cwd 或 cfg.PromptProjectDir 指定的目录）
	projectPromptsDir := cfg.PromptProjectDir
	if projectPromptsDir == "" {
		projectPromptsDir = ".codecast/prompts"
	}
	if abs, err := filepath.Abs(projectPromptsDir); err == nil {
		_ = resolver.LoadProjectDir(abs)
	}
	resolver.SetSelector(SelectorConfig{
		Variant:  cfg.PromptVariant,
		Strategy: cfg.PromptStrategy,
		Weights:  cfg.PromptWeights,
	}.ToSelector())
	systemPrompt := resolver.Build(runtime.GOOS, getCurrentDir(), projectRules, idx, cfg.PermissionMode, cfg.SessionBudgetUSD)

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"."}
	}

	// F-01 修复：把 FileScopePolicy 真正注入到工具集。
	// 之前的 ap.WithFileScope 只写到 agent metadata，从未被框架读取，
	// 导致 LLM 仍可读 /etc/passwd 等越界文件。
	scopePolicy := ap.NewFileScopePolicy()
	scopePolicy.SetScope("codecast", scopes)
	ap.RegistryWithScopePolicy(registry, scopePolicy, "codecast")

	capAgent := ap.NewAgent("CodecastAgent", systemPrompt, llmProvider,
		ap.WithMaxTurns(20),
	).WithToolkit(registry).WithMemory(memory).
		WithHooks(hooks).
		WithFileScope(scopes)

	var session *ap.Session
	if sessionID != "" {
		session = ap.NewSession(capAgent, memory, ap.SessWithID(sessionID))
	} else {
		session = ap.NewSession(capAgent, memory)
		sessionID = session.SessionID()
	}

	var tracker *cost.Tracker
	if t, err := cost.NewTracker(); err == nil {
		tracker = t
	}

	// 创建模型切换器
	modelSwitch := model.NewSwitcher(cfg)

	// 创建 TUI 渲染器
	tuiRenderer := tui.NewRenderer()

	// F10: 懒加载非关键模块（延迟到首次使用时初始化，减少启动时间）
	lazySandbox := lazy.NewValue(func() (*sandbox.Executor, error) {
		sandboxCfg := sandbox.DefaultConfig()
		return sandbox.NewExecutor(sandboxCfg), nil
	})
	lazyAutoMem := lazy.NewValue(func() (*automem.AutoPersister, error) {
		return automem.NewAutoPersister(getCurrentDir()), nil
	})

	return &CodecastAgent{
		agent:         capAgent,
		config:        cfg,
		memory:        memory,
		registry:      registry,
		session:       session,
		sessionID:     sessionID,
		costTracker:   tracker,
		mcpRegistry:   mcpReg,
		permMgr:       permMgr,
		indexer:       idx,
		modelSwitcher: modelSwitch,
		diffPreview:   diffPrev,
		renderer:      tuiRenderer,
		undoMgr:       undoMgr,
		checkpointMgr: checkpointMgr,
		budgetCtrl:    budgetCtrl,
		lazySandbox:   lazySandbox,
		lazyAutoMem:   lazyAutoMem,
		mcpWarnings:   mcpWarnings,
		sharedDB:      sharedDB,
		ab:            LoadABIntegration(""),
		routerPrompt:  NewRouterCache(),
	}, nil
}

// buildPermHook 构建权限检查 Hook
func buildPermHook(mgr *permission.Manager) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil {
			return nil
		}

		toolName := hctx.ToolCall.Name

		// 检查是否被禁止
		if mgr.IsDenied(toolName) {
			return fmt.Errorf("工具 %s 已被安全模式禁止", toolName)
		}

		// 检查是否需要确认
		if mgr.ShouldApprove(toolName) {
			args := hctx.ToolCall.Args

			// F-03 已修复：go-prompt 在调用 executor 回调期间会把终端
			// 切回 cooked mode（见 c-bata/go-prompt prompt.go 的 setUp/tearDown），
			// 所以 ConfirmPrompt 直接读 stdin 不会与 go-prompt 抢输入。
			// permission.ConfirmPrompt 用了 ANSI 颜色 + 立即 flush 把权限
			// 提示与正常 prompt 视觉上区分开。
			result := permission.ConfirmPrompt(toolName, args)

			switch result.Action {
			case permission.ActionAllow:
				return nil
			case permission.ActionDeny:
				return fmt.Errorf("用户拒绝执行工具 %s", toolName)
			case permission.ActionAlwaysAllow:
				mgr.AddAutoAllow(toolName)
				return nil
			case permission.ActionEditArgs:
				// 修改参数后放行
				if result.ModifiedArgs != "" {
					hctx.ToolCall.Args = result.ModifiedArgs
				}
				return nil
			default:
				return fmt.Errorf("用户拒绝执行工具 %s", toolName)
			}
		}

		return nil
	}
}

// buildDiffPreviewHook 构建 Diff 预览 Hook（DI-3: edit_file/write_file 执行前自动 diff 预览）
func buildDiffPreviewHook(prev *diff.Previewer) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil || prev == nil {
			return nil
		}

		toolName := hctx.ToolCall.Name
		switch toolName {
		case "edit_file", "write_file":
			// 尝试解析文件路径和内容
			args := hctx.ToolCall.Args
			filePath := extractJSONField(args, "file_path")
			if filePath == "" {
				filePath = extractJSONField(args, "path")
			}
			if filePath == "" {
				return nil
			}

			if toolName == "edit_file" {
				oldStr := extractJSONField(args, "old_string")
				newStr := extractJSONField(args, "new_string")
				if oldStr != "" {
					change := prev.PreviewEdit(filePath, oldStr, newStr)
					fmt.Println(tui.Styles.Warning.Render("即将修改文件: " + filePath))
					fmt.Println(tui.NewRenderer().RenderDiff(change.Diff))
				}
			} else if toolName == "write_file" {
				content := extractJSONField(args, "content")
				_, err := os.Stat(filePath)
				exists := err == nil
				change := prev.PreviewWrite(filePath, content, exists)
				if exists {
					fmt.Println(tui.Styles.Warning.Render("即将覆盖文件: " + filePath))
				} else {
					fmt.Println(tui.Styles.Info.Render("即将创建文件: " + filePath))
				}
				fmt.Println(tui.NewRenderer().RenderDiff(change.Diff))
			}
		}

		return nil
	}
}

// extractJSONField 从 JSON 字符串中提取字段值（简易实现）
func extractJSONField(jsonStr, field string) string {
	// 查找 "field": "value" 模式
	pattern := `"` + field + `"`
	idx := strings.Index(jsonStr, pattern)
	if idx == -1 {
		return ""
	}
	// 找到冒号后的值
	afterKey := jsonStr[idx+len(pattern):]
	// 跳过空白和冒号
	i := 0
	for i < len(afterKey) && (afterKey[i] == ' ' || afterKey[i] == ':' || afterKey[i] == '\t' || afterKey[i] == '\n' || afterKey[i] == '\r') {
		i++
	}
	if i >= len(afterKey) {
		return ""
	}
	afterKey = afterKey[i:]

	// 字符串值
	if afterKey[0] == '"' {
		// 找到结束引号（处理转义）
		j := 1
		for j < len(afterKey) {
			if afterKey[j] == '\\' && j+1 < len(afterKey) {
				j += 2
				continue
			}
			if afterKey[j] == '"' {
				return unescapeJSONString(afterKey[1:j])
			}
			j++
		}
		return afterKey[1:]
	}

	return ""
}

// unescapeJSONString 反转义 JSON 字符串
func unescapeJSONString(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	return s
}

// buildUndoHook 构建 Undo 备份 Hook（F2: edit_file/write_file 执行前自动备份）
// 注意：delete_file 内部已经自行 Backup（用自己的 undo 栈），不在此 hook 范围。
// 集成到 agent.undoMgr 需要 delete_file 接受注入式 manager — 见 agent.go:102 注释。
func buildUndoHook(mgr *undo.Manager) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil || mgr == nil {
			return nil
		}
		toolName := hctx.ToolCall.Name
		if toolName == "edit_file" || toolName == "write_file" {
			filePath := extractJSONField(hctx.ToolCall.Args, "file_path")
			if filePath == "" {
				filePath = extractJSONField(hctx.ToolCall.Args, "path")
			}
			if filePath != "" {
				_ = mgr.Backup(filePath) // 静默备份，失败不阻塞
			}
		}
		return nil
	}
}

// buildCheckpointHook 构建 Git Checkpoint Hook（F4: 会话级检查点，避免每次工具调用都 stash）
func buildCheckpointHook(mgr *checkpoint.Manager) ap.HookFunc {
	return func(ctx context.Context, hctx *ap.HookContext) error {
		if hctx.ToolCall == nil || mgr == nil {
			return nil
		}
		// 仅对文件修改工具创建检查点
		toolName := hctx.ToolCall.Name
		if toolName != "edit_file" && toolName != "write_file" && toolName != "delete_file" {
			return nil
		}
		return mgr.AutoCheckpoint(toolName, hctx.ToolCall.Args)
	}
}

// New 创建一个新的 Codecast Agent
func New(cfg *config.Config) (*CodecastAgent, error) {
	return newAgent(cfg, "")
}

// NewWithSession 创建一个恢复指定会话 ID 的 Codecast Agent
func NewWithSession(cfg *config.Config, sessionID string) (*CodecastAgent, error) {
	return newAgent(cfg, sessionID)
}

// GetSessionID 返回当前会话 ID
func (a *CodecastAgent) GetSessionID() string {
	return a.sessionID
}

// PermMgr 返回权限管理器
func (a *CodecastAgent) PermMgr() *permission.Manager {
	return a.permMgr
}

// GetIndexer 返回索引器
func (a *CodecastAgent) GetIndexer() *indexer.Indexer {
	return a.indexer
}

// GetAutoMemory 返回自动记忆持久化器（F10: 懒加载）
func (a *CodecastAgent) GetAutoMemory() *automem.AutoPersister {
	if a.lazyAutoMem == nil {
		return nil
	}
	mem, err := a.lazyAutoMem.Get()
	if err != nil {
		return nil
	}
	return mem
}

// GetModelSwitcher 返回模型切换器
func (a *CodecastAgent) GetModelSwitcher() *model.Switcher {
	return a.modelSwitcher
}

// GetSandbox 返回沙箱执行器（F10: 懒加载）
func (a *CodecastAgent) GetSandbox() *sandbox.Executor {
	if a.lazySandbox == nil {
		return nil
	}
	exec, err := a.lazySandbox.Get()
	if err != nil {
		return nil
	}
	return exec
}

// GetDiffPreviewer 返回 Diff 预览器
func (a *CodecastAgent) GetDiffPreviewer() *diff.Previewer {
	return a.diffPreview
}

// GetRenderer 返回 TUI 渲染器
func (a *CodecastAgent) GetRenderer() *tui.Renderer {
	return a.renderer
}

// GetUndoManager 返回 Undo 管理器（F2）
func (a *CodecastAgent) GetUndoManager() *undo.Manager {
	return a.undoMgr
}

// GetCheckpointManager 返回 Git Checkpoint 管理器（F4）
func (a *CodecastAgent) GetCheckpointManager() *checkpoint.Manager {
	return a.checkpointMgr
}

// GetBudgetController 返回 Budget 控制器（F8）
func (a *CodecastAgent) GetBudgetController() *budget.Controller {
	return a.budgetCtrl
}

// GetMCPWarnings 返回启动时累积的 MCP 警告（F-04）
func (a *CodecastAgent) GetMCPWarnings() []MCPWarning {
	return a.mcpWarnings
}

// GetSharedDB 返回与记忆存储共享的 SQLite 连接（F-05）。
// 返回 nil 表示 agent 跑在内存模式（无文件持久化）。
// 用途：session.Manager 等其他模块应使用此连接，
// 而不是自己再开一个 *sql.DB，避免锁竞争。
func (a *CodecastAgent) GetSharedDB() *sql.DB {
	return a.sharedDB
}

// UndoLastFileChange 回滚最近一次文件修改（F2: /undo 命令调用）
func (a *CodecastAgent) UndoLastFileChange(filePath string) (bool, error) {
	if a.undoMgr == nil {
		return false, fmt.Errorf("undo 管理器未初始化")
	}
	return a.undoMgr.Restore(filePath)
}

// SwitchModel 切换模型并重建 Provider（DI-4: Model Switcher→Provider 深度集成）
func (a *CodecastAgent) SwitchModel(modelID string) error {
	if err := a.modelSwitcher.SwitchWithConfig(modelID, a.config); err != nil {
		return err
	}

	// 重建 LLM Provider
	newProvider, err := provider.CreateProvider(a.config)
	if err != nil {
		return fmt.Errorf("重建 Provider 失败: %w", err)
	}

	// 重建 Agent（保留工具、记忆、Hooks、作用域）
	hooks := ap.NewHookManager()
	hooks.Register(ap.HookBeforeTool, buildPermHook(a.permMgr))
	hooks.Register(ap.HookBeforeTool, buildDiffPreviewHook(a.diffPreview))
	hooks.Register(ap.HookBeforeTool, buildUndoHook(a.undoMgr))
	hooks.Register(ap.HookBeforeTool, buildCheckpointHook(a.checkpointMgr))

	projectRules := loadProjectRules()
	// F-09 修复：SwitchModel 重建时也走 PromptResolver，让运行时切换后
	// 依然使用与 newAgent 一致的 prompt 选择策略。
	resolver := DefaultResolver()
	projectPromptsDir := a.config.PromptProjectDir
	if projectPromptsDir == "" {
		projectPromptsDir = ".codecast/prompts"
	}
	if abs, err := filepath.Abs(projectPromptsDir); err == nil {
		_ = resolver.LoadProjectDir(abs)
	}
	resolver.SetSelector(SelectorConfig{
		Variant:  a.config.PromptVariant,
		Strategy: a.config.PromptStrategy,
		Weights:  a.config.PromptWeights,
	}.ToSelector())
	systemPrompt := resolver.Build(runtime.GOOS, getCurrentDir(), projectRules, a.indexer, a.config.PermissionMode, a.config.SessionBudgetUSD)

	scopes := a.config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"."}
	}

	// F-01 + F-09 修复：SwitchModel 重建 agent 时必须重新注入
	// FileScopePolicy，否则 LLM 会通过未受保护的旧 toolkit 越权。
	scopePolicy := ap.NewFileScopePolicy()
	scopePolicy.SetScope("codecast", scopes)
	ap.RegistryWithScopePolicy(a.registry, scopePolicy, "codecast")

	// F-09 修复：SwitchModel 也重新构建索引器，避免新模型看到陈旧文件树
	a.indexer = indexer.NewIndexer(getCurrentDir())
	a.indexer.Build()

	capAgent := ap.NewAgent("CodecastAgent", systemPrompt, newProvider,
		ap.WithMaxTurns(20),
	).WithToolkit(a.registry).WithMemory(a.memory).
		WithHooks(hooks).
		WithFileScope(scopes)

	a.agent = capAgent
	a.session = ap.NewSession(capAgent, a.memory, ap.SessWithID(a.sessionID))

	return nil
}

// Process 处理用户输入
func (a *CodecastAgent) Process(ctx context.Context, userInput string) error {
	a.selectVariantForInput(userInput, false)
	if a.ab != nil {
		a.ab.StartRound(a.currentVariant)
	}
	resp, err := a.session.Ask(ctx, userInput)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("请求超时: %w\n💡 请检查: 1) 网络连接是否正常 2) 模型响应是否过慢，可尝试切换模型", err)
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("请求已取消: %w", err)
		}
		return err
	}
	// 使用 TUI 渲染器输出 Markdown
	fmt.Print(a.renderer.RenderMarkdown(resp.Content))
	fmt.Println()
	a.recordCost(resp.Usage, "chat")

	// 自动学习
	if mem := a.GetAutoMemory(); mem != nil {
		go mem.LearnFromConversation(userInput, resp.Content)
	}

	return nil
}

// ProcessWithResult 处理用户输入并返回结构化结果
func (a *CodecastAgent) ProcessWithResult(ctx context.Context, userInput string) (*ProcessResult, error) {
	a.selectVariantForInput(userInput, false)
	if a.ab != nil {
		a.ab.StartRound(a.currentVariant)
	}
	resp, err := a.session.Ask(ctx, userInput)
	if err != nil {
		return nil, err
	}
	a.recordCost(resp.Usage, "chat")
	return &ProcessResult{
		Content:   resp.Content,
		Usage:     resp.Usage,
		Metrics:   resp.Metrics,
		SessionID: a.sessionID,
	}, nil
}

// ProcessResult 处理结果
type ProcessResult struct {
	Content   string          `json:"content"`
	Usage     ap.AgentUsage   `json:"usage"`
	Metrics   ap.AgentMetrics `json:"metrics"`
	ToolsUsed []string        `json:"tools_used"`
	SessionID string          `json:"session_id"`
}

// recordCost 记录成本（含 F8 预算控制）
func (a *CodecastAgent) recordCost(usage ap.AgentUsage, command string) {
	if usage.TotalTokens == 0 {
		return
	}

	// F8: 预算控制记录
	var costUSD float64
	if a.budgetCtrl != nil {
		info := model.FindModel(a.config.Model)
		if info != nil {
			costUSD = float64(usage.PromptTokens)/1000*info.CostPer1kIn +
				float64(usage.CompletionTokens)/1000*info.CostPer1kOut
		}
		_ = a.budgetCtrl.Record(budget.UsageRecord{
			Model:            a.config.Model,
			Provider:         a.config.Provider,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			CostUSD:          costUSD,
			SessionID:        a.sessionID,
		})
	}

	// 成本追踪器
	if a.costTracker != nil {
		llmUsage := ap.Usage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
		// v0.3.0 A/B 埋点：附带当前 prompt variant 名，便于后续按变体聚合分析
		variant := ""
		if a.config != nil {
			variant = a.config.PromptVariant
		}
		_ = a.costTracker.RecordWithVariant(a.config.Model, a.config.Provider, "", command, llmUsage, variant)
	}

	// A/B 收敛器：结束当前轮记录 outcome。
	// success 默认 true（成功完成 LLM 调用）；/fb n 或 /undo 撤销会调
	// ABIntegration.ResolveSuccess(false) 修正。
	if a.ab != nil {
		a.ab.EndRound(usage.TotalTokens, costUSD, true)
	}
}

// ClearContext 清除会话上下文
func (a *CodecastAgent) ClearContext() {
	a.session = nil
	a.session = ap.NewSession(a.agent, a.memory)
	a.sessionID = a.session.SessionID()
}

// GetStats 返回 Agent 统计信息
func (a *CodecastAgent) GetStats() ap.AgentStats {
	return a.agent.Stats()
}

// Close 关闭 Agent 资源
func (a *CodecastAgent) Close() error {
	if a.mcpRegistry != nil {
		a.mcpRegistry.StopAll()
	}
	if a.costTracker != nil {
		_ = a.costTracker.Close()
	}
	if a.ab != nil {
		_ = a.ab.Close()
	}
	if a.memory != nil {
		return a.memory.Close()
	}
	return nil
}

// GetABIntegration 返回 A/B 收敛器（A/B 评估闭环入口）。
// /ab /fb 斜杠命令通过它读取/写入状态。
func (a *CodecastAgent) GetABIntegration() *ABIntegration {
	return a.ab
}

// RefreshConfig 重新从磁盘读 config 并应用到 agent。
// /ab apply 等改完 ~/.codecast/config.yaml 后调用，使新权重立即生效。
// 注意：不会重新构建 systemPrompt（避免历史 token 浪费），
// 后续轮次由 PromptResolver 通过 cfg.PromptWeights 读取最新值。
func (a *CodecastAgent) RefreshConfig() error {
	if a == nil {
		return nil
	}
	// 1) 重新加载 config
	newCfg := config.Load()
	if newCfg == nil {
		return fmt.Errorf("加载 config 失败")
	}
	// 2) 更新内存中的 cfg（避免重建 agent）
	a.config = newCfg
	return nil
}

func getCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// loadProjectRules 加载项目规则（DI-5: 含自动学习规则）
func loadProjectRules() string {
	loader := rules.NewLoader(getCurrentDir())
	rs, err := loader.Load()
	if err != nil || rs == nil {
		return ""
	}
	// 应用模板变量
	homeDir, _ := os.UserHomeDir()
	merged := rules.ApplyTemplateVariables(rs.Merged, getCurrentDir(), homeDir, runtime.GOOS)

	// 加载自动学习规则
	autoRulesPath := filepath.Join(getCurrentDir(), ".codecast", "auto_rules.md")
	if data, err := os.ReadFile(autoRulesPath); err == nil && len(data) > 0 {
		merged += "\n\n[自动学习规则]\n" + string(data)
	}

	return merged
}

// GetTokenBudget 获取当前模型的 Token 预算
func (a *CodecastAgent) GetTokenBudget() *ctxpkg.TokenBudget {
	return ctxpkg.NewTokenBudget(a.config.Model, 0)
}
