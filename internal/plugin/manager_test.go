package plugin

import (
	"testing"
)

func TestManager_ListPlugins(t *testing.T) {
	m := NewManager(nil, t.TempDir())

	list := m.ListPlugins()
	if list == nil {
		t.Error("ListPlugins() returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("ListPlugins() returned %d plugins, want 0", len(list))
	}
}

func TestManager_ListAvailable(t *testing.T) {
	t.Run("empty directory returns empty list", func(t *testing.T) {
		m := NewManager(nil, t.TempDir())

		list := m.ListAvailable()
		if len(list) != 0 {
			t.Errorf("ListAvailable() returned %d plugins, want 0", len(list))
		}
	})

	t.Run("nonexistent directory returns empty list", func(t *testing.T) {
		m := NewManager(nil, "/nonexistent/path/that/does/not/exist")

		list := m.ListAvailable()
		if len(list) != 0 {
			t.Errorf("ListAvailable() returned %d plugins, want 0 for nonexistent dir", len(list))
		}
	})
}

func TestPluginMeta_Fields(t *testing.T) {
	meta := PluginMeta{
		Name:        "my-plugin",
		Version:     "1.0.0",
		Description: "A test plugin",
		Author:      "test-author",
		Loaded:      true,
	}

	if meta.Name != "my-plugin" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-plugin")
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", meta.Version, "1.0.0")
	}
	if meta.Description != "A test plugin" {
		t.Errorf("Description = %q, want %q", meta.Description, "A test plugin")
	}
	if meta.Author != "test-author" {
		t.Errorf("Author = %q, want %q", meta.Author, "test-author")
	}
	if meta.Loaded != true {
		t.Errorf("Loaded = %v, want true", meta.Loaded)
	}

	t.Run("zero value", func(t *testing.T) {
		var zero PluginMeta
		if zero.Name != "" {
			t.Errorf("zero Name = %q, want empty", zero.Name)
		}
		if zero.Version != "" {
			t.Errorf("zero Version = %q, want empty", zero.Version)
		}
		if zero.Loaded != false {
			t.Errorf("zero Loaded = %v, want false", zero.Loaded)
		}
	})
}
