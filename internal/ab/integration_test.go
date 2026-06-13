package ab

import (
	"math"
	"path/filepath"
	"testing"
)

// TestAB_E2E_Loop 模拟"启动 → 喂数据 → 关闭 → 重启 → 应用"全流程。
func TestAB_E2E_Loop(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "ab_state.json")

	// 1) 启动：空状态
	c1, err := Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	c1.Config().StatePath = statePath

	// 2) 喂数据：default 失败率高，concise 成功率高
	for i := 0; i < 10; i++ {
		c1.RecordOutcome("default", 1000, 0.02, false) // 50% 成功
		c1.RecordOutcome("concise", 500, 0.01, true)   // 100% 成功
	}
	if err := c1.Save(); err != nil {
		t.Fatal(err)
	}

	// 3) 重启：状态应被恢复
	c2, err := Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	available := []string{"default", "concise"}

	// 4) ComputeWeights：concise 应得到比 default 高的权重
	weights := c2.ComputeWeights(available)
	if weights["concise"] <= weights["default"] {
		t.Errorf("expected concise weight > default, got %+v", weights)
	}

	// 5) Suggest：冷启动已过 → 选 best
	SetRandSource(newFixedSource(42))
	c2.Config().Epsilon = 0 // 纯利用
	pick := c2.CandidateSelector(available)
	// Note: CandidateSelector 的 pick 是 best score；default 算 score=1/0.02*0.75=37.5
	// concise score=1/0.01*1=100 → concise 应胜
	if pick != "concise" {
		t.Errorf("epsilon=0 should pick concise, got %q", pick)
	}
}

// TestAB_WilsonIntegration 验证 Report 输出含 CI。
func TestAB_WilsonIntegration(t *testing.T) {
	c := NewConverger(&Config{Epsilon: 0.1, MinSamples: 5})
	available := []string{"a", "b"}
	// a: 10/10 成功, b: 2/10 成功
	for i := 0; i < 10; i++ {
		c.RecordOutcome("a", 100, 0.01, true)
	}
	for i := 0; i < 10; i++ {
		success := i < 2
		c.RecordOutcome("b", 100, 0.01, success)
	}
	report := c.Report(available)

	// 应包含 "a" / "b" / "95%CI" / "100%" 标识
	if !contains(report, "95%CI") {
		t.Error("report should contain '95%CI'")
	}
	if !contains(report, "显著") {
		t.Error("report should conclude a 显著优于 b")
	}
}

// TestAB_LatencyIntegration 验证 ReportWithLatency 输出 p50/p95。
func TestAB_LatencyIntegration(t *testing.T) {
	c := NewConverger(&Config{Epsilon: 0.1, MinSamples: 2})
	lat := NewLatencyTracker(100)
	available := []string{"x"}
	for i := 0; i < 5; i++ {
		c.RecordOutcome("x", 100, 0.01, true)
		lat.Record("x", float64(100+i*50)) // 100, 150, 200, 250, 300
	}
	report := c.ReportWithLatency(available, lat)
	if !contains(report, "p50") {
		t.Error("report should contain 'p50'")
	}
	if !contains(report, "p95") {
		t.Error("report should contain 'p95'")
	}
}

// TestAB_ResetPreservesConfig 验证 Reset 不清 Config。
func TestAB_ResetPreservesConfig(t *testing.T) {
	c := NewConverger(&Config{Epsilon: 0.3, MinSamples: 7, MinWeight: 2})
	c.RecordOutcome("x", 1, 0.01, true)
	c.Reset()
	if c.Config().Epsilon != 0.3 {
		t.Errorf("epsilon should be preserved, got %v", c.Config().Epsilon)
	}
	if c.Config().MinWeight != 2 {
		t.Errorf("min_weight should be preserved, got %v", c.Config().MinWeight)
	}
}

// TestAB_ScoreMonotonicWithSuccess 评分应随成功率单调上升（avg_cost 固定时）。
func TestAB_ScoreMonotonicWithSuccess(t *testing.T) {
	prev := 0.0
	for successes := 0; successes <= 10; successes += 2 {
		s := &VariantStat{Samples: 10, TotalCostUSD: 0.1, Successes: successes}
		score := s.Score()
		if score < prev-math.Abs(prev)*0.01 {
			t.Errorf("score should be non-decreasing with success, prev=%v now=%v", prev, score)
		}
		prev = score
	}
}

// TestAB_TimeDecay_Like 验证随着 Sample 数增大，Score 稳定（不漂移）。
// 这是 sanity check：记录 1000 次后分数不能突变成 NaN/Inf。
func TestAB_StableScoreAtScale(t *testing.T) {
	s := &VariantStat{Samples: 1000, TotalCostUSD: 10.0, Successes: 950}
	score := s.Score()
	if math.IsNaN(score) || math.IsInf(score, 0) {
		t.Errorf("score at scale should be finite, got %v", score)
	}
	if score <= 0 {
		t.Errorf("expected positive score, got %v", score)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// newFixedSource 构造确定性伪随机源（用于单测）。
func newFixedSource(seed int64) *lockedSource {
	return &lockedSource{}
}

// lockedSource 是个 trivial 伪随机源，返回 0（永远走 exploit 而非 explore）。
// 避免冷启动期外还去抽样"探索"，让 epsilon=0 行为可预测。
type lockedSource struct{}

func (s *lockedSource) Int63() int64 { return 0 }
func (s *lockedSource) Seed(int64)   {}
