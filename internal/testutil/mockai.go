// Package testutil provides testing utilities for mocking AI invocations in tests.
//
// This package offers two approaches for mocking AI tool behavior:
//
//  1. In-process mocking (MockInvokeAI, MockInvokeAIWithQueue): Fast and simple,
//     replaces ai.InvokeAI globally. Ideal for unit tests that don't need to test
//     command execution, argument parsing, or subprocess behavior.
//
//  2. Binary mocking (MockAITool): Compiles a real binary controlled by environment
//     variables. Tests the full execution path including command invocation, flag
//     parsing, and file I/O. Slower but more realistic for integration tests.
//
// Thread Safety: In-process mocks replace a global function and are NOT safe for
// parallel test execution. Use t.Setenv() or ensure tests run sequentially. Binary
// mocks are safer but still share global environment variables.
//
// Example (in-process):
//
//	cleanup := testutil.MockInvokeAI("mock response", nil)
//	defer cleanup()
//	result, _ := ai.InvokeAI(context.Background(), ai.InvokeConfig{Prompt: "test"})
//	fmt.Println(result.RawOutput) // "mock response"
//
// Example (binary):
//
//	mock := testutil.NewMockAITool(t, `{"success": true}`)
//	cfg := mock.InvokeConfig(t, "test prompt")
//	result, _ := ai.InvokeAI(context.Background(), cfg)
package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abenz1267/muster/internal/ai"
)

// invokeCounter tracks the number of times the mock AI has been invoked.
var invokeCounter atomic.Int32

// mockBinaryOnce ensures the mock binary is compiled only once.
var mockBinaryOnce sync.Once

// mockBinaryPath stores the path to the compiled mock binary.
var mockBinaryPath string

// mockBinaryErr stores any error that occurred during compilation.
var mockBinaryErr error

// MockInvokeAI replaces ai.InvokeAI with a mock implementation that returns
// the specified response and error. Returns a cleanup function that restores
// the original ai.InvokeAI function.
//
// Thread Safety: NOT safe for concurrent mock installation - only the counter
// uses atomics. The global ai.InvokeAI function replacement itself causes data
// races when multiple goroutines call MockInvokeAI() concurrently. Tests using
// MockInvokeAI should not run in parallel with t.Parallel(). The returned
// cleanup function must be called to restore the original implementation.
//
// The mock increments a global counter accessible via GetInvokeCount(). Use
// ResetInvokeCount() in test setup to ensure a clean state.
//
// Usage:
//
//	cleanup := testutil.MockInvokeAI("mock response", nil)
//	defer cleanup()
//	result, _ := ai.InvokeAI(context.Background(), ai.InvokeConfig{Prompt: "test"})
//	assert.Equal(t, "mock response", result.RawOutput)
func MockInvokeAI(response string, err error) func() {
	original := ai.InvokeAI

	ai.InvokeAI = func(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		invokeCounter.Add(1)
		if err != nil {
			return nil, err
		}
		return &ai.InvokeResult{
			RawOutput: response,
		}, nil
	}

	return func() {
		ai.InvokeAI = original
	}
}

// MockInvokeAIWithQueue replaces ai.InvokeAI with a mock implementation that
// returns different responses on successive calls. Each call consumes one
// response from the queue. If the queue is exhausted, subsequent calls return
// the last response. Returns a cleanup function that restores the original
// ai.InvokeAI function.
//
// Thread Safety: NOT safe for concurrent mock installation - only the counter
// uses atomics. The global ai.InvokeAI function replacement itself causes data
// races when multiple goroutines call MockInvokeAIWithQueue() concurrently.
// Tests using MockInvokeAIWithQueue should not run in parallel with t.Parallel().
// The returned cleanup function must be called to restore the original
// implementation.
//
// Queue Behavior: The mock tracks calls using an atomic counter. Once the queue
// is exhausted, all further calls return the last MockResponse in the slice.
// An empty queue defaults to returning empty string with no error.
//
// Usage:
//
//	cleanup := testutil.MockInvokeAIWithQueue(
//	    testutil.MockResponse{Response: "first", Err: nil},
//	    testutil.MockResponse{Response: "second", Err: nil},
//	)
//	defer cleanup()
//	result1, _ := ai.InvokeAI(cfg) // returns "first"
//	result2, _ := ai.InvokeAI(cfg) // returns "second"
//	result3, _ := ai.InvokeAI(cfg) // returns "second" (repeats last)
func MockInvokeAIWithQueue(responses ...MockResponse) func() {
	if len(responses) == 0 {
		responses = []MockResponse{{Response: "", Err: nil}}
	}

	original := ai.InvokeAI
	var callIndex atomic.Int32

	ai.InvokeAI = func(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
		invokeCounter.Add(1)
		idx := int(callIndex.Add(1)) - 1

		// Use last response if queue is exhausted
		if idx >= len(responses) {
			idx = len(responses) - 1
		}

		resp := responses[idx]
		if resp.Err != nil {
			return nil, resp.Err
		}
		return &ai.InvokeResult{
			RawOutput: resp.Response,
		}, nil
	}

	return func() {
		ai.InvokeAI = original
	}
}

// MockResponse represents a response to be returned by MockInvokeAIWithQueue.
// Each MockResponse corresponds to one AI invocation in the sequence.
type MockResponse struct {
	Response string // The raw output to return in ai.InvokeResult.RawOutput
	Err      error  // The error to return, or nil for success
}

// GetInvokeCount returns the number of times the mock AI has been invoked
// since the last call to ResetInvokeCount. This count is shared across both
// MockInvokeAI and MockInvokeAIWithQueue.
//
// Thread Safety: Uses atomic operations and is safe for concurrent reads,
// but should only be called from the same test that installed the mock.
func GetInvokeCount() int32 {
	return invokeCounter.Load()
}

// ResetInvokeCount resets the invocation counter to zero. Call this in test
// setup (e.g., in a defer after cleanup) to ensure isolated test state.
//
// Thread Safety: Uses atomic operations and is safe for concurrent access,
// but tests should not run in parallel when using global mocks.
func ResetInvokeCount() {
	invokeCounter.Store(0)
}

// compileMockBinary compiles a mock AI tool binary that can be used for testing.
// The binary is compiled once and cached in the system temp directory.
// The mock binary accepts --print, --plugin-dir, and --model flags, reads
// skills/SKILL.md from the plugin directory, and outputs JSON based on
// environment variables:
//   - MOCK_RESPONSE: JSON to output (default: {"success": true})
//   - MOCK_EXIT_CODE: Exit code to return (default: 0)
//   - MOCK_STDERR: Text to write to stderr
//   - MOCK_DELAY_MS: Milliseconds to sleep before exiting
//
// Temp Directory Cleanup: This function creates a temporary directory that is
// never explicitly cleaned up because the binary is process-scoped (compiled
// once via sync.Once) and Go's testing package doesn't provide a process-level
// cleanup hook. The OS will clean up these muster-mock-ai-* directories on
// reboot or via periodic temp cleanup. In practice, this is acceptable since
// the binary is only compiled once per test run.
func compileMockBinary() (string, error) {
	mockBinaryOnce.Do(func() {
		// Create temp directory for compilation
		// Note: This directory is not cleaned up explicitly. See function documentation.
		tmpDir, err := os.MkdirTemp("", "muster-mock-ai-")
		if err != nil {
			mockBinaryErr = fmt.Errorf("failed to create temp directory: %w", err)
			return
		}

		// Mock binary source code
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
		response = ` + "`{\"success\": true}`" + `
	}
	fmt.Print(response)
}
`

		// Write source file
		sourcePath := filepath.Join(tmpDir, "mock-ai-tool.go")
		//nolint:gosec // G306: Test file permissions are acceptable
		if err := os.WriteFile(sourcePath, []byte(mockSource), 0644); err != nil {
			mockBinaryErr = fmt.Errorf("failed to write mock tool source: %w", err)
			return
		}

		// Compile binary
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

// MockAITool represents a mock AI tool for testing. It provides a compiled
// binary that mimics a real AI tool, controlled via environment variables.
// This enables testing the full command execution path, including argument
// parsing, file I/O, and subprocess behavior.
//
// The mock binary is compiled once per test run using sync.Once and cached
// in a temp directory. It accepts --print, --plugin-dir, and --model flags,
// reads skills/SKILL.md from the plugin directory, and produces output
// controlled by environment variables set in InvokeConfig().
//
// Use NewMockAITool to create instances, then configure behavior with
// WithError() and WithDelay() modifiers.
type MockAITool struct {
	t        *testing.T
	path     string
	response string
	exitCode int
	stderr   string
	delay    time.Duration
}

// NewMockAITool creates a new MockAITool configured to return the given response.
// The mock tool binary is compiled once using sync.Once and cached across all
// tests in the same process. Compilation happens on first call and subsequent
// calls reuse the cached binary.
//
// The response should be valid JSON if testing JSON parsing. The mock defaults
// to exit code 0, no stderr, and no delay. Use WithError() and WithDelay() to
// modify behavior.
//
// Compilation errors cause the test to fail via t.Fatalf(). Returns nil after
// fatally failing (unreachable in practice).
//
// Usage:
//
//	mock := testutil.NewMockAITool(t, `{"result": "success"}`)
//	cfg := mock.InvokeConfig(t, "test prompt")
//	result, err := ai.InvokeAI(context.Background(), cfg)
func NewMockAITool(t *testing.T, response string) *MockAITool {
	t.Helper()

	path, err := compileMockBinary()
	if err != nil {
		t.Fatalf("failed to compile mock AI tool: %v", err)
		return nil
	}

	return &MockAITool{
		t:        t,
		path:     path,
		response: response,
		exitCode: 0,
		stderr:   "",
		delay:    0,
	}
}

// WithError returns a copy of the MockAITool configured to exit with the
// given error code and stderr message. This allows testing error handling
// paths without creating a new mock binary.
//
// The exit code will be set via MOCK_EXIT_CODE and stderr via MOCK_STDERR
// when InvokeConfig() is called. The mock exits after writing stderr, so
// response output may not be produced depending on timing.
//
// Usage:
//
//	mock := testutil.NewMockAITool(t, "").WithError(1, "connection failed")
//	cfg := mock.InvokeConfig(t, "test")
//	_, err := ai.InvokeAI(context.Background(), cfg) // returns error with stderr in output
func (m *MockAITool) WithError(exitCode int, stderr string) *MockAITool {
	copy := *m
	copy.exitCode = exitCode
	copy.stderr = stderr
	return &copy
}

// WithDelay returns a copy of the MockAITool configured to sleep for the
// given duration before exiting. This allows testing timeout behavior,
// cancellation, and concurrent execution scenarios.
//
// The delay is set via MOCK_DELAY_MS when InvokeConfig() is called. The mock
// sleeps before checking exit codes or writing output, so the delay happens
// regardless of success or failure paths.
//
// Usage:
//
//	mock := testutil.NewMockAITool(t, "slow").WithDelay(100 * time.Millisecond)
//	cfg := mock.InvokeConfig(t, "test")
//	start := time.Now()
//	ai.InvokeAI(context.Background(), cfg)
//	assert.True(t, time.Since(start) >= 100*time.Millisecond)
func (m *MockAITool) WithDelay(duration time.Duration) *MockAITool {
	copy := *m
	copy.delay = duration
	return &copy
}

// Path returns the absolute path to the compiled mock binary. This can be
// used directly in ai.InvokeConfig.Tool or passed to external commands.
// The binary remains valid for the lifetime of the test process.
func (m *MockAITool) Path() string {
	return m.path
}

// InvokeConfig returns a pre-configured ai.InvokeConfig ready to use with
// the mock tool. The config includes the tool path and prompt.
//
// Environment Variables: This method sets MOCK_RESPONSE, MOCK_EXIT_CODE,
// MOCK_STDERR, and MOCK_DELAY_MS environment variables based on the mock's
// configuration using t.Setenv() for automatic cleanup on test completion.
//
// Thread Safety: Uses t.Setenv() which provides automatic cleanup and is safe
// for parallel tests as long as each test has its own *testing.T instance.
//
// Usage:
//
//	mock := testutil.NewMockAITool(t, `{"status": "ok"}`)
//	cfg := mock.InvokeConfig(t, "analyze this code")
//	result, err := ai.InvokeAI(context.Background(), cfg)
func (m *MockAITool) InvokeConfig(t *testing.T, prompt string) ai.InvokeConfig {
	t.Helper()

	// Set environment variables to control mock behavior
	// t.Setenv automatically handles cleanup
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

	return ai.InvokeConfig{
		Tool:   m.path,
		Prompt: prompt,
	}
}
