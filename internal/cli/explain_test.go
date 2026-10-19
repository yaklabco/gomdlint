package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestExplainGolden explains rules, tags and categories in the fixture
// project of the rules goldens, and compares the transcript of each with
// testdata/golden/<name>.golden. A rule is named by ID, name or
// markdownlint name, in any case. A tag or a category lists its rules.
func TestExplainGolden(t *testing.T) {
	skipUnlessSlashPaths(t)

	tests := []struct {
		name string
		args []string
	}{
		// no-trailing-spaces, which the project reports as errors.
		{name: "explain-id", args: []string{"MD009"}},
		// line-length, which the project turns off.
		{name: "explain-name", args: []string{"line-length"}},
		{name: "explain-name-defaults", args: []string{"Line-Length", "--defaults"}},
		// first-line-h1 is markdownlint's second name for first-line-heading.
		{name: "explain-alias", args: []string{"FIRST-LINE-H1"}},
		// ul-style is markdownlint's first name for unordered-list-style.
		{name: "explain-markdownlint-name", args: []string{"ul-style"}},
		// A rule that markdownlint does not have.
		{name: "explain-gomdlint-rule", args: []string{"table-alignment"}},
		// A rule without options.
		{name: "explain-no-options", args: []string{"no-reversed-links"}},
		{name: "explain-tag", args: []string{"whitespace"}},
		{name: "explain-category", args: []string{"Blockquotes"}},
		{name: "explain-tag-and-category", args: []string{"links"}},
		{name: "explain-typo", args: []string{"line-lenght"}}, //nolint:misspell // The typo is what the case tests.
		// Typos of a category alone, and of a category and a tag.
		{name: "explain-category-typo", args: []string{"lyout"}},
		{name: "explain-category-and-tag-typo", args: []string{"blockquots"}},
		{name: "explain-unknown", args: []string{"frobnicate"}},
		{name: "explain-no-argument"},
		// An option that allows only some values, and one that markdownlint
		// lacks.
		{name: "explain-json", args: []string{"heading-style", "--format", "json"}},
		{name: "explain-tag-json", args: []string{"blockquote", "--format", "json"}},
		{name: "explain-markdown", args: []string{"line-length", "--format", "markdown"}},
		{name: "explain-tag-markdown", args: []string{"blockquote", "--format", "markdown"}},
		{name: "explain-invalid-format", args: []string{"MD009", "--format", "html"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := inRulesProject(t)

			got := execute(t, append([]string{"explain", "--color", "never"}, tt.args...)...)

			assertGolden(t, testdataPath("golden", tt.name+".golden"), transcript(t, got, dir))
		})
	}
}

// TestExplainMarkdownDoesNotReadTheConfiguration covers explain --format
// markdown, whose page does not depend on the configuration: it does not
// read it, so a configuration that lint could not load does not stop it.
// The text page shows the configured state, so it fails as lint does.
func TestExplainMarkdownDoesNotReadTheConfiguration(t *testing.T) {
	inProject(t, map[string]string{".gomdlint.yml": "rules:\n  MD013: false\n  line-length: false\n"})

	got := execute(t, "explain", "--format", "markdown", "MD009")
	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Contains(t, got.stdout, "# no-trailing-spaces\n")

	got = execute(t, "explain", "--format", "markdown", "whitespace")
	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)

	got = execute(t, "explain", "MD009")
	assert.Equal(t, cli.ExitConfigError, got.code)
	assert.Empty(t, got.stdout)
}

// TestRuleFormatVariableOnlyForTextListings covers an invalid
// GOMDLINT_RULE_FORMAT under --defaults, which reads no configuration: only
// a text listing, which names rules as the variable says, fails on it.
func TestRuleFormatVariableOnlyForTextListings(t *testing.T) {
	inProject(t, nil)
	t.Setenv("GOMDLINT_RULE_FORMAT", "ids")

	for _, args := range [][]string{
		{"explain", "--defaults", "MD009"},
		{"explain", "--defaults", "--format", "json", "MD009"},
		{"explain", "--defaults", "--format", "markdown", "whitespace"},
		{"rules", "--defaults", "--format", "json"},
		{"rules", "--defaults", "--format", "markdown"},
	} {
		got := execute(t, args...)
		assert.Equal(t, cli.ExitSuccess, got.code, "%v: %s", args, got.stderr)
	}

	for _, args := range [][]string{{"explain", "--defaults", "whitespace"}, {"rules", "--defaults"}} {
		got := execute(t, args...)
		assert.Equal(t, cli.ExitConfigError, got.code, "%v", args)
		assert.Contains(t, got.stderr, `invalid rule format "ids"`, "%v", args)
	}
}

// TestExplainEveryRule explains each rule by its ID, its name and each of
// its aliases, which all give the same page, apart from the line that
// names an alias, and checks that each page holds the rule's catalog
// texts.
func TestExplainEveryRule(t *testing.T) {
	inProject(t, nil)

	for _, rule := range lint.DefaultRegistry.Rules() {
		meta, ok := lint.DefaultRegistry.Meta(rule.ID())
		require.True(t, ok, "%s has a catalog entry", rule.ID())

		byID := execute(t, "explain", "--color", "never", rule.ID())
		require.Equal(t, cli.ExitSuccess, byID.code, byID.stderr)
		assert.Empty(t, byID.stderr)
		assert.Equal(t, byID, execute(t, "explain", "--color", "never", meta.Name), "%s by name", rule.ID())
		for _, alias := range meta.Aliases {
			// markdownlint lists MarkdownlintName first.
			whose := "one of markdownlint's names"
			if alias == meta.MarkdownlintName {
				whose = "markdownlint's name"
			}
			byAlias := execute(t, "explain", "--color", "never", alias)
			assert.Equal(t, cli.ExitSuccess, byAlias.code, byAlias.stderr)
			assert.Contains(t, byAlias.stdout, alias+" is "+whose+" for this rule.", "%s by alias", rule.ID())
		}

		var page rulesJSON
		got := execute(t, "explain", "--format", "json", rule.ID())
		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		require.NoError(t, json.Unmarshal([]byte(got.stdout), &page))
		require.Len(t, page.Rules, 1, "%s: the JSON of one rule", rule.ID())
		assert.Equal(t, meta.Description, page.Rules[0].Description, "%s", rule.ID())

		markdown := execute(t, "explain", "--format", "markdown", rule.ID())
		require.Equal(t, cli.ExitSuccess, markdown.code, markdown.stderr)
		assert.Contains(t, markdown.stdout, "# "+meta.Name+"\n", "%s: the page's title", rule.ID())
	}
}
