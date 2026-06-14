package testutil

import (
	"context"
	"testing"

	ap "agentprimordia/pkg"
)

// NewTestAgent creates a minimal ap.CapabilityAgent with a MockProvider for testing.
// It returns the agent and a cleanup function that closes associated resources.
//
// Usage:
//
//	provider := testutil.NewMockProvider().WithResponse("hello")
//	agent, cleanup := testutil.NewTestAgent(t, provider)
//	defer cleanup()
//	resp, err := agent.Run(ctx, ap.UserMessage("hi"))
func NewTestAgent(t testing.TB, provider *MockProvider) (*ap.CapabilityAgent, func()) {
	t.Helper()

	agent := ap.NewAgent("test-agent", "You are a helpful test assistant.", provider,
		ap.WithMaxTurns(5),
	)

	cleanup := func() {
		// No-op for now; reserved for future resource cleanup.
	}

	return agent, cleanup
}

// NewTestSession creates a minimal ap.Session backed by a MockProvider.
// It returns the session and a cleanup function.
//
// Usage:
//
//	provider := testutil.NewMockProvider().WithResponse("hello")
//	sess, cleanup := testutil.NewTestSession(t, provider)
//	defer cleanup()
//	resp, err := sess.Ask(ctx, "hi")
func NewTestSession(t testing.TB, provider *MockProvider) (*ap.Session, func()) {
	t.Helper()

	agent := ap.NewAgent("test-agent", "You are a helpful test assistant.", provider,
		ap.WithMaxTurns(5),
	)

	mem, err := ap.WithInMemory()
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}

	sess := ap.NewSession(agent, mem)

	cleanup := func() {
		if mem != nil {
			_ = mem.Close()
		}
	}

	return sess, cleanup
}

// RunAgentAsk is a convenience helper that creates a session from the mock provider,
// sends a message, and returns the response content. Useful for quick one-liner tests.
func RunAgentAsk(t testing.TB, provider *MockProvider, msg string) string {
	t.Helper()

	sess, cleanup := NewTestSession(t, provider)
	defer cleanup()

	resp, err := sess.Ask(context.Background(), msg)
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	return resp.Content
}
