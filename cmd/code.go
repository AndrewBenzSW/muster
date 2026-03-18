package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/abenz1267/muster/internal/config"
	"github.com/abenz1267/muster/internal/docker"
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
		// Task 4.2 & 6.1: Implement --yolo flag for Docker flow
		yolo, err := cmd.Flags().GetBool("yolo")
		if err != nil {
			return err
		}
		if yolo {
			return fmt.Errorf("--yolo (sandboxed container mode) is not yet implemented")
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
				true, // interactive
				"",   // slug
				".",  // worktreePath
				".",  // mainRepoPath
				"",   // planDir
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
		execCmd := exec.Command(config.ToolExecutable(resolved.Tool), cmdArgs...) //nolint:gosec // G204: Tool path validated through config system
		execCmd.Stdin = os.Stdin
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		if err := execCmd.Run(); err != nil {
			// Task 4.7: Add error handling with categories
			executable := config.ToolExecutable(resolved.Tool)
			var execErr *exec.Error
			if errors.As(err, &execErr) && execErr.Err == exec.ErrNotFound {
				return fmt.Errorf("tool %q not found: %w\n\nPlease install %s and ensure it is in your PATH.\nFor Claude Code: https://docs.anthropic.com/claude-code\nFor OpenCode: https://github.com/opencodeinterpreter/opencode", executable, err, executable)
			}
			return fmt.Errorf("failed to execute %s: %w", executable, err)
		}

		return nil
	},
}

// runDockerFlow implements the full Docker orchestration flow for --yolo flag.
// Steps:
// 1. Load all configuration (user, project, dev-agent)
// 2. Validate configuration
// 3. Collect authentication with steps=["interactive"]
// 4. Extract Docker assets
// 5. Detect workspace (worktree vs main repo)
// 6. Generate Docker Compose file
// 7. Start containers with docker compose up
// 8. Execute tool in container with docker compose exec
func runDockerFlow(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get verbose flag
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Step 1: Load all configuration
	if verbose {
		fmt.Fprintf(os.Stderr, "Loading configuration...\n")
	}
	cfg, err := config.LoadAll("", ".")
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Step 2: Validate configuration
	if verbose {
		fmt.Fprintf(os.Stderr, "Validating configuration...\n")
	}
	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Configuration validation errors:\n")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		return fmt.Errorf("configuration validation failed with %d error(s)", len(errs))
	}

	// Step 3: Collect authentication (interactive mode only for Phase 6)
	if verbose {
		fmt.Fprintf(os.Stderr, "Collecting authentication...\n")
	}
	authReqs, authErrs := docker.CollectAuthRequirements(cfg, []string{"interactive"})
	if len(authErrs) > 0 {
		fmt.Fprintf(os.Stderr, "Authentication collection errors:\n")
		for _, e := range authErrs {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		return fmt.Errorf("authentication collection failed with %d error(s)", len(authErrs))
	}

	// Step 4: Extract Docker assets
	if verbose {
		fmt.Fprintf(os.Stderr, "Extracting Docker assets...\n")
	}
	assetDir, err := docker.ExtractAssets()
	if err != nil {
		return fmt.Errorf("failed to extract Docker assets: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Assets extracted to: %s\n", assetDir)
	}

	// Step 5: Detect workspace
	if verbose {
		fmt.Fprintf(os.Stderr, "Detecting workspace...\n")
	}
	worktreeDir, mainRepoDir, err := docker.DetectWorkspace()
	if err != nil {
		return fmt.Errorf("failed to detect workspace: %w", err)
	}
	if verbose {
		if mainRepoDir != "" {
			fmt.Fprintf(os.Stderr, "Detected worktree: %s (main repo: %s)\n", worktreeDir, mainRepoDir)
		} else {
			fmt.Fprintf(os.Stderr, "Detected main repo: %s\n", worktreeDir)
		}
	}

	// Determine project name from current directory basename
	projectName := filepath.Base(worktreeDir)
	if verbose {
		fmt.Fprintf(os.Stderr, "Project name: %s\n", projectName)
	}

	// Step 6: Generate Docker Compose file
	if verbose {
		fmt.Fprintf(os.Stderr, "Generating Docker Compose file...\n")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, ".cache", "muster", "compose", projectName)
	//nolint:gosec // G301: Directory permissions 0755 are appropriate for cache directory
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Convert dev-agent config to the format expected by GenerateComposeFile
	var devAgent interface{}
	if cfg.DevAgent != nil {
		devAgent = struct {
			AllowedDomains []string
			Env            map[string]string
			Volumes        []string
			Networks       []string
		}{
			AllowedDomains: cfg.DevAgent.AllowedDomains,
			Env:            cfg.DevAgent.Env,
			Volumes:        cfg.DevAgent.Volumes,
			Networks:       cfg.DevAgent.Networks,
		}
	}

	composePath, err := docker.GenerateComposeFile(docker.ComposeOptions{
		Project:     projectName,
		Slug:        "", // Empty for interactive mode
		Auth:        authReqs,
		DevAgent:    devAgent,
		WorktreeDir: worktreeDir,
		MainRepoDir: mainRepoDir,
		AssetDir:    assetDir,
		CacheDir:    cacheDir,
	})
	if err != nil {
		return fmt.Errorf("failed to generate Docker Compose file: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Compose file generated: %s\n", composePath)
	}

	// Step 7: Start containers
	if verbose {
		fmt.Fprintf(os.Stderr, "Starting Docker containers...\n")
	}
	client, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer func() { _ = client.Close() }()

	// Check Docker is running
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("Docker check failed: %w", err)
	}

	upCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := client.ComposeUp(upCtx, composePath, projectName); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}

	// Step 8: Execute tool in container
	if verbose {
		fmt.Fprintf(os.Stderr, "Executing tool in container...\n")
	}

	// Get the resolved tool (for logging purposes)
	resolved, err := cfg.Resolve("interactive")
	if err == nil && verbose {
		fmt.Fprintf(os.Stderr, "Using: tool=%s provider=%s model=%s\n", resolved.Tool, resolved.Provider, resolved.Model)
	}

	// Execute the tool in the dev-agent service
	// Pass through stdin/stdout/stderr for interactive session
	execErr := client.ComposeExec(ctx, composePath, projectName, "dev-agent", []string{config.ToolExecutable(resolved.Tool)})

	// Note: We don't automatically stop containers on exit to allow inspection
	// Users can manually stop with 'muster down' or 'docker compose down'
	if verbose && execErr == nil {
		fmt.Fprintf(os.Stderr, "\nContainer session ended. Use 'muster down' to stop containers.\n")
	}

	return execErr
}

func init() {
	// Task 4.1: Create code command skeleton
	rootCmd.AddCommand(codeCmd)

	// Define persistent flags
	codeCmd.PersistentFlags().String("tool", "", "Override the tool to use (e.g., claude, opencode)")
	codeCmd.PersistentFlags().Bool("no-plugin", false, "Run the tool without the staged skills plugin")
	codeCmd.PersistentFlags().Bool("keep-staged", false, "Keep staged skills directory after command exits")

	// Add local flag --yolo
	codeCmd.Flags().Bool("yolo", false, "Run in sandboxed Docker container with isolated environment")
}
