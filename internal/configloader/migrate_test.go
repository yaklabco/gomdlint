package configloader

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func lintMigratedMarkdown(t *testing.T, settings, input string) []lint.Diagnostic {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".markdownlint.json")
	require.NoError(t, os.WriteFile(path, []byte(settings), 0o600))
	converted, err := ConvertMarkdownlintConfig(path)
	require.NoError(t, err)
	migratedPath := filepath.Join(filepath.Dir(path), ".gomdlint.yml")
	yamlConfig, err := converted.FileContent(migratedPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(migratedPath, yamlConfig, 0o600))
	loaded, err := Load(context.Background(), LoadOptions{
		WorkingDir: filepath.Dir(path), ExplicitPath: migratedPath, IgnoreEnv: true,
		IgnoreUserConfig: true, IgnoreSystemConfig: true, IgnoreMarkdownlint: true, NonInteractive: true,
	})
	require.NoError(t, err)
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "test.md", []byte(input), loaded.Config)
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)
	return result.Diagnostics
}

func TestMigratedWhitespacePolicy(t *testing.T) {
	input := "```text\ncode  \n```\n\nline  \ncontinued\n\nbad   \n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD009": true}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 8, diags[0].StartLine)
}

func TestMigratedWhitespaceOptions(t *testing.T) {
	input := "```text\ncode \n```\n\nline   \ncontinued\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD009": {"code_blocks": true, "br_spaces": 3}}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 2, diags[0].StartLine)
}

func TestMigratedWhitespaceStrictHardBreaks(t *testing.T) {
	input := "paragraph  \ncontinued\n\nlast  \n\n# Heading  \n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD009": {"strict": true}}`, input)
	require.Len(t, diags, 2)
	require.Equal(t, 4, diags[0].StartLine)
	require.Equal(t, 6, diags[1].StartLine)
}

func TestMigratedWhitespaceListEmptyLines(t *testing.T) {
	input := "- item\n  \n  continuation\n\n  \noutside\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD009": {"list_item_empty_lines": true, "br_spaces": 0}}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 5, diags[0].StartLine)
}

func TestMigratedLineLengthPolicy(t *testing.T) {
	input := strings.Repeat("word ", 20) + "end\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD013": true}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 81, diags[0].StartColumn)
}

func TestMigratedLineLengthOptions(t *testing.T) {
	input := "short\n\nbird tree rain snow end\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD013": {"line_length": 20}}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 21, diags[0].StartColumn)
}

func TestMigratedLineLengthContextOptions(t *testing.T) {
	input := "# bird tree rain snow end\n\nbird tree rain snow end\n\n```text\nbird tree rain end\n```\n\n" +
		"| Column | Description |\n| --- | --- |\n| value | bird tree rain snow end |\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD013": {"line_length": 20, "heading_line_length": 30, "code_block_line_length": 10, "tables": false}}`, input)
	require.Len(t, diags, 2)
	require.Equal(t, 3, diags[0].StartLine)
	require.Equal(t, 6, diags[1].StartLine)
}

func TestMigratedLineLengthSoftLimit(t *testing.T) {
	input := strings.Repeat("x", 100) + "\n\nshort " + strings.Repeat("x", 100) + "\n"
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD013": true}`, input))
	require.Len(t, lintMigratedMarkdown(t, `{"default": false, "MD013": {"strict": true}}`, input), 2)
	diags := lintMigratedMarkdown(t, `{"default": false, "MD013": {"stern": true}}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 3, diags[0].StartLine)
}

func TestMigratedLineLengthLinkTable(t *testing.T) {
	input := "| Item | Link |\n| --- | --- |\n| " + strings.Repeat("word ", 20) + "| [runbook](runbook.md) |\n"
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD013": true}`, input))
	require.Len(t, lintMigratedMarkdown(t, `{"default": false, "MD013": {"strict": true}}`, input), 1)
}

func TestMigratedLineLengthCodeIsNotRewritten(t *testing.T) {
	input := "```text\nbird tree rain snow end\n```\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD013": {"line_length": 10, "strict": true}}`, input)
	require.Len(t, diags, 1)
	require.Empty(t, diags[0].FixEdits)
}

func TestMigratedLineLengthCodeFenceOptions(t *testing.T) {
	input := "```" + strings.Repeat("x", 100) + "\nword\n```\n"
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD013": {"code_blocks": false, "strict": true}}`, input))
}

func TestMigratedLineLengthUnicodeUnits(t *testing.T) {
	settings := `{"default": false, "MD013": {"line_length": 10, "strict": true}}`
	require.Empty(t, lintMigratedMarkdown(t, settings, "😀😀😀\n"))
	diags := lintMigratedMarkdown(t, settings, "😀😀😀😀😀😀\n")
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "length 12")
	require.Equal(t, 21, diags[0].StartColumn, "source columns retain UTF-8 byte offsets")
}

func TestMigratedTitlePolicy(t *testing.T) {
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD025": true}`, "Preamble.\n\n# Section\n\n# Another section\n"))
	diags := lintMigratedMarkdown(t, `{"default": false, "MD025": true}`, "# Title\n\n# Second title\n")
	require.Len(t, diags, 1)
	require.Equal(t, 3, diags[0].StartLine)
	diags = lintMigratedMarkdown(t, `{"default": false, "MD025": true}`, "---\ntitle: Document\n---\n\n# Duplicate title\n")
	require.Len(t, diags, 1)
	require.Equal(t, 5, diags[0].StartLine)
}

func TestMigratedRuleObjectControls(t *testing.T) {
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD009": {"enabled": false}}`, "bad \n"))
	diags := lintMigratedMarkdown(t, `{"default": false, "MD009": {"severity": "error"}}`, "bad \n")
	require.Len(t, diags, 1)
	require.Equal(t, config.SeverityError, diags[0].Severity)
}

func TestMigratedHTMLDefaultIsExplicit(t *testing.T) {
	require.Len(t, lintMigratedMarkdown(t, `{"default": false, "MD033": true}`, "<sup>text</sup>\n"), 1)
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD033": {"allowed_elements": ["sup"]}}`, "<sup>text</sup>\n"))
}

func TestMigratedDefaultsRemainConfigurable(t *testing.T) {
	input := "bad \n<!-- markdownlint-configure-file {\"default\": false} -->\n"
	require.Empty(t, lintMigratedMarkdown(t, `{}`, input))
	diags := lintMigratedMarkdown(t, `{"MD009": true}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, "MD009", diags[0].RuleID)
}

func TestFileDefaultsPreserveMigratedRuleSet(t *testing.T) {
	input := "<!-- markdownlint-configure-file {\"default\": true} -->\n# Title\n\n| A | B |\n| - | - |\n"
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false}`, input))
}

func TestMigratedStyleDefaults(t *testing.T) {
	for _, tt := range []struct{ rule, input string }{
		{"MD004", "* first\n* second\n"},
		{"MD046", "    code\n"},
		{"MD048", "~~~text\ncode\n~~~\n"},
	} {
		t.Run(tt.rule, func(t *testing.T) {
			require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "`+tt.rule+`": true}`, tt.input))
		})
	}
}

func TestMigratedBlankLineMaximum(t *testing.T) {
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD012": {"maximum": 2}}`, "Text.\n\n\nMore.\n"))
	require.Len(t, lintMigratedMarkdown(t, `{"default": false, "MD012": {"maximum": 1}}`, "Text.\n\n\nMore.\n"), 1)
}

func TestMigratedTagRuleObjects(t *testing.T) {
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "whitespace": {"br_spaces": 3}}`, "text   \n"))
	require.Empty(t, lintMigratedMarkdown(t, `{"whitespace": {"enabled": false}}`, "# Title\n\ntext \n"))
}

func TestMigratedFirstHeadingPreamble(t *testing.T) {
	settings := `{"default": false, "MD041": {"allow_preamble": true}}`
	require.Empty(t, lintMigratedMarkdown(t, settings, "Preamble.\n\n# Title\n"))
	require.Empty(t, lintMigratedMarkdown(t, settings, "Preamble without a heading.\n"))
	require.Len(t, lintMigratedMarkdown(t, settings, "Preamble.\n\n## Wrong level\n"), 1)
}

func TestMigratedSeverityDefaults(t *testing.T) {
	input := "# Title\n\ntext \n"
	diags := lintMigratedMarkdown(t, `{"default": "warning"}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, config.SeverityWarning, diags[0].Severity)
	diags = lintMigratedMarkdown(t, `{"default": false, "MD009": true}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, config.SeverityError, diags[0].Severity)
	diags = lintMigratedMarkdown(t, `{"default": false, "MD009": "warning"}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, config.SeverityWarning, diags[0].Severity)
}

func TestMigratedOrderedListStyles(t *testing.T) {
	input := "1. first\n1. second\n"
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD029": true}`, input))
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD029": {"style": "one"}}`, input))
	require.Len(t, lintMigratedMarkdown(t, `{"default": false, "MD029": {"style": "ordered"}}`, input), 1)
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD029": {"style": "zero"}}`, "0. first\n0. second\n"))
}

func TestMigratedFenceLanguageOnly(t *testing.T) {
	settings := `{"default": false, "MD040": {"language_only": true}}`
	require.Len(t, lintMigratedMarkdown(t, settings, "```go title=example\ncode\n```\n"), 1)
	require.Empty(t, lintMigratedMarkdown(t, settings, "```go\ncode\n```\n"))
}

func TestMigratedProhibitedLinkText(t *testing.T) {
	settings := `{"default": false, "MD059": {"prohibited_texts": ["details"]}}`
	require.Len(t, lintMigratedMarkdown(t, settings, "[details](target)\n"), 1)
	require.Empty(t, lintMigratedMarkdown(t, settings, "[here](target)\n"))
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD059": {"prohibited_texts": []}}`, "[here](target)\n"))
}

func TestMigratedLinkTextNormalization(t *testing.T) {
	settings := `{"default": false, "MD059": true}`
	require.Len(t, lintMigratedMarkdown(t, settings, "[Here!](target)\n"), 1)
	require.Empty(t, lintMigratedMarkdown(t, settings, "[read more](target)\n"))
	require.Empty(t, lintMigratedMarkdown(t, settings, "[<b>here</b>](target)\n"))
}

func TestMigratedTableHTMLAllowlist(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n| value | <br> |\n\n<br>\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD033": {"table_allowed_elements": ["br"]}}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 5, diags[0].StartLine)
}

func TestMigratedTableDelimiterAlignment(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n| x | y |\n"
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD060": {"style": "compact"}}`, input))
	diags := lintMigratedMarkdown(t, `{"default": false, "MD060": {"style": "compact", "aligned_delimiter": true}}`, input)
	require.Len(t, diags, 2)
	require.Equal(t, 2, diags[0].StartLine)
	require.Equal(t, 7, diags[0].StartColumn)
	require.Equal(t, 13, diags[1].StartColumn)
}

func TestMigratedTableColumnStyles(t *testing.T) {
	for _, tt := range []struct {
		name, options, input string
		valid                bool
	}{
		{"aligned unequal padding", `{"style":"aligned"}`, "| A   | B |\n| --- | - |\n| x   | y |\n", true},
		{"unaligned rows", `{"style":"aligned"}`, "| A   | B |\n| --- | - |\n| x | y |\n", false},
		{"any requires one style", `{}`, "| A  | B |\n| --- | --- |\n|x|y|\n", false},
		{"compact extra padding", `{"style":"compact"}`, "| A  | B |\n| --- | --- |\n| x | y |\n", false},
		{"tight space", `{"style":"tight"}`, "|A|B|\n|---|---|\n|x |y|\n", false},
		{"display width", `{"style":"aligned"}`, "| 猫 | B |\n|----|---|\n| xx | y |\n", true},
		{"escaped pipe", `{"style":"tight"}`, "|a\\|b|c|\n|---|---|\n|x|y|\n", true},
		{"quoted tight table", `{"style":"tight"}`, "> |A|B|\n> |---|---|\n> |x|y|\n", true},
		{"ignore malformed block", `{"style":"compact"}`, "| A | B |\n| --- | --- | --- |\n", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			diags := lintMigratedMarkdown(t, `{"default":false,"MD060":`+tt.options+`}`, tt.input)
			if tt.valid {
				require.Empty(t, diags)
			} else {
				require.NotEmpty(t, diags)
			}
		})
	}
}

func TestMigratedHeadingIncrementMetadataTitle(t *testing.T) {
	input := "---\ntitle: Document\n---\n\n### Skipped level\n"
	require.Len(t, lintMigratedMarkdown(t, `{"default": false, "MD001": true}`, input), 1)
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD001": {"front_matter_title": false}}`, input))
}

func TestMigratedHeadingBlankLineOptions(t *testing.T) {
	input := "# Title\ntext\n\n## Section\nbody\n"
	diags := lintMigratedMarkdown(t, `{"default": false, "MD022": {"lines_above": [0, 2], "lines_below": [0, 0]}}`, input)
	require.Len(t, diags, 1)
	require.Equal(t, 4, diags[0].StartLine)
	input = "---\nname: Document\n---\n# Title\n\nText.\n"
	require.Empty(t, lintMigratedMarkdown(t, `{"default": false, "MD022": true}`, input))
	require.Len(t, lintMigratedMarkdown(t, `{"default": false, "MD022": {"include_front_matter": true}}`, input), 1)
}

func TestRuleDefaultsCascadeAndValidation(t *testing.T) {
	base, err := config.FromYAML([]byte("rule_defaults:\n  MD009:\n    enabled: true\n    options:\n      br_spaces: 2\n"))
	require.NoError(t, err)
	override, err := config.FromYAML([]byte("rule_defaults:\n  MD009:\n    options:\n      br_spaces: 3\n"))
	require.NoError(t, err)
	merged := MergeAll(config.NewConfig(), base, override)
	require.Equal(t, 3, merged.RuleDefaults["MD009"].Options["br_spaces"])
	require.Equal(t, 2, base.RuleDefaults["MD009"].Options["br_spaces"], "merging must not mutate a reusable baseline")
	invalid := "invalid"
	merged.RuleDefaults["MD009"] = config.RuleConfig{Severity: &invalid}
	require.False(t, Validate(merged).Valid())
}

func TestRuleDefaultsLoadAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gomdlint.yml")
	require.NoError(t, os.WriteFile(path, []byte("rule_defaults:\n  no-trailing-spaces:\n    enabled: false\n"), 0o600))
	loaded, err := Load(context.Background(), LoadOptions{
		ExplicitPath: path, IgnoreEnv: true, IgnoreUserConfig: true, IgnoreSystemConfig: true,
		IgnoreMarkdownlint: true, NonInteractive: true,
	})
	require.NoError(t, err)
	require.Contains(t, loaded.Config.RuleDefaults, "MD009")
	require.False(t, *loaded.Config.RuleDefaults["MD009"].Enabled)
	clone := loaded.Config.Clone()
	enabled := true
	clone.RuleDefaults["MD009"] = config.RuleConfig{Enabled: &enabled}
	require.False(t, *loaded.Config.RuleDefaults["MD009"].Enabled)
}

func TestMigrationPreservesEnabledRules(t *testing.T) {
	for _, input := range []string{
		`{}`, `{"default": false, "MD009": true}`, `{"headings": false, "MD025": true}`,
	} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".markdownlint.json")
			require.NoError(t, os.WriteFile(path, []byte(input), 0o600))
			result, err := ConvertMarkdownlintConfig(path)
			require.NoError(t, err)
			registry := lint.NewRegistry()
			rules.RegisterAll(registry)
			enabled := make(map[string]bool)
			for _, rule := range lint.ResolveRules(registry, result.Config) {
				enabled[rule.Rule.ID()] = true
			}
			require.False(t, enabled["MDL003"], "migration must not enable extra rules")
			switch input {
			case `{}`:
				require.True(t, enabled["MD041"], "markdownlint enables MD041 by default")
				require.True(t, enabled["MD033"])
			case `{"default": false, "MD009": true}`:
				require.Equal(t, map[string]bool{"MD009": true}, enabled)
			default:
				require.False(t, enabled["MD041"])
				require.True(t, enabled["MD025"], "specific rule overrides tag regardless of map order")
			}
		})
	}
}

// TestMigrationFileHoldsOnlyRuleSettings covers the content of a migrated
// config file, in YAML and in JSON. A markdownlint config sets rules and
// nothing else, so the file holds rules and rule_defaults only. Any other
// key, even one at its default value such as ignore: [], would replace the
// setting of a user or system config. The file loads back to the settings
// of the migration.
func TestMigrationFileHoldsOnlyRuleSettings(t *testing.T) {
	t.Parallel()

	for _, name := range []string{".gomdlint.yml", ".gomdlint.json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			converted := convertSettings(t, `{"MD013": false, "whitespace": {"enabled": false}}`)
			path := filepath.Join(t.TempDir(), name)
			content, err := converted.FileContent(path)
			require.NoError(t, err)

			var keys map[string]any
			if HoldsJSON(path) {
				require.NoError(t, json.Unmarshal(content, &keys), string(content))
			} else {
				require.NoError(t, yaml.Unmarshal(content, &keys), string(content))
				assert.True(t, strings.HasPrefix(string(content), GenerateMigrationHeader(converted.SourcePath)))
			}
			assert.ElementsMatch(t, []string{"rules", "rule_defaults"}, slices.Collect(maps.Keys(keys)))

			require.NoError(t, os.WriteFile(path, content, 0o600))
			loaded, warnings, err := loadConfigFile(path)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.Equal(t, enabledStates(converted.Config.Rules), enabledStates(loaded.Rules))
			assert.Equal(t, enabledStates(converted.Config.RuleDefaults), enabledStates(loaded.RuleDefaults))
		})
	}
}

// enabledStates maps each rule of settings to whether it is enabled.
func enabledStates(settings map[string]config.RuleConfig) map[string]bool {
	states := make(map[string]bool, len(settings))
	for id, rule := range settings {
		states[id] = rule.Enabled != nil && *rule.Enabled
	}
	return states
}

func TestConvertMarkdownlintConfig_JSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdownlint JSON config
	configContent := `{
  "MD001": true,
  "MD009": false,
  "MD013": {
    "line_length": 120,
    "tables": false
  },
  "heading-increment": true
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	if result.Config == nil {
		t.Fatal("result.Config is nil")
	}

	// Check MD001 is enabled
	md001, ok := result.Config.Rules["MD001"]
	if !ok {
		t.Fatal("MD001 rule not found in config")
	}
	if md001.Enabled == nil || !*md001.Enabled {
		t.Error("expected MD001 to be enabled")
	}

	// Check MD009 is disabled
	md009, ok := result.Config.Rules["MD009"]
	if !ok {
		t.Fatal("MD009 rule not found in config")
	}
	if md009.Enabled == nil || *md009.Enabled {
		t.Error("expected MD009 to be disabled")
	}

	// Check MD013 has options
	md013, ok := result.Config.Rules["MD013"]
	if !ok {
		t.Fatal("MD013 rule not found in config")
	}
	if md013.Options == nil {
		t.Fatal("MD013 options is nil")
	}
	if lineLen, ok := md013.Options["max"].(float64); !ok || lineLen != 120 {
		t.Errorf("expected mapped maximum 120, got %v", md013.Options["max"])
	}
}

func TestConvertMarkdownlintConfig_YAML(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdownlint YAML config
	configContent := `
default: true
MD001: true
MD009: false
MD013:
  line_length: 100
`
	configPath := filepath.Join(tmpDir, ".markdownlint.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	if result.Config == nil {
		t.Fatal("result.Config is nil")
	}

	// Check MD013 options
	md013, ok := result.Config.Rules["MD013"]
	if !ok {
		t.Fatal("MD013 rule not found in config")
	}
	if md013.Options == nil {
		t.Fatal("MD013 options is nil")
	}
}

func TestConvertMarkdownlintConfig_Aliases(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a config using aliases
	configContent := `{
  "heading-increment": true,
  "no-trailing-spaces": false,
  "line-length": {
    "line_length": 80
  }
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// Aliases should be normalized to rule IDs
	if _, ok := result.Config.Rules["MD001"]; !ok {
		t.Error("heading-increment should be normalized to MD001")
	}

	if _, ok := result.Config.Rules["MD009"]; !ok {
		t.Error("no-trailing-spaces should be normalized to MD009")
	}

	if _, ok := result.Config.Rules["MD013"]; !ok {
		t.Error("line-length should be normalized to MD013")
	}
}

func TestConvertMarkdownlintConfig_Tags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a config using tag-based disabling
	configContent := `{
  "whitespace": false
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// All whitespace rules should be disabled: markdownlint's whitespace tag.
	whitespaceRules := []string{"MD009", "MD010", "MD012", "MD027", "MD028", "MD030", "MD037", "MD038", "MD039"}
	for _, ruleID := range whitespaceRules {
		rule, ok := result.Config.Rules[ruleID]
		if !ok {
			t.Errorf("expected %s to be in config (from whitespace tag)", ruleID)
			continue
		}
		if rule.Enabled == nil || *rule.Enabled {
			t.Errorf("expected %s to be disabled (from whitespace tag)", ruleID)
		}
	}
	if len(result.Config.Rules) != len(whitespaceRules) {
		t.Errorf("the whitespace tag configured %d rules, want %d", len(result.Config.Rules), len(whitespaceRules))
	}
}

// convertSettings converts markdownlint settings written to a
// .markdownlint.json file.
func convertSettings(t *testing.T, settings string) *MigrationResult {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".markdownlint.json")
	require.NoError(t, os.WriteFile(path, []byte(settings), 0o600))
	result, err := ConvertMarkdownlintConfig(path)
	require.NoError(t, err)
	return result
}

// convertFile converts the markdownlint config written to a file named
// name.
func convertFile(t *testing.T, name, content string) *MigrationResult {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	result, err := ConvertMarkdownlintConfig(path)
	require.NoError(t, err)
	return result
}

// TestConvertMarkdownlintConfig_AppliesKeysInWrittenOrder covers the order
// in which the keys of a markdownlint config apply: the order they are
// written, as in markdownlint 0.41.1, whose getEffectiveConfig applies the
// keys of its configuration object in the order JavaScript keeps them. A
// later key replaces what an earlier one set for the same rule, so whether
// no-trailing-spaces (MD009) ends up on depends on whether its own key or
// the whitespace tag comes last. A key written twice in JSON keeps its
// first place and takes its last value, as JSON.parse and the JSONC parser
// of markdownlint-cli2 read it. For each JSON config, markdownlint 0.41.1
// leaves MD009 on exactly when md009 is set, and markdownlint-cli2 0.23.3
// does the same for the JSONC and YAML files. markdownlint-cli2 reads YAML
// with js-yaml 5, which reads a merge key (<<) as an ordinary key that
// names nothing, while gomdlint applies merge keys, as YAML 1.1 and js-yaml
// 4 do, with the merged keys in the place of the merge key and a key
// written in the mapping keeping the first place of the merged key it
// replaces, as js-yaml 4 orders them.
func TestConvertMarkdownlintConfig_AppliesKeysInWrittenOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, file, content string
		md009               bool
	}{
		{name: "tag after rule", file: ".markdownlint.json", content: `{"MD009": true, "whitespace": false}`},
		{name: "rule after tag", file: ".markdownlint.json", content: `{"whitespace": false, "MD009": true}`, md009: true},
		{name: "ID after name", file: ".markdownlint.json", content: `{"no-trailing-spaces": true, "MD009": false}`},
		{
			name: "name after ID", file: ".markdownlint.json",
			content: `{"MD009": false, "no-trailing-spaces": true}`, md009: true,
		},
		{
			name: "key written twice", file: ".markdownlint.json",
			content: `{"whitespace": false, "MD009": true, "whitespace": false}`, md009: true,
		},
		{
			name: "JSONC", file: ".markdownlint.jsonc",
			content: "{\n  // The tag comes last.\n  \"MD009\": true, /* then */ \"whitespace\": false\n}\n",
		},
		{name: "YAML tag after rule", file: ".markdownlint.yaml", content: "MD009: true\nwhitespace: false\n"},
		{name: "YAML rule after tag", file: ".markdownlint.yml", content: "whitespace: false\nMD009: true\n", md009: true},
		{
			name: "YAML merge after rule", file: ".markdownlint.yaml",
			content: "x: &ws\n  whitespace: false\nMD009: true\n<<: *ws\n",
		},
		{
			name: "YAML merge before rule", file: ".markdownlint.yaml",
			content: "x: &ws\n  whitespace: false\n<<: *ws\nMD009: true\n", md009: true,
		},
		{
			name: "YAML key replacing a merged key", file: ".markdownlint.yaml",
			content: "x: &m\n  MD009: false\n  whitespace: false\n<<: *m\nMD009: true\n",
		},
		{
			name: "YAML merges in sequence", file: ".markdownlint.yaml",
			content: "a: &a\n  MD009: true\nb: &b\n  whitespace: false\n  MD009: false\n<<: [*a, *b]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := convertFile(t, tt.file, tt.content)

			requireRuleEnabled(t, result, "MD009", tt.md009)
		})
	}
}

// TestConvertMarkdownlintConfig_EmptyConfigs covers configs that hold no
// settings, which keep markdownlint's defaults, as markdownlint-cli2 reads
// them, and configs that hold a value other than an object, which fail.
func TestConvertMarkdownlintConfig_EmptyConfigs(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ file, content string }{
		{".markdownlint.json", "null\n"},
		{".markdownlint.jsonc", "// Nothing yet.\n{}\n"},
		{".markdownlint.yaml", ""},
		{".markdownlint.yaml", "# Nothing yet.\n"},
		{".markdownlint.yml", "~\n"},
	} {
		result := convertFile(t, tt.file, tt.content)
		assert.Empty(t, result.Warnings, tt.content)
		assert.Empty(t, result.Config.Rules, tt.content)
		require.Contains(t, result.Config.RuleDefaults, "MD009", tt.content)
		assert.True(t, *result.Config.RuleDefaults["MD009"].Enabled, tt.content)
	}

	for _, tt := range []struct{ file, content string }{
		{".markdownlint.json", `["MD009"]`},
		{".markdownlint.json", ""},
		{".markdownlint.yaml", "- MD009\n"},
		{".markdownlint.yaml", "MD009\n"},
	} {
		path := filepath.Join(t.TempDir(), tt.file)
		require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))
		_, err := ConvertMarkdownlintConfig(path)
		assert.Error(t, err, tt.content)
	}
}

// TestMigratedLaterKeyReplacesOptions covers two keys that name one rule,
// line-length (MD013): as in markdownlint 0.41.1, the later key replaces the
// earlier one's options, so only its own options and markdownlint's
// defaults apply. markdownlint 0.41.1 reports the 90-character line with
// the first config and not with the second.
func TestMigratedLaterKeyReplacesOptions(t *testing.T) {
	input := "# Title\n\n" + strings.Repeat("word ", 17) + "words\n"
	require.Len(t, lintMigratedMarkdown(t,
		`{"default": false, "MD013": {"line_length": 100}, "line-length": {"tables": false}}`, input), 1)
	require.Empty(t, lintMigratedMarkdown(t,
		`{"default": false, "line-length": {"tables": false}, "MD013": {"line_length": 100}}`, input))
}

// requireRuleEnabled checks that converted settings set the rule's enabled state.
func requireRuleEnabled(t *testing.T, result *MigrationResult, ruleID string, want bool) {
	t.Helper()
	rule, ok := result.Config.Rules[ruleID]
	require.True(t, ok, "%s is configured", ruleID)
	require.NotNil(t, rule.Enabled, ruleID)
	require.Equal(t, want, *rule.Enabled, ruleID)
}

func TestConvertMarkdownlintConfig_KeysIgnoreCase(t *testing.T) {
	t.Parallel()

	// markdownlint matches names, aliases and tags in any case.
	result := convertSettings(t, `{"UL-STYLE": false, "Blank_Lines": false, "md009": false, "No-Hard-Tabs": false}`)

	for _, ruleID := range []string{"MD004", "MD009", "MD010", "MD012", "MD022", "MD031", "MD032", "MD047"} {
		requireRuleEnabled(t, result, ruleID, false)
	}
	assert.Empty(t, result.Warnings)
}

func TestConvertMarkdownlintConfig_GomdlintNames(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{"heading-blank-lines": false, "table-alignment": true}`)

	requireRuleEnabled(t, result, "MD022", false)
	requireRuleEnabled(t, result, "MDL003", true)
	assert.Empty(t, result.Warnings)
}

func TestConvertMarkdownlintConfig_UnknownRuleIDs(t *testing.T) {
	t.Parallel()

	// markdownlint removed MD002 and never had MD999.
	result := convertSettings(t, `{"MD002": false, "MD999": true}`)

	assert.NotContains(t, result.Config.Rules, "MD002")
	assert.NotContains(t, result.Config.Rules, "MD999")
	// Each is as close to several rules as to any, so neither gets a
	// suggestion.
	assert.Equal(t, []string{`unknown rule "MD002"; it is ignored`, `unknown rule "MD999"; it is ignored`}, result.Warnings)
}

// TestConvertMarkdownlintConfig_SuggestsRulesForUnknownKeys covers typos in
// a markdownlint config, which takes rules and tags alike, so either may be
// suggested.
func TestConvertMarkdownlintConfig_SuggestsRulesForUnknownKeys(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{"no-trailling-spaces": false, "tables": false}`)

	assert.Equal(t, []string{
		`unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
		`unknown rule "tables" (did you mean the tag "table"?); it is ignored`,
	}, result.Warnings)
}

// TestConvertMarkdownlintConfig_KeysNameOneRuleOrTag covers keys that
// markdownlint looks up whole, after only upper-casing them: a key of
// several words, a key with surrounding whitespace and an empty key name
// nothing there, so migration does not configure their rules either.
func TestConvertMarkdownlintConfig_KeysNameOneRuleOrTag(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{"MD009 MD010": false, " MD013": false, "whitespace ": false, "": false}`)

	for _, ruleID := range []string{"MD009", "MD010", "MD013"} {
		assert.NotContains(t, result.Config.Rules, ruleID)
	}
	// The warnings follow the keys, in the order they are written.
	assert.Equal(t, []string{
		`unknown rule "MD009 MD010" (a key names one rule or tag; write "MD009" and "MD010" as separate keys); it is ignored`,
		`unknown rule " MD013" (did you mean "line-length"?); it is ignored`,
		`unknown rule "whitespace " (did you mean the tag "whitespace"?); it is ignored`,
		`unknown rule ""; it is ignored`,
	}, result.Warnings)
}

func TestConvertMarkdownlintConfig_TagsCoverMarkdownlintRulesOnly(t *testing.T) {
	t.Parallel()

	// gomdlint files its own rules under markdownlint tags, but a
	// markdownlint tag in a markdownlint config means markdownlint's rules.
	result := convertSettings(t, `{"links": true, "url": true, "table": true}`)

	assert.NotContains(t, result.Config.Rules, "MDL001")
	assert.NotContains(t, result.Config.Rules, "MDL003")
	for _, ruleID := range []string{"MD011", "MD034", "MD042", "MD055", "MD058", "MD059"} {
		requireRuleEnabled(t, result, ruleID, true)
	}

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	for _, resolved := range lint.ResolveRules(registry, result.Config) {
		assert.NotContains(t, []string{"MDL001", "MDL003"}, resolved.Rule.ID(), "migration must not enable gomdlint's own rules")
	}
}

func TestConvertMarkdownlintConfig_SpecialKeys(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a config with special keys
	configContent := `{
  "$schema": "https://example.com/schema.json",
  "default": false,
  "extends": "some-preset",
  "MD001": true
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// Should have warnings about special keys
	if len(result.Warnings) == 0 {
		t.Error("expected warnings about default and extends")
	}

	// MD001 should still be processed
	if _, ok := result.Config.Rules["MD001"]; !ok {
		t.Error("MD001 should be in config")
	}
}

func TestConvertMarkdownlintConfig_JavaScript(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a JavaScript config file
	configPath := filepath.Join(tmpDir, ".markdownlint.cjs")
	if err := os.WriteFile(configPath, []byte("module.exports = {}"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ConvertMarkdownlintConfig(configPath)
	if err == nil {
		t.Fatal("expected error for JavaScript config file")
	}
}

func TestConvertMarkdownlintConfig_InvalidJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create an invalid JSON config
	configPath := filepath.Join(tmpDir, ".markdownlint.json")
	if err := os.WriteFile(configPath, []byte("{ invalid json }"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := ConvertMarkdownlintConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestConvertMarkdownlintConfig_JSONC(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a JSONC config with comments
	configContent := `{
  // This is a comment
  "MD001": true,
  /* Multi-line
     comment */
  "MD009": false
}`
	configPath := filepath.Join(tmpDir, ".markdownlint.jsonc")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := ConvertMarkdownlintConfig(configPath)
	if err != nil {
		t.Fatalf("ConvertMarkdownlintConfig() error = %v", err)
	}

	// Check rules were parsed correctly
	if _, ok := result.Config.Rules["MD001"]; !ok {
		t.Error("MD001 should be in config")
	}
	if _, ok := result.Config.Rules["MD009"]; !ok {
		t.Error("MD009 should be in config")
	}
}

func TestCanMigrate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{".markdownlint.json", true},
		{".markdownlint.jsonc", true},
		{".markdownlint.yaml", true},
		{".markdownlint.yml", true},
		{".markdownlint.cjs", false},
		{".markdownlint.mjs", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			result := CanMigrate(tt.path)
			if result != tt.expected {
				t.Errorf("CanMigrate(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsJavaScriptConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{".markdownlint.cjs", true},
		{".markdownlint.mjs", true},
		{".markdownlint.json", false},
		{".markdownlint.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			result := IsJavaScriptConfig(tt.path)
			if result != tt.expected {
				t.Errorf("IsJavaScriptConfig(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
