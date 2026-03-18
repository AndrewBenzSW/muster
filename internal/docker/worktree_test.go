package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:gocyclo // Test function with multiple test cases requires high complexity
func TestDetectWorkspace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name           string
		setup          func(t *testing.T, dir string) string // Returns working directory
		expectWorktree bool                                  // True if worktree expected
		expectMainRepo bool                                  // True if separate main repo expected
		expectError    bool
	}{
		{
			name: "main repository",
			setup: func(t *testing.T, dir string) string {
				// Create a main git repo
				if err := runCmd(dir, "git", "init"); err != nil {
					t.Fatalf("failed to init repo: %v", err)
				}
				if err := runCmd(dir, "git", "config", "user.email", "test@example.com"); err != nil {
					t.Fatalf("failed to config email: %v", err)
				}
				if err := runCmd(dir, "git", "config", "user.name", "Test User"); err != nil {
					t.Fatalf("failed to config name: %v", err)
				}
				// Create initial commit
				testFile := filepath.Join(dir, "test.txt")
				//nolint:gosec // G306: Test file permissions are appropriate
				if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
				if err := runCmd(dir, "git", "add", "test.txt"); err != nil {
					t.Fatalf("failed to add file: %v", err)
				}
				if err := runCmd(dir, "git", "commit", "-m", "initial commit"); err != nil {
					t.Fatalf("failed to commit: %v", err)
				}
				return dir
			},
			expectWorktree: true,
			expectMainRepo: false, // main repo's .git is within worktree
		},
		{
			name: "git worktree",
			setup: func(t *testing.T, dir string) string {
				// Create main repo
				mainRepo := filepath.Join(dir, "main")
				//nolint:gosec // G301: Test directory permissions are appropriate
				if err := os.Mkdir(mainRepo, 0755); err != nil {
					t.Fatalf("failed to create main repo dir: %v", err)
				}

				if err := runCmd(mainRepo, "git", "init"); err != nil {
					t.Fatalf("failed to init main repo: %v", err)
				}
				if err := runCmd(mainRepo, "git", "config", "user.email", "test@example.com"); err != nil {
					t.Fatalf("failed to config email: %v", err)
				}
				if err := runCmd(mainRepo, "git", "config", "user.name", "Test User"); err != nil {
					t.Fatalf("failed to config name: %v", err)
				}

				// Create initial commit on main branch
				testFile := filepath.Join(mainRepo, "test.txt")
				//nolint:gosec // G306: Test file permissions are appropriate
				if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
				if err := runCmd(mainRepo, "git", "add", "test.txt"); err != nil {
					t.Fatalf("failed to add file: %v", err)
				}
				if err := runCmd(mainRepo, "git", "commit", "-m", "initial commit"); err != nil {
					t.Fatalf("failed to commit: %v", err)
				}

				// Create a worktree
				worktreePath := filepath.Join(dir, "worktree")
				if err := runCmd(mainRepo, "git", "worktree", "add", worktreePath, "-b", "feature"); err != nil {
					t.Fatalf("failed to create worktree: %v", err)
				}

				return worktreePath
			},
			expectWorktree: true,
			expectMainRepo: true, // worktree has separate main .git
		},
		{
			name: "not a git repository",
			setup: func(t *testing.T, dir string) string {
				// Just return a non-git directory
				return dir
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			workDir := tt.setup(t, tmpDir)

			// Change to working directory
			oldDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get cwd: %v", err)
			}
			defer func() {
				_ = os.Chdir(oldDir)
			}()

			if err := os.Chdir(workDir); err != nil {
				t.Fatalf("failed to chdir: %v", err)
			}

			// Run detection
			worktreeDir, mainRepoDir, err := DetectWorkspace()

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify results
			if !tt.expectWorktree {
				if worktreeDir == "" {
					t.Error("expected worktree dir, got empty string")
				}
			} else {
				if worktreeDir == "" {
					t.Error("expected worktree dir, got empty string")
				}
			}

			if tt.expectMainRepo {
				if mainRepoDir == "" {
					t.Error("expected main repo dir, got empty string")
				}
				if worktreeDir == mainRepoDir {
					t.Error("worktree and main repo should be different for worktrees")
				}
			} else {
				if mainRepoDir != "" {
					t.Errorf("expected empty main repo dir for non-worktree, got %s", mainRepoDir)
				}
			}

			// Verify paths are absolute
			if worktreeDir != "" && !filepath.IsAbs(worktreeDir) {
				t.Errorf("worktree dir should be absolute, got: %s", worktreeDir)
			}
			if mainRepoDir != "" && !filepath.IsAbs(mainRepoDir) {
				t.Errorf("main repo dir should be absolute, got: %s", mainRepoDir)
			}
		})
	}
}

func TestRunGitCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		args        []string
		expectError bool
		expectEmpty bool
	}{
		{
			name: "valid command in git repo",
			setup: func(t *testing.T, dir string) {
				if err := runCmd(dir, "git", "init"); err != nil {
					t.Fatalf("failed to init repo: %v", err)
				}
			},
			args:        []string{"rev-parse", "--show-toplevel"},
			expectError: false,
			expectEmpty: false,
		},
		{
			name: "invalid command",
			setup: func(t *testing.T, dir string) {
				if err := runCmd(dir, "git", "init"); err != nil {
					t.Fatalf("failed to init repo: %v", err)
				}
			},
			args:        []string{"not-a-real-command"},
			expectError: true,
		},
		{
			name:        "command in non-git directory",
			setup:       func(t *testing.T, dir string) {},
			args:        []string{"rev-parse", "--show-toplevel"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(t, tmpDir)

			// Change to temp directory
			oldDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get cwd: %v", err)
			}
			defer func() {
				_ = os.Chdir(oldDir)
			}()

			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("failed to chdir: %v", err)
			}

			// Run command
			output, err := runGitCommand(tt.args...)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectEmpty {
				if output != "" {
					t.Errorf("expected empty output, got: %s", output)
				}
			} else {
				if output == "" {
					t.Error("expected non-empty output, got empty string")
				}
			}

			// Output should not have trailing whitespace
			if output != strings.TrimSpace(output) {
				t.Error("output should be trimmed")
			}
		})
	}
}

// runCmd is a helper to run commands in a specific directory.
func runCmd(dir string, name string, args ...string) error {
	//nolint:gosec // G204: Test helper with validated git commands, not arbitrary user input
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &exec.ExitError{
			ProcessState: cmd.ProcessState,
			Stderr:       output,
		}
	}
	return nil
}
