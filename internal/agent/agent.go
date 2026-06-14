package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

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
	"codecast/cli/internal/routing"
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
	// router 智能模型路由器（根据输入复杂度自动选择模型）
	router *routing.ModelRouter

	// processing 标记 StreamProcess / Process 是否正在执行。
	// 由 SIGINT handler 读取，用于决定 Ctrl+C 是取消当前请求还是退出 REPL。
	// 零值为 false，安全。
	processing atomic.Bool

	// compressedHistory 存储摘要压缩后的消息（由 SummarizeContext 写入）。
	// 下一轮 Process / StreamProcess 时会将其作为上下文前缀注入到用户消息中，
	// 注入后清空，确保只注入一次。
	compressedHistory []ap.Message

	// summarizeMu prevents concurrent summarization.
	summarizeMu sync.Mutex
	// summarizing indicates whether a summarization is in progress.
	summarizing bool
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
		permMode = "auto-edit"
	}
	permMgr, err := permission.NewManagerFromString(permMode)
	if err != nil {
		return nil, fmt.Errorf("创建权限管理器失败: %w", err)
	}

	// 初始化 HITL 配置（构建 InterruptPoints、AutoApproveTools、OnInterrupt 回调）
	permMgr.BuildHITLConfig()

	if cfg.SafeMode {
		// SafeMode: 从工具注册表中真正移除危险工具，而非仅加黑名单。
		// 这样 LLM 根本看不到这些工具，不会尝试调用。
		registry.Unregister("shell_execute")
		// web_request / web_fetch 可能不在 registry 中（取决于 DefaultToolkit 配置），
		// Unregister 不存在的工具是 no-op，安全调用。
		registry.Unregister("web_request")
		registry.Unregister("web_fetch")
		// 同时加入黑名单，防止 MCP 工具或动态注册的同名工具绕过
		permMgr.AddDeny("shell_execute")
		permMgr.AddDeny("web_request")
		permMgr.AddDeny("web_fetch")
	}

	// HITL 集成：为需要确认的工具设置 ToolPermission.ConfirmFunc。
	// 这是 AP 框架 executor 层面的权限检查（在 Hook 之前执行），
	// 与 buildPermHook 形成双重保障。
	// ConfirmFunc 返回 true=允许，false=拒绝。
	applyToolPermissions(registry, permMgr)

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

	// 创建智能模型路由器
	router := routing.NewModelRouter(cfg.Routing)

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
		router:        router,
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

// applyToolPermissions 为注册表中的工具设置 AP 框架级别的 ToolPermission。
//
// 这是 HITL 集成在 executor 层面的体现：
//   - 需要确认的工具（ShouldApprove=true）设置 RequireConfirmation=true + ConfirmFunc
//   - ConfirmFunc 调用 permission.ConfirmPrompt 获取用户输入
//   - ModeFullAuto 下所有工具不设 ConfirmFunc（自动放行）
//   - SafeMode 下被禁止的工具已在上方 Unregister，不会到达此处
//
// 与 buildPermHook 的关系：两者形成双重保障。
// ToolPermission 在 executor.Execute() 内部执行（更底层），
// buildPermHook 在 ReAct 循环的 HookBeforeTool 阶段执行（更上层）。
// 实际上 buildPermHook 先执行（因为 HookBeforeTool 在 executor 之前），
// 所以 ConfirmPrompt 只会被调用一次。
func applyToolPermissions(registry *ap.ToolRegistry, mgr *permission.Manager) {
	for _, toolName := range registry.List() {
		if !mgr.ShouldApprove(toolName) {
			continue
		}
		// 为需要确认的工具设置 ConfirmFunc
		perm := ap.ToolPermission{
			RequireConfirmation: true,
			ConfirmFunc: func(name string, args json.RawMessage) bool {
				result := permission.ConfirmPrompt(name, string(args))
				switch result.Action {
				case permission.ActionAllow:
					return true
				case permission.ActionAlwaysAllow:
					mgr.AddAutoAllow(name)
					return true
				case permission.ActionEditArgs:
					// ConfirmFunc 无法修改参数，只能允许/拒绝
					// 参数修改由 buildPermHook 处理
					return true
				default:
					return false
				}
			},
		}
		_ = registry.SetPermission(toolName, perm)
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
			// 使用标准库解析 JSON（安全：正确处理 \uXXXX、嵌套、控制字符等）
			var argsMap map[string]json.RawMessage
			if err := json.Unmarshal([]byte(hctx.ToolCall.Args), &argsMap); err != nil {
				return nil // 解析失败不阻塞，工具自身 Execute 会报错
			}
			filePath := jsonGetString(argsMap, "file_path")
			if filePath == "" {
				filePath = jsonGetString(argsMap, "path")
			}
			if filePath == "" {
				return nil
			}

			if toolName == "edit_file" {
				oldStr := jsonGetString(argsMap, "old_string")
				newStr := jsonGetString(argsMap, "new_string")
				if oldStr != "" {
					change := prev.PreviewEdit(filePath, oldStr, newStr)
					fmt.Println(tui.Styles.Warning.Render("即将修改文件: " + filePath))
					fmt.Println(tui.NewRenderer().RenderDiff(change.Diff))
				}
			} else if toolName == "write_file" {
				content := jsonGetString(argsMap, "content")
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

// jsonGetString 从 map[string]json.RawMessage 安全提取字符串值
// 使用 encoding/json 标准库，自动正确处理 \uXXXX、嵌套对象、控制字符等
// （修复了原手写 extractJSONField 的 P0 安全漏洞）
func jsonGetString(m map[string]json.RawMessage, key string) string {
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
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
			var argsMap map[string]json.RawMessage
			if err := json.Unmarshal([]byte(hctx.ToolCall.Args), &argsMap); err != nil {
				return nil // 解析失败不阻塞
			}
			filePath := jsonGetString(argsMap, "file_path")
			if filePath == "" {
				filePath = jsonGetString(argsMap, "path")
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

// IsProcessing 返回当前是否有请求在处理中（StreamProcess / Process / ProcessWithResult）。
// 由 SIGINT handler 读取，用于决定 Ctrl+C 是取消当前请求还是退出 REPL。
//
// 实现说明：使用 atomic.Bool 而非 mutex，因为：
//  1. 只做 Store/Load 单字段操作，atomic 已足够
//  2. SIGINT handler 是高频读路径，atomic.Load 比 mutex.RLock 便宜
//  3. 状态对一致性要求低 — 错过一次 "in-flight" 判定只会让用户多按一次 Ctrl+C
func (a *CodecastAgent) IsProcessing() bool {
	return a.processing.Load()
}

// GetRegistry 返回工具注册表
func (a *CodecastAgent) GetRegistry() *ap.ToolRegistry {
	return a.registry
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

// GetRouter 返回智能模型路由器
func (a *CodecastAgent) GetRouter() *routing.ModelRouter {
	return a.router
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

	// 重新应用 ToolPermission（HITL 集成）
	applyToolPermissions(a.registry, a.permMgr)

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
	// Task 1.6: 与 StreamProcess 保持一致，标记 processing 让 SIGINT 可取消
	a.processing.Store(true)
	defer a.processing.Store(false)

	a.selectVariantForInput(userInput, false)
	if a.ab != nil {
		a.ab.StartRound(a.currentVariant)
	}
	resp, err := a.session.Ask(ctx, a.injectCompressedContext(userInput))
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

	// Trigger async summarization after each turn
	a.asyncSummarize(ctx)

	return nil
}

// ProcessWithResult 处理用户输入并返回结构化结果
func (a *CodecastAgent) ProcessWithResult(ctx context.Context, userInput string) (*ProcessResult, error) {
	// Task 1.6: 与 StreamProcess / Process 保持一致
	a.processing.Store(true)
	defer a.processing.Store(false)

	a.selectVariantForInput(userInput, false)
	if a.ab != nil {
		a.ab.StartRound(a.currentVariant)
	}
	resp, err := a.session.Ask(ctx, a.injectCompressedContext(userInput))
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
	a.compressedHistory = nil
	a.session = nil
	a.session = ap.NewSession(a.agent, a.memory)
	a.sessionID = a.session.SessionID()
}

// injectCompressedContext 将摘要压缩的上下文注入到用户输入前缀中。
// 调用后清空 compressedHistory，确保只注入一次。
// 返回带上下文前缀的用户输入；如果无压缩历史则原样返回。
func (a *CodecastAgent) injectCompressedContext(userInput string) string {
	if len(a.compressedHistory) == 0 {
		return userInput
	}

	var sb strings.Builder
	sb.WriteString("[上一轮对话的摘要上下文]\n")
	for _, m := range a.compressedHistory {
		// 只注入摘要 system 消息和最近保留的消息（跳过原始 system prompt）
		if m.Role == ap.RoleSystem {
			if extra, ok := m.Metadata.Extra["summary"]; ok && extra == "true" {
				sb.WriteString(m.Content)
				sb.WriteString("\n")
			}
			// 跳过非摘要的 system 消息（原始 system prompt 由 agent 自身注入）
			continue
		}
		role := string(m.Role)
		content := m.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
	}
	sb.WriteString("\n[当前用户消息]\n")

	// 清空，确保只注入一次
	a.compressedHistory = nil

	return sb.String() + userInput
}

// GetStats 返回 Agent 统计信息
func (a *CodecastAgent) GetStats() ap.AgentStats {
	return a.agent.Stats()
}

// InjectCompressedContext 是 injectCompressedContext 的导出版本，
// 供 TUI 适配器（CodecastAgentAdapter）调用。
func (a *CodecastAgent) InjectCompressedContext(userInput string) string {
	return a.injectCompressedContext(userInput)
}

// CapabilityAgent 返回底层 *ap.CapabilityAgent，供 TUI 适配器直接调用 StreamRun。
func (a *CodecastAgent) CapabilityAgent() *ap.CapabilityAgent {
	return a.agent
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

// asyncSummarize runs summarization in a background goroutine after each turn.
// It uses a mutex to prevent concurrent summarization. If SummaryModel is
// configured, it creates a separate provider for summarization; otherwise it
// uses the main model.
func (a *CodecastAgent) asyncSummarize(ctx context.Context) {
	a.summarizeMu.Lock()
	if a.summarizing {
		a.summarizeMu.Unlock()
		return
	}
	a.summarizing = true
	a.summarizeMu.Unlock()

	go func() {
		defer func() {
			a.summarizeMu.Lock()
			a.summarizing = false
			a.summarizeMu.Unlock()
		}()

		// Check if summarization is needed using the compressor
		if a.session == nil {
			return
		}
		history := a.session.History()
		if len(history) <= 2 {
			return
		}

		// Check token budget threshold
		tb := a.GetTokenBudget()
		if tb == nil {
			return
		}
		threshold := a.config.ContextThreshold
		if threshold <= 0 {
			threshold = 0.7
		}
		if !tb.ShouldCompress(threshold) {
			return
		}

		// Build summary function — use SummaryModel if configured
		summaryFn := func(ctx context.Context, prompt string) (string, error) {
			if a.config.SummaryModel != "" {
				// Create a separate provider for summarization
				summaryCfg := *a.config
				summaryCfg.Model = a.config.SummaryModel
				summaryProvider, err := provider.CreateProvider(&summaryCfg)
				if err != nil {
					// Fallback to main provider
					resp, err := a.agent.Run(ctx, ap.UserMessage(prompt))
					if err != nil {
						return "", err
					}
					if resp == nil {
						return "", fmt.Errorf("LLM 返回空响应")
					}
					return resp.Content, nil
				}
				summaryAgent := ap.NewAgent("SummaryAgent", "You are a conversation summarizer.", summaryProvider)
				resp, err := summaryAgent.Run(ctx, ap.UserMessage(prompt))
				if err != nil {
					return "", err
				}
				if resp == nil {
					return "", fmt.Errorf("摘要 LLM 返回空响应")
				}
				return resp.Content, nil
			}
			// Use main model
			resp, err := a.agent.Run(ctx, ap.UserMessage(prompt))
			if err != nil {
				return "", err
			}
			if resp == nil {
				return "", fmt.Errorf("LLM 返回空响应")
			}
			return resp.Content, nil
		}

		compressor := ctxpkg.NewCompressor(a.config.PreserveRecent)
		compressed, err := compressor.Compress(ctx, history, summaryFn)
		if err != nil || len(compressed) == 0 {
			return
		}

		// Store compressed history for next turn injection
		a.compressedHistory = compressed

		// Persist summary to memory for session recovery
		if a.memory != nil {
			for _, m := range compressed {
				if m.Role == ap.RoleSystem {
					if extra, ok := m.Metadata.Extra["summary"]; ok && extra == "true" {
						_ = a.memory.Add(ctx, &ap.Episode{
							SessionID: a.sessionID,
							Role:      "system",
							Content:   m.Content,
							Metadata:  map[string]string{"summary": "true"},
						})
						break
					}
				}
			}
		}

		// Reset session since we have the summary saved
		a.session.Reset()
	}()
}
