package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Format represents the output format
type Format int

const (
	FormatText      Format = iota // plain text (default)
	FormatJSON                    // single JSON object
	FormatStreamJSON              // NDJSON stream
)

// ParseFormat parses a format string
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "text", "plain", "":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	case "stream-json", "streamjson", "ndjson":
		return FormatStreamJSON, nil
	default:
		return FormatText, fmt.Errorf("未知的输出格式: %s (可选: text, json, stream-json)", s)
	}
}

// Formatter formats and writes output
type Formatter struct {
	format Format
	writer io.Writer
}

// NewFormatter creates a new formatter
func NewFormatter(format Format) *Formatter {
	return &Formatter{
		format: format,
		writer: os.Stdout,
	}
}

// NewFormatterWithWriter creates a formatter with custom writer
func NewFormatterWithWriter(format Format, w io.Writer) *Formatter {
	return &Formatter{
		format: format,
		writer: w,
	}
}

// WriteResult writes a complete result
func (f *Formatter) WriteResult(result any) error {
	switch f.format {
	case FormatText:
		return f.writeTextResult(result)
	case FormatJSON:
		return f.writeJSONResult(result)
	case FormatStreamJSON:
		return f.writeJSONResult(result)
	default:
		return f.writeTextResult(result)
	}
}

// WriteEvent writes a stream event (for stream-json format)
func (f *Formatter) WriteEvent(eventType string, data any) error {
	if f.format != FormatStreamJSON {
		return nil
	}

	obj := map[string]any{
		"type": eventType,
		"ts":   time.Now().UnixMilli(),
		"data": data,
	}

	jsonData, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(f.writer, "%s\n", jsonData)
	return err
}

// WriteError writes an error
func (f *Formatter) WriteError(err error) error {
	switch f.format {
	case FormatJSON, FormatStreamJSON:
		obj := map[string]any{
			"error": err.Error(),
		}
		jsonData, jsonErr := json.Marshal(obj)
		if jsonErr != nil {
			return jsonErr
		}
		_, writeErr := fmt.Fprintf(f.writer, "%s\n", jsonData)
		return writeErr
	default:
		_, writeErr := fmt.Fprintf(f.writer, "错误: %v\n", err)
		return writeErr
	}
}

// Write writes raw text
func (f *Formatter) Write(text string) error {
	if f.format == FormatText {
		_, err := fmt.Fprint(f.writer, text)
		return err
	}
	// For JSON formats, wrap in a content event
	return f.WriteEvent("content", text)
}

// writeTextResult writes result as plain text
func (f *Formatter) writeTextResult(result any) error {
	// Use reflection or type switch to extract content
	switch r := result.(type) {
	case interface{ GetContent() string }:
		_, err := fmt.Fprint(f.writer, r.GetContent())
		return err
	default:
		// Try to marshal as JSON and extract content field
		jsonData, err := json.Marshal(result)
		if err != nil {
			_, writeErr := fmt.Fprintf(f.writer, "%v", result)
			return writeErr
		}
		var m map[string]any
		if json.Unmarshal(jsonData, &m) == nil {
			if content, ok := m["content"].(string); ok {
				_, writeErr := fmt.Fprint(f.writer, content)
				return writeErr
			}
		}
		_, writeErr := fmt.Fprintf(f.writer, "%v", result)
		return writeErr
	}
}

// writeJSONResult writes result as JSON
func (f *Formatter) writeJSONResult(result any) error {
	jsonData, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f.writer, "%s\n", jsonData)
	return err
}

// StreamEvent represents a stream event for NDJSON output
type StreamEvent struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"ts"`
	Data      any    `json:"data,omitempty"`
}

// TokenEventData represents token stream event data
type TokenEventData struct {
	Token string `json:"token"`
}

// ToolCallEventData represents tool call event data
type ToolCallEventData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// ToolResultEventData represents tool result event data
type ToolResultEventData struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

// CompleteEventData represents completion event data
type CompleteEventData struct {
	Content   string   `json:"content"`
	ToolsUsed []string `json:"tools_used"`
}

// ErrorEventData represents error event data
type ErrorEventData struct {
	Message string `json:"message"`
}

// StreamJSONFormatter provides NDJSON streaming output with typed events.
// Each event is a single JSON object per line:
//   - {"type":"token","content":"..."}
//   - {"type":"tool_call","tool":"...","args":{...}}
//   - {"type":"tool_result","tool":"...","content":"..."}
//   - {"type":"complete","result":"...","usage":{...}}
//   - {"type":"error","message":"..."}
type StreamJSONFormatter struct {
	writer io.Writer
}

// NewStreamJSONFormatter creates a new StreamJSONFormatter
func NewStreamJSONFormatter(w io.Writer) *StreamJSONFormatter {
	if w == nil {
		w = os.Stdout
	}
	return &StreamJSONFormatter{writer: w}
}

// writeLine marshals obj to JSON and writes it as a single line
func (s *StreamJSONFormatter) writeLine(obj map[string]any) error {
	jsonData, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.writer, "%s\n", jsonData)
	return err
}

// WriteToken outputs a token event: {"type":"token","content":"..."}
func (s *StreamJSONFormatter) WriteToken(content string) error {
	return s.writeLine(map[string]any{
		"type":    "token",
		"content": content,
	})
}

// WriteToolCall outputs a tool_call event: {"type":"tool_call","tool":"...","args":{...}}
func (s *StreamJSONFormatter) WriteToolCall(tool string, args any) error {
	return s.writeLine(map[string]any{
		"type": "tool_call",
		"tool": tool,
		"args": args,
	})
}

// WriteToolResult outputs a tool_result event: {"type":"tool_result","tool":"...","content":"..."}
func (s *StreamJSONFormatter) WriteToolResult(tool, content string) error {
	return s.writeLine(map[string]any{
		"type":    "tool_result",
		"tool":    tool,
		"content": content,
	})
}

// WriteComplete outputs a complete event: {"type":"complete","result":"...","usage":{...}}
func (s *StreamJSONFormatter) WriteComplete(result string, usage any) error {
	obj := map[string]any{
		"type":   "complete",
		"result": result,
	}
	if usage != nil {
		obj["usage"] = usage
	}
	return s.writeLine(obj)
}

// WriteError outputs an error event: {"type":"error","message":"..."}
func (s *StreamJSONFormatter) WriteError(message string) error {
	return s.writeLine(map[string]any{
		"type":    "error",
		"message": message,
	})
}
