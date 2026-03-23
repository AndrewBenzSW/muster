package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/abenz1267/muster/internal/config"
	"github.com/abenz1267/muster/internal/git"
	"github.com/abenz1267/muster/internal/prompt"
	"github.com/abenz1267/muster/internal/roadmap"
	"github.com/abenz1267/muster/internal/vcs"
	"github.com/spf13/cobra"
)

// vcsFactory is the factory function for creating VCS instances.
// Replaceable variable for testability (matching the ai.InvokeAI pattern).
var vcsFactory = vcs.New

// maxCIFixRetries is the maximum number of CI fix attempts
const maxCIFixRetries = 3

var outCmd = &cobra.Command{
	Use:   "out [slug]",
	Short: "Complete a roadmap item after PR merge",
	Long: `Complete a roadmap item after PR merge.

This command:
  1. Validates the roadmap item exists and is not already completed
  2. Discovers the PR URL from the item's branch or pr_url field
  3. Checks authentication with the VCS CLI (gh/glab)
  4. Monitors CI status and waits for all checks to pass (with --wait flag)
  5. Waits for PR merge (with --wait flag)
  6. Cleans up: pulls latest changes, removes worktree, deletes branch
  7. Marks the item as completed in the roadmap

The --no-fix flag skips AI-assisted CI fix attempts on failure.
The --wait flag enables polling for CI completion and merge.
The --dry-run flag shows what would happen without executing changes.`,
	Args: cobra.ExactArgs(1),
	RunE: runOut,
}

func runOut(cmd *cobra.Command, args []string) error {
	slug := args[0]

	// Get flags
	noFix, err := cmd.Flags().GetBool("no-fix")
	if err != nil {
		return err
	}
	wait, err := cmd.Flags().GetBool("wait")
	if err != nil {
		return err
	}
	timeout, err := cmd.Flags().GetDuration("timeout")
	if err != nil {
		return err
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Load config
	userCfg, err := config.LoadUserConfig("")
	if err != nil {
		if errors.Is(err, config.ErrConfigParse) {
			return fmt.Errorf("config file malformed: %w", err)
		}
		return fmt.Errorf("failed to load user config: %w", err)
	}

	projectCfg, err := config.LoadProjectConfig(".")
	if err != nil {
		if errors.Is(err, config.ErrConfigParse) {
			return fmt.Errorf("config file malformed: %w", err)
		}
		return fmt.Errorf("failed to load project config: %w", err)
	}

	// Resolve merge strategy
	strategy := config.ResolveMergeStrategy(projectCfg)
	if strategy == config.MergeStrategyDirect {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Merge strategy is 'direct' - no PR workflow to complete\n")
		return nil
	}

	// Load roadmap
	rm, err := roadmap.LoadRoadmap(".")
	if err != nil {
		if errors.Is(err, roadmap.ErrRoadmapParse) {
			return fmt.Errorf("roadmap file is malformed: %w", err)
		}
		return fmt.Errorf("failed to load roadmap: %w", err)
	}

	// Find item by slug
	item := rm.FindBySlug(slug)
	if item == nil {
		return fmt.Errorf("roadmap item %q not found", slug)
	}

	// Error if already completed
	if item.Status == roadmap.StatusCompleted {
		return fmt.Errorf("roadmap item %q is already marked as completed", slug)
	}

	// Resolve step config
	resolved, err := config.ResolveStep("out", projectCfg, userCfg)
	if err != nil {
		return fmt.Errorf("failed to resolve config: %w", err)
	}

	// Verbose logging
	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Using: tool=%s (%s) provider=%s (%s) model=%s (%s)\n",
			resolved.Tool, resolved.ToolSource,
			resolved.Provider, resolved.ProviderSource,
			resolved.Model, resolved.ModelSource)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Merge strategy: %s\n", strategy)
		if item.PRUrl != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "PR URL: %s\n", *item.PRUrl)
		} else if item.Branch != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Branch: %s\n", *item.Branch)
		}
	}

	// Create VCS client
	vcsClient, err := vcsFactory(strategy, ".")
	if err != nil {
		return fmt.Errorf("failed to create VCS client: %w", err)
	}

	// Check auth immediately (fail fast - MUST-9)
	if !dryRun {
		if err := vcsClient.CheckAuth(); err != nil {
			// Provide actionable error message
			var authErr string
			switch strategy {
			case config.MergeStrategyGitHubPR:
				authErr = "gh not authenticated: run 'gh auth login' "
			case config.MergeStrategyGitLabMR:
				authErr = "glab not authenticated: run 'glab auth login'"
			default:
				authErr = "VCS CLI not authenticated"
			}
			return fmt.Errorf("%s: %w", authErr, err)
		}
	} else {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[DRY RUN] Would check VCS authentication\n")
	}

	// Discover PR
	prRef, err := discoverPR(item, vcsClient)
	if err != nil {
		return err
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "PR reference: %s\n", prRef)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Monitor CI status (poll every 1 second)
	if err := monitorCI(monitorCIOpts{
		w:            cmd.ErrOrStderr(),
		ctx:          ctx,
		prRef:        prRef,
		vcsClient:    vcsClient,
		noFix:        noFix,
		wait:         wait,
		pollInterval: 30 * time.Second,
		dryRun:       dryRun,
		verbose:      verbose,
		resolved:     resolved,
		projectCfg:   projectCfg,
		userCfg:      userCfg,
		slug:         slug,
	}); err != nil {
		return err
	}

	// Wait for merge
	if err := waitForMerge(cmd.ErrOrStderr(), ctx, prRef, vcsClient, wait, dryRun, verbose); err != nil {
		return err
	}

	// Cleanup
	if err := cleanup(cmd.ErrOrStderr(), slug, item, rm, ".", dryRun, verbose); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Roadmap item %q completed successfully\n", slug)
	return nil
}

// discoverPR attempts to discover the PR reference from the roadmap item.
// It tries item.PRUrl first, then item.Branch via vcsClient.ViewPR.
func discoverPR(item *roadmap.RoadmapItem, vcsClient vcs.VCS) (string, error) {
	// Try PR URL first
	if item.PRUrl != nil && *item.PRUrl != "" {
		return *item.PRUrl, nil
	}

	// Try branch
	if item.Branch != nil && *item.Branch != "" {
		// ViewPR can accept a branch name and will find the associated PR/MR
		_, err := vcsClient.ViewPR(*item.Branch)
		if err != nil {
			return "", fmt.Errorf("failed to find PR for branch %q: %w", *item.Branch, err)
		}
		return *item.Branch, nil
	}

	return "", fmt.Errorf("roadmap item has no pr_url or branch - cannot discover PR")
}

// monitorCIOpts holds options for monitorCI function.
type monitorCIOpts struct {
	w            io.Writer
	ctx          context.Context
	prRef        string
	vcsClient    vcs.VCS
	noFix        bool
	wait         bool
	pollInterval time.Duration
	dryRun       bool
	verbose      bool
	resolved     *config.ResolvedConfig
	projectCfg   *config.ProjectConfig
	userCfg      *config.UserConfig
	slug         string
}

// monitorCI monitors CI status and handles failures.
// pollInterval parameter allows tests to use 0 for immediate returns.
// This function is enhanced to accept resolved config and other params for AI fix loop integration.
func monitorCI(opts monitorCIOpts) error {
	w := opts.w
	ctx := opts.ctx
	prRef := opts.prRef
	vcsClient := opts.vcsClient
	noFix := opts.noFix
	wait := opts.wait
	pollInterval := opts.pollInterval
	dryRun := opts.dryRun
	verbose := opts.verbose
	resolved := opts.resolved
	projectCfg := opts.projectCfg
	userCfg := opts.userCfg
	slug := opts.slug
	if dryRun {
		_, _ = fmt.Fprintf(w, "[DRY RUN] Would monitor CI status for PR %s\n", prRef)
		return nil
	}

	// Track fix attempts
	attempt := 0

	// Check CI once
	checks, err := vcsClient.ListChecks(prRef)
	if err != nil {
		return fmt.Errorf("failed to list CI checks: %w", err)
	}

	// Aggregate status
	status := aggregateChecks(checks)

	if verbose {
		_, _ = fmt.Fprintf(w, "CI status: %s (%d checks)\n", status, len(checks))
		for _, check := range checks {
			_, _ = fmt.Fprintf(w, "  - %s: %s\n", check.Name, check.Status)
		}
	}

	// Handle different statuses
	switch status {
	case vcs.CIStatusPassing:
		_, _ = fmt.Fprintf(w, "All CI checks passed\n")
		return nil

	case vcs.CIStatusFailing:
		if noFix {
			return fmt.Errorf("CI checks failing (--no-fix enabled)")
		}
		// Attempt AI fix
		attempt++
		if attempt > maxCIFixRetries {
			return fmt.Errorf("CI checks still failing after %d attempts; fix manually and re-run", maxCIFixRetries)
		}
		if err := fixCI(w, prRef, vcsClient, resolved, projectCfg, userCfg, slug, attempt); err != nil {
			_, _ = fmt.Fprintf(w, "Warning: CI fix failed: %v\n", err)
		}
		// Resume polling after fix attempt
		if !wait {
			_, _ = fmt.Fprintf(w, "CI fix pushed - use --wait to continue monitoring\n")
			return nil
		}
		// If wait=true, enter polling loop below

	case vcs.CIStatusPending:
		if !wait {
			_, _ = fmt.Fprintf(w, "CI checks are pending - use --wait to poll for completion\n")
			return nil
		}
		// If wait=true, enter polling loop below

	case vcs.CIStatusNone:
		_, _ = fmt.Fprintf(w, "No CI checks configured\n")
		return nil

	default:
		return fmt.Errorf("unexpected CI status: %s", status)
	}

	// Poll for completion (entered when wait=true for Failing or Pending status)
	_, _ = fmt.Fprintf(w, "Waiting for CI checks to complete...\n")
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for CI checks to complete")

		case <-ticker.C:
			if verbose {
				_, _ = fmt.Fprintf(w, "[%s] Checking CI status...\n", time.Now().Format("15:04:05"))
			}

			checks, err := vcsClient.ListChecks(prRef)
			if err != nil {
				return fmt.Errorf("failed to list CI checks: %w", err)
			}

			status := aggregateChecks(checks)
			if verbose {
				_, _ = fmt.Fprintf(w, "  Status: %s\n", status)
			}

			switch status {
			case vcs.CIStatusPassing:
				_, _ = fmt.Fprintf(w, "All CI checks passed\n")
				return nil

			case vcs.CIStatusFailing:
				if noFix {
					return fmt.Errorf("CI checks failing (--no-fix enabled)")
				}
				// Attempt AI fix
				attempt++
				if attempt > maxCIFixRetries {
					return fmt.Errorf("CI checks still failing after %d attempts; fix manually and re-run", maxCIFixRetries)
				}
				if err := fixCI(w, prRef, vcsClient, resolved, projectCfg, userCfg, slug, attempt); err != nil {
					_, _ = fmt.Fprintf(w, "Warning: CI fix failed: %v\n", err)
				}
				// Resume polling after fix attempt
				continue

			case vcs.CIStatusPending:
				// Continue polling
				continue
			}
		}
	}
}

// aggregateChecks aggregates individual check results into an overall status.
func aggregateChecks(checks []vcs.CheckResult) vcs.CIStatus {
	if len(checks) == 0 {
		return vcs.CIStatusNone
	}

	hasPending := false
	hasFailing := false

	for _, check := range checks {
		switch check.Status {
		case vcs.CIStatusFailing:
			hasFailing = true
		case vcs.CIStatusPending:
			hasPending = true
		}
	}

	if hasFailing {
		return vcs.CIStatusFailing
	}
	if hasPending {
		return vcs.CIStatusPending
	}
	return vcs.CIStatusPassing
}

// fixCI attempts to fix CI failures using AI.
// It fetches failed logs, renders the fix prompt, and invokes AI to make targeted fixes.
// Returns error if AI invocation fails (does not halt retry loop).
func fixCI(w io.Writer, prRef string, vcsClient vcs.VCS, resolved *config.ResolvedConfig, projectCfg *config.ProjectConfig, userCfg *config.UserConfig, slug string, attempt int) error {
	_, _ = fmt.Fprintf(w, "Attempting CI fix (attempt %d of %d)...\n", attempt, maxCIFixRetries)

	// Fetch failed logs
	failedLogs, err := vcsClient.GetFailedLogs(prRef)
	if err != nil {
		_, _ = fmt.Fprintf(w, "Warning: failed to retrieve CI logs: %v\n", err)
		// Use generic prompt if log retrieval fails
		failedLogs = []vcs.FailedCheckLog{
			{
				CheckName: "CI checks",
				Logs:      "CI checks failed, but detailed logs could not be retrieved. Please investigate and fix the issues.",
			},
		}
	}

	// If logs are empty (e.g., GitLab best-effort failed), use generic prompt
	if len(failedLogs) == 0 {
		failedLogs = []vcs.FailedCheckLog{
			{
				CheckName: "CI checks",
				Logs:      "CI checks failed, please investigate and fix.",
			},
		}
	}

	// Build prompt context
	promptCtx := prompt.NewPromptContext(
		resolved,
		projectCfg,
		userCfg,
		false, // not interactive
		slug,
		".", // worktreePath (current directory)
		".", // mainRepoPath
		"",  // planDir
	)
	promptCtx.Extra["FailedChecks"] = failedLogs
	promptCtx.Extra["Attempt"] = attempt

	// Render template
	promptContent, err := prompt.RenderTemplate("prompts/out/ci-fix-prompt.md.tmpl", promptCtx)
	if err != nil {
		return fmt.Errorf("failed to render CI fix prompt: %w", err)
	}

	// Invoke AI with muster-standard tier model
	_, _ = fmt.Fprintf(w, "Invoking AI to analyze and fix CI failures...\n")
	_, err = ai.InvokeAI(ai.InvokeConfig{
		Tool:    config.ToolExecutable(resolved.Tool),
		Model:   promptCtx.Models.Standard, // Use muster-standard tier
		Prompt:  promptContent,
		Verbose: false,
		Env:     config.ToolEnvOverrides(resolved, projectCfg, userCfg),
		Timeout: 10 * time.Minute, // Allow more time for CI fixes
	})
	if err != nil {
		_, _ = fmt.Fprintf(w, "Warning: AI fix attempt failed: %v\n", err)
		return err
	}

	_, _ = fmt.Fprintf(w, "AI fix attempt completed. Waiting for new CI run...\n")
	return nil
}

// waitForMerge polls for PR merge status.
func waitForMerge(w io.Writer, ctx context.Context, prRef string, vcsClient vcs.VCS, wait, dryRun, verbose bool) error {
	if dryRun {
		_, _ = fmt.Fprintf(w, "[DRY RUN] Would wait for PR merge: %s\n", prRef)
		return nil
	}

	// Check once
	prStatus, err := vcsClient.ViewPR(prRef)
	if err != nil {
		return fmt.Errorf("failed to view PR: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(w, "PR state: %s\n", prStatus.State)
		if prStatus.ReviewStatus != "" {
			_, _ = fmt.Fprintf(w, "Review status: %s\n", prStatus.ReviewStatus)
		}
	}

	// Handle current state
	switch prStatus.State {
	case vcs.PRStateMerged:
		_, _ = fmt.Fprintf(w, "PR is merged\n")
		return nil

	case vcs.PRStateClosed:
		return fmt.Errorf("PR was closed without merging")

	case vcs.PRStateOpen:
		if prStatus.ReviewStatus == vcs.ReviewStatusChangesRequested {
			return fmt.Errorf("PR has requested changes - cannot proceed")
		}

		if !wait {
			_, _ = fmt.Fprintf(w, "PR is open and not merged - use --wait to poll for merge\n")
			return nil
		}

		// Poll for merge
		_, _ = fmt.Fprintf(w, "Waiting for PR to be merged...\n")
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("timeout waiting for PR merge")

			case <-ticker.C:
				if verbose {
					_, _ = fmt.Fprintf(w, "[%s] Checking PR status...\n", time.Now().Format("15:04:05"))
				}

				prStatus, err := vcsClient.ViewPR(prRef)
				if err != nil {
					return fmt.Errorf("failed to view PR: %w", err)
				}

				if verbose {
					_, _ = fmt.Fprintf(w, "  State: %s\n", prStatus.State)
				}

				switch prStatus.State {
				case vcs.PRStateMerged:
					_, _ = fmt.Fprintf(w, "PR is merged\n")
					return nil

				case vcs.PRStateClosed:
					return fmt.Errorf("PR was closed without merging")

				case vcs.PRStateOpen:
					if prStatus.ReviewStatus == vcs.ReviewStatusChangesRequested {
						return fmt.Errorf("PR has requested changes - cannot proceed")
					}
					// Continue polling
					continue

				default:
					return fmt.Errorf("unknown PR state: %s", prStatus.State)
				}
			}
		}

	default:
		return fmt.Errorf("unknown PR state: %s", prStatus.State)
	}
}

// cleanup performs post-merge cleanup: pull latest, remove worktree, delete branch, mark completed.
// Collects all errors and marks item completed regardless of cleanup failures.
func cleanup(w io.Writer, slug string, item *roadmap.RoadmapItem, rm *roadmap.Roadmap, projectDir string, dryRun, verbose bool) error {
	var errs []error

	if dryRun {
		_, _ = fmt.Fprintf(w, "[DRY RUN] Would perform cleanup:\n")
		_, _ = fmt.Fprintf(w, "  - Pull latest changes from origin/main\n")
		if item.Branch != nil {
			_, _ = fmt.Fprintf(w, "  - Remove worktree (if exists)\n")
			_, _ = fmt.Fprintf(w, "  - Delete branch: %s\n", *item.Branch)
		}
		_, _ = fmt.Fprintf(w, "  - Mark item %q as completed\n", slug)
		return nil
	}

	// Pull latest
	if verbose {
		_, _ = fmt.Fprintf(w, "Pulling latest changes...\n")
	}
	// TODO: Hardcoded to origin/main - should detect from git config
	if err := git.PullLatest(projectDir, "origin", "main"); err != nil {
		errs = append(errs, fmt.Errorf("failed to pull latest: %w", err))
		_, _ = fmt.Fprintf(w, "Warning: %v\n", err)
	}

	// Remove worktree (if branch exists)
	if item.Branch != nil && *item.Branch != "" {
		// TODO: Worktree cleanup not implemented - requires detection of worktree path from git state

		// Delete branch
		if verbose {
			_, _ = fmt.Fprintf(w, "Deleting branch %s...\n", *item.Branch)
		}
		if err := git.DeleteBranch(projectDir, *item.Branch); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete branch: %w", err))
			_, _ = fmt.Fprintf(w, "Warning: %v\n", err)
		}
	}

	// Mark item completed (regardless of cleanup failures)
	item.Status = roadmap.StatusCompleted
	if err := roadmap.SaveRoadmap(projectDir, rm); err != nil {
		return fmt.Errorf("failed to save roadmap: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(w, "Marked item %q as completed\n", slug)
	}

	// Return collected errors if any
	// The item is marked completed regardless of cleanup failures
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(outCmd)

	// Define flags
	outCmd.Flags().Bool("no-fix", false, "Skip AI-assisted CI fix attempts on failure")
	outCmd.Flags().Bool("wait", false, "Wait for CI completion and PR merge (polls at 30s intervals)")
	outCmd.Flags().Duration("timeout", 30*time.Minute, "Maximum time to wait for CI and merge")
	outCmd.Flags().Bool("dry-run", false, "Show what would happen without executing changes")
	outCmd.Flags().Bool("verbose", false, "Enable verbose output")
}
