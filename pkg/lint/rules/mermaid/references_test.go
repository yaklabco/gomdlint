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

func TestUndefinedReferenceRule_Metadata(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewUndefinedReferenceRule()

	assert.Equal(t, "MM002", rule.ID())
	assert.Equal(t, "mermaid-undefined-reference", rule.Name())
	assert.Equal(t, "All referenced nodes/participants must be defined", rule.Description())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
	assert.True(t, rule.DefaultEnabled())
	assert.False(t, rule.CanFix())
	assert.Contains(t, rule.Tags(), "mermaid")
}

func TestUndefinedReferenceRule_ValidDiagram(t *testing.T) {
	t.Parallel()

	// Flowchart with all nodes implicitly defined through links
	md := "# Test\n\n```mermaid\nflowchart TD\n    A[Start] --> B[End]\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestUndefinedReferenceRule_GitGraphUndefinedBranch(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\ngitGraph\n    commit\n    branch develop\n    checkout develop\n    commit\n    checkout undefined-branch\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	assert.Equal(t, "MM002", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "undefined branch")
	assert.Contains(t, diags[0].Message, "undefined-branch")
	assert.False(t, diags[0].HasFix())
}

func TestUndefinedReferenceRule_StateUndefinedReference(t *testing.T) {
	t.Parallel()

	// State diagram without explicit state definitions - transitions reference undefined states
	md := "# Test\n\n```mermaid\nstateDiagram-v2\n    [*] --> Running\n    Running --> Stopped\n    Stopped --> [*]\n```\n" //nolint:dupword // test data

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	// State diagram validator reports undefined state references
	assert.NotEmpty(t, diags, "Should report undefined state references")

	for _, diag := range diags {
		assert.Equal(t, "MM002", diag.RuleID)
		assert.Contains(t, diag.Message, "undefined")
	}
}

func TestUndefinedReferenceRule_ParseError_NoReport(t *testing.T) {
	t.Parallel()

	// Invalid mermaid - should not report MM002 (MM001 handles parse errors)
	md := "# Test\n\n```mermaid\nthis is not valid mermaid\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags, "Should not report MM002 for parse errors")
}

func TestUndefinedReferenceRule_MultipleDiagrams(t *testing.T) {
	t.Parallel()

	md := "```mermaid\ngitGraph\n    commit\n    checkout undefined1\n```\n\n```mermaid\ngitGraph\n    commit\n    checkout undefined2\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Len(t, diags, 2, "Should report two undefined branch references")

	// Verify both different branches are reported
	messages := make([]string, len(diags))
	for i, d := range diags {
		messages[i] = d.Message
	}
	assert.Contains(t, messages[0], "undefined1")
	assert.Contains(t, messages[1], "undefined2")
}

func TestUndefinedReferenceRule_NilRoot(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := &lint.RuleContext{Root: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestUndefinedReferenceRule_NilFile(t *testing.T) {
	t.Parallel()

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte("# Test"))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := &lint.RuleContext{Root: file.Root, File: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestUndefinedReferenceRule_LineNumbers(t *testing.T) {
	t.Parallel()

	// gitGraph starts at line 4 (after fence), checkout undefined at relative line 3
	// Document line should be 4 + 3 - 1 = 6
	md := "# Test\n\n```mermaid\ngitGraph\n    commit\n    checkout undefined\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	// Verify line number is correctly calculated
	assert.Equal(t, 6, diags[0].StartLine, "Line number should be calculated from code block position")
}
