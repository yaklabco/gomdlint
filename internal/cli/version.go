package cli

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/logging"
)

func newVersionCommand(info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  `Print the version, commit hash, and build date of gomdlint.`,
		Run: func(_ *cobra.Command, _ []string) {
			logger := log.NewWithOptions(os.Stdout, log.Options{
				ReportTimestamp: false,
				ReportCaller:    false,
			})
			logger.SetLevel(log.InfoLevel)

			logger.Info("gomdlint",
				logging.FieldVersion, info.Version,
				logging.FieldCommit, info.Commit,
				logging.FieldBuilt, info.Date,
			)
		},
	}

	return cmd
}
