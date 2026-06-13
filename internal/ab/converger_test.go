package ab

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVariantStatAvgCost(t *testing.T) {
	s := &VariantStat{Name: "x", Samples: 4, TotalCostUSD: 0.2}
	if s.AvgCost() != 0.05 {
		t.Errorf("avgCost = %v, want 0.05", s.AvgCost())
	}
}

func TestVariantStatScoreEmpty(t *testing.T) {
	s := &VariantStat{}
	if s.Score() != 0 {
		t.Errorf("empty stat should score 0")
	}
}

func TestVariantStatScoreWithSuccess(t *testing.T) {
	s := &VariantStat{Samples: 10, TotalCostUSD: 0.1, Successes: 10}
	// baseScore = 1/0.01 = 100, successRate = 1.0, weighted = 100 * 1.0 = 100
	if s.Score() != 100.0 {
		t.Errorf("score = %v, want 100", s.Score())
	}
}

func TestColdStartPicksUnderSampled(t *testing.T) {
	c := NewConverger(&Config{Epsilon: 0.1, MinSamples: 5})
	// 所有变体都没数据
	available := []string{"a", "b", "c"}
	for _, name := range available {
		c.RecordOutcome(name, 100, 0.01, true)
	}
	// 找一个 Samples < 5 的 → 冷启动候选
	picked := c.CandidateSelector(available)
	if picked == "" {
		t.Fatal("expected a cold-start candidate")
	}
	// 累计选 4 次后：a 应有 5 次，b/c 各 1 次（取决于具体序列）
	// 我们只验证：返回非空
}

func TestCandidateSelectorAfterColdStart(t *testing.T) {
	// 注入确定性 RNG：epsilon=0 → 纯利用
	SetRandSource(rand.NewSource(42))
	c := NewConverger(&Config{Epsilon: 0, MinSamples: 2})
	available := []string{"good", "bad"}
	// 喂数据让 good 评分远高于 bad
	for i := 0; i < 10; i++ {
		c.RecordOutcome("good", 100, 0.001, true)  // 极便宜
		c.RecordOutcome("bad", 100, 100.0, false)  // 极贵 + 失败
	}
	// 选 5 次，应该全部选 good
	for i := 0; i < 5; i++ {
		pick := c.CandidateSelector(available)
		if pick != "good" {
			t.Errorf("iteration %d picked %q, want good", i, pick)
		}
	}
}

func TestCandidateSelectorExplores(t *testing.T) {
	// epsilon=1 → 纯探索（应随机）
	SetRandSource(rand.NewSource(99))
	c := NewConverger(&Config{Epsilon: 1.0, MinSamples: 2})
	available := []string{"a", "b", "c"}
	// 喂数据让 a 评分最高
	for i := 0; i < 10; i++ {
		c.RecordOutcome("a", 100, 0.001, true)
		c.RecordOutcome("b", 100, 1.0, true)
		c.RecordOutcome("c", 100, 1.0, true)
	}
	// 选 20 次，至少应出现 b 或 c（纯探索时不应只选 a）
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		seen[c.CandidateSelector(available)] = true
	}
	if len(seen) < 2 {
		t.Errorf("epsilon=1 should explore; saw only %d variants", len(seen))
	}
}

func TestComputeWeightsScalesByScore(t *testing.T) {
	c := NewConverger(&Config{Epsilon: 0, MinSamples: 1, MinWeight: 1})
	available := []string{"expensive", "cheap"}
	// 让 cheap 评分 100，让 expensive 评分 1
	for i := 0; i < 5; i++ {
		c.RecordOutcome("cheap", 100, 0.001, true)      // score = 1/0.001 = 1000
		c.RecordOutcome("expensive", 1000, 100, true)  // score = 1/100 = 0.01
	}
	weights := c.ComputeWeights(available)
	// cheap 应是 10（最高），expensive 应是 ~1（最低）
	if weights["cheap"] < weights["expensive"] {
		t.Errorf("cheap weight (%d) should be >= expensive (%d)", weights["cheap"], weights["expensive"])
	}
	// 应尊重 MinWeight
	for _, w := range weights {
		if w < 1 {
			t.Errorf("weight %d below MinWeight=1", w)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ab.json")
	c1 := NewConverger(&Config{Epsilon: 0.2, MinSamples: 3, MinWeight: 2, StatePath: path})
	c1.RecordOutcome("foo", 200, 0.05, true)
	c1.RecordOutcome("bar", 300, 0.10, false)
	if err := c1.Save(); err != nil {
		t.Fatal(err)
	}
	// 文件存在？
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// 重新加载
	c2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c2.config.Epsilon != 0.2 {
		t.Errorf("epsilon = %v, want 0.2", c2.config.Epsilon)
	}
	if c2.state.Variants["foo"].Samples != 1 {
		t.Errorf("foo samples = %d, want 1", c2.state.Variants["foo"].Samples)
	}
	if c2.state.Variants["bar"].Successes != 0 {
		t.Errorf("bar success should be 0, got %d", c2.state.Variants["bar"].Successes)
	}
}

func TestLoadMissingFileOK(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Errorf("missing file should be OK, got: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil converger")
	}
}

func TestReportFormat(t *testing.T) {
	c := NewConverger(&Config{Epsilon: 0.15, MinSamples: 2})
	available := []string{"alpha", "beta"}
	c.RecordOutcome("alpha", 100, 0.01, true)
	c.RecordOutcome("alpha", 100, 0.01, true)
	c.RecordOutcome("beta", 200, 0.05, true)
	report := c.Report(available)
	if !strings.Contains(report, "A/B 收敛状态") {
		t.Error("report missing header")
	}
	if !strings.Contains(report, "alpha") {
		t.Error("report missing alpha")
	}
	if !strings.Contains(report, "推荐下一个") {
		t.Error("report missing next-variant recommendation")
	}
}

func TestReset(t *testing.T) {
	c := NewConverger(&Config{Epsilon: 0.1, MinSamples: 5})
	c.RecordOutcome("x", 100, 0.01, true)
	if len(c.state.Variants) == 0 {
		t.Fatal("setup: should have variants")
	}
	c.Reset()
	if len(c.state.Variants) != 0 {
		t.Errorf("after reset, variants = %d, want 0", len(c.state.Variants))
	}
	if c.state.TotalDecisions != 0 {
		t.Errorf("after reset, decisions = %d, want 0", c.state.TotalDecisions)
	}
}
