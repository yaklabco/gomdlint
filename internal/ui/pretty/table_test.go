package pretty_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// tableRuleColumn returns the RULE column of the rows of a table: the
// identifier at the offset of the RULE header in each line between the
// heavy separators.
func tableRuleColumn(t *testing.T, table string) []string {
	t.Helper()

	lines := strings.Split(table, "\n")
	require.Greater(t, len(lines), 2, "table:\n%s", table)
	start := strings.Index(lines[0], "RULE")
	require.GreaterOrEqual(t, start, 0, "no RULE header in:\n%s", table)

	var rules []string
	for _, line := range lines[2:] {
		if strings.HasPrefix(line, "=") {
			break
		}
		require.Greater(t, len(line), start, "row %q ends before the RULE column", line)
		rules = append(rules, strings.Fields(line[start:])[0])
	}
	return rules
}

func TestTableFormatter_RuleColumnFollowsRuleFormat(t *testing.T) {
	file := runner.FileOutcome{
		Path: "doc.md",
		Result: &lint.PipelineResult{FileResult: &lint.FileResult{Diagnostics: []lint.Diagnostic{
			{
				RuleID: "MD009", RuleName: "no-trailing-spaces", Message: "Trailing whitespace",
				Severity: config.SeverityWarning, StartLine: 3, StartColumn: 22,
			},
			{
				RuleID: "MD053", RuleName: "link-image-reference-definitions", Message: "Unused reference definition [x]",
				Severity: config.SeverityWarning, StartLine: 17, StartColumn: 1,
			},
		}}},
	}
	result := &runner.Result{Files: []runner.FileOutcome{file}}

	tests := []struct {
		format config.RuleFormat
		want   []string
	}{
		{format: config.RuleFormatName, want: []string{"no-trailing-spaces", "link-image-reference-definitions"}},
		{format: config.RuleFormatID, want: []string{"MD009", "MD053"}},
		{format: config.RuleFormatCombined, want: []string{"MD009/no-trailing-spaces", "MD053/link-image-reference-definitions"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			// A narrow terminal shortens messages and paths, never rules.
			for _, width := range []int{200, 60} {
				formatter := pretty.NewTableFormatter(pretty.NewStyles(false), false, width, tt.format)

				assert.Equal(t, tt.want, tableRuleColumn(t, formatter.FormatTable(result)), "table at width %d", width)
				assert.Equal(t, tt.want, tableRuleColumn(t, formatter.FormatFileTable(file)), "per-file table at width %d", width)
			}
		})
	}
}
