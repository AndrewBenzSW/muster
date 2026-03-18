package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/abenz1267/muster/internal/ai"
	"github.com/abenz1267/muster/internal/config"
	"github.com/abenz1267/muster/internal/prompt"
	"github.com/abenz1267/muster/internal/roadmap"
	"github.com/abenz1267/muster/internal/ui"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new roadmap item",
	Long: `Add a new roadmap item to the project roadmap.

Batch mode (when --title is provided):
  Generates slug from title, reads context from stdin if --context is "-",
  validates and adds the item directly.

Interactive/AI mode (when --title is not provided):
  Uses AI to generate the item details from user input. Requires a TTY.
  Shows the generated item for confirmation before adding.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		title, _ := cmd.Flags().GetString("title")
		priorityStr, _ := cmd.Flags().GetString("priority")
		statusStr, _ := cmd.Flags().GetString("status")
		contextStr, _ := cmd.Flags().GetString("context")
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Load configuration following cmd/code.go:38-54 pattern
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

		// Resolve config for "add" step
		resolved, err := config.ResolveStep("add", projectCfg, userCfg)
		if err != nil {
			return fmt.Errorf("failed to resolve config: %w", err)
		}

		// Verbose logging
		if verbose {
			fmt.Fprintf(os.Stderr, "Using: tool=%s provider=%s model=%s\n", resolved.Tool, resolved.Provider, resolved.Model)
		}

		// Load existing roadmap
		rm, err := roadmap.LoadRoadmap(".")
		if err != nil {
			if errors.Is(err, roadmap.ErrRoadmapParse) {
				return fmt.Errorf("roadmap file is malformed: %w", err)
			}
			return fmt.Errorf("failed to load roadmap: %w", err)
		}

		// Batch mode when --title is provided
		if title != "" {
			return runBatchAdd(rm, title, priorityStr, statusStr, contextStr, verbose)
		}

		// Interactive/AI mode
		return runInteractiveAdd(cmd, rm, resolved, userCfg, verbose)
	},
}

// runBatchAdd handles batch mode item creation
func runBatchAdd(rm *roadmap.Roadmap, title, priorityStr, statusStr, contextStr string, verbose bool) error {
	// Generate slug from title
	slug := roadmap.GenerateSlug(title)
	if slug == "" {
		return fmt.Errorf("failed to generate slug from title")
	}

	// Handle context from stdin if --context is "-"
	var context string
	if contextStr == "-" {
		// Read from stdin with 1MB limit
		const maxContextSize = 1024 * 1024 // 1MB
		limited := io.LimitReader(os.Stdin, maxContextSize+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return fmt.Errorf("failed to read context from stdin: %w", err)
		}
		if len(data) == 0 {
			return fmt.Errorf("context from stdin is empty")
		}
		if len(data) > maxContextSize {
			return fmt.Errorf("context from stdin exceeds 1MB limit")
		}
		context = strings.TrimSpace(string(data))
		if context == "" {
			return fmt.Errorf("context from stdin is empty after trimming whitespace")
		}
	} else {
		context = contextStr
	}

	// Validate required context
	if strings.TrimSpace(context) == "" {
		return fmt.Errorf("context is required")
	}

	// Create item
	item := roadmap.RoadmapItem{
		Slug:     slug,
		Title:    title,
		Priority: roadmap.Priority(priorityStr),
		Status:   roadmap.Status(statusStr),
		Context:  context,
	}

	// Validate and add item
	if err := rm.AddItem(item); err != nil {
		return fmt.Errorf("failed to add item: %w", err)
	}

	// Save roadmap
	if err := roadmap.SaveRoadmap(".", rm); err != nil {
		return fmt.Errorf("failed to save roadmap: %w", err)
	}

	// Format confirmation
	fmt.Printf("Added roadmap item: %s\n", slug)
	fmt.Printf("  Title: %s\n", title)
	fmt.Printf("  Priority: %s\n", priorityStr)
	fmt.Printf("  Status: %s\n", statusStr)

	return nil
}

// runInteractiveAdd handles interactive/AI mode item creation
func runInteractiveAdd(cmd *cobra.Command, rm *roadmap.Roadmap, resolved *config.ResolvedConfig, userCfg *config.UserConfig, verbose bool) error {
	// Check if running in TTY
	if !ui.IsInteractive() {
		return fmt.Errorf("interactive mode requires a terminal (TTY). Use --title flag for batch mode")
	}

	// Get user input
	fmt.Fprintf(os.Stderr, "Describe the roadmap item you want to add:\n")
	userInput, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}
	if len(userInput) == 0 {
		return fmt.Errorf("no input provided")
	}

	// Create prompt context
	ctx := prompt.NewPromptContext(
		resolved,
		userCfg,
		true, // interactive
		"",   // slug
		".",  // worktreePath
		".",  // mainRepoPath
		"",   // planDir
	)
	ctx.Extra["UserInput"] = string(userInput)

	// Render template
	promptContent, err := prompt.RenderTemplate("prompts/add-item/add-item-prompt.md.tmpl", ctx)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Template path: prompts/add-item/add-item-prompt.md.tmpl\n")
		fmt.Fprintf(os.Stderr, "Invocation command: %s --print --plugin-dir <tmpdir>\n", resolved.Tool)
	}

	// Invoke AI
	result, err := ai.InvokeAI(ai.InvokeConfig{
		Tool:    resolved.Tool,
		Prompt:  promptContent,
		Verbose: verbose,
	})
	if err != nil {
		return fmt.Errorf("AI invocation failed: %w", err)
	}

	// Parse JSON response
	var item roadmap.RoadmapItem
	if err := json.Unmarshal([]byte(result.RawOutput), &item); err != nil {
		return fmt.Errorf("failed to parse AI response as JSON: %w\nRaw response: %s", err, result.RawOutput)
	}

	// Show generated item to user via picker (confirm/edit/cancel)
	fmt.Fprintf(os.Stderr, "\nGenerated roadmap item:\n")
	fmt.Fprintf(os.Stderr, "  Slug: %s\n", item.Slug)
	fmt.Fprintf(os.Stderr, "  Title: %s\n", item.Title)
	fmt.Fprintf(os.Stderr, "  Priority: %s\n", item.Priority)
	fmt.Fprintf(os.Stderr, "  Status: %s\n", item.Status)
	fmt.Fprintf(os.Stderr, "  Context: %s\n\n", item.Context)

	options := []ui.PickerOption{
		{Label: "Confirm - Add this item", Value: "confirm"},
		{Label: "Cancel - Don't add", Value: "cancel"},
	}

	choice, err := ui.DefaultPicker.Show("What would you like to do?", options, ui.DefaultPickerConfig())
	if err != nil {
		return fmt.Errorf("picker failed: %w", err)
	}

	if choice != "confirm" {
		fmt.Fprintf(os.Stderr, "Cancelled.\n")
		return nil
	}

	// Validate and add item
	if err := rm.AddItem(item); err != nil {
		return fmt.Errorf("failed to add item: %w", err)
	}

	// Save roadmap
	if err := roadmap.SaveRoadmap(".", rm); err != nil {
		return fmt.Errorf("failed to save roadmap: %w", err)
	}

	fmt.Printf("Added roadmap item: %s\n", item.Slug)

	return nil
}

func init() {
	rootCmd.AddCommand(addCmd)

	// Define flags
	addCmd.Flags().String("title", "", "Item title (batch mode)")
	addCmd.Flags().String("priority", string(roadmap.PriorityMedium), "Priority: high, medium, low, lower")
	addCmd.Flags().String("status", string(roadmap.StatusPlanned), "Status: planned, in_progress, completed, blocked")
	addCmd.Flags().String("context", "", "Context/description (use '-' to read from stdin)")
}
