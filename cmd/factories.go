// Package cmd implements the CLI commands for muster.
//
// # Testing with Mock Injection
//
// This package uses the factory pattern for dependency injection, enabling tests to replace
// production implementations with mocks. The pattern follows the vcsFactory approach from
// cmd/out.go:21.
//
// # Mock Thread Safety Warning
//
// Mock implementations are not safe for concurrent use. Each test must create its own mock
// instance with separate t.TempDir() directories. Do not share mock instances across
// t.Parallel() subtests.
//
// ## Writing Tests with Mock Factories
//
// To test commands that use factories (codingToolFactory, interactiveToolFactory,
// containerRuntimeFactory), follow this pattern:
//
// 1. Save the original factory variable
// 2. Create a mock instance (typically using internal/coding.NewMockCodingTool)
// 3. Replace the factory with a function that returns your mock
// 4. Restore the original factory in a defer
//
// Example:
//
//	func TestMyCommand_WithMock(t *testing.T) {
//	    // Save original factory
//	    original := codingToolFactory
//	    defer func() { codingToolFactory = original }()
//
//	    // Create mock instance
//	    callsDir := t.TempDir()
//	    responsesDir := t.TempDir()
//	    mockTool := coding.NewMockCodingTool(callsDir, responsesDir)
//
//	    // Replace factory
//	    codingToolFactory = func() (coding.CodingTool, error) {
//	        return mockTool, nil
//	    }
//
//	    // Create response fixture (optional - for simulating tool output)
//	    responseContent := `{"raw_output": "..."}`
//	    err := os.WriteFile(
//	        filepath.Join(responsesDir, "001-invoke.json"),
//	        []byte(responseContent),
//	        0644,
//	    )
//	    require.NoError(t, err)
//
//	    // Run your command test - it will use the mock
//	    cmd := &cobra.Command{}
//	    // ... configure cmd ...
//	    err = runMyCommand(cmd, []string{"arg"})
//	    require.NoError(t, err)
//
//	    // Verify mock was called correctly
//	    callFile := filepath.Join(callsDir, "001-invoke.json")
//	    require.FileExists(t, callFile)
//	    // ... verify call contents ...
//	}
//
// ## Mock Response Fixtures
//
// Mock tools read response fixtures from the responsesDir. To simulate specific responses:
//
// - Success response: Write JSON to "{seq}-invoke.json"
// - Error response: Write JSON with "error" field to "{seq}-invoke.json"
// - Missing response: Don't create the file (mock returns error)
//
// Example error response:
//
//	{"error": "simulated AI error"}
//
// ## Available Factories
//
// - codingToolFactory: For non-interactive AI invocations (add, sync commands)
// - interactiveToolFactory: For interactive sessions (code command)
// - containerRuntimeFactory: For Docker operations (code --yolo, down commands)
//
// See cmd/add_test.go and cmd/plan_test.go for complete examples.
package cmd

import (
	"github.com/abenz1267/muster/internal/coding"
	"github.com/abenz1267/muster/internal/docker"
)

// Factory variables for dependency injection, matching the vcsFactory pattern from cmd/out.go:21.
// These package-level variables can be replaced in tests to inject mocks without refactoring
// command implementations. Each factory returns an error to support initialization failures.

// codingToolFactory creates instances of CodingTool for non-interactive AI invocations.
// Used by commands that need single-shot AI prompts (add, sync).
var codingToolFactory = func() (coding.CodingTool, error) {
	return coding.NewClaudeCodeTool()
}

// interactiveToolFactory creates instances of InteractiveCodingTool for interactive sessions.
// Used by the code command to start terminal-connected AI sessions.
var interactiveToolFactory = func() (coding.InteractiveCodingTool, error) {
	return coding.NewClaudeCodeTool()
}

// containerRuntimeFactory creates instances of ContainerRuntime for Docker operations.
// Used by commands that need container orchestration (code --yolo, down).
var containerRuntimeFactory = func() (docker.ContainerRuntime, error) {
	return docker.NewContainerRuntime()
}
