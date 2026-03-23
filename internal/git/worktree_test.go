package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/abenz1267/muster/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveWorktree_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create main repo
	mainDir := t.TempDir()
	testutil.InitGitRepo(t, mainDir)

	// Create a worktree
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "-b", "feature") //nolint:gosec // G204: Test command with controlled temp dir paths
	cmd.Dir = mainDir
	require.NoError(t, cmd.Run())

	// Verify worktree exists
	_, err := os.Stat(worktreeDir)
	require.NoError(t, err, "worktree should exist before removal")

	// Remove worktree
	err = RemoveWorktree(mainDir, worktreeDir)
	assert.NoError(t, err)

	// Verify worktree is gone
	_, err = os.Stat(worktreeDir)
	assert.True(t, os.IsNotExist(err), "worktree should not exist after removal")

	// Explicit cleanup to avoid Windows lockfile issues
	// The directory should already be removed by git worktree remove,
	// but we attempt cleanup to ensure proper test teardown
	_ = os.RemoveAll(worktreeDir)
}

func TestRemoveWorktree_Nonexistent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	// Try to remove non-existent worktree
	nonexistentPath := filepath.Join(t.TempDir(), "nonexistent")
	err := RemoveWorktree(dir, nonexistentPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove worktree")
}

func TestRemoveWorktree_ForceRemovesCleanWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create main repo
	mainDir := t.TempDir()
	testutil.InitGitRepo(t, mainDir)

	// Create a worktree
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "-b", "feature") //nolint:gosec // G204: Test command with controlled temp dir paths
	cmd.Dir = mainDir
	require.NoError(t, cmd.Run())

	// Add a file but commit it (clean state)
	testFile := filepath.Join(worktreeDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = worktreeDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "add test file")
	cmd.Dir = worktreeDir
	require.NoError(t, cmd.Run())

	// Remove with --force should work even for clean worktree
	err = RemoveWorktree(mainDir, worktreeDir)
	assert.NoError(t, err)

	// Verify worktree is gone
	_, err = os.Stat(worktreeDir)
	assert.True(t, os.IsNotExist(err), "worktree should be removed")

	// Explicit cleanup to avoid Windows lockfile issues
	_ = os.RemoveAll(worktreeDir)
}

func TestRemoveWorktree_ForceRemovesDirtyWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create main repo
	mainDir := t.TempDir()
	testutil.InitGitRepo(t, mainDir)

	// Create a worktree
	worktreeDir := filepath.Join(t.TempDir(), "worktree")
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "-b", "feature") //nolint:gosec // G204: Test command with controlled temp dir paths
	cmd.Dir = mainDir
	require.NoError(t, cmd.Run())

	// Add uncommitted changes (dirty state)
	testFile := filepath.Join(worktreeDir, "test.txt")
	err := os.WriteFile(testFile, []byte("uncommitted content"), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	// Remove with --force should work even with uncommitted changes
	err = RemoveWorktree(mainDir, worktreeDir)
	assert.NoError(t, err)

	// Verify worktree is gone
	_, err = os.Stat(worktreeDir)
	assert.True(t, os.IsNotExist(err), "worktree should be removed even with uncommitted changes")

	// Explicit cleanup to avoid Windows lockfile issues
	_ = os.RemoveAll(worktreeDir)
}
