package agent

// agent_getters.go: CodecastAgent 的 Getter 方法集合（从 agent.go 拆分）
//
// Phase 5.2 拆分：将 ~20 个 getter 方法从 agent.go 分离，
// 使 agent.go 聚焦于 Agent 构建和生命周期管理。
//
// 包含：
//   - 会话/状态 getter: GetSessionID, IsProcessing, GetStats
//   - 组件 getter: GetRegistry, PermMgr, GetIndexer, GetSemanticIndex
//   - 子系统 getter: GetLearningRouter, GetAutoMemory, GetModelSwitcher,
//     GetRouter, GetSandbox, GetDiffPreviewer, GetRenderer,
//     GetUndoManager, GetCheckpointManager, GetBudgetController,
//     GetMCPWarnings, GetSharedDB, GetABIntegration, GetTokenBudget
//   - 操作方法: UndoLastFileChange, CapabilityAgent, InjectCompressedContext
//   - 类型适配器: tagLikeAdapter, toTagLikeSlice

import (
	"database/sql"
	"fmt"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/budget"
	"codecast/cli/internal/checkpoint"
	"codecast/cli/internal/context"
	"codecast/cli/internal/diff"
	"codecast/cli/internal/indexer"
	"codecast/cli/internal/memory"
	"codecast/cli/internal/model"
	"codecast/cli/internal/permission"
	"codecast/cli/internal/routing"
	"codecast/cli/internal/sandbox"
	"codecast/cli/internal/semantic"
	"codecast/cli/internal/tui"
	"codecast/cli/internal/undo"
)

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
func (a *CodecastAgent) GetAutoMemory() *memory.AutoPersister {
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

// GetABIntegration 返回 A/B 收敛器（A/B 评估闭环入口）。
// /ab /fb 斜杠命令通过它读取/写入状态。
func (a *CodecastAgent) GetABIntegration() *ABIntegration {
	return a.ab
}

// GetTokenBudget 获取当前模型的 Token 预算
func (a *CodecastAgent) GetTokenBudget() *context.TokenBudget {
	return context.NewTokenBudget(a.config.Model, 0)
}

// CapabilityAgent 返回底层 *ap.CapabilityAgent，供 TUI 适配器直接调用 StreamRun。
// High 修复：使用 sessionMu 保护 agent 访问
func (a *CodecastAgent) CapabilityAgent() *ap.CapabilityAgent {
	a.sessionMu.RLock()
	ag := a.agent
	a.sessionMu.RUnlock()
	return ag
}

// InjectCompressedContext 是 injectCompressedContext 的导出版本，
// 供 TUI 适配器（CodecastAgentAdapter）调用。
func (a *CodecastAgent) InjectCompressedContext(userInput string) string {
	return a.injectCompressedContext(userInput)
}

// GetStats 返回 Agent 统计信息
// High 修复：使用 sessionMu 保护 agent 访问
func (a *CodecastAgent) GetStats() ap.AgentStats {
	a.sessionMu.RLock()
	ag := a.agent
	a.sessionMu.RUnlock()
	return ag.Stats()
}
