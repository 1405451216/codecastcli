package agent

// agent_processing.go: 处理流程和上下文管理（从 agent.go 拆分）
//
// Phase 5.2 拆分：将处理逻辑、成本记录、上下文压缩、摘要等
// 从 agent.go 分离，使 agent.go 聚焦于构建和生命周期。
//
// 包含：
//   - Process / ProcessWithResult: 用户输入处理主循环
//   - recordCost: 成本记录（F8 预算控制 + A/B 埋点 + 学习路由记录）
//   - ClearContext / injectCompressedContext: 上下文管理
//   - asyncSummarize: 异步摘要压缩（P-03 修复后已实现真实压缩）

import (
	"context"
	"fmt"
	"strings"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/budget"
	"codecast/cli/internal/model"
	"codecast/cli/internal/provider"
	"codecast/cli/internal/routing"
	ctxpkg "codecast/cli/internal/context"
)

// ProcessResult 处理结果
type ProcessResult struct {
	Content   string          `json:"content"`
	Usage     ap.AgentUsage   `json:"usage"`
	Metrics   ap.AgentMetrics `json:"metrics"`
	ToolsUsed []string        `json:"tools_used"`
	SessionID string          `json:"session_id"`
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
	// High 修复：使用 sessionMu 保护 agent 访问
	a.sessionMu.RLock()
	curAgent := a.agent
	a.sessionMu.RUnlock()
	// 走 agent.Run 而非 sess.Ask：session.Response 不暴露 Usage/Metrics，
	// 成本/预算/AB 收敛需要完整 Response。
	resp, err := curAgent.Run(ctx, ap.UserMessage(a.injectCompressedContext(userInput)))
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
	// High 修复：使用 sessionMu 保护 agent 访问
	a.sessionMu.RLock()
	curAgent := a.agent
	a.sessionMu.RUnlock()
	// 走 agent.Run 而非 sess.Ask：返回完整 Response（Usage/Metrics 可用）
	resp, err := curAgent.Run(ctx, ap.UserMessage(a.injectCompressedContext(userInput)))
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

// asyncSummarize runs summarization in a background goroutine after each turn.
// It uses a mutex to prevent concurrent summarization. If SummaryModel is
// configured, it creates a separate provider for summarization; otherwise it
// uses the main model.
//
// P-03 修复：实现了真实的 LLM 摘要压缩，替代原来直接 ClearContext 的做法。
// 使用 ctxpkg.Compressor 进行智能压缩，保留最近 N 轮对话的关键信息。
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
		currentAgent := a.agent
		currentConfig := a.config
		sessionID := a.sessionID
		a.sessionMu.RUnlock()

		// 从底层 memory store 读取历史（避开 primordia 内部包）
		history := a.loadSessionHistoryAP(ctx, sessionID)
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
				summaryAgentCap, err := ap.NewAgent("SummaryAgent", "You are a conversation summarizer.", summaryProvider)
				if err != nil {
					return "", fmt.Errorf("创建摘要 Agent 失败: %w", err)
				}
				resp, err := summaryAgentCap.Run(ctx, ap.UserMessage(prompt))
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
