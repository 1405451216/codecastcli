package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ap "agentprimordia/pkg"
)

// writeTestFileME 辅助函数：写入临时文件（multi_edit 专用，避免与其他测试文件冲突）
func writeTestFileME(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件 %s 失败: %v", path, err)
	}
}

// readTestFileME 辅助函数：读取文件内容
func readTestFileME(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取文件 %s 失败: %v", path, err)
	}
	return string(data)
}

func TestMultiEdit_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	// 3 个文件
	fileA := filepath.Join(tmpDir, "a.txt")
	fileB := filepath.Join(tmpDir, "b.txt")
	fileC := filepath.Join(tmpDir, "c.txt")

	writeTestFileME(t, fileA, "alpha\ngamma\n")
	writeTestFileME(t, fileB, "beta\ngamma\n")
	writeTestFileME(t, fileC, "gamma\ndelta\n")

	tool := NewMultiEditTool()
	args, _ := json.Marshal(multiEditParams{
		Edits: []editOperation{
			{FilePath: fileA, OldString: "alpha", NewString: "ALPHA"},
			{FilePath: fileB, OldString: "beta", NewString: "BETA"},
			// fileC 上做 2 个 edit
			{FilePath: fileC, OldString: "gamma", NewString: "GAMMA"},
			{FilePath: fileC, OldString: "delta", NewString: "DELTA"},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}

	if got := readTestFileME(t, fileA); got != "ALPHA\ngamma\n" {
		t.Errorf("fileA 不匹配: %q", got)
	}
	if got := readTestFileME(t, fileB); got != "BETA\ngamma\n" {
		t.Errorf("fileB 不匹配: %q", got)
	}
	if got := readTestFileME(t, fileC); got != "GAMMA\nDELTA\n" {
		t.Errorf("fileC 不匹配: %q", got)
	}
}

func TestMultiEdit_FiveFilesSimultaneously(t *testing.T) {
	tmpDir := t.TempDir()

	files := make([]string, 5)
	for i := 0; i < 5; i++ {
		files[i] = filepath.Join(tmpDir, "f"+string(rune('0'+i))+".txt")
		writeTestFileME(t, files[i], "old_"+string(rune('0'+i)))
	}

	edits := make([]editOperation, 5)
	for i := 0; i < 5; i++ {
		edits[i] = editOperation{
			FilePath:  files[i],
			OldString: "old_" + string(rune('0'+i)),
			NewString: "new_" + string(rune('0'+i)),
		}
	}

	tool := NewMultiEditTool()
	args, _ := json.Marshal(multiEditParams{Edits: edits})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}

	for i := 0; i < 5; i++ {
		got := readTestFileME(t, files[i])
		want := "new_" + string(rune('0'+i))
		if got != want {
			t.Errorf("file[%d] 不匹配: got %q, want %q", i, got, want)
		}
	}
}

func TestMultiEdit_PreflightFailure_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	fileA := filepath.Join(tmpDir, "a.txt")
	fileB := filepath.Join(tmpDir, "b.txt")

	writeTestFileME(t, fileA, "alpha\n")
	writeTestFileME(t, fileB, "beta\n")

	tool := NewMultiEditTool()
	args, _ := json.Marshal(multiEditParams{
		Edits: []editOperation{
			{FilePath: fileA, OldString: "alpha", NewString: "ALPHA"},
			{FilePath: fileB, OldString: "NOT_EXIST", NewString: "BETA"}, // 这一项会失败
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望错误结果（未找到匹配），但得到成功")
	}

	// 验证零文件被修改
	if got := readTestFileME(t, fileA); got != "alpha\n" {
		t.Errorf("fileA 不应被修改: %q", got)
	}
	if got := readTestFileME(t, fileB); got != "beta\n" {
		t.Errorf("fileB 不应被修改: %q", got)
	}
	if !strings.Contains(result.Content, "edit[1]") {
		t.Errorf("错误信息应包含 edit[1]，实际: %s", result.Content)
	}
}

func TestMultiEdit_PreflightFailure_MultipleMatches(t *testing.T) {
	tmpDir := t.TempDir()
	fileA := filepath.Join(tmpDir, "a.txt")
	writeTestFileME(t, fileA, "foo bar foo baz foo")

	tool := NewMultiEditTool()
	args, _ := json.Marshal(multiEditParams{
		Edits: []editOperation{
			{FilePath: fileA, OldString: "foo", NewString: "qux", ReplaceAll: false},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望错误结果（多处匹配），但得到成功")
	}
	if !strings.Contains(result.Content, "3 处匹配") {
		t.Errorf("错误信息应说明匹配数: %s", result.Content)
	}

	// 文件不应被修改
	if got := readTestFileME(t, fileA); got != "foo bar foo baz foo" {
		t.Errorf("文件不应被修改: %q", got)
	}
}

func TestMultiEdit_Atomicity_RollbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 3 个文件
	fileA := filepath.Join(tmpDir, "a.txt")
	fileB := filepath.Join(tmpDir, "b.txt")
	fileC := filepath.Join(tmpDir, "c.txt")

	origA := "alpha\n"
	origB := "beta\n"
	origC := "gamma\n"
	writeTestFileME(t, fileA, origA)
	writeTestFileME(t, fileB, origB)
	writeTestFileME(t, fileC, origC)

	tool := NewMultiEditTool()
	args, _ := json.Marshal(multiEditParams{
		Edits: []editOperation{
			{FilePath: fileA, OldString: "alpha", NewString: "ALPHA"},
			{FilePath: fileB, OldString: "beta", NewString: "BETA"},
			{FilePath: fileC, OldString: "DOES_NOT_EXIST", NewString: "GAMMA"}, // 第 3 个失败
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望错误结果（预检失败），但得到成功")
	}

	// 验证：所有 3 个文件内容恢复为原始
	if got := readTestFileME(t, fileA); got != origA {
		t.Errorf("fileA 应回滚到原始: got %q, want %q", got, origA)
	}
	if got := readTestFileME(t, fileB); got != origB {
		t.Errorf("fileB 应回滚到原始: got %q, want %q", got, origB)
	}
	if got := readTestFileME(t, fileC); got != origC {
		t.Errorf("fileC 应保持原样: got %q, want %q", got, origC)
	}

	// 验证：临时文件已清理
	for _, p := range []string{fileA, fileB, fileC} {
		if _, err := os.Stat(p + ".tmp"); err == nil {
			t.Errorf("临时文件 %s.tmp 应被清理", p)
		}
	}
}

func TestMultiEdit_EmptyEdits(t *testing.T) {
	tool := NewMultiEditTool()
	args, _ := json.Marshal(multiEditParams{Edits: []editOperation{}})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望错误结果（空数组），但得到成功")
	}
	if !strings.Contains(result.Content, "edits 数组不能为空") {
		t.Errorf("错误信息不正确: %s", result.Content)
	}
}

func TestMultiEdit_TolerantMatching_TrailingWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	fileA := filepath.Join(tmpDir, "a.txt")
	// 文件末尾无尾部空白
	writeTestFileME(t, fileA, "func Foo() {\n    body\n}")

	tool := NewMultiEditTool()
	// 用户传入的 old_string 末尾有额外空白，模拟 LLM 缩进漂移
	args, _ := json.Marshal(multiEditParams{
		Edits: []editOperation{
			{
				FilePath:  fileA,
				OldString: "func Foo() {    \n    body  \n}",
				NewString: "func Foo() {\n    body // updated\n}",
			},
		},
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望容差匹配成功，但得到错误: %s", result.Content)
	}

	want := "func Foo() {\n    body // updated\n}"
	if got := readTestFileME(t, fileA); got != want {
		t.Errorf("容差匹配后内容不正确: got %q, want %q", got, want)
	}
}

func TestMultiEdit_ToolInterface(t *testing.T) {
	tool := NewMultiEditTool()
	if tool.Name() != "multi_edit" {
		t.Errorf("Name() = %q, 期望 %q", tool.Name(), "multi_edit")
	}
	if tool.Description() == "" {
		t.Error("Description() 不应为空")
	}
	if tool.Parameters() == nil {
		t.Error("Parameters() 不应为 nil")
	}
}

func TestMultiEdit_ImplementsToolInterface(t *testing.T) {
	// 验证 MultiEditTool 实现了 ap.Tool 接口
	var _ ap.Tool = NewMultiEditTool()
}

func TestMultiEdit_InvalidArgs(t *testing.T) {
	tool := NewMultiEditTool()
	// 传入无效 JSON
	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望参数解析失败错误")
	}
	if !strings.Contains(result.Content, "参数解析失败") {
		t.Errorf("错误信息不正确: %s", result.Content)
	}
}
