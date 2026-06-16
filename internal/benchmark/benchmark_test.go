package benchmark

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMockRunner(t *testing.T) {
	runner := NewMockRunner(42, 1.0) // 100% 成功率
	suite := NewDefaultSuite(runner)
	ctx := context.Background()
	results := suite.Run(ctx)

	if len(results) != len(DefaultTasks) {
		t.Fatalf("期望 %d 个结果，实际 %d", len(DefaultTasks), len(results))
	}

	successCount := 0
	for _, r := range results {
		if r.Metrics.Success {
			successCount++
		}
	}
	if successCount != len(DefaultTasks) {
		t.Errorf("100%% 成功率下应全部成功，实际 %d/%d", successCount, len(DefaultTasks))
	}
}

func TestMockRunnerZeroSuccess(t *testing.T) {
	runner := NewMockRunner(42, 0.0) // 0% 成功率
	suite := NewDefaultSuite(runner)
	results := suite.Run(context.Background())

	successCount := 0
	for _, r := range results {
		if r.Metrics.Success {
			successCount++
		}
	}
	if successCount != 0 {
		t.Errorf("0%% 成功率下应全部失败，实际 %d 成功", successCount)
	}
}

func TestAssertSuccess(t *testing.T) {
	tests := []struct {
		name          string
		task          Task
		answer        string
		modifiedFiles []string
		want          bool
	}{
		{
			name: "问答类-关键词全中",
			task: Task{
				Type:            TaskQuestion,
				ExpectedKeywords: []string{"Wilson", "区间"},
			},
			answer: "Wilson 置信区间是...",
			want:   true,
		},
		{
			name: "问答类-缺关键词",
			task: Task{
				Type:            TaskQuestion,
				ExpectedKeywords: []string{"Wilson", "贝叶斯"},
			},
			answer: "Wilson 置信区间是...",
			want:   false,
		},
		{
			name: "修改类-关键词+文件都对",
			task: Task{
				Type:            TaskEdit,
				ExpectedKeywords: []string{"buffer"},
				ExpectedFiles:   []string{"utils.go"},
			},
			answer:        "已改为 buffer",
			modifiedFiles: []string{"utils.go"},
			want:          true,
		},
		{
			name: "修改类-文件没改",
			task: Task{
				Type:            TaskEdit,
				ExpectedKeywords: []string{"buffer"},
				ExpectedFiles:   []string{"utils.go"},
			},
			answer:        "已改为 buffer",
			modifiedFiles: []string{"other.go"},
			want:          false,
		},
		{
			name: "大小写不敏感",
			task: Task{
				Type:            TaskQuestion,
				ExpectedKeywords: []string{"WILSON"},
			},
			answer: "wilson 区间",
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AssertSuccess(tt.task, tt.answer, tt.modifiedFiles)
			if got != tt.want {
				t.Errorf("AssertSuccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateReport(t *testing.T) {
	results := []TaskResult{
		{TaskID: "t1", Metrics: Metrics{Success: true, LatencyMs: 100, TokensUsed: 500, CostUSD: 0.01, ToolCalls: 2}},
		{TaskID: "t2", Metrics: Metrics{Success: false, LatencyMs: 200, TokensUsed: 800, CostUSD: 0.02, ToolCalls: 3}},
		{TaskID: "t3", Metrics: Metrics{Success: true, LatencyMs: 150, TokensUsed: 600, CostUSD: 0.015, ToolCalls: 1}},
	}
	report := GenerateReport("test-runner", results)

	if report.Summary.TotalTasks != 3 {
		t.Errorf("TotalTasks = %d, want 3", report.Summary.TotalTasks)
	}
	if report.Summary.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", report.Summary.SuccessCount)
	}
	if report.Summary.SuccessRate != 2.0/3.0 {
		t.Errorf("SuccessRate = %f, want %f", report.Summary.SuccessRate, 2.0/3.0)
	}
	if report.Summary.TotalTokens != 1900 {
		t.Errorf("TotalTokens = %d, want 1900", report.Summary.TotalTokens)
	}
	if report.Summary.TotalCostUSD != 0.045 {
		t.Errorf("TotalCostUSD = %f, want 0.045", report.Summary.TotalCostUSD)
	}
	if report.Summary.TotalToolCalls != 6 {
		t.Errorf("TotalToolCalls = %d, want 6", report.Summary.TotalToolCalls)
	}
	// 平均时延 = (100+200+150)/3 = 150
	if report.Summary.AvgLatencyMs != 150 {
		t.Errorf("AvgLatencyMs = %d, want 150", report.Summary.AvgLatencyMs)
	}
}

func TestPercentile(t *testing.T) {
	values := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	p50 := percentile(values, 50)
	if p50 != 50 && p50 != 60 { // 50% 可能在 50 或 60（看实现）
		// 接受 [50, 60]
		if p50 < 50 || p50 > 60 {
			t.Errorf("P50 = %d, 期望在 [50, 60]", p50)
		}
	}
	p95 := percentile(values, 95)
	if p95 != 100 && p95 != 90 {
		if p95 < 90 || p95 > 100 {
			t.Errorf("P95 = %d, 期望在 [90, 100]", p95)
		}
	}
}

func TestReportSaveJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "report.json")
	report := GenerateReport("test", []TaskResult{
		{TaskID: "t1", Metrics: Metrics{Success: true, LatencyMs: 100}},
	})
	if err := report.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON 失败: %v", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("文件未创建")
	}
}

func TestReportSaveMarkdown(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "report.md")
	report := GenerateReport("test", []TaskResult{
		{TaskID: "t1", Metrics: Metrics{Success: true, LatencyMs: 100, TokensUsed: 500, CostUSD: 0.01, ToolCalls: 2}},
	})
	if err := report.SaveMarkdown(path); err != nil {
		t.Fatalf("SaveMarkdown 失败: %v", err)
	}
	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, "Codecast Benchmark Report") {
		t.Error("Markdown 缺少标题")
	}
	if !strings.Contains(content, "Success Rate") {
		t.Error("Markdown 缺少成功率")
	}
}

func TestKeywordMatchRunner(t *testing.T) {
	runner := KeywordMatchRunner{}
	suite := NewDefaultSuite(runner)
	results := suite.Run(context.Background())

	if len(results) != len(DefaultTasks) {
		t.Fatalf("期望 %d 个结果，实际 %d", len(DefaultTasks), len(results))
	}
	// KeywordMatchRunner 应该 100% 成功（因为它把 ExpectedKeywords 当回答）
	for i, r := range results {
		if !r.Metrics.Success {
			t.Errorf("任务 %d (%s) 应成功但失败", i, r.TaskID)
		}
	}
}

func TestTaskTimeout(t *testing.T) {
	// 创建一个会超时的任务
	runner := &slowRunner{}
	suite := NewSuite(runner)
	suite.AddTask(Task{
		ID:          "timeout-test",
		Type:        TaskQuestion,
		Difficulty:  DiffEasy,
		Input:       "test",
		Timeout:     1, // 1 秒超时
		ExpectedKeywords: []string{"test"},
	})
	results := suite.Run(context.Background())
	if len(results) != 1 {
		t.Fatalf("期望 1 个结果，实际 %d", len(results))
	}
	if results[0].Metrics.Success {
		t.Error("超时任务不应成功")
	}
	if results[0].Metrics.Error == "" {
		t.Error("超时任务应有错误信息")
	}
}

// slowRunner 永远不返回（用于测试超时）
type slowRunner struct{}

func (slowRunner) Name() string { return "slow-runner" }
func (slowRunner) Run(ctx context.Context, task Task) (Metrics, error) {
	select {
	case <-time.After(10 * time.Second):
		return Metrics{Success: true}, nil
	case <-ctx.Done():
		return Metrics{Error: ctx.Err().Error()}, ctx.Err()
	}
}

func TestDefaultTasksCoverage(t *testing.T) {
	// 验证默认任务集覆盖 5 类 × 3 难度
	byType := make(map[TaskType]int)
	byDiff := make(map[Difficulty]int)
	for _, t := range DefaultTasks {
		byType[t.Type]++
		byDiff[t.Difficulty]++
	}
	if len(byType) != 5 {
		t.Errorf("期望 5 种任务类型，实际 %d", len(byType))
	}
	if len(byDiff) != 3 {
		t.Errorf("期望 3 种难度，实际 %d", len(byDiff))
	}
	for tp, count := range byType {
		if count != 3 {
			t.Errorf("类型 %s 期望 3 个任务，实际 %d", tp, count)
		}
	}
}
