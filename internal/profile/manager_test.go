package profile

import (
	"testing"
)

func TestManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	p := Profile{
		Name:     "test-profile",
		Model:    "gpt-4o",
		Provider: "openai",
		APIKey:   "sk-test-key",
	}

	// 保存
	err := m.Save(p)
	if err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}

	// 加载
	loaded, err := m.Load("test-profile")
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}

	if loaded.Name != p.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, p.Name)
	}
	if loaded.Model != p.Model {
		t.Errorf("Model = %q, want %q", loaded.Model, p.Model)
	}
	if loaded.Provider != p.Provider {
		t.Errorf("Provider = %q, want %q", loaded.Provider, p.Provider)
	}
	if loaded.APIKey != p.APIKey {
		t.Errorf("APIKey = %q, want %q", loaded.APIKey, p.APIKey)
	}
}

func TestManager_Load_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	_, err := m.Load("nonexistent")
	if err == nil {
		t.Error("Load(不存在的 profile) 应返回错误")
	}
}

func TestManager_List(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	profiles := []Profile{
		{Name: "profile-a", Model: "gpt-4o", Provider: "openai"},
		{Name: "profile-b", Model: "claude-sonnet-4-20250514", Provider: "anthropic"},
	}

	for _, p := range profiles {
		if err := m.Save(p); err != nil {
			t.Fatalf("Save() 失败: %v", err)
		}
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("List() 失败: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("List() 返回 %d 个 profile, want 2", len(list))
	}

	names := make(map[string]bool)
	for _, p := range list {
		names[p.Name] = true
	}
	if !names["profile-a"] {
		t.Error("List() 缺少 profile-a")
	}
	if !names["profile-b"] {
		t.Error("List() 缺少 profile-b")
	}
}

func TestManager_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	p := Profile{
		Name:     "to-delete",
		Model:    "gpt-4o",
		Provider: "openai",
	}

	if err := m.Save(p); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}

	// 确认存在
	_, err := m.Load("to-delete")
	if err != nil {
		t.Fatalf("Load() 应成功: %v", err)
	}

	// 删除
	err = m.Delete("to-delete")
	if err != nil {
		t.Fatalf("Delete() 失败: %v", err)
	}

	// 确认已删除
	_, err = m.Load("to-delete")
	if err == nil {
		t.Error("删除后 Load() 应返回错误")
	}
}

func TestManager_SetActive(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// 默认 active
	if m.GetActive() != "default" {
		t.Errorf("GetActive() = %q, want %q", m.GetActive(), "default")
	}

	// 设置 active
	m.SetActive("my-profile")
	if m.GetActive() != "my-profile" {
		t.Errorf("SetActive 后 GetActive() = %q, want %q", m.GetActive(), "my-profile")
	}
}
