package agent

import (
	"os"
	"strconv"
	"sync"

	"codecast/cli/internal/indexer"
	"codecast/cli/internal/promptab"
)

// PromptResolver 把 buildSystemPrompt 的输入适配到 promptab.Registry 的
// 渲染流程。当外部 variant 可用时优先使用；否则回落到 Go 内置的 buildSystemPrompt。
//
// 设计动机：
//  1. buildSystemPrompt 仍在被新代码路径（promptab）使用，保留作为 fallback
//  2. 用户可通过 ~/.codecast/prompts/*.yaml 替换任意 section，无需重编
//  3. CI / A/B 测试可通过 SetSelector 强制选择特定 variant
//  4. 失败安全：Registry 加载失败、变体缺失等情况下，自动用原 buildSystemPrompt
//
// 用法：
//
//	res := NewPromptResolver()                       // 默认从 ~/.codecast/prompts 加载
//	res.SetSelector(promptab.Selector{Strategy: ...})  // 可选：CLI / config 控制
//	prompt := res.Build(goos, cwd, rules, idx, mode, budgetUSD)
type PromptResolver struct {
	registry *promptab.Registry
	selector promptab.Selector
	mu       sync.RWMutex
}

var (
	defaultResolver     *PromptResolver
	defaultResolverOnce sync.Once
)

// DefaultResolver 返回进程级单例，第一次调用时初始化。
// 加载顺序（后加载覆盖先加载）：
//  1. 编译时嵌入（default / concise / safety-first）
//  2. ~/.codecast/prompts/（用户级，可选）
//  3. .codecast/prompts/（项目级，可选）
func DefaultResolver() *PromptResolver {
	defaultResolverOnce.Do(func() {
		defaultResolver = NewPromptResolver()
	})
	return defaultResolver
}

// NewPromptResolver 构造 resolver，加载嵌入与用户级 variant。
// 项目级 .codecast/prompts/ 通过 LoadProjectDir 在运行时按 cwd 加载，
// 这样用户在哪个项目就用哪套 prompts。
func NewPromptResolver() *PromptResolver {
	r := promptab.NewRegistry()
	r.Register(promptab.EmbeddedVariants()...)
	// 用户级（~/.codecast/prompts）
	if home, err := os.UserHomeDir(); err == nil {
		_ = r.LoadDir(home + "/.codecast/prompts")
	}
	return &PromptResolver{registry: r}
}

// LoadProjectDir 从指定目录加载项目级 prompts。
// 多次调用是安全的：同名 variant 后加载的会覆盖前者。
// 目录不存在不算错（用户项目里可能没有自定义 prompts）。
func (p *PromptResolver) LoadProjectDir(dir string) error {
	if p == nil || p.registry == nil {
		return nil
	}
	return p.registry.LoadDir(dir)
}

// Registry 暴露底层 promptab.Registry，便于 /prompt 斜杠命令列举/查找变体。
// 调用方不应直接修改返回的 registry。
func (p *PromptResolver) Registry() *promptab.Registry {
	return p.registry
}

// Selector 返回当前选择策略（拷贝），便于 /prompt current 展示。
func (p *PromptResolver) Selector() promptab.Selector {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.selector
}

// SetSelector 设置当前选择策略。
// 影响后续 Build 调用；不影响已渲染的字符串。
func (p *PromptResolver) SetSelector(sel promptab.Selector) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.selector = sel
}

// Build 渲染最终系统提示词。优先走 registry；失败回落到原 buildSystemPrompt。
//
// userInput 变体参数：
//   - 空字符串：保留向后兼容（路由不可用时不影响结果）
//   - 非空：若 selector.Strategy == SelectRouted，会基于此输入做 L1/L2 决策
//   - hasTools：调用方判断本次是否需要写文件/跑命令（可选）
func (p *PromptResolver) Build(goos, cwd, projectRules string, idx *indexer.Indexer, mode string, budgetUSD float64, userInput ...string) string {
	p.mu.RLock()
	sel := p.selector
	p.mu.RUnlock()

	// 可变参数：[userInput, hasTools]（位置敏感）
	// 为了避免破坏既有的 6 参数调用，把 userInput 放在第一个 variadic 参数
	if len(userInput) > 0 {
		sel.UserInput = userInput[0]
	}
	if len(userInput) > 1 {
		// 第二个参数当作 has_tools（约定 "true"/"false"）
		sel.HasTools = userInput[1] == "true"
	}
	// 若 router 未注入 → 用默认（构造一次复用）
	if sel.Strategy == promptab.SelectRouted && sel.Router == nil {
		sel.Router = promptab.NewDefaultRouter()
	}

	in := promptab.RenderInputs{
		OS:           goos,
		CWD:          cwd,
		Mode:         mode,
		Budget:       strconv.FormatFloat(budgetUSD, 'f', 2, 64),
		ModeAdvice:   modeAdviceFor(mode),
		ProjectRules: projectRules,
	}
	if idx != nil {
		in.FileTree = idx.GetFileTree()
	}

	v, _, err := p.registry.ResolveWithStrategy(sel)
	if err == nil && v != nil {
		return v.Render(in)
	}

	// fallback：原 buildSystemPrompt
	return buildSystemPrompt(goos, cwd, projectRules, idx, mode, budgetUSD)
}

// modeAdviceFor 给出与 buildSystemPrompt 内 mode 段落一致的说明。
func modeAdviceFor(mode string) string {
	switch mode {
	case "full-auto":
		return "所有工具调用无需确认，可直接执行写操作。但 scope 之外的文件仍然禁止。"
	case "auto-edit":
		return "文件编辑类操作自动通过；shell 写操作（rm/git push/curl POST）仍需用户确认。"
	default:
		return "所有写操作前必须通过 permission 提示获得用户确认。在 prompt 中清晰说明即将做什么。"
	}
}

// SelectorConfig 描述从哪里读取选择策略的来源。
type SelectorConfig struct {
	Variant  string
	Strategy string
	Weights  map[string]int
}

// ToSelector 把配置转为 promptab.Selector。
// 未知 strategy 字符串回退到 SelectFixed。
func (s SelectorConfig) ToSelector() promptab.Selector {
	out := promptab.Selector{
		Strategy: promptab.SelectFixed,
		Fixed:    s.Variant,
		Weights:  s.Weights,
	}
	switch s.Strategy {
	case "round-robin":
		out.Strategy = promptab.SelectRoundRobin
	case "weighted", "weighted-random":
		out.Strategy = promptab.SelectWeightedRandom
	case "routed", "router", "task-aware":
		out.Strategy = promptab.SelectRouted
	}
	return out
}
