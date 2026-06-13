package promptab

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// TestAllVariantsRenderable 矩阵测试：所有嵌入变体 × 多组上下文都能成功渲染。
//
// 目的：
//  1. 防回归：保证任何变体在合理输入下都能渲染为非空字符串
//  2. 防泄漏：保证渲染结果不含未替换的 {{var}} 模板占位符
//  3. 防爆栈：保证渲染结果长度有上下界（不会无限膨胀或截断到 0）
//
// 这是 CI 变体兼容性测试的核心——任何新增 section/variant 都会自动被覆盖。
func TestAllVariantsRenderable(t *testing.T) {
	vs := EmbeddedVariants()
	if len(vs) < 2 {
		t.Fatalf("expected >= 2 embedded variants, got %d", len(vs))
	}

	// 上下文矩阵
	contexts := []RenderInputs{
		{OS: "linux", CWD: "/home/user/proj", Mode: "suggest", Budget: "0", ModeAdvice: "advice"},
		{OS: "darwin", CWD: "/Users/dev", Mode: "auto-edit", Budget: "5.50", ModeAdvice: "advice"},
		{OS: "windows", CWD: "C:\\work", Mode: "full-auto", Budget: "12.34", ModeAdvice: "advice"},
		{OS: "linux", CWD: "/tmp", Mode: "suggest", Budget: "0", ModeAdvice: "advice",
			ProjectRules: "使用 Tab 缩进\n所有函数必须有注释",
			FileTree:     "src/main.go\ninternal/agent.go"},
		{}, // 全空上下文也应能渲染
	}

	for _, v := range vs {
		t.Run(v.Name, func(t *testing.T) {
			// EmbeddedVariants 不会自动 Parse；这里显式调一下，
			// 否则 Render 时 Sections 为空导致输出 0 字节
			_ = v.Parse()
			for i, ctx := range contexts {
				t.Run(fmt.Sprintf("ctx-%d", i), func(t *testing.T) {
					out := v.Render(ctx)
					if out == "" {
						t.Errorf("variant %q rendered empty for context %d", v.Name, i)
					}
					// 模板残留检测
					if hasUnresolvedPlaceholder(out) {
						t.Errorf("variant %q left unresolved placeholders: %s", v.Name, extractUnresolved(out))
					}
					// 长度上下界（嵌入式变体不会无限膨胀或过短）
					if len(out) < 100 {
						t.Errorf("variant %q too short (%d bytes) for context %d", v.Name, len(out), i)
					}
					if len(out) > 50_000 {
						t.Errorf("variant %q too long (%d bytes) for context %d", v.Name, len(out), i)
					}
				})
			}
		})
	}
}

// TestAllVariantsHaveCoreSections 保证所有变体至少有核心 section。
// 防止"作者偷懒只写 identity"导致渲染质量退化。
func TestAllVariantsHaveCoreSections(t *testing.T) {
	coreSections := []string{"identity", "tool_guide", "anti_patterns", "workflow"}
	for _, v := range EmbeddedVariants() {
		t.Run(v.Name, func(t *testing.T) {
			_ = v.Parse()
			for _, sec := range coreSections {
				if body := v.Get(sec); strings.TrimSpace(body) == "" {
					t.Errorf("variant %q missing/empty core section %q", v.Name, sec)
				}
			}
		})
	}
}

// TestVariantDeterministic 同一变体在同一输入下渲染必须幂等。
// 这关系到 A/B 测试的可信度——不能某次跑出不同结果。
func TestVariantDeterministic(t *testing.T) {
	in := RenderInputs{
		OS: "linux", CWD: "/test", Mode: "suggest", Budget: "1.00", ModeAdvice: "advice",
		ProjectRules: "rule1\nrule2",
		FileTree:     "file1\nfile2",
	}
	for _, v := range EmbeddedVariants() {
		t.Run(v.Name, func(t *testing.T) {
			first := v.Render(in)
			for i := 0; i < 5; i++ {
				again := v.Render(in)
				if first != again {
					t.Errorf("variant %q non-deterministic on iteration %d", v.Name, i)
				}
			}
		})
	}
}

// TestVariantWeightsInRange 验证权重选择不返回 weight=0 的变体。
// 业务语义：weight=0 等价"禁用"，必须严格遵守。
func TestVariantWeightsInRange(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	weights := map[string]int{
		"default":      5,
		"concise":      3,
		"safety-first": 0, // 应被禁用
	}
	for i := 0; i < 50; i++ {
		_, chosen, _ := r.ResolveWithStrategy(Selector{
			Strategy: SelectWeightedRandom,
			Weights:  weights,
		})
		if chosen == "safety-first" {
			t.Fatalf("weight=0 variant was chosen on iteration %d", i)
		}
	}
}

// TestRoundRobinCycles 验证 round-robin 能覆盖所有非空权重变体。
func TestRoundRobinCycles(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	weights := map[string]int{
		"default": 1, "concise": 1, "safety-first": 1,
	}
	seen := map[string]int{}
	for i := 0; i < 30; i++ {
		_, chosen, _ := r.ResolveWithStrategy(Selector{
			Strategy: SelectRoundRobin,
			Weights:  weights,
		})
		seen[chosen]++
	}
	for variant, count := range seen {
		if count == 0 {
			t.Errorf("variant %q never selected by round-robin", variant)
		}
	}
	if len(seen) < 3 {
		t.Errorf("expected all 3 variants to be selected at least once, got %d", len(seen))
	}
}

// TestOnSelectCallbackIsCalled 验证埋点回调被正确调用（为 cost tracker 集成做准备）。
func TestOnSelectCallbackIsCalled(t *testing.T) {
	r := NewRegistry()
	r.Register(EmbeddedVariants()...)
	events := make(chan [2]string, 10)
	r.SetOnSelect(func(name, source string) {
		events <- [2]string{name, source}
	})
	r.Resolve("concise")
	r.Resolve("default")
	r.ResolveWithStrategy(Selector{Strategy: SelectFixed, Fixed: "safety-first"})
	close(events)

	got := map[string]int{}
	for e := range events {
		got[e[0]]++
	}
	want := map[string]int{"concise": 1, "default": 1, "safety-first": 1}
	for variant, count := range want {
		if got[variant] != count {
			t.Errorf("variant %q callbacks = %d, want %d", variant, got[variant], count)
		}
	}
}

// hasUnresolvedPlaceholder 检测未替换的 {{var}} 残留
func hasUnresolvedPlaceholder(s string) bool {
	return unresolvedRe.MatchString(s)
}

var unresolvedRe = regexp.MustCompile(`\{\{[^}]+\}\}`)

func extractUnresolved(s string) string {
	return unresolvedRe.FindString(s)
}
