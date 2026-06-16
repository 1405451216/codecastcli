package agent

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	"codecast/cli/internal/routing"
	"codecast/cli/internal/rules"
	"codecast/cli/internal/sandbox"
	"codecast/cli/internal/semantic"
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
	lazySandbox *lazy.Value[*sandbox.Executor]
	lazyAutoMem *lazy.Value[*automem.AutoPersister]
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
	// learningRouter 学习型路由器（P1 L2，可选；启用后优先于 router）
	learningRouter *routing.LearningRouter
	// semanticIndex 语义索引（P3，可选；启用后提供代码库语义检索）
	semanticIndex *semantic.SemanticIndex

	// processing 标记 StreamProcess / Process 是否正在执行。
	// 由 SIGINT handler 读取，用于决定 Ctrl+C 是取消当前请求还是退出 REPL。
	processing atomic.Bool
	// C-02 修复：sessionMu 保护 session 字段的并发访问
	sessionMu sync.RWMutex

	// compressedHistory 存储摘要压缩后的消息（由 SummarizeContext 写入）。
	// 下一轮 Process / StreamProcess 时会将其作为上下文前缀注入到用户消息中，
	// 注入后清空，确保只注入一次。
	// C-03 修复：添加 compressedMu 保护并发读写。
	compressedHistory []ap.Message
	compressedMu      sync.RWMutex

	// summarizeMu prevents concurrent summarization.
	summarizeMu sync.Mutex
	// summarizing indicates whether a summarization is in progress.
	summarizing bool
}

// newAgent 内部工厂函数
//
// Phase 5.1: 启动流水线并行化。
// 将原本串行的初始化步骤按依赖关系拆分为多个阶段，
// 独立模块使用 goroutine 并行初始化，显著降低冷启动时间。
func newAgent(cfg *config.Config, sessionID string) (*CodecastAgent, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// ── Phase A: 并行初始化无依赖模块 ──────────────────────────
	type providerResult struct {
		lp  ap.Provider
		err error
	}
	type registryResult struct {
		reg *ap.ToolRegistry
		err error
	}
	type memoryResult struct {
		mem      ap.Memory
		sharedDB *sql.DB
		err      error
	}
	type permResult struct {
		mgr *permission.Manager
		err error
	}
	type rulesResult struct {
		rules string
	}
	type indexResult struct {
		idx *indexer.Indexer
		err error
	}

	var (
		provR  providerResult
		regR   registryResult
		memR   memoryResult
		permR  permResult
		rulesR rulesResult
		idxR   indexResult
		wgA    sync.WaitGroup
	)

	wgA.Add(6)

	// A1: Provider
	go func() {
		defer wgA.Done()
		provR.lp, provR.err = provider.CreateProvider(cfg)
	}()

	// A2: Toolkit + 自定义工具注册
	go func() {
		defer wgA.Done()
		reg, err := ap.DefaultToolkit(ap.ToolkitConfig{
			RootDir:     ".",
			EnableFS:    true,
			EnableShell: true,
			EnableWeb:   true,
		})
		if err != nil {
			regR.err = err
			return
		}
		reg.Register(customtools.NewEditFileTool())
		reg.Register(customtools.NewGrepSearchTool())
		reg.Register(customtools.NewGlobSearchTool())
		reg.Register(customtools.NewListFilesTool())
		reg.Register(customtools.NewDeleteFileTool(nil))
		reg.Register(customtools.NewMultiEditTool())
		reg.Register(customtools.NewReadFileTool())
		regR.reg = reg
	}()

	// A3: Memory store + shared DB
	go func() {
		defer wgA.Done()
		memPath := filepath.Join(config.GetConfigDir(), "memory.db")
		mem, err := ap.NewSQLiteStore(memPath)
		if err != nil {
			mem, err = ap.WithInMemory()
			if err != nil {
				memR.err = fmt.Errorf("初始化记忆存储失败: %w", err)
				return
			}
		}
		// 打开共享 DB 连接供 session.Manager 使用
		var sdb *sql.DB
		if mem != nil {
			var dbErr error
			sdb, dbErr = sql.Open("sqlite", memPath)
			if dbErr != nil {
				fmt.Fprintf(os.Stderr, "⚠  打开共享 DB 连接失败，session 将使用独立连接: %v\n", dbErr)
				sdb = nil
			} else if sdb != nil {
				sdb.SetMaxOpenConns(2)
			}
		}
		memR.mem = mem
		memR.sharedDB = sdb
	}()

	// A4: Permission manager
	go func() {
		defer wgA.Done()
		permMode := cfg.PermissionMode
		if permMode == "" {
			permMode = "auto-edit"
		}
		mgr, err := permission.NewManagerFromString(permMode)
		if err != nil {
			permR.err = fmt.Errorf("创建权限管理器失败: %w", err)
			return
		}
		mgr.BuildHITLConfig()
		permR.mgr = mgr
	}()

	// A5: Project rules
	go func() {
		defer wgA.Done()
		rulesR.rules = loadProjectRules()
	}()

	// A6: Indexer (最慢的模块之一，与 Provider 并行)
	go func() {
		defer wgA.Done()
		idx := indexer.NewIndexer(getCurrentDir())
		ui.StartSpinner("正在构建代码库索引...")
		// R5-C1 修复：回调中不再调用 GetIndex()（会与 BuildWithCallback 的写锁死锁）
		// 改为用局部计数器追踪进度
		var count int64
		if err := idx.BuildWithCallback(func(p string) {
			c := int(atomic.AddInt64(&count, 1))
			if c%50 == 0 {
				ui.UpdateSpinnerMessage(fmt.Sprintf("索引中: %d 个文件...", c))
			}
		}); err != nil {
			idxR.err = err
		}
		ui.StopSpinner()
		idxR.idx = idx
	}()

	wgA.Wait()

	// 收集 Phase A 错误
	if provR.err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w\n💡 请检查: 1) API Key 是否正确 2) Provider 配置是否正确 3) 使用 /model 切换到其他模型", provR.err)
	}
	if regR.err != nil {
		return nil, fmt.Errorf("创建工具集失败: %w", regR.err)
	}
	if memR.err != nil {
		return nil, memR.err
	}
	if permR.err != nil {
		return nil, permR.err
	}
	if idxR.err != nil {
		fmt.Fprintf(os.Stderr, "⚠ 代码库索引失败: %v\n", idxR.err)
	}

	registry := regR.reg
	memory := memR.mem
	sharedDB := memR.sharedDB
	permMgr := permR.mgr
	idx := idxR.idx
	projectRules := rulesR.rules
	llmProvider := provR.lp

	// 注入 sharedDB 到 session 包
	sessionpkg.SetSharedDB(sharedDB)

	// ── Phase B: SafeMode + 权限应用 ──────────────────────────
	if cfg.SafeMode {
		registry.Unregister("shell_execute")
		registry.Unregister("web_request")
		registry.Unregister("web_fetch")
		permMgr.AddDeny("shell_execute")
		permMgr.AddDeny("web_request")
		permMgr.AddDeny("web_fetch")
	}
	applyToolPermissions(registry, permMgr)

	// ── Phase C: MCP 连接 + 独立模块 ──────────────────────────
	mcpReg, mcpWarnings, _ := ConnectMCPServers(registry)
	// M-04 修复：在 newAgent 中也展示 MCP warnings
	if len(mcpWarnings) > 0 {
		for _, w := range mcpWarnings {
			fmt.Fprintf(os.Stderr, "⚠ MCP: %s\n", w)
		}
	}
	_ = mcpReg

	diffPrev := diff.NewPreviewer()
	undoMgr := undo.NewManager(getCurrentDir())
	cpCfg := checkpoint.DefaultConfig()
	cpCfg.Enabled = cfg.AutoCheckpoint
	cpCfg.AutoStash = cfg.AutoStash
	checkpointMgr := checkpoint.NewManager(getCurrentDir(), cpCfg)
	budgetCtrl := budget.NewController(budget.BudgetConfig{
		DailyLimitUSD:     cfg.DailyBudgetUSD,
		SessionLimitUSD:   cfg.SessionBudgetUSD,
		DailyTokenLimit:   cfg.DailyTokenLimit,
		SessionTokenLimit: cfg.SessionTokenLimit,
	})

	// ── Phase D: Hooks + System Prompt + Agent ──────────────────
	hooks := ap.NewHookManager()
	hooks.Register(ap.HookBeforeTool, buildPermHook(permMgr))
	hooks.Register(ap.HookBeforeTool, buildDiffPreviewHook(diffPrev))
	hooks.Register(ap.HookBeforeTool, buildUndoHook(undoMgr))
	hooks.Register(ap.HookBeforeTool, buildCheckpointHook(checkpointMgr))

	// System prompt（依赖 rules + indexer，两者已在 Phase A 并行完成）
	resolver := DefaultResolver()
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

	scopePolicy := ap.NewFileScopePolicy()
	scopePolicy.SetScope("codecast", scopes)
	// C-01 修复：FileScopePolicy 通过 AgentPrimordia 的 WithFileScope 集成到 Agent 中
	// 工具级别的路径检查由 util.HasUnsafePathSegment 实现

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

	// ── Phase E: 并行初始化非关键模块 ──────────────────────────
	type trackerResult struct {
		t *cost.Tracker
	}
	var trackR trackerResult
	var wgE sync.WaitGroup
	wgE.Add(3)

	var modelSwitch *model.Switcher
	var router *routing.ModelRouter
	var tuiRenderer *tui.Renderer

	go func() {
		defer wgE.Done()
		if t, err := cost.NewTracker(); err == nil {
			trackR.t = t
		}
	}()
	go func() {
		defer wgE.Done()
		modelSwitch = model.NewSwitcher(cfg)
		router = routing.NewModelRouter(cfg.Routing)
	}()
	go func() {
		defer wgE.Done()
		tuiRenderer = tui.NewRenderer()
	}()
	wgE.Wait()

	// F10: 懒加载非关键模块
	lazySandbox := lazy.NewValue(func() (*sandbox.Executor, error) {
		sandboxCfg := sandbox.DefaultConfig()
		return sandbox.NewExecutor(sandboxCfg), nil
	})
	lazyAutoMem := lazy.NewValue(func() (*automem.AutoPersister, error) {
		return automem.NewAutoPersister(getCurrentDir()), nil
	})

	// P1 L2: 学习型路由器（可选启用）
	var learningRouter *routing.LearningRouter
	if cfg.LearningRouting.Enabled {
		learningRouter = routing.NewLearningRouter(router, cfg.LearningRouting)
		if err := learningRouter.Load(); err != nil {
			// 加载失败不阻塞，降级为空状态
			fmt.Fprintf(os.Stderr, "⚠ 学习路由状态加载失败，使用空状态: %v\n", err)
		}
	}

	// P3: 语义索引（可选启用）
	var semanticIdx *semantic.SemanticIndex
	if cfg.SemanticIndex.Enabled {
		var embedder semantic.EmbeddingProvider
		// embedding 专用 key：为空则复用主 APIKey
		embKey := cfg.SemanticIndex.EmbeddingAPIKey
		if embKey == "" {
			embKey = cfg.APIKey
		}
		switch cfg.SemanticIndex.EmbeddingProvider {
		case "openai", "":
			ocfg := semantic.DefaultOpenAIEmbeddingConfig(embKey)
			if cfg.SemanticIndex.EmbeddingModel != "" {
				ocfg.Model = cfg.SemanticIndex.EmbeddingModel
			}
			if cfg.SemanticIndex.EmbeddingBaseURL != "" {
				ocfg.BaseURL = cfg.SemanticIndex.EmbeddingBaseURL
			} else if cfg.BaseURL != "" {
				ocfg.BaseURL = cfg.BaseURL
			}
			embedder = semantic.NewOpenAIEmbedding(ocfg)
		case "zhipu":
			zcfg := semantic.DefaultZhipuEmbeddingConfig(embKey)
			if cfg.SemanticIndex.EmbeddingModel != "" {
				zcfg.Model = cfg.SemanticIndex.EmbeddingModel
			}
			if cfg.SemanticIndex.EmbeddingBaseURL != "" {
				zcfg.BaseURL = cfg.SemanticIndex.EmbeddingBaseURL
			}
			embedder = semantic.NewZhipuEmbedding(zcfg)
		case "dashscope":
			dcfg := semantic.DefaultDashScopeEmbeddingConfig(embKey)
			if cfg.SemanticIndex.EmbeddingModel != "" {
				dcfg.Model = cfg.SemanticIndex.EmbeddingModel
			}
			if cfg.SemanticIndex.EmbeddingBaseURL != "" {
				dcfg.BaseURL = cfg.SemanticIndex.EmbeddingBaseURL
			}
			embedder = semantic.NewDashScopeEmbedding(dcfg)
		case "mock":
			embedder = semantic.NewMockEmbedding(32)
		default:
			fmt.Fprintf(os.Stderr, "⚠ 未知 embedding provider: %s\n", cfg.SemanticIndex.EmbeddingProvider)
		}

		if embedder != nil {
			statePath := cfg.SemanticIndex.StatePath
			if statePath == "" {
				if home, err := os.UserHomeDir(); err == nil {
					statePath = filepath.Join(home, ".codecast", "semantic_index.json")
				}
			}
			rootDir := cfg.ProjectRoot
			if rootDir == "" {
				rootDir = getCurrentDir()
			}
			si, err := semantic.NewSemanticIndex(semantic.SemanticIndexConfig{
				RootDir:       rootDir,
				Embedder:      embedder,
				StatePath:     statePath,
				MaxChunkLines: cfg.SemanticIndex.MaxChunkLines,
				// 注入 indexer.ExtractTags 作为符号提取器，实现按符号切块
				SymbolExtractor: semantic.SymbolExtractorFunc(func(path string, content []byte, language string) []semantic.SymbolInfo {
					tags := indexer.ExtractTags(path, content, language)
					return semantic.ExtractSymbolsFromTags(toTagLikeSlice(tags))
				}),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ 语义索引初始化失败: %v\n", err)
			} else {
				if err := si.Load(); err != nil {
					// 加载失败不阻塞，首次运行正常
					fmt.Fprintf(os.Stderr, "⚠ 语义索引加载失败（首次运行正常）: %v\n", err)
				}
				semanticIdx = si
			}
		}
	}

	return &CodecastAgent{
		agent:          capAgent,
		config:         cfg,
		memory:         memory,
		registry:       registry,
		session:        session,
		sessionID:      sessionID,
		costTracker:    trackR.t,
		mcpRegistry:    mcpReg,
		permMgr:        permMgr,
		indexer:        idx,
		modelSwitcher:  modelSwitch,
		diffPreview:    diffPrev,
		renderer:       tuiRenderer,
		undoMgr:        undoMgr,
		checkpointMgr:  checkpointMgr,
		budgetCtrl:     budgetCtrl,
		lazySandbox:    lazySandbox,
		lazyAutoMem:    lazyAutoMem,
		mcpWarnings:    mcpWarnings,
		sharedDB:       sharedDB,
		ab:             LoadABIntegration(""),
		routerPrompt:   NewRouterCache(),
		router:         router,
		learningRouter: learningRouter,
		semanticIndex:  semanticIdx,
	}, nil
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

// GetSemanticIndex 返回语义索引（P3，可能为 nil）
func (a *CodecastAgent) GetSemanticIndex() *semantic.SemanticIndex {
	if a == nil {
		return nil
	}
	return a.semanticIndex
}

// tagLikeAdapter 把 indexer.Tag 适配为 semantic.TagLike
type tagLikeAdapter struct {
	tag indexer.Tag
}

func (t tagLikeAdapter) GetName() string      { return t.tag.Name }
func (t tagLikeAdapter) GetKind() string      { return t.tag.Kind }
func (t tagLikeAdapter) GetLine() int         { return t.tag.Line }
func (t tagLikeAdapter) GetSignature() string { return t.tag.Signature }

// toTagLikeSlice 把 []indexer.Tag 转为 []semantic.TagLike
func toTagLikeSlice(tags []indexer.Tag) []semantic.TagLike {
	out := make([]semantic.TagLike, len(tags))
	for i, t := range tags {
		out[i] = tagLikeAdapter{tag: t}
	}
	return out
}

// GetLearningRouter 返回学习型路由器（P1 L2，可能为 nil）
func (a *CodecastAgent) GetLearningRouter() *routing.LearningRouter {
	if a == nil {
		return nil
	}
	return a.learningRouter
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
	a.sessionMu.RLock()
	defer a.sessionMu.RUnlock()
	return a.registry
}

// PermMgr 返回权限管理器
func (a *CodecastAgent) PermMgr() *permission.Manager {
	a.sessionMu.RLock()
	defer a.sessionMu.RUnlock()
	return a.permMgr
}

// GetIndexer 返回索引器
func (a *CodecastAgent) GetIndexer() *indexer.Indexer {
	a.sessionMu.RLock()
	defer a.sessionMu.RUnlock()
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
	
	// High 修复：使用 sessionMu 保护 permMgr 和 registry 访问
	a.sessionMu.RLock()
	permMgr := a.permMgr
	registry := a.registry
	a.sessionMu.RUnlock()
	
	hooks.Register(ap.HookBeforeTool, buildPermHook(permMgr))
	hooks.Register(ap.HookBeforeTool, buildDiffPreviewHook(a.diffPreview))
	hooks.Register(ap.HookBeforeTool, buildUndoHook(a.undoMgr))
	hooks.Register(ap.HookBeforeTool, buildCheckpointHook(a.checkpointMgr))

	// 重新应用 ToolPermission（HITL 集成）
	applyToolPermissions(registry, permMgr)

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
	// 注意：ap.RegistryWithScopePolicy 不存在，scope policy 通过工具内部路径检查实现
	// 每个工具已通过 util.HasUnsafePathSegment 做路径遍历防护

	// F-09 修复：SwitchModel 也重新构建索引器，避免新模型看到陈旧文件树
	newIndexer := indexer.NewIndexer(getCurrentDir())
	newIndexer.Build()

	// High 修复：使用 sessionMu 保护 registry 访问
	a.sessionMu.RLock()
	currentRegistry := a.registry
	currentMemory := a.memory
	a.sessionMu.RUnlock()

	capAgent := ap.NewAgent("CodecastAgent", systemPrompt, newProvider,
		ap.WithMaxTurns(20),
	).WithToolkit(currentRegistry).WithMemory(currentMemory).
		WithHooks(hooks).
		WithFileScope(scopes)

	// Critical 修复：SwitchModel 原子性更新 agent/session/indexer，持有 sessionMu
	newSession := ap.NewSession(capAgent, currentMemory, ap.SessWithID(a.sessionID))
	a.sessionMu.Lock()
	a.indexer = newIndexer
	a.agent = capAgent
	a.session = newSession
	a.sessionMu.Unlock()

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
	// High 修复：使用 sessionMu 保护 session 访问
	a.sessionMu.RLock()
	sess := a.session
	a.sessionMu.RUnlock()
	resp, err := sess.Ask(ctx, a.injectCompressedContext(userInput))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("请求超时: %w\n💡 请检查: 1) 网络连接是否正常 2) 模型响应是否过慢，可尝试切换模型", err)
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("请求已取消: %w", err)
		}
		return err
	}
	// R5-H11 修复：resp nil 检查
	if resp == nil {
		return fmt.Errorf("LLM 返回空响应")
	}
	// 使用 TUI 渲染器输出 Markdown
	fmt.Print(a.renderer.RenderMarkdown(resp.Content))
	fmt.Println()
	a.recordCost(resp.Usage, "chat")

	// 自动学习
	// High 修复：添加 context 超时控制，防止 goroutine 泄漏
	if mem := a.GetAutoMemory(); mem != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = mem.LearnFromConversation(userInput, resp.Content)
			_ = ctx // 使用 ctx 避免编译警告
		}()
	}

	// Trigger async summarization after each turn
	a.asyncSummarize()

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
	// High 修复：使用 sessionMu 保护 session 访问
	a.sessionMu.RLock()
	sess := a.session
	a.sessionMu.RUnlock()
	resp, err := sess.Ask(ctx, a.injectCompressedContext(userInput))
	if err != nil {
		return nil, err
	}
	// R5-H11 修复：resp nil 检查
	if resp == nil {
		return nil, fmt.Errorf("LLM 返回空响应")
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

	// High 修复：使用 sessionMu 保护 config 访问
	a.sessionMu.RLock()
	cfg := a.config
	budgetCtrl := a.budgetCtrl
	costTracker := a.costTracker
	ab := a.ab
	sessionID := a.sessionID
	a.sessionMu.RUnlock()

	// F8: 预算控制记录
	var costUSD float64
	if budgetCtrl != nil {
		info := model.FindModel(cfg.Model)
		if info != nil {
			costUSD = float64(usage.PromptTokens)/1000*info.CostPer1kIn +
				float64(usage.CompletionTokens)/1000*info.CostPer1kOut
		}
		_ = budgetCtrl.Record(budget.UsageRecord{
			Model:            cfg.Model,
			Provider:         cfg.Provider,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			CostUSD:          costUSD,
			SessionID:        sessionID,
		})
	}

	// 成本追踪器
	if costTracker != nil {
		llmUsage := ap.Usage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
		// v0.3.0 A/B 埋点：附带当前 prompt variant 名，便于后续按变体聚合分析
		variant := ""
		if cfg != nil {
			variant = cfg.PromptVariant
		}
		_ = costTracker.RecordWithVariant(cfg.Model, cfg.Provider, "", command, llmUsage, variant)
	}

	// A/B 收敛器：结束当前轮记录 outcome。
	// success 默认 true（成功完成 LLM 调用）；/fb n 或 /undo 撤销会调
	// ABIntegration.ResolveSuccess(false) 修正。
	if ab != nil {
		ab.EndRound(usage.TotalTokens, costUSD, true)
	}

	// P1 L2: 学习型路由器记录 outcome。
	// 用当前模型名 + 推断的 tier 记录，供下次路由决策参考。
	if a.learningRouter != nil && a.learningRouter.Config().Enabled {
		tier := routing.InferTierFromModel(cfg.Model, a.router)
		a.learningRouter.RecordOutcome(cfg.Model, tier, usage.TotalTokens, costUSD, true)
		// 异步保存（避免阻塞主循环）
		go func() {
			_ = a.learningRouter.Save()
		}()
	}
}

// inferTierFromModel 推断模型所属档位（用于学习路由记录）。
// 优先用 router 配置匹配，匹配不上按模型名启发式判断。
// 此函数在 routing 包中导出为 InferTierFromModel。

// ClearContext 清除会话上下文
func (a *CodecastAgent) ClearContext() {
	a.compressedMu.Lock()
	a.compressedHistory = nil
	a.compressedMu.Unlock()
	
	// Critical 修复：ClearContext 原子性重建 session，持有 sessionMu
	a.sessionMu.Lock()
	newSession := ap.NewSession(a.agent, a.memory)
	a.session = newSession
	a.sessionID = newSession.SessionID()
	a.sessionMu.Unlock()
}

// injectCompressedContext 将摘要压缩的上下文注入到用户输入前缀中。
// 调用后清空 compressedHistory，确保只注入一次。
// 返回带上下文前缀的用户输入；如果无压缩历史则原样返回。
// C-03 修复：使用 compressedMu 保护读写。
func (a *CodecastAgent) injectCompressedContext(userInput string) string {
	a.compressedMu.RLock()
	if len(a.compressedHistory) == 0 {
		a.compressedMu.RUnlock()
		return userInput
	}
	// 复制一份，释放读锁后再构建字符串
	history := make([]ap.Message, len(a.compressedHistory))
	copy(history, a.compressedHistory)
	a.compressedMu.RUnlock()

	var sb strings.Builder
	sb.WriteString("[上一轮对话的摘要上下文]\n")
	for _, m := range history {
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
	a.compressedMu.Lock()
	a.compressedHistory = nil
	a.compressedMu.Unlock()

	return sb.String() + userInput
}

// GetStats 返回 Agent 统计信息
// High 修复：使用 sessionMu 保护 agent 访问
func (a *CodecastAgent) GetStats() ap.AgentStats {
	a.sessionMu.RLock()
	ag := a.agent
	a.sessionMu.RUnlock()
	return ag.Stats()
}

// InjectCompressedContext 是 injectCompressedContext 的导出版本，
// 供 TUI 适配器（CodecastAgentAdapter）调用。
func (a *CodecastAgent) InjectCompressedContext(userInput string) string {
	return a.injectCompressedContext(userInput)
}

// CapabilityAgent 返回底层 *ap.CapabilityAgent，供 TUI 适配器直接调用 StreamRun。
// High 修复：使用 sessionMu 保护 agent 访问
func (a *CodecastAgent) CapabilityAgent() *ap.CapabilityAgent {
	a.sessionMu.RLock()
	ag := a.agent
	a.sessionMu.RUnlock()
	return ag
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
	// COR-03 修复：关闭 memory 前先清理 sharedDB 引用，避免 session.Manager 使用已关闭连接
	sessionpkg.SetSharedDB(nil)
	// R5-C8 修复：关闭 sharedDB 连接，防止连接泄漏
	if a.sharedDB != nil {
		_ = a.sharedDB.Close()
		a.sharedDB = nil
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
func (a *CodecastAgent) asyncSummarize() {
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

		// P-03 修复：添加 30s 超时，防止 LLM 无响应时 goroutine 永久阻塞
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Critical 修复：使用 sessionMu 保护 session/agent/config 访问
		a.sessionMu.RLock()
		if a.session == nil {
			a.sessionMu.RUnlock()
			return
		}
		history := a.session.History()
		currentAgent := a.agent
		currentConfig := a.config
		a.sessionMu.RUnlock()
		
		if len(history) <= 2 {
			return
		}

		// Check token budget threshold
		tb := a.GetTokenBudget()
		if tb == nil {
			return
		}
		threshold := currentConfig.ContextThreshold
		if threshold <= 0 {
			threshold = 0.7
		}
		if !tb.ShouldCompress(threshold) {
			return
		}

		// Build summary function — use SummaryModel if configured
		summaryFn := func(ctx context.Context, prompt string) (string, error) {
			if currentConfig.SummaryModel != "" {
				// Create a separate provider for summarization
				summaryCfg := *currentConfig
				summaryCfg.Model = currentConfig.SummaryModel
				summaryProvider, err := provider.CreateProvider(&summaryCfg)
				if err != nil {
					// Fallback to main provider
					resp, err := currentAgent.Run(ctx, ap.UserMessage(prompt))
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
			resp, err := currentAgent.Run(ctx, ap.UserMessage(prompt))
			if err != nil {
				return "", err
			}
			if resp == nil {
				return "", fmt.Errorf("LLM 返回空响应")
			}
			return resp.Content, nil
		}

		compressor := ctxpkg.NewCompressor(currentConfig.PreserveRecent)
		compressed, err := compressor.Compress(ctx, history, summaryFn)
		if err != nil || len(compressed) == 0 {
			return
		}

		// Store compressed history for next turn injection
		a.compressedMu.Lock()
		a.compressedHistory = compressed
		a.compressedMu.Unlock()

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

	// C-02 修复：session 访问需要加锁保护，防止与 asyncSummarize 并发冲突
	a.sessionMu.Lock()
	if a.session != nil {
		a.session.Reset()
	}
	a.sessionMu.Unlock()
	}()
}
