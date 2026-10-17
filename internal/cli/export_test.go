package cli

import (
	"context"
	"io"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// ExecuteWithRegistry runs the command line like Execute, except that the
// lint command checks the rules in registry instead of the built-in rules.
func ExecuteWithRegistry(ctx context.Context, registry *lint.Registry, args []string, stdout, stderr io.Writer) int {
	root := newRootCommand(BuildInfo{Version: "test"}, registry) //nolint:contextcheck // Commands receive ctx through execute below.
	return execute(ctx, root, args, stdout, stderr)
}
