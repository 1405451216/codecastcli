package context

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ap "agentprimordia/pkg"
)

// mockSummary 模拟成功的 LLM 摘要
func mockSummary(_ context.Context, prompt string) (string, error) {
	return "用户要求重构 auth 模块；修改了 3 个文件：auth.go、session.go、user.go。修复了 1 个 panic。当前进度：完成 70%。", nil
}

// failSummary 模拟失败的 LLM 调用
func failSummary(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("LLM 调用失败：连接超时")
}

// 构造历史消息的辅助函数
func mkMsgs(n int) []ap.Message {
	out := make([]ap.Message, n)
	for i := 0; i < n; i++ {
		role := ap.RoleUser
		if i%2 == 1 {
			role = ap.RoleAssistant
		}
		out[i] = ap.Message{
			Role:     role,
			Content:  fmt.Sprintf("message %d", i),
			Metadata: ap.Metadata{Timestamp: time.Now()},
		}
	}
	return out
}

// TestCompressor_FewMessages 验证消息太少时不压缩
func TestCompressor_FewMessages(t *testing.T) {
	c := NewCompressor(4) // preserveRecent = 8
	msgs := mkMsgs(5)    // 5 < 8+1，不压缩
	out, err := c.Compress(context.Background(), msgs, mockSummary)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(out) != len(msgs) {
		t.Errorf("len(out) = %d, want %d (unchanged)", len(out), len(msgs))
	}
}

// TestCompressor_Empty 验证空消息不报错
func TestCompressor_Empty(t *testing.T) {
	c := NewCompressor(4)
	out, err := c.Compress(context.Background(), nil, mockSummary)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil", out)
	}
}

// TestCompressor_HappyPath 验证摘要注入成功
func TestCompressor_HappyPath(t *testing.T) {
	c := NewCompressor(2) // preserveRecent = 4
	msgs := mkMsgs(20)   // 20 条
	// 注入 1 条 system 消息保留
	msgs = append([]ap.Message{{Role: "system", Content: "you are a coding assistant"}}, msgs...)

	out, err := c.Compress(context.Background(), msgs, mockSummary)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 应包含：1 system + 1 摘要 system + 4 recent = 6
	if len(out) != 6 {
		t.Errorf("len(out) = %d, want 6 (sys + summary + 4 recent)", len(out))
	}
	// 验证 system 消息保留
	if out[0].Role != "system" || out[0].Content != "you are a coding assistant" {
		t.Errorf("out[0] = %+v, want original system message", out[0])
	}
	// 验证摘要消息注入
	if out[1].Role != "system" {
		t.Errorf("out[1].Role = %q, want system", out[1].Role)
	}
	if !strings.Contains(out[1].Content, "[上下文摘要") {
		t.Errorf("out[1].Content missing summary marker: %q", out[1].Content)
	}
	if extra := out[1].Metadata.Extra["summary"]; extra != "true" {
		t.Errorf("out[1] metadata.summary = %q, want true", extra)
	}
	// 验证 recent 是原消息的最后 4 条
	if out[5].Content != "message 19" {
		t.Errorf("out[5].Content = %q, want message 19", out[5].Content)
	}
}

// TestCompressor_SummaryFailure_FallsBackToPartial 摘要失败时降级
func TestCompressor_SummaryFailure_FallsBackToPartial(t *testing.T) {
	c := NewCompressor(2) // preserveRecent = 4
	msgs := mkMsgs(20)
	msgs = append([]ap.Message{{Role: "system", Content: "sys"}}, msgs...)

	out, err := c.Compress(context.Background(), msgs, failSummary)
	if err != nil {
		t.Fatalf("err = %v, want nil (degraded)", err)
	}
	// 降级：sys + 后一半 (20/2=10)
	// 期望：1 sys + 10 = 11
	if len(out) != 11 {
		t.Errorf("len(out) = %d, want 11 (sys + half of non-sys)", len(out))
	}
	// 不应该有摘要 system 消息
	for _, m := range out {
		if extra, ok := m.Metadata.Extra["summary"]; ok && extra == "true" {
			t.Errorf("degraded path should not contain summary message")
		}
	}
}

// TestCompressor_EmptySummaryTreatedAsFailure 空摘要走降级
func TestCompressor_EmptySummaryTreatedAsFailure(t *testing.T) {
	c := NewCompressor(2)
	msgs := mkMsgs(20)
	emptyFn := func(_ context.Context, _ string) (string, error) {
		return "", nil
	}
	out, err := c.Compress(context.Background(), msgs, emptyFn)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 降级路径：不应有 summary
	for _, m := range out {
		if extra, ok := m.Metadata.Extra["summary"]; ok && extra == "true" {
			t.Errorf("empty summary should trigger fallback")
		}
	}
}

// TestCompressor_HighlightsExtraction 验证高亮提取
func TestCompressor_HighlightsExtraction(t *testing.T) {
	c := NewCompressor(1) // preserveRecent = 2
	msgs := []ap.Message{
		{Role: "user", Content: "please edit_file auth.go and write_file user.go"},
		{Role: "assistant", Content: "I will refactor session.go now"},
		{Role: "assistant", Content: "got error: undefined symbol foo"},
		{Role: "user", Content: "fix the panic please"},
		{Role: "assistant", Content: "fixed it"},
		{Role: "user", Content: "now add tests"},
	}
	// 6 条非系统消息，preserveRecent=2 => oldMsgs=4
	hl := c.extractHighlights(msgs[:4])
	if !strings.Contains(hl, "[文件操作]") {
		t.Errorf("highlights missing [文件操作]: %q", hl)
	}
	if !strings.Contains(hl, "[错误]") {
		t.Errorf("highlights missing [错误]: %q", hl)
	}
}

// TestCompressor_HighlightsEmpty 无关键字时返回空
func TestCompressor_HighlightsEmpty(t *testing.T) {
	c := NewCompressor(1)
	msgs := []ap.Message{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "hi there"},
	}
	hl := c.extractHighlights(msgs)
	if hl != "" {
		t.Errorf("hl = %q, want empty", hl)
	}
}

// TestCompressor_PreservesSystemMessages 验证所有 system 消息都保留
func TestCompressor_PreservesSystemMessages(t *testing.T) {
	c := NewCompressor(1)
	msgs := []ap.Message{
		{Role: "system", Content: "sys A"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "system", Content: "sys B"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a3"},
	}
	// 6 个非系统，preserveRecent=2 => old=4
	out, err := c.Compress(context.Background(), msgs, mockSummary)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 应包含 2 sys + 1 summary + 2 recent = 5
	if len(out) != 5 {
		t.Errorf("len(out) = %d, want 5", len(out))
	}
	// 验证 sys A 和 sys B 都在
	sysCount := 0
	for _, m := range out {
		if m.Role == "system" && m.Content != "" && !strings.Contains(m.Content, "[上下文摘要") {
			sysCount++
		}
	}
	if sysCount != 2 {
		t.Errorf("preserved sys count = %d, want 2", sysCount)
	}
}

// TestCompressor_SummaryTruncation 验证超长摘要被截断
func TestCompressor_SummaryTruncation(t *testing.T) {
	c := &Compressor{
		preserveRecent:  2,
		maxSummaryChars: 50,
	}
	longFn := func(_ context.Context, _ string) (string, error) {
		return strings.Repeat("x", 200), nil
	}
	msgs := mkMsgs(10)
	out, err := c.Compress(context.Background(), msgs, longFn)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 找摘要消息
	for _, m := range out {
		if extra, ok := m.Metadata.Extra["summary"]; ok && extra == "true" {
			if !strings.Contains(m.Content, "...(摘要截断)") {
				t.Errorf("summary not truncated: len=%d, content tail: %q",
					len(m.Content), m.Content[max(0, len(m.Content)-30):])
			}
			return
		}
	}
	t.Errorf("no summary message found in output")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// TestCompressor_Truncate 验证内部 truncate 函数
func TestCompressor_Truncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short = %q, want hello", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate long = %q, want hello...", got)
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("truncate empty = %q, want empty", got)
	}
}

// TestCompressor_AllSystem_NoCompress 全部是 system 消息时不压缩
func TestCompressor_AllSystem_NoCompress(t *testing.T) {
	c := NewCompressor(2)
	msgs := []ap.Message{
		{Role: "system", Content: "sys1"},
		{Role: "system", Content: "sys2"},
		{Role: "system", Content: "sys3"},
		{Role: "system", Content: "sys4"},
		{Role: "system", Content: "sys5"},
		{Role: "system", Content: "sys6"},
		{Role: "system", Content: "sys7"},
		{Role: "system", Content: "sys8"},
		{Role: "system", Content: "sys9"},
		{Role: "system", Content: "sys10"},
	}
	out, err := c.Compress(context.Background(), msgs, mockSummary)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 非系统消息数为 0，<= preserveRecent(4) 应直接返回
	if len(out) != len(msgs) {
		t.Errorf("len(out) = %d, want %d (no compression when all system)",
			len(out), len(msgs))
	}
}
