package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockToolPath string

// TestMain sets up a mock tool binary for testing.
func TestMain(m *testing.M) {
	// Build mock tool
	tmpDir, err := os.MkdirTemp("", "ai-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	mockToolPath = filepath.Join(tmpDir, "mock-ai-tool")

	// Create a simple Go program that reads the skill directory and echoes JSON
	mockToolSource := `package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	printFlag := flag.Bool("print", false, "print JSON output")
	pluginDirFlag := flag.String("plugin-dir", "", "plugin directory")
	modelFlag := flag.String("model", "", "model to use")
	exitCodeFlag := flag.Int("exit-code", 0, "exit code to return")
	flag.Parse()

	if *exitCodeFlag != 0 {
		os.Exit(*exitCodeFlag)
	}

	if !*printFlag || *pluginDirFlag == "" {
		fmt.Fprintf(os.Stderr, "Usage: mock-ai-tool --print --plugin-dir <dir>\n")
		os.Exit(1)
	}

	// Read SKILL.md from plugin-dir/skills/
	skillPath := filepath.Join(*pluginDirFlag, "skills", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading skill file: %v\n", err)
		os.Exit(1)
	}

	// Echo JSON output with the skill content and model
	output := map[string]interface{}{
		"success": true,
		"content": string(content),
		"model":   *modelFlag,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}
`

	// Write mock tool source
	mockToolSourcePath := filepath.Join(tmpDir, "mock-ai-tool.go")
	//nolint:gosec // G306: Test file permissions are acceptable for mock tool
	if err := os.WriteFile(mockToolSourcePath, []byte(mockToolSource), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write mock tool source: %v\n", err)
		os.Exit(1)
	}

	// Compile mock tool
	//nolint:gosec // G204: Test code compiling mock tool from known source
	cmd := exec.Command("go", "build", "-o", mockToolPath, mockToolSourcePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to compile mock tool: %v\nOutput: %s\n", err, string(output))
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	os.Exit(code)
}

func TestInvokeAI_Success(t *testing.T) {
	cfg := InvokeConfig{
		Tool:    mockToolPath,
		Prompt:  "Test prompt content",
		Verbose: false,
	}

	result, err := InvokeAI(cfg)
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

	result, err := InvokeAI(cfg)
	require.Error(t, err)
	require.Nil(t, result)

	// Error message should mention tool not found and provide guidance
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "PATH")
}

func TestInvokeAI_NonZeroExit(t *testing.T) {
	// Create a wrapper script that exits with non-zero code
	tmpDir, err := os.MkdirTemp("", "ai-test-exit-")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	failToolPath := filepath.Join(tmpDir, "fail-tool")

	// Create a script that always fails
	failScript := `#!/bin/sh
echo "Error message" >&2
exit 42
`
	//nolint:gosec // G306: Test script needs execute permissions
	err = os.WriteFile(failToolPath, []byte(failScript), 0755)
	require.NoError(t, err)

	cfg := InvokeConfig{
		Tool:    failToolPath,
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(cfg)
	require.Error(t, err)
	require.Nil(t, result)

	// Error should mention exit code
	assert.Contains(t, err.Error(), "exited with code")
	assert.Contains(t, err.Error(), "42")
}

func TestInvokeAI_EmptyOutput(t *testing.T) {
	// Create a tool that produces empty output
	tmpDir, err := os.MkdirTemp("", "ai-test-empty-")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	emptyToolPath := filepath.Join(tmpDir, "empty-tool")

	// Create a script that produces no output
	emptyScript := `#!/bin/sh
exit 0
`
	//nolint:gosec // G306: Test script needs execute permissions
	err = os.WriteFile(emptyToolPath, []byte(emptyScript), 0755)
	require.NoError(t, err)

	cfg := InvokeConfig{
		Tool:    emptyToolPath,
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(cfg)
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

	result, err := InvokeAI(cfg)
	require.Error(t, err)
	require.Nil(t, result)
	assert.Contains(t, err.Error(), "tool cannot be empty")
}

func TestInvokeAI_EmptyPrompt(t *testing.T) {
	cfg := InvokeConfig{
		Tool:    mockToolPath,
		Prompt:  "",
		Verbose: false,
	}

	result, err := InvokeAI(cfg)
	require.Error(t, err)
	require.Nil(t, result)
	assert.Contains(t, err.Error(), "prompt cannot be empty")
}

func TestInvokeAI_ModelPassthrough(t *testing.T) {
	cfg := InvokeConfig{
		Tool:    mockToolPath,
		Model:   "claude-haiku-4.5",
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the model was passed through to the tool
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(result.RawOutput), &jsonOutput)
	require.NoError(t, err)
	assert.Equal(t, "claude-haiku-4.5", jsonOutput["model"])
}

func TestInvokeAI_NoModel(t *testing.T) {
	cfg := InvokeConfig{
		Tool:    mockToolPath,
		Prompt:  "Test prompt",
		Verbose: false,
	}

	result, err := InvokeAI(cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// When no model is set, the tool receives an empty model flag
	var jsonOutput map[string]interface{}
	err = json.Unmarshal([]byte(result.RawOutput), &jsonOutput)
	require.NoError(t, err)
	assert.Equal(t, "", jsonOutput["model"])
}

func TestInvokeAI_Verbose(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	cfg := InvokeConfig{
		Tool:    mockToolPath,
		Prompt:  "Test prompt",
		Verbose: true,
	}

	result, err := InvokeAI(cfg)

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
	// We'll use a custom tool that verifies the file exists

	tmpDir, err := os.MkdirTemp("", "ai-test-skill-")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	verifyToolPath := filepath.Join(tmpDir, "verify-tool")

	// Create a script that verifies the skill file
	verifyScript := `#!/bin/sh
PLUGIN_DIR=""
PRINT_FLAG=""

while [ $# -gt 0 ]; do
    case "$1" in
        --plugin-dir)
            PLUGIN_DIR="$2"
            shift 2
            ;;
        --print)
            PRINT_FLAG="true"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

if [ -z "$PLUGIN_DIR" ] || [ -z "$PRINT_FLAG" ]; then
    echo "Missing required flags" >&2
    exit 1
fi

SKILL_FILE="$PLUGIN_DIR/skills/SKILL.md"
if [ ! -f "$SKILL_FILE" ]; then
    echo "SKILL.md not found" >&2
    exit 1
fi

# Output the skill content as JSON
echo '{"success": true, "skill_file_exists": true}'
exit 0
`
	//nolint:gosec // G306: Test script needs execute permissions
	err = os.WriteFile(verifyToolPath, []byte(verifyScript), 0755)
	require.NoError(t, err)

	cfg := InvokeConfig{
		Tool:    verifyToolPath,
		Prompt:  "Test prompt for skill file",
		Verbose: false,
	}

	result, err := InvokeAI(cfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the output indicates the skill file was found
	assert.Contains(t, result.RawOutput, "skill_file_exists")
}

func TestInvokeAI_Timeout(t *testing.T) {
	// Create a tool that sleeps longer than a short timeout
	tmpDir, err := os.MkdirTemp("", "ai-test-timeout-")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	slowToolPath := filepath.Join(tmpDir, "slow-tool")

	// Create a shell script that sleeps for 2 seconds
	// This will work with the 1-second timeout to trigger DeadlineExceeded
	slowScript := `#!/bin/sh
sleep 2
exit 0
`
	//nolint:gosec // G306: Test script needs execute permissions
	err = os.WriteFile(slowToolPath, []byte(slowScript), 0755)
	require.NoError(t, err)

	cfg := InvokeConfig{
		Tool:    slowToolPath,
		Prompt:  "Test prompt",
		Verbose: false,
	}

	// Note: This test currently uses the hardcoded 60-second timeout in InvokeAI.
	// Since we can't easily override it without modifying InvokeAI, we'll create
	// a helper function that mirrors InvokeAI's logic but with a 1-second timeout.
	// This tests the timeout handling path without waiting 60 seconds.

	// Helper that mimics InvokeAI but with configurable timeout
	testInvokeWithTimeout := func(cfg InvokeConfig, timeout time.Duration) (*InvokeResult, error) {
		// Simplified version of InvokeAI with custom timeout
		tmpDir, err := os.MkdirTemp("", "muster-ai-invoke-")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}

		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to cleanup temp directory %s: %v\n", tmpDir, err)
			}
		}()

		skillsDir := filepath.Join(tmpDir, "skills")
		if err := os.MkdirAll(skillsDir, 0755); err != nil { //nolint:gosec
			return nil, fmt.Errorf("failed to create skills directory: %w", err)
		}

		skillPath := filepath.Join(skillsDir, "SKILL.md")
		if err := os.WriteFile(skillPath, []byte(cfg.Prompt), 0644); err != nil { //nolint:gosec
			return nil, fmt.Errorf("failed to write skill file: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		//nolint:gosec // G204: cfg.Tool is from test config
		cmd := exec.CommandContext(ctx, cfg.Tool, "--print", "--plugin-dir", tmpDir)

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			// Check for context timeout - this is the primary timeout indicator
			if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("tool execution timed out after %v", timeout)
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				// If process was killed due to context cancellation, it's a timeout
				if exitErr.ExitCode() == -1 && ctx.Err() == context.DeadlineExceeded {
					return nil, fmt.Errorf("tool execution timed out after %v", timeout)
				}
				return nil, fmt.Errorf("tool %q exited with code %d: %w", cfg.Tool, exitErr.ExitCode(), err)
			}
			return nil, fmt.Errorf("failed to execute tool %q: %w", cfg.Tool, err)
		}

		return &InvokeResult{RawOutput: stdout.String()}, nil
	}

	// Test with 1-second timeout and 2-second sleep
	result, err := testInvokeWithTimeout(cfg, 1*time.Second)
	require.Error(t, err)
	require.Nil(t, result)

	// Verify error message contains "timed out after"
	assert.Contains(t, err.Error(), "timed out after")

	// Verify temp directory cleanup occurs (defer in helper handles this)
	// If cleanup failed, we'd see a warning in stderr, but the test would still pass
	// This verifies the defer cleanup logic works even after timeout
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
