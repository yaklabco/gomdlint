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

	opts := reporter.Options{
		WorkingDir: root, Registry: registry,
		RuleHelp: ruledocs.RuleHelp(registry), BuiltInRule: ruledocs.IsBuiltInRule,
	}
	descriptors := decodeSARIF(t, opts, result).Runs[0].Tool.Driver.Rules
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
// that takes a built-in rule's ID under another name. Without
// Options.BuiltInRule, both get the help that explain prints for them,
// but no help URI. TestSARIFReporter_DescribesARuleThatReplacesABuiltinOne
// covers the gomdlint command's options.
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

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry, RuleHelp: ruledocs.RuleHelp(registry)}, result).Runs[0].Tool.Driver.Rules

	require.Len(t, descriptors, 2)
	for _, descriptor := range descriptors {
		assert.Empty(t, descriptor.HelpURI, "%s: no generated page", descriptor.Name)
		require.NotNil(t, descriptor.Help, descriptor.Name)
		assert.True(t, strings.HasPrefix(descriptor.Help.Text, descriptor.Name+" ("+descriptor.ID+")"),
			"%s: the page that explain prints: %q", descriptor.Name, descriptor.Help.Text)
		assert.True(t, strings.HasPrefix(descriptor.Help.Markdown, "# "+descriptor.Name+"\n"),
			"%s: the Markdown page: %q", descriptor.Name, descriptor.Help.Markdown)
		for _, help := range []string{descriptor.Help.Text, descriptor.Help.Markdown} {
			assert.NotContains(t, help, "markdownlint/blob", "%s: markdownlint does not document it", descriptor.Name)
			assert.NotContains(t, help, "This document breaks the rule", "%s: no built-in rule's example", descriptor.Name)
		}
	}
	assert.Contains(t, descriptors[0].Help.Text, "Custom reasons.", "the catalog entry's rationale")
	assert.Contains(t, descriptors[1].Help.Text, "markdownlint does not have this rule.",
		"a custom rule under a built-in rule's ID")
}

// TestSARIFReporter_TagsFallBackToTheRule covers a custom rule whose
// catalog entry lists no tags: it keeps the tags it reports itself, after
// its category, as a rule without an entry does.
func TestSARIFReporter_TagsFallBackToTheRule(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	untagged := lint.NewBaseRule("ZZ001", "untagged-entry", "A rule.", []string{"own", "tags"}, false)
	registry.Register(&untagged)
	registry.Describe(lint.RuleMeta{ID: "ZZ001", Name: "untagged-entry", Category: lint.CategoryCode})

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), "x\n", lint.Diagnostic{RuleID: "ZZ001", StartLine: 1}),
	}}

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result).Runs[0].Tool.Driver.Rules

	require.Len(t, descriptors, 1)
	assert.Equal(t, []any{"code", "own", "tags"}, descriptors[0].Properties["tags"])
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
	opts := reporter.Options{WorkingDir: root, Registry: registry, ToolVersion: "v1.2.3", RuleHelp: ruledocs.RuleHelp(registry)}

	first := string(reportSARIF(t, opts, result))
	second := string(reportSARIF(t, opts, result))
	assert.Equal(t, first, second)
}

// TestSARIFReporter_HelpIsOptIn covers a reporter without Options.RuleHelp:
// descriptors keep their name, descriptions, default level and catalog
// tags, but carry no help and no help URI, as the reporter does not render
// rule pages itself.
func TestSARIFReporter_HelpIsOptIn(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), "x\n", lint.Diagnostic{RuleID: "MD004", StartLine: 1}),
	}}

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result).Runs[0].Tool.Driver.Rules

	require.Len(t, descriptors, 1)
	descriptor := descriptors[0]
	assert.Equal(t, "unordered-list-style", descriptor.Name)
	assert.NotNil(t, descriptor.ShortDescription)
	assert.NotNil(t, descriptor.FullDescription)
	assert.NotNil(t, descriptor.DefaultConfig)
	assert.Equal(t, []any{"lists", "bullet", "ul"}, descriptor.Properties["tags"], "the category, then the catalog tags")
	assert.Nil(t, descriptor.Help)
	assert.Empty(t, descriptor.HelpURI)
}

// companyPolicyRule is a custom implementation of a rule, of its own type.
type companyPolicyRule struct {
	lint.BaseRule
}

// DefaultSeverity reports the rule's findings as errors.
func (r *companyPolicyRule) DefaultSeverity() config.Severity {
	return config.SeverityError
}

// TestSARIFReporter_DescribesARuleThatReplacesABuiltinOne covers a registry
// with the built-in rules in which a custom rule then replaces MD009: under
// another name, as another implementation under the built-in rule's own
// name, or as a wrapper around the built-in rule. The registry still holds
// the built-in rule's catalog entry for MD009, but none of these is the
// built-in rule, so its descriptor describes the rule itself: its own
// description, tags and default severity, with no help and no help URI.
func TestSARIFReporter_DescribesARuleThatReplacesABuiltinOne(t *testing.T) {
	t.Parallel()

	builtIn := rules.NewTrailingWhitespaceRule()
	policy := func(name string) reporter.SARIFRule {
		return reporter.SARIFRule{
			ID:               "MD009",
			Name:             name,
			ShortDescription: &reporter.SARIFMultiformatText{Text: "Company policy."},
			FullDescription:  &reporter.SARIFMultiformatText{Text: "Company policy."},
			DefaultConfig:    &reporter.SARIFRuleConfig{Level: "error"},
			Properties:       map[string]any{"tags": []any{"policy"}},
		}
	}
	builtInTags := make([]any, 0, len(builtIn.Tags()))
	for _, tag := range builtIn.Tags() {
		builtInTags = append(builtInTags, tag)
	}

	for _, tt := range []struct {
		name string
		rule lint.Rule
		want reporter.SARIFRule
	}{
		{
			name: "another name",
			rule: &companyPolicyRule{BaseRule: lint.NewBaseRule("MD009", "company-policy", "Company policy.", []string{"policy"}, false)},
			want: policy("company-policy"),
		},
		{
			name: "the built-in name",
			rule: &companyPolicyRule{BaseRule: lint.NewBaseRule("MD009", "no-trailing-spaces", "Company policy.", []string{"policy"}, false)},
			want: policy("no-trailing-spaces"),
		},
		{
			name: "a wrapper",
			rule: struct{ lint.Rule }{builtIn},
			want: reporter.SARIFRule{
				ID:               "MD009",
				Name:             builtIn.Name(),
				ShortDescription: &reporter.SARIFMultiformatText{Text: builtIn.Description()},
				FullDescription:  &reporter.SARIFMultiformatText{Text: builtIn.Description()},
				DefaultConfig:    &reporter.SARIFRuleConfig{Level: "warning"},
				Properties:       map[string]any{"tags": builtInTags},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := lint.NewRegistry()
			rules.RegisterAll(registry)
			registry.Register(tt.rule)

			root := newGitWorkTree(t, false)
			result := &runner.Result{Files: []runner.FileOutcome{
				sarifFile(filepath.Join(root, "a.md"), "x\n", lint.Diagnostic{RuleID: "MD009", StartLine: 1}),
			}}
			opts := reporter.Options{
				WorkingDir: root, Registry: registry,
				RuleHelp: ruledocs.RuleHelp(registry), BuiltInRule: ruledocs.IsBuiltInRule,
			}
			descriptors := decodeSARIF(t, opts, result).Runs[0].Tool.Driver.Rules

			require.Len(t, descriptors, 1)
			assert.Equal(t, tt.want, descriptors[0])
		})
	}
}

// TestSARIFReporter_CustomIDsKeepTheirCatalogEntries covers a custom rule
// under an ID of its own, which Options.BuiltInRule does not recognize as a
// built-in rule either: as no built-in rule has its ID, the registry's
// catalog entry for it is its own, and its descriptor takes descriptions,
// tags and help from that entry.
func TestSARIFReporter_CustomIDsKeepTheirCatalogEntries(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	custom := lint.NewBaseRule("ZZ001", "custom-rule", "The rule's own words.", []string{"own"}, false)
	registry.Register(&custom)
	registry.Describe(lint.RuleMeta{
		ID: "ZZ001", Name: "custom-rule", Category: lint.CategoryLayout, Tags: []string{"custom"},
		Description: "Custom lines are fine.", Rationale: "Custom reasons.", Fix: "There is no automatic fix.",
	})

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), "x\n", lint.Diagnostic{RuleID: "ZZ001", StartLine: 1}),
	}}
	opts := reporter.Options{
		WorkingDir: root, Registry: registry, RuleHelp: ruledocs.RuleHelp(registry),
		BuiltInRule: func(lint.Rule) bool { return false },
	}
	descriptors := decodeSARIF(t, opts, result).Runs[0].Tool.Driver.Rules

	require.Len(t, descriptors, 1)
	descriptor := descriptors[0]
	require.NotNil(t, descriptor.Help, "the catalog entry's page")
	assert.Contains(t, descriptor.Help.Text, "Custom reasons.")
	descriptor.Help = nil
	assert.Equal(t, reporter.SARIFRule{
		ID:               "ZZ001",
		Name:             "custom-rule",
		ShortDescription: &reporter.SARIFMultiformatText{Text: "Custom lines are fine."},
		FullDescription: &reporter.SARIFMultiformatText{
			Text: "Custom lines are fine. Custom reasons. There is no automatic fix.",
		},
		DefaultConfig: &reporter.SARIFRuleConfig{Level: "warning"},
		Properties:    map[string]any{"tags": []any{"layout", "custom"}},
	}, descriptor)
}
