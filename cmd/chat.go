package cmd

import (
	"context"
	"fmt"
	"strings"

	"codecast/cli/internal/agent"
	"codecast/cli/internal/config"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat [message]",
	Short: "单轮对话模式",
	Long:  "发送单条消息给 AI Agent 并获取回复",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		message := strings.Join(args, " ")

		cfg := config.Load()
		if err := cfg.Validate(); err != nil {
			color.Red("%v", err)
			return
		}

		codecastAgent, err := agent.New(cfg)
		if err != nil {
			color.Red("初始化 Agent 失败: %v", err)
			return
		}
		defer codecastAgent.Close()

		ctx := context.Background()
		if err := codecastAgent.StreamProcess(ctx, message); err != nil {
			color.Red("Error: %v", err)
		}
		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
