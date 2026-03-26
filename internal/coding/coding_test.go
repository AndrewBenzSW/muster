package coding

import (
	"context"
	"testing"
	"time"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface assertions
var (
	_ CodingTool            = (*ClaudeCodeTool)(nil)
	_ InteractiveCodingTool = (*ClaudeCodeTool)(nil)
)

func TestNewClaudeCodeTool(t *testing.T) {
	tool, err := NewClaudeCodeTool()
	require.NoError(t, err)
	assert.NotNil(t, tool)
}

// TestClaudeCodeTool_ContextCancellation verifies that when a context is cancelled,
// the AI invocation stops promptly.
func TestClaudeCodeTool_ContextCancellation(t *testing.T) {
	// Save original InvokeAI and restore after test
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	// Create a channel to signal when InvokeAI is called
	invokeCalled := make(chan struct{})

	// Mock InvokeAI to simulate a long-running operation that respects context cancellation
	ai.InvokeAI = func(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		close(invokeCalled)

		// Simulate a long operation that checks context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &ai.InvokeResult{RawOutput: "should not reach here"}, nil
		}
	}

	// Create a context that we'll cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Create tool and invoke
	tool, err := NewClaudeCodeTool()
	require.NoError(t, err)

	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}

	// Measure how long the call takes
	start := time.Now()
	result, err := tool.Invoke(ctx, cfg)
	elapsed := time.Since(start)

	// Wait for InvokeAI to be called (or timeout)
	select {
	case <-invokeCalled:
		// Good, it was called
	case <-time.After(1 * time.Second):
		t.Fatal("InvokeAI was not called within 1 second")
	}

	// Should return an error due to context cancellation
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)

	// Should return quickly (not wait the full 5 seconds)
	assert.Less(t, elapsed, 1*time.Second, "context cancellation should be respected promptly")
}

// TestClaudeCodeTool_ContextTimeout verifies that context timeout is respected.
func TestClaudeCodeTool_ContextTimeout(t *testing.T) {
	// Save original InvokeAI and restore after test
	originalInvokeAI := ai.InvokeAI
	defer func() { ai.InvokeAI = originalInvokeAI }()

	// Mock InvokeAI to simulate a long-running operation
	ai.InvokeAI = func(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		// Simulate a long operation that checks context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return &ai.InvokeResult{RawOutput: "should not reach here"}, nil
		}
	}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create tool and invoke
	tool, err := NewClaudeCodeTool()
	require.NoError(t, err)

	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}

	// Measure how long the call takes
	start := time.Now()
	result, err := tool.Invoke(ctx, cfg)
	elapsed := time.Since(start)

	// Should return an error due to context timeout
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Should timeout around 100ms, not wait the full 5 seconds
	assert.Less(t, elapsed, 1*time.Second, "context timeout should be respected promptly")
}
