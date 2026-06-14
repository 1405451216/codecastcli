package testutil

import (
	"context"
	"fmt"
	"sync"

	ap "agentprimordia/pkg"
)

// MockProvider simulates an LLM Provider for testing.
// It implements the ap.Provider interface (Complete, Stream, CallTools, Info)
// and records all calls for later inspection.
type MockProvider struct {
	mu          sync.Mutex
	responses   []mockResponseEntry
	toolCalls   []mockToolCallEntry
	streamCfgs  []mockStreamConfig
	errors      []error
	callIndex   int
	callHistory []MockCall
}

// mockResponseEntry holds a preset text response for Complete.
type mockResponseEntry struct {
	content string
	usage   ap.Usage
}

// mockToolCallEntry holds a preset tool call response for CallTools.
type mockToolCallEntry struct {
	content   string
	toolCalls []ap.FunctionCall
	usage     ap.Usage
}

// mockStreamConfig holds preset stream chunk configuration.
type mockStreamConfig struct {
	chunks []string
	usage  ap.Usage
}

// MockCall records a single call made to the mock provider.
type MockCall struct {
	Method   string // "Complete", "Stream", or "CallTools"
	Messages []ap.ChatMessage
	Index    int
}

// NewMockProvider creates a new MockProvider with sensible defaults.
// If no responses are preset, Complete returns a default response.
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// WithResponse presets a text response for the next Complete call.
// Returns the receiver for chaining.
func (m *MockProvider) WithResponse(content string) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, mockResponseEntry{
		content: content,
		usage:   ap.Usage{PromptTokens: 10, CompletionTokens: max(1, len(content)/4), TotalTokens: 10 + max(1, len(content)/4)},
	})
	return m
}

// WithToolCall presets a tool call response for the next CallTools call.
// name is the tool name; args is the JSON arguments string.
// Returns the receiver for chaining.
func (m *MockProvider) WithToolCall(name string, args string) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls = append(m.toolCalls, mockToolCallEntry{
		content: "",
		toolCalls: []ap.FunctionCall{
			{
				ID:        fmt.Sprintf("call_%d", len(m.toolCalls)),
				Name:      name,
				Arguments: args,
			},
		},
		usage: ap.Usage{PromptTokens: 20, CompletionTokens: 30, TotalTokens: 50},
	})
	return m
}

// WithStreamChunks presets stream output chunks for the next Stream call.
// Each string in chunks becomes a separate Chunk delivered on the channel.
// Returns the receiver for chaining.
func (m *MockProvider) WithStreamChunks(chunks ...string) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamCfgs = append(m.streamCfgs, mockStreamConfig{
		chunks: chunks,
		usage:  ap.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	})
	return m
}

// WithError presets an error for the next call (any method).
// Returns the receiver for chaining.
func (m *MockProvider) WithError(err error) *MockProvider {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = append(m.errors, err)
	return m
}

// Complete implements the ap.Provider interface.
func (m *MockProvider) Complete(ctx context.Context, req *ap.CompletionRequest) (*ap.CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.callIndex
	m.callIndex++

	m.callHistory = append(m.callHistory, MockCall{
		Method:   "Complete",
		Messages: req.Messages,
		Index:    idx,
	})

	// Check for preset error
	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}

	// Return preset response or default
	if len(m.responses) == 0 {
		return &ap.CompletionResponse{
			ID:      fmt.Sprintf("mock-complete-%d", idx),
			Model:   "mock-model",
			Content: "This is a default mock response",
			Role:    "assistant",
			Usage:   ap.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
		}, nil
	}

	resp := m.responses[0]
	m.responses = m.responses[1:]
	return &ap.CompletionResponse{
		ID:      fmt.Sprintf("mock-complete-%d", idx),
		Model:   "mock-model",
		Content: resp.content,
		Role:    "assistant",
		Usage:   resp.usage,
	}, nil
}

// Stream implements the ap.Provider interface.
func (m *MockProvider) Stream(ctx context.Context, req *ap.CompletionRequest) (<-chan ap.Chunk, error) {
	m.mu.Lock()

	idx := m.callIndex
	m.callIndex++

	m.callHistory = append(m.callHistory, MockCall{
		Method:   "Stream",
		Messages: req.Messages,
		Index:    idx,
	})

	// Check for preset error
	if idx < len(m.errors) && m.errors[idx] != nil {
		m.mu.Unlock()
		return nil, m.errors[idx]
	}

	// Get stream config or fall back to Complete
	var chunks []string
	var usage ap.Usage
	if len(m.streamCfgs) > 0 {
		cfg := m.streamCfgs[0]
		m.streamCfgs = m.streamCfgs[1:]
		chunks = cfg.chunks
		usage = cfg.usage
	} else {
		// Fall back: use the next Complete response as a single chunk
		content := "This is a default mock stream response"
		if len(m.responses) > 0 {
			r := m.responses[0]
			m.responses = m.responses[1:]
			content = r.content
			usage = r.usage
		}
		chunks = []string{content}
	}
	m.mu.Unlock()

	ch := make(chan ap.Chunk, len(chunks)+1)
	go func() {
		defer close(ch)
		for i, chunk := range chunks {
			select {
			case <-ctx.Done():
				return
			default:
			}
			isLast := i == len(chunks)-1
			var chunkUsage *ap.Usage
			if isLast {
				chunkUsage = &usage
			}
			ch <- ap.Chunk{
				Content: chunk,
				Done:    isLast,
				Usage:   chunkUsage,
			}
		}
	}()

	return ch, nil
}

// CallTools implements the ap.Provider interface.
func (m *MockProvider) CallTools(ctx context.Context, req *ap.ToolCallRequest) (*ap.ToolCallResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.callIndex
	m.callIndex++

	m.callHistory = append(m.callHistory, MockCall{
		Method:   "CallTools",
		Messages: req.Messages,
		Index:    idx,
	})

	// Check for preset error
	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}

	// Return preset tool call response or default empty
	if len(m.toolCalls) == 0 {
		return &ap.ToolCallResponse{
			Content:   "",
			ToolCalls: []ap.FunctionCall{},
			Usage:     ap.Usage{},
		}, nil
	}

	resp := m.toolCalls[0]
	m.toolCalls = m.toolCalls[1:]
	return &ap.ToolCallResponse{
		Content:   resp.content,
		ToolCalls: resp.toolCalls,
		Usage:     resp.usage,
	}, nil
}

// Info implements the ap.Provider interface.
func (m *MockProvider) Info() ap.ModelInfo {
	return ap.ModelInfo{
		Name:              "mock-model",
		Provider:          "mock",
		MaxContext:        4096,
		SupportsTools:     true,
		SupportsStreaming: true,
	}
}

// CallHistory returns all calls made to the mock provider.
func (m *MockProvider) CallHistory() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockCall, len(m.callHistory))
	copy(result, m.callHistory)
	return result
}

// CallCount returns the total number of calls made.
func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.callHistory)
}

// Reset clears all preset responses, errors, and call history.
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = nil
	m.toolCalls = nil
	m.streamCfgs = nil
	m.errors = nil
	m.callIndex = 0
	m.callHistory = nil
}
