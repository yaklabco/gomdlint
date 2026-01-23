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

func TestDuplicateIDRule_Metadata(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewDuplicateIDRule()

	assert.Equal(t, "MM003", rule.ID())
	assert.Equal(t, "mermaid-duplicate-id", rule.Name())
	assert.Equal(t, "Diagram identifiers must be unique", rule.Description())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
	assert.True(t, rule.DefaultEnabled())
	assert.False(t, rule.CanFix())
	assert.Contains(t, rule.Tags(), "mermaid")
}

func TestDuplicateIDRule_ValidDiagram(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nflowchart TD\n    A[Start] --> B[End]\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestDuplicateIDRule_SequenceDuplicateParticipant(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nsequenceDiagram\n    participant A\n    participant A\n    A->>B: Hello\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	assert.Equal(t, "MM003", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "duplicate participant ID")
	assert.Contains(t, diags[0].Message, "'A'")
	assert.False(t, diags[0].HasFix())
}

func TestDuplicateIDRule_StateDuplicateID(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nstateDiagram-v2\n    state \"Idle\" as S1\n    state \"Running\" as S1\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	assert.Equal(t, "MM003", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "duplicate state ID")
	assert.Contains(t, diags[0].Message, "S1")
}

func TestDuplicateIDRule_ClassDuplicateName(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nclassDiagram\n    class Animal\n    class Animal\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	assert.Equal(t, "MM003", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "duplicate class name")
	assert.Contains(t, diags[0].Message, "Animal")
}

func TestDuplicateIDRule_GitGraphDuplicateBranch(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\ngitGraph\n    commit\n    branch develop\n    branch develop\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	assert.Equal(t, "MM003", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "duplicate branch")
	assert.Contains(t, diags[0].Message, "develop")
}

func TestDuplicateIDRule_ParseError_NoReport(t *testing.T) {
	t.Parallel()

	// Invalid mermaid - should not report MM003 (MM001 handles parse errors)
	md := "# Test\n\n```mermaid\nthis is not valid mermaid\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags, "Should not report MM003 for parse errors")
}

func TestDuplicateIDRule_MultipleDiagrams(t *testing.T) {
	t.Parallel()

	md := "```mermaid\nsequenceDiagram\n    participant A\n    participant A\n```\n\n```mermaid\nclassDiagram\n    class B\n    class B\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Len(t, diags, 2, "Should report duplicate in each diagram")
}

func TestDuplicateIDRule_NilRoot(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewDuplicateIDRule()
	ctx := &lint.RuleContext{Root: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestDuplicateIDRule_NilFile(t *testing.T) {
	t.Parallel()

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte("# Test"))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := &lint.RuleContext{Root: file.Root, File: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestDuplicateIDRule_LineNumbers(t *testing.T) {
	t.Parallel()

	// sequenceDiagram starts at line 4 (after fence), duplicate at relative line 3
	// Document line should be 4 + 3 - 1 = 6
	md := "# Test\n\n```mermaid\nsequenceDiagram\n    participant A\n    participant A\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewDuplicateIDRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	// Verify line number is correctly calculated
	assert.Equal(t, 6, diags[0].StartLine, "Line number should be calculated from code block position")
}
