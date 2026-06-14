// Package context: 摘要式上下文压缩 (Task 1.4)
//
// 区别于 compress.go 的预算/触发器（被动），
// compressor.go 提供主动的"摘要式"压缩：把旧消息浓缩成一段 system 摘要，
// 保留最近 N 轮原文，从而让长对话既能继续又能携带关键上下文。
//
// 设计要点：
//   - Compress 是纯函数（除传入的 summaryFn 之外无副作用），方便单测。
//   - 摘要失败时降级到"保留后一半消息"——比直接清空（ClearContext）更安全。
//   - 摘要文本超过 maxSummaryChars 时被截断并加标记，防止摘要本身撑爆上下文。
//   - 自动高亮文件操作/错误事件，提示 LLM 关注。
package context

import (
	"context"
	"fmt"
	"strings"
	"time"

	ap "agentprimordia/pkg"
)

// SummaryFn 由调用方注入的摘要生成函数。
// 典型实现：调用 LLM provider.Run(...) 拿到一段简短摘要。
type SummaryFn func(ctx context.Context, prompt string) (string, error)

// Compressor 摘要式压缩器
type Compressor struct {
	summaryModel    string
	preserveRecent  int // 保留最近 N 条消息（N 轮 = 2N 条）
	maxSummaryChars int
}

// NewCompressor 创建 Compressor。preserveRounds 传 0 则默认 4 轮。
func NewCompressor(preserveRounds int) *Compressor {
	if preserveRounds <= 0 {
		preserveRounds = 4
	}
	return &Compressor{
		preserveRecent:  preserveRounds * 2,
		maxSummaryChars: 2000,
	}
}

// Compress 摘要式压缩。
// 返回：压缩后的消息切片（system 消息 + 摘要 system 消息 + 最近消息）。
// 降级策略：摘要失败时返回 sysMsgs + 后一半非系统消息，err 为 nil。
func (c *Compressor) Compress(ctx context.Context, msgs []ap.Message, summaryFn SummaryFn) ([]ap.Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}
	// 太少不压缩（保留 1 条 system 缓冲）
	if len(msgs) <= c.preserveRecent+1 {
		return msgs, nil
	}

	var sysMsgs, nonSysMsgs []ap.Message
	for _, m := range msgs {
		if m.Role == "system" {
			sysMsgs = append(sysMsgs, m)
		} else {
			nonSysMsgs = append(nonSysMsgs, m)
		}
	}
	if len(nonSysMsgs) <= c.preserveRecent {
		return msgs, nil
	}

	oldMsgs := nonSysMsgs[:len(nonSysMsgs)-c.preserveRecent]
	recentMsgs := nonSysMsgs[len(nonSysMsgs)-c.preserveRecent:]

	// 1) 高亮（供 LLM 摘要时参考，也作为摘要前缀增强信号）
	highlights := c.extractHighlights(oldMsgs)

	// 2) 调 LLM 摘要；失败时降级
	summary, err := summaryFn(ctx, c.buildSummaryPrompt(oldMsgs, highlights))
	if err != nil || summary == "" {
		// 降级：保留后一半消息
		keep := nonSysMsgs[len(nonSysMsgs)/2:]
		out := append([]ap.Message{}, sysMsgs...)
		return append(out, keep...), nil
	}

	if len(summary) > c.maxSummaryChars {
		summary = summary[:c.maxSummaryChars] + "\n...(摘要截断)"
	}

	// 3) 装配
	summaryMsg := ap.Message{
		Role:    "system",
		Content: fmt.Sprintf("[上下文摘要 - 已压缩 %d 条旧消息]\n%s", len(oldMsgs), summary),
		Metadata: ap.Metadata{
			Timestamp: time.Now(),
			Extra: map[string]string{
				"summary":          "true",
				"compressed_count": fmt.Sprintf("%d", len(oldMsgs)),
			},
		},
	}

	out := append([]ap.Message{}, sysMsgs...)
	out = append(out, summaryMsg)
	out = append(out, recentMsgs...)
	return out, nil
}

// buildSummaryPrompt 构造摘要 prompt（带高亮提示）
func (c *Compressor) buildSummaryPrompt(messages []ap.Message, highlights string) string {
	var sb strings.Builder
	sb.WriteString("请用 200 字以内总结以下对话的关键信息：\n")
	sb.WriteString("- 用户要求了什么\n- 做了哪些文件修改（列出文件名）\n")
	sb.WriteString("- 遇到了哪些错误及如何解决的\n- 当前任务进度\n\n")
	if highlights != "" {
		sb.WriteString("关键事件:\n")
		sb.WriteString(highlights)
		sb.WriteString("\n\n")
	}
	sb.WriteString("对话内容:\n")
	for _, m := range messages {
		role := string(m.Role)
		if role == "" {
			role = "unknown"
		}
		content := m.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
	}
	return sb.String()
}

// extractHighlights 从旧消息中提取文件操作和错误事件
func (c *Compressor) extractHighlights(messages []ap.Message) string {
	var highlights []string
	for _, m := range messages {
		low := strings.ToLower(m.Content)
		if strings.Contains(low, "edit_file") || strings.Contains(low, "write_file") ||
			strings.Contains(low, "create_file") || strings.Contains(low, "delete_file") {
			highlights = append(highlights, fmt.Sprintf("[文件操作] %s", truncate(m.Content, 200)))
		}
		if strings.Contains(low, "error") || strings.Contains(low, "failed") ||
			strings.Contains(low, "panic") {
			highlights = append(highlights, fmt.Sprintf("[错误] %s", truncate(m.Content, 200)))
		}
	}
	if len(highlights) == 0 {
		return ""
	}
	return strings.Join(highlights, "\n---\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
