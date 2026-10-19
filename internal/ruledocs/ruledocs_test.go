package ruledocs_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	_ "github.com/yaklabco/gomdlint/pkg/lint/rules" // Register built-in rules
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// assertLintsClean lints content, a Markdown document that ruledocs wrote,
// with the built-in rules under cfg, and fails for each finding.
func assertLintsClean(t *testing.T, path string, content []byte, cfg *config.Config) {
	t.Helper()

	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry)
	result, err := engine.LintFile(context.Background(), path, content, cfg)
	require.NoError(t, err)
	assert.Empty(t, result.RuleErrors, "%s", path)
	lines := strings.Split(string(content), "\n")
	for _, diag := range result.Diagnostics {
		var line string
		if diag.StartLine >= 1 && diag.StartLine <= len(lines) {
			line = lines[diag.StartLine-1]
		}
		t.Errorf("%s:%d:%d: %s: %s\n%s", path, diag.StartLine, diag.StartColumn, diag.RuleName, diag.Message, line)
	}
}

// TestMarkdownPagesLintClean writes the Markdown page of every built-in
// rule, as explain --format markdown prints it and the docs will hold it,
// and lints it with gomdlint's own rules and defaults.
func TestMarkdownPagesLintClean(t *testing.T) {
	for _, entry := range ruledocs.Entries(lint.DefaultRegistry, nil) {
		var page bytes.Buffer
		require.NoError(t, ruledocs.WriteMarkdownPage(&page, entry))

		assertLintsClean(t, entry.Name+".md", page.Bytes(), config.NewConfig())
	}
}

// TestMarkdownListLintsClean writes the Markdown table of every built-in
// rule, as rules --format markdown prints it, and lints it with gomdlint's
// own rules. Its rows are longer than 80 characters and cannot wrap, so
// line-length skips tables, as the repository's own configuration does.
func TestMarkdownListLintsClean(t *testing.T) {
	var list bytes.Buffer
	require.NoError(t, ruledocs.WriteMarkdown(&list, "Rules", ruledocs.Entries(lint.DefaultRegistry, nil)))

	cfg := config.NewConfig()
	cfg.Rules["MD013"] = config.RuleConfig{Options: map[string]any{"ignore_tables": true}}
	assertLintsClean(t, "rules.md", list.Bytes(), cfg)
}

// TestTextPagesFitTheTerminal writes the text page of every built-in rule
// and checks that its lines fit in 80 columns, apart from the address of
// markdownlint's documentation, and that wrapping keeps each word of a
// configuration or command line that the page quotes on one line.
func TestTextPagesFitTheTerminal(t *testing.T) {
	for _, entry := range ruledocs.Entries(lint.DefaultRegistry, nil) {
		var page bytes.Buffer
		require.NoError(t, ruledocs.WriteTextPage(&page, entry, entry.Name))
		text := page.String()

		for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			if !strings.HasPrefix(line, "markdownlint: https://") {
				assert.LessOrEqual(t, utf8.RuneCountInString(line), 80, "%s: %q", entry.ID, line)
			}
		}
		for _, literal := range []string{entry.Name + ": false", entry.Name + ": error", "--disable " + entry.Name} {
			assert.Contains(t, text, literal, "%s: the literal stays on one line", entry.ID)
		}
		assert.NotContains(t, text, "\uE000", "%s", entry.ID)
	}
}

// TestEntriesDescribeRulesWithoutCatalogEntries covers a registry whose
// rules have no catalog entry, or one without texts: each entry takes the
// rule's own name, description and fix flag, and its declared options.
func TestEntriesDescribeRulesWithoutCatalogEntries(t *testing.T) {
	registry := lint.NewRegistry()
	plain := lint.NewBaseRule("XX001", "plain-rule", "The rule's own description", []string{"own-tag"}, true)
	registry.Register(&plain)
	registry.Register(&declaringRule{lint.NewBaseRule("XX002", "declaring-rule", "Declares an option", nil, false)})
	registry.Describe(lint.RuleMeta{ID: "XX002", Name: "declaring-rule", Category: lint.CategoryCode, Tags: []string{"code"}})

	entries := ruledocs.Entries(registry, nil)

	require.Len(t, entries, 2)
	assert.Equal(t, "XX001", entries[0].ID, "in rule ID order")
	assert.Equal(t, "plain-rule", entries[0].Name)
	assert.Equal(t, "The rule's own description", entries[0].Description)
	assert.True(t, entries[0].Fixable)
	assert.Empty(t, entries[0].Tags, "only catalog tags, which the rule flags accept")
	assert.Equal(t, ruledocs.State{Enabled: true, Severity: config.SeverityWarning}, entries[0].State)
	assert.Equal(t, entries[0].Default, entries[0].State)

	assert.Equal(t, lint.CategoryCode, entries[1].Category)
	assert.Equal(t, "Declares an option", entries[1].Description)
	assert.True(t, entries[1].OptionsDeclared)
	require.Len(t, entries[1].Options, 1)
	assert.Equal(t, "level", entries[1].Options[0].Name)
}

// TestWriteTextCounts covers the count that ends a text listing: the rules
// listed, and how many there were to choose from when that is more. Total
// is optional.
func TestWriteTextCounts(t *testing.T) {
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)[:2]
	countLine := func(opts ruledocs.ListOptions) string {
		var out bytes.Buffer
		require.NoError(t, ruledocs.WriteText(&out, entries, opts))
		lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
		return lines[len(lines)-1]
	}

	assert.Equal(t, "2 rules, 2 on, 1 fixable. Run 'gomdlint explain <rule>' for details.", countLine(ruledocs.ListOptions{}))
	assert.Equal(t, "2 rules, 2 on, 1 fixable. Run 'gomdlint explain <rule>' for details.", countLine(ruledocs.ListOptions{Total: 2}))
	assert.Equal(t, "2 of 55 rules, 2 on, 1 fixable. Run 'gomdlint explain <rule>' for details.", countLine(ruledocs.ListOptions{Total: 55}))
}

// quietRule is a rule that is off by default and reports errors.
type quietRule struct {
	lint.BaseRule
}

func (*quietRule) DefaultEnabled() bool { return false }

func (*quietRule) DefaultSeverity() config.Severity { return config.SeverityError }

// TestPagesOfARuleOffByDefault covers the pages of a rule that is off by
// default, at severity error, and that markdownlint does not have: the
// lines that say how the configuration turns it on, the sentence on
// turning it on, and its other names, which are not markdownlint's.
func TestPagesOfARuleOffByDefault(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(&quietRule{lint.NewBaseRule("XX001", "quiet-rule", "", nil, false)})
	registry.Describe(lint.RuleMeta{ID: "XX001", Name: "quiet-rule", Aliases: []string{"hush", "mute"}, OptionsDeclared: true})
	page := func(cfg *config.Config, asked string) string {
		var out bytes.Buffer
		require.NoError(t, ruledocs.WriteTextPage(&out, ruledocs.Entries(registry, cfg)[0], asked))
		return out.String()
	}

	assert.Equal(t, ""+
		"quiet-rule (XX001)\n"+
		"Off by default, severity error. Not fixable.\n"+
		"markdownlint does not have this rule.\n"+
		"Also named hush and mute.\n"+
		"\n"+
		"This rule has no options.\n"+
		"\n"+
		"Under rules, quiet-rule: true turns the rule on and quiet-rule: warning turns it\n"+
		"on with severity warning. The flag --enable quiet-rule turns it on for one run.\n"+
		"\n"+
		"Silence one line:\n"+
		"  <!-- markdownlint-disable-next-line quiet-rule -->\n",
		page(nil, "quiet-rule"))

	on := config.NewConfig()
	on.Rules["XX001"] = config.RuleConfig{Enabled: config.Bool(true)}
	assert.Contains(t, page(on, "quiet-rule"), "\nYour configuration turns it on.\n")
	assert.Contains(t, page(on, " HUSH "), "\nhush is another name for this rule.\nAlso named mute.\n")

	severity := string(config.SeverityWarning)
	on.Rules["XX001"] = config.RuleConfig{Enabled: config.Bool(true), Severity: &severity}
	assert.Contains(t, page(on, "quiet-rule"), "\nYour configuration turns it on, at severity warning.\n")

	var markdown bytes.Buffer
	require.NoError(t, ruledocs.WriteMarkdownPage(&markdown, ruledocs.Entries(registry, nil)[0]))
	assert.NotContains(t, markdown.String(), "\n\n\n", "a page without a description has no empty block")
	assert.Contains(t, markdown.String(), "- Default: off, severity error\n")
	assert.Contains(t, markdown.String(), "- Also named: `hush`, `mute`\n")
}

// declaringRule is a rule that declares its options itself.
type declaringRule struct {
	lint.BaseRule
}

func (*declaringRule) DeclaredOptions() []lint.OptionSpec {
	return []lint.OptionSpec{{Name: "level", Type: lint.OptionInteger, Default: 1, Description: "A level."}}
}

// TestFilter covers the filters of the rules command: words that a rule
// must hold in its name, an alias, its ID, its category or a tag, or its
// description, in any case, and the category, tag, fix and state filters.
// Several values of one filter match any of them, and different filters
// must all match.
func TestFilter(t *testing.T) {
	entry := ruledocs.Entry{
		RuleMeta: lint.RuleMeta{
			ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"},
			Category: lint.CategoryLists, Tags: []string{"bullet", "ul"}, Fixable: true,
			Description: "Every bullet uses the same marker.",
		},
		State: ruledocs.State{Enabled: false, Severity: config.SeverityWarning},
	}

	for _, tt := range []struct {
		name   string
		filter ruledocs.Filter
		want   bool
	}{
		{name: "no filter", want: true},
		{name: "a word of the name", filter: ruledocs.Filter{Words: []string{"LIST"}}, want: true},
		{name: "an alias", filter: ruledocs.Filter{Words: []string{"ul-style"}}, want: true},
		{name: "the ID", filter: ruledocs.Filter{Words: []string{"md00"}}, want: true},
		{name: "the category", filter: ruledocs.Filter{Words: []string{"lists"}}, want: true},
		{name: "a tag", filter: ruledocs.Filter{Words: []string{"bullet"}}, want: true},
		{name: "the description", filter: ruledocs.Filter{Words: []string{"same marker"}}, want: true},
		{name: "every word", filter: ruledocs.Filter{Words: []string{"bullet", "marker"}}, want: true},
		{name: "a word it lacks", filter: ruledocs.Filter{Words: []string{"bullet", "table"}}},
		{name: "a category", filter: ruledocs.Filter{Categories: []lint.Category{lint.CategoryCode, lint.CategoryLists}}, want: true},
		{name: "another category", filter: ruledocs.Filter{Categories: []lint.Category{lint.CategoryCode}}},
		{name: "a tag in any case", filter: ruledocs.Filter{Tags: []string{"html", "UL"}}, want: true},
		{name: "a tag with spaces around it", filter: ruledocs.Filter{Tags: []string{" ul "}}, want: true},
		{name: "another tag", filter: ruledocs.Filter{Tags: []string{"html"}}},
		{name: "fixable", filter: ruledocs.Filter{Fixable: true}, want: true},
		{name: "disabled", filter: ruledocs.Filter{Disabled: true}, want: true},
		{name: "enabled", filter: ruledocs.Filter{Enabled: true}},
		{name: "filters combine", filter: ruledocs.Filter{Fixable: true, Tags: []string{"bullet"}, Enabled: true}},
	} {
		assert.Equal(t, tt.want, tt.filter.Matches(entry), tt.name)
	}

	// Words look in the category's key, not in its title, Whitespace and
	// layout here.
	layout := ruledocs.Entry{RuleMeta: lint.RuleMeta{ID: "MD013", Name: "line-length", Category: lint.CategoryLayout}}
	assert.True(t, (&ruledocs.Filter{Words: []string{"LAYOUT"}}).Matches(layout))
	assert.False(t, (&ruledocs.Filter{Words: []string{"whitespace"}}).Matches(layout))
}
