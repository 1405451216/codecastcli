package agent

// agent.go: CodecastAgent 核心定义与构建工厂
//
// Phase 5.2 拆分后，本文件仅保留：
//   - CodecastAgent 结构体定义
//   - newAgent 内部工厂函数（启动流水线）
//
// 已拆分到独立文件的职责：
//   - agent_hooks.go:     Hook 构建函数（buildPermHook, buildScopeHook 等）
//   - agent_getters.go:   ~20 个 Getter 方法 + 操作方法
//   - agent_processing.go: Process/ProcessWithResult/recordCost/上下文压缩/摘要
//   - agent_lifecycle.go:  New/NewWithSession/Close/RefreshConfig/SwitchModel
//   - mcp_integration.go:  MCP 连接与管理
//   - stream.go:          StreamProcess 流式处理
//   - prompt.go / prompt_resolver.go: 系统提示词构建
//   - retry.go:           重试逻辑
//   - router_cache.go / router_select.go: 智能路由
//   - ab_integration.go:  A/B 测试集成
//   - tui_adapter.go:     TUI 适配器

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/budget"
	"codecast/cli/internal/checkpoint"
	"codecast/cli/internal/config"
	"codecast/cli/internal/cost"
	"codecast/cli/internal/diff"
	"codecast/cli/internal/indexer"
	"codecast/cli/internal/lazy"
	automem "codecast/cli/internal/memory"
	"codecast/cli/internal/model"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/provider"
	"codecast/cli/internal/routing"
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
	processing atomic.Bool
	// C-02 修复：sessionMu 保护 session 字段的并发访问
	sessionMu sync.RWMutex

	// compressedHistory 存储摘要压缩后的消息（由 SummarizeContext 写入）。
	compressedHistory []ap.Message
	compressedMu      sync.RWMutex

	// summarizeMu prevents concurrent summarization.
	summarizeMu sync.Mutex
	// summarizing indicates whether a summarization is in progress.
	summarizing bool

	// scopes 文件访问范围（从 config.Scopes 注入，用于 buildScopeHook）
	scopes []string
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
	// F-01 修复：FileScope 路径校验必须在权限检查之前执行
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"."}
	}
	if len(scopes) > 0 && !(len(scopes) == 1 && scopes[0] == ".") {
		hooks.Register(ap.HookBeforeTool, buildScopeHook(scopes))
	}
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

	scopePolicy := ap.NewFileScopePolicy()
	scopePolicy.SetScope("codecast", scopes)

	capAgent, err := ap.NewAgent("CodecastAgent", systemPrompt, llmProvider,
		ap.WithMaxTurns(20),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Agent 失败: %w", err)
	}
	capAgent = capAgent.WithToolkit(registry).WithMemory(memory).
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
			fmt.Fprintf(os.Stderr, "⚠ 学习路由状态加载失败，使用空状态: %v\n", err)
		}
	}

	// P3: 语义索引（可选启用）
	var semanticIdx *semantic.SemanticIndex
	if cfg.SemanticIndex.Enabled {
		var embedder semantic.EmbeddingProvider
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
				SymbolExtractor: semantic.SymbolExtractorFunc(func(path string, content []byte, language string) []semantic.SymbolInfo {
					tags := indexer.ExtractTags(path, content, language)
					return semantic.ExtractSymbolsFromTags(toTagLikeSlice(tags))
				}),
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ 语义索引初始化失败: %v\n", err)
			} else {
				if err := si.Load(); err != nil {
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
		scopes:         scopes,
	}, nil
}
