package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"codecast/cli/internal/session"
	"codecast/cli/internal/ui"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// rootSessionCmd 是 `codecast session` 根命令。
//
// 在 v0.2.0 中，session 子命令已迁移到交互模式的 `/session` 斜杠命令。
var rootSessionCmd = &cobra.Command{
	Use:   "session",
	Short: "会话管理（已迁移: 在交互模式中使用 /session）",
	Long: `⚠️  codecast session 子命令已在 v0.2.0 中迁移到交互模式。

请在交互模式（运行 codecast 后）中直接使用 /session 斜杠命令：

  /session list                       — 列出所有会话
  /session show <session-id>          — 查看会话历史
  /session delete <session-id>        — 删除会话
  /session export <session-id> [file] — 导出会话为 Markdown`,
	Run: func(cmd *cobra.Command, args []string) {
		color.Yellow("⚠️  `codecast session` 子命令已在 v0.2.0 中迁移到交互模式。")
		fmt.Println()
		color.Cyan("请在交互模式中使用 /session 斜杠命令：")
		color.White("  /session list               — 列出所有会话")
		color.White("  /session show <id>          — 查看会话历史")
		color.White("  /session delete <id>        — 删除会话")
		color.White("  /session export <id> [file] — 导出会话为 Markdown")
		fmt.Println()
		color.White("直接运行 `codecast` 进入交互模式，然后输入 /session 即可。")
	},
}

// ============== 可复用函数 ==============

// sessionRunList 列出所有会话
func sessionRunList(jsonOut bool) error {
	mgr, err := session.NewManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	sessions, err := mgr.List()
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}

	if len(sessions) == 0 {
		color.Yellow("暂无会话记录")
		return nil
	}

	color.Cyan("💬 会话列表 (%d)", len(sessions))
	fmt.Printf("  %-30s %-8s %-20s %-20s\n", "会话 ID", "消息数", "创建时间", "最后更新")
	fmt.Println(strings.Repeat("-", 85))
	for _, s := range sessions {
		sid := s.SessionID
		if len(sid) > 30 {
			sid = sid[:27] + "..."
		}
		fmt.Printf("  %-30s %-8d %-20s %-20s\n",
			sid, s.MessageCount,
			s.CreatedAt.Format("01-02 15:04"),
			s.UpdatedAt.Format("01-02 15:04"))
	}
	return nil
}

// sessionRunShow 查看会话历史
func sessionRunShow(sessionID string, jsonOut bool) error {
	mgr, err := session.NewManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	msgs, err := mgr.GetHistory(sessionID, 100)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(msgs)
	}

	if len(msgs) == 0 {
		color.Yellow("会话 %s 暂无消息", sessionID)
		return nil
	}

	color.Cyan("💬 会话历史: %s (%d 条消息)", sessionID, len(msgs))
	fmt.Println(strings.Repeat("-", 60))
	for _, msg := range msgs {
		timeStr := msg.CreatedAt.Format("15:04:05")
		switch msg.Role {
		case "user":
			color.Cyan("[%s] 用户:", timeStr)
			fmt.Println(msg.Content)
		case "assistant":
			color.Green("[%s] 助手:", timeStr)
			fmt.Println(msg.Content)
		default:
			fmt.Printf("[%s] %s:\n%s\n", timeStr, msg.Role, msg.Content)
		}
		fmt.Println()
	}
	return nil
}

// sessionRunDelete 删除会话
func sessionRunDelete(sessionID string) error {
	mgr, err := session.NewManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	if err := mgr.Delete(sessionID); err != nil {
		return err
	}
	ui.PrintSuccess(fmt.Sprintf("会话 %s 已删除", sessionID))
	return nil
}

// sessionRunExport 导出会话为 Markdown
func sessionRunExport(sessionID, outputFile string) error {
	if outputFile == "" {
		outputFile = sessionID + ".md"
	}

	mgr, err := session.NewManager()
	if err != nil {
		return err
	}
	defer mgr.Close()

	msgs, err := mgr.GetHistory(sessionID, 1000)
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 会话记录: %s\n\n", sessionID))
	sb.WriteString(fmt.Sprintf("> 导出时间: %s\n\n", timeNow()))
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("## 用户 (%s)\n\n%s\n\n", msg.CreatedAt.Format("15:04:05"), msg.Content))
		case "assistant":
			sb.WriteString(fmt.Sprintf("## 助手 (%s)\n\n%s\n\n", msg.CreatedAt.Format("15:04:05"), msg.Content))
		default:
			sb.WriteString(fmt.Sprintf("## %s (%s)\n\n%s\n\n", msg.Role, msg.CreatedAt.Format("15:04:05"), msg.Content))
		}
	}

	if err := os.WriteFile(outputFile, []byte(sb.String()), 0644); err != nil {
		return err
	}
	ui.PrintSuccess(fmt.Sprintf("会话已导出到 %s", outputFile))
	return nil
}

// timeNow 返回当前时间的格式化字符串
func timeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func init() {
	// 仅保留 rootSessionCmd，不再注册子命令
	rootCmd.AddCommand(rootSessionCmd)
}
