package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Model != "gpt-4o" {
		t.Errorf("默认模型应为 gpt-4o, 得到 %s", cfg.Model)
	}
	if cfg.Provider != "openai" {
		t.Errorf("默认 Provider 应为 openai, 得到 %s", cfg.Provider)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "完整配置",
			cfg:     &Config{APIKey: "test-key", Model: "gpt-4o", Provider: "openai"},
			wantErr: false,
		},
		{
			name:    "缺少 APIKey",
			cfg:     &Config{Model: "gpt-4o", Provider: "openai"},
			wantErr: true,
		},
		{
			name:    "缺少 Model",
			cfg:     &Config{APIKey: "test-key", Provider: "openai"},
			wantErr: true,
		},
		{
			name:    "缺少 Provider",
			cfg:     &Config{APIKey: "test-key", Model: "gpt-4o"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	}()

	cfg := &Config{
		APIKey:   "test-api-key",
		Model:    "claude-3-5-sonnet",
		Provider: "anthropic",
		BaseURL:  "https://api.anthropic.com",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded := Load()
	if loaded.APIKey != cfg.APIKey {
		t.Errorf("APIKey mismatch: got %s, want %s", loaded.APIKey, cfg.APIKey)
	}
	if loaded.Model != cfg.Model {
		t.Errorf("Model mismatch: got %s, want %s", loaded.Model, cfg.Model)
	}
	if loaded.Provider != cfg.Provider {
		t.Errorf("Provider mismatch: got %s, want %s", loaded.Provider, cfg.Provider)
	}
}

func TestGetConfigDir(t *testing.T) {
	dir := GetConfigDir()
	if dir == "" {
		t.Error("GetConfigDir() 不应返回空字符串")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("GetConfigDir() 应返回绝对路径, 得到 %s", dir)
	}
}
