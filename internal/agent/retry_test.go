package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestWithRetry_Success_NoRetry 第一次 fn 成功就立即返回，且只调用一次
func TestWithRetry_Success_NoRetry(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		return nil
	}
	cfg := DefaultRetryConfig()
	// BaseDelay 设小一点以让测试快
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 10 * time.Millisecond

	err := WithRetry(context.Background(), cfg, fn)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
}

// TestWithRetry_RetryableError_RetriesUpToMax 429 错误重试 3 次后第 3 次成功
func TestWithRetry_RetryableError_RetriesUpToMax(t *testing.T) {
	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return fmt.Errorf("HTTP 429 Too Many Requests")
		}
		return nil
	}
	cfg := DefaultRetryConfig()
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 5 * time.Millisecond

	err := WithRetry(context.Background(), cfg, fn)
	if err != nil {
		t.Fatalf("expected nil after retries, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

// TestWithRetry_NonRetryableError_ReturnsImmediately 401 错误立即返回，不重试
func TestWithRetry_NonRetryableError_ReturnsImmediately(t *testing.T) {
	attempts := 0
	authErr := errors.New("HTTP 401 Unauthorized: invalid api key")
	fn := func() error {
		attempts++
		return authErr
	}
	cfg := DefaultRetryConfig()
	cfg.BaseDelay = 1 * time.Millisecond

	err := WithRetry(context.Background(), cfg, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, authErr) {
		t.Fatalf("expected wrapped auth error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt (no retry on 401), got %d", attempts)
	}
}

// TestWithRetry_ContextCancelDuringBackoff 在退避等待过程中 ctx 取消 → 立即返回 ctx.Err
func TestWithRetry_ContextCancelDuringBackoff(t *testing.T) {
	cfg := DefaultRetryConfig()
	// BaseDelay 设大一些，确保在等待期间 context 被取消
	cfg.BaseDelay = 200 * time.Millisecond
	cfg.MaxDelay = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	fn := func() error {
		attempts++
		return fmt.Errorf("rate_limit_exceeded")
	}

	// 50ms 后取消
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := WithRetry(ctx, cfg, fn)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected ctx error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// 取消应当让循环几乎立刻退出（远小于一个完整的 backoff 周期）
	if elapsed > 300*time.Millisecond {
		t.Fatalf("retry loop didn't honor cancel quickly: elapsed=%v", elapsed)
	}
	// 至少重试了 1 次（首次失败进入 backoff）
	if attempts < 1 {
		t.Fatalf("expected at least 1 attempt, got %d", attempts)
	}
}

// TestWithRetry_AllRetriesFail_ReturnsWrappedError 重试 N 次后仍失败，错误包含原始 err
func TestWithRetry_AllRetriesFail_ReturnsWrappedError(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxRetries = 2
	cfg.BaseDelay = 1 * time.Millisecond
	cfg.MaxDelay = 5 * time.Millisecond

	attempts := 0
	original := errors.New("503 service unavailable")
	fn := func() error {
		attempts++
		return original
	}

	err := WithRetry(context.Background(), cfg, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// MaxRetries=2 意味着总调用 = MaxRetries+1 = 3 次
	if attempts != 3 {
		t.Fatalf("expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
	// 错误应包含原始 err 和"重试 N 次"文案
	if !errors.Is(err, original) {
		t.Fatalf("expected wrapped original error, got %v", err)
	}
	if !strings.Contains(err.Error(), "重试 2 次后仍然失败") {
		t.Fatalf("expected retry-count message in err, got %v", err)
	}
}

// TestExponentialBackoff 验证退避时间在 [base*2^attempt, base*2^attempt + jitter] 区间
func TestExponentialBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	max := 30 * time.Second

	// attempt=0: 100ms, jitter ∈ [0, 25ms)
	d0 := exponentialBackoff(base, max, 0)
	if d0 < base {
		t.Fatalf("attempt 0 delay %v < base %v", d0, base)
	}
	if d0 > base+base/4 {
		t.Fatalf("attempt 0 delay %v > expected upper bound", d0)
	}

	// attempt=1: 200ms, jitter ∈ [0, 50ms)
	d1 := exponentialBackoff(base, max, 1)
	if d1 < 2*base {
		t.Fatalf("attempt 1 delay %v < 2*base", d1)
	}
	if d1 > 2*base+2*base/4 {
		t.Fatalf("attempt 1 delay %v > expected upper bound", d1)
	}

	// attempt=2: 400ms
	d2 := exponentialBackoff(base, max, 2)
	if d2 < 4*base {
		t.Fatalf("attempt 2 delay %v < 4*base", d2)
	}
	if d2 > 4*base+4*base/4 {
		t.Fatalf("attempt 2 delay %v > expected upper bound", d2)
	}

	// 极大 attempt 应当被 MaxDelay 截断
	dHuge := exponentialBackoff(base, max, 20)
	if dHuge > max+max/4 {
		t.Fatalf("attempt 20 delay %v should be capped near max %v", dHuge, max)
	}
	if dHuge < max {
		t.Fatalf("attempt 20 delay %v should be >= max %v (capped at max before jitter)", dHuge, max)
	}
}

// TestIsRetryable 各种 error 字符串的匹配
func TestIsRetryable(t *testing.T) {
	codes := DefaultRetryConfig().RetryableCodes

	cases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"429 status code", errors.New("HTTP 429 Too Many Requests"), true},
		{"500 status code", errors.New("upstream returned 500"), true},
		{"502 status code", errors.New("bad gateway 502"), true},
		{"503 status code", errors.New("service unavailable (503)"), true},
		{"504 status code", errors.New("gateway timeout 504"), true},
		{"rate_limit token", errors.New("rate_limit_exceeded"), true},
		{"overloaded token", errors.New("server is overloaded, retry later"), true},
		{"timeout token", errors.New("i/o timeout"), true},
		{"connection_reset", errors.New("read: connection reset by peer"), true},
		{"connection_refused", errors.New("dial tcp: connection refused"), true},
		{"case insensitive", errors.New("HTTP 429 TOO MANY REQUESTS"), true},
		// 非可重试
		{"401 unauthorized", errors.New("HTTP 401 Unauthorized"), false},
		{"403 forbidden", errors.New("HTTP 403 Forbidden"), false},
		{"400 bad request", errors.New("HTTP 400 Bad Request: invalid param"), false},
		{"404 not found", errors.New("HTTP 404 Not Found"), false},
		{"unrelated error", errors.New("context deadline exceeded"), false},
		{"empty message", errors.New(""), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isRetryable(c.err, codes)
			if got != c.expected {
				t.Fatalf("isRetryable(%q) = %v, want %v", c.err, got, c.expected)
			}
		})
	}
}

// TestIsRetryable_NilSafe nil error 应当不重试（不 panic）
func TestIsRetryable_NilSafe(t *testing.T) {
	if isRetryable(nil, []string{"429"}) {
		t.Fatal("nil error should not be retryable")
	}
}

// TestDefaultRetryConfig 验证默认值符合规范
func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 1*time.Second {
		t.Errorf("BaseDelay = %v, want 1s", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
	want := []string{
		"429", "rate_limit", "overloaded",
		"500", "502", "503", "504",
		"timeout",
		"connection_reset", "connection reset",
		"connection_refused", "connection refused",
	}
	if len(cfg.RetryableCodes) != len(want) {
		t.Fatalf("RetryableCodes len = %d, want %d (got %v)", len(cfg.RetryableCodes), len(want), cfg.RetryableCodes)
	}
	for i, c := range want {
		if cfg.RetryableCodes[i] != c {
			t.Errorf("RetryableCodes[%d] = %q, want %q", i, cfg.RetryableCodes[i], c)
		}
	}
}

// TestWithRetry_ContextAlreadyCanceled 在调用前 ctx 就已取消时，应当立即返回 ctx.Err
func TestWithRetry_ContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	attempts := 0
	fn := func() error {
		attempts++
		return errors.New("429")
	}
	err := WithRetry(ctx, DefaultRetryConfig(), fn)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if attempts != 0 {
		t.Fatalf("expected 0 attempts (ctx canceled before start), got %d", attempts)
	}
}
