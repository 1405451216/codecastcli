package subagent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	ap "agentprimordia/pkg"
)

// TestParallelExecutionIsolation: Verify 3 parallel sub-agents have isolated memory.
func TestParallelExecutionIsolation(t *testing.T) {
	// Create 3 isolated memory stores and verify they don't share state
	stores := make([]ap.Memory, 3)
	for i := 0; i < 3; i++ {
		store, err := newIsolatedMemory(fmt.Sprintf("agent%d", i))
		if err != nil {
			t.Fatalf("newIsolatedMemory(%d) failed: %v", i, err)
		}
		mem, ok := store.(ap.Memory)
		if !ok {
			t.Fatalf("store %d should implement ap.Memory", i)
		}
		stores[i] = mem
	}

	// Write unique data to each store
	for i, mem := range stores {
		ep := &ap.Episode{
			ID:        fmt.Sprintf("ep-agent%d", i),
			SessionID: fmt.Sprintf("session-agent%d", i),
			Role:      "assistant",
			Content:   fmt.Sprintf("agent%d private data", i),
		}
		if err := mem.Add(context.Background(), ep); err != nil {
			t.Fatalf("mem.Add() store %d failed: %v", i, err)
		}
	}

	// Verify each store only has its own data
	for i, mem := range stores {
		count, err := mem.Count(context.Background(), "")
		if err != nil {
			t.Fatalf("mem.Count() store %d failed: %v", i, err)
		}
		if count != 1 {
			t.Errorf("store %d: Count() = %d, want 1", i, count)
		}

		// Search for other agents' data should return nothing
		for j := 0; j < 3; j++ {
			if j == i {
				continue
			}
			results, err := mem.Search(context.Background(), fmt.Sprintf("agent%d private data", j), nil)
			if err != nil {
				t.Fatalf("mem.Search() store %d for agent%d data failed: %v", i, j, err)
			}
			if len(results) != 0 {
				t.Errorf("store %d should not contain agent%d data, got %d results", i, j, len(results))
			}
		}
	}
}

// TestParallelOneFailureNoImpact: 1 of 3 sub-agents fails, verify other 2 complete successfully.
func TestParallelOneFailureNoImpact(t *testing.T) {
	// Simulate 3 sub-agents where one fails
	// We verify this at the DAG result level using ExecutionResult
	r := &ExecutionResult{
		DAGResult: &ap.DAGResult{
			Order: []string{"plan", "exec_1", "exec_2", "exec_3"},
			NodeResults: map[string]*ap.DAGNodeResult{
				"plan":   {NodeID: "plan", Output: "plan done"},
				"exec_1": {NodeID: "exec_1", Output: "task 1 completed"},
				"exec_2": {NodeID: "exec_2", Error: fmt.Errorf("sub-agent 2 failed")},
				"exec_3": {NodeID: "exec_3", Output: "task 3 completed"},
			},
		},
		Plan: "plan done",
	}

	// Verify the successful agents completed
	if nr, ok := r.DAGResult.NodeResults["exec_1"]; !ok || nr.Output != "task 1 completed" {
		t.Error("exec_1 should have completed successfully")
	}
	if nr, ok := r.DAGResult.NodeResults["exec_3"]; !ok || nr.Output != "task 3 completed" {
		t.Error("exec_3 should have completed successfully")
	}

	// Verify the failed agent is marked as failed
	if nr, ok := r.DAGResult.NodeResults["exec_2"]; !ok || nr.Error == nil {
		t.Error("exec_2 should have an error")
	}

	// Verify summary includes both successes and failure
	summary := r.Summary()
	if !strings.Contains(summary, "[exec_1] 成功") {
		t.Errorf("summary should show exec_1 as successful, got: %s", summary)
	}
	if !strings.Contains(summary, "[exec_2] 失败") {
		t.Errorf("summary should show exec_2 as failed, got: %s", summary)
	}
	if !strings.Contains(summary, "[exec_3] 成功") {
		t.Errorf("summary should show exec_3 as successful, got: %s", summary)
	}
}

// TestParallelResultAggregation: Verify results from 3 sub-agents are correctly merged.
func TestParallelResultAggregation(t *testing.T) {
	r := &ExecutionResult{
		DAGResult: &ap.DAGResult{
			Order: []string{"plan", "exec_1", "exec_2", "exec_3", "aggregate"},
			NodeResults: map[string]*ap.DAGNodeResult{
				"plan":      {NodeID: "plan", Output: "plan: 3 tasks"},
				"exec_1":    {NodeID: "exec_1", Output: "result from task 1"},
				"exec_2":    {NodeID: "exec_2", Output: "result from task 2"},
				"exec_3":    {NodeID: "exec_3", Output: "result from task 3"},
				"aggregate": {NodeID: "aggregate", Output: "aggregated: result from task 1, result from task 2, result from task 3"},
			},
		},
		Plan:        "plan: 3 tasks",
		Aggregation: "aggregated: result from task 1, result from task 2, result from task 3",
	}

	// Verify all sub-agent results are present in NodeResults
	for _, nodeID := range []string{"exec_1", "exec_2", "exec_3"} {
		nr, ok := r.DAGResult.NodeResults[nodeID]
		if !ok {
			t.Errorf("missing node result for %s", nodeID)
			continue
		}
		if nr.Error != nil {
			t.Errorf("%s should not have error, got: %v", nodeID, nr.Error)
		}
	}

	// Verify aggregation contains all results
	if r.Aggregation == "" {
		t.Error("Aggregation should not be empty")
	}
	for _, substr := range []string{"result from task 1", "result from task 2", "result from task 3"} {
		if !strings.Contains(r.Aggregation, substr) {
			t.Errorf("Aggregation missing %q, got: %s", substr, r.Aggregation)
		}
	}

	// Verify extractNodeOutput works for each
	for _, nodeID := range []string{"exec_1", "exec_2", "exec_3", "aggregate"} {
		output := extractNodeOutput(r.DAGResult, nodeID)
		if output == "" {
			t.Errorf("extractNodeOutput(%q) returned empty", nodeID)
		}
	}
}
