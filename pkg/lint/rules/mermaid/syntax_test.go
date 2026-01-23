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

func TestSyntaxRule_Metadata(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewSyntaxRule()

	assert.Equal(t, "MM001", rule.ID())
	assert.Equal(t, "mermaid-syntax", rule.Name())
	assert.Equal(t, "Mermaid diagram syntax must be valid", rule.Description())
	assert.Equal(t, config.SeverityError, rule.DefaultSeverity())
	assert.True(t, rule.DefaultEnabled())
	assert.False(t, rule.CanFix())
	assert.Contains(t, rule.Tags(), "mermaid")
}

func TestSyntaxRule_ValidDiagram(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nflowchart TD\n    A --> B\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewSyntaxRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestSyntaxRule_InvalidDiagram(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nthis is not valid mermaid\n```\n" //nolint:goconst // test data

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewSyntaxRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	assert.Equal(t, "MM001", diags[0].RuleID)
	assert.Equal(t, config.SeverityError, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "Invalid mermaid syntax")
	assert.False(t, diags[0].HasFix())
}

func TestSyntaxRule_MultipleDiagrams(t *testing.T) {
	t.Parallel()

	md := "```mermaid\nflowchart TD\n    A --> B\n```\n\n```mermaid\ninvalid1\n```\n\n```mermaid\ninvalid2\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewSyntaxRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Len(t, diags, 2, "Should report two invalid diagrams")
}

func TestSyntaxRule_NilRoot(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewSyntaxRule()
	ctx := &lint.RuleContext{Root: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestSyntaxRule_NilFile(t *testing.T) {
	t.Parallel()

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte("# Test"))
	require.NoError(t, err)

	rule := mermaid.NewSyntaxRule()
	ctx := &lint.RuleContext{Root: file.Root, File: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestSyntaxRule_DirectionError_NoReport(t *testing.T) {
	t.Parallel()

	// Direction errors should be reported by MM004, not MM001
	md := "# Test\n\n```mermaid\nflowchart INVALID\n    A --> B\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewSyntaxRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags, "MM001 should not report direction errors - MM004 handles those")
}
