package cost

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	ap "agentprimordia/pkg"
	_ "modernc.org/sqlite"
)

// newTestTracker 直接打开 dbPath 并构造 Tracker，绕开 NewTracker 对
// config.GetConfigDir() 的依赖。调用方负责 Close。
func newTestTracker(t *testing.T, dbPath string) *Tracker {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := initSchema(db); err != nil {
		t.Fatalf("initSchema: %v", err)
	}
	return &Tracker{db: db}
}

func TestNewTracker(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)

	tracker, err := NewTracker()
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	defer tracker.Close()
}

// TestRecordConcurrent 验证 Tracker.Record 在并发场景下不丢数据（F-12）。
// 直接打开一个临时 sqlite 文件并构造 Tracker，绕开 NewTracker 对
// config.GetConfigDir() 的依赖（HOME 在并发测试中不可靠）。
func TestRecordConcurrent(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "cost.db")
	tracker := newTestTracker(t, dbPath)
	defer tracker.Close()

	const goroutines = 10
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				usage := ap.Usage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				}
				if err := tracker.Record("gpt-4o", "openai", "s1", "test", usage); err != nil {
					t.Errorf("Record: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	summary, err := tracker.Summary()
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	want := goroutines * perGoroutine
	if summary.CallCount != want {
		t.Errorf("CallCount = %d, want %d", summary.CallCount, want)
	}
	if summary.TotalTokens != int64(want*15) {
		t.Errorf("TotalTokens = %d, want %d", summary.TotalTokens, want*15)
	}
}

func TestTracker_RecordAndSummary(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)

	tracker, err := NewTracker()
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	defer tracker.Close()

	usage := ap.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	if err := tracker.Record("gpt-4o", "openai", "sess_123", "chat", usage); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	summary, err := tracker.Summary()
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}

	if summary.CallCount != 1 {
		t.Errorf("CallCount = %d, want 1", summary.CallCount)
	}
	if summary.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", summary.TotalTokens)
	}
	if summary.TotalPromptTokens != 100 {
		t.Errorf("TotalPromptTokens = %d, want 100", summary.TotalPromptTokens)
	}
	if summary.TotalCompTokens != 50 {
		t.Errorf("TotalCompTokens = %d, want 50", summary.TotalCompTokens)
	}
	if summary.TotalCostUSD <= 0 {
		t.Errorf("TotalCostUSD 应大于 0, 得到 %f", summary.TotalCostUSD)
	}

	// 检查按模型统计
	if len(summary.ByModel) != 1 {
		t.Errorf("ByModel 长度 = %d, want 1", len(summary.ByModel))
	}
	m, ok := summary.ByModel["gpt-4o"]
	if !ok {
		t.Fatal("ByModel 中应包含 gpt-4o")
	}
	if m.Calls != 1 {
		t.Errorf("模型调用次数 = %d, want 1", m.Calls)
	}
}

func TestTracker_RecentRecords(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)

	tracker, err := NewTracker()
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	defer tracker.Close()

	for i := 0; i < 5; i++ {
		usage := ap.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
		if err := tracker.Record("gpt-4o-mini", "openai", "sess_test", "chat", usage); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	records, err := tracker.RecentRecords(3)
	if err != nil {
		t.Fatalf("RecentRecords() error = %v", err)
	}
	if len(records) != 3 {
		t.Errorf("RecentRecords(3) = %d, want 3", len(records))
	}
}

func TestTracker_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)

	tracker, err := NewTracker()
	if err != nil {
		t.Fatalf("NewTracker() error = %v", err)
	}
	defer tracker.Close()

	usage := ap.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	_ = tracker.Record("gpt-4o", "openai", "sess_1", "chat", usage)

	if err := tracker.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	summary, err := tracker.Summary()
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.CallCount != 0 {
		t.Errorf("Clear 后 CallCount = %d, want 0", summary.CallCount)
	}
}
