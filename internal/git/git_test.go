package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abenz1267/muster/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunGit_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	output, err := RunGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, "main", output)
}

func TestRunGit_InvalidCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	_, err := RunGit(dir, "invalid-command")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git invalid-command failed")
}

func TestRunGit_NonGitDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	// Don't initialize as git repo

	_, err := RunGit(dir, "status")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git status failed")
}

func TestRunGit_SetsDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create two separate repos
	dir1 := t.TempDir()
	testutil.InitGitRepo(t, dir1)

	dir2 := t.TempDir()
	testutil.InitGitRepo(t, dir2)

	// Create a file in dir1 only
	testFile := filepath.Join(dir1, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Verify RunGit operates in the correct directory
	// dir1 should have the uncommitted file
	output1, err := RunGit(dir1, "status", "--porcelain")
	require.NoError(t, err)
	assert.Contains(t, output1, "test.txt", "dir1 should show uncommitted test.txt")

	// dir2 should not have it
	output2, err := RunGit(dir2, "status", "--porcelain")
	require.NoError(t, err)
	assert.NotContains(t, output2, "test.txt", "dir2 should not show test.txt")
}
