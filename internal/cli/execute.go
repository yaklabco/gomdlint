package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Execute runs the gomdlint command line with args and returns the process
// exit code. Command output goes to stdout. Errors go to stderr, except lint
// findings, which the report already describes.
func Execute(ctx context.Context, info BuildInfo, args []string, stdout, stderr io.Writer) int {
	root := NewRootCommand(info) //nolint:contextcheck // Commands receive ctx through execute below.
	return execute(ctx, root, args, stdout, stderr)
}

// execute runs root with args and maps the outcome to an exit code. A panic
// in the command becomes an internal error: Go would otherwise exit with
// status 2, which means warnings under --strict.
func execute(ctx context.Context, root *cobra.Command, args []string, stdout, stderr io.Writer) int {
	code := ExitInternalError
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				fmt.Fprintf(stderr, "Error: internal error: %v\n", recovered)
				code = ExitInternalError
			}
		}()
		code = run(ctx, root, args, stdout, stderr)
	}()
	return code
}

// run executes root and reports any error it returns.
func run(ctx context.Context, root *cobra.Command, args []string, stdout, stderr io.Writer) int {
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
