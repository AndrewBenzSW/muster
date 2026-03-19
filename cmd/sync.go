package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/abenz1267/muster/internal/config"
	"github.com/abenz1267/muster/internal/prompt"
	"github.com/abenz1267/muster/internal/roadmap"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync roadmap items from source to target",
	Long: `Sync roadmap items from source file to target file.

This command:
  1. Loads source roadmap directly from specified file
  2. Loads target roadmap with fallback chain
  3. Matches items by exact slug match
  4. Uses AI for fuzzy matching of remaining unmatched items
  5. Updates target with matched items, adds new items
  6. Optionally deletes unmatched target items (with --delete flag)

By default, syncs from .roadmap.json to .muster/roadmap.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		sourcePath, err := cmd.Flags().GetString("source")
		if err != nil {
			return err
		}
		targetPath, err := cmd.Flags().GetString("target")
		if err != nil {
			return err
		}
		yesFlag, err := cmd.Flags().GetBool("yes")
		if err != nil {
			return err
		}
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		deleteFlag, err := cmd.Flags().GetBool("delete")
		if err != nil {
			return err
		}
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Load config following cmd/code.go:38-54 pattern
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

		// Resolve config for sync step
		resolved, err := config.ResolveStep("sync", projectCfg, userCfg)
		if err != nil {
			return fmt.Errorf("failed to resolve config: %w", err)
		}

		// Verbose logging
		if verbose {
			fmt.Fprintf(os.Stderr, "Using: tool=%s (%s) provider=%s (%s) model=%s (%s)\n",
				resolved.Tool, resolved.ToolSource,
				resolved.Provider, resolved.ProviderSource,
				resolved.Model, resolved.ModelSource)
			fmt.Fprintf(os.Stderr, "Source: %s\n", sourcePath)
			fmt.Fprintf(os.Stderr, "Target: %s\n", targetPath)
		}

		// Load source roadmap directly (not using fallback chain)
		sourceRoadmap, err := roadmap.LoadRoadmapFile(sourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("source file not found: %s", sourcePath)
			}
			return fmt.Errorf("failed to load source roadmap: %w", err)
		}

		// Load target roadmap directly from file (if exists), otherwise create empty
		targetRoadmap, err := roadmap.LoadRoadmapFile(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Target doesn't exist, start with empty roadmap
				targetRoadmap = &roadmap.Roadmap{Items: []roadmap.RoadmapItem{}}
			} else {
				return fmt.Errorf("failed to load target roadmap: %w", err)
			}
		}

		// Perform sync algorithm
		updated, added, deleted, err := performSync(
			sourceRoadmap,
			targetRoadmap,
			resolved,
			userCfg,
			deleteFlag,
			yesFlag,
			verbose,
		)
		if err != nil {
			return err
		}

		// Dry-run: display summary and exit
		if dryRun {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Dry-run mode: No changes will be saved\n\n")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Summary:\n")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Updated: %d items\n", updated)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Added: %d items\n", added)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Deleted: %d items\n", deleted)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nRun without --dry-run to apply changes.\n")
			return nil
		}

		// Save target roadmap
		if err := saveRoadmapFile(targetPath, targetRoadmap); err != nil {
			return fmt.Errorf("failed to save target roadmap: %w", err)
		}

		// Print summary
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Sync complete:\n")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Updated: %d items\n", updated)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Added: %d items\n", added)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Deleted: %d items\n", deleted)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nTarget roadmap saved to: %s\n", targetPath)

		return nil
	},
}

// MatchResult represents an AI fuzzy match result
type MatchResult struct {
	SourceSlug string  `json:"source_slug"`
	TargetSlug string  `json:"target_slug"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// performSync executes the sync algorithm and returns counts of updated, added, deleted items
func performSync(
	sourceRoadmap *roadmap.Roadmap,
	targetRoadmap *roadmap.Roadmap,
	resolved *config.ResolvedConfig,
	userCfg *config.UserConfig,
	deleteFlag bool,
	yesFlag bool,
	verbose bool,
) (updated, added, deleted int, err error) {
	// Track which items have been matched
	sourceMatched := make(map[string]bool)
	targetMatched := make(map[string]bool)

	// Step 1: Exact slug matching
	for i := range sourceRoadmap.Items {
		sourceItem := &sourceRoadmap.Items[i]
		for j := range targetRoadmap.Items {
			targetItem := &targetRoadmap.Items[j]
			if sourceItem.Slug == targetItem.Slug {
				// Mark as matched
				sourceMatched[sourceItem.Slug] = true
				targetMatched[targetItem.Slug] = true

				// Update target item from source
				updateItemFields(targetItem, sourceItem)
				updated++
				break
			}
		}
	}

	// Step 2: Collect unmatched items
	var unmatchedSource []roadmap.RoadmapItem
	var unmatchedTarget []roadmap.RoadmapItem

	for i := range sourceRoadmap.Items {
		if !sourceMatched[sourceRoadmap.Items[i].Slug] {
			unmatchedSource = append(unmatchedSource, sourceRoadmap.Items[i])
		}
	}

	for i := range targetRoadmap.Items {
		if !targetMatched[targetRoadmap.Items[i].Slug] {
			unmatchedTarget = append(unmatchedTarget, targetRoadmap.Items[i])
		}
	}

	// Step 3: AI fuzzy matching (only if both lists are non-empty)
	if len(unmatchedSource) > 0 && len(unmatchedTarget) > 0 {
		fmt.Fprintf(os.Stderr, "Matching items with AI (this may take a moment)...\n")

		// Render prompt template
		ctx := prompt.NewPromptContext(
			resolved,
			userCfg,
			true, // interactive
			"",   // slug
			".",  // worktreePath
			".",  // mainRepoPath
			"",   // planDir
		)

		// Populate Extra with source and target items
		ctx.Extra["SourceItems"] = unmatchedSource
		ctx.Extra["TargetItems"] = unmatchedTarget

		// Render template
		promptContent, err := prompt.RenderTemplate("prompts/sync-match/sync-match-prompt.md.tmpl", ctx)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to render sync-match prompt: %w", err)
		}

		// Invoke AI
		aiResult, err := ai.InvokeAI(ai.InvokeConfig{
			Tool:    config.ToolExecutable(resolved.Tool),
			Model:   resolved.Model,
			Prompt:  promptContent,
			Verbose: verbose,
		})
		if err != nil {
			// If AI fails, treat all remaining as unmatched (add as new)
			if verbose {
				fmt.Fprintf(os.Stderr, "Warning: AI matching failed: %v\n", err)
				fmt.Fprintf(os.Stderr, "Continuing with exact matches only...\n")
			}
		} else {
			// Parse AI response
			var matches []MatchResult
			jsonStr := ai.ExtractJSON(aiResult.RawOutput)
			if err := json.Unmarshal([]byte(jsonStr), &matches); err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Warning: failed to parse AI response: %v\n", err)
					fmt.Fprintf(os.Stderr, "Continuing with exact matches only...\n")
				}
			} else {
				// Process matches
				for _, match := range matches {
					// Find source and target items
					var sourceItem *roadmap.RoadmapItem
					var targetItem *roadmap.RoadmapItem

					for i := range unmatchedSource {
						if unmatchedSource[i].Slug == match.SourceSlug {
							sourceItem = &unmatchedSource[i]
							break
						}
					}

					for i := range targetRoadmap.Items {
						if targetRoadmap.Items[i].Slug == match.TargetSlug {
							targetItem = &targetRoadmap.Items[i]
							break
						}
					}

					if sourceItem == nil || targetItem == nil {
						continue // Invalid match, skip
					}

					// Check confidence threshold
					if match.Confidence >= 0.7 {
						// Auto-accept high confidence matches
						sourceMatched[sourceItem.Slug] = true
						targetMatched[targetItem.Slug] = true
						updateItemFields(targetItem, sourceItem)
						updated++
					} else if !yesFlag {
						// Low confidence: prompt user
						accept := promptUserForMatch(sourceItem, targetItem, match.Confidence, match.Reason)
						if accept {
							sourceMatched[sourceItem.Slug] = true
							targetMatched[targetItem.Slug] = true
							updateItemFields(targetItem, sourceItem)
							updated++
						}
					} else {
						// --yes flag: accept all matches regardless of confidence
						sourceMatched[sourceItem.Slug] = true
						targetMatched[targetItem.Slug] = true
						updateItemFields(targetItem, sourceItem)
						updated++
					}
				}
			}
		}
	}

	// Step 4: Add unmatched source items as new items
	for i := range sourceRoadmap.Items {
		if !sourceMatched[sourceRoadmap.Items[i].Slug] {
			targetRoadmap.Items = append(targetRoadmap.Items, sourceRoadmap.Items[i])
			targetMatched[sourceRoadmap.Items[i].Slug] = true // Protect from deletion in Step 5
			added++
		}
	}

	// Step 5: Delete unmatched target items if --delete flag is set
	if deleteFlag {
		var newItems []roadmap.RoadmapItem
		for i := range targetRoadmap.Items {
			if targetMatched[targetRoadmap.Items[i].Slug] {
				newItems = append(newItems, targetRoadmap.Items[i])
			} else {
				deleted++
			}
		}
		targetRoadmap.Items = newItems
	}

	return updated, added, deleted, nil
}

// updateItemFields updates target item fields from source item
// For required fields: always overwrite
// For optional fields: if source is non-nil overwrite (including nil to clear), if source is nil preserve target
func updateItemFields(target *roadmap.RoadmapItem, source *roadmap.RoadmapItem) {
	// Required fields: always overwrite
	target.Title = source.Title
	target.Priority = source.Priority
	target.Status = source.Status
	target.Context = source.Context

	// Optional fields: overwrite only if source has a value (including explicit nil to clear)
	// Note: In JSON, omitted fields are nil, but we want to distinguish between
	// "field not set in source" and "field explicitly set to empty in source"
	// For this implementation, we'll overwrite if source field is non-nil
	if source.PRUrl != nil {
		target.PRUrl = source.PRUrl
	}
	if source.Branch != nil {
		target.Branch = source.Branch
	}
}

// promptUserForMatch prompts the user to accept or skip a low-confidence match
func promptUserForMatch(source *roadmap.RoadmapItem, target *roadmap.RoadmapItem, confidence float64, reason string) bool {
	fmt.Fprintf(os.Stderr, "\nLow confidence match (%.0f%%):\n", confidence*100)
	fmt.Fprintf(os.Stderr, "  Source: %s (%s)\n", source.Slug, source.Title)
	fmt.Fprintf(os.Stderr, "  Target: %s (%s)\n", target.Slug, target.Title)
	fmt.Fprintf(os.Stderr, "  Reason: %s\n", reason)
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  1) Accept match (%s → %s)\n", source.Slug, target.Slug)
	fmt.Fprintf(os.Stderr, "  2) Skip (add as new item)\n")
	fmt.Fprintf(os.Stderr, "\nChoice [1/2]: ")

	var choice string
	_, _ = fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	return choice == "1"
}

// saveRoadmapFile saves a roadmap to a specific file path
func saveRoadmapFile(path string, r *roadmap.Roadmap) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil { //nolint:gosec // G301: Standard directory permissions
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Marshal roadmap
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal roadmap: %w", err)
	}

	// Append trailing newline for POSIX compliance
	data = append(data, '\n')

	// Write file
	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: Standard file permissions
		return fmt.Errorf("failed to write roadmap to %s: %w", path, err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(syncCmd)

	// Define flags
	syncCmd.Flags().String("source", ".roadmap.json", "Source roadmap file path")
	syncCmd.Flags().String("target", ".muster/roadmap.json", "Target roadmap file path")
	syncCmd.Flags().Bool("yes", false, "Accept all AI matches without confirmation")
	syncCmd.Flags().Bool("dry-run", false, "Preview changes without saving")
	syncCmd.Flags().Bool("delete", false, "Delete target items not matched by any source item")
}
