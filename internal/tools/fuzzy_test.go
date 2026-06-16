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

// --- fuzzyMatchLines 单元测试 ---

func TestFuzzyMatchLines_ExactMatch(t *testing.T) {
	original := "line1\nline2\nline3\n"
	old := "line2"
	r := fuzzyMatchLines(original, old)
	if r.Confidence != 1.0 {
		t.Errorf("精确匹配期望置信度 1.0，得到 %f", r.Confidence)
	}
	if r.StartLine != 2 {
		t.Errorf("期望起始行 2，得到 %d", r.StartLine)
	}
	if r.Matched != "line2" {
		t.Errorf("期望匹配 'line2'，得到 %q", r.Matched)
	}
}

func TestFuzzyMatchLines_SingleCharDiff(t *testing.T) {
	// old 与原文差一个字符，置信度应 >0.85 自动应用
	original := "func foo() {\n\treturn 42\n}\n"
	old := "func foo() {\n\treturn 43\n}" // 42 vs 43
	r := fuzzyMatchLines(original, old)
	if r.Confidence < FuzzyMatchThreshold {
		t.Errorf("单字符差异期望置信度 >= %.2f，得到 %.4f", FuzzyMatchThreshold, r.Confidence)
	}
	if r.StartLine != 1 {
		t.Errorf("期望起始行 1，得到 %d", r.StartLine)
	}
}

func TestFuzzyMatchLines_IndentationDrift(t *testing.T) {
	// LLM 输出 4 空格缩进，原文是 tab，应仍能高置信度匹配
	original := "func main() {\n\tfmt.Println(\"hi\")\n}\n"
	old := "func main() {\n    fmt.Println(\"hi\")\n}"
	r := fuzzyMatchLines(original, old)
	if r.Confidence < FuzzyMatchThreshold {
		t.Errorf("缩进漂移期望置信度 >= %.2f，得到 %.4f", FuzzyMatchThreshold, r.Confidence)
	}
}

func TestFuzzyMatchLines_NoMatch(t *testing.T) {
	original := "aaa\nbbb\nccc\n"
	old := "zzzzzzzzzzz"
	r := fuzzyMatchLines(original, old)
	if r.Confidence >= FuzzyMatchThreshold {
		t.Errorf("完全不相似的内容不应达到阈值，得到置信度 %.4f", r.Confidence)
	}
}

func TestFuzzyMatchLines_WindowTooLarge(t *testing.T) {
	// old 超过 FuzzyMaxWindow 行，应直接返回零值
	original := "a\n"
	old := strings.Repeat("x\n", FuzzyMaxWindow+1)
	r := fuzzyMatchLines(original, old)
	if r.Confidence != 0 {
		t.Errorf("超大窗口期望零值，得到置信度 %f", r.Confidence)
	}
}

func TestFuzzyMatchLines_OriginalShorterThanOld(t *testing.T) {
	original := "a\nb\n"
	old := "a\nb\nc\nd\n"
	r := fuzzyMatchLines(original, old)
	if r.Confidence != 0 {
		t.Errorf("原文比 old 短期望零值，得到置信度 %f", r.Confidence)
	}
}

func TestFuzzyMatchLines_MultiLineBestWindow(t *testing.T) {
	// 原文有多个相似窗口，应选置信度最高的
	original := "func a() {\n\treturn 1\n}\nfunc b() {\n\treturn 2\n}\n"
	old := "func b() {\n\treturn 2\n}"
	r := fuzzyMatchLines(original, old)
	if r.Confidence != 1.0 {
		t.Errorf("期望精确匹配（多窗口中选最佳），得到置信度 %f", r.Confidence)
	}
	if r.StartLine != 4 {
		t.Errorf("期望起始行 4，得到 %d", r.StartLine)
	}
}

func TestLevenshtein_Basic(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"Saturday", "Sunday", 3},
	}
	for _, c := range cases {
		got := levenshtein([]rune(c.a), []rune(c.b))
		if got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLineSimilarity_Unicode(t *testing.T) {
	// 中文行相似度计算不应截断
	a := "你好世界"
	b := "你好世界！"
	sim := lineSimilarity(a, b)
	if sim <= 0 || sim >= 1 {
		t.Errorf("中文相似度应在 (0,1)，得到 %f", sim)
	}
}

// --- 集成测试：通过 EditFileTool.Execute 验证 fuzzy 自动应用 ---

func TestEditFile_FuzzyAutoApply_SingleCharDiff(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")
	content := "func compute(x int) int {\n\treturn x * 2\n}\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	// old_string 故意把 * 2 写成 * 3（单字符差异），fuzzy 应自动应用
	args, _ := json.Marshal(editFileParams{
		FilePath:  tmpFile,
		OldString: "func compute(x int) int {\n\treturn x * 3\n}",
		NewString: "func compute(x int) int {\n\treturn x * 4\n}",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Errorf("期望 fuzzy 自动应用成功，但得到错误: %s", result.Content)
	}

	data, _ := os.ReadFile(tmpFile)
	got := string(data)
	expected := "func compute(x int) int {\n\treturn x * 4\n}\n"
	if got != expected {
		t.Errorf("文件内容不匹配\n期望: %q\n得到: %q", expected, got)
	}
}

func TestEditFile_FuzzyLowConfidence_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	// old_string 与原文差异较大，置信度应低于阈值
	args, _ := json.Marshal(editFileParams{
		FilePath:  tmpFile,
		OldString: "completely different content here",
		NewString: "replacement",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望低置信度时返回错误，但得到成功")
	}
	// 错误信息应包含"置信度"提示
	if !strings.Contains(result.Content, "置信度") && !strings.Contains(result.Content, "未找到") {
		t.Errorf("错误信息应包含置信度提示，得到: %s", result.Content)
	}
	// 文件不应被修改
	data, _ := os.ReadFile(tmpFile)
	if string(data) != content {
		t.Errorf("低置信度时文件不应被修改\n期望: %q\n得到: %q", content, string(data))
	}
}

func TestEditFile_FuzzyIndentationDrift(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")
	// 原文用 tab 缩进
	content := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}

	tool := NewEditFileTool()
	// old_string 用 4 空格代替 tab，fuzzy 应能匹配
	args, _ := json.Marshal(editFileParams{
		FilePath:  tmpFile,
		OldString: "func main() {\n    fmt.Println(\"hello\")\n}",
		NewString: "func main() {\n    fmt.Println(\"world\")\n}",
	})

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Errorf("期望 fuzzy 缩进漂移匹配成功，但得到错误: %s", result.Content)
	}

	data, _ := os.ReadFile(tmpFile)
	got := string(data)
	// 替换后应包含 "world"
	if !strings.Contains(got, "world") {
		t.Errorf("期望文件包含 'world'，得到: %q", got)
	}
}

// 验证 fuzzyResult 零值安全
func TestFuzzyResult_ZeroValue(t *testing.T) {
	var r fuzzyResult
	if r.Matched != "" || r.Confidence != 0 || r.StartLine != 0 {
		t.Errorf("零值 fuzzyResult 不正确: %+v", r)
	}
}

// 验证 _ 占位避免 unused import（ap 在集成测试中使用）
var _ = ap.NewToolResult
