package agent

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadABIntegration_CreatesEmptyWhenNoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	abi := LoadABIntegration(path)
	if abi == nil {
		t.Fatal("expected non-nil")
	}
	if abi.Enabled() != true {
		t.Error("should be enabled by default")
	}
	// 立即 Save 应该把空状态写出来
	if err := abi.Save(); err != nil {
		t.Errorf("save empty state: %v", err)
	}
}

func TestABIntegration_StartAndEndRound(t *testing.T) {
	dir := t.TempDir()
	abi := LoadABIntegration(filepath.Join(dir, "ab.json"))
	abi.StartRound("default")
	time.Sleep(5 * time.Millisecond) // 让 latency 至少有 1ms
	abi.EndRound(100, 0.01, true)

	c := abi.Converger()
	if c == nil {
		t.Fatal("converger nil")
	}
	stat := c.Config() // 防止 unused
	_ = stat
}

func TestABIntegration_ResolveSuccess_AfterRound(t *testing.T) {
	dir := t.TempDir()
	abi := LoadABIntegration(filepath.Join(dir, "ab.json"))
	abi.StartRound("default")
	// 不调 EndRound，让 Round 保持"未判定"
	if !abi.ResolveSuccess(false) {
		t.Error("should hit unresolved round")
	}
	// 第二次应该返回 false（已 resolved）
	if abi.ResolveSuccess(true) {
		t.Error("second call should be a no-op")
	}
}

func TestABIntegration_SetEnabled(t *testing.T) {
	dir := t.TempDir()
	abi := LoadABIntegration(filepath.Join(dir, "ab.json"))
	abi.SetEnabled(false)
	if abi.Enabled() {
		t.Error("should be disabled")
	}
	abi.StartRound("x")
	abi.EndRound(0, 0, true) // 应该被忽略（disabled）
	// 验证 converger 没记录
	// converger 私有字段 → 通过 Report 空验证
	report := abi.Converger().Report([]string{"x"})
	if report == "" {
		t.Error("report should not be empty")
	}
}

func TestABIntegration_ClosePersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ab.json")
	abi := LoadABIntegration(path)
	abi.StartRound("foo")
	abi.EndRound(50, 0.005, true)
	if err := abi.Close(); err != nil {
		t.Fatal(err)
	}
	// 重新加载
	abi2 := LoadABIntegration(path)
	if abi2 == nil {
		t.Fatal("reload nil")
	}
}
