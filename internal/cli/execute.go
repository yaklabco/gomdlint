package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Execute runs the gomdlint command line with args and returns the process
// exit code. Command output goes to stdout. Errors go to stderr, except lint
// findings, which the report already describes.
func Execute(ctx context.Context, info BuildInfo, args []string, stdout, stderr io.Writer) int {
	root := NewRootCommand(info) //nolint:contextcheck // Commands receive ctx through ExecuteContextC below.
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	cmd, err := root.ExecuteContextC(ctx)
	code := ExitCode(err)
	if err == nil || errors.Is(err, ErrLintIssuesFound) {
		return code
	}

	fmt.Fprintf(stderr, "Error: %v\n", err)
	if code == ExitInvalidUsage && cmd != nil {
		fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", cmd.CommandPath())
	}

	return code
}
