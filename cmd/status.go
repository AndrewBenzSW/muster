package cmd

import (
	"errors"
	"fmt"

	"github.com/abenz1267/muster/internal/roadmap"
	"github.com/abenz1267/muster/internal/ui"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [slug]",
	Short: "Display roadmap status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load roadmap from current directory
		rm, err := roadmap.LoadRoadmap(".")
		if err != nil {
			// Handle parse errors with file-specific message
			if errors.Is(err, roadmap.ErrRoadmapParse) {
				return fmt.Errorf("roadmap file is malformed: %w", err)
			}
			return fmt.Errorf("failed to load roadmap: %w", err)
		}

		// If slug argument is provided, show detail view
		if len(args) > 0 {
			slug := args[0]
			item := rm.FindBySlug(slug)
			if item == nil {
				return fmt.Errorf("roadmap item with slug %q not found", slug)
			}

			output, err := ui.FormatRoadmapDetail(*item)
			if err != nil {
				return fmt.Errorf("failed to format roadmap detail: %w", err)
			}

			if _, err := fmt.Fprintln(cmd.OutOrStdout(), output); err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}
			return nil
		}

		// No slug provided, show table view of all items
		output, err := ui.FormatRoadmapTable(rm.Items)
		if err != nil {
			return fmt.Errorf("failed to format roadmap table: %w", err)
		}

		if _, err := fmt.Fprintln(cmd.OutOrStdout(), output); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
