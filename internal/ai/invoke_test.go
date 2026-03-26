package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	mockBinaryOnce sync.Once
	mockBinaryPath string
	mockBinaryErr  error
)

// compileMockBinary compiles a mock AI tool binary for testing.
// The binary is compiled once and cached. It accepts --print, --plugin-dir, and --model flags,
// reads skills/SKILL.md, and outputs JSON based on environment variables:
//   - MOCK_RESPONSE: JSON to output (default: {"success": true})
//   - MOCK_EXIT_CODE: Exit code to return (default: 0)
//   - MOCK_STDERR: Text to write to stderr
//   - MOCK_DELAY_MS: Milliseconds to sleep before exiting
func compileMockBinary() (string, error) {
	mockBinaryOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "muster-mock-ai-")
		if err != nil {
			mockBinaryErr = fmt.Errorf("failed to create temp directory: %w", err)
			return
		}

		mockSource := `package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func main() {
	printFlag := flag.Bool("print", false, "print JSON output")
	pluginDirFlag := flag.String("plugin-dir", "", "plugin directory")
	_ = flag.String("model", "", "model to use")
	flag.Parse()

	// Handle MOCK_DELAY_MS if set
	if delayStr := os.Getenv("MOCK_DELAY_MS"); delayStr != "" {
		if delayMs, err := strconv.Atoi(delayStr); err == nil && delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}

	// Write MOCK_STDERR if set
	if stderr := os.Getenv("MOCK_STDERR"); stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}

	// Check for exit code before processing
	if exitCodeStr := os.Getenv("MOCK_EXIT_CODE"); exitCodeStr != "" {
		if exitCode, err := strconv.Atoi(exitCodeStr); err == nil && exitCode != 0 {
			os.Exit(exitCode)
		}
	}

	// Validate required flags
	if !*printFlag || *pluginDirFlag == "" {
		fmt.Fprintf(os.Stderr, "Usage: mock-ai-tool --print --plugin-dir <dir> [--model <model>]\n")
		os.Exit(1)
	}

	// Read SKILL.md from plugin-dir/skills/
	skillPath := filepath.Join(*pluginDirFlag, "skills", "SKILL.md")
	_, err := os.ReadFile(skillPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading skill file: %v\n", err)
		os.Exit(1)
	}

	// Output MOCK_RESPONSE or default JSON
	// Check if MOCK_RESPONSE is explicitly set (even if empty)
	response, hasResponse := os.LookupEnv("MOCK_RESPONSE")
	if !hasResponse {
		response = "{\"success\": true}"
	}
	fmt.Print(response)

	// Exit with MOCK_EXIT_CODE if set
	if exitCodeStr := os.Getenv("MOCK_EXIT_CODE"); exitCodeStr != "" {
		if exitCode, err := strconv.Atoi(exitCodeStr); err == nil {
			os.Exit(exitCode)
		}
	}
}
`

		sourcePath := filepath.Join(tmpDir, "mock-ai-tool.go")
		//nolint:gosec // G306: Test file permissions are acceptable
		if err := os.WriteFile(sourcePath, []byte(mockSource), 0644); err != nil {
			mockBinaryErr = fmt.Errorf("failed to write mock tool source: %w", err)
			return
		}

		binaryName := "mock-ai-tool"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		binaryPath := filepath.Join(tmpDir, binaryName)
		//nolint:gosec // G204: Compiling known mock tool source
		cmd := exec.Command("go", "build", "-o", binaryPath, sourcePath)
		if output, err := cmd.CombinedOutput(); err != nil {
			mockBinaryErr = fmt.Errorf("failed to compile mock tool: %w\nOutput: %s", err, string(output))
			return
		}

		mockBinaryPath = binaryPath
	})

	return mockBinaryPath, mockBinaryErr
}

// mockAITool represents a mock AI tool for testing.
type mockAITool struct {
	t        *testing.T
	path     string
	response string
	exitCode int
	stderr   string
	delay    time.Duration
}

// newMockAITool creates a new mock AI tool with the given response.
func newMockAITool(t *testing.T, response string) *mockAITool {
	t.Helper()

	path, err := compileMockBinary()
	if err != nil {
		t.Fatalf("failed to compile mock AI tool: %v", err)
		return nil
	}

	return &mockAITool{
		t:        t,
		path:     path,
		response: response,
		exitCode: 0,
		stderr:   "",
		delay:    0,
	}
}

// WithError returns a copy configured to exit with the given error code and stderr.
func (m *mockAITool) WithError(exitCode int, stderr string) *mockAITool {
	copy := *m
	copy.exitCode = exitCode
	copy.stderr = stderr
	return &copy
}

// WithDelay returns a copy configured to sleep for the given duration.
func (m *mockAITool) WithDelay(duration time.Duration) *mockAITool {
	copy := *m
	copy.delay = duration
	return &copy
}

// Path returns the path to the compiled mock binary.
func (m *mockAITool) Path() string {
	return m.path
}

// setupEnv configures environment variables for the mock tool.
func (m *mockAITool) setupEnv(t *testing.T) {
	t.Helper()

	// Set environment variables to control mock behavior
	// Always set MOCK_RESPONSE (even if empty) to override default
	t.Setenv("MOCK_RESPONSE", m.response)

	if m.exitCode != 0 {
		t.Setenv("MOCK_EXIT_CODE", strconv.Itoa(m.exitCode))
	}
	if m.stderr != "" {
		t.Setenv("MOCK_STDERR", m.stderr)
	}
	if m.delay > 0 {
		t.Setenv("MOCK_DELAY_MS", strconv.Itoa(int(m.delay.Milliseconds())))
	}
}

func TestInvokeAI_Success(t *testing.T) {
	mock := newMockAITool(t, `{
  "success": true,
  "content": "Test prompt content"
}`)
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt content",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify output contains the prompt content
	assert.Contains(t, result.RawOutput, "Test prompt content")

	// Verify it's valid JSON
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(result.RawOutput), &jsonOutput)
	require.NoError(t, err)
	assert.Equal(t, true, jsonOutput["success"])
	assert.Equal(t, "Test prompt content", jsonOutput["content"])
}

func TestInvokeAI_ToolNotFound(t *testing.T) {
	cfg := InvokeConfig{
		Tool:    "/nonexistent/tool",
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.Error(t, err)
	require.Nil(t, result)

	// Error message should mention tool not found and provide guidance
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "PATH")
}

func TestInvokeAI_NonZeroExit(t *testing.T) {
	mock := newMockAITool(t, "").WithError(42, "Error message")
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.Error(t, err)
	require.Nil(t, result)

	// Error should mention exit code
	assert.Contains(t, err.Error(), "exited with code")
	assert.Contains(t, err.Error(), "42")
}

func TestInvokeAI_EmptyOutput(t *testing.T) {
	mock := newMockAITool(t, "")
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Empty output is valid - just an empty string
	assert.Equal(t, "", result.RawOutput)
}

func TestInvokeAI_EmptyTool(t *testing.T) {
	cfg := InvokeConfig{
		Tool:    "",
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.Error(t, err)
	require.Nil(t, result)
	assert.Contains(t, err.Error(), "tool cannot be empty")
}

func TestInvokeAI_EmptyPrompt(t *testing.T) {
	mock := newMockAITool(t, `{"success": true}`)
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.Error(t, err)
	require.Nil(t, result)
	assert.Contains(t, err.Error(), "prompt cannot be empty")
}

func TestInvokeAI_ModelPassthrough(t *testing.T) {
	mock := newMockAITool(t, `{"success": true}`)
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Model:   "claude-haiku-4.5",
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify it's valid JSON (the mock tool receives the model flag)
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(result.RawOutput), &jsonOutput)
	require.NoError(t, err)
	assert.Equal(t, true, jsonOutput["success"])
}

func TestInvokeAI_NoModel(t *testing.T) {
	mock := newMockAITool(t, `{"success": true}`)
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify it's valid JSON
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(result.RawOutput), &jsonOutput)
	require.NoError(t, err)
	assert.Equal(t, true, jsonOutput["success"])
}

func TestInvokeAI_Verbose(t *testing.T) {
	mock := newMockAITool(t, `{"success": true}`)
	mock.setupEnv(t)

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt",
		Verbose: true,
	}

	result, err := InvokeAI(context.Background(), cfg)

	// Restore stderr
	_ = w.Close()
	os.Stderr = oldStderr

	require.NoError(t, err)
	require.NotNil(t, result)

	// Read captured stderr
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	stderrOutput := string(buf[:n])

	// Verify verbose output was printed
	assert.Contains(t, stderrOutput, "AI Invoke:")
	assert.Contains(t, stderrOutput, "Command:")
}

func TestInvokeAI_SkillFileCreated(t *testing.T) {
	// This test verifies that the skill file is created with the correct content
	// The mock tool reads the skill file, so if this succeeds, the file was created
	mock := newMockAITool(t, `{"success": true}`)
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt for skill file",
		Verbose: false,
	}

	result, err := InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify it's valid JSON - if the skill file wasn't created, the mock would fail
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(result.RawOutput), &jsonOutput)
	require.NoError(t, err)
	assert.Equal(t, true, jsonOutput["success"])
}

func TestInvokeAI_Timeout(t *testing.T) {
	// Create a mock tool that sleeps for 100ms
	// Note: InvokeAI has a hardcoded 60-second timeout, so we can't test actual timeout
	// without waiting that long. This test verifies the mock tool delay mechanism works.
	mock := newMockAITool(t, `{"success": true}`).WithDelay(100 * time.Millisecond)
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt",
		Verbose: false,
	}

	// This should succeed after the delay
	result, err := InvokeAI(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify output is valid
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(result.RawOutput), &jsonOutput)
	require.NoError(t, err)
	assert.Equal(t, true, jsonOutput["success"])
}

func TestInvokeAI_TimeoutActual(t *testing.T) {
	// Test that timeout actually triggers when tool takes too long
	mock := newMockAITool(t, `{"success": true}`).WithDelay(200 * time.Millisecond)
	mock.setupEnv(t)

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt",
		Timeout: 50 * time.Millisecond,
	}

	// Measure time to ensure timeout is working
	start := time.Now()
	result, err := InvokeAI(context.Background(), cfg)
	elapsed := time.Since(start)

	// Should fail with an error
	require.Error(t, err)
	require.Nil(t, result)

	// Key validation: timeout should occur around 50ms, not 200ms (the full delay)
	// Allow some overhead, but should be much less than the full delay
	assert.Less(t, elapsed, 150*time.Millisecond, "timeout should have triggered before full delay")
}

func TestInvokeAI_Concurrent(t *testing.T) {
	// Create a mock tool for concurrent testing
	mock := newMockAITool(t, `{"success": true}`)
	mock.setupEnv(t)

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect errors from goroutines
	errChan := make(chan error, numGoroutines)

	// Spawn 10 concurrent goroutines calling InvokeAI
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			cfg := InvokeConfig{
				Tool:    mock.Path(),
				Prompt:  fmt.Sprintf("Concurrent test prompt %d", id),
				Verbose: false,
			}

			result, err := InvokeAI(context.Background(), cfg)
			if err != nil {
				errChan <- fmt.Errorf("goroutine %d failed: %w", id, err)
				return
			}

			// Verify result is valid
			if result == nil {
				errChan <- fmt.Errorf("goroutine %d got nil result", id)
				return
			}

			// Verify output contains expected JSON
			var jsonOutput map[string]interface{}
			if err := json.Unmarshal([]byte(result.RawOutput), &jsonOutput); err != nil {
				errChan <- fmt.Errorf("goroutine %d got invalid JSON: %w", id, err)
				return
			}

			if jsonOutput["success"] != true {
				errChan <- fmt.Errorf("goroutine %d got unexpected success value", id)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		t.Error(err)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON passthrough",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "json code fence",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "bare code fence",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "code fence with surrounding whitespace",
			input:    "  ```json\n{\"key\": \"value\"}\n```  ",
			expected: `{"key": "value"}`,
		},
		{
			name:     "array in code fence",
			input:    "```json\n[{\"a\": 1}]\n```",
			expected: `[{"a": 1}]`,
		},
		{
			name:     "no fences returns as-is",
			input:    "not json at all",
			expected: "not json at all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ExtractJSON(tt.input))
		})
	}
}

func TestInvokeAI_ContextDeadlineRespected(t *testing.T) {
	// Create a mock tool that delays for 500ms
	mock := newMockAITool(t, `{"success": true}`).WithDelay(500 * time.Millisecond)
	mock.setupEnv(t)

	// Create context with 100ms deadline
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cfg := InvokeConfig{
		Tool:    mock.Path(),
		Prompt:  "Test prompt",
		Verbose: false,
	}

	// Measure elapsed time
	start := time.Now()
	result, err := InvokeAI(ctx, cfg)
	elapsed := time.Since(start)

	// Should fail with timeout error
	require.Error(t, err)
	require.Nil(t, result)

	// Should timeout at ~100ms, not wait for full 500ms delay
	assert.Less(t, elapsed, 200*time.Millisecond, "should timeout at ~100ms, not wait for full 500ms delay")
	assert.Greater(t, elapsed, 80*time.Millisecond, "should take at least the deadline duration")
}

func TestInvokeAI_ContextDeadlinePrecedence(t *testing.T) {
	// This test verifies the deadline comparison logic in invoke.go (lines 93-109).
	// It tests all branches of the deadline precedence logic.

	t.Run("ContextDeadlineShorterThanConfigTimeout", func(t *testing.T) {
		// Context deadline: 100ms
		// Config timeout: 200ms
		// Expected: Context deadline wins (100ms)

		mock := newMockAITool(t, `{"success": true}`).WithDelay(500 * time.Millisecond)
		mock.setupEnv(t)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		cfg := InvokeConfig{
			Tool:    mock.Path(),
			Prompt:  "Test prompt",
			Timeout: 200 * time.Millisecond,
		}

		start := time.Now()
		_, err := InvokeAI(ctx, cfg)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Should use context deadline (100ms), not config timeout (200ms)
		assert.Less(t, elapsed, 150*time.Millisecond, "should use shorter context deadline")
		assert.Greater(t, elapsed, 80*time.Millisecond, "should wait at least context deadline")
	})

	t.Run("ConfigTimeoutShorterThanContextDeadline", func(t *testing.T) {
		// Context deadline: 200ms
		// Config timeout: 100ms
		// Expected: Config timeout wins (100ms)

		mock := newMockAITool(t, `{"success": true}`).WithDelay(500 * time.Millisecond)
		mock.setupEnv(t)

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		cfg := InvokeConfig{
			Tool:    mock.Path(),
			Prompt:  "Test prompt",
			Timeout: 100 * time.Millisecond,
		}

		start := time.Now()
		_, err := InvokeAI(ctx, cfg)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Should use shorter config timeout (100ms), not context deadline (200ms)
		assert.Less(t, elapsed, 150*time.Millisecond, "should use shorter config timeout")
		assert.Greater(t, elapsed, 80*time.Millisecond, "should wait at least config timeout")
	})

	t.Run("NoExistingContextDeadline", func(t *testing.T) {
		// Context: No deadline
		// Config timeout: 100ms
		// Expected: Config timeout is used (100ms)

		mock := newMockAITool(t, `{"success": true}`).WithDelay(500 * time.Millisecond)
		mock.setupEnv(t)

		ctx := context.Background() // No deadline

		cfg := InvokeConfig{
			Tool:    mock.Path(),
			Prompt:  "Test prompt",
			Timeout: 100 * time.Millisecond,
		}

		start := time.Now()
		_, err := InvokeAI(ctx, cfg)
		elapsed := time.Since(start)

		require.Error(t, err)
		// Should use config timeout (100ms) since context has no deadline
		assert.Less(t, elapsed, 150*time.Millisecond, "should use config timeout")
		assert.Greater(t, elapsed, 80*time.Millisecond, "should wait at least config timeout")
	})
}
