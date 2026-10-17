package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// ruleLine returns the line of the rules listing that starts with identifier.
func ruleLine(t *testing.T, listing, identifier string) string {
	t.Helper()

	for _, line := range strings.Split(listing, "\n") {
		if strings.HasPrefix(line, identifier+" ") {
			return line
		}
	}
	t.Fatalf("no line for %s in:\n%s", identifier, listing)
	return ""
}

func TestRulesCommandPrintsPlainTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format     string
		identifier string
	}{
		{format: "name", identifier: "no-trailing-spaces"},
		{format: "id", identifier: "MD009"},
		{format: "combined", identifier: "MD009/no-trailing-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()

			got := execute(t, "rules", "--rule-format", tt.format)

			require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
			assert.NotContains(t, got.stdout, "INFO")
			assert.NotContains(t, got.stdout, "\x1b[", "no styling in plain output")
			assert.True(t, strings.HasPrefix(got.stdout, "RULE "), "starts with a header:\n%s", got.stdout)

			line := ruleLine(t, got.stdout, tt.identifier)
			assert.Contains(t, line, "warning")
			assert.Contains(t, line, "yes")
			assert.Contains(t, line, "trailing spaces")
		})
	}
}

func TestVersionCommandPrintsPlainVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := cli.Execute(context.Background(),
		cli.BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2024-01-01"},
		[]string{"version"}, &stdout, &stderr)

	require.Equal(t, cli.ExitSuccess, code, stderr.String())
	assert.Equal(t, "gomdlint 1.2.3 (commit abc123, built 2024-01-01)\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestLintHelpNamesGomdlint(t *testing.T) {
	t.Parallel()

	got := execute(t, "lint", "--help")

	assert.NotContains(t, got.stdout, " mdlint ")
	assert.Contains(t, got.stdout, "gomdlint lint docs/")
	assert.Contains(t, got.stdout, "instead of the project config")
}
