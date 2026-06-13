package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config 是 Codecast CLI 的配置
type Config struct {
	APIKey              string   `yaml:"api_key"`
	Model               string   `yaml:"model"`
	Provider            string   `yaml:"provider"`
	BaseURL             string   `yaml:"base_url,omitempty"`
	SafeMode            bool     `yaml:"safe_mode"`
	PermissionMode      string   `yaml:"permission_mode,omitempty"`
	Scopes              []string `yaml:"scopes,omitempty"`
	SummaryModel        string   `yaml:"summary_model,omitempty"`
	ContextBudget       int      `yaml:"context_budget,omitempty"`
	ContextThreshold    float64  `yaml:"context_threshold,omitempty"`
	ContextCompress     bool     `yaml:"context_compress,omitempty"`
	PreserveRecent      int      `yaml:"preserve_recent,omitempty"`
	ProjectRoot         string   `yaml:"project_root,omitempty"`
	// F1: 智能上下文管理
	AutoCompact      bool    `yaml:"auto_compact,omitempty"`
	AutoCompactRatio float64 `yaml:"auto_compact_ratio,omitempty"` // 触发压缩的上下文使用比例，默认0.8
	// F4: 自动 Git Checkpoint
	AutoCheckpoint bool `yaml:"auto_checkpoint,omitempty"`
	AutoStash      bool `yaml:"auto_stash,omitempty"`
	// F8: 成本预算控制
	DailyBudgetUSD    float64 `yaml:"daily_budget_usd,omitempty"`
	SessionBudgetUSD  float64 `yaml:"session_budget_usd,omitempty"`
	DailyTokenLimit   int     `yaml:"daily_token_limit,omitempty"`
	SessionTokenLimit int     `yaml:"session_token_limit,omitempty"`
	// Prompt A/B 框架（见 internal/promptab）
	//  - PromptVariant: 固定选择某 variant（"default" / "concise" / "safety-first" / 用户自定义）
	//  - PromptStrategy: 选择策略（fixed / round-robin / weighted）
	//  - PromptWeights: 加权选择的权重（variant name → weight）
	PromptVariant   string         `yaml:"prompt_variant,omitempty"`
	PromptStrategy  string         `yaml:"prompt_strategy,omitempty"`
	PromptWeights   map[string]int `yaml:"prompt_weights,omitempty"`
	// PromptProjectDir: 项目级 prompts 目录覆盖（默认 .codecast/prompts）
	PromptProjectDir string `yaml:"prompt_project_dir,omitempty"`
}

// MaskedAPIKey 返回遮蔽后的 API Key（仅显示前4位和后4位）
func (c *Config) MaskedAPIKey() string {
	if len(c.APIKey) <= 8 {
		return "****"
	}
	return c.APIKey[:4] + "****" + c.APIKey[len(c.APIKey)-4:]
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		Model:    "gpt-4o",
		Provider: "openai",
	}
}

// Load 从文件和环境变量加载配置
func Load() *Config {
	cfg := Default()

	// 尝试从文件加载
	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".codecast", "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				log.Printf("警告: 配置文件解析失败: %v", err)
			}
		}
	}

	// 环境变量覆盖
	if v := os.Getenv("CODECAST_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("CODECAST_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("CODECAST_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv("CODECAST_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("CODECAST_PERMISSION_MODE"); v != "" {
		cfg.PermissionMode = v
	}
	if v := os.Getenv("CODECAST_SAFE_MODE"); v == "true" || v == "1" {
		cfg.SafeMode = true
	}
	if v := os.Getenv("CODECAST_AUTO_COMPACT"); v == "true" || v == "1" {
		cfg.AutoCompact = true
	}
	if v := os.Getenv("CODECAST_AUTO_CHECKPOINT"); v == "true" || v == "1" {
		cfg.AutoCheckpoint = true
	}
	// Prompt A/B 框架
	if v := os.Getenv("CODECAST_PROMPT_VARIANT"); v != "" {
		cfg.PromptVariant = v
	}
	if v := os.Getenv("CODECAST_PROMPT_STRATEGY"); v != "" {
		cfg.PromptStrategy = v
	}

	// Viper 覆盖（命令行 flags 绑定的值）
	if viperVal := getViperString("model"); viperVal != "" {
		cfg.Model = viperVal
	}
	if viperVal := getViperString("provider"); viperVal != "" {
		cfg.Provider = viperVal
	}
	if viperVal := getViperString("api_key"); viperVal != "" {
		cfg.APIKey = viperVal
	}
	if viperVal := getViperString("base_url"); viperVal != "" {
		cfg.BaseURL = viperVal
	}
	if viperVal := getViperString("permission_mode"); viperVal != "" {
		cfg.PermissionMode = viperVal
	}
	if viperBool := getViperBool("safe_mode"); viperBool {
		cfg.SafeMode = true
	}
	if viperBool := getViperBool("auto_compact"); viperBool {
		cfg.AutoCompact = true
	}
	if viperVal := getViperFloat64("auto_compact_ratio"); viperVal > 0 {
		cfg.AutoCompactRatio = viperVal
	}
	if viperBool := getViperBool("auto_checkpoint"); viperBool {
		cfg.AutoCheckpoint = true
	}
	if viperBool := getViperBool("auto_stash"); viperBool {
		cfg.AutoStash = true
	}
	if viperVal := getViperFloat64("daily_budget_usd"); viperVal > 0 {
		cfg.DailyBudgetUSD = viperVal
	}
	if viperVal := getViperFloat64("session_budget_usd"); viperVal > 0 {
		cfg.SessionBudgetUSD = viperVal
	}
	if viperVal := getViperInt("daily_token_limit"); viperVal > 0 {
		cfg.DailyTokenLimit = viperVal
	}
	if viperVal := getViperInt("session_token_limit"); viperVal > 0 {
		cfg.SessionTokenLimit = viperVal
	}
	if viperVal := getViperString("prompt_variant"); viperVal != "" {
		cfg.PromptVariant = viperVal
	}
	if viperVal := getViperString("prompt_strategy"); viperVal != "" {
		cfg.PromptStrategy = viperVal
	}
	if viperVal := getViperString("prompt_project_dir"); viperVal != "" {
		cfg.PromptProjectDir = viperVal
	}
	if viper.IsSet("prompt_weights") {
		// viper 解析 map[string]int 需要通过 AllSettings
		all := viper.AllSettings()
		if w, ok := all["prompt_weights"].(map[string]interface{}); ok {
			cfg.PromptWeights = make(map[string]int, len(w))
			for k, v := range w {
				switch n := v.(type) {
				case int:
					cfg.PromptWeights[k] = n
				case int64:
					cfg.PromptWeights[k] = int(n)
				case float64:
					cfg.PromptWeights[k] = int(n)
				}
			}
		}
	}

	return cfg
}

// Save 保存配置到文件
func Save(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".codecast")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	// 在 YAML 数据前添加安全警告注释
	warning := "# ⚠️ 警告: API Key 以明文存储。请勿将此文件提交到版本控制系统。\n# Warning: API Key is stored in plaintext. Do not commit this file to version control.\n"
	data = append([]byte(warning), data...)

	return os.WriteFile(configPath, data, 0600)
}

// GetConfigDir 返回配置目录
func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codecast"
	}
	return filepath.Join(home, ".codecast")
}

// Validate 验证配置是否有效
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("API Key 未配置，请使用 /config set api_key <key> 或设置 CODECAST_API_KEY 环境变量")
	}
	if c.Model == "" {
		return fmt.Errorf("模型未配置")
	}
	if c.Provider == "" {
		return fmt.Errorf("Provider 未配置")
	}
	return nil
}

// Viper 辅助函数（安全读取 viper 值）
func getViperString(key string) string {
	if viper.IsSet(key) {
		return viper.GetString(key)
	}
	return ""
}

func getViperBool(key string) bool {
	if viper.IsSet(key) {
		return viper.GetBool(key)
	}
	return false
}

func getViperFloat64(key string) float64 {
	if viper.IsSet(key) {
		return viper.GetFloat64(key)
	}
	return 0
}

func getViperInt(key string) int {
	if viper.IsSet(key) {
		return viper.GetInt(key)
	}
	return 0
}
