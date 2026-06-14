package tui

import (
	"strings"
	"testing"
)

func TestDAGViewProgress(t *testing.T) {
	dv := NewDAGView("test")

	// Empty DAG: progress is 0
	if p := dv.Progress(); p != 0 {
		t.Errorf("empty DAG: Progress() = %f, want 0", p)
	}

	// Add nodes
	dv.AddNode("a", "Task A")
	dv.AddNode("b", "Task B")
	dv.AddChildNode("a", "a1", "Subtask A1")

	// All pending: progress is 0
	if p := dv.Progress(); p != 0 {
		t.Errorf("all pending: Progress() = %f, want 0", p)
	}

	// Complete one of three
	dv.UpdateNode("a", StatusCompleted, "done")
	if p := dv.Progress(); p != 1.0/3.0 {
		t.Errorf("1/3 completed: Progress() = %f, want %f", p, 1.0/3.0)
	}

	// Complete all
	dv.UpdateNode("b", StatusCompleted, "done")
	dv.UpdateNode("a1", StatusCompleted, "done")
	if p := dv.Progress(); p != 1.0 {
		t.Errorf("all completed: Progress() = %f, want 1.0", p)
	}
}

func TestDAGViewRender(t *testing.T) {
	dv := NewDAGView("Pipeline")
	dv.AddNode("build", "Build")
	dv.AddNode("test", "Test")
	dv.AddChildNode("build", "compile", "Compile")
	dv.UpdateNode("build", StatusCompleted, "success")
	dv.UpdateNode("compile", StatusRunning, "in progress")
	dv.UpdateNode("test", StatusPending, "")
	dv.SetTokenCount(2500)

	rendered := dv.Render(80)

	// Verify the output contains expected elements
	if !strings.Contains(rendered, "Pipeline") {
		t.Errorf("rendered output should contain title 'Pipeline', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Build") {
		t.Errorf("rendered output should contain node label 'Build', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Test") {
		t.Errorf("rendered output should contain node label 'Test', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Compile") {
		t.Errorf("rendered output should contain child label 'Compile', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Progress") {
		t.Errorf("rendered output should contain 'Progress', got:\n%s", rendered)
	}
	// Verify box drawing characters
	if !strings.Contains(rendered, "┌") || !strings.Contains(rendered, "└") {
		t.Errorf("rendered output should contain box drawing characters, got:\n%s", rendered)
	}
}

func TestDAGViewUpdateNode(t *testing.T) {
	dv := NewDAGView("test")
	dv.AddNode("n1", "Node 1")

	// Verify initial state
	node := dv.nodeIndex["n1"]
	if node.Status != StatusPending {
		t.Errorf("initial status: got %q, want %q", node.Status, StatusPending)
	}
	if node.Detail != "" {
		t.Errorf("initial detail: got %q, want empty", node.Detail)
	}

	// Update to running
	dv.UpdateNode("n1", StatusRunning, "working")
	if node.Status != StatusRunning {
		t.Errorf("after running: got %q, want %q", node.Status, StatusRunning)
	}
	if node.Detail != "working" {
		t.Errorf("after running detail: got %q, want %q", node.Detail, "working")
	}
	if node.StartTime.IsZero() {
		t.Errorf("StartTime should be set when status is Running")
	}

	// Update to completed
	dv.UpdateNode("n1", StatusCompleted, "done")
	if node.Status != StatusCompleted {
		t.Errorf("after completed: got %q, want %q", node.Status, StatusCompleted)
	}
	if node.EndTime.IsZero() {
		t.Errorf("EndTime should be set when status is Completed")
	}

	// Update non-existent node should not panic
	dv.UpdateNode("nonexistent", StatusFailed, "nope")
}

func TestDAGViewAddChildNode(t *testing.T) {
	dv := NewDAGView("test")
	dv.AddNode("parent", "Parent")
	dv.AddChildNode("parent", "child1", "Child 1")

	parent := dv.nodeIndex["parent"]
	if len(parent.Children) != 1 {
		t.Fatalf("parent should have 1 child, got %d", len(parent.Children))
	}
	if parent.Children[0].ID != "child1" {
		t.Errorf("child ID: got %q, want %q", parent.Children[0].ID, "child1")
	}

	// Child added to non-existent parent becomes a root
	dv.AddChildNode("missing", "orphan", "Orphan")
	if len(dv.roots) != 2 {
		t.Errorf("orphan should become root; roots count = %d, want 2", len(dv.roots))
	}
}
