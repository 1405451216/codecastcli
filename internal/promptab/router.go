package promptab

// router.go: 任务感知的变体路由（L1 关键词规则 + L2 复杂度启发式）。
//
// 路由优先级：
//   1) L1 关键词规则（danger / write / refactor / question）→ 固定变体
//   2) L2 复杂度启发式（基于输入特征打分） → default / concise
//   3) 回落：调用方传入的 weighted 策略
//
// 设计动机：
//   - 让"危险操作"自动 safety-first（不依赖 A/B 探索）
//   - 让"简单问题"自动 concise（省钱）
//   - 让"长任务"自动 default（质量）
//   - 路由不污染 A/B 评估：路由命中时直接用对应变体，不进 weighted 抽样
//
// 用法：
//
//	rt := NewRouter()
//	rt.LoadRules(rules)        // L1 关键词规则
//	decision := rt.Route(in)   // L1 + L2
//	if decision.Variant != "" { use decision.Variant }
//	else { use weighted strategy }

import (
	"sort"
	"strings"
	"unicode"
)

// Router 任务感知的路由决策器。
type Router struct {
	// rules 是 L1 关键词规则集合，按 priority 降序、index 升序匹配。
	rules []*RouteRule
	// complexity 是 L2 复杂度阈值（可外部覆盖）。
	complexity ComplexityConfig
}

// ComplexityConfig L2 复杂度启发式的阈值。
// 用户可通过 .codecast/prompts/routing.yaml 的 `complexity:` 段覆盖。
type ComplexityConfig struct {
	// LongTaskChars 长任务阈值（> 此字符数判定为"长任务" → default）
	LongTaskChars int `yaml:"long_task_chars"`
	// ShortQuestionChars 短问句阈值（≤ 此字符数且是问句 → concise）
	ShortQuestionChars int `yaml:"short_question_chars"`
	// HasToolHint 出现工具/写动作关键词 → default
	HasToolHint []string `yaml:"has_tool_hint"`
}

// DefaultComplexityConfig 返回默认阈值。
// 调过几轮交互后这些数字已经基本合理：
//   - 普通问答 ≈ 30 字
//   - 中等任务 ≈ 100-200 字
//   - 重构/改文件 ≈ 200+ 字
func DefaultComplexityConfig() ComplexityConfig {
	return ComplexityConfig{
		LongTaskChars:       200,
		ShortQuestionChars:  50,
		HasToolHint:         []string{"重构", "refactor", "重写", "改", "fix", "修复", "添加", "add", "删除", "delete", "实现", "implement", "create", "创建", "写", "write"},
	}
}

// NewRouter 构造路由决策器。
func NewRouter() *Router {
	return &Router{
		rules:      nil,
		complexity: DefaultComplexityConfig(), // 默认带工具关键词
	}
}

// LoadRules 加载 L1 关键词规则集（追加，不清空已有）。
// 同 priority 多次出现的按追加顺序评估。
func (r *Router) LoadRules(rules []*RouteRule) {
	if r == nil {
		return
	}
	r.rules = append(r.rules, rules...)
	// 稳定排序：priority 降序，index 升序
	sort.SliceStable(r.rules, func(i, j int) bool {
		if r.rules[i].Priority != r.rules[j].Priority {
			return r.rules[i].Priority > r.rules[j].Priority
		}
		return r.rules[i].Index < r.rules[j].Index
	})
}

// SetComplexityConfig 覆盖 L2 阈值。
// 仅在传入字段为零值时使用默认值；非零值视为用户明确指定。
func (r *Router) SetComplexityConfig(cfg ComplexityConfig) {
	if r == nil {
		return
	}
	def := DefaultComplexityConfig()
	if cfg.LongTaskChars <= 0 {
		cfg.LongTaskChars = def.LongTaskChars
	}
	if cfg.ShortQuestionChars <= 0 {
		cfg.ShortQuestionChars = def.ShortQuestionChars
	}
	if len(cfg.HasToolHint) == 0 {
		cfg.HasToolHint = def.HasToolHint
	}
	r.complexity = cfg
}

// RouteRule L1 关键词路由规则。
// 匹配逻辑：用户输入 lower-case 后 contains 任一 Keywords 子串（任一即命中）。
// 不同 Priority 的规则按优先级匹配：danger 100 / write 80 / refactor 60 / question 40 / default 20。
type RouteRule struct {
	// Name 给人看（"danger" / "write" / "question"）
	Name string `yaml:"name"`
	// Variant 命中后使用的变体名
	Variant string `yaml:"variant"`
	// Keywords 触发词列表（任一匹配即命中），不区分大小写
	Keywords []string `yaml:"keywords"`
	// Priority 数字越大越优先；同优先级按 LoadRules 顺序
	Priority int `yaml:"priority"`
	// Index 内部排序用（同 Priority 时按此）
	Index int `yaml:"-"`
	// Description 给人看的备注
	Description string `yaml:"description"`
}

// DefaultRouteRules 返回编译时嵌入的默认 L1 规则。
// 用户可通过 ~/.codecast/prompts/routing.yaml 的 `rules:` 段追加/覆盖。
func DefaultRouteRules() []*RouteRule {
	defaults := []RouteRule{
		{Name: "danger", Variant: "safety-first", Priority: 100, Description: "危险操作：删除/重置/强制覆盖",
			Keywords: []string{"rm -rf", "drop table", "force push", "git push -f", "format c:", "del /f", "删除所有", "清空所有", "重置数据库", "truncate"}},
		{Name: "secret", Variant: "safety-first", Priority: 95, Description: "敏感凭据操作",
			Keywords: []string{"api key", "apikey", "密码", "password", "secret", "token", "凭据"}},
		{Name: "review", Variant: "code-reviewer", Priority: 70, Description: "代码审查请求",
			Keywords: []string{"review", "审查", "code review", "pr review", "看看这段", "帮我检查"}},
		{Name: "plan", Variant: "decision-tree", Priority: 60, Description: "先规划再动手",
			Keywords: []string{"规划", "plan", "思路", "怎么做", "how to", "approach"}},
		{Name: "explain", Variant: "mentor-coach", Priority: 50, Description: "教学/讲解型",
			Keywords: []string{"解释", "explain", "什么是", "what is", "为什么", "why", "教我", "teach me"}},
		{Name: "shell", Variant: "shell-only", Priority: 55, Description: "shell 单行命令场景",
			Keywords: []string{"shell", "终端", "terminal", "bash", "zsh", "ps aux", "grep"}},
	}
	out := make([]*RouteRule, 0, len(defaults))
	for i := range defaults {
		r := defaults[i]
		r.Index = i
		out = append(out, &r)
	}
	return out
}

// RouteInput 路由决策的输入。
type RouteInput struct {
	// UserInput 用户原始输入
	UserInput string
	// HasTools 这次任务是否需要写文件 / 跑命令（由调用方判断，可选）
	HasTools bool
	// Available 已知变体名列表（用于校验 Rule.Variant 实际存在）
	Available []string
}

// RouteDecision 路由决策结果。
type RouteDecision struct {
	// Variant 推荐的变体名；空字符串表示"未匹配，请调用方走 weighted 策略"
	Variant string
	// Source 决策来源：`l1:<rule_name>` / `l2:<reason>` / ""
	Source string
	// Reason 给人看的理由
	Reason string
	// Score 仅 L2 决策有：复杂度分（0-100）
	Score int
}

// Route 按优先级 L1 → L2 → "" 决策。
// 注意：本函数不返回 error；规则错误（变体名不存在）会被静默忽略，
// 因为 L1/L2 的目的是"挑变体"，不挑不出来的部分交给 A/B 兜底。
//
// 行为说明：当 in.Available 为空时跳过可用性校验（让上层 Registry 兜底）。
func (r *Router) Route(in RouteInput) RouteDecision {
	if r == nil {
		return RouteDecision{}
	}
	available := make(map[string]bool, len(in.Available))
	for _, n := range in.Available {
		available[n] = true
	}
	skipAvailabilityCheck := len(in.Available) == 0

	// === L1 关键词规则 ===
	lower := strings.ToLower(in.UserInput)
	for _, rule := range r.rules {
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				if !skipAvailabilityCheck && !available[rule.Variant] {
					continue
				}
				return RouteDecision{
					Variant: rule.Variant,
					Source:  "l1:" + rule.Name,
					Reason:  "L1 关键词命中 '" + kw + "' → " + rule.Name + " (" + rule.Variant + ")",
				}
			}
		}
	}

	// === L2 复杂度启发式 ===
	score, l2Reason := r.scoreComplexity(in.UserInput, in.HasTools)
	// 启发式分桶：>=60 走 default（兜住"中等 + 工具诉求"场景），
	// <=20 走 concise（兜住"极短问句"场景），
	// 21~59 不决策，回落给 A/B（让数据说话）
	switch {
	case score >= 60:
		if available["default"] {
			return RouteDecision{
				Variant: "default",
				Source:  "l2:complexity",
				Reason:  "L2 复杂度 " + l2Reason + " → default",
				Score:   score,
			}
		}
	case score <= 20:
		if available["concise"] {
			return RouteDecision{
				Variant: "concise",
				Source:  "l2:complexity",
				Reason:  "L2 复杂度 " + l2Reason + " → concise",
				Score:   score,
			}
		}
	}
	// score 在 21-69 之间：未匹配，回落
	return RouteDecision{
		Source: "",
		Reason: "L1/L2 未匹配（score=" + itoa(score) + "）→ 调用方走 A/B 策略",
		Score:  score,
	}
}

// scoreComplexity 返回 0-100 的复杂度分 + 给人看的理由。
// 设计意图：
//   - 0-20：极简问题（"这是什么？"）→ concise
//   - 21-69：通用（落到 A/B）
//   - 70-100：长/多步任务 → default
func (r *Router) scoreComplexity(input string, hasTools bool) (int, string) {
	if input == "" {
		return 0, "空输入"
	}
	score := 0
	reasons := []string{}

	// 1) 字符数（含中英文统一按 rune 算）
	runes := []rune(strings.TrimSpace(input))
	n := len(runes)
	if n >= r.complexity.LongTaskChars {
		score += 50
		reasons = append(reasons, "长输入("+itoa(n)+"字符)")
	} else if n >= 80 { // 中等：>80 字符
		score += 30
		reasons = append(reasons, "中长输入("+itoa(n)+"字符)")
	} else if n <= r.complexity.ShortQuestionChars {
		score -= 20
		reasons = append(reasons, "短输入("+itoa(n)+"字符)")
	}

	// 2) 工具/写动作关键词 — 这是"硬信号"，命中即大幅加分
	lower := strings.ToLower(input)
	hits := 0
	for _, kw := range r.complexity.HasToolHint {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hits++
		}
	}
	if hits > 0 {
		// 单次命中给 35，多命中累加（封顶 65）
		score += 35 + 10*minInt(hits-1, 3)
		reasons = append(reasons, "含工具诉求("+itoa(hits)+"命中)")
	}

	// 3) 调用方告知"这次需要工具"
	if hasTools {
		score += 30
		reasons = append(reasons, "agent 检测到工具需求")
	}

	// 4) 多步标记：换行/句号/分号（含中英文标点）
	steps := strings.Count(input, "\n") + strings.Count(input, "。") +
		strings.Count(input, ".") + strings.Count(input, ";") +
		strings.Count(input, "；") + strings.Count(input, "，") +
		strings.Count(input, ",") + strings.Count(input, ":") +
		strings.Count(input, "：")
	if steps >= 4 {
		score += 25
		reasons = append(reasons, "多步任务("+itoa(steps)+"标记)")
	} else if steps >= 2 {
		score += 10
		reasons = append(reasons, "含分句("+itoa(steps)+"标记)")
	}

	// 5) 问号结尾 → 倾向 concise
	if endsWithQuestionMark(input) && n < r.complexity.LongTaskChars {
		score -= 15
		reasons = append(reasons, "问句结尾")
	}

	// 截断到 [0, 100]
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	reason := joinReasons(reasons)
	return score, reason
}

func endsWithQuestionMark(s string) bool {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return false
	}
	last := r[len(r)-1]
	return last == '?' || last == '？' || unicode.IsPunct(last) && strings.ContainsRune("?!？！", last)
}

func joinReasons(rs []string) string {
	out := ""
	for i, r := range rs {
		if i > 0 {
			out += ", "
		}
		out += r
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func itoa(n int) string {
	// 避免引入 strconv 包 import（与 stats.go 风格一致）
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
