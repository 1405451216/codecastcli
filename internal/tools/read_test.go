package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
}

func intPtr(v int) *int { return &v }


func TestReadFile_FullContentWithLineNumbers(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "small.txt")
	writeTempFile(t, tmpFile, "alpha\nbeta\ngamma\n")

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{FilePath: tmpFile})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}

	// 验证行号与内容
	for _, want := range []string{"   1│ alpha", "   2│ beta", "   3│ gamma"} {
		if !strings.Contains(result.Content, want) {
			t.Errorf("输出缺少 %q\n实际:\n%s", want, result.Content)
		}
	}
}

func TestReadFile_StartAndEndRange(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "range.txt")
	writeTempFile(t, tmpFile, "L0\nL1\nL2\nL3\nL4\n")

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{
		FilePath:  tmpFile,
		StartLine: intPtr(1),
		EndLine:   intPtr(3),
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}

	// 1-based 行号：start_line=1 → 行 2，end_line=3 → 行 4
	mustHave := []string{"   2│ L1", "   3│ L2", "   4│ L3"}
	for _, want := range mustHave {
		if !strings.Contains(result.Content, want) {
			t.Errorf("范围读取缺少 %q\n实际:\n%s", want, result.Content)
		}
	}
	mustNotHave := []string{"   1│ L0", "   5│ L4"}
	for _, dont := range mustNotHave {
		if strings.Contains(result.Content, dont) {
			t.Errorf("范围读取包含范围外内容 %q\n实际:\n%s", dont, result.Content)
		}
	}
}

func TestReadFile_EndLineMinusOneMeansToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tail.txt")
	writeTempFile(t, tmpFile, "a\nb\nc\n")

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{
		FilePath:  tmpFile,
		StartLine: intPtr(1),
		EndLine:   intPtr(-1),
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}
	for _, want := range []string{"   2│ b", "   3│ c"} {
		if !strings.Contains(result.Content, want) {
			t.Errorf("end_line=-1 应读到末尾，缺少 %q\n实际:\n%s", want, result.Content)
		}
	}
}

func TestReadFile_LargeFileTruncationHint(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "big.txt")

	// 构造 600 行的文件
	var sb strings.Builder
	for i := 0; i < 600; i++ {
		sb.WriteString("line\n")
	}
	writeTempFile(t, tmpFile, sb.String())

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{FilePath: tmpFile})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}

	// 应包含 ⚠ 提示
	if !strings.Contains(result.Content, "⚠") {
		t.Errorf("大文件应输出截断提示，实际:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "600 行") {
		t.Errorf("截断提示应包含行数 600，实际:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "start_line") || !strings.Contains(result.Content, "end_line") {
		t.Errorf("截断提示应建议使用 start_line/end_line，实际:\n%s", result.Content)
	}
}

func TestReadFile_LargeFileNoTruncationWhenRangeSpecified(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "big2.txt")
	var sb strings.Builder
	for i := 0; i < 600; i++ {
		sb.WriteString("line\n")
	}
	writeTempFile(t, tmpFile, sb.String())

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{
		FilePath:  tmpFile,
		StartLine: intPtr(0),
		EndLine:   intPtr(5),
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功，但得到错误: %s", result.Content)
	}
	if strings.Contains(result.Content, "⚠") {
		t.Errorf("指定了 start_line/end_line 时不应出现截断提示，实际:\n%s", result.Content)
	}
}

func TestReadFile_BinaryFileDetected(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "binary.bin")
	// 写入含 null 字节的"二进制"内容
	if err := os.WriteFile(tmpFile, []byte{0x48, 0x65, 0x00, 0x6C, 0x6C, 0x6F}, 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{FilePath: tmpFile})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("二进制文件应返回错误，实际: %s", result.Content)
	}
	if !strings.Contains(result.Content, "二进制") {
		t.Errorf("错误信息应提及二进制，实际: %s", result.Content)
	}
}

func TestReadFile_NonExistentFile(t *testing.T) {
	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{FilePath: "/nonexistent/path/that/does/not/exist.txt"})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("不存在的文件应返回错误，实际: %s", result.Content)
	}
}

func TestReadFile_StartLineOutOfRange(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "short.txt")
	writeTempFile(t, tmpFile, "a\nb\nc\n")

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{
		FilePath:  tmpFile,
		StartLine: intPtr(100),
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("start_line 超出范围应返回错误，实际: %s", result.Content)
	}
	if !strings.Contains(result.Content, "start_line") {
		t.Errorf("错误信息应提及 start_line，实际: %s", result.Content)
	}
}

func TestReadFile_StartGreaterThanEnd(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "srt.txt")
	writeTempFile(t, tmpFile, "a\nb\nc\nd\ne\n")

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{
		FilePath:  tmpFile,
		StartLine: intPtr(3),
		EndLine:   intPtr(1),
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("start_line > end_line 应返回错误，实际: %s", result.Content)
	}
}

func TestReadFile_EncodingValidation(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "enc.txt")
	writeTempFile(t, tmpFile, "hello")

	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{
		FilePath: tmpFile,
		Encoding: "gbk",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("不支持的编码应返回错误，实际: %s", result.Content)
	}
}

func TestReadFile_EmptyFilePath(t *testing.T) {
	tool := NewReadFileTool()
	args, _ := json.Marshal(readFileParams{FilePath: ""})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("空 file_path 应返回错误，实际: %s", result.Content)
	}
}

func TestReadFile_InvalidJSON(t *testing.T) {
	tool := NewReadFileTool()
	result, err := tool.Execute(context.Background(), []byte(`{invalid`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Errorf("非法 JSON 应返回错误，实际: %s", result.Content)
	}
}
