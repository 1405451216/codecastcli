// grep_search 工具的测试：覆盖 ripgrep 路径与 native 回退路径。
//
// 关键策略：
//  1. parseRipgrepJSON 直接用 io.Reader 测试 —— 跳过实际 ripgrep 二进制
//  2. native 路径通过 NewGrepSearchToolWithRG("") 强制开启
//  3. ripgrep 端到端用 NewGrepSearchToolWithRG("cat") + 自制 JSON Lines 输入测试
//     （cat 在 Windows + Linux/macOS 都存在，可作为最小 stub）
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestParseRipgrepJSON_Valid 测试标准 ripgrep JSON Lines 输出解析。
// 模拟一次小仓库搜索，验证 begin/match/end 三种事件都被正确处理。
func TestParseRipgrepJSON_Valid(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"begin","data":{"path":{"text":"foo.go"}}}`,
		`{"type":"match","data":{"path":{"text":"foo.go"},"lines":{"text":"func Hello()\n"},"line_number":10}}`,
		`{"type":"match","data":{"path":{"text":"foo.go"},"lines":{"text":"\t// MARKER\n"},"line_number":11}}`,
		`{"type":"end","data":{"path":{"text":"foo.go"},"stats":{"matches":2}}}`,
		`{"type":"summary","data":{"elapsed_total":{"human":"0.01s","nanos":10000000,"secs":0},"searches":1,"searches_with_match":1,"matches":2}}`,
	}, "\n")

	matches, err := parseRipgrepJSON(strings.NewReader(input), 50)
	if err != nil {
		t.Fatalf("parseRipgrepJSON 失败: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("期望 2 条匹配，得到 %d", len(matches))
	}
	if matches[0].File != "foo.go" {
		t.Errorf("期望 foo.go，得到 %s", matches[0].File)
	}
	if matches[0].Line != 10 {
		t.Errorf("期望 line=10，得到 %d", matches[0].Line)
	}
	if matches[0].Content != "func Hello()" {
		t.Errorf("期望 'func Hello()'，得到 %q", matches[0].Content)
	}
	// 第二条匹配应继承同文件的 begin
	if matches[1].File != "foo.go" || matches[1].Line != 11 {
		t.Errorf("第二条匹配丢失 begin 上下文: %+v", matches[1])
	}
	// 末尾换行被 TrimRight
	if strings.HasSuffix(matches[0].Content, "\n") {
		t.Error("尾部换行未被 TrimRight")
	}
}

// TestParseRipgrepJSON_MultipleFiles 测试跨文件匹配时 currentFile 切换正确。
func TestParseRipgrepJSON_MultipleFiles(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"begin","data":{"path":{"text":"a.go"}}}`,
		`{"type":"match","data":{"path":{"text":"a.go"},"lines":{"text":"a1\n"},"line_number":1}}`,
		`{"type":"end","data":{"path":{"text":"a.go"}}}`,
		`{"type":"begin","data":{"path":{"text":"b.go"}}}`,
		`{"type":"match","data":{"path":{"text":"b.go"},"lines":{"text":"b1\n"},"line_number":5}}`,
		`{"type":"end","data":{"path":{"text":"b.go"}}}`,
	}, "\n")

	matches, err := parseRipgrepJSON(strings.NewReader(input), 50)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("期望 2 条匹配，得到 %d", len(matches))
	}
	if matches[0].File != "a.go" || matches[0].Line != 1 {
		t.Errorf("第一条应为 a.go:1，得到 %+v", matches[0])
	}
	if matches[1].File != "b.go" || matches[1].Line != 5 {
		t.Errorf("第二条应为 b.go:5，得到 %+v", matches[1])
	}
}

// TestParseRipgrepJSON_MaxResultsTruncated 验证 maxResults 限制触发哨兵错误。
func TestParseRipgrepJSON_MaxResultsTruncated(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"match","data":{"path":{"text":"x.go"},"lines":{"text":"m1\n"},"line_number":1}}`,
		`{"type":"match","data":{"path":{"text":"x.go"},"lines":{"text":"m2\n"},"line_number":2}}`,
		`{"type":"match","data":{"path":{"text":"x.go"},"lines":{"text":"m3\n"},"line_number":3}}`,
	}, "\n")

	matches, err := parseRipgrepJSON(strings.NewReader(input), 2)
	if err != errMaxResultsReached {
		t.Errorf("期望 errMaxResultsReached，得到 %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("期望 2 条匹配（达到上限即停），得到 %d", len(matches))
	}
}

// TestParseRipgrepJSON_EmptyInput 验证空输入（无匹配）不报错。
func TestParseRipgrepJSON_EmptyInput(t *testing.T) {
	matches, err := parseRipgrepJSON(strings.NewReader(""), 50)
	if err != nil {
		t.Errorf("空输入应无错误，得到 %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("空输入应无匹配，得到 %d", len(matches))
	}
}

// TestParseRipgrepJSON_MalformedLines 验证单行 JSON 损坏时继续读后续行。
func TestParseRipgrepJSON_MalformedLines(t *testing.T) {
	input := strings.Join([]string{
		`{not valid json`,
		`{"type":"match","data":{"path":{"text":"x.go"},"lines":{"text":"good\n"},"line_number":1}}`,
		`{"type":"end","data":{"path":{"text":"x.go"}}}`,
	}, "\n")

	matches, err := parseRipgrepJSON(strings.NewReader(input), 50)
	if err != nil {
		t.Errorf("单行损坏应被忽略，得到 err=%v", err)
	}
	if len(matches) != 1 || matches[0].File != "x.go" {
		t.Errorf("期望 1 条匹配，得到 %+v", matches)
	}
}

// TestExecuteNative_HappyPath 走 native 路径（rgPath="" 强制跳过 ripgrep）。
func TestExecuteNative_HappyPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\nfunc Alpha() {}\nfunc Beta() {}\n")

	tool := NewGrepSearchToolWithRG("") // 强制 native
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern:    "func",
		Path:       root,
		MaxResults: 50,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Alpha") {
		t.Errorf("结果缺少 Alpha: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Beta") {
		t.Errorf("结果缺少 Beta: %s", result.Content)
	}
}

// TestExecuteNative_NoMatch 验证 native 路径在无匹配时返回标准消息。
func TestExecuteNative_NoMatch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")

	tool := NewGrepSearchToolWithRG("")
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "ZZZ_NONEXISTENT",
		Path:    root,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Errorf("无匹配不应返回错误: %s", result.Content)
	}
	if !strings.Contains(result.Content, "未找到") {
		t.Errorf("应包含 '未找到'，得到: %s", result.Content)
	}
}

// TestExecuteNative_FilePattern 验证 file_pattern 过滤。
func TestExecuteNative_FilePattern(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.go"), "package go // HIT\n")
	writeFile(t, filepath.Join(root, "py.py"), "package py // HIT\n")

	tool := NewGrepSearchToolWithRG("")
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern:     "HIT",
		Path:        root,
		FilePattern: "*.go",
		MaxResults:  50,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(result.Content, "py.py") {
		t.Errorf("file_pattern=*.go 不应包含 py.py: %s", result.Content)
	}
	if !strings.Contains(result.Content, "go.go") {
		t.Errorf("应包含 go.go: %s", result.Content)
	}
}

// TestExecuteNative_CaseInsensitive 验证 case_insensitive 标志。
func TestExecuteNative_CaseInsensitive(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "Hello World\n")

	tool := NewGrepSearchToolWithRG("")
	// 默认（不忽略大小写）应找不到 "hello"
	r1, _ := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "hello", Path: root, MaxResults: 50,
	}))
	if !strings.Contains(r1.Content, "未找到") {
		t.Errorf("默认应大小写敏感: %s", r1.Content)
	}
	// case_insensitive=true 应匹配到
	r2, _ := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "hello", Path: root, MaxResults: 50, CaseInsensitive: true,
	}))
	if strings.Contains(r2.Content, "未找到") {
		t.Errorf("case_insensitive 应匹配: %s", r2.Content)
	}
}

// TestExecute_InvalidArgs 验证参数解析失败的错误路径。
func TestExecute_InvalidArgs(t *testing.T) {
	tool := NewGrepSearchToolWithRG("")
	result, err := tool.Execute(context.Background(), json.RawMessage(`{not json}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("非法 JSON 应返回错误结果")
	}
}

// TestExecute_EmptyPattern 验证空 pattern 的错误路径。
func TestExecute_EmptyPattern(t *testing.T) {
	tool := NewGrepSearchToolWithRG("")
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "",
		Path:    ".",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("空 pattern 应返回错误")
	}
}

// TestExecute_DefaultMaxResults 验证 MaxResults 未传时默认为 50。
func TestExecute_DefaultMaxResults(t *testing.T) {
	root := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("MARKER\n")
	}
	writeFile(t, filepath.Join(root, "many.go"), sb.String())

	tool := NewGrepSearchToolWithRG("")
	// MaxResults=0 → 走默认值
	result, _ := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "MARKER",
		Path:    root,
	}))
	// 头部会写"找到 N 处匹配"。默认 50 + 截断尾标 → N 应 ≤ 50
	if !strings.Contains(result.Content, "已达到最大结果数") {
		t.Errorf("60 个匹配应触发截断尾标: %s", result.Content)
	}
	// 解析头部"找到 N 处匹配"
	var found int
	_, err := fmt.Sscanf(result.Content, "找到 %d 处匹配:", &found)
	if err != nil {
		t.Fatalf("无法解析匹配数: %v\n内容: %s", err, result.Content)
	}
	if found != 50 {
		t.Errorf("默认 max_results 应为 50，实际为 %d", found)
	}
}

// TestExecuteWithRipgrep_FakeBinary 用一个简单的脚本作为"假 ripgrep"，
// 验证 --json 参数被正确构造 + 输出被正确解析。
//
// 策略：在 tmpDir 写一个 .bat / .sh 脚本，输出预制的 JSON Lines。
func TestExecuteWithRipgrep_FakeBinary(t *testing.T) {
	root := t.TempDir()
	// 即使没有真文件也要保证 Path 存在
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}

	// 准备"假 ripgrep"输出
	fakeJSON := strings.Join([]string{
		`{"type":"begin","data":{"path":{"text":"fake.go"}}}`,
		`{"type":"match","data":{"path":{"text":"fake.go"},"lines":{"text":"FAKE_MATCH\n"},"line_number":3}}`,
		`{"type":"end","data":{"path":{"text":"fake.go"}}}`,
	}, "\n")

	// 写跨平台 fake 二进制：使用环境变量传递 JSON，输出到 stdout，退出码 0
	fakeBin := filepath.Join(t.TempDir(), "fakerg")
	if runtime.GOOS == "windows" {
		fakeBin += ".bat"
		script := "@echo off\r\necho " + strings.ReplaceAll(fakeJSON, "\n", "\necho ") + "\r\nexit /b 0\r\n"
		if err := os.WriteFile(fakeBin, []byte(script), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		script := "#!/bin/sh\nprintf '%b' '" + strings.ReplaceAll(fakeJSON, "\n", "\\n") + "'\nexit 0\n"
		if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}

	tool := NewGrepSearchToolWithRG(fakeBin)
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern:    "anything",
		Path:       root,
		MaxResults: 50,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望成功: %s", result.Content)
	}
	if !strings.Contains(result.Content, "FAKE_MATCH") {
		t.Errorf("未解析 fake ripgrep 输出: %s", result.Content)
	}
	if !strings.Contains(result.Content, "fake.go:3") {
		t.Errorf("应包含 fake.go:3: %s", result.Content)
	}
}

// TestExecuteWithRipgrep_ExitCode1_NoMatch 验证 ripgrep 退出码 1（无匹配）被视作正常。
func TestExecuteWithRipgrep_ExitCode1_NoMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(t.TempDir(), "fakerg")
	if runtime.GOOS == "windows" {
		fakeBin += ".bat"
		script := "@echo off\r\nexit /b 1\r\n"
		if err := os.WriteFile(fakeBin, []byte(script), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		script := "#!/bin/sh\nexit 1\n"
		if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}

	tool := NewGrepSearchToolWithRG(fakeBin)
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "anything",
		Path:    root,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Errorf("ripgrep 退出码 1 不应报错: %s", result.Content)
	}
	if !strings.Contains(result.Content, "未找到") {
		t.Errorf("应报告无匹配: %s", result.Content)
	}
}

// TestExecuteWithRipgrep_ExitCode2_FallsBack 验证 ripgrep 退出码 2（实际错误）触发 fallback。
func TestExecuteWithRipgrep_ExitCode2_FallsBack(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main // HIT\n")

	fakeBin := filepath.Join(t.TempDir(), "fakerg")
	if runtime.GOOS == "windows" {
		fakeBin += ".bat"
		// 写无效 JSON + 退出码 2
		script := "@echo off\r\necho {not valid\recho \r\nexit /b 2\r\n"
		if err := os.WriteFile(fakeBin, []byte(script), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		// 输出空行 + 退出码 2 → 解析器正常返回 0 matches，退出码 2 触发 fallback
		script := "#!/bin/sh\nexit 2\n"
		if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}

	tool := NewGrepSearchToolWithRG(fakeBin)
	// fake 失败 → 应自动回退到 native → native 找到 HIT
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "HIT",
		Path:    root,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Errorf("ripgrep 失败应回退到 native，不应报错: %s", result.Content)
	}
	if !strings.Contains(result.Content, "HIT") {
		t.Errorf("fallback 后应找到 HIT: %s", result.Content)
	}
}

// TestExecute_FallbackWhenBinaryMissing 验证二进制路径无效时自动 fallback。
func TestExecute_FallbackWhenBinaryMissing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main // HIT\n")

	// 指向不存在的二进制
	tool := NewGrepSearchToolWithRG(filepath.Join(t.TempDir(), "does-not-exist"))
	result, err := tool.Execute(context.Background(), mustJSON(t, grepSearchParams{
		Pattern: "HIT",
		Path:    root,
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Errorf("二进制不存在应 fallback，不应报错: %s", result.Content)
	}
	if !strings.Contains(result.Content, "HIT") {
		t.Errorf("fallback 应通过 native 找到 HIT: %s", result.Content)
	}
}
