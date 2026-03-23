package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// RunGit runs a git command in the specified directory.
// Returns trimmed stdout on success, or an error with stderr details on failure.
func RunGit(dir string, args ...string) (string, error) {
	//nolint:gosec // G204: Git subcommands are controlled by package functions, not arbitrary user input
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w\nStderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
