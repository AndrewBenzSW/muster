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

func TestCurrentBranch_Main(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	branch, err := CurrentBranch(dir)
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestCurrentBranch_Feature(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	// Create and checkout a feature branch
	cmd := exec.Command("git", "checkout", "-b", "feature-branch")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	branch, err := CurrentBranch(dir)
	require.NoError(t, err)
	assert.Equal(t, "feature-branch", branch)
}

func TestCurrentBranch_NotGitRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	// Don't initialize as git repo

	_, err := CurrentBranch(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get current branch")
}

func TestPullLatest_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a bare repo to act as remote
	bareDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	require.NoError(t, cmd.Run())

	// Clone it
	cloneDir := t.TempDir()
	cmd = exec.Command("git", "clone", bareDir, cloneDir) //nolint:gosec // G204: Test command with controlled temp dir paths
	require.NoError(t, cmd.Run())

	// Configure git user in clone
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = cloneDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = cloneDir
	require.NoError(t, cmd.Run())

	// Create initial commit in clone
	testFile := filepath.Join(cloneDir, "test.txt")
	err := os.WriteFile(testFile, []byte("initial"), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = cloneDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = cloneDir
	require.NoError(t, cmd.Run())

	// Push to bare repo
	cmd = exec.Command("git", "push", "origin", "master")
	cmd.Dir = cloneDir
	require.NoError(t, cmd.Run())

	// Pull should succeed
	err = PullLatest(cloneDir, "origin", "master")
	assert.NoError(t, err)
}

func TestPullLatest_NoRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	// Attempt to pull from non-existent remote
	err := PullLatest(dir, "origin", "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to pull origin/main")
}

func TestDeleteBranch_Merged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "feature")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Add a commit on feature branch
	testFile := filepath.Join(dir, "feature.txt")
	err := os.WriteFile(testFile, []byte("feature content"), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	cmd = exec.Command("git", "add", "feature.txt")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "add feature")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Merge into main
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "merge", "feature")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Delete should succeed
	err = DeleteBranch(dir, "feature")
	assert.NoError(t, err)

	// Verify branch is gone
	cmd = exec.Command("git", "branch")
	cmd.Dir = dir
	output, err := cmd.Output()
	require.NoError(t, err)
	assert.NotContains(t, string(output), "feature")
}

func TestDeleteBranch_Unmerged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	// Create a feature branch with changes
	cmd := exec.Command("git", "checkout", "-b", "feature")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Add a commit
	testFile := filepath.Join(dir, "feature.txt")
	err := os.WriteFile(testFile, []byte("feature content"), 0644) //nolint:gosec // G306: Test file permissions
	require.NoError(t, err)

	cmd = exec.Command("git", "add", "feature.txt")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "add feature")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Switch back to main without merging
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Delete should fail (branch not merged)
	err = DeleteBranch(dir, "feature")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete branch feature")
}

func TestDeleteBranch_Current(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)

	// Try to delete current branch
	err := DeleteBranch(dir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete branch main")
}
