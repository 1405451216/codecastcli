package diff

import (
	"strings"
	"testing"
)

func TestChangeTypeName(t *testing.T) {
	tests := []struct {
		ct       ChangeType
		expected string
	}{
		{ChangeAdd, "新增"},
		{ChangeDelete, "删除"},
		{ChangeModify, "修改"},
		{ChangeType(99), "未知"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ChangeTypeName(tt.ct)
			if result != tt.expected {
				t.Errorf("ChangeTypeName(%d) = %q, want %q", tt.ct, result, tt.expected)
			}
		})
	}
}

func TestPreviewer_PreviewEdit(t *testing.T) {
	p := NewPreviewer()
	change := p.PreviewEdit("main.go", "old line\n", "new line\n")

	if change.Path != "main.go" {
		t.Errorf("Path = %q, want %q", change.Path, "main.go")
	}
	if change.Type != ChangeModify {
		t.Errorf("Type = %d, want %d", change.Type, ChangeModify)
	}
	if change.OldContent != "old line\n" {
		t.Errorf("OldContent = %q, want %q", change.OldContent, "old line\n")
	}
	if change.NewContent != "new line\n" {
		t.Errorf("NewContent = %q, want %q", change.NewContent, "new line\n")
	}
	if !strings.Contains(change.Diff, "-old line") {
		t.Errorf("Diff 应包含 '-old line', 实际: %q", change.Diff)
	}
	if !strings.Contains(change.Diff, "+new line") {
		t.Errorf("Diff 应包含 '+new line', 实际: %q", change.Diff)
	}
}

func TestPreviewer_PreviewWrite_NewFile(t *testing.T) {
	p := NewPreviewer()
	change := p.PreviewWrite("newfile.go", "package main", false)

	if change.Path != "newfile.go" {
		t.Errorf("Path = %q, want %q", change.Path, "newfile.go")
	}
	if change.Type != ChangeAdd {
		t.Errorf("Type = %d, want %d (ChangeAdd)", change.Type, ChangeAdd)
	}
	if change.NewContent != "package main" {
		t.Errorf("NewContent = %q, want %q", change.NewContent, "package main")
	}
	if !strings.Contains(change.Diff, "/dev/null") {
		t.Errorf("新文件 Diff 应包含 /dev/null, 实际: %q", change.Diff)
	}
}

func TestPreviewer_PreviewWrite_ExistingFile(t *testing.T) {
	p := NewPreviewer()
	change := p.PreviewWrite("main.go", "package main", true)

	if change.Path != "main.go" {
		t.Errorf("Path = %q, want %q", change.Path, "main.go")
	}
	if change.Type != ChangeModify {
		t.Errorf("Type = %d, want %d (ChangeModify)", change.Type, ChangeModify)
	}
	if change.NewContent != "package main" {
		t.Errorf("NewContent = %q, want %q", change.NewContent, "package main")
	}
	if !strings.Contains(change.Diff, "修改") {
		t.Errorf("已存在文件 Diff 应包含 '修改', 实际: %q", change.Diff)
	}
}

func TestPreviewer_PreviewDelete(t *testing.T) {
	p := NewPreviewer()
	change := p.PreviewDelete("main.go", "package main")

	if change.Path != "main.go" {
		t.Errorf("Path = %q, want %q", change.Path, "main.go")
	}
	if change.Type != ChangeDelete {
		t.Errorf("Type = %d, want %d (ChangeDelete)", change.Type, ChangeDelete)
	}
	if change.OldContent != "package main" {
		t.Errorf("OldContent = %q, want %q", change.OldContent, "package main")
	}
	if !strings.Contains(change.Diff, "/dev/null") {
		t.Errorf("删除文件 Diff 应包含 /dev/null, 实际: %q", change.Diff)
	}
	if !strings.Contains(change.Diff, "文件已删除") {
		t.Errorf("删除文件 Diff 应包含 '文件已删除', 实际: %q", change.Diff)
	}
}

func TestFormatChange(t *testing.T) {
	tests := []struct {
		name     string
		change   *FileChange
		contains []string
	}{
		{
			name: "新增文件",
			change: &FileChange{
				Path: "new.go",
				Type: ChangeAdd,
				Diff: "--- /dev/null\n+++ new.go\n+content",
			},
			contains: []string{"new.go", "新增", "--- /dev/null"},
		},
		{
			name: "删除文件",
			change: &FileChange{
				Path: "old.go",
				Type: ChangeDelete,
				Diff: "--- old.go\n+++ /dev/null",
			},
			contains: []string{"old.go", "删除", "--- old.go"},
		},
		{
			name: "修改文件",
			change: &FileChange{
				Path: "main.go",
				Type: ChangeModify,
				Diff: "--- main.go\n+++ main.go\n-old\n+new",
			},
			contains: []string{"main.go", "修改", "-old"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatChange(tt.change)
			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("FormatChange 结果应包含 %q, 实际: %q", substr, result)
				}
			}
		})
	}
}
