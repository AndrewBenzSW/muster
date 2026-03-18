package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain_SuccessExitCode(t *testing.T) {
	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", "/tmp/muster-test", ".")
	err := buildCmd.Run()
	require.NoError(t, err, "building binary should succeed")

	// Run the binary with help flag (should succeed)
	cmd := exec.Command("/tmp/muster-test", "--help")
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "command should exit with code 0")

	// Verify output contains expected help text
	outputStr := string(output)
	assert.Contains(t, outputStr, "muster", "help output should contain command name")
	assert.Contains(t, outputStr, "Usage:", "help output should contain usage")
}

func TestMain_SuccessExitCode_Version(t *testing.T) {
	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", "/tmp/muster-test-version", ".")
	err := buildCmd.Run()
	require.NoError(t, err, "building binary should succeed")

	// Run the binary with version command (should succeed)
	cmd := exec.Command("/tmp/muster-test-version", "version")
	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "version command should exit with code 0")

	// Verify output contains version information
	outputStr := string(output)
	assert.True(t, strings.Contains(outputStr, "Version:") || strings.Contains(outputStr, "version"),
		"version output should contain version information")
}

func TestMain_ErrorExitCode_InvalidCommand(t *testing.T) {
	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", "/tmp/muster-test-error", ".")
	err := buildCmd.Run()
	require.NoError(t, err, "building binary should succeed")

	// Run the binary with invalid command (should fail)
	cmd := exec.Command("/tmp/muster-test-error", "nonexistent-command")
	output, err := cmd.CombinedOutput()

	// Command should exit with non-zero code
	assert.Error(t, err, "invalid command should exit with error code")

	// Check the exit code is 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 1, exitErr.ExitCode(), "exit code should be 1 for error")
	}

	// Output should indicate an error
	outputStr := string(output)
	assert.Contains(t, outputStr, "Error:", "error output should contain error message")
}
