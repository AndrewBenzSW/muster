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

// InitGitRepo initializes a git repository in the specified directory.
// It runs git init -b main, configures user.email and user.name,
// creates an initial file, and makes an initial commit.
// Returns the directory path. Fails the test on any error.
func InitGitRepo(t *testing.T, dir string) string {
	t.Helper()

	// git init -b main
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// git config user.email
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.email failed: %v", err)
	}

	// git config user.name
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.name failed: %v", err)
	}

	// Create an initial file
	initialFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test Repository\n"), 0644); err != nil { //nolint:gosec // G306: Test file permissions
		t.Fatalf("failed to create initial file: %v", err)
	}

	// git add .
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	// git commit -m "initial"
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	return dir
}
