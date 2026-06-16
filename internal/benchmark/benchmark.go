// Package benchmark 提供 Codecast 自建评估框架。
//
// 设计目标：
//   - 不依赖外部 LLM（用 mock provider 跑通框架）
//   - 覆盖 5 类任务：问答/修改/重构/调试/多文件
//   - 评估 4 个指标：成功率/成本/时延/工具调用数
//   - 可对比不同模型/变体/路由策略
//   - 输出 JSON + Markdown 报告
//
// 与 SWE-bench 的区别：
//   - SWE-bench 需要真实 GitHub issue + 测试套件，环境重
//   - 本框架是轻量级自评，适合 CI 跑 + 快速回归
//   - 任务集可手工构造，预期结果明确
package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// TaskType 任务类型
type TaskType string

const (
	TaskQuestion  TaskType = "question"  // 问答/解释
	TaskEdit      TaskType = "edit"      // 单点修改
	TaskRefactor  TaskType = "refactor"  // 重构
	TaskDebug     TaskType = "debug"     // 调试
	TaskMultiFile TaskType = "multi-file" // 多文件
)

// Difficulty 难度
type Difficulty string

const (
	DiffEasy   Difficulty = "easy"
	DiffMedium Difficulty = "medium"
	DiffHard   Difficulty = "hard"
)

// Task 单个 benchmark 任务
type Task struct {
	ID          string     `json:"id"`
	Type        TaskType   `json:"type"`
	Difficulty  Difficulty `json:"difficulty"`
	Description string     `json:"description"`
	// Input 用户输入
	Input string `json:"input"`
	// ExpectedKeywords 预期回答应包含的关键词（成功率判断）
	ExpectedKeywords []string `json:"expected_keywords"`
	// ExpectedFiles 预期会修改的文件（修改类任务）
	ExpectedFiles []string `json:"expected_files,omitempty"`
	// Timeout 超时秒数
	Timeout int `json:"timeout,omitempty"`
}

// Metrics 单次任务的指标
type Metrics struct {
	// Success 是否成功（回答包含预期关键词 或 修改了预期文件）
	Success bool `json:"success"`
	// LatencyMs 时延（毫秒）
	LatencyMs int64 `json:"latency_ms"`
	// TokensUsed 总 token 数
	TokensUsed int `json:"tokens_used"`
	// CostUSD 成本（美元）
	CostUSD float64 `json:"cost_usd"`
	// ToolCalls 工具调用次数
	ToolCalls int `json:"tool_calls"`
	// Error 错误信息（若有）
	Error string `json:"error,omitempty"`
}

// TaskResult 单个任务执行结果
type TaskResult struct {
	TaskID  string  `json:"task_id"`
	Metrics Metrics `json:"metrics"`
}

// Runner 执行器接口。
// 不同实现可对接真实 Agent / mock / 录制回放。
type Runner interface {
	// Run 执行单个任务，返回指标
	Run(ctx context.Context, task Task) (Metrics, error)
	// Name 执行器名称（用于报告标识）
	Name() string
}

// Suite benchmark 套件
type Suite struct {
	Tasks  []Task
	Runner Runner
}

// NewSuite 创建套件
func NewSuite(runner Runner) *Suite {
	return &Suite{Runner: runner}
}

// AddTask 添加任务
func (s *Suite) AddTask(t Task) {
	if t.Timeout == 0 {
		t.Timeout = 30
	}
	s.Tasks = append(s.Tasks, t)
}

// Run 执行所有任务，返回结果
func (s *Suite) Run(ctx context.Context) []TaskResult {
	results := make([]TaskResult, len(s.Tasks))
	var wg sync.WaitGroup
	// 串行执行（避免并发干扰 + token 限流）
	for i, task := range s.Tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()
			result := TaskResult{TaskID: t.ID}
			timeout := time.Duration(t.Timeout) * time.Second
			taskCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			start := time.Now()
			m, err := s.Runner.Run(taskCtx, t)
			m.LatencyMs = time.Since(start).Milliseconds()
			if err != nil {
				m.Error = err.Error()
				m.Success = false
			}
			result.Metrics = m
			results[idx] = result
		}(i, task)
		wg.Wait() // 串行：每个任务等待完成再下一个
	}
	return results
}

// Report 评估报告
type Report struct {
	RunnerName string         `json:"runner_name"`
	Timestamp  time.Time      `json:"timestamp"`
	Results    []TaskResult   `json:"results"`
	Summary    SummaryMetrics `json:"summary"`
}

// SummaryMetrics 汇总指标
type SummaryMetrics struct {
	TotalTasks     int     `json:"total_tasks"`
	SuccessCount   int     `json:"success_count"`
	SuccessRate    float64 `json:"success_rate"`
	AvgLatencyMs   int64   `json:"avg_latency_ms"`
	P95LatencyMs   int64   `json:"p95_latency_ms"`
	TotalTokens    int     `json:"total_tokens"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	AvgCostPerTask float64 `json:"avg_cost_per_task"`
	TotalToolCalls int     `json:"total_tool_calls"`
	// 按类型分组的成功率
	ByType map[TaskType]float64 `json:"by_type"`
}

// GenerateReport 生成报告
func GenerateReport(runnerName string, results []TaskResult) *Report {
	r := &Report{
		RunnerName: runnerName,
		Timestamp:  time.Now(),
		Results:    results,
	}
	r.Summary = computeSummary(results)
	return r
}

// computeSummary 计算汇总指标
func computeSummary(results []TaskResult) SummaryMetrics {
	s := SummaryMetrics{
		TotalTasks: len(results),
		ByType:     make(map[TaskType]float64),
	}
	if len(results) == 0 {
		return s
	}

	latencies := make([]int64, 0, len(results))

	for _, r := range results {
		if r.Metrics.Success {
			s.SuccessCount++
		}
		latencies = append(latencies, r.Metrics.LatencyMs)
		s.TotalTokens += r.Metrics.TokensUsed
		s.TotalCostUSD += r.Metrics.CostUSD
		s.TotalToolCalls += r.Metrics.ToolCalls
	}

	s.SuccessRate = float64(s.SuccessCount) / float64(s.TotalTasks)
	s.AvgCostPerTask = s.TotalCostUSD / float64(s.TotalTasks)

	// 平均时延
	var sumLatency int64
	for _, l := range latencies {
		sumLatency += l
	}
	s.AvgLatencyMs = sumLatency / int64(len(latencies))

	// P95 时延
	s.P95LatencyMs = percentile(latencies, 95)

	return s
}

// percentile 计算百分位数
func percentile(values []int64, p int) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// SaveJSON 保存为 JSON
func (r *Report) SaveJSON(path string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}
	data, err := jsonMarshalIndent(r)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveMarkdown 保存为 Markdown 报告
func (r *Report) SaveMarkdown(path string) error {
	dir := filepath.Dir(path)
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}
	md := r.Markdown()
	return os.WriteFile(path, []byte(md), 0644)
}

// Markdown 生成 Markdown 报告
func (r *Report) Markdown() string {
	var b []byte
	w := func(format string, args ...any) {
		b = append(b, []byte(fmt.Sprintf(format, args...))...)
	}
	w("# Codecast Benchmark Report\n\n")
	w("**Runner**: %s  \n", r.RunnerName)
	w("**Time**: %s  \n", r.Timestamp.Format("2006-01-02 15:04:05"))
	w("**Tasks**: %d  \n\n", r.Summary.TotalTasks)

	w("## Summary\n\n")
	w("| Metric | Value |\n|--------|-------|\n")
	w("| Success Rate | %.2f%% (%d/%d) |\n",
		r.Summary.SuccessRate*100, r.Summary.SuccessCount, r.Summary.TotalTasks)
	w("| Avg Latency | %dms |\n", r.Summary.AvgLatencyMs)
	w("| P95 Latency | %dms |\n", r.Summary.P95LatencyMs)
	w("| Total Tokens | %d |\n", r.Summary.TotalTokens)
	w("| Total Cost | $%.4f |\n", r.Summary.TotalCostUSD)
	w("| Avg Cost/Task | $%.4f |\n", r.Summary.AvgCostPerTask)
	w("| Total Tool Calls | %d |\n\n", r.Summary.TotalToolCalls)

	w("## Details\n\n")
	w("| Task | Success | Latency | Tokens | Cost | Tools | Error |\n")
	w("|------|---------|---------|--------|------|-------|-------|\n")
	for _, r := range r.Results {
		errMsg := r.Metrics.Error
		if len(errMsg) > 30 {
			errMsg = errMsg[:30] + "..."
		}
		succ := "✗"
		if r.Metrics.Success {
			succ = "✓"
		}
		w("| %s | %s | %dms | %d | $%.4f | %d | %s |\n",
			r.TaskID, succ, r.Metrics.LatencyMs, r.Metrics.TokensUsed,
			r.Metrics.CostUSD, r.Metrics.ToolCalls, errMsg)
	}
	return string(b)
}

// jsonMarshalIndent 序列化为带缩进的 JSON
func jsonMarshalIndent(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// jsonMarshal 序列化为 JSON
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
