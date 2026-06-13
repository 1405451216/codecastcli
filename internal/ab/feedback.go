package ab

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// FeedbackCollector 收集用户对 agent 输出的"接受/拒绝"信号，
// 用于 ab.Converger 调整权重。
//
// 设计原则：
//   - 不阻塞主流程——仅在显式调用时询问
//   - 询问是非阻塞的（带短 timeout）
//   - 默认跳过（空输入）→ 不计入 success/failure
//   - 'y' / 'yes' → 记 success = true
//   - 'n' / 'no'  → 记 success = false
//   - 其他（Enter / 其他字符）→ 跳过
//
// 用法（典型场景）：
//
//	fb := ab.NewFeedbackCollector(converger)
//	fb.StartRound("claude-style", 1234, 0.0123)
//	// ... agent 输出 ...
//	fb.Ask()   // 阻塞 50ms 或用户输入
type FeedbackCollector struct {
	converger *Converger
	timeout   time.Duration
	// 当前轮：变体名 + 调用指标
	currentVar  string
	currentCost float64
	currentTok  int
}

// NewFeedbackCollector 创建反馈收集器
func NewFeedbackCollector(c *Converger) *FeedbackCollector {
	return &FeedbackCollector{
		converger: c,
		timeout:   50 * time.Millisecond,
	}
}

// StartRound 开始一轮反馈询问。
// 调用方在 agent.StreamProcess 完成（或类似回调）时调用。
func (f *FeedbackCollector) StartRound(variant string, tokens int, costUSD float64) {
	if f == nil {
		return
	}
	f.currentVar = variant
	f.currentCost = costUSD
	f.currentTok = tokens
}

// Ask 询问用户对上一轮输出的评价。
// 返回：
//   - asked: 是否询问了用户（true=用户输入了 y/n）
//   - success: true=接受；false=拒绝；仅在 asked=true 时有意义
//
// 简化实现：直接读 stdin 一行（短期阻塞 <= 用户输入时间）。
// 未来可改为非阻塞 + select + channel。
func (f *FeedbackCollector) Ask() (asked bool, success bool) {
	if f == nil || f.converger == nil || f.currentVar == "" {
		return false, false
	}
	fmt.Fprintf(os.Stderr,
		"\n[ab] 上一轮 (variant=%s, cost=$%.4f) 是否有效？y/n/Enter=skip: ",
		f.currentVar, f.currentCost)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(strings.ToLower(line))
	switch answer {
	case "y", "yes":
		f.converger.RecordOutcome(f.currentVar, f.currentTok, f.currentCost, true)
		return true, true
	case "n", "no":
		f.converger.RecordOutcome(f.currentVar, f.currentTok, f.currentCost, false)
		return true, false
	default:
		return false, false
	}
}
