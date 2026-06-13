package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".go", "go"},
		{".py", "python"},
		{".js", "javascript"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".jsx", "javascript"},
		{".rs", "rust"},
		{".java", "java"},
		{".kt", "kotlin"},
		{".c", "c"},
		{".h", "c"},
		{".cpp", "cpp"},
		{".cc", "cpp"},
		{".cxx", "cpp"},
		{".hpp", "cpp"},
		{".cs", "csharp"},
		{".rb", "ruby"},
		{".php", "php"},
		{".swift", "swift"},
		{".scala", "scala"},
		{".r", "r"},
		{".R", "r"},
		{".sql", "sql"},
		{".sh", "shell"},
		{".bash", "shell"},
		{".ps1", "powershell"},
		{".yaml", "yaml"},
		{".yml", "yaml"},
		{".json", "json"},
		{".xml", "xml"},
		{".html", "html"},
		{".css", "css"},
		{".md", "markdown"},
		{".dockerfile", "dockerfile"},
		{".toml", "toml"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := detectLanguage(tt.ext)
			if result != tt.expected {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.ext, result, tt.expected)
			}
		})
	}
}

func TestDetectLanguage_Unknown(t *testing.T) {
	result := detectLanguage(".xyz")
	if result != "" {
		t.Errorf("detectLanguage(%q) = %q, want empty string", ".xyz", result)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatSize(tt.size)
			if result != tt.expected {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.size, result, tt.expected)
			}
		})
	}
}

func TestIndexer_Build(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建测试文件结构
	files := map[string]string{
		"main.go":          "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"hello\") }",
		"app.py":           "import os\ndef hello():\n    pass",
		"utils/helper.js":  "const fs = require('fs');\nmodule.exports = {}",
		"README.md":        "# Test Project",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if dir := filepath.Dir(fullPath); dir != tmpDir {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("创建目录失败: %v", err)
			}
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}
	}

	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build() 失败: %v", err)
	}

	index := idx.GetIndex()

	if index.TotalFiles != 4 {
		t.Errorf("TotalFiles = %d, want 4", index.TotalFiles)
	}

	if index.RootDir != tmpDir {
		t.Errorf("RootDir = %q, want %q", index.RootDir, tmpDir)
	}

	if index.IndexedAt.IsZero() {
		t.Error("IndexedAt 不应为零值")
	}

	// 验证文件条目
	if len(index.Files) != 4 {
		t.Errorf("len(Files) = %d, want 4", len(index.Files))
	}

	// 验证语言统计
	if index.Languages["go"] != 1 {
		t.Errorf("Languages[go] = %d, want 1", index.Languages["go"])
	}
	if index.Languages["python"] != 1 {
		t.Errorf("Languages[python] = %d, want 1", index.Languages["python"])
	}
	if index.Languages["javascript"] != 1 {
		t.Errorf("Languages[javascript] = %d, want 1", index.Languages["javascript"])
	}
	if index.Languages["markdown"] != 1 {
		t.Errorf("Languages[markdown] = %d, want 1", index.Languages["markdown"])
	}
}

func TestIndexer_SearchFiles(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"main.go":        "package main",
		"handler.go":     "package main",
		"app.py":         "import os",
		"utils/helper.go": "package utils",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if dir := filepath.Dir(fullPath); dir != tmpDir {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("创建目录失败: %v", err)
			}
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}
	}

	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build() 失败: %v", err)
	}

	// 搜索 .go 文件
	results := idx.SearchFiles(".go")
	if len(results) != 3 {
		t.Errorf("SearchFiles(.go) 返回 %d 个结果, want 3", len(results))
	}

	// 搜索 main
	results = idx.SearchFiles("main")
	if len(results) != 1 {
		t.Errorf("SearchFiles(main) 返回 %d 个结果, want 1", len(results))
	}

	// 搜索不存在的文件
	results = idx.SearchFiles("nonexistent")
	if len(results) != 0 {
		t.Errorf("SearchFiles(nonexistent) 返回 %d 个结果, want 0", len(results))
	}
}

func TestIndexer_SearchByLanguage(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"main.go":    "package main",
		"handler.go": "package main",
		"app.py":     "import os",
		"utils.js":   "const x = 1",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}
	}

	idx := NewIndexer(tmpDir)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build() 失败: %v", err)
	}

	// 搜索 Go 文件
	results := idx.SearchByLanguage("go")
	if len(results) != 2 {
		t.Errorf("SearchByLanguage(go) 返回 %d 个结果, want 2", len(results))
	}

	// 搜索 Python 文件
	results = idx.SearchByLanguage("python")
	if len(results) != 1 {
		t.Errorf("SearchByLanguage(python) 返回 %d 个结果, want 1", len(results))
	}

	// 搜索不存在的语言
	results = idx.SearchByLanguage("rust")
	if len(results) != 0 {
		t.Errorf("SearchByLanguage(rust) 返回 %d 个结果, want 0", len(results))
	}
}
