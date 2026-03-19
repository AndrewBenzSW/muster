package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// InvokeConfig holds configuration for AI tool invocation.
type InvokeConfig struct {
	Tool    string            // path or name of the AI tool executable
	Model   string            // model to use (passed as --model flag; empty means tool default)
	Prompt  string            // prompt content to stage as a skill file
	Verbose bool              // if true, print command and config to stderr
	Timeout time.Duration     // timeout for tool execution (default: 60 seconds if not set)
	Env     map[string]string // additional environment variables (e.g., ANTHROPIC_BASE_URL for local models)
}

// InvokeResult contains the result of AI tool invocation.
type InvokeResult struct {
	RawOutput string // captured stdout from the tool
}

// InvokeAI invokes an AI tool with a single-shot skill file staged in a temp directory.
// The function stages the prompt as a SKILL.md file, executes the tool with --print and
// --plugin-dir flags to get non-interactive JSON output, and captures the result.
//
// Stages:
//  1. Create temporary directory
//  2. Write prompt to tmpDir/skills/SKILL.md
//  3. Execute: tool --print --plugin-dir tmpDir
//  4. Capture stdout to buffer, stderr to os.Stderr
//  5. Clean up temp directory
//
// Error handling:
//   - exec.ErrNotFound: returns error with tool installation guidance
//   - non-zero exit code: returns error including stderr
//   - timeout after configured duration (default: 60 seconds)
//
// If Verbose is true, prints the tool command and resolved config to stderr.
func InvokeAI(cfg InvokeConfig) (*InvokeResult, error) {
	// Validate config
	if cfg.Tool == "" {
		return nil, errors.New("tool cannot be empty")
	}
	if cfg.Prompt == "" {
		return nil, errors.New("prompt cannot be empty")
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "muster-ai-invoke-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Ensure cleanup on completion
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup temp directory %s: %v\n", tmpDir, err)
		}
	}()

	// Create skills directory
	skillsDir := filepath.Join(tmpDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil { //nolint:gosec // G301: Standard directory permissions for temp skill staging
		return nil, fmt.Errorf("failed to create skills directory: %w", err)
	}

	// Write prompt to SKILL.md
	skillPath := filepath.Join(skillsDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(cfg.Prompt), 0644); err != nil { //nolint:gosec // G306: Standard file permissions for temp skill file
		return nil, fmt.Errorf("failed to write skill file: %w", err)
	}

	// Use configured timeout, default to 120 seconds if not set
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Build command: tool --print --plugin-dir tmpDir [--model model]
	args := []string{"--print", "--plugin-dir", tmpDir}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	//nolint:gosec // G204: cfg.Tool is from config, tmpDir is internal temp directory
	cmd := exec.CommandContext(ctx, cfg.Tool, args...)

	// Pipe prompt content via stdin (claude --print reads the prompt from stdin)
	cmd.Stdin = strings.NewReader(cfg.Prompt)

	// Capture stdout to buffer
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	// Apply environment overrides (e.g., ANTHROPIC_BASE_URL for local models)
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Print verbose info if requested
	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "AI Invoke: tool=%s, tmpDir=%s\n", cfg.Tool, tmpDir)
		fmt.Fprintf(os.Stderr, "Command: %s %v\n", cfg.Tool, cmd.Args[1:])
		if len(cfg.Env) > 0 {
			for k, v := range cfg.Env {
				fmt.Fprintf(os.Stderr, "Env override: %s=%s\n", k, v)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Env overrides: none\n")
		}
	}

	// Execute command
	err = cmd.Run()
	if err != nil {
		// Check for context timeout
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("tool execution timed out after %v", timeout)
		}

		// Non-zero exit code — include captured stdout which often contains the error details
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			output := strings.TrimSpace(stdout.String())
			if output != "" {
				return nil, fmt.Errorf("tool %q exited with code %d:\n%s", cfg.Tool, exitErr.ExitCode(), output)
			}
			return nil, fmt.Errorf("tool %q exited with code %d: %w", cfg.Tool, exitErr.ExitCode(), err)
		}

		// Check for specific error types (file not found, permission denied, etc.)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("tool %q not found: ensure it is installed and in your PATH", cfg.Tool)
		}

		// Other execution error
		return nil, fmt.Errorf("failed to execute tool %q: %w", cfg.Tool, err)
	}

	// Return result
	return &InvokeResult{
		RawOutput: stdout.String(),
	}, nil
}

// codeBlockRe matches markdown fenced code blocks (```json ... ``` or ``` ... ```).
var codeBlockRe = regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*\n(.*?)\\s*```\\s*$")

// ExtractJSON strips markdown code fences from AI output if present,
// returning the inner content. If no fences are found, returns the
// input unchanged. This handles the common case where models wrap
// JSON output in ```json ... ``` despite being told not to.
func ExtractJSON(raw string) string {
	if m := codeBlockRe.FindStringSubmatch(raw); len(m) == 2 {
		return m[1]
	}
	return raw
}
