package agent

// ab_integration.go: 把 internal/ab 收敛器接到 CodecastAgent。
//
// 接入点：
//   1. newAgent() 启动时懒加载 Converger（文件不存在 → 空状态）
//   2. recordCost() 末尾调 RecordOutcome(variant, tokens, cost, success)
//   3. 用户用 /undo 撤销 → success=false 信号
//   4. /fb y /n → 显式反馈信号
//   5. /model 切换 → LatencyTracker 立即计算本轮时延
//   6. 每 10 轮自动 Save 一次（防崩溃丢数据）

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/ab"
)

// abStateFileName 是 ab_state.json 在 ~/.codecast/ 下的文件名。
const abStateFileName = "ab_state.json"

// abSaveEveryN 每 N 轮自动 Save 一次（防崩溃丢数据）。
const abSaveEveryN = 10

// ABIntegration 包裹收敛器与时延追踪。
// 与 CodecastAgent 同生命周期，由 Close() 释放。
type ABIntegration struct {
	converger  *ab.Converger
	latency    *ab.LatencyTracker
	statePath  string
	mu         sync.Mutex
	turnsSince int // 上次 Save 后的轮数
	// currentRound 是当前等待 success/fail 判定的"轮"信息。
	// 同一会话的 success 信号（/fb y、用户回车接受）会回填到这一条。
	currentRound *abRoundInfo
	enabled      bool
}

// abRoundInfo 一轮调用的元数据。
// success 信号延迟到达时按 currentRound 命中最近一轮。
type abRoundInfo struct {
	variant  string
	tokens   int
	costUSD  float64
	startAt  time.Time
	resolved bool // true=已记账（无论 success/fail）
}

// LoadABIntegration 加载（或初始化）收敛器+时延追踪。
// statePath 可为 "" → 用默认 ~/.codecast/ab_state.json。
func LoadABIntegration(statePath string) *ABIntegration {
	if statePath == "" {
		home, _ := os.UserHomeDir()
		statePath = filepath.Join(home, ".codecast", abStateFileName)
	}
	c, err := ab.Load(statePath)
	if err != nil {
		// 加载失败时用空 converger（不影响主流程）
		c = ab.NewConverger(ab.DefaultConfig())
	}
	c.Config().StatePath = statePath
	return &ABIntegration{
		converger:  c,
		latency:    ab.NewLatencyTracker(200),
		statePath:  statePath,
		enabled:    true,
	}
}

// Converger 返回底层收敛器（供 /ab 斜杠命令展示）。
func (a *ABIntegration) Converger() *ab.Converger { return a.converger }

// Latency 返回时延追踪器。
func (a *ABIntegration) Latency() *ab.LatencyTracker { return a.latency }

// Enabled 收敛器是否启用。
func (a *ABIntegration) Enabled() bool {
	if a == nil {
		return false
	}
	return a.enabled
}

// SetEnabled 开关。关闭后所有 Record* 调用变成 no-op。
func (a *ABIntegration) SetEnabled(v bool) {
	if a == nil {
		return
	}
	a.enabled = v
}

// StartRound 标记一轮 LLM 调用的开始（记录变体名 + 起始时间）。
// 后续 EndRound 会计算时延并 RecordOutcome。
func (a *ABIntegration) StartRound(variant string) {
	if a == nil || !a.enabled {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currentRound = &abRoundInfo{
		variant: variant,
		startAt: time.Now(),
	}
}

// EndRound 标记一轮 LLM 调用结束。
// 调用方把 cost.Tracker 已算好的 tokens / costUSD 传进来。
// success=true 表示用户接受本轮输出。
func (a *ABIntegration) EndRound(tokens int, costUSD float64, success bool) {
	if a == nil || !a.enabled || a.converger == nil {
		return
	}
	a.mu.Lock()
	round := a.currentRound
	if round == nil {
		// 没有 StartRound 上下文（老调用路径），用空 variant
		round = &abRoundInfo{variant: ""}
	}
	round.resolved = true
	a.mu.Unlock()

	// 记时延
	latencyMs := float64(time.Since(round.startAt)) / float64(time.Millisecond)
	a.latency.Record(round.variant, latencyMs)

	// 记 outcome
	a.converger.RecordOutcome(round.variant, tokens, costUSD, success)

	// 累计轮数，定期 Save
	a.mu.Lock()
	a.turnsSince++
	shouldSave := a.turnsSince >= abSaveEveryN
	if shouldSave {
		a.turnsSince = 0
	}
	a.mu.Unlock()
	if shouldSave {
		_ = a.converger.Save()
	}
}

// Save 强制持久化。
func (a *ABIntegration) Save() error {
	if a == nil || a.converger == nil {
		return nil
	}
	return a.converger.Save()
}

// Close 收尾（最后一次 Save）。
func (a *ABIntegration) Close() error {
	if a == nil {
		return nil
	}
	return a.Save()
}

// ResolveSuccess 把当前未判定轮标记为 success/fail。
// 用于 /fb y、/fb n、/undo 撤销等延迟信号。
// 返回 true 表示命中了未判定轮。
func (a *ABIntegration) ResolveSuccess(success bool) bool {
	if a == nil || !a.enabled || a.converger == nil {
		return false
	}
	a.mu.Lock()
	round := a.currentRound
	if round == nil || round.resolved {
		a.mu.Unlock()
		return false
	}
	// 同一轮从"未知"切到 success/fail：
	// 直接 RecordOutcome（会叠加 samples；但 success 之前可能为 true）。
	// 为避免重复计数，本轮 EndRound 调用时应跳过 RecordOutcome。
	// 简化策略：把 round.resolved 置 true 并补记一次。
	round.resolved = true
	a.mu.Unlock()
	a.converger.RecordOutcome(round.variant, 0, 0, success)
	return true
}

// ComputeVariantEstimate 估算一次 LLM 调用的成本（USD），与 budget/recordCost 同步。
// 独立成函数避免 agent.go 与 budget 包循环依赖。
func ComputeVariantEstimate(modelID string, usage ap.AgentUsage) float64 {
	// 复制自 internal/agent/agent.go recordCost 的算法
	// 避免引入 internal/model 包做单测
	if usage.TotalTokens == 0 {
		return 0
	}
	// 用一个保守默认：$5/1M input + $15/1M output（与 gpt-4o 接近）
	// 实际值在 recordCost 里用 model.FindModel 算，这里只做兜底
	const (
		in  = 0.005 / 1000
		out = 0.015 / 1000
	)
	return float64(usage.PromptTokens)*in + float64(usage.CompletionTokens)*out
}
