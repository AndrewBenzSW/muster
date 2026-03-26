package coding

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/abenz1267/muster/internal/ai"
)

// ClaudeCodeTool is an implementation of CodingTool and InteractiveCodingTool
// that wraps the claude-code CLI tool.
//
// NewClaudeCodeTool currently returns nil error; future versions may validate
// tool binary availability or version compatibility.
type ClaudeCodeTool struct{}

// NewClaudeCodeTool creates a new ClaudeCodeTool instance.
func NewClaudeCodeTool() (*ClaudeCodeTool, error) {
	return &ClaudeCodeTool{}, nil
}

// Invoke executes the AI tool with the provided configuration.
func (c *ClaudeCodeTool) Invoke(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error) {
	return ai.InvokeAI(ctx, cfg)
}

// RunInteractive starts an interactive coding session with the configured tool.
// Connects stdin/stdout/stderr to the terminal for direct user interaction.
func (c *ClaudeCodeTool) RunInteractive(ctx context.Context, cfg InteractiveConfig) error {
	// Build command arguments
	args := []string{}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.PluginDir != "" {
		args = append(args, "--plugin-dir", cfg.PluginDir)
	}

	// Print verbose info if requested
	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "Interactive: tool=%s, model=%s, plugin-dir=%s\n", cfg.Tool, cfg.Model, cfg.PluginDir)
		if len(cfg.Env) > 0 {
			for k, v := range cfg.Env {
				fmt.Fprintf(os.Stderr, "Env override: %s=%s\n", k, v)
			}
		}
	}

	// Create command with context
	//nolint:gosec // G204: cfg.Tool is from config resolution, not direct user input
	cmd := exec.CommandContext(ctx, cfg.Tool, args...)

	// Connect to terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Apply environment overrides
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	// Execute command
	err := cmd.Run()
	if err != nil {
		// Check for tool not found
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("tool %q not found: ensure it is installed and in your PATH", cfg.Tool)
		}
		return fmt.Errorf("interactive session with %q failed: %w", cfg.Tool, err)
	}

	return nil
}
