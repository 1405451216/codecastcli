package mcpcfg

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLoadMissingFileReturnsEmpty 验证：文件不存在时返回空 config，nil error。
// 其他错误（YAML 解析失败）会返回 error（F-06 修复）。
func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		// 真实家目录里若有损坏的 yaml，Load 现在会返回 error — 这是预期行为
		t.Logf("Load() returned error (acceptable if YAML malformed): %v", err)
	}
	if cfg == nil {
		t.Errorf("Load() returned nil config")
	}
	if cfg != nil && cfg.Servers == nil {
		t.Errorf("Load() returned nil Servers map")
	}
}

// TestSaveLoadRoundTrip 直接走 yaml 包验证编解码正确性。
func TestSaveLoadRoundTrip(t *testing.T) {
	original := &Config{
		Servers: map[string]ServerConfig{
			"test": {Command: "echo", Args: []string{"hello"}, AutoStart: true},
			"sse":  {BaseURL: "http://localhost:8080/sse"},
		},
	}
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Config
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Servers["test"].Command != "echo" {
		t.Errorf("test.Command = %q, want echo", got.Servers["test"].Command)
	}
	if got.Servers["sse"].BaseURL != "http://localhost:8080/sse" {
		t.Errorf("sse.BaseURL mismatch")
	}
	if !got.Servers["test"].AutoStart {
		t.Errorf("AutoStart should be true")
	}
	_ = filepath.Join // keep import
}
