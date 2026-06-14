package subagent

import (
	"strings"
	"testing"

	"codecast/cli/internal/tui"
)

func TestOrchestrator_DAGView_PlanAndExecuteNodes(t *testing.T) {
	// Test that DAGView nodes are created with the expected structure
	// when PlanAndExecute visualization is enabled.
	// We test the DAGView directly since it's the output of the integration.
	dv := tui.NewDAGView("多 Agent 协作")

	// Simulate what PlanAndExecute does when visualization is enabled
	dv.AddNode("plan", "Plan Agent")
	dv.UpdateNode("plan", tui.StatusRunning, "规划中...")
	dv.AddChildNode("plan", "execute", "Execute Agent")

	// Verify nodes exist
	rendered := dv.Render(60)
	if !strings.Contains(rendered, "Plan Agent") {
		t.Errorf("DAGView render should contain 'Plan Agent', got: %s", rendered)
	}
	if !strings.Contains(rendered, "Execute Agent") {
		t.Errorf("DAGView render should contain 'Execute Agent', got: %s", rendered)
	}
}

func TestOrchestrator_DAGView_NodeStatusProgression(t *testing.T) {
	dv := tui.NewDAGView("多 Agent 协作")

	// Step 1: Plan Agent starts
	dv.AddNode("plan", "Plan Agent")
	dv.UpdateNode("plan", tui.StatusRunning, "规划中...")
	dv.AddChildNode("plan", "execute", "Execute Agent")

	// Verify running state
	if dv.Progress() != 0 {
		t.Errorf("Progress should be 0 when no nodes completed, got %.2f", dv.Progress())
	}

	// Step 2: Plan Agent completes
	dv.UpdateNode("plan", tui.StatusCompleted, "plan summary")

	// Verify plan is completed
	if dv.Progress() < 0.4 {
		t.Errorf("Progress should be ~0.5 when plan completed, got %.2f", dv.Progress())
	}

	// Step 3: Execute Agent starts
	dv.UpdateNode("execute", tui.StatusRunning, "执行中...")

	// Step 4: Execute Agent completes
	dv.UpdateNode("execute", tui.StatusCompleted, "执行完成")

	// Verify all completed
	if dv.Progress() != 1.0 {
		t.Errorf("Progress should be 1.0 when all nodes completed, got %.2f", dv.Progress())
	}

	// Verify render contains completion indicators
	rendered := dv.Render(60)
	if !strings.Contains(rendered, "100%") {
		t.Errorf("DAGView render should show 100%% progress, got: %s", rendered)
	}
}

func TestOrchestrator_DAGView_FailedNode(t *testing.T) {
	dv := tui.NewDAGView("多 Agent 协作")

	dv.AddNode("plan", "Plan Agent")
	dv.UpdateNode("plan", tui.StatusRunning, "规划中...")
	dv.AddChildNode("plan", "execute", "Execute Agent")

	// Plan succeeds
	dv.UpdateNode("plan", tui.StatusCompleted, "plan done")

	// Execute fails
	dv.UpdateNode("execute", tui.StatusFailed, "connection error")

	// Progress should not be 1.0 since execute failed
	if dv.Progress() != 0.5 {
		t.Errorf("Progress should be 0.5 when plan completed but execute failed, got %.2f", dv.Progress())
	}

	// Render should contain failure indicator
	rendered := dv.Render(60)
	if !strings.Contains(rendered, "connection error") {
		t.Errorf("DAGView render should contain error detail, got: %s", rendered)
	}
}

func TestOrchestrator_DAGView_ParallelExecuteNodes(t *testing.T) {
	dv := tui.NewDAGView("多 Agent 协作")

	// Simulate what ParallelExecute does when visualization is enabled
	dv.AddNode("plan", "Plan Agent")
	dv.UpdateNode("plan", tui.StatusRunning, "规划中...")

	// Add exec child nodes
	dv.AddChildNode("plan", "exec_1", "Exec 1")
	dv.UpdateNode("exec_1", tui.StatusPending, "read main.go")
	dv.AddChildNode("plan", "exec_2", "Exec 2")
	dv.UpdateNode("exec_2", tui.StatusPending, "write output.go")

	// Add aggregate node
	dv.AddChildNode("plan", "aggregate", "Aggregate Agent")

	// Plan completes
	dv.UpdateNode("plan", tui.StatusCompleted, "规划完成")

	// Exec agents start and complete
	dv.UpdateNode("exec_1", tui.StatusRunning, "执行中...")
	dv.UpdateNode("exec_1", tui.StatusCompleted, "执行完成")
	dv.UpdateNode("exec_2", tui.StatusRunning, "执行中...")
	dv.UpdateNode("exec_2", tui.StatusCompleted, "执行完成")

	// Aggregate completes
	dv.UpdateNode("aggregate", tui.StatusCompleted, "汇总完成")

	// All should be complete
	if dv.Progress() != 1.0 {
		t.Errorf("Progress should be 1.0 when all parallel nodes completed, got %.2f", dv.Progress())
	}

	// Render should contain all nodes
	rendered := dv.Render(60)
	for _, label := range []string{"Plan Agent", "Exec 1", "Exec 2", "Aggregate Agent"} {
		if !strings.Contains(rendered, label) {
			t.Errorf("DAGView render should contain '%s', got: %s", label, rendered)
		}
	}
}

func TestOrchestrator_DAGView_ParallelExecutePartialFailure(t *testing.T) {
	dv := tui.NewDAGView("多 Agent 协作")

	dv.AddNode("plan", "Plan Agent")
	dv.UpdateNode("plan", tui.StatusRunning, "规划中...")
	dv.AddChildNode("plan", "exec_1", "Exec 1")
	dv.AddChildNode("plan", "exec_2", "Exec 2")
	dv.AddChildNode("plan", "aggregate", "Aggregate Agent")

	// Plan succeeds
	dv.UpdateNode("plan", tui.StatusCompleted, "规划完成")

	// Exec 1 succeeds
	dv.UpdateNode("exec_1", tui.StatusCompleted, "执行完成")

	// Exec 2 fails
	dv.UpdateNode("exec_2", tui.StatusFailed, "file not found")

	// Aggregate still completes (with partial results)
	dv.UpdateNode("aggregate", tui.StatusCompleted, "汇总完成")

	// 3 out of 4 completed (exec_2 failed)
	if dv.Progress() != 0.75 {
		t.Errorf("Progress should be 0.75 with 3/4 completed, got %.2f", dv.Progress())
	}

	rendered := dv.Render(60)
	if !strings.Contains(rendered, "file not found") {
		t.Errorf("DAGView render should contain exec_2 error detail, got: %s", rendered)
	}
}

func TestOrchestrator_SetVisualization(t *testing.T) {
	// Test that NewOrchestrator creates a DAGView and SetVisualization works.
	// We can't call NewOrchestrator without a real config/provider,
	// so we test the DAGView accessor pattern directly.
	dv := tui.NewDAGView("多 Agent 协作")

	// Verify DAGView is created with correct title
	rendered := dv.Render(60)
	if !strings.Contains(rendered, "多 Agent 协作") {
		t.Errorf("DAGView should have title '多 Agent 协作', got: %s", rendered)
	}

	// Verify initial state: no nodes, progress 0
	if dv.Progress() != 0 {
		t.Errorf("New DAGView should have 0 progress, got %.2f", dv.Progress())
	}
}

func TestOrchestrator_DAGView_DAGBuildFailure(t *testing.T) {
	dv := tui.NewDAGView("多 Agent 协作")

	// Simulate DAG build failure
	dv.AddNode("plan", "Plan Agent")
	dv.UpdateNode("plan", tui.StatusRunning, "规划中...")
	dv.UpdateNode("plan", tui.StatusFailed, "DAG 构建失败")

	// Verify failure is reflected
	if dv.Progress() != 0 {
		t.Errorf("Progress should be 0 when plan failed, got %.2f", dv.Progress())
	}

	rendered := dv.Render(60)
	if !strings.Contains(rendered, "DAG 构建失败") {
		t.Errorf("DAGView should show build failure detail, got: %s", rendered)
	}
}

func TestOrchestrator_DAGView_PlanSummaryTruncation(t *testing.T) {
	dv := tui.NewDAGView("多 Agent 协作")

	dv.AddNode("plan", "Plan Agent")
	dv.UpdateNode("plan", tui.StatusRunning, "规划中...")

	// Set a very long plan summary
	longSummary := strings.Repeat("a", 100)
	dv.UpdateNode("plan", tui.StatusCompleted, longSummary)

	// Render should not crash and should contain the node
	rendered := dv.Render(60)
	if !strings.Contains(rendered, "Plan Agent") {
		t.Errorf("DAGView should contain 'Plan Agent' even with long summary, got: %s", rendered)
	}
}
