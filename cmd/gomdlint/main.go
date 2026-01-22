// Package main is the entry point for the gomdlint CLI.
package main

import (
	"errors"
	"os"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/internal/logging"

	// Import rules package to register built-in rules via init().
	_ "github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// Build-time variables set by GoReleaser via ldflags.
//
//nolint:gochecknoglobals // Version variables must be package-level for ldflags injection
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Build and execute the root command.
	info := cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	rootCmd := cli.NewRootCommand(info)

	if err := rootCmd.Execute(); err != nil {
		// Don't log ErrLintIssuesFound - it's just a signal for exit code.
		if !errors.Is(err, cli.ErrLintIssuesFound) {
			logger := logging.Default()
			logger.Error("command failed", logging.FieldError, err)
		}
		return 1
	}

	return 0
}
