package configloader

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// legacyMarkdownlintRuleConfig is config.MarkdownlintRuleConfig as it was
// before the rule catalog declared options, on main at 580bd58 (#42): it
// reads a value with JavaScript's truthiness, sets some of markdownlint's
// defaults explicitly, renames a few options by hand, and passes every
// other option through. It is kept here, unchanged apart from its name, as
// the reference that TestMarkdownlintTranslationKeepsEffectiveOptions
// compares the catalog-driven translation with.
func legacyMarkdownlintRuleConfig(ruleID string, value any) config.RuleConfig {
	enabled := true
	severity := string(config.SeverityError)
	cfg := config.RuleConfig{Enabled: &enabled, Severity: &severity}
	switch ruleID {
	case "MD001":
		cfg.Options = map[string]any{"front_matter_title": config.MarkdownlintFrontMatterTitlePattern}
	case "MD004", "MD046", "MD048":
		cfg.Options = map[string]any{"style": "consistent"}
	case "MD009":
		cfg.Options = map[string]any{"ignore_code_blocks": true, "br_spaces": 2}
	case "MD013":
		cfg.Options = map[string]any{
			"max": 80, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true,
		}
	case "MD025":
		cfg.Options = map[string]any{"title_only": true}
	case "MD022", "MD060":
		cfg.Options = map[string]any{"markdownlint_compatible": true}
	case "MD029":
		cfg.Options = map[string]any{"style": "one_or_ordered"}
	case "MD033":
		cfg.Options = map[string]any{"allowed_elements": []string{}}
	case "MD059":
		cfg.Options = map[string]any{
			"patterns": []string{"click here", "here", "link", "more"}, "markdownlint_compatible": true,
		}
	}
	switch typed := value.(type) {
	case string:
		*cfg.Enabled = typed != ""
		if typed == "warning" {
			severity = typed
		}
	case map[string]any:
		if cfg.Options == nil {
			cfg.Options = make(map[string]any)
		}
		for key, option := range typed {
			if ruleID == "MD059" && key == "prohibited_texts" {
				key = "patterns"
			}
			if ruleID == "MD012" && key == "maximum" {
				key = "max_consecutive"
			}
			switch key {
			case "enabled":
				*cfg.Enabled = legacyTruthy(option)
				continue
			case "severity":
				if explicitSeverity, ok := option.(string); ok {
					cfg.Severity = &explicitSeverity
				}
				continue
			}
			if ruleID == "MD013" {
				switch key {
				case "line_length":
					key = "max"
				case "heading_line_length":
					key = "heading_max"
				case "code_block_line_length":
					key = "code_block_max"
				case "code_blocks", "tables", "headings":
					if include, ok := option.(bool); ok {
						cfg.Options["ignore_"+key] = !include
						continue
					}
				}
			}
			if ruleID == "MD009" && key == "code_blocks" {
				if include, ok := option.(bool); ok {
					cfg.Options["ignore_code_blocks"] = !include
					continue
				}
			}
			cfg.Options[key] = option
		}
	default:
		// Booleans, null, numbers and arrays, as JavaScript tests them.
		*cfg.Enabled = legacyTruthy(value)
	}
	return cfg
}

// legacyTruthy is markdownlintTruthy of pkg/config on main at 580bd58: it
// reports whether value is truthy in JavaScript.
func legacyTruthy(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Bool:
		return reflected.Bool()
	case reflect.String:
		return reflected.Len() > 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflected.Uint() != 0
	case reflect.Float32, reflect.Float64:
		number := reflected.Float()
		return number != 0 && !math.IsNaN(number)
	default:
		return true
	}
}

// legacyMarkdownlintDefaultRuleConfig is config.MarkdownlintDefaultRuleConfig
// on main at 580bd58, unchanged apart from the names it calls.
func legacyMarkdownlintDefaultRuleConfig(ruleID string, value any) config.RuleConfig {
	switch text, isText := value.(string); {
	case isText && text == "warning":
		return legacyMarkdownlintRuleConfig(ruleID, "warning")
	case legacyTruthy(value):
		return legacyMarkdownlintRuleConfig(ruleID, true)
	default:
		return legacyMarkdownlintRuleConfig(ruleID, false)
	}
}

// legacyTranslator translates rules as migration did on main at 580bd58,
// before the catalog declared options.
func legacyTranslator() markdownlintTranslator {
	return markdownlintTranslator{
		rule: func(_ *lint.Registry, ruleID string, value any) (config.RuleConfig, []lint.OptionIssue) {
			return legacyMarkdownlintRuleConfig(ruleID, value), nil
		},
		defaultRule: func(_ *lint.Registry, ruleID string, value any) config.RuleConfig {
			return legacyMarkdownlintDefaultRuleConfig(ruleID, value)
		},
	}
}

// lowerLayer is a configuration layer under a markdownlint config, such as
// a user config: it sets options of the rules that #42's translation writes
// options for, both options that it writes and options that it leaves out.
func lowerLayer() *config.Config {
	return &config.Config{
		RuleDefaults: map[string]config.RuleConfig{
			"MD001": {Options: map[string]any{"front_matter_title": "^name:"}},
			"MD004": {Options: map[string]any{"style": "dash"}},
			"MD009": {Options: map[string]any{"br_spaces": 4, "strict": true}},
			"MD013": {Options: map[string]any{"max": 20, "ignore_urls": true, "stern": true}},
			"MD022": {Options: map[string]any{"markdownlint_compatible": false, "lines_above": 2}},
		},
		Rules: map[string]config.RuleConfig{
			"MD024": {Options: map[string]any{"siblings_only": true}},
			"MD025": {Options: map[string]any{"title_only": false, "level": 2}},
			"MD029": {Options: map[string]any{"style": "ordered"}},
			"MD033": {Options: map[string]any{"allowed_elements": []any{"br"}}},
			"MD046": {Options: map[string]any{"style": "indented"}},
			"MD048": {Options: map[string]any{"style": "tilde"}},
			"MD059": {Options: map[string]any{"patterns": []any{"x"}, "markdownlint_compatible": false}},
			"MD060": {Options: map[string]any{"markdownlint_compatible": false, "style": "tight"}},
		},
	}
}

// markdownlintFixture is a markdownlint config file.
type markdownlintFixture struct {
	name, file, content string
}

// everyOptionSettings sets every markdownlint parameter to a value other
// than its default.
const everyOptionSettings = `{
	"MD001": {"front_matter_title": "^name:"},
	"MD003": {"style": "atx"},
	"MD004": {"style": "dash"},
	"MD007": {"indent": 4, "start_indented": true, "start_indent": 3},
	"MD009": {"br_spaces": 3, "code_blocks": true, "list_item_empty_lines": true, "strict": true},
	"MD010": {"code_blocks": false, "ignore_code_languages": ["make"], "spaces_per_tab": 4},
	"MD012": {"maximum": 2},
	"MD013": {"line_length": 100, "heading_line_length": 90, "code_block_line_length": 120,
		"code_blocks": false, "tables": false, "headings": false, "strict": true, "stern": true},
	"MD022": {"lines_above": 2, "lines_below": [1, 2], "include_front_matter": true},
	"MD024": {"siblings_only": true},
	"MD025": {"level": 2, "front_matter_title": ""},
	"MD026": {"punctuation": ".,"},
	"MD027": {"list_items": false},
	"MD029": {"style": "ordered"},
	"MD030": {"ul_single": 2, "ol_single": 2, "ul_multi": 3, "ol_multi": 3},
	"MD031": {"list_items": false},
	"MD033": {"allowed_elements": ["br"], "table_allowed_elements": ["td"]},
	"MD035": {"style": "***"},
	"MD036": {"punctuation": "."},
	"MD040": {"allowed_languages": ["go"], "language_only": true},
	"MD041": {"level": 2, "front_matter_title": false, "allow_preamble": true},
	"MD043": {"headings": ["# Title", "*"], "match_case": true},
	"MD044": {"names": ["GitHub"], "code_blocks": false, "html_elements": false},
	"MD046": {"style": "fenced"},
	"MD048": {"style": "tilde"},
	"MD049": {"style": "underscore"},
	"MD050": {"style": "asterisk"},
	"MD051": {"ignore_case": true, "ignored_pattern": "^top$"},
	"MD052": {"ignored_labels": [], "shortcut_syntax": true},
	"MD053": {"ignored_definitions": []},
	"MD054": {"autolink": false, "inline": false, "full": false, "collapsed": false, "shortcut": false, "url_inline": false},
	"MD055": {"style": "leading_only"},
	"MD059": {"prohibited_texts": ["details"]},
	"MD060": {"style": "tight", "aligned_delimiter": true}
}`

// markdownlintFixtures returns the markdownlint configs of the #19
// regression tests (the TestMigrated tests), of the conversion tests and of
// the CLI's migration goldens, and a few more: every parameter set, every
// rule set to true, false or a severity, gomdlint's own options, and an
// option that no rule reads.
func markdownlintFixtures() []markdownlintFixture {
	settings := []string{
		// TestMigrated*.
		`{"default": false, "MD009": true}`,
		`{"default": false, "MD009": {"code_blocks": true, "br_spaces": 3}}`,
		`{"default": false, "MD009": {"strict": true}}`,
		`{"default": false, "MD009": {"list_item_empty_lines": true, "br_spaces": 0}}`,
		`{"default": false, "MD013": true}`,
		`{"default": false, "MD013": {"line_length": 20}}`,
		`{"default": false, "MD013": {"line_length": 20, "heading_line_length": 30, "code_block_line_length": 10, "tables": false}}`,
		`{"default": false, "MD013": {"strict": true}}`,
		`{"default": false, "MD013": {"stern": true}}`,
		`{"default": false, "MD013": {"line_length": 10, "strict": true}}`,
		`{"default": false, "MD013": {"code_blocks": false, "strict": true}}`,
		`{"default": false, "MD025": true}`,
		`{"default": false, "MD009": {"enabled": false}}`,
		`{"default": false, "MD009": {"severity": "error"}}`,
		`{"default": false, "MD033": true}`,
		`{"default": false, "MD033": {"allowed_elements": ["sup"]}}`,
		`{}`,
		`{"MD009": true}`,
		`{"default": false}`,
		`{"default": false, "MD004": true}`,
		`{"default": false, "MD046": true}`,
		`{"default": false, "MD048": true}`,
		`{"default": false, "MD012": {"maximum": 2}}`,
		`{"default": false, "MD012": {"maximum": 1}}`,
		`{"default": false, "whitespace": {"br_spaces": 3}}`,
		`{"whitespace": {"enabled": false}}`,
		`{"default": false, "MD041": {"allow_preamble": true}}`,
		`{"default": "warning"}`,
		`{"default": false, "MD009": "warning"}`,
		`{"default": false, "MD029": true}`,
		`{"default": false, "MD029": {"style": "one"}}`,
		`{"default": false, "MD029": {"style": "ordered"}}`,
		`{"default": false, "MD029": {"style": "zero"}}`,
		`{"default": false, "MD040": {"language_only": true}}`,
		`{"default": false, "MD059": {"prohibited_texts": ["details"]}}`,
		`{"default": false, "MD059": {"prohibited_texts": []}}`,
		`{"default": false, "MD059": true}`,
		`{"default": false, "MD033": {"table_allowed_elements": ["br"]}}`,
		`{"default": false, "MD060": {"style": "compact"}}`,
		`{"default": false, "MD060": {"style": "compact", "aligned_delimiter": true}}`,
		`{"default": false, "MD060": {"style": "aligned"}}`,
		`{"default": false, "MD060": {"style": "tight"}}`,
		`{"default": false, "MD060": {}}`,
		`{"default": false, "MD001": true}`,
		`{"default": false, "MD001": {"front_matter_title": false}}`,
		`{"default": false, "MD022": {"lines_above": [0, 2], "lines_below": [0, 0]}}`,
		`{"default": false, "MD022": true}`,
		`{"default": false, "MD022": {"include_front_matter": true}}`,
		`{"headings": false, "MD025": true}`,
		// TestConvertMarkdownlintConfig_*.
		`{"MD001": true, "MD009": false, "MD013": {"line_length": 120, "tables": false}, "heading-increment": true}`,
		`{"heading-increment": true, "no-trailing-spaces": false, "line-length": {"line_length": 80}}`,
		`{"whitespace": false}`,
		`{"UL-STYLE": false, "Blank_Lines": false, "md009": false, "No-Hard-Tabs": false}`,
		`{"heading-blank-lines": false, "table-alignment": true}`,
		`{"MD002": false, "MD999": true}`,
		`{"no-trailling-spaces": false, "tables": false}`,
		`{"MD009 MD010": false, " MD013": false, "whitespace ": false, "": false}`,
		`{"links": true, "url": true, "table": true}`,
		`{"$schema": "https://example.com/schema.json", "default": false, "extends": "some-preset", "MD001": true}`,
		// The CLI's migration goldens.
		`{"no-trailling-spaces": false, "tables": false, "MD009 MD010": false}`,
		// #42's tests of key order, the default key and truthiness.
		`{"MD009": true, "whitespace": false}`,
		`{"whitespace": false, "MD009": true}`,
		`{"no-trailing-spaces": true, "MD009": false}`,
		`{"whitespace": false, "MD009": true, "whitespace": false}`,
		`{"MD013": false, "whitespace": {"enabled": false}}`,
		`{"Default": false, "default": true}`,
		`{"MD013": false, "dEfAuLt": false, "DEFAULT": true}`,
		`{"default": {"enabled": false, "br_spaces": 5}}`,
		`{"default": 0, "MD009": 1, "MD010": "", "MD013": {"enabled": 0}, "MD033": [], "MD041": "off"}`,
		`{"Default": "warning"}`,
		`{"default": false, "MD013": {"line_length": 100}, "line-length": {"tables": false}}`,
		`{"default": false, "line-length": {"tables": false}, "MD013": {"line_length": 100}}`,
		// More.
		everyOptionSettings,
		`{"default": false, "MD013": {"ignore_urls": true, "markdownlint_compatible": false, "bogus": 1}}`,
		`{"default": "error", "MD013": "warning", "MD009": null, "MD033": {"enabled": true}}`,
	}

	fixtures := make([]markdownlintFixture, 0, len(settings)+4)
	for _, content := range settings {
		fixtures = append(fixtures, markdownlintFixture{name: content, file: ".markdownlint.json", content: content})
	}
	return append(fixtures,
		markdownlintFixture{
			name: "TestConvertMarkdownlintConfig_YAML", file: ".markdownlint.yaml",
			content: "default: true\nMD001: true\nMD009: false\nMD013:\n  line_length: 100\n",
		},
		markdownlintFixture{
			name: "JSON with comments", file: ".markdownlint.jsonc",
			content: "{\n  // The tag comes last.\n  \"MD013\": {\"line_length\": 90}, /* then */ \"whitespace\": false\n}\n",
		},
		markdownlintFixture{
			name: "YAML merge", file: ".markdownlint.yaml",
			content: "x: &ws\n  whitespace: false\n<<: *ws\nMD009: true\nMD013:\n  code_blocks: false\n",
		},
		markdownlintFixture{
			name: "YAML merged options", file: ".markdownlint.yml",
			content: "base: &length\n  line_length: 100\nMD013:\n  <<: *length\n  tables: false\n",
		},
	)
}

// effectiveOptions describes how linting runs each rule under cfg: for
// every rule that cfg turns on, its severity, whether --fix applies its
// fixes, and the value it reads for each option it declares, the value
// set or else the default. Options that no rule reads change nothing, so
// they are left out.
func effectiveOptions(t *testing.T, cfg *config.Config) map[string]string {
	t.Helper()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	effective := make(map[string]string)
	for _, resolved := range lint.ResolveRules(registry, cfg) {
		meta, ok := registry.Meta(resolved.Rule.ID())
		require.True(t, ok, resolved.Rule.ID())

		var set map[string]any
		if resolved.Config != nil {
			set = resolved.Config.Options
		}
		values := make(map[string]any, len(meta.Options))
		for _, takesAnother := range []bool{false, true} {
			for _, spec := range meta.Options {
				if (spec.DefaultFrom != "") != takesAnother {
					continue
				}
				switch value, ok := set[spec.Name]; {
				case ok && value != nil:
					values[spec.Name] = value
				case takesAnother:
					values[spec.Name] = values[spec.DefaultFrom]
				default:
					values[spec.Name] = spec.Default
				}
			}
		}

		encoded, err := json.Marshal(map[string]any{
			"severity": resolved.Severity, "fixable": resolved.Fixable, "options": values,
		})
		require.NoError(t, err)
		effective[resolved.Rule.ID()] = string(encoded)
	}
	return effective
}

// TestMarkdownlintTranslationKeepsEffectiveOptions checks that translating
// rule options through the catalog leaves every rule as #42 left it. For
// each fixture, #42's translation and the catalog's give the same rules,
// with the same severities, fixes and option values, and so do the two ways
// of using a markdownlint config: loading it directly for a run, and
// loading the gomdlint config that migrate writes from it.
func TestMarkdownlintTranslationKeepsEffectiveOptions(t *testing.T) {
	t.Parallel()

	for _, fixture := range markdownlintFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			source := filepath.Join(dir, fixture.file)
			require.NoError(t, os.WriteFile(source, []byte(fixture.content), 0o600))

			legacy, err := convertMarkdownlintConfig(source, legacyTranslator())
			require.NoError(t, err)
			converted, err := ConvertMarkdownlintConfig(source)
			require.NoError(t, err)
			want := effectiveOptions(t, legacy.Config)
			assert.Equal(t, want, effectiveOptions(t, converted.Config), "the translation")
			assert.Equal(t, effectiveOptions(t, merge(lowerLayer(), legacy.Config)),
				effectiveOptions(t, merge(lowerLayer(), converted.Config)), "the translation over a lower layer")

			direct, err := Load(context.Background(), LoadOptions{
				WorkingDir: dir, IgnoreEnv: true, IgnoreUserConfig: true, IgnoreSystemConfig: true, NonInteractive: true,
			})
			require.NoError(t, err)
			assert.Equal(t, want, effectiveOptions(t, direct.Config), "the markdownlint config loaded for a run")

			migrated := filepath.Join(t.TempDir(), ".gomdlint.yml")
			content, err := converted.FileContent(migrated)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(migrated, content, 0o600))
			loaded, err := Load(context.Background(), LoadOptions{
				WorkingDir: filepath.Dir(migrated), ExplicitPath: migrated, IgnoreEnv: true,
				IgnoreUserConfig: true, IgnoreSystemConfig: true, IgnoreMarkdownlint: true, NonInteractive: true,
			})
			require.NoError(t, err)
			assert.Equal(t, want, effectiveOptions(t, loaded.Config), "the migrated config")
		})
	}
}

// TestMarkdownlintTranslationWritesWhat42Wrote checks the settings that the
// catalog-driven translation gives each rule against #42's, for a rule key
// and for the default key, with every kind of value: the same state, the
// same severity and the same options, including the defaults that #42
// wrote out for some options of some rules, which replace the options of a
// lower configuration layer. Options that are nil and options that are
// empty count as the same: both write options: {} and set nothing.
func TestMarkdownlintTranslationWritesWhat42Wrote(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	values := []any{true, false, nil, "warning", "", float64(0), map[string]any{}, map[string]any{"enabled": false}}
	encode := func(cfg config.RuleConfig) string {
		if cfg.Options == nil {
			cfg.Options = map[string]any{}
		}
		encoded, err := json.Marshal(cfg)
		require.NoError(t, err)
		return string(encoded)
	}
	for _, id := range registry.IDs() {
		for _, value := range values {
			converted, issues := registry.MarkdownlintRuleConfig(id, value)
			assert.Empty(t, issues, "%s %#v", id, value)
			assert.Equal(t, encode(legacyMarkdownlintRuleConfig(id, value)), encode(converted), "%s %#v", id, value)
			assert.Equal(t, encode(legacyMarkdownlintDefaultRuleConfig(id, value)),
				encode(registry.MarkdownlintDefaultRuleConfig(id, value)), "%s default %#v", id, value)
		}
	}
}
