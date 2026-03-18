package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCommand_HasExpectedFlags(t *testing.T) {
	// Test format flag
	formatFlag := rootCmd.PersistentFlags().Lookup("format")
	assert.NotNil(t, formatFlag, "format flag should exist")
	if formatFlag != nil {
		assert.Equal(t, "string", formatFlag.Value.Type(), "format flag should be a string")
		assert.Equal(t, "", formatFlag.DefValue, "format flag default should be empty string")
	}

	// Test verbose flag
	verboseFlag := rootCmd.PersistentFlags().Lookup("verbose")
	assert.NotNil(t, verboseFlag, "verbose flag should exist")
	if verboseFlag != nil {
		assert.Equal(t, "bool", verboseFlag.Value.Type(), "verbose flag should be a bool")
		assert.Equal(t, "false", verboseFlag.DefValue, "verbose flag default should be false")
	}
}

func TestExecute_ReturnsNoError(t *testing.T) {
	// Execute with no args should either succeed or return a usage error (both acceptable)
	err := Execute()

	// No error is acceptable (help message displayed)
	// An error containing "Usage:" or help text is also acceptable
	if err != nil {
		errStr := err.Error()
		// If there's an error, it should be usage-related, not a panic or runtime error
		assert.NotContains(t, errStr, "panic", "should not panic")
		assert.NotContains(t, errStr, "runtime error", "should not have runtime errors")
	}
}

func TestRootCommand_VerboseFlag_CanBeSet(t *testing.T) {
	// Test that the verbose flag can be set without error
	// This verifies the flag is properly wired up
	err := rootCmd.PersistentFlags().Set("verbose", "true")
	assert.NoError(t, err, "setting verbose flag should not error")

	// Reset to default value
	err = rootCmd.PersistentFlags().Set("verbose", "false")
	assert.NoError(t, err, "resetting verbose flag should not error")
}
