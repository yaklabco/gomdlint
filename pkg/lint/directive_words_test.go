package lint_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// everyLineRule reports every line of a file under the ID and name it is
// given, as a custom rule may.
type everyLineRule struct {
	optionRule

	tags []string
}

func (r *everyLineRule) Tags() []string { return r.tags }

func (r *everyLineRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	diags := make([]lint.Diagnostic, 0, len(ctx.File.Lines))
	for line := 1; line <= len(ctx.File.Lines); line++ {
		diags = append(diags, lint.Diagnostic{
			RuleID: r.ID(), RuleName: r.Name(), Message: "line", StartLine: line, StartColumn: 1, EndLine: line, EndColumn: 1,
		})
	}
	return diags, nil
}

// TestEngineDirectiveNoticesSuggestWordsACommentHolds covers suggestions for
// a custom rule whose name holds whitespace, and for a tag that does.
// markdownlint 0.41.1 accepts such names and tags, but splits a comment's
// words at whitespace, so no comment word names them. The notice offers the
// rule's ID instead of its name, and leaves the tag out. A configure-file
// key may hold the whitespace, so the tag is offered there.
func TestEngineDirectiveNoticesSuggestWordsACommentHolds(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(&everyLineRule{optionRule: optionRule{id: "CUSTOM001", name: "my rule"}})
	registry.Describe(lint.RuleMeta{ID: "CUSTOM001", Name: "my rule", Tags: []string{"odd tags"}})
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	notices := func(t *testing.T, input string) []lint.Notice {
		t.Helper()
		result, err := engine.LintFile(context.Background(), "test.md", []byte(input), config.NewConfig())
		require.NoError(t, err)
		return result.Notices
	}

	found := notices(t, "<!-- markdownlint-disable my-rul -->\ntext\n")
	require.Len(t, found, 1)
	assert.Equal(t, []string{"CUSTOM001"}, found[0].Suggestions)
	assert.Equal(t, `markdownlint-disable: unknown rule "my-rul" (did you mean "CUSTOM001"?); it is ignored`, found[0].Message)

	found = notices(t, "<!-- markdownlint-disable odd-tag -->\ntext\n")
	require.Len(t, found, 1)
	assert.Empty(t, found[0].Suggestions, "no comment word names the tag")

	found = notices(t, "<!-- markdownlint-configure-file {\"odd-tag\": false} -->\ntext\n")
	require.Len(t, found, 1)
	assert.Equal(t, []string{"odd tags"}, found[0].Suggestions, "a configure-file key names the tag")
}

// TestEngineDirectiveWordsFoldFullUpperCases covers the characters whose
// upper case under JavaScript's toUpperCase is longer than one character
// and not ASCII, such as U+0149, whose upper case is U+02BC and N.
// markdownlint 0.41.1 upper-cases a rule's name and a comment's word so
// before it compares them, so a comment that writes the name in either
// form names the rule, as markdownlint 0.41.1 does with the same custom
// rule.
func TestEngineDirectiveWordsFoldFullUpperCases(t *testing.T) {
	tests := []struct{ name, word string }{
		{name: "\u0149-rule", word: "\u02bcN-RULE"},
		{name: "\u02bcn-rule", word: "\u0149-rule"},
		{name: "\u1f80-rule", word: "\u1f08\u0399-rule"},
		{name: "\ufb13-rule", word: "\u0544\u0546-RULE"},
		{name: "\u0390-rule", word: "\u0399\u0308\u0301-rule"},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			registry := lint.NewRegistry()
			registry.Register(&everyLineRule{optionRule: optionRule{id: "CUSTOM002", name: tt.name}})

			result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
				context.Background(), "test.md", []byte("<!-- markdownlint-disable "+tt.word+" -->\ntext\n"), config.NewConfig())
			require.NoError(t, err)

			assert.Empty(t, result.Diagnostics, "the comment names the rule")
			assert.Empty(t, result.Notices)
		})
	}
}

// TestEngineMarkdownlintDefaultCoversCustomMDLRules covers a custom rule
// under an ID that starts with MDL, the namespace of gomdlint's own rules.
// markdownlint 0.41.1 applies the default key to every rule it runs,
// custom ones included, and only gomdlint's own rules, which markdownlint
// lacks, stay off. So a configure-file comment that sets default to true
// leaves the custom MDL900 on, as it leaves ACME1.
func TestEngineMarkdownlintDefaultCoversCustomMDLRules(t *testing.T) {
	registry := lint.NewRegistry()
	for _, id := range []string{"MDL900", "ACME1"} {
		registry.Register(&everyLineRule{optionRule: optionRule{id: id, name: "custom-" + id}})
	}
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(context.Background(), "test.md",
		[]byte("<!-- markdownlint-configure-file {\"default\": true} -->\ntext\n"), config.NewConfig())
	require.NoError(t, err)

	reported := map[string]bool{}
	for _, diag := range result.Diagnostics {
		reported[diag.RuleID] = true
	}
	assert.Equal(t, map[string]bool{"MDL900": true, "ACME1": true}, reported)
	assert.False(t, lint.IsGomdlintOnlyRule("MDL900"))
	assert.True(t, lint.IsGomdlintOnlyRule("MDL001"))
}
