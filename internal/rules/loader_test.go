package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyTemplateVariables(t *testing.T) {
	input := "project={{PROJECT_ROOT}} home={{HOME}} os={{OS}}"
	got := ApplyTemplateVariables(input, "/my/project", "/home/user", "linux")
	want := "project=/my/project home=/home/user os=linux"
	if got != want {
		t.Errorf("ApplyTemplateVariables() = %q, want %q", got, want)
	}
}

func TestApplyTemplateVariables_NoVariables(t *testing.T) {
	input := "no variables here"
	got := ApplyTemplateVariables(input, "/root", "/home", "windows")
	if got != input {
		t.Errorf("ApplyTemplateVariables() = %q, want %q (unchanged)", got, input)
	}
}

func TestApplyTemplateVariables_MultipleOccurrences(t *testing.T) {
	input := "{{PROJECT_ROOT}}/src and {{PROJECT_ROOT}}/test"
	got := ApplyTemplateVariables(input, "/proj", "/home", "darwin")
	want := "/proj/src and /proj/test"
	if got != want {
		t.Errorf("ApplyTemplateVariables() = %q, want %q", got, want)
	}
}

func TestGetRulesPath(t *testing.T) {
	got := GetRulesPath("/my/project")
	want := filepath.Join("/my/project", ".codecast", "rules.md")
	if got != want {
		t.Errorf("GetRulesPath() = %q, want %q", got, want)
	}
}

func TestLoader_Load_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader(tmpDir)
	// Override homeDir to tmpDir so it won't read real ~/.codecast/rules.md
	loader.homeDir = tmpDir

	rs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if rs.Global != "" {
		t.Errorf("Global = %q, want empty", rs.Global)
	}
	if rs.Project != "" {
		t.Errorf("Project = %q, want empty", rs.Project)
	}
	if rs.Local != "" {
		t.Errorf("Local = %q, want empty", rs.Local)
	}
	if rs.Merged != "" {
		t.Errorf("Merged = %q, want empty", rs.Merged)
	}
}

func TestLoader_Load_WithFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project rules file
	codecastDir := filepath.Join(tmpDir, ".codecast")
	if err := os.MkdirAll(codecastDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codecastDir, "rules.md"), []byte("project rules"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codecastDir, "rules.local.md"), []byte("local rules"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create global rules file
	globalDir := filepath.Join(tmpDir, "home", ".codecast")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "rules.md"), []byte("global rules"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	loader.homeDir = filepath.Join(tmpDir, "home")

	rs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if rs.Global != "global rules" {
		t.Errorf("Global = %q, want %q", rs.Global, "global rules")
	}
	if rs.Project != "project rules" {
		t.Errorf("Project = %q, want %q", rs.Project, "project rules")
	}
	if rs.Local != "local rules" {
		t.Errorf("Local = %q, want %q", rs.Local, "local rules")
	}
	if !strings.Contains(rs.Merged, "project rules") || !strings.Contains(rs.Merged, "global rules") || !strings.Contains(rs.Merged, "local rules") {
		t.Errorf("Merged = %q, want to contain all rule contents", rs.Merged)
	}
}

func TestInitProject(t *testing.T) {
	tmpDir := t.TempDir()

	err := InitProject(tmpDir)
	if err != nil {
		t.Fatalf("InitProject() returned error: %v", err)
	}

	// Verify .codecast directory exists
	codecastDir := filepath.Join(tmpDir, ".codecast")
	info, err := os.Stat(codecastDir)
	if err != nil {
		t.Fatalf("Stat .codecast: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".codecast is not a directory")
	}

	// Verify rules.md exists and has content
	rulesPath := filepath.Join(codecastDir, "rules.md")
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("ReadFile rules.md: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("rules.md is empty")
	}
	if !strings.Contains(string(data), "项目规则") {
		t.Errorf("rules.md does not contain expected template content")
	}

	// Verify .gitignore exists
	gitignorePath := filepath.Join(codecastDir, ".gitignore")
	gitignoreData, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("ReadFile .gitignore: %v", err)
	}
	if !strings.Contains(string(gitignoreData), "rules.local.md") {
		t.Errorf(".gitignore does not contain rules.local.md")
	}
}

func TestInitProject_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	// First init should succeed
	if err := InitProject(tmpDir); err != nil {
		t.Fatalf("First InitProject() returned error: %v", err)
	}

	// Second init should fail because rules.md already exists
	err := InitProject(tmpDir)
	if err == nil {
		t.Errorf("Second InitProject() returned nil, want error")
	}
	if !strings.Contains(err.Error(), "已存在") {
		t.Errorf("Error = %q, want to contain '已存在'", err.Error())
	}
}
