package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// TestDefaultRuleSet checks that a run without configuration runs every
// registered rule, as markdownlint runs every one of its rules, gomdlint's
// own MDL rules included.
func TestDefaultRuleSet(t *testing.T) {
	var off []string
	for _, rule := range lint.DefaultRegistry.Rules() {
		if !rule.DefaultEnabled() {
			off = append(off, rule.ID())
		}
	}
	assert.Empty(t, off, "rules that are off by default")

	var resolved []string
	for _, rr := range lint.ResolveRules(lint.DefaultRegistry, config.NewConfig()) {
		resolved = append(resolved, rr.Rule.ID())
	}
	assert.Equal(t, lint.DefaultRegistry.IDs(), resolved, "the rules that run without configuration")
}

// TestDefaultSeverities checks that every registered rule reports warnings
// without configuration. markdownlint reports every finding as an error,
// and gomdlint reports every one as a warning, which fails a run with
// --strict. An info finding never fails a run, so no rule defaults to it.
func TestDefaultSeverities(t *testing.T) {
	notWarning := make(map[string]config.Severity)
	for _, rule := range lint.DefaultRegistry.Rules() {
		if severity := rule.DefaultSeverity(); severity != config.SeverityWarning {
			notWarning[rule.ID()] = severity
		}
	}
	assert.Empty(t, notWarning, "rules whose default severity is not warning")

	for _, rr := range lint.ResolveRules(lint.DefaultRegistry, config.NewConfig()) {
		assert.Equal(t, config.SeverityWarning, rr.Severity, "%s without configuration", rr.Rule.ID())
	}
}

// lintWithDefaultRules lints input the way the CLI does: with the default
// registry, the parser for cfg's flavor and the engine, which resolves the
// rules from cfg.
func lintWithDefaultRules(t *testing.T, input string, cfg *config.Config) []lint.Diagnostic {
	t.Helper()
	engine := lint.NewEngine(goldmark.New(string(cfg.Flavor)), lint.DefaultRegistry)
	result, err := engine.LintFile(context.Background(), "doc.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)
	return result.Diagnostics
}

// findingLines returns the lines that rule id reports, in order.
func findingLines(diags []lint.Diagnostic, id string) []int {
	var lines []int
	for _, diag := range diags {
		if diag.RuleID == id {
			lines = append(lines, diag.StartLine)
		}
	}
	return lines
}

// TestFormerlyOptInRulesReportWithoutConfig lints, without configuration,
// a document that each rule that used to be off by default and checks
// something with its default options reports. The findings match
// markdownlint 0.41 with its default configuration, and MDL001, which
// markdownlint does not have, uses its default style "consistent". MD043 and
// MD044, which check nothing without options, are covered by
// TestConfiguredRulesRunByDefault.
func TestFormerlyOptInRulesReportWithoutConfig(t *testing.T) {
	tests := []struct {
		id    string
		input string
		lines []int
	}{
		{id: "MD033", input: "# Title\n\nSome <b>bold</b> text.\n", lines: []int{3}},
		{id: "MD041", input: "Introduction first.\n\n# Title\n", lines: []int{1}},
		{id: "MD059", input: "# Title\n\nRead the guide [here](https://example.com/guide).\n", lines: []int{3}},
		{
			// Neither aligned, compact nor tight: the closest style,
			// aligned, reports the pipes that do not line up.
			id:    "MD060",
			input: "# Title\n\n| Name | Description |\n|---|---|\n| item | a longer description |\n",
			lines: []int{4, 4, 5},
		},
		{
			// A link to another host sets no style, so the relative link
			// to the guide does, and the file: URL breaks it.
			id:    "MDL001",
			input: "# Title\n\nSee [the site](https://example.com) and [the guide](docs/guide.md).\n\nOr [the copy](file:///tmp/guide.md).\n",
			lines: []int{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			diags := lintWithDefaultRules(t, tt.input, config.NewConfig())

			assert.Equal(t, tt.lines, findingLines(diags, tt.id))
		})
	}

	// markdownlint allows every link and image style by default, so MD054
	// runs but reports nothing until an option disallows a style. The
	// option alone, without "enabled: true", is enough.
	t.Run("MD054", func(t *testing.T) {
		input := "# Title\n\nSee <https://example.com> and [the guide](docs/guide.md).\n"
		assert.Empty(t, findingLines(lintWithDefaultRules(t, input, config.NewConfig()), "MD054"))

		cfg := config.NewConfig()
		cfg.Rules["MD054"] = config.RuleConfig{Options: map[string]any{"autolink": false}}
		assert.Equal(t, []int{3}, findingLines(lintWithDefaultRules(t, input, cfg), "MD054"))
	})
}

// TestConfiguredRulesRunByDefault checks required-headings (MD043) and
// proper-names (MD044), which check nothing until they are given a heading
// outline or a list of names. Like markdownlint, gomdlint runs them by
// default: without options they report nothing, and options alone, without
// "enabled: true", make them check the document.
func TestConfiguredRulesRunByDefault(t *testing.T) {
	input := "# Title\n\nUse github here.\n\n## Other\n"
	tests := []struct {
		id      string
		options map[string]any
		lines   []int
	}{
		{id: "MD043", options: map[string]any{"headings": []any{"# Title", "## Usage"}}, lines: []int{5}},
		{id: "MD044", options: map[string]any{"names": []any{"GitHub"}}, lines: []int{3}},
	}

	diags := lintWithDefaultRules(t, input, config.NewConfig())
	for _, tt := range tests {
		assert.Empty(t, findingLines(diags, tt.id), "%s reports nothing without options", tt.id)
	}

	for _, tt := range tests {
		t.Run(tt.id+" with options alone", func(t *testing.T) {
			cfg := config.NewConfig()
			cfg.Rules[tt.id] = config.RuleConfig{Options: tt.options}

			assert.Equal(t, tt.lines, findingLines(lintWithDefaultRules(t, input, cfg), tt.id))
		})
	}
}
