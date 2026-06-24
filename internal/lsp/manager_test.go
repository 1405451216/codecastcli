package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewManager_SetsRootDir 验证构造函数记录工作目录且不抛错。
func TestNewManager_SetsRootDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.rootDir != dir {
		t.Errorf("rootDir = %q, want %q", m.rootDir, dir)
	}
	// NewManager 会自动 detectAvailableServers；
	// 若 gopls 在 PATH 中，AvailableServers 会包含 "go"（这是预期行为）。
	// 此处只验证不会 panic / nil，集合内容依赖宿主环境。
	_ = m.AvailableServers()
}

// TestDetectProjectLanguages_FindsGo 临时目录里放 go.mod，应识别为 go 项目。
func TestDetectProjectLanguages_FindsGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	m := NewManager(dir)
	langs := m.detectProjectLanguages()
	found := false
	for _, l := range langs {
		if l == "go" {
			found = true
		}
	}
	if !found {
		t.Errorf("detectProjectLanguages did not include go; got %v", langs)
	}
}

// TestIsAvailableAndInstalled_KnownUnknown 验证未知语言返回 false。
func TestIsAvailableAndInstalled_KnownUnknown(t *testing.T) {
	m := NewManager(t.TempDir())
	if m.IsAvailable("brainfuck") {
		t.Error("IsAvailable(brainfuck) = true, want false")
	}
	if m.IsInstalled("brainfuck") {
		t.Error("IsInstalled(brainfuck) = true, want false")
	}
}

// TestInstallHint_ReturnsSomething 验证支持的语言有安装提示；
// 不支持的语言允许返回空（fallback）。
func TestInstallHint_ReturnsSomething(t *testing.T) {
	supported := []string{"go", "python", "typescript"}
	for _, lang := range supported {
		h := installHint(lang)
		if h == "" {
			t.Errorf("installHint(%q) = empty, want non-empty hint", lang)
		}
	}
}

// TestStopAll_NeverPanics 验证对未启动管理器调用 StopAll 是安全的。
func TestStopAll_NeverPanics(t *testing.T) {
	m := NewManager(t.TempDir())
	// 二次调用也安全
	m.StopAll()
	m.StopAll()
}

// TestServerInfo_UnknownLanguage 验证未检测过的语言返回 found=false。
func TestServerInfo_UnknownLanguage(t *testing.T) {
	m := NewManager(t.TempDir())
	_, _, found := m.ServerInfo("nonsense-language")
	if found {
		t.Error("ServerInfo for unknown language reported found=true")
	}
}
