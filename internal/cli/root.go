// Package cli provides the Cobra command structure for gomdlint.
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/logging"
)

// BuildInfo holds build-time version information.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCommand creates the root gomdlint command with all subcommands.
func NewRootCommand(info BuildInfo) *cobra.Command {
	var debug bool
	var configPath string
	var color string

	rootCmd := &cobra.Command{
		Use:   "gomdlint",
		Short: "A blisteringly fast, self-fixing Markdown linter",
		Long: `gomdlint is a blisteringly fast, self-fixing Markdown linter written in Go.

It targets CommonMark and GitHub Flavored Markdown (GFM), providing a rich
rule system for both syntax and style checks. gomdlint can automatically fix
many issues while ensuring safety through conflict detection, dry-run mode,
and optional backups.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateColorMode(color); err != nil {
				return err
			}
			level := "info"
			if debug {
				level = "debug"
			}
			// Commands log diagnostics to their own error stream.
			logger := logging.NewWithWriter(cmd.ErrOrStderr(), level)
			cmd.SetContext(logging.WithLogger(cmd.Context(), logger))
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags.
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "path to config file")
	rootCmd.PersistentFlags().StringVar(&color, "color", "auto",
		"colorize output: auto, always, never")

	// Add subcommands.
	rootCmd.AddCommand(newLintCommand(info))
	rootCmd.AddCommand(newRulesCommand())
	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newMigrateCommand())
	rootCmd.AddCommand(newVersionCommand(info))

	// Flag parsing errors are usage errors. Subcommands inherit this.
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return withExitCode(ExitInvalidUsage, err)
	})
	classifyCommandErrors(rootCmd)

	// Apply styled help formatting.
	helpFormatter := NewHelpFormatter(color, os.Stdout)
	helpFormatter.ApplyToCommand(rootCmd)

	return rootCmd
}

// validateColorMode rejects --color values other than auto, always and never.
func validateColorMode(mode string) error {
	switch mode {
	case "auto", "always", "never":
		return nil
	default:
		return usageError("invalid color mode %q; valid modes: auto, always, never", mode)
	}
}

// classifyCommandErrors gives every error a command returns an exit code.
// Commands classify their own failures; anything they miss is an internal
// error. This keeps unclassified errors from Cobra itself, which are usage
// errors, distinguishable from command failures.
func classifyCommandErrors(cmd *cobra.Command) {
	if runE := cmd.RunE; runE != nil {
		cmd.RunE = func(command *cobra.Command, args []string) error {
			return withExitCode(ExitInternalError, runE(command, args))
		}
	}
	if preRunE := cmd.PersistentPreRunE; preRunE != nil {
		cmd.PersistentPreRunE = func(command *cobra.Command, args []string) error {
			return withExitCode(ExitInternalError, preRunE(command, args))
		}
	}
	for _, sub := range cmd.Commands() {
		classifyCommandErrors(sub)
	}
}
