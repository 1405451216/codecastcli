package lazy

import (
	"sync"
	"testing"
)

// BenchmarkLazyValueGet benchmarks lazy value Get after initialization.
func BenchmarkLazyValueGet(b *testing.B) {
	v := NewValue(func() (string, error) {
		return "bench-value", nil
	})

	// Pre-load so we benchmark the cached path.
	v.Get()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Get()
	}
}

// BenchmarkLazyValueGetConcurrent benchmarks concurrent Get calls.
func BenchmarkLazyValueGetConcurrent(b *testing.B) {
	v := NewValue(func() (int, error) {
		return 42, nil
	})

	// Pre-load so we benchmark the cached path under contention.
	v.Get()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		const goroutines = 8
		wg.Add(goroutines)
		for j := 0; j < goroutines; j++ {
			go func() {
				defer wg.Done()
				v.Get()
			}()
		}
		wg.Wait()
	}
}
