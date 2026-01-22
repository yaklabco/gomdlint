package reporter

import (
	"bytes"
	"context"
	"strings"
	"testing"

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
