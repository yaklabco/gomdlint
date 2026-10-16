package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  `Print the version, commit hash, and build date of gomdlint.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "gomdlint %s (commit %s, built %s)\n",
				info.Version, info.Commit, info.Date)
			if err != nil {
				return withExitCode(ExitIOError, fmt.Errorf("write version: %w", err))
			}
			return nil
		},
	}

	return cmd
}
