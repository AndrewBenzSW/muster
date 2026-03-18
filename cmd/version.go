package cmd

import (
	"fmt"
	"runtime"

	"github.com/abenz1267/muster/internal/ui"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		info := ui.VersionInfo{
			Version:   version,
			Commit:    commit,
			Date:      date,
			GoVersion: runtime.Version(),
			Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		}

		output, err := ui.FormatVersion(info)
		if err != nil {
			return fmt.Errorf("failed to format version: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), output)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
