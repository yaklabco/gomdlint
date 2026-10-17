package reporter

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/clipperhouse/displaywidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/analysis"
	"github.com/yaklabco/gomdlint/pkg/config"
)

func TestSummaryRenderer_EmptyReport(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := Options{
		Writer: &buf,
		Color:  "never",
	}

	renderer := NewSummaryRenderer(opts)
	report := &analysis.Report{
		Totals: analysis.Totals{},
	}

	err := renderer.Render(context.Background(), report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No issues found")
}

func TestSummaryRenderer_ShowsRulesTable(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := Options{
		Writer:       &buf,
		Color:        "never",
		SummaryOrder: config.SummaryOrderRules,
	}

	renderer := NewSummaryRenderer(opts)
	report := &analysis.Report{
		ByRule: []analysis.RuleAnalysis{
			{RuleID: "MD009", RuleName: "no-trailing-spaces", Issues: 5, Errors: 3, Warnings: 2, Fixable: true},
			{RuleID: "MD001", RuleName: "heading-increment", Issues: 2, Errors: 2, Warnings: 0, Fixable: false},
		},
		ByFile: []analysis.FileAnalysis{
			{Path: "README.md", Issues: 4, Errors: 3, Warnings: 1},
		},
		Totals: analysis.Totals{Issues: 7, Errors: 5, Warnings: 2, Files: 1, FilesWithIssues: 1},
	}

	err := renderer.Render(context.Background(), report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Rules Summary")
	assert.Contains(t, output, "no-trailing-spaces")
	assert.Contains(t, output, "heading-increment")
	assert.Contains(t, output, "Files Summary")
	assert.Contains(t, output, "README.md")
}

func TestSummaryRenderer_FilesFirstOrder(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := Options{
		Writer:       &buf,
		Color:        "never",
		SummaryOrder: config.SummaryOrderFiles,
	}

	renderer := NewSummaryRenderer(opts)
	report := &analysis.Report{
		ByRule: []analysis.RuleAnalysis{
			{RuleID: "MD009", RuleName: "no-trailing-spaces", Issues: 1},
		},
		ByFile: []analysis.FileAnalysis{
			{Path: "README.md", Issues: 1},
		},
		Totals: analysis.Totals{Issues: 1, Files: 1, FilesWithIssues: 1},
	}

	err := renderer.Render(context.Background(), report)
	require.NoError(t, err)

	output := buf.String()
	filesIdx := strings.Index(output, "Files Summary")
	rulesIdx := strings.Index(output, "Rules Summary")

	assert.Greater(t, rulesIdx, filesIdx, "Files should come before Rules when SummaryOrderFiles")
}

func TestSummaryRenderer_ShowsTotals(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := Options{
		Writer: &buf,
		Color:  "never",
	}

	renderer := NewSummaryRenderer(opts)
	report := &analysis.Report{
		Totals: analysis.Totals{
			Issues:          10,
			Errors:          6,
			Warnings:        4,
			Files:           5,
			FilesWithIssues: 3,
		},
	}

	err := renderer.Render(context.Background(), report)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "10")
	assert.Contains(t, output, "6 errors")
	assert.Contains(t, output, "4 warnings")
	assert.Contains(t, output, "3 files")
}

// TestSummaryRenderer_TotalCountsAgree covers the totals line of summary
// output with zero, one and two issues of each severity and files with
// issues. Zero severity counts are left out.
func TestSummaryRenderer_TotalCountsAgree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		totals analysis.Totals
		want   string
	}{
		{
			totals: analysis.Totals{Issues: 2, Errors: 1, Warnings: 1, Files: 1, FilesWithIssues: 1},
			want:   "Total: 2 issues (1 error, 1 warning) in 1 file\n",
		},
		{
			totals: analysis.Totals{Issues: 4, Errors: 2, Warnings: 2, Files: 2, FilesWithIssues: 2},
			want:   "Total: 4 issues (2 errors, 2 warnings) in 2 files\n",
		},
		{
			totals: analysis.Totals{Issues: 1, Warnings: 1, Files: 3, FilesWithIssues: 1},
			want:   "Total: 1 issue (1 warning) in 1 file\n",
		},
		{
			// The breakdown adds up to the issues: info findings count too.
			totals: analysis.Totals{Issues: 3, Errors: 1, Warnings: 1, Infos: 1, Files: 1, FilesWithIssues: 1},
			want:   "Total: 3 issues (1 error, 1 warning, 1 info) in 1 file\n",
		},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		renderer := NewSummaryRenderer(Options{Writer: &buf, Color: "never"})

		require.NoError(t, renderer.Render(context.Background(), &analysis.Report{Totals: tt.totals}))

		assert.True(t, strings.HasSuffix(buf.String(), tt.want), "want the totals %q, got:\n%s", tt.want, buf.String())
	}
}

func TestSummaryRenderer_FixableIndicator(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	opts := Options{
		Writer: &buf,
		Color:  "never",
	}

	renderer := NewSummaryRenderer(opts)
	report := &analysis.Report{
		ByRule: []analysis.RuleAnalysis{
			{RuleID: "MD009", RuleName: "fixable-rule", Issues: 1, Fixable: true},
			{RuleID: "MD001", RuleName: "not-fixable", Issues: 1, Fixable: false},
		},
		Totals: analysis.Totals{Issues: 2},
	}

	err := renderer.Render(context.Background(), report)
	require.NoError(t, err)

	output := buf.String()
	// The fixable rule should have an indicator
	assert.Contains(t, output, "✓")
}

func TestSummaryRenderer_AlignsNonASCIIPaths(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	renderer := NewSummaryRenderer(Options{Writer: &buf, Color: "never", SummaryOrder: config.SummaryOrderFiles})
	report := &analysis.Report{
		ByFile: []analysis.FileAnalysis{
			{Path: "plain.md", Issues: 1, Warnings: 1},
			{Path: "résumé.md", Issues: 1, Warnings: 1},
			{Path: "文档/说明.md", Issues: 1, Warnings: 1},
			{Path: "/srv/" + strings.Repeat("x", 80) + "/doc.md", Issues: 1, Warnings: 1},
		},
		Totals: analysis.Totals{Issues: 4, Warnings: 4, Files: 4, FilesWithIssues: 4},
	}

	require.NoError(t, renderer.Render(context.Background(), report))

	lines := strings.Split(buf.String(), "\n")
	require.Greater(t, len(lines), 7)
	header := lines[2]
	require.True(t, strings.HasPrefix(header, "File "), "unexpected header %q", header)
	for _, row := range lines[4:8] {
		assert.Equal(t, displaywidth.String(header), displaywidth.String(row), "row %q is not aligned with the header", row)
	}
}

func TestSummaryRenderer_TruncatesLongPathsOnCharacterBoundary(t *testing.T) {
	t.Parallel()

	// A path wider than the column made of two-byte letters and of wide
	// characters, so any byte-based cut would split one.
	longPath := "/srv/" + strings.Repeat("é", 40) + strings.Repeat("文", 10) + "/readme.md"
	require.Greater(t, displaywidth.String(longPath), maxFilePathLength)

	var buf bytes.Buffer
	renderer := NewSummaryRenderer(Options{Writer: &buf, Color: "never"})
	report := &analysis.Report{
		ByFile: []analysis.FileAnalysis{{Path: longPath, Issues: 1, Warnings: 1}},
		Totals: analysis.Totals{Issues: 1, Warnings: 1, Files: 1, FilesWithIssues: 1},
	}

	require.NoError(t, renderer.Render(context.Background(), report))

	output := buf.String()
	assert.True(t, utf8.ValidString(output), "summary output must stay valid UTF-8")
	assert.Contains(t, output, "…")
	assert.Contains(t, output, "文/readme.md", "the end of the path is kept")
}

// TestSummaryRenderer_FitsRuleColumnToLongestIdentifier covers rule names of
// any width, which the rules table shows whole. The rule column is as wide
// as the widest identifier, measured in display cells, so every row lines
// up with the header, and the separators span the widest row.
func TestSummaryRenderer_FitsRuleColumnToLongestIdentifier(t *testing.T) {
	t.Parallel()

	// Wider than the table separators, with two-byte letters, so counting
	// bytes instead of cells would misalign the rows.
	longName := "x" + strings.Repeat("é", 70)
	shortName := "short"

	var buf bytes.Buffer
	renderer := NewSummaryRenderer(Options{Writer: &buf, Color: "never"})
	report := &analysis.Report{
		ByRule: []analysis.RuleAnalysis{
			{RuleID: "XX1", RuleName: longName, Issues: 2, Warnings: 2},
			{RuleID: "XX2", RuleName: shortName, Issues: 1, Warnings: 1},
		},
		ByFile: []analysis.FileAnalysis{{Path: "doc.md", Issues: 3, Warnings: 3}},
		Totals: analysis.Totals{Issues: 3, Warnings: 3, Files: 1, FilesWithIssues: 1},
	}

	require.NoError(t, renderer.Render(context.Background(), report))

	output := buf.String()
	assert.True(t, utf8.ValidString(output), "summary output must stay valid UTF-8")
	assert.NotContains(t, output, ellipsis, "no rule name is cut")

	lines := strings.Split(output, "\n")
	require.Greater(t, len(lines), 12)
	header := lines[2]
	assert.True(t, strings.HasPrefix(header, "Rule"+strings.Repeat(" ", displaywidth.String(longName)-len("Rule"))+"   Count"),
		"the rule column is as wide as the longest name, got %q", header)
	assert.True(t, strings.HasPrefix(lines[4], longName+" "), "unexpected row %q", lines[4])
	assert.True(t, strings.HasPrefix(lines[5], shortName+" "), "unexpected row %q", lines[5])
	for _, row := range lines[4:6] {
		assert.Equal(t, displaywidth.String(header), displaywidth.String(row), "row %q is not aligned with the header", row)
	}

	// The separators of both tables span the rules table, the wider one.
	require.Equal(t, "Files Summary", lines[7])
	for _, separator := range []string{lines[1], lines[3], lines[8], lines[10]} {
		assert.True(t, strings.HasPrefix(separator, "─"), "unexpected separator %q", separator)
		assert.Equal(t, displaywidth.String(header), displaywidth.String(separator), "separator %q", separator)
	}
}

// TestSummaryRenderer_RuleFormat covers the rules table, which names rules
// the way --rule-format asks: by name unless the option says otherwise.
func TestSummaryRenderer_RuleFormat(t *testing.T) {
	t.Parallel()

	report := &analysis.Report{
		ByRule: []analysis.RuleAnalysis{
			{RuleID: "MD053", RuleName: "link-image-reference-definitions", Issues: 2, Warnings: 2},
			{RuleID: "MD009", RuleName: "no-trailing-spaces", Issues: 1, Warnings: 1, Fixable: true},
			{RuleID: "XX1", Issues: 1, Warnings: 1}, // A rule without a name is named by its ID.
		},
		Totals: analysis.Totals{Issues: 4, Warnings: 4, Files: 1, FilesWithIssues: 1},
	}

	tests := []struct {
		name   string
		format config.RuleFormat
		want   []string
	}{
		{name: "default", want: []string{"link-image-reference-definitions", "no-trailing-spaces", "XX1"}},
		{name: "name", format: config.RuleFormatName, want: []string{"link-image-reference-definitions", "no-trailing-spaces", "XX1"}},
		{name: "id", format: config.RuleFormatID, want: []string{"MD053", "MD009", "XX1"}},
		{
			name:   "combined",
			format: config.RuleFormatCombined,
			want:   []string{"MD053/link-image-reference-definitions", "MD009/no-trailing-spaces", "XX1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			renderer := NewSummaryRenderer(Options{Writer: &buf, Color: "never", RuleFormat: tt.format})
			require.NoError(t, renderer.Render(context.Background(), report))

			lines := strings.Split(buf.String(), "\n")
			require.Greater(t, len(lines), 6)
			got := make([]string, 0, len(tt.want))
			for _, row := range lines[4:7] {
				got = append(got, strings.Fields(row)[0])
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
