package cmd

// interactive_session.go: 会话管理相关函数（从 interactive.go 拆分）
//
// 包含：会话导出、会话列表、last_session 读写、恢复信息显示。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"codecast/cli/internal/config"
	"codecast/cli/internal/session"

	"github.com/fatih/color"
)

func exportCurrentSession() {
	filename := fmt.Sprintf("codecast-session-%s.md", time.Now().Format("20060102-150405"))
	exportCurrentSessionTo(filename)
}

func exportCurrentSessionTo(filename string) {
	mgr, err := session.NewManager()
	if err != nil {
		color.Red("导出失败: %v", err)
		return
	}
	defer mgr.Close()

	sessions, err := mgr.List()
	if err != nil {
		color.Red("导出失败: %v", err)
		return
	}

	if len(sessions) == 0 {
		color.Yellow("没有可导出的会话记录")
		return
	}

	// 导出最近更新的会话
	sess := sessions[0]
	msgs, err := mgr.GetHistory(sess.SessionID, 1000)
	if err != nil {
		color.Red("导出失败: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 会话记录: %s\n\n", sess.SessionID))
	sb.WriteString(fmt.Sprintf("> 导出时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
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

	if err := os.WriteFile(filename, []byte(sb.String()), 0644); err != nil {
		color.Red("保存文件失败: %v", err)
		return
	}
	color.Green("✓ 会话已导出到 %s", filename)
}

func listSessions() {
	mgr, err := session.NewManager()
	if err != nil {
		color.Red("获取会话列表失败: %v", err)
		return
	}
	defer mgr.Close()

	sessions, err := mgr.List()
	if err != nil {
		color.Red("获取会话列表失败: %v", err)
		return
	}

	if len(sessions) == 0 {
		color.Yellow("暂无会话记录")
		return
	}

	color.Cyan("💬 会话列表 (%d)", len(sessions))
	for i, s := range sessions {
		fmt.Printf("  %d. %s (%d 条消息, 最后更新: %s)\n",
			i+1, s.SessionID, s.MessageCount, s.UpdatedAt.Format("01-02 15:04"))
	}
}

// lastSessionPath 返回 last_session 文件路径
func lastSessionPath() string {
	return filepath.Join(config.GetConfigDir(), "last_session")
}

// readLastSession 从 last_session 文件读取最近的会话 ID
func readLastSession() (string, error) {
	data, err := os.ReadFile(lastSessionPath())
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("last_session 文件为空")
	}
	return id, nil
}

// saveLastSession 将当前会话 ID 保存到 last_session 文件
func saveLastSession(sessionID string) error {
	configDir := config.GetConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(lastSessionPath(), []byte(sessionID+"\n"), 0644)
}

// displayResumeInfo 显示恢复会话的信息
func displayResumeInfo(sessionID string) {
	mgr, err := session.NewManager()
	if err != nil {
		color.Green("✓ 已恢复会话: %s", sessionID)
		return
	}
	defer mgr.Close()

	msgs, err := mgr.GetHistory(sessionID, 1000)
	if err != nil {
		color.Green("✓ 已恢复会话: %s", sessionID)
		return
	}

	color.Green("✓ 已恢复会话: %s (%d 条历史消息)", sessionID, len(msgs))

	// 显示最近 3 条消息摘要
	recentCount := 3
	if len(msgs) < recentCount {
		recentCount = len(msgs)
	}
	if recentCount > 0 {
		fmt.Println("最近消息:")
		start := len(msgs) - recentCount
		for i := start; i < len(msgs); i++ {
			msg := msgs[i]
			roleLabel := "用户"
			if msg.Role == "assistant" {
				roleLabel = "助手"
			}
			summary := truncateRunes(msg.Content, 50)
			fmt.Printf("  %d. [%s] %s\n", i-start+1, roleLabel, summary)
		}
	}
	fmt.Println()
}

// truncateRunes 按 rune 截断字符串到指定长度
func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}
