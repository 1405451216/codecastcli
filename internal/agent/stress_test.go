package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/errors"
	"codecast/cli/internal/testutil"
)

// TestStressConcurrentComplete 并发调用 Complete，验证无 race/panic/死锁
func TestStressConcurrentComplete(t *testing.T) {
	provider := testutil.NewMockProvider().WithResponse("ok")

	const N = 20
	var wg sync.WaitGroup
	errs := make([]error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, errs[idx] = provider.Complete(ctx, &ap.CompletionRequest{})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("request %d: unexpected error: %v", i, err)
		}
	}

	if got := provider.CallCount(); got != N {
		t.Errorf("CallCount = %d, want %d", got, N)
	}
}

// TestStressConcurrentStream 并发流式请求，验证 channel 正确关闭
func TestStressConcurrentStream(t *testing.T) {
	provider := testutil.NewMockProvider().
		WithStreamChunks("a", "b", "c", "d", "e")

	const N = 10
	var wg sync.WaitGroup
	errs := make([]error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			ch, err := provider.Stream(ctx, &ap.CompletionRequest{})
			if err != nil {
				errs[idx] = err
				return
			}
			// 消费所有 chunk
			for range ch {
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("stream %d: unexpected error: %v", i, err)
		}
	}
}

// TestStressContextCancelDuringStream 在流式输出中途取消 context
func TestStressContextCancelDuringStream(t *testing.T) {
	provider := testutil.NewMockProvider().
		WithStreamChunks("token1", "token2", "token3", "token4", "token5")

	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithCancel(context.Background())

		ch, err := provider.Stream(ctx, &ap.CompletionRequest{})
		if err != nil {
			cancel()
			t.Fatalf("iteration %d: Stream failed: %v", i, err)
		}

		// 读取 1-2 个 chunk 后取消
		count := 0
		for range ch {
			count++
			if count >= 2 {
				cancel()
				break
			}
		}
		// 确保 channel 最终关闭
		for range ch {
		}
		cancel()
	}
	// 无 panic = pass
}

// TestStressConcurrentAgentSession 并发使用多个 Agent Session
func TestStressConcurrentAgentSession(t *testing.T) {
	provider := testutil.NewMockProvider().WithResponse("hello world")

	const N = 5
	var wg sync.WaitGroup
	results := make([]string, N)
	errs := make([]error, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess, cleanup := testutil.NewTestSession(t, provider)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := sess.Ask(ctx, fmt.Sprintf("request %d", idx))
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = resp.Content
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("session %d: error: %v", i, err)
		}
	}
	for i, r := range results {
		if r == "" {
			t.Errorf("session %d: empty response", i)
		}
	}
}

// TestStressDegradationMatrixConcurrent 并发读写降级矩阵
func TestStressDegradationMatrixConcurrent(t *testing.T) {
	matrix := errors.NewDegradationMatrix()

	const N = 50
	var wg sync.WaitGroup
	wg.Add(3)

	// Writer 1: 设置各模块降级状态
	go func() {
		defer wg.Done()
		modules := []string{"lsp", "treesitter", "indexer", "mcp", "provider", "budget"}
		for i := 0; i < N; i++ {
			mod := modules[i%len(modules)]
			matrix.ReportDegradation(mod, &errors.DegradationStatus{
				Level:  errors.DegradationMinor,
				Reason: fmt.Sprintf("failure-%d", i),
			})
		}
	}()

	// Writer 2: 恢复模块
	go func() {
		defer wg.Done()
		modules := []string{"lsp", "treesitter", "indexer", "mcp", "provider", "budget"}
		for i := 0; i < N; i++ {
			mod := modules[i%len(modules)]
			matrix.ReportDegradation(mod, &errors.DegradationStatus{
				Level: errors.DegradationNone,
			})
		}
	}()

	// Reader: 持续查询状态
	go func() {
		defer wg.Done()
		for i := 0; i < N*2; i++ {
			_ = matrix.IsDegraded()
			_ = matrix.GetOverallLevel()
			_ = matrix.Summary()
		}
	}()

	wg.Wait()
	// 无 race / panic = pass
}

// TestStressUserFacingErrorConcurrent 并发创建和格式化 UserFacingError
func TestStressUserFacingErrorConcurrent(t *testing.T) {
	codes := []errors.ErrorCode{
		errors.ErrProviderNotFound,
		errors.ErrProviderAuth,
		errors.ErrBudgetExceeded,
		errors.ErrProviderRateLimit,
		errors.ErrToolExecFailed,
		errors.ErrPermissionDenied,
	}

	const N = 30
	var wg sync.WaitGroup

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code := codes[idx%len(codes)]
			ufe := &errors.UserFacingError{
				Code:       code,
				Message:    fmt.Sprintf("test error %d", idx),
				Hint:       fmt.Sprintf("hint %d", idx),
				WrappedErr: fmt.Errorf("wrapped %d", idx),
			}
			// 并发调用 Error() 和 Unwrap()
			s := ufe.Error()
			if s == "" {
				t.Errorf("empty error string for code %s", code)
			}
			inner := ufe.Unwrap()
			if inner == nil {
				t.Errorf("nil unwrap for code %s", code)
			}
		}(i)
	}
	wg.Wait()
}
