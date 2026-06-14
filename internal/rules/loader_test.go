package rules

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestLoader_Load_SubModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .codecast/rules/ directory with sub-module files
	codecastDir := filepath.Join(tmpDir, ".codecast")
	rulesDir := filepath.Join(codecastDir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create sub-module rule files
	if err := os.WriteFile(filepath.Join(rulesDir, "backend.md"), []byte("backend rules"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "frontend.md"), []byte("frontend rules"), 0644); err != nil {
		t.Fatal(err)
	}
	// Also create a non-.md file that should be ignored
	if err := os.WriteFile(filepath.Join(rulesDir, "ignore.txt"), []byte("should be ignored"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	loader.homeDir = tmpDir // avoid reading real home dir

	rs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if len(rs.SubModules) != 2 {
		t.Fatalf("SubModules count = %d, want 2", len(rs.SubModules))
	}

	// Check alphabetical order: backend.md before frontend.md
	if rs.SubModules[0].Filename != "backend.md" {
		t.Errorf("SubModules[0].Filename = %q, want %q", rs.SubModules[0].Filename, "backend.md")
	}
	if rs.SubModules[0].Content != "backend rules" {
		t.Errorf("SubModules[0].Content = %q, want %q", rs.SubModules[0].Content, "backend rules")
	}
	if rs.SubModules[1].Filename != "frontend.md" {
		t.Errorf("SubModules[1].Filename = %q, want %q", rs.SubModules[1].Filename, "frontend.md")
	}
	if rs.SubModules[1].Content != "frontend rules" {
		t.Errorf("SubModules[1].Content = %q, want %q", rs.SubModules[1].Content, "frontend rules")
	}

	// Check merged output contains sub-module headers
	if !strings.Contains(rs.Merged, "=== 项目子模块规则: backend.md ===") {
		t.Errorf("Merged should contain sub-module header for backend.md")
	}
	if !strings.Contains(rs.Merged, "=== 项目子模块规则: frontend.md ===") {
		t.Errorf("Merged should contain sub-module header for frontend.md")
	}
	if strings.Contains(rs.Merged, "ignore.txt") {
		t.Errorf("Merged should NOT contain non-.md file")
	}
}

func TestLoader_Load_SubModules_NoDir(t *testing.T) {
	tmpDir := t.TempDir()

	loader := NewLoader(tmpDir)
	loader.homeDir = tmpDir

	rs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if len(rs.SubModules) != 0 {
		t.Errorf("SubModules count = %d, want 0 when rules/ dir doesn't exist", len(rs.SubModules))
	}
}

func TestLoader_Load_SubModules_AlphabeticalOrder(t *testing.T) {
	tmpDir := t.TempDir()

	rulesDir := filepath.Join(tmpDir, ".codecast", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create files in non-alphabetical order
	for _, name := range []string{"z-last.md", "a-first.md", "m-middle.md"} {
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte("content of "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	loader := NewLoader(tmpDir)
	loader.homeDir = tmpDir

	rs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if len(rs.SubModules) != 3 {
		t.Fatalf("SubModules count = %d, want 3", len(rs.SubModules))
	}

	// Verify alphabetical order
	expected := []string{"a-first.md", "m-middle.md", "z-last.md"}
	for i, exp := range expected {
		if rs.SubModules[i].Filename != exp {
			t.Errorf("SubModules[%d].Filename = %q, want %q", i, rs.SubModules[i].Filename, exp)
		}
	}

	// Verify merged output order: a-first appears before m-middle, which appears before z-last
	idxA := strings.Index(rs.Merged, "a-first.md")
	idxM := strings.Index(rs.Merged, "m-middle.md")
	idxZ := strings.Index(rs.Merged, "z-last.md")
	if idxA >= idxM || idxM >= idxZ {
		t.Errorf("Sub-modules not in alphabetical order in merged output: a=%d, m=%d, z=%d", idxA, idxM, idxZ)
	}
}

func TestLoader_Load_SubModules_SizeLimit(t *testing.T) {
	tmpDir := t.TempDir()

	rulesDir := filepath.Join(tmpDir, ".codecast", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	loader.homeDir = tmpDir
	// Set a small limit for testing
	loader.maxSubModuleSize = 100

	// Create files: alphabetical order is a-small.md then b-large.md
	smallContent := strings.Repeat("a", 80)
	if err := os.WriteFile(filepath.Join(rulesDir, "a-small.md"), []byte(smallContent), 0644); err != nil {
		t.Fatal(err)
	}

	largeContent := strings.Repeat("b", 50)
	if err := os.WriteFile(filepath.Join(rulesDir, "b-large.md"), []byte(largeContent), 0644); err != nil {
		t.Fatal(err)
	}

	rs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if len(rs.SubModules) != 2 {
		t.Fatalf("SubModules count = %d, want 2", len(rs.SubModules))
	}

	// a-small.md should be fully loaded (80 bytes)
	if len(rs.SubModules[0].Content) != 80 {
		t.Errorf("a-small.md content length = %d, want 80", len(rs.SubModules[0].Content))
	}

	// b-large.md should be truncated (100 - 80 = 20 bytes remaining)
	if len(rs.SubModules[1].Content) != 20 {
		t.Errorf("b-large.md content length = %d, want 20 (truncated)", len(rs.SubModules[1].Content))
	}

	// Original size should still be tracked
	if rs.SubModules[1].Size != 50 {
		t.Errorf("b-large.md original size = %d, want 50", rs.SubModules[1].Size)
	}
}

func TestApplyTemplateVariables_NewStyle(t *testing.T) {
	input := "project={{.ProjectName}} cwd={{.CWD}} date={{.Date}} os={{.OS}} arch={{.Arch}}"
	got := ApplyTemplateVariables(input, "/my/project", "/home/user", "linux")

	expectedName := filepath.Base("/my/project")
	if !strings.Contains(got, "project="+expectedName) {
		t.Errorf("Expected ProjectName to be replaced with %q, got %q", expectedName, got)
	}
	if !strings.Contains(got, "cwd=/my/project") {
		t.Errorf("Expected CWD to be replaced, got %q", got)
	}
	if !strings.Contains(got, "date="+time.Now().Format("2006-01-02")) {
		t.Errorf("Expected Date to be replaced with today's date, got %q", got)
	}
	if !strings.Contains(got, "os="+runtime.GOOS) {
		t.Errorf("Expected OS to be replaced with %s, got %q", runtime.GOOS, got)
	}
	if !strings.Contains(got, "arch="+runtime.GOARCH) {
		t.Errorf("Expected Arch to be replaced with %s, got %q", runtime.GOARCH, got)
	}
}

func TestApplyTemplateVariables_UnknownVariable(t *testing.T) {
	input := "unknown={{.UnknownVar}} known={{.OS}}"
	got := ApplyTemplateVariables(input, "/proj", "/home", "linux")

	// Unknown variables should be left as-is
	if !strings.Contains(got, "{{.UnknownVar}}") {
		t.Errorf("Unknown variable should be left as-is, got %q", got)
	}
	// {{.OS}} uses runtime.GOOS, not the goos parameter
	if !strings.Contains(got, "known="+runtime.GOOS) {
		t.Errorf("Known variable should be replaced with runtime.GOOS, got %q", got)
	}
}

func TestApplyTemplateVariables_MixedOldAndNew(t *testing.T) {
	input := "{{PROJECT_ROOT}} and {{.ProjectName}}"
	got := ApplyTemplateVariables(input, "/my/project", "/home", "linux")

	if !strings.Contains(got, "/my/project") {
		t.Errorf("Old-style variable should be replaced, got %q", got)
	}
	expectedName := filepath.Base("/my/project")
	if !strings.Contains(got, expectedName) {
		t.Errorf("New-style ProjectName should be replaced with basename %q, got %q", expectedName, got)
	}
}
