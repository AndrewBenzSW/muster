package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/abenz1267/muster/internal/ai"
)

// MockInfraError represents an error in the mock infrastructure itself
// (missing response files, malformed JSON, etc.), not a simulated tool error.
type MockInfraError struct {
	Message string
}

// Error implements the error interface.
func (e *MockInfraError) Error() string {
	return "mock infrastructure error: " + e.Message
}

// SimulatedToolError represents a simulated error from the AI tool itself
// (e.g., tool returned non-zero exit code, API error).
type SimulatedToolError struct {
	Message string
}

// Error implements the error interface.
func (e *SimulatedToolError) Error() string {
	return e.Message
}

// MockCodingTool is a file-based mock implementation of CodingTool.
// It records invocations to JSON files and reads responses from pre-staged files.
//
// This mock is not safe for concurrent use. Each test must create its own mock
// instance with separate t.TempDir() directories. Do not share mock instances
// across t.Parallel() subtests.
//
// Does not validate input parameters - tests should verify correct config by
// inspecting call files.
type MockCodingTool struct {
	callsDir     string
	responsesDir string
	callCount    atomic.Int32
}

// NewMockCodingTool creates a new file-based mock coding tool.
func NewMockCodingTool(callsDir, responsesDir string) *MockCodingTool {
	return &MockCodingTool{
		callsDir:     callsDir,
		responsesDir: responsesDir,
	}
}

// invokeCall represents a recorded invocation call.
type invokeCall struct {
	Tool    string            `json:"tool"`
	Model   string            `json:"model"`
	Prompt  string            `json:"prompt"`
	Timeout string            `json:"timeout"`
	Env     map[string]string `json:"env,omitempty"`
}

// invokeResponse represents a mock response to an invocation.
type invokeResponse struct {
	RawOutput string `json:"raw_output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Invoke records the call and returns a response from a pre-staged file.
func (m *MockCodingTool) Invoke(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
	// Increment call counter
	callNum := m.callCount.Add(1)

	// Record the call
	call := invokeCall{
		Tool:    cfg.Tool,
		Model:   cfg.Model,
		Prompt:  cfg.Prompt,
		Timeout: cfg.Timeout.String(),
		Env:     cfg.Env,
	}

	callData, err := json.MarshalIndent(call, "", "  ")
	if err != nil {
		return nil, &MockInfraError{Message: fmt.Sprintf("failed to marshal call: %v", err)}
	}

	callPath := filepath.Join(m.callsDir, fmt.Sprintf("%03d-invoke.json", callNum))
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	if err := os.WriteFile(callPath, callData, 0644); err != nil {
		return nil, &MockInfraError{Message: fmt.Sprintf("failed to write call file: %v", err)}
	}

	// Read the response
	responsePath := filepath.Join(m.responsesDir, fmt.Sprintf("%03d-invoke.json", callNum))
	//nolint:gosec // G304: responsePath is constructed from test temp dirs, not user input
	responseData, err := os.ReadFile(responsePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &MockInfraError{Message: fmt.Sprintf("missing response file for Invoke call #%d (expected %s)", callNum, responsePath)}
		}
		return nil, &MockInfraError{Message: fmt.Sprintf("failed to read response file: %v", err)}
	}

	var resp invokeResponse
	if err := json.Unmarshal(responseData, &resp); err != nil {
		return nil, &MockInfraError{Message: fmt.Sprintf("malformed response JSON: %v", err)}
	}

	// Check for simulated error
	if resp.Error != "" {
		return nil, &SimulatedToolError{Message: resp.Error}
	}

	// Return success result
	return &ai.InvokeResult{
		RawOutput: resp.RawOutput,
	}, nil
}

// MockInteractiveCodingTool is a file-based mock implementation of InteractiveCodingTool.
// It records invocations to JSON files and optionally simulates errors from response files.
//
// This mock is not safe for concurrent use. Each test must create its own mock
// instance with separate t.TempDir() directories. Do not share mock instances
// across t.Parallel() subtests.
//
// Does not validate input parameters - tests should verify correct config by
// inspecting call files.
type MockInteractiveCodingTool struct {
	callsDir  string
	callCount atomic.Int32
}

// NewMockInteractiveCodingTool creates a new file-based mock interactive coding tool.
func NewMockInteractiveCodingTool(callsDir string) *MockInteractiveCodingTool {
	return &MockInteractiveCodingTool{
		callsDir: callsDir,
	}
}

// interactiveCall represents a recorded interactive call.
type interactiveCall struct {
	Tool      string            `json:"tool"`
	Model     string            `json:"model"`
	PluginDir string            `json:"plugin_dir"`
	Env       map[string]string `json:"env,omitempty"`
	Verbose   bool              `json:"verbose"`
}

// interactiveResponse represents a mock response to an interactive call.
type interactiveResponse struct {
	Error string `json:"error,omitempty"`
}

// RunInteractive records the call and optionally returns a simulated error.
func (m *MockInteractiveCodingTool) RunInteractive(ctx context.Context, cfg InteractiveConfig) error {
	// Increment call counter
	callNum := m.callCount.Add(1)

	// Record the call
	call := interactiveCall(cfg)

	callData, err := json.MarshalIndent(call, "", "  ")
	if err != nil {
		return &MockInfraError{Message: fmt.Sprintf("failed to marshal call: %v", err)}
	}

	callPath := filepath.Join(m.callsDir, fmt.Sprintf("%03d-interactive.json", callNum))
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	if err := os.WriteFile(callPath, callData, 0644); err != nil {
		return &MockInfraError{Message: fmt.Sprintf("failed to write call file: %v", err)}
	}

	// Check for optional response file
	responsePath := filepath.Join(m.callsDir, fmt.Sprintf("%03d-interactive-response.json", callNum))
	//nolint:gosec // G304: responsePath is constructed from test temp dir, not user input
	responseData, err := os.ReadFile(responsePath)
	if err != nil {
		// If response file doesn't exist, return success (no simulated error)
		if os.IsNotExist(err) {
			return nil
		}
		return &MockInfraError{Message: fmt.Sprintf("failed to read response file: %v", err)}
	}

	var resp interactiveResponse
	if err := json.Unmarshal(responseData, &resp); err != nil {
		return &MockInfraError{Message: fmt.Sprintf("malformed response JSON: %v", err)}
	}

	// Check for simulated error
	if resp.Error != "" {
		return &SimulatedToolError{Message: resp.Error}
	}

	return nil
}
