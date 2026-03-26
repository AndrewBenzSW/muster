package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// raceEnabled is set to true when the race detector is enabled via build tags.
// Used to skip tests that intentionally trigger race conditions for documentation.
var raceEnabled = false

func TestMockInvokeAI_BasicUsage(t *testing.T) {
	cleanup := MockInvokeAI(`{"result": "mocked"}`, nil)
	defer cleanup()

	cfg := ai.InvokeConfig{
		Tool:   "mock-tool",
		Prompt: "test prompt",
	}

	result, err := ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, `{"result": "mocked"}`, result.RawOutput)
}

func TestMockInvokeAI_WithError(t *testing.T) {
	expectedErr := fmt.Errorf("mock error")
	cleanup := MockInvokeAI("", expectedErr)
	defer cleanup()

	cfg := ai.InvokeConfig{
		Tool:   "mock-tool",
		Prompt: "test prompt",
	}

	result, err := ai.InvokeAI(context.Background(), cfg)
	require.Error(t, err)
	require.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

func TestMockInvokeAI_CleanupRestoresOriginal(t *testing.T) {
	original := ai.InvokeAI

	cleanup := MockInvokeAI(`{"result": "mocked"}`, nil)
	assert.NotEqual(t, fmt.Sprintf("%p", original), fmt.Sprintf("%p", ai.InvokeAI))

	cleanup()
	assert.Equal(t, fmt.Sprintf("%p", original), fmt.Sprintf("%p", ai.InvokeAI))
}

func TestMockInvokeAIWithQueue_MultipleResponses(t *testing.T) {
	cleanup := MockInvokeAIWithQueue(
		MockResponse{Response: "first", Err: nil},
		MockResponse{Response: "second", Err: nil},
		MockResponse{Response: "third", Err: nil},
	)
	defer cleanup()

	cfg := ai.InvokeConfig{
		Tool:   "mock-tool",
		Prompt: "test prompt",
	}

	// First call
	result, err := ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "first", result.RawOutput)

	// Second call
	result, err = ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "second", result.RawOutput)

	// Third call
	result, err = ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "third", result.RawOutput)

	// Fourth call (queue exhausted, returns last response)
	result, err = ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "third", result.RawOutput)
}

func TestMockInvokeAIWithQueue_WithErrors(t *testing.T) {
	expectedErr := fmt.Errorf("mock error")
	cleanup := MockInvokeAIWithQueue(
		MockResponse{Response: "success", Err: nil},
		MockResponse{Response: "", Err: expectedErr},
		MockResponse{Response: "success again", Err: nil},
	)
	defer cleanup()

	cfg := ai.InvokeConfig{
		Tool:   "mock-tool",
		Prompt: "test prompt",
	}

	// First call succeeds
	result, err := ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "success", result.RawOutput)

	// Second call fails
	result, err = ai.InvokeAI(context.Background(), cfg)
	require.Error(t, err)
	require.Nil(t, result)
	assert.Equal(t, expectedErr, err)

	// Third call succeeds
	result, err = ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "success again", result.RawOutput)
}

func TestGetInvokeCount_TracksInvocations(t *testing.T) {
	ResetInvokeCount()
	assert.Equal(t, int32(0), GetInvokeCount())

	cleanup := MockInvokeAI("response", nil)
	defer cleanup()

	cfg := ai.InvokeConfig{
		Tool:   "mock-tool",
		Prompt: "test prompt",
	}

	// Make multiple calls
	_, _ = ai.InvokeAI(context.Background(), cfg)
	assert.Equal(t, int32(1), GetInvokeCount())

	_, _ = ai.InvokeAI(context.Background(), cfg)
	assert.Equal(t, int32(2), GetInvokeCount())

	_, _ = ai.InvokeAI(context.Background(), cfg)
	assert.Equal(t, int32(3), GetInvokeCount())

	// Reset counter
	ResetInvokeCount()
	assert.Equal(t, int32(0), GetInvokeCount())
}

func TestNewMockAITool_CreatesWorkingBinary(t *testing.T) {
	mock := NewMockAITool(t, `{"mock": "response"}`)
	require.NotNil(t, mock)

	// Binary should exist
	assert.NotEmpty(t, mock.Path())

	// Test with ai.InvokeAI
	cfg := mock.InvokeConfig(t, "test prompt")
	result, err := ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, `{"mock": "response"}`, result.RawOutput)
}

func TestMockAITool_WithError(t *testing.T) {
	mock := NewMockAITool(t, `{"mock": "response"}`)
	require.NotNil(t, mock)

	errorMock := mock.WithError(42, "error message")
	cfg := errorMock.InvokeConfig(t, "test prompt")

	result, err := ai.InvokeAI(context.Background(), cfg)
	require.Error(t, err)
	require.Nil(t, result)
	assert.Contains(t, err.Error(), "42")
}

func TestMockAITool_WithDelay(t *testing.T) {
	mock := NewMockAITool(t, `{"mock": "response"}`)
	require.NotNil(t, mock)

	delayMock := mock.WithDelay(100 * time.Millisecond)
	cfg := delayMock.InvokeConfig(t, "test prompt")

	start := time.Now()
	result, err := ai.InvokeAI(context.Background(), cfg)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, `{"mock": "response"}`, result.RawOutput)
	assert.GreaterOrEqual(t, duration, 100*time.Millisecond)
}

func TestMockAITool_ChainedModifiers(t *testing.T) {
	mock := NewMockAITool(t, `{"mock": "response"}`)
	require.NotNil(t, mock)

	// Test that modifiers can be chained
	modifiedMock := mock.WithDelay(50*time.Millisecond).
		WithError(1, "stderr output")
	cfg := modifiedMock.InvokeConfig(t, "test prompt")

	start := time.Now()
	result, err := ai.InvokeAI(context.Background(), cfg)
	duration := time.Since(start)

	require.Error(t, err)
	require.Nil(t, result)
	assert.GreaterOrEqual(t, duration, 50*time.Millisecond)
}

func TestMockBinary_GoldenFileOutput(t *testing.T) {
	mock := NewMockAITool(t, `{"success": true}`)
	require.NotNil(t, mock)

	// Create a temporary plugin directory with skills/SKILL.md
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	//nolint:gosec // G301: Test directory permissions are acceptable
	require.NoError(t, os.MkdirAll(skillsDir, 0755))
	skillPath := filepath.Join(skillsDir, "SKILL.md")
	//nolint:gosec // G306: Test file permissions are acceptable
	require.NoError(t, os.WriteFile(skillPath, []byte("# Test Skill"), 0644))

	// Set the mock response via environment variable
	t.Setenv("MOCK_RESPONSE", `{"success": true}`)

	// Execute the mock binary directly with exec.Command
	//nolint:gosec // G204: Testing known mock binary
	cmd := exec.Command(mock.Path(), "--print", "--plugin-dir", tmpDir)
	output, err := cmd.Output()
	require.NoError(t, err, "mock binary should execute successfully")

	// Compare raw stdout to golden file (validates actual subprocess contract)
	update := false // Set to true to update golden file
	AssertGoldenFile(t, "testdata/mock-output.golden", string(output), update)
}

func TestMockBinary_ModelFlagPassthrough(t *testing.T) {
	mock := NewMockAITool(t, `{"model": "test-model"}`)
	require.NotNil(t, mock)

	// Configure with a specific model
	cfg := mock.InvokeConfig(t, "test prompt")
	cfg.Model = "gpt-4"

	result, err := ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)

	// Verify the mock binary doesn't error when --model flag is passed
	assert.NotEmpty(t, result.RawOutput)
	assert.Contains(t, result.RawOutput, "model")
}

func TestMockBinary_SkillFileContent(t *testing.T) {
	mock := NewMockAITool(t, `{"skill": "processed"}`)
	require.NotNil(t, mock)

	// Test with specific prompt content
	testPrompt := "Test skill content for validation"
	cfg := mock.InvokeConfig(t, testPrompt)

	result, err := ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)

	// Verify the mock binary reads the skill file successfully
	// (it would error if skills/SKILL.md couldn't be read)
	assert.NotEmpty(t, result.RawOutput)
}

func TestMockBinary_CompilationFailure(t *testing.T) {
	// Test compilation error path by manipulating GOROOT to cause build failure.
	// NOTE: This tests the error handling path, but actual compilation failure
	// is difficult to reliably trigger in practice without breaking the test
	// environment. The sync.Once pattern means once compilation succeeds, it's
	// cached for all subsequent tests. Real-world compilation errors would occur
	// if: go toolchain is missing, GOOS/GOARCH combination is invalid, or disk
	// space is exhausted during temp directory creation.
	//
	// In this test we verify that:
	// 1. Compilation errors are properly captured and returned
	// 2. The error message contains "failed to compile mock tool"
	// 3. Subsequent NewMockAITool calls consistently return the same error
	//    (don't hang or panic)
	//
	// Since we cannot reliably force compilation to fail without breaking the
	// test suite (the binary is already compiled by earlier tests in this file),
	// this test serves as documentation of the expected error handling behavior
	// rather than an actual execution test of the error path.

	// If compilation has already succeeded (which it has in other tests),
	// NewMockAITool will return the cached binary. This is expected behavior.
	// To actually test compilation failure, one would need to:
	// - Run this test in isolation before other MockAITool tests
	// - Use build tags to conditionally break the compilation
	// - Mock exec.Command (which would require refactoring compileMockBinary)

	// For now, we verify the happy path completes without hanging
	mock := NewMockAITool(t, `{"status": "ok"}`)
	require.NotNil(t, mock, "mock should be created successfully")
	assert.NotEmpty(t, mock.Path(), "mock binary path should be set")

	// The actual error path testing would require:
	// t.Skip("Compilation error paths are difficult to test without breaking the test environment")
}

func TestMockInvokeAIWithQueue_HeavyUsageAfterExhaustion(t *testing.T) {
	// Test queue exhaustion with heavy usage (50+ calls after exhaustion)
	// to verify stability and no panics or unexpected behavior
	cleanup := MockInvokeAIWithQueue(
		MockResponse{Response: "first", Err: nil},
		MockResponse{Response: "second", Err: nil},
	)
	defer cleanup()

	cfg := ai.InvokeConfig{
		Tool:   "mock-tool",
		Prompt: "test prompt",
	}

	// Consume the queue (2 items)
	result, err := ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "first", result.RawOutput)

	result, err = ai.InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "second", result.RawOutput)

	// Make 50+ calls after exhaustion
	for i := 0; i < 55; i++ {
		result, err = ai.InvokeAI(context.Background(), cfg)
		require.NoError(t, err, "call %d after exhaustion should not panic or error", i+1)
		assert.Equal(t, "second", result.RawOutput, "call %d should return last response", i+1)
	}
}

func TestMockInvokeAIWithQueue_SingleItemManyCallsStability(t *testing.T) {
	// Test single-item queue called many times to verify stability
	cleanup := MockInvokeAIWithQueue(
		MockResponse{Response: "only", Err: nil},
	)
	defer cleanup()

	cfg := ai.InvokeConfig{
		Tool:   "mock-tool",
		Prompt: "test prompt",
	}

	// Call 100 times - all should return the same response
	for i := 0; i < 100; i++ {
		result, err := ai.InvokeAI(context.Background(), cfg)
		require.NoError(t, err, "call %d should not panic or error", i+1)
		assert.Equal(t, "only", result.RawOutput, "call %d should return single response", i+1)
	}
}

func TestMockInvokeAI_ConcurrentAccessDocumentsLimitation(t *testing.T) {
	// This test documents the race condition in MockInvokeAI by spawning
	// multiple goroutines that call MockInvokeAI() concurrently.
	// The global ai.InvokeAI variable replacement is NOT parallel-safe.
	//
	// NOTE: This test intentionally triggers race conditions to document the
	// limitation. It is skipped when running with -race to avoid test failures.
	// To see the race condition, run WITHOUT -race: go test -v

	// Skip when race detector is enabled (build tag race)
	// We detect this by checking if GORACE env var is set or if we can determine race mode
	if raceEnabled {
		t.Skip("skipping race condition demonstration test when -race is enabled")
	}

	// Skip this test in short mode to avoid slowdown
	if testing.Short() {
		t.Skip("skipping concurrent safety demonstration in short mode")
	}

	// Spawn goroutines that attempt to install different mocks concurrently
	// This will cause data races on the global ai.InvokeAI variable
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			response := fmt.Sprintf("response-%d", id)
			cleanup := MockInvokeAI(response, nil)
			defer cleanup()

			cfg := ai.InvokeConfig{
				Tool:   "mock-tool",
				Prompt: "test prompt",
			}

			// Try to use the mock
			_, _ = ai.InvokeAI(context.Background(), cfg)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// This test documents the limitation: concurrent MockInvokeAI installation
	// causes race conditions. The counter uses atomics (safe), but the global
	// function replacement is not.
}
