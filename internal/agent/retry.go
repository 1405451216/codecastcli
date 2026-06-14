package agent

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries     int           // 最大重试次数
	BaseDelay      time.Duration // 基础延迟
	MaxDelay       time.Duration // 最大延迟
	RetryableCodes []string      // 可重试的错误码（子串匹配，不区分大小写）
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		RetryableCodes: []string{
			"429", "rate_limit", "overloaded",
			"500", "502", "503", "504",
			"timeout",
			// Go 标准库错误信息用空格分隔 (e.g. "connection reset by peer",
			// "connection refused")，因此同时匹配下划线和空格两种写法
			"connection_reset", "connection reset",
			"connection_refused", "connection refused",
		},
	}
}

// WithRetry 带重试的执行包装器。
// 循环 MaxRetries+1 次：fn 成功立即返回；非可重试错误立即返回；可重试错误
// 在指数退避（带抖动）后重试；context 取消立即返回 ctx.Err()。
// 警告信息输出到 stderr，避免污染 stdout 流。
func WithRetry(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// 支持 context 取消：循环开始就检查
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !isRetryable(lastErr, cfg.RetryableCodes) {
			return lastErr // 不可重试错误直接返回
		}
		if attempt < cfg.MaxRetries {
			delay := exponentialBackoff(cfg.BaseDelay, cfg.MaxDelay, attempt)
			// 把"还要重试"的提示提前到等待前打印，让用户在 spinner 阶段就看到
			fmt.Fprintf(os.Stderr, "⚠ 请求失败 (%v)，%v 后重试 (%d/%d)...\n",
				lastErr, delay, attempt+1, cfg.MaxRetries)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return fmt.Errorf("重试 %d 次后仍然失败: %w", cfg.MaxRetries, lastErr)
}

// exponentialBackoff 指数退避 + 抖动（抖动为 delay 的 [0, 1/4) 区间）。
// attempt 从 0 开始：base, 2*base, 4*base, 8*base, ...，上限 MaxDelay。
func exponentialBackoff(base, max time.Duration, attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	// base * 2^attempt；math.Pow 返回 float64，转换为 Duration
	mult := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(base) * mult)
	if delay <= 0 || delay > max {
		delay = max
	}
	// 抖动：[0, delay/4)。delay 极小时抖动为 0
	if delay > 0 {
		jitter := time.Duration(rand.Int63n(int64(delay) / 4))
		delay += jitter
	}
	return delay
}

// isRetryable 错误是否可重试：对 err.Error() 做小写转换并对 RetryableCodes 做子串匹配。
// 子串匹配能同时覆盖 "HTTP 429"、"rate_limit_exceeded"、"overloaded (529)" 等常见表述。
func isRetryable(err error, codes []string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, code := range codes {
		c := strings.ToLower(code)
		if c == "" {
			continue
		}
		if strings.Contains(msg, c) {
			return true
		}
	}
	return false
}
