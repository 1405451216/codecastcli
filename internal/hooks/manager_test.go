package hooks

import (
	"os"
	"path/filepath"
	"testing"

	ap "agentprimordia/pkg"
)

func TestParseHookPoint(t *testing.T) {
	tests := []struct {
		input string
		want  ap.HookPoint
	}{
		{"before_tool", ap.HookBeforeTool},
		{"after_tool", ap.HookAfterTool},
		{"before_run", ap.HookBeforeRun},
		{"after_run", ap.HookAfterRun},
		{"before_turn", ap.HookBeforeTurn},
		{"after_turn", ap.HookAfterTurn},
		{"before_llm", ap.HookBeforeLLM},
		{"after_llm", ap.HookAfterLLM},
		{"on_error", ap.HookOnError},
		{"on_complete", ap.HookOnComplete},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseHookPoint(tt.input)
			if err != nil {
				t.Fatalf("parseHookPoint(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseHookPoint(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}

	t.Run("case insensitive", func(t *testing.T) {
		got, err := parseHookPoint("BEFORE_TOOL")
		if err != nil {
			t.Fatalf("parseHookPoint(%q) unexpected error: %v", "BEFORE_TOOL", err)
		}
		if got != ap.HookBeforeTool {
			t.Errorf("parseHookPoint(%q) = %v, want %v", "BEFORE_TOOL", got, ap.HookBeforeTool)
		}
	})
}

func TestParseHookPoint_Invalid(t *testing.T) {
	_, err := parseHookPoint("unknown_point")
	if err == nil {
		t.Errorf("parseHookPoint(%q) expected error, got nil", "unknown_point")
	}

	_, err = parseHookPoint("before_unknown")
	if err == nil {
		t.Errorf("parseHookPoint(%q) expected error, got nil", "before_unknown")
	}
}

func TestHookManager_Add(t *testing.T) {
	m := NewHookManager(t.TempDir())

	hook := HookConfig{
		Name:    "test-hook",
		Point:   "after_tool",
		Command: "echo hello",
		Enabled: true,
	}
	m.Add(hook)

	list := m.List()
	if len(list) != 1 {
		t.Fatalf("List() returned %d hooks, want 1", len(list))
	}
	if list[0].Name != "test-hook" {
		t.Errorf("hook Name = %q, want %q", list[0].Name, "test-hook")
	}
	if list[0].Command != "echo hello" {
		t.Errorf("hook Command = %q, want %q", list[0].Command, "echo hello")
	}
}

func TestHookManager_List(t *testing.T) {
	m := NewHookManager(t.TempDir())

	// Initially empty
	list := m.List()
	if len(list) != 0 {
		t.Fatalf("List() returned %d hooks, want 0", len(list))
	}

	// Add multiple hooks
	m.Add(HookConfig{Name: "hook1", Point: "before_tool", Command: "cmd1", Enabled: true})
	m.Add(HookConfig{Name: "hook2", Point: "after_tool", Command: "cmd2", Enabled: false})

	list = m.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d hooks, want 2", len(list))
	}

	// Verify returned list is a copy (modifying it doesn't affect internal state)
	list[0].Name = "modified"
	inner := m.List()
	if inner[0].Name != "hook1" {
		t.Errorf("List() should return a copy, internal state was modified")
	}
}

func TestHookManager_Remove(t *testing.T) {
	m := NewHookManager(t.TempDir())

	m.Add(HookConfig{Name: "hook1", Point: "before_tool", Command: "cmd1", Enabled: true})
	m.Add(HookConfig{Name: "hook2", Point: "after_tool", Command: "cmd2", Enabled: true})
	m.Add(HookConfig{Name: "hook3", Point: "on_error", Command: "cmd3", Enabled: true})

	// Remove middle hook
	m.Remove("hook2")

	list := m.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d hooks, want 2", len(list))
	}
	for _, h := range list {
		if h.Name == "hook2" {
			t.Error("hook2 should have been removed")
		}
	}

	// Remove non-existent hook (no-op)
	m.Remove("nonexistent")
	list = m.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d hooks after removing nonexistent, want 2", len(list))
	}
}

func TestInitHooksTemplate(t *testing.T) {
	hooksDir := t.TempDir()

	err := InitHooksTemplate(hooksDir)
	if err != nil {
		t.Fatalf("InitHooksTemplate() error: %v", err)
	}

	// Verify hooks.yaml was created
	configPath := filepath.Join(hooksDir, "hooks.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read hooks.yaml: %v", err)
	}
	if len(data) == 0 {
		t.Error("hooks.yaml is empty")
	}

	// Verify the template can be loaded back by HookManager
	m := NewHookManager(hooksDir)
	if err := m.Load(); err != nil {
		t.Fatalf("Load() error after InitHooksTemplate: %v", err)
	}
	list := m.List()
	if len(list) != 1 {
		t.Fatalf("List() returned %d hooks, want 1", len(list))
	}
	if list[0].Name != "log-tool-usage" {
		t.Errorf("template hook Name = %q, want %q", list[0].Name, "log-tool-usage")
	}
	if list[0].Enabled != false {
		t.Errorf("template hook Enabled = %v, want false", list[0].Enabled)
	}
}
