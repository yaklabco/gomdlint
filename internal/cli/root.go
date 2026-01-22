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
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			if debug {
				logging.SetLevel("debug")
			}
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
	rootCmd.AddCommand(newLintCommand())
	rootCmd.AddCommand(newRulesCommand())
	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newMigrateCommand())
	rootCmd.AddCommand(newVersionCommand(info))

	// Apply styled help formatting.
	helpFormatter := NewHelpFormatter(color, os.Stdout)
	helpFormatter.ApplyToCommand(rootCmd)

	return rootCmd
}
