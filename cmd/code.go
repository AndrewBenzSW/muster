package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/abenz1267/muster/internal/config"
	"github.com/abenz1267/muster/internal/prompt"
	"github.com/spf13/cobra"
)

var codeCmd = &cobra.Command{
	Use:   "code",
	Short: "Launch Claude/OpenCode with workflow skills staged",
	Long: `Launch Claude/OpenCode CLI in the current directory with workflow skills staged.

This command:
  1. Loads project and user configuration
  2. Resolves the tool, provider, and model triple
  3. Stages workflow skill templates to a temporary directory
  4. Executes the resolved tool with the staged skills as a plugin

The staged skills enable AI assistants to orchestrate roadmap-driven workflows
using Claude Agent SDK skills.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Task 4.2: Stub --yolo with helpful error
		yolo, err := cmd.Flags().GetBool("yolo")
		if err != nil {
			return err
		}
		if yolo {
			return fmt.Errorf("The --yolo flag (sandboxed container mode) is not yet implemented. It will be available in Phase 2: Docker container orchestration. For now, use 'muster code' to launch Claude/OpenCode locally with workflow skills staged.")
		}

		// Task 4.3: Implement config loading
		userCfg, err := config.LoadUserConfig("")
		if err != nil {
			// Task 4.7: Categorize config errors
			if errors.Is(err, config.ErrConfigParse) {
				return fmt.Errorf("config file malformed: %w", err)
			}
			return fmt.Errorf("failed to load user config: %w", err)
		}

		projectCfg, err := config.LoadProjectConfig(".")
		if err != nil {
			// Task 4.7: Categorize config errors
			if errors.Is(err, config.ErrConfigParse) {
				return fmt.Errorf("config file malformed: %w", err)
			}
			return fmt.Errorf("failed to load project config: %w", err)
		}

		// Task 4.4: Implement config resolution
		resolved, err := config.ResolveCode(projectCfg, userCfg)
		if err != nil {
			return fmt.Errorf("failed to resolve config: %w", err)
		}

		// Override tool if --tool flag is set
		tool, err := cmd.Flags().GetString("tool")
		if err != nil {
			return err
		}
		if tool != "" {
			resolved.Tool = tool
		}

		// Log resolved triple if verbose (verbose is a persistent flag from root)
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			fmt.Fprintf(os.Stderr, "Using: tool=%s provider=%s model=%s\n", resolved.Tool, resolved.Provider, resolved.Model)
		}

		// Task 4.6: Implement process execution
		noPlugin, err := cmd.Flags().GetBool("no-plugin")
		if err != nil {
			return err
		}

		// Build command arguments
		var cmdArgs []string
		var tmpDir string
		if !noPlugin {
			// Task 4.5: Implement template staging
			ctx := prompt.NewPromptContext(
				resolved,
				userCfg,
				true,  // interactive
				"",    // slug
				".",   // worktreePath
				".",   // mainRepoPath
				"",    // planDir
			)

			var cleanup func()
			tmpDir, cleanup, err = prompt.StageSkills(ctx)
			if err != nil {
				// Task 4.7: Categorize staging errors
				if errors.Is(err, prompt.ErrTemplateRender) {
					return fmt.Errorf("template render failure: %w", err)
				}
				return fmt.Errorf("staging failure (temp dir: %s): %w", tmpDir, err)
			}
			shouldCleanup := true
			defer func() {
				if shouldCleanup {
					cleanup()
				}
			}()

			// If --keep-staged is set, skip cleanup
			keepStaged, err := cmd.Flags().GetBool("keep-staged")
			if err != nil {
				return err
			}
			if keepStaged {
				shouldCleanup = false
				fmt.Fprintf(os.Stderr, "Staged skills kept at: %s\n", tmpDir)
			}

			cmdArgs = append(cmdArgs, "--plugin-dir", tmpDir)
		}

		// Execute the tool
		execCmd := exec.Command(resolved.Tool, cmdArgs...)
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			// Task 4.7: Add error handling with categories
			var execErr *exec.Error
			if errors.As(err, &execErr) && execErr.Err == exec.ErrNotFound {
				return fmt.Errorf("tool %q not found: %w\n\nPlease install %s and ensure it is in your PATH.\nFor Claude Code: https://docs.anthropic.com/claude-code\nFor OpenCode: https://github.com/opencodeinterpreter/opencode", resolved.Tool, err, resolved.Tool)
			}
			return fmt.Errorf("failed to execute %s: %w", resolved.Tool, err)
		}

		return nil
	},
}

func init() {
	// Task 4.1: Create code command skeleton
	rootCmd.AddCommand(codeCmd)

	// Define persistent flags
	codeCmd.PersistentFlags().String("tool", "", "Override the tool to use (e.g., claude-code, opencode)")
	codeCmd.PersistentFlags().Bool("no-plugin", false, "Run the tool without the staged skills plugin")
	codeCmd.PersistentFlags().Bool("keep-staged", false, "Keep staged skills directory after command exits")

	// Add local flag --yolo
	codeCmd.Flags().Bool("yolo", false, "Run in sandboxed container mode (not yet implemented)")
}
