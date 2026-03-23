package git

import "fmt"

// CurrentBranch returns the name of the current branch in the repository.
func CurrentBranch(dir string) (string, error) {
	branch, err := RunGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return branch, nil
}

// PullLatest pulls the latest changes from the specified remote and branch.
func PullLatest(dir, remote, branch string) error {
	_, err := RunGit(dir, "pull", remote, branch)
	if err != nil {
		return fmt.Errorf("failed to pull %s/%s: %w", remote, branch, err)
	}
	return nil
}

// DeleteBranch deletes the specified branch.
// Uses -d (not -D) for safety - will fail if branch is not fully merged.
func DeleteBranch(dir, branch string) error {
	_, err := RunGit(dir, "branch", "-d", branch)
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %w", branch, err)
	}
	return nil
}
