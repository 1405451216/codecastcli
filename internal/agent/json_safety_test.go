package agent

import (
	"encoding/json"
	"testing"
)

// TestJSONUnmarshalSafe: Test that json.Unmarshal handles \u0022 escapes correctly.
func TestJSONUnmarshalSafe(t *testing.T) {
	input := `{"key": "value with \u0022quotes\u0022 inside"}`
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	got := jsonGetString(m, "key")
	want := `value with "quotes" inside`
	if got != want {
		t.Errorf("jsonGetString with \\u0022 escapes = %q, want %q", got, want)
	}
}

// TestJSONUnmarshalNested: Test nested JSON values are preserved.
func TestJSONUnmarshalNested(t *testing.T) {
	input := `{"outer": "before", "inner": {"nested_key": "nested_value"}, "after": "end"}`
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// String fields should be extractable
	if got := jsonGetString(m, "outer"); got != "before" {
		t.Errorf("jsonGetString(outer) = %q, want %q", got, "before")
	}
	if got := jsonGetString(m, "after"); got != "end" {
		t.Errorf("jsonGetString(after) = %q, want %q", got, "end")
	}

	// Nested object should return empty string (not a string value)
	if got := jsonGetString(m, "inner"); got != "" {
		t.Errorf("jsonGetString(inner) = %q, want empty (not a string)", got)
	}

	// But the raw message should still be intact
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(m["inner"], &inner); err != nil {
		t.Fatalf("unmarshal nested object failed: %v", err)
	}
	if got := jsonGetString(inner, "nested_key"); got != "nested_value" {
		t.Errorf("nested nested_key = %q, want %q", got, "nested_value")
	}
}

// TestJSONUnmarshalEmptyArgs: Test empty args string doesn't panic.
func TestJSONUnmarshalEmptyArgs(t *testing.T) {
	var m map[string]json.RawMessage

	// Empty object
	if err := json.Unmarshal([]byte(`{}`), &m); err != nil {
		t.Fatalf("json.Unmarshal({}) failed: %v", err)
	}
	if got := jsonGetString(m, "any_key"); got != "" {
		t.Errorf("jsonGetString on empty object = %q, want empty", got)
	}

	// Empty string (invalid JSON) should not panic
	m = nil
	if err := json.Unmarshal([]byte(``), &m); err == nil {
		// If it somehow succeeds, verify no panic
		if got := jsonGetString(m, "key"); got != "" {
			t.Errorf("jsonGetString on nil map = %q, want empty", got)
		}
	}

	// nil map should not panic
	if got := jsonGetString(nil, "key"); got != "" {
		t.Errorf("jsonGetString(nil, key) = %q, want empty", got)
	}
}

// TestJSONUnmarshalSpecialChars: Test various special characters in JSON.
func TestJSONUnmarshalSpecialChars(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		key      string
		expected string
	}{
		{
			name:     "newline escape",
			json:     `{"key": "line1\nline2"}`,
			key:      "key",
			expected: "line1\nline2",
		},
		{
			name:     "tab escape",
			json:     `{"key": "col1\tcol2"}`,
			key:      "key",
			expected: "col1\tcol2",
		},
		{
			name:     "backslash escape",
			json:     `{"key": "path\\to\\file"}`,
			key:      "key",
			expected: "path\\to\\file",
		},
		{
			name:     "unicode CJK",
			json:     `{"key": "中文测试"}`,
			key:      "key",
			expected: "中文测试",
		},
		{
			name:     "emoji",
			json:     `{"key": "hello 🌍"}`,
			key:      "key",
			expected: "hello 🌍",
		},
		{
			name:     "mixed escapes",
			json:     `{"key": "a\u000Ab\u0009c"}`,
			key:      "key",
			expected: "a\nb\tc",
		},
		{
			name:     "forward slashes (not escaped)",
			json:     `{"key": "http://example.com/path"}`,
			key:      "key",
			expected: "http://example.com/path",
		},
		{
			name:     "escaped forward slashes",
			json:     `{"key": "http:\/\/example.com\/path"}`,
			key:      "key",
			expected: "http://example.com/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tt.json), &m); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			got := jsonGetString(m, tt.key)
			if got != tt.expected {
				t.Errorf("jsonGetString = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestJSONGetStringSafety: Test the jsonGetString helper with edge cases.
func TestJSONGetStringSafety(t *testing.T) {
	t.Run("missing key returns empty", func(t *testing.T) {
		var m map[string]json.RawMessage
		json.Unmarshal([]byte(`{"other": "val"}`), &m)
		if got := jsonGetString(m, "missing"); got != "" {
			t.Errorf("missing key = %q, want empty", got)
		}
	})

	t.Run("null value returns empty", func(t *testing.T) {
		var m map[string]json.RawMessage
		json.Unmarshal([]byte(`{"key": null}`), &m)
		if got := jsonGetString(m, "key"); got != "" {
			t.Errorf("null value = %q, want empty", got)
		}
	})

	t.Run("number value returns empty", func(t *testing.T) {
		var m map[string]json.RawMessage
		json.Unmarshal([]byte(`{"key": 42}`), &m)
		if got := jsonGetString(m, "key"); got != "" {
			t.Errorf("number value = %q, want empty", got)
		}
	})

	t.Run("boolean value returns empty", func(t *testing.T) {
		var m map[string]json.RawMessage
		json.Unmarshal([]byte(`{"key": true}`), &m)
		if got := jsonGetString(m, "key"); got != "" {
			t.Errorf("boolean value = %q, want empty", got)
		}
	})

	t.Run("array value returns empty", func(t *testing.T) {
		var m map[string]json.RawMessage
		json.Unmarshal([]byte(`{"key": [1,2,3]}`), &m)
		if got := jsonGetString(m, "key"); got != "" {
			t.Errorf("array value = %q, want empty", got)
		}
	})

	t.Run("empty string value returns empty string", func(t *testing.T) {
		var m map[string]json.RawMessage
		json.Unmarshal([]byte(`{"key": ""}`), &m)
		if got := jsonGetString(m, "key"); got != "" {
			t.Errorf("empty string value = %q, want empty string", got)
		}
	})

	t.Run("long string value works", func(t *testing.T) {
		var m map[string]json.RawMessage
		longStr := stringsRepeat("a", 10000)
		json.Unmarshal([]byte(`{"key": "`+longStr+`"}`), &m)
		if got := jsonGetString(m, "key"); len(got) != 10000 {
			t.Errorf("long string length = %d, want 10000", len(got))
		}
	})
}

// stringsRepeat avoids importing strings just for one use in test
func stringsRepeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
