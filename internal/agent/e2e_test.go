package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/testutil"
)

// TestSimpleConversation tests a basic back-and-forth conversation
// using the mock provider to verify the agent pipeline works end-to-end.
func TestSimpleConversation(t *testing.T) {
	provider := testutil.NewMockProvider().
		WithResponse("Hello! I'm a test assistant.").
		WithResponse("The answer is 42.")

	sess, cleanup := testutil.NewTestSession(t, provider)
	defer cleanup()

	// First turn
	resp, err := sess.Ask(context.Background(), "Hi there!")
	if err != nil {
		t.Fatalf("first Ask failed: %v", err)
	}
	if resp.Content != "Hello! I'm a test assistant." {
		t.Fatalf("unexpected first response: %q", resp.Content)
	}

	// Second turn
	resp, err = sess.Ask(context.Background(), "What is the answer?")
	if err != nil {
		t.Fatalf("second Ask failed: %v", err)
	}
	if resp.Content != "The answer is 42." {
		t.Fatalf("unexpected second response: %q", resp.Content)
	}
}

// TestToolCallWorkflow tests an agent that calls a tool.
// This verifies that the mock provider's CallTools integration works
// correctly with the AP framework's ReAct loop.
func TestToolCallWorkflow(t *testing.T) {
	provider := testutil.NewMockProvider().
		// First LLM call: agent decides to call a tool
		WithToolCall("read_file", `{"file_path": "/tmp/test.txt"}`).
		// Second LLM call: agent summarizes the tool result
		WithResponse("The file contains: hello world")

	agent, cleanup := testutil.NewTestAgent(t, provider)
	defer cleanup()

	// Register a tool so the ReAct loop uses CallTools instead of Complete.
	// The tool doesn't need to actually execute — we just need it registered
	// so the LLM sees it in the tool definitions and the agent calls CallTools.
	registry := ap.NewToolRegistry()
	registry.Register(&stubTool{})
	agent.WithToolkit(registry)

	// Run the agent directly (without session, to test the ReAct loop)
	resp, err := agent.Run(context.Background(), ap.UserMessage("Read the file /tmp/test.txt and tell me what's in it"))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// The agent should have made at least 1 CallTools call
	history := provider.CallHistory()
	hasCallTools := false
	for _, call := range history {
		if call.Method == "CallTools" {
			hasCallTools = true
			break
		}
	}
	if !hasCallTools {
		t.Fatal("expected at least one CallTools call in history")
	}

	// Verify the response was produced
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// stubTool is a minimal Tool implementation for testing.
type stubTool struct{}

func (s *stubTool) Name() string                       { return "read_file" }
func (s *stubTool) Description() string                 { return "Read a file from disk" }
func (s *stubTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_path": {"type": "string", "description": "Path to the file to read"}
		},
		"required": ["file_path"]
	}`)
}
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (*ap.ToolResult, error) {
	return &ap.ToolResult{Content: "file content: hello world"}, nil
}

// TestErrorRecovery tests agent behavior when the LLM returns an error.
// This verifies that the mock provider can simulate failures and the
// AP framework handles them gracefully.
// The AP framework retries up to 3 times (maxRetries=2), so we preset
// 3 errors to exhaust all retries.
func TestErrorRecovery(t *testing.T) {
	rateLimitErr := errors.New("API rate limit exceeded")
	provider := testutil.NewMockProvider().
		WithError(rateLimitErr).
		WithError(rateLimitErr).
		WithError(rateLimitErr)

	agent, cleanup := testutil.NewTestAgent(t, provider)
	defer cleanup()

	// The agent should propagate the error after exhausting retries
	_, err := agent.Run(context.Background(), ap.UserMessage("Hello"))
	if err == nil {
		t.Fatal("expected error from mock provider, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("expected rate limit error, got: %v", err)
	}
}

// TestStreamConversation tests streaming output via the mock provider.
func TestStreamConversation(t *testing.T) {
	provider := testutil.NewMockProvider().
		WithStreamChunks("Hello", " ", "World", "!")

	agent, cleanup := testutil.NewTestAgent(t, provider)
	defer cleanup()

	eventCh, err := agent.StreamRun(context.Background(), ap.UserMessage("Say hello"))
	if err != nil {
		t.Fatalf("StreamRun failed: %v", err)
	}

	var sb strings.Builder
	for event := range eventCh {
		if event.Type == ap.StreamEventToken {
			sb.WriteString(event.Content)
		}
	}

	result := sb.String()
	if !strings.Contains(result, "Hello") {
		t.Fatalf("expected stream to contain 'Hello', got: %q", result)
	}
}

// TestMockProviderResetBetweenTests verifies that resetting the mock provider
// between sub-tests prevents state leakage.
func TestMockProviderResetBetweenTests(t *testing.T) {
	provider := testutil.NewMockProvider()

	t.Run("first", func(t *testing.T) {
		provider.WithResponse("first response")
		resp, err := provider.Complete(context.Background(), &ap.CompletionRequest{
			Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content != "first response" {
			t.Fatalf("expected 'first response', got %q", resp.Content)
		}
		provider.Reset()
	})

	t.Run("second", func(t *testing.T) {
		// After reset, should get default response
		resp, err := provider.Complete(context.Background(), &ap.CompletionRequest{
			Messages: []ap.ChatMessage{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Content != "This is a default mock response" {
			t.Fatalf("expected default response after reset, got %q", resp.Content)
		}
	})
}

// TestCallHistoryTracking verifies that the mock provider correctly
// records all calls for later inspection.
func TestCallHistoryTracking(t *testing.T) {
	provider := testutil.NewMockProvider().
		WithResponse("hello")

	_, _ = provider.Complete(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "msg1"}},
	})
	_, _ = provider.Stream(context.Background(), &ap.CompletionRequest{
		Messages: []ap.ChatMessage{{Role: "user", Content: "msg2"}},
	})

	history := provider.CallHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 calls in history, got %d", len(history))
	}
	if history[0].Method != "Complete" {
		t.Errorf("expected first call to be Complete, got %q", history[0].Method)
	}
	if history[1].Method != "Stream" {
		t.Errorf("expected second call to be Stream, got %q", history[1].Method)
	}
}
