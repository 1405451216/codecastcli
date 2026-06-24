package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"codecast/cli/internal/docs"
	"codecast/cli/internal/logx"
	"codecast/cli/internal/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:     "codecast",
	Short:   "Codecast AI Agent CLI - 基于 AgentPrimordia 的终端 AI 助手",
	Version: version.ShortVersion(),
	Long: `Codecast CLI 是一个 AI 驱动的终端 Agent 工具。
基于 AgentPrimordia 框架，支持智能对话、代码生成、文件操作、命令执行等功能。

支持 13+ LLM Provider: OpenAI / Anthropic / Gemini / Ollama / DeepSeek / Qwen / GLM 等`,
	Run: func(cmd *cobra.Command, args []string) {
		// F-10：runInteractive 现在返回 error，便于测试/包装
		if err := runInteractive(); err != nil {
			color.Red("启动失败: %v", err)
			// 注意：cobra 的 Execute() 内部会调用 os.Exit，
			// 所以这里的 os.Exit(1) 是冗余但无害的防御性退出。
			// 测试时可通过 RunE + mock 替代此路径。
			os.Exit(1)
		}
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径")
	rootCmd.PersistentFlags().StringP("model", "m", "gpt-4o", "使用的 AI 模型")
	rootCmd.PersistentFlags().StringP("provider", "p", "openai", "LLM Provider (openai/anthropic/gemini/ollama/deepseek/qwen/glm)")
	rootCmd.PersistentFlags().StringP("api-key", "k", "", "API Key")
	rootCmd.PersistentFlags().StringP("base-url", "u", "", "API Base URL (可选)")
	rootCmd.PersistentFlags().String("log-level", "info", "日志级别 (debug/info/warn/error)")
	rootCmd.PersistentFlags().String("log-format", "text", "日志格式 (text/json)")
	rootCmd.PersistentFlags().BoolP("continue", "c", false, "继续最近的会话")
	rootCmd.PersistentFlags().StringP("resume", "r", "", "恢复指定会话 ID")
	rootCmd.PersistentFlags().String("permission-mode", "auto-edit", "权限审批模式 (suggest/auto-edit/full-auto)")
	rootCmd.PersistentFlags().StringArray("scope", []string{}, "文件访问范围（可多次指定，默认为当前目录）")
	rootCmd.PersistentFlags().Bool("safe", false, "安全模式（禁用 Shell 和 Web 工具）")
	// --tui flag: 启用 Bubble Tea TUI 模式
	// 集成方式：在 rootCmd.Run 中检查 viper.GetBool("tui")，
	// 若为 true 则调用 agent.RunTUI(ag, cfg.Model, cfg.PermissionMode) 替代 runInteractive()。
	// 示例：
	//   if viper.GetBool("tui") {
	//       ag, err := agent.NewCodecastAgent(cfg, sessionID)
	//       if err != nil { ... }
	//       return agent.RunTUI(ag, cfg.Model, cfg.PermissionMode)
	//   }
	rootCmd.PersistentFlags().Bool("tui", false, "启用 Bubble Tea TUI 界面（替代 go-prompt REPL）")
	// F1: 智能上下文管理
	rootCmd.PersistentFlags().Bool("auto-compact", false, "上下文接近上限时自动压缩")
	rootCmd.PersistentFlags().Float64("auto-compact-ratio", 0.8, "自动压缩触发比例（0.0-1.0）")
	// F4: 自动 Git Checkpoint
	rootCmd.PersistentFlags().Bool("auto-checkpoint", false, "文件修改前自动创建 Git 检查点")
	rootCmd.PersistentFlags().Bool("auto-stash", true, "使用 git stash 而非 git commit 作为检查点")
	// F8: 成本预算控制
	rootCmd.PersistentFlags().Float64("daily-budget", 0, "每日预算上限（USD，0=不限制）")
	rootCmd.PersistentFlags().Float64("session-budget", 0, "每会话预算上限（USD，0=不限制）")
	// Prompt A/B 框架（见 internal/promptab）
	rootCmd.PersistentFlags().String("prompt-variant", "", "系统提示词变体 (default/concise/safety-first/用户自定义)")
	rootCmd.PersistentFlags().String("prompt-strategy", "fixed", "变体选择策略 (fixed/round-robin/weighted)")
	rootCmd.PersistentFlags().String("prompt-project-dir", "", "项目级 prompts 目录（默认 .codecast/prompts）")
	rootCmd.PersistentFlags().StringToInt("prompt-weight", map[string]int{}, "加权选择权重，可多次指定，如 --prompt-weight default=5")

	if err := viper.BindPFlag("model", rootCmd.PersistentFlags().Lookup("model")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 model flag 失败:", err)
	}
	if err := viper.BindPFlag("provider", rootCmd.PersistentFlags().Lookup("provider")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 provider flag 失败:", err)
	}
	if err := viper.BindPFlag("api_key", rootCmd.PersistentFlags().Lookup("api-key")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 api-key flag 失败:", err)
	}
	if err := viper.BindPFlag("base_url", rootCmd.PersistentFlags().Lookup("base-url")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 base-url flag 失败:", err)
	}
	if err := viper.BindPFlag("log_level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 log-level flag 失败:", err)
	}
	if err := viper.BindPFlag("log_format", rootCmd.PersistentFlags().Lookup("log-format")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 log-format flag 失败:", err)
	}
	if err := viper.BindPFlag("continue", rootCmd.PersistentFlags().Lookup("continue")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 continue flag 失败:", err)
	}
	if err := viper.BindPFlag("resume", rootCmd.PersistentFlags().Lookup("resume")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 resume flag 失败:", err)
	}
	if err := viper.BindPFlag("permission_mode", rootCmd.PersistentFlags().Lookup("permission-mode")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 permission-mode flag 失败:", err)
	}
	if err := viper.BindPFlag("scopes", rootCmd.PersistentFlags().Lookup("scope")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 scope flag 失败:", err)
	}
	if err := viper.BindPFlag("safe_mode", rootCmd.PersistentFlags().Lookup("safe")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 safe flag 失败:", err)
	}
	if err := viper.BindPFlag("tui", rootCmd.PersistentFlags().Lookup("tui")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 tui flag 失败:", err)
	}
	if err := viper.BindPFlag("auto_compact", rootCmd.PersistentFlags().Lookup("auto-compact")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 auto-compact flag 失败:", err)
	}
	if err := viper.BindPFlag("auto_compact_ratio", rootCmd.PersistentFlags().Lookup("auto-compact-ratio")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 auto-compact-ratio flag 失败:", err)
	}
	if err := viper.BindPFlag("auto_checkpoint", rootCmd.PersistentFlags().Lookup("auto-checkpoint")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 auto-checkpoint flag 失败:", err)
	}
	if err := viper.BindPFlag("auto_stash", rootCmd.PersistentFlags().Lookup("auto-stash")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 auto-stash flag 失败:", err)
	}
	if err := viper.BindPFlag("daily_budget_usd", rootCmd.PersistentFlags().Lookup("daily-budget")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 daily-budget flag 失败:", err)
	}
	if err := viper.BindPFlag("session_budget_usd", rootCmd.PersistentFlags().Lookup("session-budget")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 session-budget flag 失败:", err)
	}
	if err := viper.BindPFlag("prompt_variant", rootCmd.PersistentFlags().Lookup("prompt-variant")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 prompt-variant flag 失败:", err)
	}
	if err := viper.BindPFlag("prompt_strategy", rootCmd.PersistentFlags().Lookup("prompt-strategy")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 prompt-strategy flag 失败:", err)
	}
	if err := viper.BindPFlag("prompt_project_dir", rootCmd.PersistentFlags().Lookup("prompt-project-dir")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 prompt-project-dir flag 失败:", err)
	}
	if err := viper.BindPFlag("prompt_weights", rootCmd.PersistentFlags().Lookup("prompt-weight")); err != nil {
		fmt.Fprintln(os.Stderr, "绑定 prompt-weight flag 失败:", err)
	}

	// 初始化日志系统
	level, _ := logx.ParseLevel(viper.GetString("log_level"))
	logx.Init(
		logx.WithLevel(level),
		logx.WithFormat(viper.GetString("log_format")),
		logx.WithOutput(logx.DefaultLogPath()),
	)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		viper.AddConfigPath(filepath.Join(home, ".codecast"))
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("CODECAST")

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

// F9: Man Page 命令
var manCmd = &cobra.Command{
	Use:   "man",
	Short: "生成 man page",
	Run: func(cmd *cobra.Command, args []string) {
		format, _ := cmd.Flags().GetString("format")
		switch format {
		case "groff":
			fmt.Println(docs.GenerateManPage())
		case "markdown", "md":
			fmt.Println(docs.GenerateMarkdownHelp())
		default:
			fmt.Println(docs.GenerateMarkdownHelp())
		}
	},
}

// Version 命令
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		full, _ := cmd.Flags().GetBool("full")
		if full {
			fmt.Println(version.FullVersion())
		} else {
			fmt.Println(version.ShortVersion())
		}
	},
}

func init() {
	manCmd.Flags().String("format", "markdown", "输出格式 (groff/markdown)")
	rootCmd.AddCommand(manCmd)

	versionCmd.Flags().Bool("full", false, "显示完整版本信息（含 commit 和构建时间）")
	rootCmd.AddCommand(versionCmd)
}
