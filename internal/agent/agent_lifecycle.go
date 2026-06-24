package agent

// agent_lifecycle.go: Agent 生命周期管理（从 agent.go 拆分）
//
// Phase 5.2 拆分：将 Close、RefreshConfig、SwitchModel 等生命周期方法
// 从 agent.go 分离，使 agent.go 仅保留核心构建逻辑。
//
// 包含：
//   - New / NewWithSession: 公共构造函数
//   - Close: 资源清理
//   - RefreshConfig: 配置热重载
//   - SwitchModel: 模型切换（含 F-01/F-09 修复）

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	"codecast/cli/internal/indexer"
	"codecast/cli/internal/provider"
	"codecast/cli/internal/rules"
)

// New 创建一个新的 Codecast Agent
func New(cfg *config.Config) (*CodecastAgent, error) {
	return newAgent(cfg, "")
}

// loadSessionHistoryAP 从底层 memory store 读取当前会话的消息历史并
// 转换为 ap.Message 切片，供 compressor.Compress 使用。
// 替代 session.History()（其返回的 session.Message 类型属于 primordia
// 内部包，外部模块无法 import）。
func (a *CodecastAgent) loadSessionHistoryAP(ctx context.Context, sessionID string) []ap.Message {
	a.sessionMu.RLock()
	mem := a.memory
	a.sessionMu.RUnlock()
	if mem == nil || sessionID == "" {
		return nil
	}
	// ap.Memory.Search 返回 []*Episode（不是 *SearchResult），
	// 接收 *SearchOptions。空 query 配合 SessionID 可拿到该会话全部消息。
	episodes, err := mem.Search(ctx, "", &ap.SearchOptions{
		SessionID: sessionID,
		Limit:     200,
	})
	if err != nil || len(episodes) == 0 {
		return nil
	}
	out := make([]ap.Message, 0, len(episodes))
	for _, ep := range episodes {
		if ep == nil || ep.Role == "" {
			continue
		}
		out = append(out, ap.Message{
			Role:    ap.Role(ep.Role),
			Content: ep.Content,
		})
	}
	return out
}

// NewWithSession 创建一个恢复指定会话 ID 的 Codecast Agent
func NewWithSession(cfg *config.Config, sessionID string) (*CodecastAgent, error) {
	return newAgent(cfg, sessionID)
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
	// COR-03 修复：关闭 memory 前先关闭 sharedDB 连接
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

// SwitchModel 切换模型并重建 Provider（DI-4: Model Switcher→Provider 深度集成）
//
// F-01 修复：重建时重新注册 buildScopeHook，确保新 agent 的 FileScope 生效。
// F-09 修复：重建时也走 PromptResolver，让运行时切换后依然使用一致的 prompt 选择策略。
// F-09 修复：重建索引器，避免新模型看到陈旧文件树。
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

	// F-01 修复：FileScope 路径校验必须在权限检查之前执行
	if len(a.scopes) > 0 && !(len(a.scopes) == 1 && a.scopes[0] == ".") {
		hooks.Register(ap.HookBeforeTool, buildScopeHook(a.scopes))
	}
	hooks.Register(ap.HookBeforeTool, buildPermHook(permMgr))
	hooks.Register(ap.HookBeforeTool, buildDiffPreviewHook(a.diffPreview))
	hooks.Register(ap.HookBeforeTool, buildUndoHook(a.undoMgr))
	hooks.Register(ap.HookBeforeTool, buildCheckpointHook(a.checkpointMgr))

	// 重新应用 ToolPermission（HITL 集成）
	applyToolPermissions(registry, permMgr)

	projectRules := loadProjectRules()
	// F-09 修复：SwitchModel 重建时也走 PromptResolver
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

	// F-01 + F-09 修复：SwitchModel 重建 agent 时必须重新注入 scope policy
	scopePolicy := ap.NewFileScopePolicy()
	scopePolicy.SetScope("codecast", scopes)

	// F-09 修复：SwitchModel 也重新构建索引器
	newIndexer := indexer.NewIndexer(getCurrentDir())
	newIndexer.Build()

	// High 修复：使用 sessionMu 保护 registry 访问
	a.sessionMu.RLock()
	currentRegistry := a.registry
	currentMemory := a.memory
	a.sessionMu.RUnlock()

	capAgent, err := ap.NewAgent("CodecastAgent", systemPrompt, newProvider,
		ap.WithMaxTurns(20),
	)
	if err != nil {
		return fmt.Errorf("重建 Agent 失败: %w", err)
	}
	capAgent = capAgent.WithToolkit(currentRegistry).WithMemory(currentMemory).
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

// getCurrentDir 返回当前工作目录
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
