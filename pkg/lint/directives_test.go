package lint_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestEngineMarkdownlintDirectives(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
	}{
		{"disable enable", "<!-- markdownlint-disable MD009 -->\nbad \n<!-- markdownlint-enable MD009 -->\nbad \n", []int{4}},
		{"next line", "<!-- markdownlint-disable-next-line no-trailing-spaces -->\nbad \nbad \n", []int{3}},
		{"same line", "bad <!-- markdownlint-disable-line whitespace --> \nbad \n", []int{2}},
		{"whole file", "bad \n<!-- markdownlint-disable-file MD009 -->\nbad \n", nil},
		{"all rules", "<!-- markdownlint-disable -->\nbad \n<!-- markdownlint-enable -->\nbad \n", []int{4}},
		{"unknown rule", "<!-- markdownlint-disable unknown -->\nbad \n", []int{2}},
		{"code is inert", "```html\n<!-- markdownlint-disable MD009 -->\n```\nbad \n", []int{4}},
		{"capture restore", "<!-- markdownlint-capture -->\n<!-- markdownlint-disable MD009 -->\nbad \n<!-- markdownlint-restore -->\nbad \n", []int{5}},
		{"ordered on one line", "<!-- markdownlint-disable MD009 --><!-- markdownlint-enable MD009 -->\nbad \n", []int{2}},
		{"inline code is inert", "`<!-- markdownlint-disable MD009 -->`\nbad \n", []int{2}},
		{"file then local enable", "<!-- markdownlint-disable-file MD009 -->\n<!-- markdownlint-enable MD009 -->\nbad \n", []int{3}},
		{"line override applied last", "<!-- markdownlint-disable-next-line MD009 -->\n<!-- markdownlint-enable MD009 --> \nbad \n", []int{3}},
		{"metadata is inert", "---\nnote: '<!-- markdownlint-disable MD009 -->'\n---\nbad \n", []int{4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := lint.NewRegistry()
			registry.Register(rules.NewTrailingWhitespaceRule())
			cfg := config.NewConfig()
			cfg.Fix = true
			result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
				context.Background(), "test.md", []byte(tt.input), cfg)
			require.NoError(t, err)
			var lines []int
			for _, diag := range result.Diagnostics {
				lines = append(lines, diag.StartLine)
			}
			require.Equal(t, tt.wantLines, lines)
			require.Len(t, result.Edits, len(tt.wantLines), "suppressed diagnostics must not produce fixes")
		})
	}
}

// TestEngineDirectiveGrammarFollowsMarkdownlint lints comments that
// markdownlint 0.41.1 reads as directives, or as plain text, and checks the
// lines on which no-trailing-spaces reports. The expected lines come from
// markdownlint 0.41.1 itself, run on the same documents with only MD009
// enabled. As in its inlineCommentStartRe, the comment's kind must be
// followed by whitespace or "-->", whitespace is JavaScript's, and case
// folds ASCII letters only. A comment inside the parameter of another
// counts too, because markdownlint resumes its search right after the kind.
func TestEngineDirectiveGrammarFollowsMarkdownlint(t *testing.T) {
	tests := []struct {
		name, input string
		want        []int
		plain       bool // The comment is plain text, so it gets no notices.
	}{
		{name: "disable-next-lines", input: "# T\n\n<!-- markdownlint-disable-next-lines MD009 -->\nbad \n\nstill bad \n", want: []int{4, 6}, plain: true},
		{
			name:  "capture-x",
			input: "# T\n\n<!-- markdownlint-disable MD009 -->\n<!-- markdownlint-capture-x -->\n<!-- markdownlint-enable MD009 -->\n<!-- markdownlint-restore -->\n\nbad \n",
			want:  []int{8},
		},
		{name: "disablex", input: "# T\n\n<!-- markdownlint-disablex MD009 -->\nbad \n", want: []int{4}, plain: true},
		{name: "disable-file2", input: "# T\n\n<!-- markdownlint-disable-file2 MD009 -->\nbad \n", want: []int{4}, plain: true},
		{name: "configure-file-brace", input: "# T\n\n<!-- markdownlint-configure-file{\"MD009\": false} -->\nbad \n", want: []int{4}, plain: true},
		{name: "dashes-then-space", input: "# T\n\n<!-- markdownlint-disable--  -->\nbad \n", want: []int{4}, plain: true},
		{name: "kelvin-sign", input: "# T\n\n<!-- mar\u212adownlint-disable MD009 -->\nbad \n", want: []int{4}, plain: true},
		{name: "long-s", input: "# T\n\n<!-- markdownlint-di\u017fable MD009 -->\nbad \n", want: []int{4}, plain: true},
		{name: "disable-closed", input: "# T\n\n<!-- markdownlint-disable-->\nbad \n"},
		{name: "disable-file-closed", input: "# T\n\nbad \n\n<!-- markdownlint-disable-file-->\n"},
		{name: "disable-next-line-closed", input: "# T\n\n<!-- markdownlint-disable-next-line-->\nbad \nbad \n", want: []int{5}},
		{name: "disable-line-closed", input: "# T\n\nbad <!-- markdownlint-disable-line--> \nbad \n", want: []int{4}},
		{name: "enable-closed", input: "# T\n\n<!-- markdownlint-disable MD009 -->\nbad \n<!-- markdownlint-enable-->\nbad \n", want: []int{6}},
		{name: "enable-file-closed", input: "# T\n\n<!-- markdownlint-disable-file MD009 -->\n<!-- markdownlint-enable-file-->\nbad \n", want: []int{5}},
		{
			name:  "capture-restore-closed",
			input: "# T\n\n<!-- markdownlint-capture-->\n<!-- markdownlint-disable MD009 -->\nbad \n<!-- markdownlint-restore-->\nbad \n",
			want:  []int{7},
		},
		{name: "configure-file-closed", input: "# T\n\n<!-- markdownlint-configure-file {\"MD009\": false}-->\nbad \n"},
		{name: "upper-case", input: "# T\n\n<!-- MARKDOWNLINT-DISABLE MD009 -->\nbad \n"},
		{name: "tab", input: "# T\n\n<!-- markdownlint-disable\tMD009 -->\nbad \n"},
		{name: "no-break-spaces", input: "# T\n\n<!--\u00a0markdownlint-disable\u00a0MD009 -->\nbad \n"},
		{name: "nested", input: "# T\n\n<!-- markdownlint-disable <!-- markdownlint-enable MD009 -->\nbad \n", want: []int{4}},
		// The parameter splits into words at JavaScript's whitespace too,
		// which holds the byte order mark but not the next-line character.
		{name: "feff-separator", input: "# T\n\n<!-- markdownlint-disable\ufeffMD009 -->\nbad \n"},
		{name: "feff-between-words", input: "# T\n\n<!-- markdownlint-disable MD010\ufeffMD009 -->\nbad \n"},
		{name: "next-line-is-no-space", input: "# T\n\n<!-- markdownlint-disable MD010\u0085MD009 -->\nbad \n", want: []int{4}},
		{name: "next-line-after-word", input: "# T\n\n<!-- markdownlint-disable MD009\u0085 -->\nbad \n", want: []int{4}},
		// Go and JavaScript disagree on these two characters around the kind.
		{name: "next-line-after-kind", input: "# T\n\n<!-- markdownlint-disable\u0085MD009 -->\nbad \n", want: []int{4}, plain: true},
		{name: "vertical-tab-before-name", input: "# T\n\n<!--\u000bmarkdownlint-disable MD009 -->\nbad \n"},
		// The disable comment reads the capture comment's words too.
		{name: "nested-capture-word", input: "# T\n\n<!-- markdownlint-disable <!-- markdownlint-capture MD009 -->\nbad \n"},
	}
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	describeCatalog(registry)
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.LintFile(context.Background(), "test.md", []byte(tt.input), config.NewConfig())
			require.NoError(t, err)

			var lines []int
			for _, diag := range result.Diagnostics {
				lines = append(lines, diag.StartLine)
			}
			assert.Equal(t, tt.want, lines)
			if tt.plain {
				assert.Empty(t, result.Notices, "a comment that is no directive gets no notices")
			}
		})
	}
}

// TestEngineDirectiveWordsFoldCaseAsMarkdownlint covers words that
// markdownlint upper-cases with JavaScript's toUpperCase before it looks
// them up, which turns the long s, the dotless i and ligatures such as st
// into ASCII letters but leaves the dotted capital I and the Kelvin sign as
// they are. The expected findings come from markdownlint 0.41.1, run on the
// same documents with MD009, MD012 and MD032 on.
func TestEngineDirectiveWordsFoldCaseAsMarkdownlint(t *testing.T) {
	tests := []struct {
		name, input string
		want        []string // line:rule
	}{
		{name: "long s", input: "# T\n\n<!-- markdownlint-disable no-trailing-\u017fpaces -->\nbad \n"},
		{name: "dotless i", input: "# T\n\n<!-- markdownlint-disable no-tra\u0131ling-spaces -->\nbad \n"},
		{name: "long s in a tag", input: "# T\n\n<!-- markdownlint-disable white\u017fpace -->\nbad \n"},
		{name: "ligature st", input: "# T\n\n<!-- markdownlint-disable blanks-around-li\ufb06s -->\nText\n- item\n"},
		{name: "dotted capital i", input: "# T\n\n<!-- markdownlint-disable NO-TRA\u0130LING-SPACES -->\nbad \n", want: []string{"4:MD009"}},
		{name: "kelvin sign", input: "# T\n\n<!-- markdownlint-disable no-multiple-blan\u212as -->\nText.\n\n\nMore.\n", want: []string{"6:MD012"}},
		{
			name:  "configure-file key",
			input: "# T\n\n<!-- markdownlint-configure-file {\"no-trailing-\u017fpaces\": false} -->\nbad \n",
		},
	}
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	registry.Register(rules.NewMultipleBlankLinesRule())
	registry.Register(rules.NewBlanksAroundListsRule())
	describeCatalog(registry)
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.LintFile(context.Background(), "test.md", []byte(tt.input), config.NewConfig())
			require.NoError(t, err)

			var got []string
			for _, diag := range result.Diagnostics {
				got = append(got, fmt.Sprintf("%d:%s", diag.StartLine, diag.RuleID))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestEngineConfigureFileRuleKeysInAnySpelling covers a configure-file
// comment that writes a rule key last, after its tag, in every spelling
// that markdownlint reads as the rule. The keys apply in the order they are
// written, so the rule's own setting wins: markdownlint 0.41.1 reports MD009
// on line 5 for each of them.
func TestEngineConfigureFileRuleKeysInAnySpelling(t *testing.T) {
	for _, key := range []string{"no-trailing-spaces", "NO-TRAILING-SPACES", "no-trailing-\u017fpaces", "no-tra\u0131ling-spaces"} {
		t.Run(key, func(t *testing.T) {
			input := "# T\n\n<!-- markdownlint-configure-file " +
				"{\"default\": false, \"whitespace\": false, \"" + key + "\": true} -->\n\nbad \n"

			result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
				context.Background(), "test.md", []byte(input), config.NewConfig())
			require.NoError(t, err)

			var got []string
			for _, diag := range result.Diagnostics {
				got = append(got, fmt.Sprintf("%d:%s", diag.StartLine, diag.RuleID))
			}
			assert.Equal(t, []string{"5:MD009"}, got)
		})
	}
}

func TestEngineDirectiveCanEnableConfiguredDisabledRule(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	cfg := config.NewConfig()
	disabled := false
	cfg.Rules["MD009"] = config.RuleConfig{Enabled: &disabled}
	input := "bad \n<!-- markdownlint-enable-file MD009 -->\n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "test.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, 1, result.Diagnostics[0].StartLine)
}

func TestSuppressedHeadingFixDoesNotLeak(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewHeadingBlankLinesRule())
	cfg := config.NewConfig()
	cfg.Fix = true
	input := "# Document\n\n<!-- markdownlint-disable MD022 -->\n## Suppressed\ntext\n<!-- markdownlint-enable MD022 -->\n\n## Checked\ntext\n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "test.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 1)
	output := fix.ApplyEdits([]byte(input), result.Edits)
	require.Equal(t, "# Document\n\n<!-- markdownlint-disable MD022 -->\n## Suppressed\ntext\n<!-- markdownlint-enable MD022 -->\n\n## Checked\n\ntext\n", string(output))
}

// describeCatalog gives registry the catalog entries of the built-in rules,
// so that it resolves their aliases and tags as the product does.
func describeCatalog(registry *lint.Registry) {
	for _, meta := range rules.Catalog() {
		registry.Describe(meta)
	}
}

func TestEngineMarkdownlintHeadingAlias(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewHeadingBlankLinesRule())
	describeCatalog(registry)
	input := "<!-- markdownlint-disable blanks-around-headings -->\n# Document\ntext\n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "test.md", []byte(input), config.NewConfig())
	require.NoError(t, err)
	require.Empty(t, result.Diagnostics)
}

func TestEngineAdditionalMarkdownlintAliases(t *testing.T) {
	for _, tt := range []struct {
		alias, input string
		rule         lint.Rule
	}{
		{"ul-style", "- one\n\n* two\n", rules.NewUnorderedListStyleRule()},
		{"no-multiple-blanks", "text\n\n\nmore\n", rules.NewMultipleBlankLinesRule()},
		{"blank_lines", "# Document\ntext\n", rules.NewHeadingBlankLinesRule()},
	} {
		t.Run(tt.alias, func(t *testing.T) {
			registry := lint.NewRegistry()
			registry.Register(tt.rule)
			describeCatalog(registry)
			input := "<!-- markdownlint-disable " + tt.alias + " -->\n" + tt.input
			result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
				context.Background(), "test.md", []byte(input), config.NewConfig())
			require.NoError(t, err)
			require.Empty(t, result.Diagnostics)
		})
	}
}

// reportedLines lints input with every built-in rule at its default
// settings, plus the rules in enable, and returns the lines ruleID reports.
func reportedLines(t *testing.T, input, ruleID string, enable ...string) []int {
	t.Helper()
	cfg := config.NewConfig()
	cfg.EnableRules = enable
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
		context.Background(), "test.md", []byte(input), cfg)
	require.NoError(t, err)
	var lines []int
	for _, diag := range result.Diagnostics {
		if diag.RuleID == ruleID {
			lines = append(lines, diag.StartLine)
		}
	}
	return lines
}

func TestEngineDirectiveTagsFollowMarkdownlint(t *testing.T) {
	const (
		mixedBullets = "- one\n\n* two\n"
		extraBlank   = "Text.\n\n\nMore.\n"
		bareFence    = "```\ncode\n```\n"
		shortTable   = "| A | B |\n| - | - |\n| 1 | 2 |\n"
		mixedLinks   = "[a](docs/a.md) and [b](file:///home/user/b.md)\n"
	)
	tests := []struct {
		tag, ruleID, body string
		enable            []string
		silenced          bool
	}{
		// markdownlint v0.41's tags.
		{tag: "bullet", ruleID: "MD004", body: mixedBullets, silenced: true},
		{tag: "ul", ruleID: "MD004", body: mixedBullets, silenced: true},
		{tag: "Bullet", ruleID: "MD004", body: mixedBullets, silenced: true},
		{tag: "blank_lines", ruleID: "MD012", body: extraBlank, silenced: true},
		{tag: "language", ruleID: "MD040", body: bareFence, silenced: true},
		{tag: "whitespace", ruleID: "MD022", body: "## Section\ntext\n"},
		{tag: "links", ruleID: "MD045", body: "![](image.png)\n"},
		// Rules that markdownlint lacks take tags from its vocabulary.
		{tag: "table", ruleID: "MDL003", body: shortTable, silenced: true},
		{tag: "links", ruleID: "MDL001", body: mixedLinks, enable: []string{"MDL001"}, silenced: true},
		{tag: "url", ruleID: "MDL001", body: mixedLinks, enable: []string{"MDL001"}, silenced: true},
		// gomdlint's former tags keep working for now.
		{tag: "style", ruleID: "MD004", body: mixedBullets, silenced: true},
		{tag: "lists", ruleID: "MD004", body: mixedBullets, silenced: true},
		{tag: "layout", ruleID: "MD012", body: extraBlank, silenced: true},
		{tag: "metadata", ruleID: "MD041", body: "Text.\n", enable: []string{"MD041"}, silenced: true},
		{tag: "tables", ruleID: "MDL003", body: shortTable, silenced: true},
		{tag: "gfm", ruleID: "MDL003", body: shortTable, silenced: true},
	}
	for _, tt := range tests {
		t.Run(tt.tag+" "+tt.ruleID, func(t *testing.T) {
			reported := reportedLines(t, tt.body, tt.ruleID, tt.enable...)
			require.NotEmpty(t, reported, "the body must make %s report", tt.ruleID)

			directed := reportedLines(t, "<!-- markdownlint-disable "+tt.tag+" -->\n\n"+tt.body, tt.ruleID, tt.enable...)
			if tt.silenced {
				assert.Empty(t, directed, "markdownlint-disable %s must silence %s", tt.tag, tt.ruleID)
			} else {
				assert.Len(t, directed, len(reported), "markdownlint-disable %s must not silence %s", tt.tag, tt.ruleID)
			}
		})
	}
}

func TestEngineConfigureFileTagsFollowMarkdownlint(t *testing.T) {
	const mixedBullets = "- one\n\n* two\n"
	require.NotEmpty(t, reportedLines(t, mixedBullets, "MD004"))

	assert.Empty(t, reportedLines(t, "<!-- markdownlint-configure-file {\"bullet\": false} -->\n\n"+mixedBullets, "MD004"))
	assert.NotEmpty(t, reportedLines(t, "<!-- markdownlint-configure-file {\"whitespace\": false} -->\n\n## Section\ntext\n", "MD022"))
}

// TestEngineDirectiveTagsInPartialRegistries lints with a registry that
// holds the whole catalog but only one rule. A word that the catalog knows,
// as a tag or a rule identifier, must resolve as in the full registry,
// covering only its registered rules even when it has none. Only other
// words fall back to the rules' own tags.
func TestEngineDirectiveTagsInPartialRegistries(t *testing.T) {
	const (
		looseHeading = "# Document\ntext\n"
		missingAlt   = "![](image.png)\n"
		mixedBullets = "- one\n\n* two\n"
	)
	tests := []struct {
		name, directive, body string
		rule                  lint.Rule
		want                  int
	}{
		// MD022 tags itself whitespace and MD045 tags itself links, but
		// markdownlint's tags leave them out.
		{"disable whitespace", "markdownlint-disable whitespace", looseHeading, rules.NewHeadingBlankLinesRule(), 1},
		{"disable links", "markdownlint-disable links", missingAlt, rules.NewImageAltTextRule(), 1},
		{"configure-file whitespace", `markdownlint-configure-file {"whitespace": false}`, looseHeading, rules.NewHeadingBlankLinesRule(), 1},
		{"configure-file links", `markdownlint-configure-file {"links": false}`, missingAlt, rules.NewImageAltTextRule(), 1},
		// Catalog words whose rules are not registered configure nothing.
		{
			"configure-file without registered rules",
			`markdownlint-configure-file {"bullet": false, "MD004": false, "ul-style": false}`,
			looseHeading, rules.NewHeadingBlankLinesRule(), 1,
		},
		// A catalog tag still covers its registered rules.
		{"disable blank_lines", "markdownlint-disable blank_lines", looseHeading, rules.NewHeadingBlankLinesRule(), 0},
		{"configure-file blank_lines", `markdownlint-configure-file {"blank_lines": false}`, looseHeading, rules.NewHeadingBlankLinesRule(), 0},
		// gomdlint's former tags are not catalog words, so they fall back.
		{"disable style", "markdownlint-disable style", mixedBullets, rules.NewUnorderedListStyleRule(), 0},
		{"configure-file style", `markdownlint-configure-file {"style": false}`, mixedBullets, rules.NewUnorderedListStyleRule(), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := lint.NewRegistry()
			registry.Register(tt.rule)
			describeCatalog(registry)
			engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)

			result, err := engine.LintFile(context.Background(), "test.md", []byte(tt.body), config.NewConfig())
			require.NoError(t, err)
			require.Len(t, result.Diagnostics, 1, "the body must make %s report once", tt.rule.ID())

			input := "<!-- " + tt.directive + " -->\n\n" + tt.body
			result, err = engine.LintFile(context.Background(), "test.md", []byte(input), config.NewConfig())
			require.NoError(t, err)
			assert.Len(t, result.Diagnostics, tt.want)
		})
	}
}

func TestEngineConfigureFileIsFileScoped(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	cfg := config.NewConfig()
	cfg.Fix = true
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	input := "bad   \n<!-- markdownlint-configure-file {\"MD009\": {\"br_spaces\": 3}} -->\n"
	result, err := engine.LintFile(context.Background(), "configured.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Empty(t, result.Diagnostics)
	require.Empty(t, result.Edits)
	result, err = engine.LintFile(context.Background(), "ordinary.md", []byte("bad   \n"), cfg)
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 1)
	require.Len(t, result.Edits, 1)
	require.Empty(t, cfg.Rules, "file directives must not mutate the shared configuration")
}

func TestEngineConfigureFileDefaultPreservesExplicitRules(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		registry := lint.NewRegistry()
		registry.Register(rules.NewTrailingWhitespaceRule())
		cfg := config.NewConfig()
		if explicit {
			enabled := true
			cfg.Rules["MD009"] = config.RuleConfig{Enabled: &enabled}
		}
		input := "bad \n<!-- markdownlint-configure-file {\"default\": false} -->\n"
		result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(context.Background(), "test.md", []byte(input), cfg)
		require.NoError(t, err)
		if explicit {
			require.Len(t, result.Diagnostics, 1)
		} else {
			require.Empty(t, result.Diagnostics)
		}
	}
}

// TestEngineConfigureFileDefaultValue covers the value of a configure-file
// comment's default key, which markdownlint 0.41.1 reads as JavaScript
// does: any object turns every rule on, whatever it holds, while 0 and ""
// turn them off. markdownlint reports the trailing space with the first
// comment only.
func TestEngineConfigureFileDefaultValue(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	for comment, reported := range map[string]bool{
		`{"default": {"enabled": false}}`: true,
		`{"default": 0}`:                  false,
		`{"default": ""}`:                 false,
	} {
		input := "<!-- markdownlint-configure-file " + comment + " -->\n\n# Title\n\ntext \n"

		result, err := engine.LintFile(context.Background(), "test.md", []byte(input), config.NewConfig())

		require.NoError(t, err, comment)
		assert.Equal(t, reported, len(result.Diagnostics) == 1, comment)
	}
}

// TestEngineConfigureFileIgnoresInvalidConfiguration covers configure-file
// comments that configure nothing in markdownlint 0.41.1: invalid JSON,
// null, and a key that names no rule. gomdlint used to fail the file. The
// file is linted, MD009 reports, and a notice names each problem but null,
// which holds no settings.
func TestEngineConfigureFileIgnoresInvalidConfiguration(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	for value, notices := range map[string]int{`{"MD009":`: 1, `null`: 0, `{"unknown-rule": false}`: 1} {
		input := "<!-- markdownlint-configure-file " + value + " -->\nbad \n"
		result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(context.Background(), "test.md", []byte(input), config.NewConfig())
		require.NoError(t, err, value)
		require.Len(t, result.Diagnostics, 1, value)
		assert.Equal(t, 2, result.Diagnostics[0].StartLine, value)
		assert.Len(t, result.Notices, notices, value)
	}
}

// TestEngineConfigureFileKeysNameOneRuleOrTag covers configure-file keys
// that markdownlint 0.41 ignores. getEffectiveConfig in its
// lib/markdownlint.mjs looks each key up whole, after only upper-casing
// it, so a key of several words, a key with surrounding whitespace and an
// empty key name no rule there. gomdlint used to apply such a key to each
// of its words, or to every rule when it was empty, and then to fail the
// file. It now ignores them, as it ignores other unknown keys, with a
// notice, so both rules report.
func TestEngineConfigureFileKeysNameOneRuleOrTag(t *testing.T) {
	const body = "bad \ntab\there\n"
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	registry.Register(rules.NewHardTabsRule())
	describeCatalog(registry)
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)

	result, err := engine.LintFile(context.Background(), "test.md", []byte(body), config.NewConfig())
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 2, "the body must make MD009 and MD010 report")

	tests := []struct {
		name, key, want string
	}{
		{
			name: "two rules", key: "MD009 MD010",
			want: `unknown rule "MD009 MD010" (a key names one rule or tag; write "MD009" and "MD010" as separate keys)`,
		},
		{
			name: "two rules and a tag", key: "no-trailing-spaces\tno-hard-tabs  whitespace",
			want: `unknown rule "no-trailing-spaces\tno-hard-tabs  whitespace" ` +
				`(a key names one rule or tag; write "no-trailing-spaces", "no-hard-tabs" and "whitespace" as separate keys)`,
		},
		{name: "empty", key: "", want: `unknown rule ""`},
		{name: "space before", key: " MD009", want: `unknown rule " MD009" (did you mean "no-trailing-spaces"?)`},
		{name: "space after", key: "whitespace ", want: `unknown rule "whitespace " (did you mean the tag "whitespace"?)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parameter, err := json.Marshal(map[string]bool{tt.key: false})
			require.NoError(t, err)
			input := "<!-- markdownlint-configure-file " + string(parameter) + " -->\n" + body

			result, err := engine.LintFile(context.Background(), "test.md", []byte(input), config.NewConfig())

			require.NoError(t, err)
			assert.Len(t, result.Diagnostics, 2, "the key configures neither rule")
			require.Len(t, result.Notices, 1)
			assert.Equal(t, "markdownlint-configure-file: "+tt.want+"; it is ignored", result.Notices[0].Message)
		})
	}
}

func TestEngineConfigureFileSuggestsRulesForUnknownKeys(t *testing.T) {
	input := "<!-- markdownlint-configure-file {\"no-trailling-spaces\": false} -->\nbad \n"

	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
		context.Background(), "test.md", []byte(input), config.NewConfig())

	require.NoError(t, err)
	require.Len(t, result.Notices, 1)
	assert.Equal(t, []string{"no-trailing-spaces"}, result.Notices[0].Suggestions)
	assert.Equal(t,
		`markdownlint-configure-file: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
		result.Notices[0].Message)
}

// TestEngineConfigureFileSettingReplacesRuleDefaults covers a
// markdownlint-configure-file comment without a default key that names
// line-length (MD013), by its ID, its name or its tag, over rule_defaults
// that set its max to 20. The comment turns the rule on, which resets its
// options, so the comment gives MD013 markdownlint's max of 80, not the
// baseline's 20 (see also TestEngineConfigureFileResetsOptions).
func TestEngineConfigureFileSettingReplacesRuleDefaults(t *testing.T) {
	const (
		lineLength = "MD013"
		paragraph  = "This paragraph is longer than twenty characters, yes.\n"
	)
	cfg := config.NewConfig()
	cfg.RuleDefaults = map[string]config.RuleConfig{lineLength: {Options: map[string]any{"max": 20}}}
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry)
	lineLengthLines := func(t *testing.T, input string) []int {
		t.Helper()
		result, err := engine.LintFile(context.Background(), "test.md", []byte(input), cfg)
		require.NoError(t, err)
		var lines []int
		for _, diag := range result.Diagnostics {
			if diag.RuleID == lineLength {
				lines = append(lines, diag.StartLine)
			}
		}
		return lines
	}

	assert.Equal(t, []int{3}, lineLengthLines(t, "# Title\n\n"+paragraph), "the baseline applies without a comment")
	for _, settings := range []string{`{"MD013": true}`, `{"line-length": {"strict": false}}`, `{"line_length": "warning"}`} {
		input := "# Title\n\n<!-- markdownlint-configure-file " + settings + " -->\n\n" + paragraph
		assert.Empty(t, lineLengthLines(t, input), "%s gives MD013 markdownlint's line length", settings)
	}
}

// TestEngineConfigureFileOptionSpellings covers rule options in
// markdownlint-configure-file: markdownlint's spellings configure the rule,
// an unknown option is ignored, as markdownlint ignores it, and of two
// spellings of one option markdownlint's applies, as markdownlint knows no
// other, where a config file rejects the two. They used to fail the file.
func TestEngineConfigureFileOptionSpellings(t *testing.T) {
	const lineLength = "MD013"
	long := strings.Repeat("word ", 20) + "end\n"
	lintWith := func(t *testing.T, settings string) (*lint.FileResult, error) {
		t.Helper()
		input := "# Title\n\n<!-- markdownlint-configure-file " + settings + " -->\n\n" + long
		cfg := config.NewConfig()
		cfg.EnableRules = []string{lineLength}
		cfg.DisableRules = []string{"MD041", "MD033"}
		return lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
			context.Background(), "test.md", []byte(input), cfg)
	}
	// lineLengthFindings counts the findings of MD013 on the long line.
	lineLengthFindings := func(result *lint.FileResult) int {
		count := 0
		for _, diag := range result.Diagnostics {
			if diag.RuleID == lineLength && diag.StartLine == 5 {
				count++
			}
		}
		return count
	}

	result, err := lintWith(t, `{"MD013": {"line_length": 120}}`)
	require.NoError(t, err)
	assert.Zero(t, lineLengthFindings(result), "markdownlint's line_length sets max")

	result, err = lintWith(t, `{"line-length": {"code_blocks": false, "lin_length": 120}}`)
	require.NoError(t, err)
	assert.Equal(t, 1, lineLengthFindings(result), "an unknown option changes nothing")

	result, err = lintWith(t, `{"MD013": {"line_length": 120, "max": 100}}`)
	require.NoError(t, err)
	assert.Zero(t, lineLengthFindings(result), "markdownlint's line_length applies, not max")
	require.Len(t, result.Notices, 1)
	assert.Equal(t, lint.NoticeOptionConflict, result.Notices[0].Kind)
	assert.Equal(t, 3, result.Notices[0].Line)
}

// TestEngineConfigureFileAppliesKeysInWrittenOrder covers the order in which
// configure-file comments apply their keys: the order they are written, as
// in markdownlint 0.41.1, whose getEffectiveConfig applies the keys of its
// configuration object in order, a later key replacing what an earlier one
// set for the same rule. markdownlint spreads each comment's object over
// its configuration, so the comments of a file act as one object: a key
// written again, in the same comment or a later one, keeps its first place
// and takes its last value, as a JavaScript object's keys do. The first key
// named default, in any case, sets every rule's baseline before the other
// keys apply, wherever it is written. Each case gives what markdownlint
// 0.41.1 reports with the configuration {}: whether no-trailing-spaces
// (MD009) reports the trailing space. gomdlint used to apply rule keys after
// the others, so a rule's own setting won over its tag's, and the last
// default key in byte order won.
func TestEngineConfigureFileAppliesKeysInWrittenOrder(t *testing.T) {
	tests := []struct {
		name     string
		comments []string
		reported bool
	}{
		{name: "tag after rule", comments: []string{`{"MD009": true, "whitespace": false}`}},
		{name: "rule after tag", comments: []string{`{"whitespace": false, "MD009": true}`}, reported: true},
		{name: "ID after name", comments: []string{`{"no-trailing-spaces": true, "MD009": false}`}},
		{name: "name after ID", comments: []string{`{"MD009": false, "no-trailing-spaces": true}`}, reported: true},
		{
			name:     "key written twice",
			comments: []string{`{"whitespace": false, "MD009": true, "whitespace": false}`}, reported: true,
		},
		{
			name:     "key written again in a later comment",
			comments: []string{`{"whitespace": false, "MD009": true}`, `{"whitespace": false}`}, reported: true,
		},
		{
			name:     "rule written again in a later comment",
			comments: []string{`{"MD009": true, "whitespace": false}`, `{"MD009": true}`},
		},
		{name: "rule in a later comment", comments: []string{`{"whitespace": false}`, `{"MD009": true}`}, reported: true},
		{name: "first default key", comments: []string{`{"default": false, "Default": true}`}},
		{name: "first default key in another case", comments: []string{`{"Default": false, "default": true}`}},
		{name: "default keys in two comments", comments: []string{`{"default": false}`, `{"Default": true}`}},
		{name: "rule before default", comments: []string{`{"MD009": true, "default": false}`}, reported: true},
		{name: "rule in an earlier comment than default", comments: []string{`{"MD009": false}`, `{"default": true}`}},
	}
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	describeCatalog(registry)
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input strings.Builder
			for _, comment := range tt.comments {
				input.WriteString("<!-- markdownlint-configure-file " + comment + " -->\n\n")
			}
			input.WriteString("# Title\n\ntext \n")

			for range 20 { // Map order must not decide.
				result, err := engine.LintFile(context.Background(), "test.md", []byte(input.String()), config.NewConfig())
				require.NoError(t, err)
				assert.Equal(t, tt.reported, len(result.Diagnostics) == 1, input.String())
			}
		})
	}
}

func TestEngineAdjacentHeadingFix(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewHeadingBlankLinesRule())
	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.Rules["MD022"] = config.RuleConfig{Options: map[string]any{"markdownlint_compatible": true}}
	input := "# Title\n## Section\n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(context.Background(), "test.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 2)
	require.Equal(t, "# Title\n\n## Section\n", string(fix.ApplyEdits([]byte(input), result.Edits)))
}

func TestEngineHeadingFixHonorsSuppressedBoundaryRequirement(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewHeadingBlankLinesRule())
	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.Rules["MD022"] = config.RuleConfig{Options: map[string]any{
		"markdownlint_compatible": true, "lines_above": 1, "lines_below": 2,
	}}
	input := "<!-- markdownlint-disable-next-line MD022 -->\n# Title\n## Section\n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(context.Background(), "test.md", []byte(input), cfg)
	require.NoError(t, err)
	require.Len(t, result.Diagnostics, 1)
	require.Equal(t, "<!-- markdownlint-disable-next-line MD022 -->\n# Title\n\n## Section\n", string(fix.ApplyEdits([]byte(input), result.Edits)))
}
