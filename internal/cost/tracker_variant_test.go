package cost

import (
	"path/filepath"
	"testing"

	ap "agentprimordia/pkg"
)

// TestRecordWithVariant 验证 v0.3.0 新增的 RecordWithVariant 能正确写库。
func TestRecordWithVariant(t *testing.T) {
	dir := t.TempDir()
	tracker := newTestTracker(t, filepath.Join(dir, "test.db"))
	tracker.Close()
	// 重新打开验证迁移是幂等的
	tracker2 := newTestTracker(t, filepath.Join(dir, "test.db"))
	defer tracker2.Close()

	usage := ap.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	if err := tracker2.RecordWithVariant("gpt-4o", "openai", "sess-1", "stream", usage, "concise"); err != nil {
		t.Fatalf("RecordWithVariant: %v", err)
	}

	// 校验 RecentRecords 包含 prompt_variant
	recs, err := tracker2.RecentRecords(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].PromptVariant != "concise" {
		t.Errorf("PromptVariant = %q, want concise", recs[0].PromptVariant)
	}
}

// TestRecordBackwardCompatible 验证老式 Record() 仍能工作（variant 为空）。
func TestRecordBackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	tracker := newTestTracker(t, filepath.Join(dir, "test.db"))
	defer tracker.Close()

	usage := ap.Usage{PromptTokens: 50, CompletionTokens: 25, TotalTokens: 75}
	if err := tracker.Record("gpt-4o", "openai", "sess-1", "stream", usage); err != nil {
		t.Fatal(err)
	}
	recs, _ := tracker.RecentRecords(10)
	if len(recs) != 1 {
		t.Fatalf("expected 1, got %d", len(recs))
	}
	if recs[0].PromptVariant != "" {
		t.Errorf("legacy Record should leave PromptVariant empty, got %q", recs[0].PromptVariant)
	}
}

// TestSummaryByVariant 验证 A/B 聚合查询的正确性。
func TestSummaryByVariant(t *testing.T) {
	dir := t.TempDir()
	tracker := newTestTracker(t, filepath.Join(dir, "test.db"))
	defer tracker.Close()

	// 写入混合 variant 的多条记录
	usage100 := ap.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	usage200 := ap.Usage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300}

	tracker.RecordWithVariant("gpt-4o", "openai", "s1", "stream", usage100, "default")
	tracker.RecordWithVariant("gpt-4o", "openai", "s1", "stream", usage100, "default")
	tracker.RecordWithVariant("gpt-4o", "openai", "s1", "stream", usage200, "concise")
	tracker.Record("gpt-4o", "openai", "s1", "stream", usage100) // legacy, variant=""

	stats, err := tracker.SummaryByVariant()
	if err != nil {
		t.Fatal(err)
	}

	// 应有 3 个 variant: default (2 calls), concise (1), (unset) (1)
	if len(stats) != 3 {
		t.Fatalf("expected 3 variant groups, got %d: %+v", len(stats), stats)
	}

	byVariant := map[string]VariantStat{}
	for _, s := range stats {
		byVariant[s.Variant] = s
	}

	if s, ok := byVariant["default"]; !ok || s.Calls != 2 {
		t.Errorf("default: %+v", s)
	} else if s.AvgCostUSD <= 0 {
		t.Errorf("default avg cost should be > 0, got %f", s.AvgCostUSD)
	}

	if s, ok := byVariant["concise"]; !ok || s.Calls != 1 {
		t.Errorf("concise: %+v", s)
	} else if s.TotalTokens != 300 {
		t.Errorf("concise total tokens = %d, want 300", s.TotalTokens)
	}

	if s, ok := byVariant["(unset)"]; !ok || s.Calls != 1 {
		t.Errorf("(unset): %+v", s)
	}
}

// TestSummaryByVariantEmpty 验证空数据库的聚合返回空切片。
func TestSummaryByVariantEmpty(t *testing.T) {
	dir := t.TempDir()
	tracker := newTestTracker(t, filepath.Join(dir, "test.db"))
	defer tracker.Close()

	stats, err := tracker.SummaryByVariant()
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Errorf("empty db should return 0 stats, got %d", len(stats))
	}
}

// TestMigrationIdempotent 验证 initSchema 重复调用是幂等的（升级老库不报错）。
func TestMigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	t1 := newTestTracker(t, dbPath)
	t1.Close()
	// 重新打开 → 再调一次 initSchema
	t2 := newTestTracker(t, dbPath)
	t2.Close()
	// 第三次打开（确保 ALTER 失败被正确吞掉）
	t3 := newTestTracker(t, dbPath)
	defer t3.Close()
}
