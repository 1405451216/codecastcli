package pool

import (
	"testing"
	"time"
)

func TestTaskDefinition_Fields(t *testing.T) {
	td := TaskDefinition{
		ID:         "task-1",
		Title:      "Read main.go",
		Prompt:     "Read the file main.go and summarize it",
		SessionID:  "sess-abc",
		FilesScope: []string{"main.go", "utils.go"},
		MaxTurns:   10,
		Metadata:   map[string]string{"priority": "high"},
	}

	if td.ID != "task-1" {
		t.Errorf("ID = %q, want %q", td.ID, "task-1")
	}
	if td.Title != "Read main.go" {
		t.Errorf("Title = %q, want %q", td.Title, "Read main.go")
	}
	if td.Prompt != "Read the file main.go and summarize it" {
		t.Errorf("Prompt = %q, want %q", td.Prompt, "Read the file main.go and summarize it")
	}
	if td.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", td.SessionID, "sess-abc")
	}
	if len(td.FilesScope) != 2 {
		t.Errorf("FilesScope len = %d, want 2", len(td.FilesScope))
	}
	if td.FilesScope[0] != "main.go" || td.FilesScope[1] != "utils.go" {
		t.Errorf("FilesScope = %v, want [main.go utils.go]", td.FilesScope)
	}
	if td.MaxTurns != 10 {
		t.Errorf("MaxTurns = %d, want 10", td.MaxTurns)
	}
	if td.Metadata["priority"] != "high" {
		t.Errorf("Metadata[priority] = %q, want %q", td.Metadata["priority"], "high")
	}

	t.Run("zero value", func(t *testing.T) {
		var zero TaskDefinition
		if zero.ID != "" {
			t.Errorf("zero ID = %q, want empty", zero.ID)
		}
		if zero.MaxTurns != 0 {
			t.Errorf("zero MaxTurns = %d, want 0", zero.MaxTurns)
		}
		if zero.FilesScope != nil {
			t.Errorf("zero FilesScope = %v, want nil", zero.FilesScope)
		}
		if zero.Metadata != nil {
			t.Errorf("zero Metadata = %v, want nil", zero.Metadata)
		}
	})
}

func TestTaskExecutionResult_Fields(t *testing.T) {
	r := TaskExecutionResult{
		TaskID:   "task-1",
		Content:  "File summary here",
		Error:    "",
		Duration: 3 * time.Second,
		Status:   "completed",
	}

	if r.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want %q", r.TaskID, "task-1")
	}
	if r.Content != "File summary here" {
		t.Errorf("Content = %q, want %q", r.Content, "File summary here")
	}
	if r.Error != "" {
		t.Errorf("Error = %q, want empty", r.Error)
	}
	if r.Duration != 3*time.Second {
		t.Errorf("Duration = %v, want %v", r.Duration, 3*time.Second)
	}
	if r.Status != "completed" {
		t.Errorf("Status = %q, want %q", r.Status, "completed")
	}

	t.Run("with error", func(t *testing.T) {
		r := TaskExecutionResult{
			TaskID:   "task-2",
			Error:    "timeout",
			Duration: 5 * time.Minute,
			Status:   "failed",
		}
		if r.Error != "timeout" {
			t.Errorf("Error = %q, want %q", r.Error, "timeout")
		}
		if r.Status != "failed" {
			t.Errorf("Status = %q, want %q", r.Status, "failed")
		}
	})

	t.Run("zero value", func(t *testing.T) {
		var zero TaskExecutionResult
		if zero.TaskID != "" {
			t.Errorf("zero TaskID = %q, want empty", zero.TaskID)
		}
		if zero.Duration != 0 {
			t.Errorf("zero Duration = %v, want 0", zero.Duration)
		}
		if zero.Status != "" {
			t.Errorf("zero Status = %q, want empty", zero.Status)
		}
	})
}
