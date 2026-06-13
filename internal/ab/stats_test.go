package ab

import (
	"math"
	"testing"
)

func TestWilsonInterval_BoundaryCases(t *testing.T) {
	// n=0
	if lo, hi := WilsonInterval(0, 0); lo != 0 || hi != 0 {
		t.Errorf("n=0: got (%v,%v) want (0,0)", lo, hi)
	}
	// 100% 成功
	lo, hi := WilsonInterval(10, 10)
	if lo < 0.65 || hi != 1.0 {
		t.Errorf("all success: got (%v,%v) want lo>=0.65, hi=1", lo, hi)
	}
	// 0% 成功
	lo, hi = WilsonInterval(0, 10)
	if lo != 0 || hi > 0.30 {
		t.Errorf("no success: got (%v,%v) want lo=0, hi<0.30", lo, hi)
	}
	// 50% 中庸
	lo, hi = WilsonInterval(50, 100)
	if math.Abs((lo+hi)/2-0.5) > 0.01 {
		t.Errorf("center off: got (%v,%v)", lo, hi)
	}
}

func TestWilsonInterval_ShrinksWithN(t *testing.T) {
	lo1, hi1 := WilsonInterval(5, 10)   // 50% with n=10
	lo2, hi2 := WilsonInterval(50, 100) // 50% with n=100
	width1 := hi1 - lo1
	width2 := hi2 - lo2
	if width2 >= width1 {
		t.Errorf("n=100 width (%v) should be narrower than n=10 (%v)", width2, width1)
	}
}

func TestIsSignificantlyBetter_RealCase(t *testing.T) {
	// A: 90/100 = 90% success, B: 50/100 = 50%
	if !IsSignificantlyBetter(90, 100, 50, 100, 5) {
		t.Error("A should be significantly better than B")
	}
	// A: 55/100, B: 50/100 → 应不显著
	if IsSignificantlyBetter(55, 100, 50, 100, 5) {
		t.Error("A=55% vs B=50% should not be significant")
	}
	// 冷启动：samples 不够
	if IsSignificantlyBetter(9, 9, 5, 100, 10) {
		t.Error("should not be significant when n < minSamples")
	}
}

func TestPercentile_Empty(t *testing.T) {
	if v := Percentile(nil, 50); v != 0 {
		t.Errorf("empty: got %v want 0", v)
	}
}

func TestPercentile_Single(t *testing.T) {
	if v := Percentile([]float64{42}, 50); v != 42 {
		t.Errorf("single p50: got %v want 42", v)
	}
	if v := Percentile([]float64{42}, 99); v != 42 {
		t.Errorf("single p99: got %v want 42", v)
	}
}

func TestPercentile_LinearInterpolation(t *testing.T) {
	// [1,2,3,4,5], p=50 → rank=2 → 3
	if v := Percentile([]float64{1, 2, 3, 4, 5}, 50); v != 3 {
		t.Errorf("p50: got %v want 3", v)
	}
	// p=25 → rank=1 → 2
	if v := Percentile([]float64{1, 2, 3, 4, 5}, 25); v != 2 {
		t.Errorf("p25: got %v want 2", v)
	}
	// p=0 → min
	if v := Percentile([]float64{1, 2, 3, 4, 5}, 0); v != 1 {
		t.Errorf("p0: got %v want 1", v)
	}
	// p=100 → max
	if v := Percentile([]float64{1, 2, 3, 4, 5}, 100); v != 5 {
		t.Errorf("p100: got %v want 5", v)
	}
}

func TestPercentile_DoesNotMutateInput(t *testing.T) {
	in := []float64{3, 1, 4, 1, 5, 9, 2, 6}
	saved := append([]float64(nil), in...)
	_ = Percentile(in, 50)
	for i, v := range in {
		if v != saved[i] {
			t.Errorf("input mutated at %d: %v != %v", i, v, saved[i])
			break
		}
	}
}

func TestLatencyTracker_RecordAndStats(t *testing.T) {
	tr := NewLatencyTracker(0)
	for _, v := range []float64{100, 200, 300, 400, 500} {
		tr.Record("a", v)
	}
	s := tr.Stats("a")
	if s.Count != 5 {
		t.Errorf("count = %d, want 5", s.Count)
	}
	if s.P50 != 300 {
		t.Errorf("p50 = %v, want 300", s.P50)
	}
	if s.P95 < 400 {
		t.Errorf("p95 = %v, want >= 400", s.P95)
	}
}

func TestLatencyTracker_RingBuffer(t *testing.T) {
	tr := NewLatencyTracker(3)
	tr.Record("x", 1)
	tr.Record("x", 2)
	tr.Record("x", 3)
	tr.Record("x", 4) // 1 应被挤出
	s := tr.Stats("x")
	if s.Count != 3 {
		t.Errorf("after ring overflow: count = %d, want 3", s.Count)
	}
	// 残留应是 [2,3,4] → p50=3
	if s.P50 != 3 {
		t.Errorf("after ring: p50 = %v, want 3 (oldest evicted)", s.P50)
	}
}
