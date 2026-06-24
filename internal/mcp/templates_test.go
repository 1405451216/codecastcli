package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListTemplates(t *testing.T) {
	templates := ListTemplates()
	if len(templates) == 0 {
		t.Fatal("ListTemplates returned empty list")
	}
	// 验证每个模板都有必填字段
	for _, tmpl := range templates {
		if tmpl.Name == "" {
			t.Error("template has empty Name")
		}
		if tmpl.Command == "" {
			t.Errorf("template %q has empty Command", tmpl.Name)
		}
		if tmpl.Category == "" {
			t.Errorf("template %q has empty Category", tmpl.Name)
		}
	}
}

func TestListTemplatesByCategory(t *testing.T) {
	// 获取所有类别
	categories := GetCategories()
	if len(categories) == 0 {
		t.Fatal("GetCategories returned empty list")
	}

	for _, cat := range categories {
		templates := ListTemplatesByCategory(cat)
		if len(templates) == 0 {
			t.Errorf("ListTemplatesByCategory(%q) returned empty list", cat)
		}
		for _, tmpl := range templates {
			if tmpl.Category != cat {
				t.Errorf("template %q has category %q, expected %q", tmpl.Name, tmpl.Category, cat)
			}
		}
	}

	// 不存在的类别应返回空
	unknown := ListTemplatesByCategory("nonexistent-category-xyz")
	if len(unknown) != 0 {
		t.Errorf("expected empty list for unknown category, got %d items", len(unknown))
	}
}

func TestGetTemplate(t *testing.T) {
	// 查找存在的模板
	tmpl, ok := GetTemplate("filesystem")
	if !ok {
		t.Fatal("GetTemplate(\"filesystem\") returned false")
	}
	if tmpl.Name != "filesystem" {
		t.Errorf("expected Name=filesystem, got %q", tmpl.Name)
	}
	if tmpl.Command == "" {
		t.Error("filesystem template has empty Command")
	}

	// 查找不存在的模板
	_, ok = GetTemplate("nonexistent-template-xyz")
	if ok {
		t.Error("GetTemplate should return false for nonexistent template")
	}
}

func TestGetCategories(t *testing.T) {
	categories := GetCategories()
	if len(categories) == 0 {
		t.Fatal("GetCategories returned empty list")
	}

	// 验证无重复
	seen := make(map[string]bool)
	for _, cat := range categories {
		if seen[cat] {
			t.Errorf("duplicate category: %q", cat)
		}
		seen[cat] = true
	}

	// 验证 core 类别一定存在
	if !seen["core"] {
		t.Error("expected 'core' category to exist")
	}
}

func TestSaveTemplateConfig(t *testing.T) {
	dir := t.TempDir()

	templates := []ServerTemplate{
		{
			Name:        "test-server",
			Description: "A test server",
			Command:     "npx",
			Args:        []string{"-y", "test"},
			Category:    "test",
		},
	}

	err := SaveTemplateConfig(dir, templates)
	if err != nil {
		t.Fatalf("SaveTemplateConfig: %v", err)
	}

	// 验证文件存在
	configPath := filepath.Join(dir, ".codecast", "mcp.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// 验证 JSON 内容包含模板名
	if len(data) == 0 {
		t.Error("config file is empty")
	}
	content := string(data)
	if !containsStr(content, "test-server") {
		t.Error("config file does not contain template name")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
