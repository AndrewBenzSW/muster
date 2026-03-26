package coding

import (
	"context"

	"github.com/abenz1267/muster/internal/ai"
)

// CodingTool is an interface for non-interactive AI coding tool invocation.
// Implementations execute AI tools with a single-shot prompt and capture output.
//
// The context parameter is passed through to the underlying AI invocation and supports
// cancellation and deadlines.
type CodingTool interface {
	// Invoke executes the AI tool with the provided configuration and returns the result.
	// The context supports cancellation and deadline propagation to the underlying tool.
	Invoke(ctx context.Context, cfg ai.InvokeConfig) (*ai.InvokeResult, error)
}

// InteractiveCodingTool is an interface for interactive AI coding tool sessions.
// Implementations run interactive sessions with stdin/stdout/stderr connected to the terminal.
type InteractiveCodingTool interface {
	// RunInteractive starts an interactive coding session with the configured tool.
	RunInteractive(ctx context.Context, cfg InteractiveConfig) error
}

// InteractiveConfig holds configuration for interactive AI tool sessions.
type InteractiveConfig struct {
	Tool      string            // path or name of the AI tool executable
	Model     string            // model to use (passed as --model flag; empty means tool default)
	PluginDir string            // directory containing skills/plugins (passed as --plugin-dir)
	Env       map[string]string // additional environment variables (e.g., ANTHROPIC_BASE_URL for local models)
	Verbose   bool              // if true, print command and config to stderr
}
