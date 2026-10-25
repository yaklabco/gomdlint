package reporter_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

const (
	// rulePagesURL is where the descriptors' help URIs point: the generated
	// rule pages under docs/rules on main, next to the configuration's JSON
	// Schema, whose $id is on main too.
	rulePagesURL = "https://github.com/yaklabco/gomdlint/blob/main/docs/rules/"

	// rulePagesDir is the directory of the generated rule pages, relative
	// to this package.
	rulePagesDir = "../../docs/rules"

	// sarifTagLimit is the number of tags of a rule that GitHub code
	// scanning keeps.
	sarifTagLimit = 10
)

// describeEveryBuiltinRule reports one finding of every built-in rule and
// returns the rule descriptors of the log, in catalog order, with the
// registry that described them.
func describeEveryBuiltinRule(t *testing.T) ([]reporter.SARIFRule, *lint.Registry) {
	t.Helper()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	catalog := rules.Catalog()

	diags := make([]lint.Diagnostic, 0, len(catalog))
	for _, meta := range catalog {
		diags = append(diags, lint.Diagnostic{RuleID: meta.ID, RuleName: meta.Name, StartLine: 1})
	}
	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{sarifFile(filepath.Join(root, "a.md"), "x\n", diags...)}}

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result).Runs[0].Tool.Driver.Rules
	require.Len(t, descriptors, len(catalog))
	return descriptors, registry
}

// expectedLevel is the SARIF level of a severity.
func expectedLevel(severity config.Severity) string {
	switch severity {
	case config.SeverityError:
		return "error"
	case config.SeverityInfo:
		return "note"
	}
	return "warning"
}

// TestSARIFReporter_DescribesEveryRuleWithHelp covers the descriptor of every
// built-in rule: its ID and name, both descriptions, the page that explain
// prints as help, as text and as Markdown, the address of its generated
// page, its category and markdownlint tags, and its default severity.
func TestSARIFReporter_DescribesEveryRuleWithHelp(t *testing.T) {
	t.Parallel()

	descriptors, registry := describeEveryBuiltinRule(t)
	entries := ruledocs.Entries(registry, nil)
	require.Len(t, entries, len(descriptors))

	for idx, meta := range rules.Catalog() {
		descriptor := descriptors[idx]
		require.Equal(t, meta.ID, descriptor.ID)
		entry := entries[idx]
		require.Equal(t, meta.ID, entry.ID)

		assert.Equal(t, meta.Name, descriptor.Name, "%s: the canonical name", meta.ID)
		assert.NotNil(t, descriptor.ShortDescription, "%s: a short description", meta.ID)
		assert.NotNil(t, descriptor.FullDescription, "%s: a full description", meta.ID)

		var text, markdown bytes.Buffer
		require.NoError(t, ruledocs.WriteTextPage(&text, entry, ""))
		require.NoError(t, ruledocs.WriteMarkdownPage(&markdown, entry))
		if assert.NotNil(t, descriptor.Help, "%s: help", meta.ID) {
			assert.Equal(t, text.String(), descriptor.Help.Text, "%s: the page that explain prints", meta.ID)
			assert.Equal(t, markdown.String(), descriptor.Help.Markdown,
				"%s: the page that explain --format markdown prints", meta.ID)
		}

		assert.Equal(t, rulePagesURL+meta.Name+".md", descriptor.HelpURI, "%s: the rule's generated page", meta.ID)

		wantTags := []any{string(meta.Category)}
		for _, tag := range meta.Tags {
			if !slices.Contains(wantTags, any(tag)) {
				wantTags = append(wantTags, tag)
			}
		}
		assert.Equal(t, wantTags, descriptor.Properties["tags"], "%s: the category, then markdownlint's tags", meta.ID)
		tags, ok := descriptor.Properties["tags"].([]any)
		if assert.True(t, ok, "%s: tags", meta.ID) {
			assert.LessOrEqual(t, len(tags), sarifTagLimit, "%s: GitHub keeps every tag", meta.ID)
		}

		rule, found := registry.GetByID(meta.ID)
		require.True(t, found)
		if assert.NotNil(t, descriptor.DefaultConfig, meta.ID) {
			assert.Equal(t, expectedLevel(rule.DefaultSeverity()), descriptor.DefaultConfig.Level,
				"%s: the default severity", meta.ID)
		}
	}
}

// TestSARIFReporter_HelpURIsNameGeneratedPages checks that the help URI of
// every built-in rule names a generated page of the repository's docs/rules.
func TestSARIFReporter_HelpURIsNameGeneratedPages(t *testing.T) {
	t.Parallel()

	descriptors, _ := describeEveryBuiltinRule(t)
	for _, descriptor := range descriptors {
		page, found := strings.CutPrefix(descriptor.HelpURI, rulePagesURL)
		if !assert.True(t, found, "%s: %q is under %s", descriptor.ID, descriptor.HelpURI, rulePagesURL) {
			continue
		}
		require.NotContains(t, page, "/", "%s: a page of docs/rules itself", descriptor.ID)

		content, err := os.ReadFile(filepath.Join(rulePagesDir, page))
		if assert.NoError(t, err, "%s: %s exists", descriptor.ID, page) {
			assert.True(t, ruledocs.IsGenerated(content), "%s: %s is a generated page", descriptor.ID, page)
		}
	}
}

// TestSARIFReporter_KeepsTenTags covers a rule with more tags than GitHub
// keeps: the descriptor keeps its category and the first of its tags, ten
// in all, each once.
func TestSARIFReporter_KeepsTenTags(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	tagged := lint.NewBaseRule("ZZ001", "many-tags", "A rule with many tags.", []string{"own"}, false)
	registry.Register(&tagged)
	registry.Describe(lint.RuleMeta{
		ID: "ZZ001", Name: "many-tags", Category: lint.CategoryHeadings, Description: "A rule with many tags.",
		Tags: []string{"t1", "headings", "t2", "t2", "t3", "t4", "t5", "t6", "t7", "t8", "t9", "t10", "t11"},
	})

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), "x\n", lint.Diagnostic{RuleID: "ZZ001", StartLine: 1}),
	}}

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result).Runs[0].Tool.Driver.Rules

	require.Len(t, descriptors, 1)
	assert.Equal(t, []any{"headings", "t1", "t2", "t3", "t4", "t5", "t6", "t7", "t8", "t9"}, descriptors[0].Properties["tags"])
}

// TestSARIFReporter_OnlyBuiltinRulesLinkToAPage covers rules without a
// generated page: a custom rule with a catalog entry, and a custom rule
// that takes a built-in rule's ID under another name. Both get the help
// that explain prints for them, but no help URI.
func TestSARIFReporter_OnlyBuiltinRulesLinkToAPage(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	custom := lint.NewBaseRule("ZZ001", "custom-rule", "A custom rule.", nil, false)
	impostor := lint.NewBaseRule("MD009", "my-trailing-spaces", "Another rule under MD009.", nil, false)
	registry.Register(&custom)
	registry.Register(&impostor)
	registry.Describe(lint.RuleMeta{
		ID: "ZZ001", Name: "custom-rule", Category: lint.CategoryLayout,
		Description: "Custom lines are fine.", Rationale: "Custom reasons.",
	})

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), "x\n",
			lint.Diagnostic{RuleID: "ZZ001", StartLine: 1},
			lint.Diagnostic{RuleID: "MD009", StartLine: 1},
		),
	}}

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result).Runs[0].Tool.Driver.Rules

	require.Len(t, descriptors, 2)
	for _, descriptor := range descriptors {
		assert.Empty(t, descriptor.HelpURI, "%s: no generated page", descriptor.Name)
		if assert.NotNil(t, descriptor.Help, descriptor.Name) {
			assert.True(t, strings.HasPrefix(descriptor.Help.Text, descriptor.Name+" ("+descriptor.ID+")"),
				"%s: the page that explain prints: %q", descriptor.Name, descriptor.Help.Text)
			assert.True(t, strings.HasPrefix(descriptor.Help.Markdown, "# "+descriptor.Name+"\n"),
				"%s: the Markdown page: %q", descriptor.Name, descriptor.Help.Markdown)
		}
	}
	assert.Contains(t, descriptors[0].Help.Text, "Custom reasons.", "the catalog entry's rationale")
}

// TestSARIFReporter_DescriptorsAreDeterministic reports every built-in rule
// twice and expects the same log.
func TestSARIFReporter_DescriptorsAreDeterministic(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	catalog := rules.Catalog()
	diags := make([]lint.Diagnostic, 0, len(catalog))
	for idx := len(catalog) - 1; idx >= 0; idx-- {
		diags = append(diags, lint.Diagnostic{RuleID: catalog[idx].ID, StartLine: 1})
	}
	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{sarifFile(filepath.Join(root, "a.md"), "x\n", diags...)}}
	opts := reporter.Options{WorkingDir: root, Registry: registry, ToolVersion: "v1.2.3"}

	assert.Equal(t, string(reportSARIF(t, opts, result)), string(reportSARIF(t, opts, result)))
}
