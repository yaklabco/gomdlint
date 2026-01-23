package mermaid_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules/mermaid"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestInvalidDirectionRule_Metadata(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewInvalidDirectionRule()

	assert.Equal(t, "MM004", rule.ID())
	assert.Equal(t, "mermaid-invalid-direction", rule.Name())
	assert.Equal(t, "Flowchart direction must be valid (TB, TD, BT, RL, LR)", rule.Description())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
	assert.True(t, rule.DefaultEnabled())
	assert.False(t, rule.CanFix())
	assert.Contains(t, rule.Tags(), "mermaid")
}

func TestInvalidDirectionRule_ValidDirections(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		direction string
	}{
		{"TB (top-bottom)", "TB"},
		{"TD (top-down)", "TD"},
		{"BT (bottom-top)", "BT"},
		{"RL (right-left)", "RL"},
		{"LR (left-right)", "LR"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			md := "# Test\n\n```mermaid\nflowchart " + tc.direction + "\n    A[Start] --> B[End]\n```\n"

			parser := goldmark.New(goldmark.FlavorGFM)
			file, err := parser.Parse(context.Background(), "test.md", []byte(md))
			require.NoError(t, err)

			rule := mermaid.NewInvalidDirectionRule()
			ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

			diags, err := rule.Apply(ctx)
			require.NoError(t, err)
			assert.Empty(t, diags, "Valid direction %s should not produce diagnostics", tc.direction)
		})
	}
}

func TestInvalidDirectionRule_InvalidDirection(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nflowchart INVALID\n    A --> B\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewInvalidDirectionRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	assert.Equal(t, "MM004", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "Invalid flowchart direction")
	assert.Contains(t, diags[0].Message, "TB, TD, BT, RL, or LR")
	assert.False(t, diags[0].HasFix())
}

func TestInvalidDirectionRule_ParseError_NoReport(t *testing.T) {
	t.Parallel()

	// Invalid mermaid - should not report MM004 (MM001 handles parse errors)
	md := "# Test\n\n```mermaid\nthis is not valid mermaid\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewInvalidDirectionRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags, "Should not report MM004 for parse errors")
}

func TestInvalidDirectionRule_NonFlowchart_NoReport(t *testing.T) {
	t.Parallel()

	// Sequence diagrams don't have directions
	md := "# Test\n\n```mermaid\nsequenceDiagram\n    A->>B: Hello\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewInvalidDirectionRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags, "Should not report MM004 for non-flowchart diagrams")
}

func TestInvalidDirectionRule_LineNumbers(t *testing.T) {
	t.Parallel()

	// Layout:
	// Line 1: # Test
	// Line 2: (empty)
	// Line 3: ```mermaid
	// Line 4: flowchart WRONG  (content starts here)
	// Line 5:     A --> B
	// Line 6: ```
	md := "# Test\n\n```mermaid\nflowchart WRONG\n    A --> B\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewInvalidDirectionRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	// Diagnostic is attached to the code block node, which has StartLine at the
	// first content line (line 4), not the fence line
	assert.Equal(t, 4, diags[0].StartLine, "Line number should point to first content line of code block")
}

func TestInvalidDirectionRule_NilRoot(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewInvalidDirectionRule()
	ctx := &lint.RuleContext{Root: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestInvalidDirectionRule_NilFile(t *testing.T) {
	t.Parallel()

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte("# Test"))
	require.NoError(t, err)

	rule := mermaid.NewInvalidDirectionRule()
	ctx := &lint.RuleContext{Root: file.Root, File: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestInvalidDirectionRule_MultipleDiagrams(t *testing.T) {
	t.Parallel()

	md := "```mermaid\nflowchart INVALID1\n    A --> B\n```\n\n```mermaid\nflowchart TD\n    C --> D\n```\n\n```mermaid\nflowchart INVALID2\n    E --> F\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewInvalidDirectionRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Len(t, diags, 2, "Should report invalid direction in each flowchart with errors")

	// Verify both errors are about invalid directions
	for _, d := range diags {
		assert.Contains(t, d.Message, "Invalid flowchart direction")
	}
}
