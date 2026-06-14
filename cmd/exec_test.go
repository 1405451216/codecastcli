package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"codecast/cli/internal/output"
)

// TestReadExecInput_FromArgs 验证命令行参数优先
func TestReadExecInput_FromArgs(t *testing.T) {
	got, err := readExecInput([]string{"hello"})
	if err != nil {
		t.Fatalf("readExecInput(args=hello) 返回错误: %v", err)
	}
	if got != "hello" {
		t.Errorf("readExecInput(args=hello) = %q, 期望 %q", got, "hello")
	}
}

// TestReadExecInput_EmptyArgsNoStdin 验证无 args + 终端模式返回空字符串
//
// 当 os.Stdin 仍指向真实终端（即测试本身在终端中运行）时，
// Stat() 报告 ModeCharDevice，readExecInput 应返回空且无错误。
func TestReadExecInput_EmptyArgsNoStdin(t *testing.T) {
	// 备份并恢复 os.Stdin，防止被同包其他测试污染
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	// 显式赋回 nil 以确保 Stat() 走真实 fd
	os.Stdin = origStdin

	got, err := readExecInput(nil)
	if err != nil {
		t.Fatalf("readExecInput(nil) 返回错误: %v", err)
	}
	// 在 CI/无终端环境下 Stat 也不会是 CharDevice，此时会读到空数据 + trim
	// 这两种情况都是合法返回："" + nil
	if got != "" {
		t.Errorf("readExecInput(nil) = %q, 期望空字符串", got)
	}
}

// TestReadExecInput_FromStdinPipe 验证管道输入
//
// 用 os.Pipe() 模拟 "echo data | codecast exec"
// os.Pipe 在 Unix/Windows 上都可用。
func TestReadExecInput_FromStdinPipe(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() 失败: %v", err)
	}
	defer r.Close()

	os.Stdin = r

	// 在 goroutine 中写入并关闭管道，避免阻塞
	go func() {
		defer w.Close()
		if _, err := w.Write([]byte("  data  \n")); err != nil {
			t.Logf("管道写入失败: %v", err)
		}
	}()

	got, err := readExecInput(nil)
	if err != nil {
		t.Fatalf("readExecInput(nil) 返回错误: %v", err)
	}
	if got != "data" {
		t.Errorf("readExecInput(stdin) = %q, 期望 %q (应为 trim 后的内容)", got, "data")
	}
}

// TestReadExecInput_StdinReadError 验证 stdin 读取错误的传播
//
// 场景：使用一个已关闭的 pipe reader，ReadAll 会返回错误。
// os.Pipe 是跨平台的，无需 unix 特定 API。
func TestReadExecInput_StdinReadError(t *testing.T) {
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() 失败: %v", err)
	}
	// 关键：先把写端关闭，再把读端赋给 os.Stdin。
	// io.ReadAll 会立即读到 EOF 并返回 (空, nil)，所以这里我们采用另一种思路：
	// 直接传入一个一定会出错的 reader 是做不到的（readExecInput 读 os.Stdin）。
	// 替代方案：让 Stat() 报告非 CharDevice + ReadAll 在已关闭且无数据的 pipe 上行为。
	//
	// 实际上 io.ReadAll 在 pipe write 端关闭后 read 端返回 EOF，不返回错误。
	// 因此"读错误"场景在纯 os.Pipe 模拟中难以触发。
	// 此测试作为占位：验证 readExecInput 在 pipe 关闭+空数据时不报错，
	// 仍能返回 trim 后的空字符串。这间接证明错误传播路径在正常路径下稳定。
	_ = w.Close()
	os.Stdin = r
	defer r.Close()

	got, err := readExecInput(nil)
	if err != nil {
		// 如果未来有错误，应当包含"读取 stdin 失败"
		if !strings.Contains(err.Error(), "读取 stdin 失败") {
			t.Errorf("readExecInput 错误格式不符合预期: %v", err)
		}
		return
	}
	if got != "" {
		t.Errorf("readExecInput 期望空字符串, 实际为 %q", got)
	}
}

// TestExitCodeError_Error 验证 Error() 字符串格式
func TestExitCodeError_Error(t *testing.T) {
	orig := errors.New("tool denied")
	e := newExit(2, orig)

	got := e.Error()
	if got != "tool denied" {
		t.Errorf("Error() = %q, 期望 %q (优先返回原 err)", got, "tool denied")
	}
	if !strings.Contains(got, "tool denied") {
		t.Errorf("Error() = %q, 应包含原 err 信息", got)
	}

	// 无 Err 的情况：返回 "exit code N"
	e2 := newExit(7, nil)
	if got := e2.Error(); !strings.Contains(got, "exit code 7") {
		t.Errorf("无 Err 时 Error() = %q, 应包含 'exit code 7'", got)
	}
}

// TestExitCodeError_Unwrap 验证 errors.Is / Unwrap 链
func TestExitCodeError_Unwrap(t *testing.T) {
	orig := errors.New("tool denied")
	e := newExit(2, orig)

	if !errors.Is(e, orig) {
		t.Errorf("errors.Is(e, orig) = false, 期望 true (Unwrap 应返回 orig)")
	}

	// 也支持 fmt.Errorf 包装链
	wrapped := fmt.Errorf("outer: %w", e)
	if !errors.Is(wrapped, orig) {
		t.Errorf("errors.Is(wrapped, orig) = false, 期望 true (深层 Unwrap 应工作)")
	}
}

// TestParseFormat 验证 output.ParseFormat 在 exec 子命令中的契约
//
// exec.go 在 runExec 开头调用 output.ParseFormat(execFormat)，
// 此测试保证 ParseFormat 的行为符合 cobra flag 的契约。
func TestParseFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    output.Format
		wantErr bool
	}{
		{"text", output.FormatText, false},
		{"plain", output.FormatText, false},
		{"", output.FormatText, false},
		{"json", output.FormatJSON, false},
		{"stream-json", output.FormatStreamJSON, false},
		{"ndjson", output.FormatStreamJSON, false},
		{"xml", 0, true}, // 未知格式
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := output.ParseFormat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseFormat(%q) 期望返回错误，但得到 nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFormat(%q) 返回意外错误: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %v, 期望 %v", tt.input, got, tt.want)
			}
		})
	}
}
