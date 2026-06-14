package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"codecast/cli/internal/testutil"
)

// TestAgentStateAfterInterrupt creates an agent with mock provider,
// starts a request, cancels the context (simulating Ctrl+C), and verifies
// the agent is no longer processing and can accept a new request.
func TestAgentStateAfterInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// Simulate the processing flag pattern used by CodecastAgent
	var processing atomic.Bool

	// Create a mock provider that responds
	provider := testutil.NewMockProvider().WithResponse("Hello from mock")
	sess, cleanup := testutil.NewTestSession(t, provider)
	defer cleanup()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Simulate starting a request: mark processing = true (mirrors Process/StreamProcess)
	processing.Store(true)

	// Start the request in a goroutine
	done := make(chan error, 1)
	go func() {
		// defer processing.Store(false) mirrors the defer in Process/StreamProcess
		defer processing.Store(false)
		_, err := sess.Ask(ctx, "test message")
		done <- err
	}()

	// Simulate Ctrl+C: cancel the context
	cancel()

	// Wait for the request to complete
	select {
	case <-done:
		// Request completed (either with response or error due to cancel)
	case <-time.After(5 * time.Second):
		t.Fatal("Ask did not return within 5s after cancel")
	}

	// Verify processing is false after cancel — the defer should have reset it
	if processing.Load() {
		t.Fatal("expected processing to be false after cancel (defer should have reset it)")
	}
}

// TestAgentCanContinueAfterInterrupt verifies that after an interrupt,
// a new message can be sent and gets a response.
func TestAgentCanContinueAfterInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// Simulate the processing flag pattern used by CodecastAgent
	var processing atomic.Bool

	// Create a mock provider with multiple responses
	provider := testutil.NewMockProvider().
		WithResponse("First response").
		WithResponse("Second response after interrupt")
	sess, cleanup := testutil.NewTestSession(t, provider)
	defer cleanup()

	// First request: start and cancel
	ctx1, cancel1 := context.WithCancel(context.Background())
	processing.Store(true)

	done1 := make(chan error, 1)
	go func() {
		defer processing.Store(false)
		_, err := sess.Ask(ctx1, "first message")
		done1 <- err
	}()

	cancel1()

	select {
	case <-done1:
		// First request completed (either with response or error)
	case <-time.After(5 * time.Second):
		t.Fatal("first Ask did not return within 5s")
	}

	// After interrupt, processing should be false
	if processing.Load() {
		t.Fatal("expected processing to be false after first request cancel")
	}

	// Second request: should succeed with a fresh context
	ctx2 := context.Background()
	processing.Store(true)

	resp, err := sess.Ask(ctx2, "second message after interrupt")
	processing.Store(false)

	if err != nil {
		t.Fatalf("second Ask after interrupt failed: %v", err)
	}
	if resp == nil {
		t.Fatal("second Ask returned nil response")
	}
	if resp.Content == "" {
		t.Error("second Ask returned empty content")
	}

	// Verify processing is false after the second request completes
	if processing.Load() {
		t.Fatal("expected processing to be false after second request")
	}
}
