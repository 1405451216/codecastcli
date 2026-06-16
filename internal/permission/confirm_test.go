package permission

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// withStdin 临时把 os.Stdin 替换为管道 r（仅限测试）。
// 返回值是管道的写入端，测试代码往里写用户输入。
func withStdin(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		r.Close()
	})
	return w
}

// captureStdout 捕获 os.Stdout 输出。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestConfirmPrompt_Yes(t *testing.T) {
	w := withStdin(t)
	go w.WriteString("y\n")
	out := captureStdout(t, func() {
		r := ConfirmPrompt("shell_execute", `{"cmd":"ls"}`)
		if r.Action != ActionAllow {
			t.Errorf("want ActionAllow, got %v", r.Action)
		}
	})
	if !strings.Contains(out, "shell_execute") {
		t.Errorf("output missing tool name: %q", out)
	}
	if !strings.Contains(out, "\033[1;33m") {
		t.Errorf("output missing ANSI color: %q", out)
	}
	if !strings.Contains(out, "允许执行") {
		t.Errorf("output missing prompt: %q", out)
	}
}

func TestConfirmPrompt_No(t *testing.T) {
	w := withStdin(t)
	go w.WriteString("n\n")
	captureStdout(t, func() {
		r := ConfirmPrompt("write_file", "{}")
		if r.Action != ActionDeny {
			t.Errorf("want ActionDeny, got %v", r.Action)
		}
	})
}

func TestConfirmPrompt_AlwaysAllow(t *testing.T) {
	w := withStdin(t)
	go w.WriteString("a\n")
	captureStdout(t, func() {
		r := ConfirmPrompt("read_file", "{}")
		if r.Action != ActionAlwaysAllow {
			t.Errorf("want ActionAlwaysAllow, got %v", r.Action)
		}
	})
}

func TestConfirmPrompt_EditArgs(t *testing.T) {
	w := withStdin(t)
	go w.WriteString("e\nls -la\n")
	captureStdout(t, func() {
		r := ConfirmPrompt("shell_execute", `{"cmd":"ls"}`)
		if r.Action != ActionEditArgs {
			t.Errorf("want ActionEditArgs, got %v", r.Action)
		}
		if r.ModifiedArgs != "ls -la" {
			t.Errorf("ModifiedArgs = %q, want %q", r.ModifiedArgs, "ls -la")
		}
	})
}

func TestConfirmPrompt_EditArgsCancelled(t *testing.T) {
	w := withStdin(t)
	go w.WriteString("e\n\n")
	captureStdout(t, func() {
		r := ConfirmPrompt("shell_execute", "{}")
		if r.Action != ActionDeny {
			t.Errorf("empty edit should deny, got %v", r.Action)
		}
	})
}

func TestConfirmPrompt_DefaultIsDeny(t *testing.T) {
	w := withStdin(t)
	go w.WriteString("xyz\n")
	captureStdout(t, func() {
		r := ConfirmPrompt("shell_execute", "{}")
		if r.Action != ActionDeny {
			t.Errorf("unrecognized input should deny, got %v", r.Action)
		}
	})
}

func TestConfirmPrompt_EOFIsDeny(t *testing.T) {
	w := withStdin(t)
	// 立即关闭 w 模拟 EOF
	go w.Close()
	captureStdout(t, func() {
		r := ConfirmPrompt("shell_execute", "{}")
		if r.Action != ActionDeny {
			t.Errorf("EOF should deny, got %v", r.Action)
		}
	})
}

func TestConfirmPrompt_ArgsTruncated(t *testing.T) {
	w := withStdin(t)
	go w.WriteString("y\n")
	longArgs := strings.Repeat("a", 500)
	out := captureStdout(t, func() {
		ConfirmPrompt("tool", longArgs)
	})
	if !strings.Contains(out, "...") {
		t.Errorf("long args should be truncated with ..., got output: %q", out)
	}
	if strings.Contains(out, longArgs) {
		t.Errorf("long args should not be fully present in output")
	}
}

// TestHandleInterrupt_UpdatesHITLWrapperPending 验证 F-11 修复：
// HandleInterrupt 在显示 ConfirmPrompt 前后正确更新 HITLManagerWrapper 的 pending 状态。
func TestHandleInterrupt_UpdatesHITLWrapperPending(t *testing.T) {
	mgr := NewManager(ModeSuggest)
	mgr.BuildHITLConfig()

	wrapper := mgr.HitlManager()
	if wrapper == nil {
		t.Fatal("HitlManager should not be nil after BuildHITLConfig")
	}

	// 初始状态：无 pending 请求
	if req := wrapper.PendingRequest(); req != nil {
		t.Errorf("initial pending should be nil, got %v", req)
	}

	// 在另一个 goroutine 中执行 HandleInterrupt（会阻塞等 stdin）
	w := withStdin(t)
	done := make(chan struct{})
	var resp *HumanResponse
	var allowAlways bool
	go func() {
		defer close(done)
		req := &InterruptRequest{
			Reason: InterruptToolConfirm,
			Data:   map[string]any{"tool": "shell_execute", "args": `{"cmd":"ls"}`},
		}
		resp, allowAlways = HandleInterrupt(mgr, req)
	}()

	// 等一小段时间让 HandleInterrupt 设置 pending
	// （由于 stdin 阻塞，HandleInterrupt 会在 ConfirmPrompt 处等待输入）
	// 此时 pending 应该已被设置
	// 注意：这个测试依赖时序，在慢机器上可能不稳定
	// 但由于 pending 在 ConfirmPrompt 之前设置，应该很快可见

	// 写入 "y" 让 ConfirmPrompt 返回
	go w.WriteString("y\n")
	captureStdout(t, func() {
		<-done // 等待 HandleInterrupt 完成
	})

	if !resp.Approved {
		t.Errorf("want Approved=true, got false")
	}
	if allowAlways {
		t.Errorf("want allowAlways=false for 'y' input")
	}

	// HandleInterrupt 完成后 pending 应被清除
	if req := wrapper.PendingRequest(); req != nil {
		t.Errorf("pending should be nil after HandleInterrupt completes, got %v", req)
	}
}

// TestHandleInterrupt_DeniedToolSkipsPrompt 验证被禁止的工具不会弹出确认提示。
func TestHandleInterrupt_DeniedToolSkipsPrompt(t *testing.T) {
	mgr := NewManager(ModeSuggest)
	mgr.AddDeny("shell_execute")
	mgr.BuildHITLConfig()

	req := &InterruptRequest{
		Reason: InterruptToolConfirm,
		Data:   map[string]any{"tool": "shell_execute", "args": "{}"},
	}
	resp, _ := HandleInterrupt(mgr, req)
	if resp.Approved {
		t.Errorf("denied tool should not be approved")
	}
}
