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

	t.Run("parallel execute with aggregation node", func(t *testing.T) {
		r := &ExecutionResult{
			DAGResult: &ap.DAGResult{
				Order: []string{"plan", "exec_1", "exec_2", "aggregate"},
				NodeResults: map[string]*ap.DAGNodeResult{
					"plan":      {NodeID: "plan", Output: "plan done"},
					"exec_1":    {NodeID: "exec_1", Output: "task 1 done"},
					"exec_2":    {NodeID: "exec_2", Output: "task 2 done"},
					"aggregate": {NodeID: "aggregate", Output: "all tasks completed"},
				},
			},
			Plan:        "plan done",
			Aggregation: "all tasks completed",
		}
		got := r.Summary()
		if !strings.Contains(got, "DAG 执行完成，共 4 个节点") {
			t.Errorf("Summary() missing header, got: %q", got)
		}
		if !strings.Contains(got, "[exec_1] 成功") {
			t.Errorf("Summary() missing exec_1 success, got: %q", got)
		}
		if !strings.Contains(got, "[exec_2] 成功") {
			t.Errorf("Summary() missing exec_2 success, got: %q", got)
		}
		if !strings.Contains(got, "[aggregate] 成功") {
			t.Errorf("Summary() missing aggregate success, got: %q", got)
		}
		if r.Aggregation != "all tasks completed" {
			t.Errorf("Aggregation = %q, want %q", r.Aggregation, "all tasks completed")
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

func TestParallelExecute_IsolatedMemory(t *testing.T) {
	t.Run("each sub-task gets independent in-memory store", func(t *testing.T) {
		// Verify that newIsolatedMemory creates distinct store instances
		// that do not share state with each other.
		store1, err := newIsolatedMemory("test1")
		if err != nil {
			t.Fatalf("newIsolatedMemory() store1 failed: %v", err)
		}
		store2, err := newIsolatedMemory("test2")
		if err != nil {
			t.Fatalf("newIsolatedMemory() store2 failed: %v", err)
		}

		// Cast to full Memory interface for Count/Search assertions
		mem1, ok1 := store1.(ap.Memory)
		mem2, ok2 := store2.(ap.Memory)
		if !ok1 || !ok2 {
			t.Fatal("isolated memory stores should implement ap.Memory")
		}

		// Write to store1, verify store2 is unaffected
		ep1 := &ap.Episode{
			ID:        "ep-1",
			SessionID: "session-1",
			Role:      "user",
			Content:   "task A context",
		}
		if err := mem1.Add(t.Context(), ep1); err != nil {
			t.Fatalf("mem1.Add() failed: %v", err)
		}

		// store2 should have no episodes
		count, err := mem2.Count(t.Context(), "")
		if err != nil {
			t.Fatalf("mem2.Count() failed: %v", err)
		}
		if count != 0 {
			t.Errorf("mem2.Count() = %d, want 0 (stores should be independent)", count)
		}

		// store1 should have 1 episode
		count, err = mem1.Count(t.Context(), "")
		if err != nil {
			t.Fatalf("mem1.Count() failed: %v", err)
		}
		if count != 1 {
			t.Errorf("mem1.Count() = %d, want 1", count)
		}
	})

	t.Run("agents with isolated memory do not cross-contaminate", func(t *testing.T) {
		// Create two agents with separate memory stores and verify
		// that writing to one agent's memory does not appear in the other.
		store1, err := newIsolatedMemory("agent1")
		if err != nil {
			t.Fatalf("newIsolatedMemory() store1 failed: %v", err)
		}
		store2, err := newIsolatedMemory("agent2")
		if err != nil {
			t.Fatalf("newIsolatedMemory() store2 failed: %v", err)
		}

		mem1, ok1 := store1.(ap.Memory)
		mem2, ok2 := store2.(ap.Memory)
		if !ok1 || !ok2 {
			t.Fatal("isolated memory stores should implement ap.Memory")
		}

		// Simulate agent1 writing context
		ep := &ap.Episode{
			ID:        "ep-agent1",
			SessionID: "session-agent1",
			Role:      "assistant",
			Content:   "agent1 private context data",
		}
		if err := mem1.Add(t.Context(), ep); err != nil {
			t.Fatalf("mem1.Add() failed: %v", err)
		}

		// Verify mem2 has no knowledge of agent1's context
		results, err := mem2.Search(t.Context(), "agent1 private", nil)
		if err != nil {
			t.Fatalf("mem2.Search() failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("mem2.Search() returned %d results, want 0 (no cross-contamination)", len(results))
		}
	})
}

func TestExecutionResult_Aggregation(t *testing.T) {
	t.Run("Aggregation field is populated from aggregate node", func(t *testing.T) {
		r := &ExecutionResult{
			DAGResult: &ap.DAGResult{
				Order: []string{"plan", "exec_1", "exec_2", "aggregate"},
				NodeResults: map[string]*ap.DAGNodeResult{
					"plan":      {NodeID: "plan", Output: "plan"},
					"exec_1":    {NodeID: "exec_1", Output: "result 1"},
					"exec_2":    {NodeID: "exec_2", Output: "result 2"},
					"aggregate": {NodeID: "aggregate", Output: "summary of all"},
				},
			},
			Plan:        "plan",
			Aggregation: "summary of all",
		}
		if r.Aggregation != "summary of all" {
			t.Errorf("Aggregation = %q, want %q", r.Aggregation, "summary of all")
		}
	})

	t.Run("Aggregation is empty when aggregate node is absent", func(t *testing.T) {
		r := &ExecutionResult{
			DAGResult: &ap.DAGResult{
				Order: []string{"plan", "execute"},
				NodeResults: map[string]*ap.DAGNodeResult{
					"plan":    {NodeID: "plan", Output: "plan"},
					"execute": {NodeID: "execute", Output: "exec"},
				},
			},
			Plan:      "plan",
			Execution: "exec",
		}
		if r.Aggregation != "" {
			t.Errorf("Aggregation = %q, want empty string", r.Aggregation)
		}
	})
}
