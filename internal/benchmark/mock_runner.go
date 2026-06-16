package benchmark

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// MockRunner 用于验证 benchmark 框架本身。
//
// 不调用任何真实 LLM，按预设概率返回成功/失败，
// 并模拟时延和 token 消耗。适合 CI 跑通流程。
type MockRunner struct {
	// Seed 随机种子（可复现）
	Seed int64
	// SuccessRate 模拟成功率（0-1）
	SuccessRate float64
	// BaseLatency 基础时延
	BaseLatency time.Duration
	// TokensPerTask 每任务 token 数
	TokensPerTask int
	// CostPerToken 每 token 成本（USD）
	CostPerToken float64
	// ToolCallsPerTask 每任务工具调用数
	ToolCallsPerTask int
	// NamePrefix 执行器名称前缀
	NamePrefix string

	rng *rand.Rand
}

// NewMockRunner 创建 mock runner
func NewMockRunner(seed int64, successRate float64) *MockRunner {
	return &MockRunner{
		Seed:             seed,
		SuccessRate:      successRate,
		BaseLatency:      100 * time.Millisecond,
		TokensPerTask:    500,
		CostPerToken:     0.00001,
		ToolCallsPerTask: 2,
		NamePrefix:       "mock",
		rng:              rand.New(rand.NewSource(seed)),
	}
}

// Name 执行器名称
func (m *MockRunner) Name() string {
	return m.NamePrefix + "-runner"
}

// Run 执行单个任务
func (m *MockRunner) Run(ctx context.Context, task Task) (Metrics, error) {
	// 模拟时延（按难度调整）
	latency := m.BaseLatency
	switch task.Difficulty {
	case DiffMedium:
		latency *= 2
	case DiffHard:
		latency *= 4
	}
	// 加随机抖动
	jitter := time.Duration(m.rng.Intn(100)) * time.Millisecond
	select {
	case <-time.After(latency + jitter):
	case <-ctx.Done():
		return Metrics{Error: ctx.Err().Error()}, ctx.Err()
	}

	// 模拟成功率
	success := m.rng.Float64() < m.SuccessRate

	// 模拟 token 消耗（按难度）
	tokens := m.TokensPerTask
	switch task.Difficulty {
	case DiffMedium:
		tokens *= 2
	case DiffHard:
		tokens *= 4
	}
	// 加随机
	tokens += m.rng.Intn(200)

	// 模拟工具调用数
	toolCalls := m.ToolCallsPerTask
	switch task.Type {
	case TaskEdit, TaskRefactor:
		toolCalls += 2
	case TaskMultiFile:
		toolCalls += 4
	case TaskDebug:
		toolCalls += 1
	}

	return Metrics{
		Success:    success,
		TokensUsed: tokens,
		CostUSD:    float64(tokens) * m.CostPerToken,
		ToolCalls:  toolCalls,
	}, nil
}

// KeywordMatchRunner 关键词匹配 runner。
//
// 把任务输入"假装"成 LLM 输出（直接拼上 ExpectedKeywords），
// 用于验证成功率判断逻辑本身是否正确。
type KeywordMatchRunner struct{}

// Name 执行器名称
func (KeywordMatchRunner) Name() string {
	return "keyword-match-runner"
}

// Run 把 ExpectedKeywords 拼成"回答"，验证 success 判断
func (KeywordMatchRunner) Run(ctx context.Context, task Task) (Metrics, error) {
	// 模拟"回答"：把预期关键词拼起来
	answer := strings.Join(task.ExpectedKeywords, " ")
	success := true
	for _, kw := range task.ExpectedKeywords {
		if !strings.Contains(answer, kw) {
			success = false
			break
		}
	}
	return Metrics{
		Success:    success,
		TokensUsed: len(task.ExpectedKeywords) * 10,
		CostUSD:    0,
		ToolCalls:  0,
	}, nil
}

// AssertSuccess 判断任务是否成功（暴露给真实 Runner 使用）
//
// 判断规则：
//   - 问答类：回答包含所有 ExpectedKeywords
//   - 修改/多文件类：回答包含关键词 且 修改了预期文件（若 ExpectedFiles 非空）
//   - 重构/调试类：回答包含关键词
func AssertSuccess(task Task, answer string, modifiedFiles []string) bool {
	// 关键词检查
	for _, kw := range task.ExpectedKeywords {
		if !strings.Contains(strings.ToLower(answer), strings.ToLower(kw)) {
			return false
		}
	}
	// 文件检查（仅当任务声明了 ExpectedFiles）
	if len(task.ExpectedFiles) > 0 {
		modifiedSet := make(map[string]bool, len(modifiedFiles))
		for _, f := range modifiedFiles {
			modifiedSet[f] = true
		}
		for _, expected := range task.ExpectedFiles {
			if !modifiedSet[expected] {
				return false
			}
		}
	}
	return true
}
