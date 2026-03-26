package coding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockCodingTool_SequentialCallRecording(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()
	mock := NewMockCodingTool(callsDir, responsesDir)

	// Stage responses for 3 calls
	for i := 1; i <= 3; i++ {
		resp := invokeResponse{RawOutput: "output " + string(rune('0'+i))}
		respData, err := json.Marshal(resp)
		require.NoError(t, err)
		respPath := filepath.Join(responsesDir, fmt.Sprintf("%03d-invoke.json", i))
		//nolint:gosec // G306: Standard file permissions for test files in temp directory
		err = os.WriteFile(respPath, respData, 0644)
		require.NoError(t, err)
	}

	// Make 3 calls
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		cfg := ai.InvokeConfig{
			Tool:    "test-tool",
			Model:   "test-model",
			Prompt:  "test prompt " + string(rune('0'+i)),
			Timeout: 30 * time.Second,
			Env:     map[string]string{"KEY": "value"},
		}
		_, err := mock.Invoke(ctx, cfg)
		require.NoError(t, err)
	}

	// Verify 3 numbered files were created
	for i := 1; i <= 3; i++ {
		callPath := filepath.Join(callsDir, fmt.Sprintf("%03d-invoke.json", i))
		require.FileExists(t, callPath)

		// Verify JSON content
		//nolint:gosec // G304: callPath is constructed from test temp dir, not user input
		data, err := os.ReadFile(callPath)
		require.NoError(t, err)

		var call invokeCall
		err = json.Unmarshal(data, &call)
		require.NoError(t, err)

		assert.Equal(t, "test-tool", call.Tool)
		assert.Equal(t, "test-model", call.Model)
		assert.Equal(t, "test prompt "+string(rune('0'+i)), call.Prompt)
		assert.Equal(t, "30s", call.Timeout)
		assert.Equal(t, map[string]string{"KEY": "value"}, call.Env)
	}
}

func TestMockCodingTool_ResponseReading(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()
	mock := NewMockCodingTool(callsDir, responsesDir)

	// Stage response fixture
	resp := invokeResponse{RawOutput: "expected output"}
	respData, err := json.Marshal(resp)
	require.NoError(t, err)
	respPath := filepath.Join(responsesDir, "001-invoke.json")
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	err = os.WriteFile(respPath, respData, 0644)
	require.NoError(t, err)

	// Call Invoke
	ctx := context.Background()
	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}
	result, err := mock.Invoke(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "expected output", result.RawOutput)
}

func TestMockCodingTool_MissingResponseFile(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()
	mock := NewMockCodingTool(callsDir, responsesDir)

	// Don't stage any response file
	ctx := context.Background()
	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}
	result, err := mock.Invoke(ctx, cfg)
	require.Error(t, err)
	assert.Nil(t, result)

	// Verify it's a MockInfraError with the expected details
	var mockErr *MockInfraError
	require.True(t, errors.As(err, &mockErr))
	assert.Contains(t, mockErr.Message, "missing response file for Invoke call #1")
	assert.Contains(t, mockErr.Message, filepath.Join(responsesDir, "001-invoke.json"))
}

func TestMockCodingTool_MalformedJSON(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()
	mock := NewMockCodingTool(callsDir, responsesDir)

	// Stage malformed JSON response
	respPath := filepath.Join(responsesDir, "001-invoke.json")
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	err := os.WriteFile(respPath, []byte("{invalid json}"), 0644)
	require.NoError(t, err)

	// Call Invoke
	ctx := context.Background()
	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}
	result, err := mock.Invoke(ctx, cfg)
	require.Error(t, err)
	assert.Nil(t, result)

	// Verify it's a MockInfraError
	var mockErr *MockInfraError
	require.True(t, errors.As(err, &mockErr))
	assert.Contains(t, mockErr.Message, "malformed response JSON")
}

func TestMockCodingTool_ErrorSimulation(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()
	mock := NewMockCodingTool(callsDir, responsesDir)

	// Stage error response
	resp := invokeResponse{Error: "simulated tool failure"}
	respData, err := json.Marshal(resp)
	require.NoError(t, err)
	respPath := filepath.Join(responsesDir, "001-invoke.json")
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	err = os.WriteFile(respPath, respData, 0644)
	require.NoError(t, err)

	// Call Invoke
	ctx := context.Background()
	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}
	result, err := mock.Invoke(ctx, cfg)
	require.Error(t, err)
	assert.Nil(t, result)

	// Verify it's a SimulatedToolError
	var simErr *SimulatedToolError
	require.True(t, errors.As(err, &simErr))
	assert.Equal(t, "simulated tool failure", simErr.Message)
}

func TestMockCodingTool_ErrorDistinction(t *testing.T) {
	callsDir := t.TempDir()
	responsesDir := t.TempDir()
	mock := NewMockCodingTool(callsDir, responsesDir)

	// Test 1: Missing file returns MockInfraError
	ctx := context.Background()
	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}
	_, err := mock.Invoke(ctx, cfg)
	require.Error(t, err)

	var mockErr *MockInfraError
	var simErr *SimulatedToolError
	assert.True(t, errors.As(err, &mockErr), "missing file should return MockInfraError")
	assert.False(t, errors.As(err, &simErr), "missing file should not return SimulatedToolError")

	// Verify error message contains useful information
	assert.Contains(t, mockErr.Message, "001-invoke.json", "error message should include expected file path")
	assert.Contains(t, mockErr.Message, "missing response file for Invoke call", "error message should explain the issue")

	// Test 2: Simulated error returns SimulatedToolError
	resp := invokeResponse{Error: "tool error"}
	respData, err := json.Marshal(resp)
	require.NoError(t, err)
	respPath := filepath.Join(responsesDir, "002-invoke.json")
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	err = os.WriteFile(respPath, respData, 0644)
	require.NoError(t, err)

	_, err = mock.Invoke(ctx, cfg)
	require.Error(t, err)

	assert.False(t, errors.As(err, &mockErr), "simulated error should not return MockInfraError")
	assert.True(t, errors.As(err, &simErr), "simulated error should return SimulatedToolError")

	// Verify error message contains the simulated error text
	assert.Equal(t, "tool error", simErr.Message, "error message should contain simulated tool error")
}

func TestMockInteractiveCodingTool_RecordsCall(t *testing.T) {
	callsDir := t.TempDir()
	mock := NewMockInteractiveCodingTool(callsDir)

	// Call RunInteractive
	ctx := context.Background()
	cfg := InteractiveConfig{
		Tool:      "test-tool",
		Model:     "test-model",
		PluginDir: "/test/plugins",
		Env:       map[string]string{"KEY": "value"},
		Verbose:   true,
	}
	err := mock.RunInteractive(ctx, cfg)
	require.NoError(t, err)

	// Verify call was recorded
	callPath := filepath.Join(callsDir, "001-interactive.json")
	require.FileExists(t, callPath)

	//nolint:gosec // G304: callPath is constructed from test temp dir, not user input
	data, err := os.ReadFile(callPath)
	require.NoError(t, err)

	var call interactiveCall
	err = json.Unmarshal(data, &call)
	require.NoError(t, err)

	assert.Equal(t, "test-tool", call.Tool)
	assert.Equal(t, "test-model", call.Model)
	assert.Equal(t, "/test/plugins", call.PluginDir)
	assert.Equal(t, map[string]string{"KEY": "value"}, call.Env)
	assert.True(t, call.Verbose)
}

func TestMockInteractiveCodingTool_SimulatedError(t *testing.T) {
	callsDir := t.TempDir()
	mock := NewMockInteractiveCodingTool(callsDir)

	// Stage error response
	resp := interactiveResponse{Error: "simulated failure"}
	respData, err := json.Marshal(resp)
	require.NoError(t, err)
	respPath := filepath.Join(callsDir, "001-interactive-response.json")
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	err = os.WriteFile(respPath, respData, 0644)
	require.NoError(t, err)

	// Call RunInteractive
	ctx := context.Background()
	cfg := InteractiveConfig{
		Tool:      "test-tool",
		Model:     "test-model",
		PluginDir: "/test/plugins",
	}
	err = mock.RunInteractive(ctx, cfg)
	require.Error(t, err)

	// Verify it's a SimulatedToolError
	var simErr *SimulatedToolError
	require.True(t, errors.As(err, &simErr))
	assert.Equal(t, "simulated failure", simErr.Message)
}

func TestMockInteractiveCodingTool_MalformedResponseJSON(t *testing.T) {
	callsDir := t.TempDir()
	mock := NewMockInteractiveCodingTool(callsDir)

	// Stage malformed JSON response
	respPath := filepath.Join(callsDir, "001-interactive-response.json")
	//nolint:gosec // G306: Standard file permissions for test files in temp directory
	err := os.WriteFile(respPath, []byte("{invalid json}"), 0644)
	require.NoError(t, err)

	// Call RunInteractive
	ctx := context.Background()
	cfg := InteractiveConfig{
		Tool:      "test-tool",
		Model:     "test-model",
		PluginDir: "/test/plugins",
	}
	err = mock.RunInteractive(ctx, cfg)
	require.Error(t, err)

	// Verify it's a MockInfraError
	var mockErr *MockInfraError
	require.True(t, errors.As(err, &mockErr))
	assert.Contains(t, mockErr.Message, "malformed response JSON")
}

func TestMockInteractiveCodingTool_MissingResponseFile_Success(t *testing.T) {
	callsDir := t.TempDir()
	mock := NewMockInteractiveCodingTool(callsDir)

	// Don't stage any response file
	ctx := context.Background()
	cfg := InteractiveConfig{
		Tool:      "test-tool",
		Model:     "test-model",
		PluginDir: "/test/plugins",
	}
	err := mock.RunInteractive(ctx, cfg)
	// Missing response file should return nil (success)
	require.NoError(t, err)
}

func TestMockCodingTool_UnreadableResponseFile(t *testing.T) {
	// Skip on Windows where permission manipulation is different
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping file permission test on Windows")
	}

	callsDir := t.TempDir()
	responsesDir := t.TempDir()
	mock := NewMockCodingTool(callsDir, responsesDir)

	// Create response file
	resp := invokeResponse{RawOutput: "test output"}
	respData, err := json.Marshal(resp)
	require.NoError(t, err)
	respPath := filepath.Join(responsesDir, "001-invoke.json")
	//nolint:gosec // G306: Test file with restricted permissions
	err = os.WriteFile(respPath, respData, 0644)
	require.NoError(t, err)

	// Make file unreadable
	err = os.Chmod(respPath, 0000)
	require.NoError(t, err)
	// Ensure we restore permissions for cleanup
	//nolint:gosec // G302: Restore file permissions for cleanup after test
	defer func() { _ = os.Chmod(respPath, 0644) }()

	// Call Invoke
	ctx := context.Background()
	cfg := ai.InvokeConfig{
		Tool:   "test-tool",
		Prompt: "test prompt",
	}
	result, err := mock.Invoke(ctx, cfg)
	require.Error(t, err)
	assert.Nil(t, result)

	// Verify it's a MockInfraError with read error details
	var mockErr *MockInfraError
	require.True(t, errors.As(err, &mockErr))
	assert.Contains(t, mockErr.Message, "failed to read response file")
}
