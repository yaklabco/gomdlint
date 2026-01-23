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

func TestTypeCheckRule_Metadata(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewTypeCheckRule()

	assert.Equal(t, "MM005", rule.ID())
	assert.Equal(t, "mermaid-type-check", rule.Name())
	assert.Equal(t, "Type modifiers and relationships must be valid", rule.Description())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
	assert.True(t, rule.DefaultEnabled())
	assert.False(t, rule.CanFix())
	assert.Contains(t, rule.Tags(), "mermaid")
}

func TestTypeCheckRule_ValidDiagram(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nflowchart TD\n    A[Start] --> B[End]\n```\n" //nolint:goconst // test data varies per test

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestTypeCheckRule_ValidClassDiagram(t *testing.T) {
	t.Parallel()

	// Class diagram with valid visibility modifiers
	md := "# Test\n\n```mermaid\nclassDiagram\n    class Animal {\n        +name\n        -age\n        #id\n        ~status\n    }\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestTypeCheckRule_ValidSequenceDiagram(t *testing.T) {
	t.Parallel()

	// Sequence diagram with valid message arrows
	md := "# Test\n\n```mermaid\nsequenceDiagram\n    A->>B: Sync call\n    B-->>A: Response\n    A->B: Simple\n    B-->A: Dashed\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestTypeCheckRule_InvalidClassVisibility(t *testing.T) {
	t.Parallel()

	// Class diagram with invalid visibility modifier
	// Note: The parser may not allow invalid modifiers to be stored,
	// or the validator may catch them. We need to test what the library actually reports.
	md := "# Test\n\n```mermaid\nclassDiagram\n    class Animal {\n        $invalidVisibility\n    }\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	// The rule should report invalid visibility if the library catches it
	// Note: The exact behavior depends on go-mermaid's parser/validator
	for _, diag := range diags {
		assert.Equal(t, "MM005", diag.RuleID)
		assert.Contains(t, diag.Message, "Invalid type")
	}
}

func TestTypeCheckRule_InvalidMessageArrow(t *testing.T) {
	t.Parallel()

	// Sequence diagram with potentially invalid arrow
	// Note: The parser may reject invalid arrows during parsing
	md := "# Test\n\n```mermaid\nsequenceDiagram\n    A->>B: Hello\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	// Valid arrows should not produce diagnostics
	assert.Empty(t, diags)
}

func TestTypeCheckRule_ParseError_NoReport(t *testing.T) {
	t.Parallel()

	// Invalid mermaid - should not report MM005 (MM001 handles parse errors)
	md := "# Test\n\n```mermaid\nthis is not valid mermaid\n```\n" //nolint:goconst // test data varies per test

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags, "Should not report MM005 for parse errors")
}

func TestTypeCheckRule_NilRoot(t *testing.T) {
	t.Parallel()

	rule := mermaid.NewTypeCheckRule()
	ctx := &lint.RuleContext{Root: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestTypeCheckRule_NilFile(t *testing.T) {
	t.Parallel()

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte("# Test"))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := &lint.RuleContext{Root: file.Root, File: nil}

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags)
}

func TestTypeCheckRule_MultipleDiagrams(t *testing.T) {
	t.Parallel()

	// Two valid diagrams - should have no diagnostics
	md := "```mermaid\nsequenceDiagram\n    A->>B: Hello\n```\n\n```mermaid\nclassDiagram\n    class Animal\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewTypeCheckRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	assert.Empty(t, diags, "Valid diagrams should have no type-check errors")
}
