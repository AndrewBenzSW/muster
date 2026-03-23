package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abenz1267/muster/internal/config"
	"github.com/abenz1267/muster/internal/prompt"
	"github.com/abenz1267/muster/internal/roadmap"
	"github.com/abenz1267/muster/internal/ui"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [slug]",
	Short: "Create a planning workspace for a roadmap item",
	Long: `Create a planning workspace for a roadmap item.

This command:
  1. Resolves the roadmap item (by slug argument or interactive picker)
  2. Creates a planning directory structure (.muster/work/{slug}/plan/)
  3. Generates an implementation plan using AI

If no slug is provided, shows an interactive picker to select an item.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPlan,
}

// planInvoker is the function variable for invoking Claude Code during planning.
// Replaceable variable for testability (matching the vcsFactory pattern from cmd/out.go).
var planInvoker = func(resolved *config.ResolvedConfig, projectCfg *config.ProjectConfig, userCfg *config.UserConfig, tmpDir string) error {
	// Build command
	cmdArgs := []string{"--plugin-dir", tmpDir, "--model", resolved.Model}
	execCmd := exec.Command(config.ToolExecutable(resolved.Tool), cmdArgs...) //nolint:gosec // G204: Tool path validated through config system

	// Connect stdin/stdout/stderr for foreground execution
	execCmd.Stdin = os.Stdin
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	// Apply environment overrides
	envOverrides := config.ToolEnvOverrides(resolved, projectCfg, userCfg)
	if len(envOverrides) > 0 {
		execCmd.Env = os.Environ()
		for k, v := range envOverrides {
			execCmd.Env = append(execCmd.Env, k+"="+v)
		}
	}

	// Run and check exit code
	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("claude code invocation failed: %w", err)
	}

	return nil
}

func runPlan(cmd *cobra.Command, args []string) error {
	// Get flags
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Capture command streams
	errOut := cmd.ErrOrStderr()

	// Step 1: Get project root
	projectRoot := "."

	// Step 2: Load project config
	projectCfg, err := config.LoadProjectConfig(projectRoot)
	if err != nil {
		if errors.Is(err, config.ErrConfigParse) {
			return fmt.Errorf("config file malformed: %w", err)
		}
		return fmt.Errorf("failed to load project config: %w", err)
	}

	// Step 3: Load user config
	userCfg, err := config.LoadUserConfig("")
	if err != nil {
		if errors.Is(err, config.ErrConfigParse) {
			return fmt.Errorf("config file malformed: %w", err)
		}
		return fmt.Errorf("failed to load user config: %w", err)
	}

	// Step 4: Resolve step config (use "plan" as step name)
	resolved, err := config.ResolveStep("plan", projectCfg, userCfg)
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}

	// Verbose logging
	if verbose {
		_, _ = fmt.Fprintf(errOut, "Using: tool=%s (%s) provider=%s (%s) model=%s (%s)\n",
			resolved.Tool, resolved.ToolSource,
			resolved.Provider, resolved.ProviderSource,
			resolved.Model, resolved.ModelSource)
	}

	// Step 5: Load roadmap
	rm, err := roadmap.LoadRoadmap(projectRoot)
	if err != nil {
		if errors.Is(err, roadmap.ErrRoadmapParse) {
			return fmt.Errorf("roadmap file is malformed: %w", err)
		}
		return fmt.Errorf("failed to load roadmap: %w", err)
	}

	// Step 6: Resolve slug
	interactive := ui.IsInteractive()
	slug, item, err := resolveSlug(args, rm, interactive, errOut, ui.DefaultPicker)
	if err != nil {
		return err
	}

	if verbose {
		_, _ = fmt.Fprintf(errOut, "Resolved slug: %s (%s)\n", slug, item.Title)
	}

	// Warning for blocked status
	if interactive && item.Status == roadmap.StatusBlocked {
		_, _ = fmt.Fprintf(errOut, "Warning: item status is 'blocked'\n")
	}

	// Step 7: Create directories
	planDir, err := ensurePlanDir(projectRoot, slug)
	if err != nil {
		return fmt.Errorf("failed to create plan directory: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(errOut, "Plan directory: %s\n", planDir)
	}

	// Step 7.5: Check if plan already exists
	planFilePath := filepath.Join(planDir, "implementation-plan.md")
	if _, err := os.Stat(planFilePath); err == nil {
		// Plan exists
		overwrite, err := confirmOverwrite(planFilePath, interactive, os.Stdin, errOut, force, verbose)
		if err != nil {
			return err
		}
		if !overwrite {
			return nil
		}
	}

	// Step 8: Build PromptContext
	cwd, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve project root: %w", err)
	}

	ctx := prompt.NewPromptContext(resolved, projectCfg, userCfg, true, slug, cwd, cwd, planDir)

	// Step 9: Stage skills
	tmpDir, cleanup, err := prompt.StageSkills(ctx)
	if err != nil {
		return fmt.Errorf("failed to stage skills: %w", err)
	}
	defer cleanup()

	// Step 10: Invoke Claude Code
	if err := planInvoker(resolved, projectCfg, userCfg, tmpDir); err != nil {
		return err
	}

	// Step 11: Verify output
	info, err := os.Stat(planFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("planning completed but implementation-plan.md was not created at %s", planFilePath)
		}
		return fmt.Errorf("failed to verify plan file: %w", err)
	}

	if info.Size() == 0 {
		return fmt.Errorf("plan file exists but is empty at %s", planFilePath)
	}

	// Step 12: Output formatting
	relativePath, err := filepath.Rel(cwd, planFilePath)
	if err != nil {
		// Fallback to absolute path if relative path computation fails
		relativePath = planFilePath
	}

	if verbose {
		_, _ = fmt.Fprintf(errOut, "Plan created successfully at: %s\n", planFilePath)
	}

	// Output according to mode
	if ui.GetOutputMode() == ui.JSONMode {
		// JSON output
		output := map[string]string{
			"slug":      slug,
			"plan_path": relativePath,
		}
		jsonBytes, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", string(jsonBytes))
	} else {
		// Table mode (default)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Implementation plan created: %s\n", relativePath)
	}

	return nil
}

// confirmOverwrite prompts the user to confirm overwriting an existing plan file.
// Returns true if overwrite should proceed, false otherwise.
func confirmOverwrite(planPath string, interactive bool, stdin io.Reader, w io.Writer, force bool, verbose bool) (bool, error) {
	// If --force, skip prompt and overwrite
	if force {
		if verbose {
			_, _ = fmt.Fprintf(w, "Overwriting existing plan\n")
		}
		return true, nil
	}

	// Non-interactive without --force: error
	if !interactive {
		return false, fmt.Errorf("plan exists at %s; use --force to overwrite in non-interactive mode", planPath)
	}

	// Interactive mode: prompt user
	_, _ = fmt.Fprintf(w, "Plan already exists. Overwrite? (y/N) ")
	reader := bufio.NewReader(stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		return true, nil
	}

	return false, nil
}

// ensurePlanDir creates the planning directory structure and returns the absolute path.
func ensurePlanDir(projectRoot, slug string) (string, error) {
	// Create base plan directory
	planDir := filepath.Join(projectRoot, ".muster", "work", slug, "plan")
	//nolint:gosec // G301: Standard directory permissions for plan storage
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create plan directory: %w", err)
	}

	// Create subdirectories
	researchDir := filepath.Join(planDir, "research")
	//nolint:gosec // G301: Standard directory permissions for plan storage
	if err := os.MkdirAll(researchDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create research directory: %w", err)
	}

	synthesisDir := filepath.Join(planDir, "synthesis")
	//nolint:gosec // G301: Standard directory permissions for plan storage
	if err := os.MkdirAll(synthesisDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create synthesis directory: %w", err)
	}

	// Get absolute path
	absPath, err := filepath.Abs(planDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	return absPath, nil
}

// resolveSlug resolves the roadmap item slug from arguments or interactive picker.
// It returns the slug, the item, or an error.
func resolveSlug(args []string, rm *roadmap.Roadmap, interactive bool, w io.Writer, picker ui.Picker) (string, *roadmap.RoadmapItem, error) {
	// Argument mode
	if len(args) > 0 {
		slug := args[0]
		item := rm.FindBySlug(slug)
		if item == nil {
			return "", nil, fmt.Errorf("roadmap item %q not found", slug)
		}

		// Print warning if completed and interactive
		if interactive && item.Status == roadmap.StatusCompleted {
			_, _ = fmt.Fprintf(w, "Warning: planning an already-completed item\n")
		}

		return slug, item, nil
	}

	// Picker mode
	if !interactive {
		return "", nil, fmt.Errorf("slug argument required in non-interactive mode")
	}

	if len(rm.Items) == 0 {
		return "", nil, fmt.Errorf("roadmap is empty")
	}

	// Filter out completed items
	var candidates []roadmap.RoadmapItem
	for _, item := range rm.Items {
		if item.Status != roadmap.StatusCompleted {
			candidates = append(candidates, item)
		}
	}

	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("no non-completed items in roadmap")
	}

	// Priority order: high > medium > low > lower
	priorityOrder := map[roadmap.Priority]int{
		roadmap.PriorityHigh:   0,
		roadmap.PriorityMedium: 1,
		roadmap.PriorityLow:    2,
		roadmap.PriorityLower:  3,
	}

	// Sort by priority then alphabetically
	sort.Slice(candidates, func(i, j int) bool {
		pi := priorityOrder[candidates[i].Priority]
		pj := priorityOrder[candidates[j].Priority]

		if pi != pj {
			return pi < pj
		}

		// Same priority: sort alphabetically by slug
		return candidates[i].Slug < candidates[j].Slug
	})

	// Build picker options
	options := make([]ui.PickerOption, len(candidates))
	for i, item := range candidates {
		label := fmt.Sprintf("%s - %s [%s, %s]", item.Slug, item.Title, item.Priority, item.Status)
		options[i] = ui.PickerOption{
			Label: label,
			Value: item.Slug,
		}
	}

	// Show picker
	selectedSlug, err := picker.Show("Select a roadmap item to plan:", options, ui.DefaultPickerConfig())
	if err != nil {
		return "", nil, fmt.Errorf("failed to select item: %w", err)
	}

	// Find the selected item
	item := rm.FindBySlug(selectedSlug)
	if item == nil {
		return "", nil, fmt.Errorf("selected item %q not found", selectedSlug)
	}

	return selectedSlug, item, nil
}

func init() {
	rootCmd.AddCommand(planCmd)

	// Define flags
	planCmd.Flags().Bool("force", false, "Overwrite existing plan directory")
}
