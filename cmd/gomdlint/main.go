// Package main is the entry point for the gomdlint CLI.
package main

import (
	"context"
	"io"
	"os"

	"github.com/yaklabco/gomdlint/internal/cli"

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
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command line in args and returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	info := cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}

	return cli.Execute(ctx, info, args, stdout, stderr)
}
