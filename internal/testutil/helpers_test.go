package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssertGoldenFile_UpdateTrue_CreatesFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	goldenPath := filepath.Join(tmpDir, "testdata", "golden.txt")
	actualContent := "test content\nline 2"

	// Call with update=true to create the file
	AssertGoldenFile(t, goldenPath, actualContent, true)

	// Verify the file was created
	content, err := os.ReadFile(goldenPath) //nolint:gosec // G304: Test file path
	require.NoError(t, err, "golden file should be created")
	assert.Equal(t, actualContent, string(content), "file content should match")
}

func TestAssertGoldenFile_UpdateFalse_ComparesCorrectly(t *testing.T) {
	// Create a temporary golden file
	tmpDir := t.TempDir()
	goldenPath := filepath.Join(tmpDir, "golden.txt")
	expectedContent := "expected content"

	err := os.WriteFile(goldenPath, []byte(expectedContent), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err, "setup should succeed")

	// Test with matching content (should not fail)
	AssertGoldenFile(t, goldenPath, expectedContent, false)
}

func TestAssertGoldenFile_UpdateFalse_FailsWhenContentDoesNotMatch(t *testing.T) {
	// Create a temporary golden file
	tmpDir := t.TempDir()
	goldenPath := filepath.Join(tmpDir, "golden.txt")
	expectedContent := "expected content"

	err := os.WriteFile(goldenPath, []byte(expectedContent), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err, "setup should succeed")

	// Create a mock testing.T to capture the failure
	mockT := &testing.T{}

	// Test with non-matching content (should fail)
	actualContent := "different content"
	AssertGoldenFile(mockT, goldenPath, actualContent, false)

	// Verify that the test failed
	assert.True(t, mockT.Failed(), "test should fail when content doesn't match")
}

func TestRequireCommand_SucceedsForKnownCommand(t *testing.T) {
	// Test with a command that should exist on all systems
	RequireCommand(t, "ls")
	// If we reach here, the test passed (RequireCommand didn't fail the test)
}

func TestRequireCommand_FailsForNonExistentCommand(t *testing.T) {
	// We can't easily test that RequireCommand fails without causing the test to fail
	// Instead, we test the underlying behavior by checking LookPath directly
	_, err := exec.LookPath("this-command-definitely-does-not-exist-12345")
	assert.Error(t, err, "LookPath should return error for non-existent command")

	// Note: RequireCommand uses require.NoError which calls FailNow()
	// Testing this would require running it in a subprocess or using testing.T's Run method
}
