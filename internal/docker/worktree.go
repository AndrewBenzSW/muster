package docker

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// DetectWorkspace detects the git workspace configuration.
// Returns the worktree directory and main repository .git directory.
//
// Per review S8: Uses `git rev-parse --show-toplevel` for working directory
// and `git rev-parse --git-common-dir` for main .git location.
//
// If paths differ, this is a worktree (mount both).
// If same, this is main repo (mount once read-write).
func DetectWorkspace() (worktreeDir, mainRepoDir string, err error) {
	// Get the top-level directory of the working tree
	worktreeDir, err = runGitCommand("rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("failed to detect worktree: %w", err)
	}

	// Get the common .git directory (for worktrees, this is the main repo's .git)
	gitCommonDir, err := runGitCommand("rev-parse", "--git-common-dir")
	if err != nil {
		return "", "", fmt.Errorf("failed to detect git common dir: %w", err)
	}

	// Resolve to absolute path if relative
	if !strings.HasPrefix(gitCommonDir, "/") {
		// git-common-dir can be relative (like ".git" for main repo or "../.git/worktrees/xxx" for worktree)
		absGitDir, err := runGitCommand("rev-parse", "--absolute-git-dir")
		if err != nil {
			return "", "", fmt.Errorf("failed to resolve absolute git dir: %w", err)
		}
		gitCommonDir = absGitDir
	}

	// Determine if this is a worktree or main repo
	// For main repo: git-common-dir is within the worktree-dir
	// For worktree: git-common-dir points to main repo's .git
	if strings.HasPrefix(gitCommonDir, worktreeDir) {
		// Main repo: git dir is inside worktree
		// Mount worktree once (includes .git)
		return worktreeDir, "", nil
	}

	// Worktree: git dir is outside worktree
	// Need to mount both worktree and main .git separately
	return worktreeDir, gitCommonDir, nil
}

// runGitCommand runs a git command and returns the trimmed output.
func runGitCommand(args ...string) (string, error) {
	//nolint:gosec // G204: Arguments are validated git subcommands, not arbitrary user input
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w\nStderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
