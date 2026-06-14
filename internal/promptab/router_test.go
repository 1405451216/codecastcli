package promptab

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRouter_L1_DangerKeywords(t *testing.T) {
	r := NewDefaultRouter()
	avail := []string{"default", "concise", "safety-first", "code-reviewer"}
	cases := []struct {
		input    string
		expected string
		source   string
	}{
		{"rm -rf /", "safety-first", "l1:danger"},
		{"force push to main", "safety-first", "l1:danger"},
		{"drop table users", "safety-first", "l1:danger"},
		{"truncate logs", "safety-first", "l1:danger"},
		{"我的 api key 在哪？", "safety-first", "l1:secret"},
	}
	for _, tc := range cases {
		dec := r.Route(RouteInput{UserInput: tc.input, Available: avail})
		if dec.Variant != tc.expected {
			t.Errorf("input=%q: variant=%q want %q", tc.input, dec.Variant, tc.expected)
		}
		if dec.Source != tc.source {
			t.Errorf("input=%q: source=%q want %q", tc.input, dec.Source, tc.source)
		}
	}
}

func TestRouter_L1_ReviewAndExplain(t *testing.T) {
	r := NewDefaultRouter()
	avail := DefaultRouteRules() // 任意 non-nil slice
	_ = avail
	availNames := []string{"default", "concise", "safety-first", "code-reviewer", "mentor-coach", "decision-tree", "shell-only"}
	cases := []struct {
		input    string
		expected string
	}{
		{"帮我 review 这段代码", "code-reviewer"},
		{"请 code review 我的 PR", "code-reviewer"},
		{"解释一下什么是 goroutine", "mentor-coach"},
		{"什么是 context.Context？", "mentor-coach"},
		{"先用 plan 模式思考", "decision-tree"},
		{"how to implement this?", "decision-tree"},
		{"在 shell 里跑 ps aux", "shell-only"},
	}
	for _, tc := range cases {
		dec := r.Route(RouteInput{UserInput: tc.input, Available: availNames})
		if dec.Variant != tc.expected {
			t.Errorf("input=%q: variant=%q want %q", tc.input, dec.Variant, tc.expected)
		}
	}
}

func TestRouter_L2_Complexity(t *testing.T) {
	r := NewRouter() // 不用默认规则
	avail := []string{"default", "concise", "safety-first", "code-reviewer", "mentor-coach", "decision-tree", "shell-only"}
	cases := []struct {
		input    string
		expected string // "" = 期望回落
		minScore int
		maxScore int
	}{
		// 极短问句
		{"为什么？", "concise", 0, 20},
		{"这是什么？", "concise", 0, 20},
		// 中等任务
		{"帮我看下这个文件结构", "", 0, 100}, // 长度中等，未明确匹配
		// 长任务
		{"重构 user 模块，把所有 service 层抽到独立文件，加单元测试，覆盖 happy path / 边界 / 错误码，保留向后兼容", "default", 60, 100},
		// 多步 + 工具诉求
		{"1. 创建新文件\n2. 实现 parser\n3. 写测试", "default", 60, 100},
	}
	for _, tc := range cases {
		dec := r.Route(RouteInput{UserInput: tc.input, Available: avail})
		t.Logf("input=%q → variant=%q score=%d reason=%q", tc.input, dec.Variant, dec.Score, dec.Reason)
		if tc.expected != "" && dec.Variant != tc.expected {
			t.Errorf("input=%q: variant=%q want %q (score=%d, source=%q)",
				tc.input, dec.Variant, tc.expected, dec.Score, dec.Source)
		}
		if dec.Score < tc.minScore || dec.Score > tc.maxScore {
			t.Errorf("input=%q: score=%d not in [%d,%d]", tc.input, dec.Score, tc.minScore, tc.maxScore)
		}
	}
}

func TestRouter_UnavailableVariantFallsThrough(t *testing.T) {
	// L1 规则指向不存在的变体 → 跳过，落到 L2
	r := NewRouter()
	r.LoadRules([]*RouteRule{
		{Name: "myrule", Variant: "nonexistent", Priority: 100, Keywords: []string{"foo"}},
	})
	// "foo bar" 短句：L2 应判为 score=0（短输入+无工具）→ concise
	dec := r.Route(RouteInput{
		UserInput: "foo bar",
		Available: []string{"default", "concise", "safety-first"},
	})
	if dec.Variant != "concise" {
		t.Errorf("short input should map to concise, got %q (source=%s, score=%d)",
			dec.Variant, dec.Source, dec.Score)
	}
	// 同样输入但 available 不含 concise → 期望回落到 "" 让 A/B 处理
	dec2 := r.Route(RouteInput{
		UserInput: "foo bar",
		Available: []string{"default", "safety-first"},
	})
	if dec2.Variant != "" {
		t.Errorf("without concise, L2 should return empty (fall through to A/B), got %q", dec2.Variant)
	}
}

func TestRouter_YAMLLoad(t *testing.T) {
	dir := t.TempDir()
	yaml := `
rules:
  - name: my-review
    variant: code-reviewer
    priority: 80
    description: "项目级 code review 规则"
    keywords: ["lgtm", "nit"]
complexity:
  long_task_chars: 300
  short_question_chars: 30
  has_tool_hint: ["foo", "bar"]
`
	path := filepath.Join(dir, "routing.yaml")
	if err := writeFile(path, yaml); err != nil {
		t.Fatal(err)
	}

	r := NewRouter()
	if err := r.LoadRoutingFromFile(path); err != nil {
		t.Fatal(err)
	}
	avail := []string{"default", "code-reviewer", "concise"}
	dec := r.Route(RouteInput{UserInput: "lgtm but nit: typo", Available: avail})
	if dec.Variant != "code-reviewer" {
		t.Errorf("YAML rule should match 'lgtm', got %q", dec.Variant)
	}
	if r.complexity.LongTaskChars != 300 {
		t.Errorf("YAML complexity.LongTaskChars = %d, want 300", r.complexity.LongTaskChars)
	}
}

func TestRouter_LoadDir(t *testing.T) {
	dir := t.TempDir()
	yaml1 := `
rules:
  - name: r1
    variant: default
    priority: 50
    keywords: ["alpha"]
`
	yaml2 := `
rules:
  - name: r2
    variant: concise
    priority: 60
    keywords: ["beta"]
`
	if err := writeFile(filepath.Join(dir, "routing-a.yaml"), yaml1); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "routing-b.yaml"), yaml2); err != nil {
		t.Fatal(err)
	}
	// 不匹配 routing*.yaml 模式的应被忽略
	if err := writeFile(filepath.Join(dir, "other.yaml"), "rules: []"); err != nil {
		t.Fatal(err)
	}

	r := NewRouter()
	if err := r.LoadRoutingFromDir(dir); err != nil {
		t.Fatal(err)
	}
	avail := []string{"default", "concise"}
	if dec := r.Route(RouteInput{UserInput: "alpha", Available: avail}); dec.Variant != "default" {
		t.Errorf("alpha → default expected, got %q", dec.Variant)
	}
	if dec := r.Route(RouteInput{UserInput: "beta", Available: avail}); dec.Variant != "concise" {
		t.Errorf("beta → concise expected, got %q", dec.Variant)
	}
}

func TestRouter_PriorityOrder(t *testing.T) {
	r := NewRouter()
	r.LoadRules([]*RouteRule{
		{Name: "weak", Variant: "concise", Priority: 10, Keywords: []string{"common"}},
		{Name: "strong", Variant: "safety-first", Priority: 100, Keywords: []string{"common"}},
	})
	avail := []string{"default", "concise", "safety-first"}
	dec := r.Route(RouteInput{UserInput: "common word", Available: avail})
	if dec.Variant != "safety-first" {
		t.Errorf("higher priority should win, got %q", dec.Variant)
	}
}

func TestComplexity_QuestionMark(t *testing.T) {
	r := NewRouter()
	cases := []struct {
		input string
		high  bool // true = score 应 >= 70
	}{
		{"这是？", false},      // 短问句
		{"为什么 goroutine 这么设计，背后有什么 trade-off？", false}, // 中长 + 问号
	}
	for _, tc := range cases {
		score, _ := r.scoreComplexity(tc.input, false)
		if tc.high && score < 70 {
			t.Errorf("input=%q: expected high score, got %d", tc.input, score)
		}
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
