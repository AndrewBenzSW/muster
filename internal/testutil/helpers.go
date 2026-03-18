package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertGoldenFile compares actual content with a golden file.
// If update is true, it writes the actual content to the golden file.
func AssertGoldenFile(t *testing.T, goldenPath string, actual string, update bool) {
	t.Helper()

	if update {
		// Ensure directory exists
		dir := filepath.Dir(goldenPath)
		if err := os.MkdirAll(dir, 0755); err != nil { //nolint:gosec // G301: Test directory permissions
			t.Fatalf("failed to create golden file directory: %v", err)
		}

		// Write actual content to golden file
		if err := os.WriteFile(goldenPath, []byte(actual), 0644); err != nil { //nolint:gosec // G306: Test file permissions
			t.Fatalf("failed to update golden file: %v", err)
		}
		t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	// Read golden file
	expected, err := os.ReadFile(goldenPath) //nolint:gosec // G304: Test fixture path
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
	}

	// Compare
	assert.Equal(t, string(expected), actual, "content should match golden file %s", goldenPath)
}

// RequireCommand checks if a command exists in PATH and fails the test if it doesn't.
func RequireCommand(t *testing.T, name string) {
	t.Helper()

	_, err := exec.LookPath(name)
	require.NoError(t, err, "command %s must be available in PATH", name)
}
