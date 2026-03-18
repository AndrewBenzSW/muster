package cmd

import (
	"fmt"

	"github.com/abenz1267/muster/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "muster",
	Short: "AI-assisted development workflow orchestration",
	Long: `muster consolidates AI-assisted development workflows into a single tool.
It manages the full lifecycle of roadmap items — from planning through
implementation, review, and release — using AI coding agents orchestrated
through configurable pipelines.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}

		if format != "" {
			// Validate format
			if format != "json" && format != "table" {
				return fmt.Errorf("invalid format: %s (must be 'json' or 'table')", format)
			}

			// Set output mode based on format flag
			if format == "json" {
				ui.SetOutputMode(ui.JSONMode)
			} else {
				ui.SetOutputMode(ui.TableMode)
			}
		} else {
			// Auto-detect based on TTY
			if ui.IsInteractive() {
				ui.SetOutputMode(ui.TableMode)
			} else {
				ui.SetOutputMode(ui.JSONMode)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringP("format", "f", "", "Output format (json|table)")
	// Note: verbose flag is reserved for future use to enable detailed logging
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
