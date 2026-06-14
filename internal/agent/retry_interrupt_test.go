package agent

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestRetryInterruptOnCancel: Create a retry scenario, cancel the context,
// verify it stops immediately.
func TestRetryInterruptOnCancel(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     5,
		BaseDelay:      1 * time.Second,
		MaxDelay:       30 * time.Second,
		RetryableCodes: []string{"500"},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	callCount := 0
	err := WithRetry(ctx, cfg, func() error {
		callCount++
		return fmt.Errorf("HTTP 500 error")
	})

	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
	if ctx.Err() != context.Canceled {
		t.Errorf("ctx.Err() = %v, want context.Canceled", ctx.Err())
	}
	// Should have stopped after context cancellation, not exhausted all retries
	if callCount > 3 {
		t.Errorf("callCount = %d, want <= 3 (should stop after cancel)", callCount)
	}
}

// TestRetryNoRetryOn401: Verify 401 errors are not retried.
func TestRetryNoRetryOn401(t *testing.T) {
	cfg := DefaultRetryConfig()

	callCount := 0
	err := WithRetry(context.Background(), cfg, func() error {
		callCount++
		return fmt.Errorf("HTTP 401 unauthorized")
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (401 should not be retried)", callCount)
	}
}

// TestRetryNoRetryOn403: Verify 403 errors are not retried.
func TestRetryNoRetryOn403(t *testing.T) {
	cfg := DefaultRetryConfig()

	callCount := 0
	err := WithRetry(context.Background(), cfg, func() error {
		callCount++
		return fmt.Errorf("HTTP 403 forbidden")
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (403 should not be retried)", callCount)
	}
}

// TestRetryOn429: Verify 429 errors are retried up to 3 times.
func TestRetryOn429(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     3,
		BaseDelay:      1 * time.Millisecond, // fast for tests
		MaxDelay:       5 * time.Millisecond,
		RetryableCodes: []string{"429"},
	}

	callCount := 0
	err := WithRetry(context.Background(), cfg, func() error {
		callCount++
		return fmt.Errorf("HTTP 429 rate_limit exceeded")
	})

	if err == nil {
		t.Error("expected error after exhausting retries, got nil")
	}
	// Should try MaxRetries+1 times (initial + 3 retries = 4)
	if callCount != 4 {
		t.Errorf("callCount = %d, want 4 (1 initial + 3 retries)", callCount)
	}
}

// TestRetryOn500: Verify 500 errors are retried up to 3 times.
func TestRetryOn500(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     3,
		BaseDelay:      1 * time.Millisecond,
		MaxDelay:       5 * time.Millisecond,
		RetryableCodes: []string{"500"},
	}

	callCount := 0
	err := WithRetry(context.Background(), cfg, func() error {
		callCount++
		return fmt.Errorf("HTTP 500 internal server error")
	})

	if err == nil {
		t.Error("expected error after exhausting retries, got nil")
	}
	// Should try MaxRetries+1 times (initial + 3 retries = 4)
	if callCount != 4 {
		t.Errorf("callCount = %d, want 4 (1 initial + 3 retries)", callCount)
	}
}

// TestRetrySuccessOnSecondAttempt: Verify retry succeeds if the second attempt works.
func TestRetrySuccessOnSecondAttempt(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     3,
		BaseDelay:      1 * time.Millisecond,
		MaxDelay:       5 * time.Millisecond,
		RetryableCodes: []string{"500"},
	}

	callCount := 0
	err := WithRetry(context.Background(), cfg, func() error {
		callCount++
		if callCount < 2 {
			return fmt.Errorf("HTTP 500 error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected nil error on retry success, got: %v", err)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2 (1 fail + 1 success)", callCount)
	}
}
