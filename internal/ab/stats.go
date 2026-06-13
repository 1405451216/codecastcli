// Package ab: 统计函数。
//
// 包含：
//   - Wilson 95% 置信区间：用于胜率估计，避免 n 小时 ±50% 的离谱区间
//   - 显著性检验：两变体胜率差是否显著（区间重叠检验）
//   - 百分位时延：p50 / p95
//
// 这些函数都不依赖 State，纯数学，便于单测。
package ab

import (
	"math"
	"sort"
	"sync"
)

// 95% 置信水平的 z 值（双侧）。Welford 近似。
const z95 = 1.959963984540054

// WilsonInterval 返回 (lower, upper) 95% 置信区间。
// 公式见 https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval#Wilson_score_interval
// 行为：
//   - successes == 0           → (0, upper)
//   - successes == trials      → (lower, 1)
//   - trials == 0              → (0, 0)
func WilsonInterval(successes, trials int) (float64, float64) {
	if trials == 0 {
		return 0, 0
	}
	n := float64(trials)
	p := float64(successes) / n
	denom := 1 + z95*z95/n
	center := (p + z95*z95/(2*n)) / denom
	margin := (z95 / denom) * math.Sqrt(p*(1-p)/n+z95*z95/(4*n*n))
	lo := center - margin
	hi := center + margin
	if lo < 0 {
		lo = 0
	}
	if hi > 1 {
		hi = 1
	}
	return lo, hi
}

// IsSignificantlyBetter 判断 A 的胜率是否显著高于 B。
// 判定：lower(A) > upper(B) 才算"显著更好"（更严格的单侧检验）。
// 当 n 很小（< MinSamples）时一律返回 false，避免冷启动期误判。
func IsSignificantlyBetter(aSuccess, aTrials, bSuccess, bTrials int, minSamples int) bool {
	if aTrials < minSamples || bTrials < minSamples {
		return false
	}
	aLo, _ := WilsonInterval(aSuccess, aTrials)
	_, bHi := WilsonInterval(bSuccess, bTrials)
	return aLo > bHi
}

// Percentile 返回第 p（0-100）百分位数。
// 输入切片会被拷贝排序，不修改原切片。
// 行为：
//   - 长度 0 → 0
//   - 长度 1 → 唯一值
//   - p <= 0 → 最小值
//   - p >= 100 → 最大值
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	// 线性插值（与 numpy.percentile linear 一致）
	rank := (p / 100) * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// VariantLatencyStats 一个变体的时延统计。
type VariantLatencyStats struct {
	Name    string
	P50     float64
	P95     float64
	Mean    float64
	Count   int
}

// LatencyTracker 记录每个变体的 LLM 调用时延（毫秒）。
// 用环形 buffer 防止长会话 OOM，默认每变体最多保留 200 条。
type LatencyTracker struct {
	maxPerVariant int
	mu            sync.Mutex
	samples       map[string][]float64
}

// NewLatencyTracker 构造追踪器。maxPerVariant=0 → 默认 200。
func NewLatencyTracker(maxPerVariant int) *LatencyTracker {
	if maxPerVariant <= 0 {
		maxPerVariant = 200
	}
	return &LatencyTracker{
		maxPerVariant: maxPerVariant,
		samples:       make(map[string][]float64),
	}
}

// Record 追加一条样本（毫秒）。
func (t *LatencyTracker) Record(variant string, latencyMs float64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	buf := t.samples[variant]
	buf = append(buf, latencyMs)
	if len(buf) > t.maxPerVariant {
		// 简单的 FIFO 截断（保留最近 N 条）
		buf = buf[len(buf)-t.maxPerVariant:]
	}
	t.samples[variant] = buf
}

// Stats 返回某变体的时延统计。
func (t *LatencyTracker) Stats(variant string) VariantLatencyStats {
	if t == nil {
		return VariantLatencyStats{Name: variant}
	}
	t.mu.Lock()
	samples := append([]float64(nil), t.samples[variant]...)
	t.mu.Unlock()
	if len(samples) == 0 {
		return VariantLatencyStats{Name: variant}
	}
	var sum float64
	for _, v := range samples {
		sum += v
	}
	return VariantLatencyStats{
		Name:  variant,
		P50:   Percentile(samples, 50),
		P95:   Percentile(samples, 95),
		Mean:  sum / float64(len(samples)),
		Count: len(samples),
	}
}

// AllStats 返回所有变体的统计。
func (t *LatencyTracker) AllStats(available []string) []VariantLatencyStats {
	out := make([]VariantLatencyStats, 0, len(available))
	for _, name := range available {
		out = append(out, t.Stats(name))
	}
	return out
}

// Reset 清空所有时延样本。
func (t *LatencyTracker) Reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.samples = make(map[string][]float64)
}
