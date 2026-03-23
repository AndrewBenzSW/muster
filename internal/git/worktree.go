package git

import "fmt"

// RemoveWorktree removes a git worktree.
// Uses --force because this runs after PR merge, so the worktree is disposable.
func RemoveWorktree(repoDir, worktreePath string) error {
	_, err := RunGit(repoDir, "worktree", "remove", "--force", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to remove worktree %s: %w", worktreePath, err)
	}
	return nil
}
