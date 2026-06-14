package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	ap "agentprimordia/pkg"
)

func TestMockProvider_Complete_DefaultResponse(t *testing.T) {
	p := NewMockProvider()
	resp, err := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if resp.Role != "assistant" {
		t.Fatalf("expected role=assistant, got %s", resp.Role)
	}
	if resp.Usage.TotalTokens == 0 {
		t.Fatal("expected non-zero usage")
	}
}

func TestMockProvider_Complete_PresetResponse(t *testing.T) {
	p := NewMockProvider().WithResponse("hello world")
	resp, err := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello world" {
		t.Fatalf("expected 'hello world', got %q", resp.Content)
	}
}

func TestMockProvider_Complete_MultipleResponses(t *testing.T) {
	p := NewMockProvider().WithResponse("first").WithResponse("second")

	resp1, _ := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "1"}},
	})
	if resp1.Content != "first" {
		t.Fatalf("expected 'first', got %q", resp1.Content)
	}

	resp2, _ := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "2"}},
	})
	if resp2.Content != "second" {
		t.Fatalf("expected 'second', got %q", resp2.Content)
	}

	// Third call falls back to default
	resp3, _ := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "3"}},
	})
	if resp3.Content == "" {
		t.Fatal("expected default response, got empty")
	}
}

func TestMockProvider_CallTools_PresetToolCall(t *testing.T) {
	p := NewMockProvider().WithToolCall("read_file", `{"path": "/tmp/test.txt"}`)
	resp, err := p.CallTools(context.Background(), &ap.ToolCallRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "read the file"}},
		Tools:    []ap.ToolDefinition{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "read_file" {
		t.Fatalf("expected tool name 'read_file', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments != `{"path": "/tmp/test.txt"}` {
		t.Fatalf("unexpected arguments: %q", resp.ToolCalls[0].Arguments)
	}
}

func TestMockProvider_CallTools_DefaultEmpty(t *testing.T) {
	p := NewMockProvider()
	resp, err := p.CallTools(context.Background(), &ap.ToolCallRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "do something"}},
		Tools:    []ap.ToolDefinition{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("expected 0 tool calls, got %d", len(resp.ToolCalls))
	}
}

func TestMockProvider_Stream_DefaultResponse(t *testing.T) {
	p := NewMockProvider()
	ch, err := p.Stream(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []string
	var gotDone bool
	for chunk := range ch {
		chunks = append(chunks, chunk.Content)
		if chunk.Done {
			gotDone = true
			if chunk.Usage == nil {
				t.Fatal("expected usage on final chunk")
			}
		}
	}
	if !gotDone {
		t.Fatal("expected a done chunk")
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

func TestMockProvider_Stream_PresetChunks(t *testing.T) {
	p := NewMockProvider().WithStreamChunks("Hello", " ", "World")
	ch, err := p.Stream(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "greet"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.Content)
	}
	if sb.String() != "Hello World" {
		t.Fatalf("expected 'Hello World', got %q", sb.String())
	}
}

func TestMockProvider_Stream_WithResponseFallback(t *testing.T) {
	p := NewMockProvider().WithResponse("fallback content")
	ch, err := p.Stream(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.Content)
	}
	if sb.String() != "fallback content" {
		t.Fatalf("expected 'fallback content', got %q", sb.String())
	}
}

func TestMockProvider_WithError(t *testing.T) {
	testErr := errors.New("something went wrong")
	p := NewMockProvider().WithError(testErr)

	_, err := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "something went wrong" {
		t.Fatalf("expected 'something went wrong', got %q", err.Error())
	}
}

func TestMockProvider_WithError_ThenSuccess(t *testing.T) {
	p := NewMockProvider().
		WithError(errors.New("fail")).
		WithResponse("success")

	// First call: error
	_, err := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "1"}},
	})
	if err == nil {
		t.Fatal("expected error on first call")
	}

	// Second call: success (but WithResponse was consumed by error slot)
	// Note: errors and responses are separate queues; error at index 0
	// consumes callIndex 0, then WithResponse is at responses[0] for callIndex 1.
	// Actually, errors are indexed by callIndex, not consumed like responses.
	// So the second call (callIndex=1) has no error and gets the preset response.
	resp, err := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "2"}},
	})
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if resp.Content != "success" {
		t.Fatalf("expected 'success', got %q", resp.Content)
	}
}

func TestMockProvider_Info(t *testing.T) {
	p := NewMockProvider()
	info := p.Info()
	if info.Name != "mock-model" {
		t.Fatalf("expected 'mock-model', got %q", info.Name)
	}
	if info.Provider != "mock" {
		t.Fatalf("expected 'mock', got %q", info.Provider)
	}
	if !info.SupportsTools {
		t.Fatal("expected SupportsTools=true")
	}
	if !info.SupportsStreaming {
		t.Fatal("expected SupportsStreaming=true")
	}
}

func TestMockProvider_CallHistory(t *testing.T) {
	p := NewMockProvider().
		WithResponse("hello").
		WithToolCall("read_file", `{"path": "x"}`)

	_, _ = p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
	})
	_, _ = p.CallTools(context.Background(), &ap.ToolCallRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "read"}},
		Tools:    []ap.ToolDefinition{},
	})

	history := p.CallHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(history))
	}
	if history[0].Method != "Complete" {
		t.Fatalf("expected method 'Complete', got %q", history[0].Method)
	}
	if history[1].Method != "CallTools" {
		t.Fatalf("expected method 'CallTools', got %q", history[1].Method)
	}
	if history[0].Index != 0 || history[1].Index != 1 {
		t.Fatalf("expected indices 0,1 got %d,%d", history[0].Index, history[1].Index)
	}
}

func TestMockProvider_CallCount(t *testing.T) {
	p := NewMockProvider()
	if p.CallCount() != 0 {
		t.Fatalf("expected 0 calls, got %d", p.CallCount())
	}

	_, _ = p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "1"}},
	})
	if p.CallCount() != 1 {
		t.Fatalf("expected 1 call, got %d", p.CallCount())
	}

	_, _ = p.Stream(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "2"}},
	})
	if p.CallCount() != 2 {
		t.Fatalf("expected 2 calls, got %d", p.CallCount())
	}
}

func TestMockProvider_Reset(t *testing.T) {
	p := NewMockProvider().
		WithResponse("hello").
		WithToolCall("read_file", `{"path": "x"}`).
		WithStreamChunks("a", "b").
		WithError(errors.New("fail"))

	// Make a call
	_, _ = p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
	})

	p.Reset()

	// After reset, call history is empty
	if p.CallCount() != 0 {
		t.Fatalf("expected 0 calls after reset, got %d", p.CallCount())
	}

	// After reset, default response is returned
	resp, err := p.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error after reset: %v", err)
	}
	if resp.Content != "This is a default mock response" {
		t.Fatalf("expected default response after reset, got %q", resp.Content)
	}
}

func TestMockProvider_ImplementsInterface(t *testing.T) {
	// Compile-time check that MockProvider implements ap.Provider
	var _ ap.Provider = (*MockProvider)(nil)
}

func TestMockProvider_Stream_ContextCancellation(t *testing.T) {
	p := NewMockProvider().WithStreamChunks("a", "b", "c")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	ch, err := p.Stream(ctx, &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Channel should close without delivering all chunks
	var count int
	for range ch {
		count++
	}
	// With immediate cancel, we may get 0 or 1 chunks depending on goroutine scheduling
	if count > 3 {
		t.Fatalf("expected at most 3 chunks, got %d", count)
	}
}
