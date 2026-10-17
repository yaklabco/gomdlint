package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestExecutePanicIsInternalError covers a command that panics. Go exits
// a crashed program with status 2, which the exit code contract reserves
// for warnings under --strict, so the panic must map to 70 instead.
func TestExecutePanicIsInternalError(t *testing.T) {
	t.Parallel()

	root := &cobra.Command{
		Use:           "gomdlint",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(*cobra.Command, []string) error {
			panic("boom")
		},
	}

	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), root, nil, &stdout, &stderr)

	assert.Equal(t, ExitInternalError, code)
	assert.Contains(t, stderr.String(), "internal error: boom")
}
