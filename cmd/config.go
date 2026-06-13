package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"codecast/cli/internal/config"
	"codecast/cli/internal/provider"
	"codecast/cli/internal/wizard"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootConfigCmd 是 `codecast config` 根命令。
//
// 在 v0.2.0 中，`codecast config` 子命令已迁移到交互模式的 `/config` 斜杠命令。
// 保留此根命令是为了在 shell 中直接调用时给出明确的迁移提示，
// 避免静默失败。所有实际逻辑现在由 handleConfigCommand 处理。
var rootConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "配置管理（已迁移: 在交互模式中使用 /config）",
	Long: `⚠️  codecast config 子命令已在 v0.2.0 中废弃。

请在交互模式（运行 codecast 后）中直接使用 /config 斜杠命令：

  /config                    查看当前配置
  /config get <key>          获取某个配置项
  /config set <key> <value>  设置某个配置项
  /config list               列出全部配置项
  /config wizard             启动交互式配置向导
  /config providers          列出支持的 LLM Provider
  /config init               初始化配置文件

完整配置项参见: /config list`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow("⚠️  `codecast config` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		color.Cyan("请在交互模式中使用 /config 斜杠命令：")
		color.White("  /config              — 查看当前配置")
		color.White("  /config get <key>    — 获取配置项")
		color.White("  /config set <k> <v>  — 设置配置项")
		color.White("  /config list         — 列出所有配置")
		color.White("  /config wizard       — 交互式配置向导")
		color.White("  /config providers    — 列出支持的 Provider")
		fmt.Println()
		color.White("直接运行 `codecast` 进入交互模式，然后输入 /config 即可。")
	},
}

// ============== 可复用函数（供 /config 斜杠命令和将来扩展使用） ==============

// configSet 设置一个配置项
func configSet(key, value string) error {
	cfg := config.Load()

	switch key {
	case "api_key":
		cfg.APIKey = value
	case "model":
		cfg.Model = value
	case "provider":
		cfg.Provider = value
	case "base_url":
		cfg.BaseURL = value
	case "permission_mode":
		cfg.PermissionMode = value
	case "safe_mode":
		cfg.SafeMode = value == "true" || value == "1"
	case "auto_compact":
		cfg.AutoCompact = value == "true" || value == "1"
	case "auto_compact_ratio":
		if f, err := parseFloat(value); err == nil {
			cfg.AutoCompactRatio = f
		} else {
			return fmt.Errorf("invalid float for auto_compact_ratio: %s", value)
		}
	case "auto_checkpoint":
		cfg.AutoCheckpoint = value == "true" || value == "1"
	case "auto_stash":
		cfg.AutoStash = value == "true" || value == "1"
	case "daily_budget_usd":
		if f, err := parseFloat(value); err == nil {
			cfg.DailyBudgetUSD = f
		} else {
			return fmt.Errorf("invalid float for daily_budget_usd: %s", value)
		}
	case "session_budget_usd":
		if f, err := parseFloat(value); err == nil {
			cfg.SessionBudgetUSD = f
		} else {
			return fmt.Errorf("invalid float for session_budget_usd: %s", value)
		}
	case "daily_token_limit":
		if n, err := parseInt(value); err == nil {
			cfg.DailyTokenLimit = n
		} else {
			return fmt.Errorf("invalid int for daily_token_limit: %s", value)
		}
	case "session_token_limit":
		if n, err := parseInt(value); err == nil {
			cfg.SessionTokenLimit = n
		} else {
			return fmt.Errorf("invalid int for session_token_limit: %s", value)
		}
	case "prompt_variant":
		cfg.PromptVariant = value
	case "prompt_strategy":
		switch value {
		case "fixed", "round-robin", "weighted", "weighted-random":
			cfg.PromptStrategy = value
		default:
			return fmt.Errorf("invalid prompt_strategy: %s (allowed: fixed/round-robin/weighted/weighted-random)", value)
		}
	case "prompt_project_dir":
		cfg.PromptProjectDir = value
	default:
		return fmt.Errorf("未知配置项: %s\n支持的配置项: api_key, model, provider, base_url, permission_mode, safe_mode, auto_compact, auto_compact_ratio, auto_checkpoint, auto_stash, daily_budget_usd, session_budget_usd, daily_token_limit, session_token_limit, prompt_variant, prompt_strategy, prompt_project_dir", key)
	}

	return config.Save(cfg)
}

// configGet 获取一个配置项的字符串表示
func configGet(key string) (string, error) {
	cfg := config.Load()

	switch key {
	case "api_key":
		if cfg.APIKey == "" {
			return "(未设置)", nil
		}
		if len(cfg.APIKey) <= 4 {
			return "***", nil
		}
		return "***" + cfg.APIKey[len(cfg.APIKey)-4:], nil
	case "model":
		return cfg.Model, nil
	case "provider":
		return cfg.Provider, nil
	case "base_url":
		return cfg.BaseURL, nil
	case "permission_mode":
		return cfg.PermissionMode, nil
	case "safe_mode":
		return strconv.FormatBool(cfg.SafeMode), nil
	case "auto_compact":
		return strconv.FormatBool(cfg.AutoCompact), nil
	case "auto_compact_ratio":
		return fmt.Sprintf("%.2f", cfg.AutoCompactRatio), nil
	case "auto_checkpoint":
		return strconv.FormatBool(cfg.AutoCheckpoint), nil
	case "auto_stash":
		return strconv.FormatBool(cfg.AutoStash), nil
	case "daily_budget_usd":
		return fmt.Sprintf("%.2f", cfg.DailyBudgetUSD), nil
	case "session_budget_usd":
		return fmt.Sprintf("%.2f", cfg.SessionBudgetUSD), nil
	case "daily_token_limit":
		return strconv.Itoa(cfg.DailyTokenLimit), nil
	case "session_token_limit":
		return strconv.Itoa(cfg.SessionTokenLimit), nil
	case "prompt_variant":
		if cfg.PromptVariant == "" {
			return "(未设置, 使用 default)", nil
		}
		return cfg.PromptVariant, nil
	case "prompt_strategy":
		if cfg.PromptStrategy == "" {
			return "fixed", nil
		}
		return cfg.PromptStrategy, nil
	case "prompt_project_dir":
		if cfg.PromptProjectDir == "" {
			return "(未设置, 使用 .codecast/prompts)", nil
		}
		return cfg.PromptProjectDir, nil
	default:
		return "", fmt.Errorf("未知配置项: %s", key)
	}
}

// configList 打印所有配置项
func configList() {
	cfg := config.Load()
	color.Yellow("当前配置:")
	fmt.Printf("  api_key:            %s\n", maskKey(cfg.APIKey))
	fmt.Printf("  model:              %s\n", cfg.Model)
	fmt.Printf("  provider:           %s\n", cfg.Provider)
	fmt.Printf("  base_url:           %s\n", cfg.BaseURL)
	fmt.Printf("  permission_mode:    %s\n", cfg.PermissionMode)
	fmt.Printf("  safe_mode:          %v\n", cfg.SafeMode)
	fmt.Printf("  auto_compact:       %v\n", cfg.AutoCompact)
	fmt.Printf("  auto_compact_ratio: %.2f\n", cfg.AutoCompactRatio)
	fmt.Printf("  auto_checkpoint:    %v\n", cfg.AutoCheckpoint)
	fmt.Printf("  auto_stash:         %v\n", cfg.AutoStash)
	fmt.Printf("  daily_budget_usd:   %.2f\n", cfg.DailyBudgetUSD)
	fmt.Printf("  session_budget_usd: %.2f\n", cfg.SessionBudgetUSD)
	fmt.Printf("  daily_token_limit:  %d\n", cfg.DailyTokenLimit)
	fmt.Printf("  session_token_limit:%d\n", cfg.SessionTokenLimit)
	promptVariant := cfg.PromptVariant
	if promptVariant == "" {
		promptVariant = "(default)"
	}
	promptStrategy := cfg.PromptStrategy
	if promptStrategy == "" {
		promptStrategy = "fixed"
	}
	promptProjectDir := cfg.PromptProjectDir
	if promptProjectDir == "" {
		promptProjectDir = "(.codecast/prompts)"
	}
	fmt.Printf("  prompt_variant:     %s\n", promptVariant)
	fmt.Printf("  prompt_strategy:    %s\n", promptStrategy)
	if len(cfg.PromptWeights) > 0 {
		fmt.Printf("  prompt_weights:     %v\n", cfg.PromptWeights)
	}
	fmt.Printf("  prompt_project_dir: %s\n", promptProjectDir)
}

// configInit 初始化配置文件
func configInit() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}
	configDir := filepath.Join(home, ".codecast")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	cfg := config.Default()
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("初始化配置失败: %w", err)
	}
	color.Green("✓ 配置文件已初始化: %s", filepath.Join(configDir, "config.yaml"))
	color.Yellow("请使用以下命令配置 API Key:")
	fmt.Println("  /config set api_key <your-api-key>")
	fmt.Println("  /config set provider openai")
	fmt.Println("  /config set model gpt-4o")
	return nil
}

// configWizard 运行交互式配置向导
func configWizard() error {
	color.Cyan("Codecast CLI 配置向导")
	color.White("========================\n")

	wizCfg, err := wizard.RunWizard()
	if err != nil {
		return fmt.Errorf("配置向导失败: %w", err)
	}

	cfg := config.Load()
	cfg.APIKey = wizCfg.APIKey
	cfg.Provider = wizCfg.Provider
	cfg.Model = wizCfg.Model
	cfg.PermissionMode = wizCfg.PermissionMode
	cfg.SafeMode = wizCfg.SafeMode

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	color.Green("\n✓ 配置已保存!")
	color.White("  Provider: %s", cfg.Provider)
	color.White("  Model:    %s", cfg.Model)
	return nil
}

// configProviders 列出支持的 Provider
func configProviders() {
	color.Yellow("支持的 LLM Provider:")
	for _, p := range provider.ListSupportedProviders() {
		fmt.Printf("  %-12s - %s\n", p, provider.ProviderDisplayName(p))
	}
}

// ============== 工具函数 ==============

func maskKey(key string) string {
	if key == "" {
		return "(未设置)"
	}
	if len(key) <= 4 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func init() {
	// 仅保留 rootConfigCmd，不再注册任何子命令。
	// 实际配置操作请在交互模式中通过 /config 斜杠命令完成。
	rootCmd.AddCommand(rootConfigCmd)
}
