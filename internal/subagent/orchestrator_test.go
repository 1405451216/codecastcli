package subagent

import (
	"fmt"
	"strings"
	"testing"

	ap "agentprimordia/pkg"
)

func TestExecutionResult_Summary(t *testing.T) {
	t.Run("nil DAGResult returns empty", func(t *testing.T) {
		r := &ExecutionResult{DAGResult: nil}
		got := r.Summary()
		if got != "" {
			t.Errorf("Summary() = %q, want empty string", got)
		}
	})

	t.Run("single successful node", func(t *testing.T) {
		r := &ExecutionResult{
			DAGResult: &ap.DAGResult{
				Order: []string{"plan"},
				NodeResults: map[string]*ap.DAGNodeResult{
					"plan": {NodeID: "plan", Output: "done"},
				},
			},
		}
		got := r.Summary()
		if !strings.Contains(got, "DAG 执行完成，共 1 个节点") {
			t.Errorf("Summary() missing header, got: %q", got)
		}
		if !strings.Contains(got, "[plan] 成功") {
			t.Errorf("Summary() missing plan node status, got: %q", got)
		}
	})

	t.Run("multiple nodes with failure", func(t *testing.T) {
		r := &ExecutionResult{
			DAGResult: &ap.DAGResult{
				Order: []string{"plan", "execute"},
				NodeResults: map[string]*ap.DAGNodeResult{
					"plan":    {NodeID: "plan", Output: "plan done"},
					"execute": {NodeID: "execute", Error: fmt.Errorf("boom")},
				},
			},
		}
		got := r.Summary()
		if !strings.Contains(got, "DAG 执行完成，共 2 个节点") {
			t.Errorf("Summary() missing header, got: %q", got)
		}
		if !strings.Contains(got, "[plan] 成功") {
			t.Errorf("Summary() missing plan success, got: %q", got)
		}
		if !strings.Contains(got, "[execute] 失败: boom") {
			t.Errorf("Summary() missing execute failure, got: %q", got)
		}
	})

	t.Run("node in Order but missing from NodeResults", func(t *testing.T) {
		r := &ExecutionResult{
			DAGResult: &ap.DAGResult{
				Order:       []string{"plan", "missing"},
				NodeResults: map[string]*ap.DAGNodeResult{
					"plan": {NodeID: "plan", Output: "ok"},
				},
			},
		}
		got := r.Summary()
		if !strings.Contains(got, "[plan] 成功") {
			t.Errorf("Summary() missing plan node, got: %q", got)
		}
		// "missing" node should not appear since it's not in NodeResults
		if strings.Contains(got, "[missing]") {
			t.Errorf("Summary() should not contain missing node, got: %q", got)
		}
	})
}

func TestPlanTask_Fields(t *testing.T) {
	task := PlanTask{
		ID:          "t1",
		Description: "read main.go",
		DependsOn:   []string{"t0"},
		Priority:    1,
	}

	if task.ID != "t1" {
		t.Errorf("ID = %q, want %q", task.ID, "t1")
	}
	if task.Description != "read main.go" {
		t.Errorf("Description = %q, want %q", task.Description, "read main.go")
	}
	if len(task.DependsOn) != 1 || task.DependsOn[0] != "t0" {
		t.Errorf("DependsOn = %v, want [t0]", task.DependsOn)
	}
	if task.Priority != 1 {
		t.Errorf("Priority = %d, want 1", task.Priority)
	}

	t.Run("empty DependsOn", func(t *testing.T) {
		task := PlanTask{
			ID:          "t2",
			Description: "standalone task",
			DependsOn:   nil,
			Priority:    0,
		}
		if task.DependsOn != nil {
			t.Errorf("DependsOn = %v, want nil", task.DependsOn)
		}
	})
}
