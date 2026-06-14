package agent

// router_select.go: 每轮 Process 入口的变体选择。

// selectVariantForInput 根据当前 strategy 决策变体，并写入 a.currentVariant。
//   - hasTools 暂时固定为 false：未来可让 LLM 在调用工具时回调本函数
//   - 路由命中时，本函数会覆盖 a.config.PromptVariant（仅在内存，不写盘）
//     → 让 ab.RecordOutcome 记录到正确的 variant 名下
func (a *CodecastAgent) selectVariantForInput(userInput string, hasTools bool) {
	if a.routerPrompt == nil || a.config == nil {
		return
	}
	// 收集可用变体（与 PromptResolver.Registry().Names() 等价）
	available := a.availableVariantNames()

	strategy := a.config.PromptStrategy
	variant, source := a.routerPrompt.SelectForInput(
		strategy,
		a.config.PromptVariant,
		a.config.PromptWeights,
		userInput,
		hasTools,
		available,
	)
	a.currentVariant = variant
	// 仅在 routed 时才覆盖 cfg.PromptVariant（避免破坏 fixed 策略）
	if strategy == "routed" || strategy == "task-aware" || strategy == "router" {
		a.config.PromptVariant = variant
		_ = source // 静默，未来可写到 console 调试
	}
}

// availableVariantNames 返回 PromptResolver 中所有已注册变体名。
func (a *CodecastAgent) availableVariantNames() []string {
	resolver := DefaultResolver()
	if resolver == nil {
		return []string{"default"}
	}
	return resolver.Registry().Names()
}
