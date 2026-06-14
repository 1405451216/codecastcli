package routing

import (
	"strings"
	"testing"
	"time"
)

// BenchmarkRoute benchmarks the Route() method.
// Asserts < 50ms per call (should be microseconds).
func BenchmarkRoute(b *testing.B) {
	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)

	inputs := []struct {
		input     string
		fileCount int
	}{
		{"解释这段代码", 0},
		{"修改这个函数的逻辑", 4},
		{"重构整个认证模块，需要重新设计架构并迁移所有现有逻辑到新的微服务体系结构", 10},
		{strings.Repeat("很长的输入内容", 50), 0},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc := inputs[i%len(inputs)]
		router.Route(tc.input, tc.fileCount)
	}

	// Assert < 50ms per call
	if b.N > 0 {
		avg := b.Elapsed() / time.Duration(b.N)
		if avg > 50*time.Millisecond {
			b.Fatalf("average per-call time %v exceeds 50ms threshold", avg)
		}
	}
}

// TestRouteLatencyUnder50ms times 1000 Route() calls and asserts average < 1ms.
func TestRouteLatencyUnder50ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	cfg := DefaultRoutingConfig()
	router := NewModelRouter(cfg)

	inputs := []struct {
		input     string
		fileCount int
	}{
		{"解释这段代码", 0},
		{"修改这个函数的逻辑", 4},
		{"重构整个认证模块，需要重新设计架构并迁移所有现有逻辑到新的微服务体系结构", 10},
		{strings.Repeat("很长的输入内容", 50), 0},
	}

	const iterations = 1000
	start := time.Now()
	for i := 0; i < iterations; i++ {
		tc := inputs[i%len(inputs)]
		model := router.Route(tc.input, tc.fileCount)
		if model == "" {
			t.Fatal("Route returned empty model")
		}
	}
	elapsed := time.Since(start)
	avg := elapsed / iterations

	t.Logf("1000 Route() calls: total=%v, avg=%v", elapsed, avg)

	if avg >= 1*time.Millisecond {
		t.Errorf("average Route() latency %v >= 1ms, expected < 1ms", avg)
	}
}
