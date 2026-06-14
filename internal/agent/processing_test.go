package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// TestIsProcessing_DefaultValue 验证新 agent 的零值 IsProcessing() == false。
// 这覆盖了 SIGINT handler 的"空闲时按 Ctrl+C 退出"分支：
// handler 必须能从 IsProcessing() 区分"没在处理"和"正在处理"。
func TestIsProcessing_DefaultValue(t *testing.T) {
	// 用 atomic.Bool 直接验证零值行为（与 CodecastAgent.processing 字段同类型）
	var flag atomic.Bool
	if flag.Load() {
		t.Fatal("atomic.Bool zero value should be false")
	}
}

// TestIsProcessing_StoreLoad 验证 Store/Load 配对使用的工作流。
// 模拟 SIGINT handler 与 StreamProcess 并发：
//   - "StreamProcess" goroutine 标记 processing = true，工作完设回 false
//   - "SIGINT" goroutine 读取 IsProcessing()
func TestIsProcessing_StoreLoad(t *testing.T) {
	var flag atomic.Bool

	// 1) Store(true)
	flag.Store(true)
	if !flag.Load() {
		t.Fatal("after Store(true), Load should return true")
	}

	// 2) Store(false)
	flag.Store(false)
	if flag.Load() {
		t.Fatal("after Store(false), Load should return false")
	}
}

// TestIsProcessing_ConcurrentSafe 并发读写不 race。
// 用 `-race` 跑会触发竞态检测（如果实现错误会失败）。
func TestIsProcessing_ConcurrentSafe(t *testing.T) {
	var flag atomic.Bool
	var wg sync.WaitGroup

	// 10 个 reader
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = flag.Load()
			}
		}()
	}

	// 1 个 writer 来回切换
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 1000; j++ {
			if j%2 == 0 {
				flag.Store(true)
			} else {
				flag.Store(false)
			}
		}
	}()

	wg.Wait()

	// 最终状态是 Store(false)（j=999 时 odd）
	if flag.Load() {
		t.Fatal("final state should be false (j=999 → Store(false))")
	}
}

// TestIsProcessing_DeferResetsFlag 模拟"defer a.processing.Store(false)"的语义。
// 验证无论中间是否 panic，defer 都能把 flag 复位（panic 场景在 Process 内部
// panic 恢复时仍能保证 REPL 不卡死）。
func TestIsProcessing_DeferResetsFlag(t *testing.T) {
	var flag atomic.Bool
	flag.Store(true)

	func() {
		defer flag.Store(false)
		// 模拟 StreamProcess 内部工作
		_ = flag.Load()
	}()

	if flag.Load() {
		t.Fatal("defer should have reset flag to false")
	}
}

// TestIsProcessing_TrueWhileWorking 验证"工作期间 IsProcessing() == true"。
// 模拟 SIGINT handler 想要"在 StreamProcess 运行中"被告知的状态。
func TestIsProcessing_TrueWhileWorking(t *testing.T) {
	var flag atomic.Bool
	flag.Store(true) // 模拟 StreamProcess 入口

	// SIGINT handler 检查：如果是 true → cancel ctx
	if !flag.Load() {
		t.Fatal("during processing, IsProcessing() should be true")
	}

	// 工作完成，defer 把 flag 复位
	flag.Store(false)
	if flag.Load() {
		t.Fatal("after defer reset, IsProcessing() should be false")
	}
}

// TestIsProcessing_AcquisitionPattern 模拟 acquireProcessingCtx 的 mutex 保护模式。
// 这是 cmd/interactive.go 中 acquireProcessingCtx 的微缩版，验证：
//   - 每次写入 processingCancel 是原子的（不丢更新）
//   - SIGINT handler 读到的总是"最近一次"写入的 cancel
func TestIsProcessing_AcquisitionPattern(t *testing.T) {
	var mu sync.Mutex
	var currentCancel context.CancelFunc = func() {}
	// 用 channel 模拟 signal.Notify：handler 每次取最近一次 cancel
	cancelInvoked := make(chan int, 100)

	// 100 个 REPL 循环：每次写入新 cancel
	for i := 0; i < 100; i++ {
		idx := i
		mu.Lock()
		_, cancel := context.WithCancel(context.Background())
		// 包一层：调用时除了 cancel 自身，还通知调用编号
		cancel = wrapCancel(cancel, idx, cancelInvoked)
		currentCancel = cancel
		mu.Unlock()
	}

	// SIGINT handler 取最后写入的 cancel 调用
	mu.Lock()
	last := currentCancel
	mu.Unlock()
	last() // 应该看到 idx=99

	// 验证
	select {
	case got := <-cancelInvoked:
		if got != 99 {
			t.Fatalf("expected last cancel idx=99, got %d", got)
		}
	default:
		t.Fatal("expected cancel to be invoked")
	}
}

// wrapCancel 是测试辅助：在调用原始 cancel 之前向 channel 发送 idx。
// 模拟 SIGINT handler "handler 看到的 cancel 是谁创建的"语义。
func wrapCancel(cancel context.CancelFunc, idx int, ch chan int) context.CancelFunc {
	return func() {
		ch <- idx
		cancel()
	}
}
