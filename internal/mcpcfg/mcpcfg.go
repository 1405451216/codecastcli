package mcpcfg

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"codecast/cli/internal/config"

	"gopkg.in/yaml.v3"
)

// ServerConfig 描述一个 MCP 服务器的配置
type ServerConfig struct {
	Command   string   `json:"command" yaml:"command"`
	Args      []string `json:"args,omitempty" yaml:"args,omitempty"`
	BaseURL   string   `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	AutoStart bool     `json:"auto_start,omitempty" yaml:"auto_start,omitempty"`
}

// Config 是 MCP 服务器配置集合
type Config struct {
	Servers map[string]ServerConfig `json:"servers" yaml:"servers"`
}

// Load 从 ~/.codecast/mcp_servers.yaml 加载配置（F-06 修复：返回 error，
// YAML 解析错误不再静默）。文件不存在属于正常情况（首次启动），返回空配置。
func Load() (*Config, error) {
	cfg := &Config{Servers: make(map[string]ServerConfig)}

	configPath := filepath.Join(config.GetConfigDir(), "mcp_servers.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		// 其他读错误（权限、磁盘）需要暴露
		return cfg, fmt.Errorf("读取 %s 失败: %w", configPath, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		// F-06：YAML 解析错误曾被静默吞掉，导致用户配置错误时
		// 看不到任何反馈。改为 slog.Warn + 返回 error，让调用方展示。
		slog.Warn("mcp_servers.yaml 解析失败", "path", configPath, "error", err)
		return cfg, fmt.Errorf("解析 %s 失败: %w", configPath, err)
	}
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]ServerConfig)
	}
	return cfg, nil
}

// Save 将配置保存到 ~/.codecast/mcp_servers.yaml
func Save(cfg *Config) error {
	configDir := config.GetConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	configPath := filepath.Join(configDir, "mcp_servers.yaml")
	return os.WriteFile(configPath, data, 0600)
}
