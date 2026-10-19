package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/clipperhouse/displaywidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// ruleLine returns the line of the rules listing whose rule, indented under
// its category, starts with identifier.
func ruleLine(t *testing.T, listing, identifier string) string {
	t.Helper()

	for _, line := range strings.Split(listing, "\n") {
		if strings.HasPrefix(line, "  "+identifier+" ") {
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
		header     string
		identifier string
	}{
		{format: "name", header: "NAME", identifier: "no-trailing-spaces"},
		{format: "id", header: "ID", identifier: "MD009"},
		{format: "combined", header: "RULE", identifier: "MD009/no-trailing-spaces"},
	}
	md009, ok := lint.DefaultRegistry.Meta("MD009")
	require.True(t, ok)
	require.NotEmpty(t, md009.Description)

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()

			// The test runs in the package directory, so --defaults keeps
			// the repository's own configuration out of the listing.
			got := execute(t, "rules", "--defaults", "--rule-format", tt.format)

			require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
			assert.NotContains(t, got.stdout, "INFO")
			assert.NotContains(t, got.stdout, "\x1b[", "no styling in plain output")
			assert.True(t, strings.HasPrefix(got.stdout, tt.header+" "), "starts with a header:\n%s", got.stdout)

			line := ruleLine(t, got.stdout, tt.identifier)
			assert.Contains(t, line, "warning")
			assert.Contains(t, line, "yes")
			assert.True(t, strings.HasSuffix(line, "  "+md009.Description), "the catalog description: %q", line)
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

// TestLintHelpShowsRuleNamesAndTags covers the help for the rule flags,
// which take rules by name or ID and markdownlint's tags.
func TestLintHelpShowsRuleNamesAndTags(t *testing.T) {
	t.Parallel()

	got := execute(t, "lint", "--help")

	assert.Contains(t, got.stdout, "gomdlint lint --disable line-length")
	assert.Contains(t, got.stdout, "gomdlint lint --disable whitespace")
	assert.Contains(t, got.stdout, "rules to disable, by name, ID or tag")
}

// TestLintTableKeepsCharactersWhole covers table output for a path of wide
// characters and for a message of two-byte letters too long for its
// column. Both used to be cut by bytes, in the middle of a character, which
// made the output invalid UTF-8. A writer that is not a terminal gets a
// table 100 columns wide.
func TestLintTableKeepsCharactersWhole(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{
			name:    "wide path",
			path:    "docs/开发使用指南.md",
			content: "# Title\n\n[unused]: https://example.com\n",
			want:    " docs/开发使用指南.md ",
		},
		{
			name:    "long message",
			path:    "doc.md",
			content: "# Title\n\nText.\n\n[" + strings.Repeat("äöü", 30) + "]: https://example.com\n",
			want:    "Unused reference definition [äöü",
		},
	}

	for _, tt := range tests {
		for _, ruleFormat := range []string{"name", "id", "combined"} {
			t.Run(tt.name+" "+ruleFormat, func(t *testing.T) {
				inProject(t, map[string]string{tt.path: tt.content})

				got := execute(t, "lint", "--color", "never", "--format", "table", "--rule-format", ruleFormat, tt.path)

				require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
				assert.Contains(t, got.stdout, tt.want)
				for _, line := range strings.Split(got.stdout, "\n") {
					if assert.True(t, utf8.ValidString(line), "line %q is not valid UTF-8", line) {
						assert.LessOrEqual(t, displaywidth.String(line), 100, "line %q is wider than the table", line)
					}
				}
			})
		}
	}
}
