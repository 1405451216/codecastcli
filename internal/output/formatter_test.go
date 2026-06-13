package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// --- ParseFormat ---

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"text", FormatText},
		{"TEXT", FormatText},
		{"plain", FormatText},
		{"PLAIN", FormatText},
		{"", FormatText},
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"Json", FormatJSON},
		{"stream-json", FormatStreamJSON},
		{"STREAM-JSON", FormatStreamJSON},
		{"streamjson", FormatStreamJSON},
		{"STREAMJSON", FormatStreamJSON},
		{"ndjson", FormatStreamJSON},
		{"NDJSON", FormatStreamJSON},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if err != nil {
				t.Fatalf("ParseFormat(%q) 返回了意外错误: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %v, 期望 %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFormat_Invalid(t *testing.T) {
	_, err := ParseFormat("xml")
	if err == nil {
		t.Fatal("ParseFormat(\"xml\") 期望返回错误，但返回了 nil")
	}
	if !strings.Contains(err.Error(), "未知的输出格式") {
		t.Errorf("错误信息应包含 '未知的输出格式'，实际为: %v", err)
	}
}

// --- WriteResult ---

func TestFormatter_WriteResult_Text(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatText, &buf)

	// 带有 content 字段的 map
	result := map[string]any{"content": "hello world", "other": 123}
	if err := f.WriteResult(result); err != nil {
		t.Fatalf("WriteResult 返回错误: %v", err)
	}

	got := buf.String()
	if got != "hello world" {
		t.Errorf("WriteResult (text) 输出 = %q, 期望 %q", got, "hello world")
	}
}

func TestFormatter_WriteResult_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatJSON, &buf)

	result := map[string]any{"content": "hello", "count": 42}
	if err := f.WriteResult(result); err != nil {
		t.Fatalf("WriteResult 返回错误: %v", err)
	}

	got := buf.String()
	// 应该是合法 JSON 且包含 content 和 count 字段
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &parsed); err != nil {
		t.Fatalf("输出不是合法 JSON: %v, 内容: %q", err, got)
	}
	if parsed["content"] != "hello" {
		t.Errorf("JSON content = %v, 期望 \"hello\"", parsed["content"])
	}
	if parsed["count"] != float64(42) {
		t.Errorf("JSON count = %v, 期望 42", parsed["count"])
	}
}

// --- WriteError ---

func TestFormatter_WriteError_Text(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatText, &buf)

	if err := f.WriteError(errors.New("something failed")); err != nil {
		t.Fatalf("WriteError 返回错误: %v", err)
	}

	got := buf.String()
	want := "错误: something failed\n"
	if got != want {
		t.Errorf("WriteError (text) 输出 = %q, 期望 %q", got, want)
	}
}

func TestFormatter_WriteError_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatJSON, &buf)

	if err := f.WriteError(errors.New("something failed")); err != nil {
		t.Fatalf("WriteError 返回错误: %v", err)
	}

	got := buf.String()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &parsed); err != nil {
		t.Fatalf("输出不是合法 JSON: %v, 内容: %q", err, got)
	}
	if parsed["error"] != "something failed" {
		t.Errorf("JSON error = %v, 期望 \"something failed\"", parsed["error"])
	}
}

// --- WriteEvent ---

func TestFormatter_WriteEvent_StreamJSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatStreamJSON, &buf)

	data := map[string]any{"token": "hi"}
	if err := f.WriteEvent("token", data); err != nil {
		t.Fatalf("WriteEvent 返回错误: %v", err)
	}

	got := buf.String()
	line := strings.TrimSpace(got)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("输出不是合法 JSON: %v, 内容: %q", err, line)
	}
	if parsed["type"] != "token" {
		t.Errorf("type = %v, 期望 \"token\"", parsed["type"])
	}
	if _, ok := parsed["ts"].(float64); !ok {
		t.Errorf("ts 应为数字，实际为 %v", parsed["ts"])
	}
	tokenData, ok := parsed["data"].(map[string]any)
	if !ok {
		t.Fatalf("data 应为对象，实际为 %v", parsed["data"])
	}
	if tokenData["token"] != "hi" {
		t.Errorf("data.token = %v, 期望 \"hi\"", tokenData["token"])
	}
}

func TestFormatter_WriteEvent_NonStreamFormat(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatText, &buf)

	err := f.WriteEvent("token", "some data")
	if err != nil {
		t.Fatalf("WriteEvent 返回了意外错误: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("非 StreamJSON 格式下 WriteEvent 应为 no-op，但写入了: %q", buf.String())
	}
}

// --- Write ---

func TestFormatter_Write_Text(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatText, &buf)

	if err := f.Write("plain text"); err != nil {
		t.Fatalf("Write 返回错误: %v", err)
	}

	got := buf.String()
	if got != "plain text" {
		t.Errorf("Write (text) 输出 = %q, 期望 %q", got, "plain text")
	}
}

func TestFormatter_Write_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatterWithWriter(FormatJSON, &buf)

	// FormatJSON 时 Write 内部调用 WriteEvent，但 WriteEvent 对非 StreamJSON 是 no-op
	// 所以 FormatJSON 下 Write 也是 no-op
	if err := f.Write("hello"); err != nil {
		t.Fatalf("Write 返回错误: %v", err)
	}

	got := buf.String()
	if got != "" {
		t.Errorf("Write (json) 应为 no-op，但写入了: %q", got)
	}
}
