package lint_test

import (
	"context"
	"testing"

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

func TestEngineConfigureFileRejectsInvalidConfiguration(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	for _, value := range []string{`{"MD009":`, `null`, `{"unknown-rule": false}`} {
		input := "<!-- markdownlint-configure-file " + value + " -->\nbad \n"
		result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(context.Background(), "test.md", []byte(input), config.NewConfig())
		require.ErrorContains(t, err, "line 1")
		require.Nil(t, result)
	}
}

func TestEngineConfigureFileRuleOverridesTag(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	input := "bad \n<!-- markdownlint-configure-file {\"MD009\": true, \"whitespace\": false} -->\n"
	for range 20 {
		result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(context.Background(), "test.md", []byte(input), config.NewConfig())
		require.NoError(t, err)
		require.Len(t, result.Diagnostics, 1)
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
