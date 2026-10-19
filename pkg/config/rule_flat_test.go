package config_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestRuleConfig_FlatOptions covers markdownlint's shape of a rule's
// settings, with the options directly under the rule key, in YAML and in
// JSON, under rules and under rule_defaults. Every key that is not a rule
// setting is an option.
func TestRuleConfig_FlatOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		section func(*config.Config) map[string]config.RuleConfig
		key     string
		want    map[string]any
	}{
		{
			name:    "YAML",
			content: "rules:\n  line-length:\n    line_length: 100\n    code_blocks: false\n",
			section: rulesSection, key: "line-length",
			want: map[string]any{"line_length": 100, "code_blocks": false},
		},
		{
			name:    "JSON",
			content: `{"rules": {"MD013": {"line_length": 100, "tables": false}}}`,
			section: rulesSection, key: "MD013",
			want: map[string]any{"line_length": 100, "tables": false},
		},
		{
			name:    "flow mapping",
			content: "rules:\n  MD013: {line_length: 100}\n",
			section: rulesSection, key: "MD013",
			want: map[string]any{"line_length": 100},
		},
		{
			name:    "list value",
			content: "rules:\n  no-inline-html:\n    allowed_elements: [br, sup]\n",
			section: rulesSection, key: "no-inline-html",
			want: map[string]any{"allowed_elements": []any{"br", "sup"}},
		},
		{
			name:    "null value",
			content: "rules:\n  MD013:\n    line_length:\n",
			section: rulesSection, key: "MD013",
			want: map[string]any{"line_length": nil},
		},
		{
			name:    "rule_defaults",
			content: "rule_defaults:\n  MD009:\n    br_spaces: 3\n",
			section: ruleDefaultsSection, key: "MD009",
			want: map[string]any{"br_spaces": 3},
		},
		{
			name:    "rule_defaults in JSON",
			content: `{"rule_defaults": {"no-trailing-spaces": {"code_blocks": true}}}`,
			section: ruleDefaultsSection, key: "no-trailing-spaces",
			want: map[string]any{"code_blocks": true},
		},
		{
			name:    "next to empty options",
			content: "rules:\n  MD013:\n    options:\n    line_length: 100\n",
			section: rulesSection, key: "MD013",
			want: map[string]any{"line_length": 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.FromYAML([]byte(tt.content))
			require.NoError(t, err)

			section := tt.section(cfg)
			require.Contains(t, section, tt.key)
			rule := section[tt.key]
			assert.Equal(t, tt.want, rule.Options)
			assert.Nil(t, rule.Enabled, "options leave enabled alone")
			assert.Nil(t, rule.Severity, "options leave the severity alone")
			assert.Nil(t, rule.AutoFix, "options leave auto-fix alone")
		})
	}
}

func rulesSection(cfg *config.Config) map[string]config.RuleConfig { return cfg.Rules }

func ruleDefaultsSection(cfg *config.Config) map[string]config.RuleConfig { return cfg.RuleDefaults }

// TestRuleConfig_FlatOptionsBesideSettings covers a mapping that holds rule
// settings, flat options and an options mapping at once. The settings stay
// settings, and the flat options and the options mapping merge into one set
// of options, so no key is dropped.
func TestRuleConfig_FlatOptionsBesideSettings(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"YAML": "rules:\n  MD013:\n    enabled: true\n    severity: error\n    auto_fix: false\n" +
			"    line_length: 100\n    options:\n      tables: false\n",
		"JSON": `{"rules": {"MD013": {"enabled": true, "severity": "error", "auto_fix": false,` +
			` "line_length": 100, "options": {"tables": false}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.FromYAML([]byte(content))
			require.NoError(t, err)

			rule := cfg.Rules["MD013"]
			require.NotNil(t, rule.Enabled)
			assert.True(t, *rule.Enabled)
			require.NotNil(t, rule.Severity)
			assert.Equal(t, "error", *rule.Severity)
			require.NotNil(t, rule.AutoFix)
			assert.False(t, *rule.AutoFix)
			assert.Equal(t, map[string]any{"line_length": 100, "tables": false}, rule.Options)
		})
	}
}

// TestRuleConfig_SettingKeysAreNeverOptions covers keys named like a rule
// setting: they always set the setting, so a rule option with such a name
// can only be written under options. No built-in rule has one (see the
// catalog's tests).
func TestRuleConfig_SettingKeysAreNeverOptions(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte(
		"rules:\n  custom:\n    enabled: false\n    severity: error\n    auto_fix: true\n" +
			"    options:\n      severity: high\n"))
	require.NoError(t, err)

	rule := cfg.Rules["custom"]
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	require.NotNil(t, rule.Severity)
	assert.Equal(t, "error", *rule.Severity)
	require.NotNil(t, rule.AutoFix)
	assert.True(t, *rule.AutoFix)
	assert.Equal(t, map[string]any{"severity": "high"}, rule.Options,
		"an option named like a setting is read from options only")
}

// TestRuleConfig_RuleSettingKeys checks that RuleSettingKeys lists exactly
// the keys that RuleConfig's fields read.
func TestRuleConfig_RuleSettingKeys(t *testing.T) {
	t.Parallel()

	var tags []string
	typ := reflect.TypeFor[config.RuleConfig]()
	for idx := range typ.NumField() {
		if field := typ.Field(idx); field.IsExported() {
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			tags = append(tags, name)
		}
	}
	assert.ElementsMatch(t, tags, config.RuleSettingKeys())
	assert.Equal(t, []string{"enabled", "severity", "auto_fix", "options"}, config.RuleSettingKeys())

	keys := config.RuleSettingKeys()
	keys[0] = "changed"
	assert.Equal(t, "enabled", config.RuleSettingKeys()[0], "the caller gets its own list")
}

// TestRuleConfig_RejectsAnOptionWrittenTwice covers an option written both
// directly under the rule key and under options. Either value could win,
// so reading the flat option fails and names the line of the flat key. Two
// different spellings of one option, such as line_length and max, read side
// by side: only the rule's declared options tell that they are one, and the
// loader reports them.
func TestRuleConfig_RejectsAnOptionWrittenTwice(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"YAML": "rules:\n  MD013:\n    line_length: 100\n    options:\n      line_length: 90\n",
		"JSON": "{\"rules\": {\n  \"MD013\": {\n    \"line_length\": 100, \"options\": {\"line_length\": 90}}}}",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.FromYAML([]byte(content))

			require.Error(t, err)
			assert.Contains(t, err.Error(),
				`line 3: "line_length" and "options.line_length" both set option "line_length"; keep one of them`)
			var conflict *config.FlatOptionConflictError
			require.ErrorAs(t, err, &conflict)
			assert.Equal(t, config.FlatOptionConflictError{Section: "rules", Rule: "MD013", Name: "line_length", Line: 3},
				*conflict)
		})
	}

	cfg, err := config.FromYAML([]byte("rules:\n  MD013:\n    line_length: 100\n    options:\n      max: 90\n"))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"line_length": 100, "max": 90}, cfg.Rules["MD013"].Options)
}

// TestRuleConfig_OptionWrittenTwiceThroughAMerge covers a flat option that
// a merge key brings in, also set under options. The error names the line
// where the flat key is written, in the merged mapping.
func TestRuleConfig_OptionWrittenTwiceThroughAMerge(t *testing.T) {
	t.Parallel()

	_, err := config.FromYAML([]byte(
		"base: &base\n  max: 100\nrules:\n  MD013:\n    <<: *base\n    options:\n      max: 90\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `line 2: "max" and "options.max" both set option "max"; keep one of them`)
}

// yamlErrors starts the message of a *yaml.TypeError, before its errors.
const yamlErrors = "yaml: unmarshal errors:\n  "

// repeatedKeyCase is a key written twice in one mapping: literal writes it
// out twice, and alias writes it the second time, or the first, through an
// alias, on the same lines, so both must fail with want, YAML's error for
// a key written twice.
type repeatedKeyCase struct {
	name, literal, alias, want string
}

// repeatedRuleSettingKeys are keys written twice in the settings of a rule
// or under its options, which decoding a rule's settings rejects.
var repeatedRuleSettingKeys = []repeatedKeyCase{
	{
		name:    "flat option",
		literal: "rules:\n  MD013:\n    line_length: 100\n    line_length: 20\n",
		alias:   "rules:\n  MD013:\n    &opt line_length: 100\n    *opt : 20\n",
		want:    `line 4: mapping key "line_length" already defined at line 3`,
	},
	{
		name:    "alias first",
		literal: "ignore: [line_length]\nrules:\n  MD013:\n    line_length: 20\n    line_length: 100\n",
		alias:   "ignore: [&opt line_length]\nrules:\n  MD013:\n    *opt : 20\n    line_length: 100\n",
		want:    `line 5: mapping key "line_length" already defined at line 4`,
	},
	{
		name:    "option under options",
		literal: "rules:\n  MD013:\n    options:\n      max: 100\n      max: 20\n",
		alias:   "rules:\n  MD013:\n    options:\n      &opt max: 100\n      *opt : 20\n",
		want:    `line 5: mapping key "max" already defined at line 4`,
	},
	{
		name:    "option in a merged mapping",
		literal: "base: &base\n  max: 100\n  max: 20\nrules:\n  MD013:\n    <<: *base\n",
		alias:   "base: &base\n  &opt max: 100\n  *opt : 20\nrules:\n  MD013:\n    <<: *base\n",
		want:    `line 3: mapping key "max" already defined at line 2`,
	},
	{
		name:    "severity",
		literal: "rules:\n  MD013:\n    severity: error\n    severity: warning\n",
		alias:   "rules:\n  MD013:\n    &set severity: error\n    *set : warning\n",
		want:    `line 4: mapping key "severity" already defined at line 3`,
	},
	{
		name:    "enabled",
		literal: "rules:\n  MD013:\n    enabled: false\n    enabled: true\n",
		alias:   "rules:\n  MD013:\n    &set enabled: false\n    *set : true\n",
		want:    `line 4: mapping key "enabled" already defined at line 3`,
	},
	{
		name:    "options",
		literal: "rules:\n  MD013:\n    options: {max: 1}\n    options: {max: 2}\n",
		alias:   "rules:\n  MD013:\n    &set options: {max: 1}\n    *set : {max: 2}\n",
		want:    `line 4: mapping key "options" already defined at line 3`,
	},
	{
		name:    "setting in rule_defaults",
		literal: "rule_defaults:\n  MD013:\n    auto_fix: false\n    auto_fix: true\n",
		alias:   "rule_defaults:\n  MD013:\n    &set auto_fix: false\n    *set : true\n",
		want:    `line 4: mapping key "auto_fix" already defined at line 3`,
	},
	{
		// YAML names every pair of keys that it takes for one.
		name:    "three times",
		literal: "rules:\n  MD013:\n    max: 1\n    max: 2\n    max: 3\n",
		alias:   "rules:\n  MD013:\n    &opt max: 1\n    *opt : 2\n    *opt : 3\n",
		want: `line 4: mapping key "max" already defined at line 3` + "\n  " +
			`line 5: mapping key "max" already defined at line 3` + "\n  " +
			`line 5: mapping key "max" already defined at line 4`,
	},
	{
		// YAML lists the pairs by the earlier key.
		name:    "two keys in turn",
		literal: "rules:\n  MD013:\n    max: 1\n    tables: true\n    tables: false\n    max: 2\n",
		alias:   "rules:\n  MD013:\n    &a max: 1\n    &b tables: true\n    *b : false\n    *a : 2\n",
		want: `line 6: mapping key "max" already defined at line 3` + "\n  " +
			`line 5: mapping key "tables" already defined at line 4`,
	},
	{
		// YAML rejects the settings, which write options out twice, before
		// it reaches the first options mapping.
		name:    "inside a key written out twice",
		literal: "rules:\n  MD013:\n    options:\n      max: 1\n      max: 2\n    options:\n      max: 3\n",
		alias:   "rules:\n  MD013:\n    options:\n      &opt max: 1\n      *opt : 2\n    options:\n      max: 3\n",
		want:    `line 6: mapping key "options" already defined at line 3`,
	},
	{
		// YAML rejects the settings before it reaches the mapping that
		// they merge.
		name:    "settings before the mappings they merge",
		literal: "base: &base\n  max: 1\n  max: 2\nrules:\n  MD013:\n    <<: *base\n    tables: true\n    tables: false\n",
		alias:   "base: &base\n  max: 1\n  max: 2\nrules:\n  MD013:\n    <<: *base\n    &t tables: true\n    *t : false\n",
		want:    `line 8: mapping key "tables" already defined at line 7`,
	},
	{
		// YAML reports a key that repeats under options beside the other
		// errors of the rule's settings.
		name:    "beside another error of the rule",
		literal: "rules:\n  MD013:\n    enabled: maybe\n    options:\n      max: 1\n      max: 2\n",
		alias:   "rules:\n  MD013:\n    enabled: maybe\n    options:\n      &opt max: 1\n      *opt : 2\n",
		want: "line 3: cannot unmarshal !!str `maybe` into bool\n  " +
			`line 6: mapping key "max" already defined at line 5`,
	},
	{
		// YAML reaches the options where the settings write them, before
		// the settings that follow.
		name:    "under options before another error of the rule",
		literal: "rules:\n  MD013:\n    options:\n      max: 1\n      max: 2\n    enabled: maybe\n",
		alias:   "rules:\n  MD013:\n    options:\n      &opt max: 1\n      *opt : 2\n    enabled: maybe\n",
		want: `line 5: mapping key "max" already defined at line 4` + "\n  " +
			"line 6: cannot unmarshal !!str `maybe` into bool",
	},
	{
		// YAML decodes nothing below options that repeat a key.
		name:    "under options above a value that repeats a key",
		literal: "rules:\n  MD013:\n    options:\n      max: 1\n      max: 2\n      nested: {x: 1, x: 2}\n",
		alias:   "rules:\n  MD013:\n    options:\n      &opt max: 1\n      *opt : 2\n      nested: {x: 1, x: 2}\n",
		want:    `line 5: mapping key "max" already defined at line 4`,
	},
	{
		// YAML decodes the later merged mapping too, and lists its errors.
		name:    "in a merged mapping before another merged mapping",
		literal: "a: &a {enabled: true, enabled: false}\nb: &b {enabled: maybe}\nrules:\n  MD013:\n    <<: [*a, *b]\n",
		alias:   "a: &a {&k enabled: true, *k : false}\nb: &b {enabled: maybe}\nrules:\n  MD013:\n    <<: [*a, *b]\n",
		want: `line 1: mapping key "enabled" already defined at line 1` + "\n  " +
			"line 2: cannot unmarshal !!str `maybe` into bool",
	},
	{
		// Two aliases of one anchor name the key, not the anchor.
		name:    "two aliases of one anchor in a merged mapping",
		literal: "x: [max]\na: &a {max: 1, max: 2}\nrules:\n  MD013:\n    <<: *a\n",
		alias:   "x: [&k max]\na: &a {*k : 1, *k : 2}\nrules:\n  MD013:\n    <<: *a\n",
		want:    `line 2: mapping key "max" already defined at line 2`,
	},
	{
		name:    "beside a key written out twice in a merged mapping",
		literal: "a: &a {max: 1, max: 2, tables: 1, tables: 2}\nrules:\n  MD013:\n    <<: *a\n",
		alias:   "a: &a {&k max: 1, *k : 2, tables: 1, tables: 2}\nrules:\n  MD013:\n    <<: *a\n",
		want: `line 1: mapping key "max" already defined at line 1` + "\n  " +
			`line 1: mapping key "tables" already defined at line 1`,
	},
	{
		// YAML decodes the options and the other mappings that they merge.
		name:    "in a mapping that the options merge",
		literal: "a: &a {max: 1, max: 2}\nb: &b\n  ? [x]\n  : 1\nrules:\n  MD013:\n    options:\n      <<: [*a, *b]\n",
		alias:   "a: &a {&k max: 1, *k : 2}\nb: &b\n  ? [x]\n  : 1\nrules:\n  MD013:\n    options:\n      <<: [*a, *b]\n",
		want: `line 1: mapping key "max" already defined at line 1` + "\n  " +
			"line 3: cannot unmarshal !!seq into string",
	},
	{
		name:    "in an option value",
		literal: "rules:\n  MD013:\n    options:\n      x: {a: 1, a: 2}\n",
		alias:   "rules:\n  MD013:\n    options:\n      x: {&k a: 1, *k : 2}\n",
		want:    `line 4: mapping key "a" already defined at line 4`,
	},
	{
		// YAML reaches a merged mapping after the settings' own keys.
		name:    "in a merged mapping beside another error",
		literal: "rules:\n  MD013:\n    <<: {max: 1, max: 2}\n    enabled: maybe\n",
		alias:   "rules:\n  MD013:\n    <<: {&opt max: 1, *opt : 2}\n    enabled: maybe\n",
		want: "line 4: cannot unmarshal !!str `maybe` into bool\n  " +
			`line 3: mapping key "max" already defined at line 3`,
	},
}

// TestRuleConfig_RejectsAKeyWrittenTwiceThroughAnAlias covers two keys in
// one mapping of a rule's settings that read as one name, where an alias
// writes one of them. YAML rejects a key written twice, but compares an
// alias key by its anchor, so decoding would keep one of the values
// silently, or, for a setting, fail with an error that names a type of
// gomdlint's. Decoding fails instead with YAML's error for a key written
// twice, at the line of the later key, the same error as for the key
// written out twice, for settings, for options directly under the rule key
// and for options under options, with yaml.Unmarshal and with FromYAML.
func TestRuleConfig_RejectsAKeyWrittenTwiceThroughAnAlias(t *testing.T) {
	t.Parallel()

	for _, tt := range repeatedRuleSettingKeys {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			literalErr := decodeExpanded(t, tt.literal)
			aliasErr := decodeExpanded(t, tt.alias)

			require.Error(t, literalErr)
			require.Error(t, aliasErr)
			assert.Equal(t, yamlErrors+tt.want, literalErr.Error())
			assert.Equal(t, literalErr.Error(), aliasErr.Error())

			_, literalErr = config.FromYAML([]byte(tt.literal))
			_, aliasErr = config.FromYAML([]byte(tt.alias))
			require.Error(t, literalErr)
			require.Error(t, aliasErr)
			assert.Equal(t, "parse yaml: "+yamlErrors+tt.want, literalErr.Error())
			assert.Equal(t, literalErr.Error(), aliasErr.Error())
		})
	}
}

// decodeExpanded decodes content into a Config with its alias keys
// expanded, as the configuration loader decodes a file.
func decodeExpanded(t *testing.T, content string) error {
	t.Helper()

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(content), &doc))
	var cfg config.Config
	return config.ExpandAliasKeys(&doc).Decode(&cfg)
}

// TestRuleConfig_ReportsTheErrorsOfEveryRule covers keys written twice in
// the settings of two rules: decoding reports both, each with its line.
func TestRuleConfig_ReportsTheErrorsOfEveryRule(t *testing.T) {
	t.Parallel()

	_, err := config.FromYAML([]byte(`rules:
  MD013:
    &a line_length: 1
    *a : 2
  MD009:
    options:
      &b br_spaces: 1
      *b : 2
`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `line 4: mapping key "line_length" already defined at line 3`)
	assert.Contains(t, err.Error(), `line 8: mapping key "br_spaces" already defined at line 7`)
}

// repeatedRuleKeys are rule keys written twice under rules or
// rule_defaults, which FromYAML rejects (see config.ExpandAliasKeys).
var repeatedRuleKeys = []repeatedKeyCase{
	{
		name:    "under rules",
		literal: "rules:\n  MD013: false\n  MD013: true\n",
		alias:   "rules:\n  &id MD013: false\n  *id : true\n",
		want:    `line 3: mapping key "MD013" already defined at line 2`,
	},
	{
		name:    "under rule_defaults",
		literal: "rule_defaults:\n  MD013: false\n  MD013: true\n",
		alias:   "rule_defaults:\n  &id MD013: false\n  *id : true\n",
		want:    `line 3: mapping key "MD013" already defined at line 2`,
	},
	{
		name:    "alias first",
		literal: "ignore: [MD013]\nrules:\n  MD013: false\n  MD013: true\n",
		alias:   "ignore: [&id MD013]\nrules:\n  *id : false\n  MD013: true\n",
		want:    `line 4: mapping key "MD013" already defined at line 3`,
	},
	{
		name:    "two anchors of one name",
		literal: "ignore: [MD013, MD013]\nrules:\n  MD013: false\n  MD013: true\n",
		alias:   "ignore: [&a MD013, &b MD013]\nrules:\n  *a : false\n  *b : true\n",
		want:    `line 4: mapping key "MD013" already defined at line 3`,
	},
	{
		name:    "settings of both kinds",
		literal: "rules:\n  MD013:\n    enabled: false\n  MD013:\n    severity: error\n",
		alias:   "rules:\n  &id MD013:\n    enabled: false\n  *id :\n    severity: error\n",
		want:    `line 4: mapping key "MD013" already defined at line 2`,
	},
	{
		name:    "in a merged mapping",
		literal: "base: &base\n  MD013: false\n  MD013: true\nrules:\n  <<: *base\n",
		alias:   "base: &base\n  &id MD013: false\n  *id : true\nrules:\n  <<: *base\n",
		want:    `line 3: mapping key "MD013" already defined at line 2`,
	},
	{
		name:    "three times",
		literal: "rules:\n  MD013: false\n  MD013: true\n  MD013: false\n",
		alias:   "rules:\n  &id MD013: false\n  *id : true\n  *id : false\n",
		want: `line 3: mapping key "MD013" already defined at line 2` + "\n  " +
			`line 4: mapping key "MD013" already defined at line 2` + "\n  " +
			`line 4: mapping key "MD013" already defined at line 3`,
	},
	{
		name:    "three times beside a merge key",
		literal: "base: &base\n  MD010: false\nrules:\n  <<: *base\n  MD013: false\n  MD013: true\n  MD013: false\n",
		alias:   "base: &base\n  MD010: false\nrules:\n  <<: *base\n  &id MD013: false\n  *id : true\n  *id : false\n",
		want: `line 6: mapping key "MD013" already defined at line 5` + "\n  " +
			`line 7: mapping key "MD013" already defined at line 5` + "\n  " +
			`line 7: mapping key "MD013" already defined at line 6`,
	},
	{
		// Decoding reaches the sections in the order the file writes them.
		name:    "rule_defaults before rules",
		literal: "rule_defaults:\n  MD009: false\n  MD009: true\nrules:\n  MD013: false\n  MD013: true\n",
		alias:   "rule_defaults:\n  &a MD009: false\n  *a : true\nrules:\n  &b MD013: false\n  *b : true\n",
		want: `line 3: mapping key "MD009" already defined at line 2` + "\n  " +
			`line 6: mapping key "MD013" already defined at line 5`,
	},
	{
		// YAML rejects the document, which writes rules out twice, before
		// it reaches the first rules mapping.
		name:    "inside rules written out twice",
		literal: "rules:\n  MD013: false\n  MD013: true\nrules:\n  MD009: false\n",
		alias:   "rules:\n  &id MD013: false\n  *id : true\nrules:\n  MD009: false\n",
		want:    `line 4: mapping key "rules" already defined at line 1`,
	},
	{
		// YAML compares keys by their text, so a null key repeats too,
		// though decoding skips it.
		name:    "null",
		literal: "rules:\n  ~: false\n  ~: true\n",
		alias:   "rules:\n  &n ~: false\n  *n : true\n",
		want:    `line 3: mapping key "~" already defined at line 2`,
	},
	{
		// YAML names a binary key by its text.
		name:    "binary",
		literal: "rules:\n  !!binary TUQwMTM=: false\n  !!binary TUQwMTM=: true\n",
		alias:   "rules:\n  &b !!binary TUQwMTM= : false\n  *b : true\n",
		want:    `line 3: mapping key "TUQwMTM=" already defined at line 2`,
	},
	{
		// Decoding merges the sections in the order the merged mapping
		// writes them.
		name:    "sections that only a merge key brings in",
		literal: "base: &base\n  rule_defaults: {MD009: false, MD009: true}\n  rules: {MD013: false, MD013: true}\n<<: *base\n",
		alias:   "base: &base\n  rule_defaults: {&a MD009: false, *a : true}\n  rules: {&b MD013: false, *b : true}\n<<: *base\n",
		want: `line 2: mapping key "MD009" already defined at line 2` + "\n  " +
			`line 3: mapping key "MD013" already defined at line 3`,
	},
	{
		// YAML rejects the rules before it reaches the mapping that they
		// merge.
		name:    "rules before the mappings they merge",
		literal: "base: &base\n  MD009: false\n  MD009: true\nrules:\n  <<: *base\n  MD013: false\n  MD013: true\n",
		alias:   "base: &base\n  MD009: false\n  MD009: true\nrules:\n  <<: *base\n  &id MD013: false\n  *id : true\n",
		want:    `line 7: mapping key "MD013" already defined at line 6`,
	},
}

// TestFromYAML_RejectsARuleKeyWrittenTwiceThroughAnAlias covers two rule
// keys under rules or rule_defaults that read as one key, where an alias
// writes one of them. Decoding the rules into a map would keep one of the
// values silently, so FromYAML fails with the error that a key written
// out twice gets.
func TestFromYAML_RejectsARuleKeyWrittenTwiceThroughAnAlias(t *testing.T) {
	t.Parallel()

	for _, tt := range repeatedRuleKeys {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, literalErr := config.FromYAML([]byte(tt.literal))
			_, aliasErr := config.FromYAML([]byte(tt.alias))

			require.Error(t, literalErr)
			require.Error(t, aliasErr)
			assert.Equal(t, "parse yaml: "+yamlErrors+tt.want, literalErr.Error())
			assert.Equal(t, literalErr.Error(), aliasErr.Error())
		})
	}
}

// TestFromYAML_RejectsATopLevelKeyRepeatedThroughAnAlias covers top-level
// keys that an alias repeats: a key that gomdlint does not read, which
// decoding would skip silently, and rules, which decoding would report as
// a field set twice. Each gets the error that its literal twin gets, which
// YAML gives before it reaches the rules, so the rule key that an alias
// also repeats in the first document goes unreported, as in its twin.
func TestFromYAML_RejectsATopLevelKeyRepeatedThroughAnAlias(t *testing.T) {
	t.Parallel()

	for _, tt := range []repeatedKeyCase{
		{
			name:    "a key that gomdlint does not read",
			literal: "ignore: [note]\nnote: 1\nnote: 2\nrules:\n  MD013: false\n  MD013: true\n",
			alias:   "ignore: [&k note]\nnote: 1\n*k : 2\nrules:\n  &id MD013: false\n  *id : true\n",
			want:    `line 3: mapping key "note" already defined at line 2`,
		},
		{
			name:    "rules",
			literal: "rules:\n  MD013: false\nrules:\n  MD009: false\n",
			alias:   "&r rules:\n  MD013: false\n*r :\n  MD009: false\n",
			want:    `line 3: mapping key "rules" already defined at line 1`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, literalErr := config.FromYAML([]byte(tt.literal))
			_, aliasErr := config.FromYAML([]byte(tt.alias))

			require.Error(t, literalErr)
			require.Error(t, aliasErr)
			assert.Equal(t, "parse yaml: "+yamlErrors+tt.want, literalErr.Error())
			assert.Equal(t, literalErr.Error(), aliasErr.Error())
		})
	}
}

// TestFromYAML_KeepsTheErrorsOfADocumentWithoutAliasKeys covers documents
// that write no alias key, beside another error: one that repeats no key
// but holds a key that decoding rejects, and mappings that YAML rejects for
// a key written out twice, though a merge key or a key of another type
// keeps them from being mappings of string keys. Such a document is its
// own literal twin, so the checks leave it to decoding, which reports
// every error, as it did before the checks.
func TestFromYAML_KeepsTheErrorsOfADocumentWithoutAliasKeys(t *testing.T) {
	t.Parallel()

	const ignoreError = "line %d: cannot unmarshal !!str `vendor/**` into []string"
	for name, tt := range map[string]struct{ content, want string }{
		"no repeat under a rule": {
			content: "rules:\n  MD013:\n    ? [a]\n    : 1\n    enabled: maybe\n",
			want:    "line 3: cannot unmarshal !!seq into string\n  line 5: cannot unmarshal !!str `maybe` into bool",
		},
		"no repeat at the top": {
			content: "? [a]\n: 1\nflavor: [x]\n",
			want:    "line 1: cannot unmarshal !!seq into string\n  line 3: cannot unmarshal !!seq into config.Flavor",
		},
		"a rule key beside a merge key": {
			content: "x-shared: &shared\n  MD009: false\nrules:\n  <<: *shared\n" +
				"  MD013: {line_length: 120}\n  MD013: {line_length: 100}\nignore: vendor/**\n",
			want: `line 6: mapping key "MD013" already defined at line 5` + "\n  " + fmt.Sprintf(ignoreError, 7),
		},
		"a merge key": {
			content: "rules:\n  <<: {MD010: false}\n  <<: {MD009: false}\nignore: vendor/**\n",
			want:    `line 3: mapping key "<<" already defined at line 2` + "\n  " + fmt.Sprintf(ignoreError, 4),
		},
		"a number and a string": {
			content: "rules:\n  1: false\n  \"1\": true\nignore: vendor/**\n",
			want:    `line 3: mapping key "1" already defined at line 2` + "\n  " + fmt.Sprintf(ignoreError, 4),
		},
		"a binary option under the rule key": {
			content: "rules:\n  MD013:\n    max: 1\n    !!binary bWF4: 2\n",
			want:    `line 4: "max" and "bWF4" both set option "max"; keep one of them`,
		},
		"a binary option under options": {
			content: "rules:\n  MD013:\n    options:\n      max: 1\n      !!binary bWF4: 2\n",
			want:    `line 5: "options.max" and "options.bWF4" both set option "max"; keep one of them`,
		},
		"a binary option in a merged mapping": {
			content: "base: &b {max: 1, !!binary bWF4: 2}\nrules:\n  MD013:\n    <<: *b\n",
			want:    `line 1: "max" and "bWF4" both set option "max"; keep one of them`,
		},
		"a binary setting": {
			content: "rules:\n  MD013:\n    enabled: true\n    !!binary ZW5hYmxlZA==: false\n",
			want:    "line 4: field enabled already set in type config.settings",
		},
		"a sequence key in a merged mapping that merges in turn": {
			content: "c: &c {tables: true}\na: &a\n  <<: *c\n  ? [x]\n  : 1\nrules:\n  MD013:\n    <<: *a\n",
			want:    "line 4: cannot unmarshal !!seq into string",
		},
		"a setting beside a merge key": {
			content: "rules:\n  MD013:\n    <<: {tables: true}\n    max: 1\n    max: 2\nflavor: [x]\n",
			want:    `line 5: mapping key "max" already defined at line 4` + "\n  line 6: cannot unmarshal !!seq into config.Flavor",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.FromYAML([]byte(tt.content))

			require.Error(t, err)
			assert.Equal(t, "parse yaml: "+yamlErrors+tt.want, err.Error())
		})
	}
}

// TestFromYAML_LeavesKeysThatYAMLDoesNotCompareToDecoding covers a binary
// key and a string key that decode to one name. YAML compares keys by
// their text, so it takes them for two keys. Under rules, decoding keeps
// the later value, with or without an alias, and under a rule they are an
// error, both as before.
func TestFromYAML_LeavesKeysThatYAMLDoesNotCompareToDecoding(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"rules:\n  MD013: false\n  !!binary TUQwMTM=: true\n",
		"ignore: [&b !!binary TUQwMTM=]\nrules:\n  MD013: false\n  *b : true\n",
	} {
		cfg, err := config.FromYAML([]byte(content))
		require.NoError(t, err, content)
		require.NotNil(t, cfg.Rules["MD013"].Enabled, content)
		assert.True(t, *cfg.Rules["MD013"].Enabled, content)
	}

	// Under a rule, such keys are an error, as they were before. Read with
	// its alias keys expanded, the alias key is named as it is written out;
	// decoded with yaml.Unmarshal alone, it keeps its old name.
	content := []byte("ignore: [&b !!binary bWF4]\nrules:\n  MD013:\n    max: 1\n    *b : 2\n")
	_, err := config.FromYAML(content)
	require.Error(t, err)
	assert.Equal(t, "parse yaml: "+yamlErrors+`line 5: "max" and "bWF4" both set option "max"; keep one of them`, err.Error())
	var decoded config.Config
	err = yaml.Unmarshal(content, &decoded)
	require.Error(t, err)
	assert.Equal(t, yamlErrors+`line 5: "max" and "*b" both set option "max"; keep one of them`, err.Error())
}

// TestFromYAML_LeavesAliasKeysOfCollectionsAsTheyWere covers alias keys
// that name a collection, which ExpandAliasKeys leaves as aliases. Each
// document gets base's error, among them the error of a collection that
// contains the alias itself, which decoding catches only through the
// alias. Expanded, such a key would make a cycle that decoding never
// leaves.
func TestFromYAML_LeavesAliasKeysOfCollectionsAsTheyWere(t *testing.T) {
	t.Parallel()

	const containsItself = "yaml: anchor 'm' value contains itself"
	for name, tt := range map[string]struct{ content, want string }{
		"under options": {
			content: "rules:\n  MD013:\n    options:\n      x: &m {*m : 1}\n",
			want:    containsItself,
		},
		"under the rule key": {
			content: "rules:\n  MD013:\n    x: &m {*m : 1}\n",
			want:    `rules.MD013: line 3: option "x": ` + containsItself,
		},
		"deeper": {
			content: "rules:\n  MD013:\n    options:\n      x: &m {k: {*m : 1}}\n",
			want:    containsItself,
		},
		"under rules beside a merge key": {
			content: "rules:\n  <<: {}\n  ? &m {*m : 1}\n  : false\n",
			want:    containsItself,
		},
		"at the top beside a merge key": {
			content: "<<: {}\n? &m {*m : 1}\n: x\n",
			want:    containsItself,
		},
		"entered through another alias": {
			content: "t: &t\n  p: &p\n    ? *t\n    : 1\nrules:\n  MD013:\n    options:\n      x: *p\n",
			want:    "yaml: anchor 't' value contains itself",
		},
		"a collection key": {
			content: "x: &c {a: 1, a: 2}\nrules:\n  MD013:\n    options:\n      ? *c\n      : 1\n",
			want:    yamlErrors + `line 1: mapping key "a" already defined at line 1`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := config.FromYAML([]byte(tt.content))

			require.Error(t, err)
			assert.Equal(t, "parse yaml: "+tt.want, err.Error())
		})
	}
}

// TestFromYAML_LeavesAnAliasOfAMergeKeyAnOrdinaryKey covers an alias key
// that names a merge key. Decoding reads an alias key as an ordinary key,
// so ExpandAliasKeys leaves it as an alias, and the document decodes as
// it did, where the key written out would merge.
func TestFromYAML_LeavesAnAliasOfAMergeKeyAnOrdinaryKey(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte("x: &m <<\nrules:\n  *m : {MD013: false}\n"))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"MD013": false}, cfg.Rules["<<"].Options)

	cfg, err = config.FromYAML([]byte("x: &m <<\nrules:\n  MD013:\n    options: {*m : {max: 5}}\n"))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"<<": map[string]any{"max": 5}}, cfg.Rules["MD013"].Options)
}

// TestFromYAML_LeavesAFailingMergeToDecoding covers documents in which an
// alias makes a key repeat, beside an error that ends decoding: a merge of
// a node that is no mapping, a mapping that merges itself, and a value
// that its tag rejects. The document gets that error alone, as its literal
// twin does. So does a document without aliases whose first error ends
// decoding before the rules, as before.
func TestFromYAML_LeavesAFailingMergeToDecoding(t *testing.T) {
	t.Parallel()

	// fromYAML starts FromYAML's message for a document it cannot parse.
	const fromYAML = "parse yaml: "
	for _, tt := range []struct{ literal, alias, want string }{
		{
			literal: "a: &a {MD013: false, MD013: true}\nrules:\n  <<: [*a, 5]\n",
			alias:   "a: &a {&id MD013: false, *id : true}\nrules:\n  <<: [*a, 5]\n",
			want:    "yaml: map merge requires map or sequence of maps as the value",
		},
		{
			literal: "a: &a {MD013: false, MD013: true}\nrules: &r\n  <<: [*a, *r]\n",
			alias:   "a: &a {&id MD013: false, *id : true}\nrules: &r\n  <<: [*a, *r]\n",
			want:    "yaml: anchor 'r' value contains itself",
		},
		{
			literal: "rules:\n  MD013: false\n  MD013: true\n<<: 5\n",
			alias:   "rules:\n  &id MD013: false\n  *id : true\n<<: 5\n",
			want:    "yaml: map merge requires map or sequence of maps as the value",
		},
		{
			literal: "a: &a {enabled: true, enabled: false}\nb: &b {enabled: !!int abc}\nrules:\n  MD013:\n    <<: [*a, *b]\n",
			alias:   "a: &a {&k enabled: true, *k : false}\nb: &b {enabled: !!int abc}\nrules:\n  MD013:\n    <<: [*a, *b]\n",
			want:    "yaml: cannot decode !!str `abc` as a !!int",
		},
		{
			literal: "a: &a {max: 1, max: 2}\nrules:\n  MD013:\n    options:\n      <<: *a\n      bad: !!int abc\n",
			alias:   "a: &a {&k max: 1, *k : 2}\nrules:\n  MD013:\n    options:\n      <<: *a\n      bad: !!int abc\n",
			want:    "yaml: cannot decode !!str `abc` as a !!int",
		},
		{
			literal: "flavor: !!int abc\nrules:\n  <<: {MD013: false}\n  ? [x]\n  : 1\n",
			alias:   "flavor: !!int abc\nrules:\n  <<: {MD013: false}\n  ? [x]\n  : 1\n",
			want:    "yaml: cannot decode !!str `abc` as a !!int",
		},
	} {
		_, literalErr := config.FromYAML([]byte(tt.literal))
		_, aliasErr := config.FromYAML([]byte(tt.alias))

		require.Error(t, literalErr)
		require.Error(t, aliasErr)
		assert.Equal(t, fromYAML+tt.want, literalErr.Error())
		assert.Equal(t, literalErr.Error(), aliasErr.Error())
	}
}

// TestFromYAML_RejectsAKeyRepeatedInAFlatOptionValue covers the value of an
// option written directly under the rule key, which ReadFlatOptions
// decodes: a key that an alias repeats in it gets the error that the key
// written out twice gets.
func TestFromYAML_RejectsAKeyRepeatedInAFlatOptionValue(t *testing.T) {
	t.Parallel()

	_, literalErr := config.FromYAML([]byte("rules:\n  MD013:\n    x: {a: 1, a: 2}\n"))
	_, aliasErr := config.FromYAML([]byte("rules:\n  MD013:\n    x: {&k a: 1, *k : 2}\n"))

	require.Error(t, literalErr)
	require.Error(t, aliasErr)
	assert.Equal(t, `parse yaml: rules.MD013: line 3: option "x": `+yamlErrors+`line 3: mapping key "a" already defined at line 3`,
		literalErr.Error())
	assert.Equal(t, literalErr.Error(), aliasErr.Error())
}

// TestFromYAML_MergedRuleKeysYieldToWrittenOnes covers YAML's precedence
// for merge keys under rules, which a rule key brought in twice follows
// without an error: a rule key written in the mapping replaces a merged
// one, as under a rule.
func TestFromYAML_MergedRuleKeysYieldToWrittenOnes(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte("base: &base\n  MD013: false\n  MD009: false\nrules:\n  <<: *base\n  MD013: true\n"))
	require.NoError(t, err)

	require.NotNil(t, cfg.Rules["MD013"].Enabled)
	assert.True(t, *cfg.Rules["MD013"].Enabled)
	require.NotNil(t, cfg.Rules["MD009"].Enabled)
	assert.False(t, *cfg.Rules["MD009"].Enabled)
}

// TestExpandAliasKeys covers decoding a node with its alias keys
// expanded. A key that an alias repeats gets YAML's error for the key
// written out twice, beside the document's other errors and in YAML's
// order, as the literal twin gets them. A document with alias keys that
// repeat nothing is copied and decodes as the document does, as a map and
// as a configuration: beside merge keys, merge sequences, shared anchors,
// an alias of a merge key and a tagged key, and in a node that contains
// itself, which fails alike. Two aliases of one anchor name, which yaml.v3
// compares by that name, decode when they name two keys, as the twin does.
func TestExpandAliasKeys(t *testing.T) {
	t.Parallel()

	parse := func(content string) *yaml.Node {
		t.Helper()

		var doc yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(content), &doc))
		return &doc
	}

	var shape map[string]map[string]any
	err := config.ExpandAliasKeys(parse("a: {x: 1, x: 2}\nb: {&k y: 1, *k : 2, z: [1]}\n")).Decode(&shape)
	var typeErr *yaml.TypeError
	require.ErrorAs(t, err, &typeErr)
	assert.Equal(t, []string{
		`line 1: mapping key "x" already defined at line 1`,
		`line 2: mapping key "y" already defined at line 2`,
	}, typeErr.Errors)

	copied := "ignore: [&k MD013, &j tables, &n !!str 10, &m <<]\n" +
		"rules:\n  *k : {line_length: 80, *j : false}\n" +
		"base: &b {x: 1, *j : 2}\nother: {<<: *b, y: *b}\n" +
		"seq: {<<: [*b, {z: 3}], w: [*b, *b]}\n" +
		"keys: {*n : ten, *m : {a: 1}}\n"
	doc := parse(copied)
	expanded := config.ExpandAliasKeys(doc)
	require.NotSame(t, doc, expanded)
	var decoded, plain map[string]any
	require.NoError(t, expanded.Decode(&decoded))
	require.NoError(t, doc.Decode(&plain))
	assert.Equal(t, plain, decoded)
	assert.Equal(t, map[string]any{"10": "ten", "<<": map[string]any{"a": 1}}, decoded["keys"],
		"a tagged key keeps its tag, and an alias of a merge key stays an ordinary key")
	var decodedCfg, plainCfg config.Config
	require.NoError(t, expanded.Decode(&decodedCfg))
	require.NoError(t, doc.Decode(&plainCfg))
	assert.Equal(t, plainCfg, decodedCfg)

	self := parse("k: &k key\nself: &s {*k : *s}\n")
	selfExpanded := config.ExpandAliasKeys(self)
	require.NotSame(t, self, selfExpanded)
	var fromCopy, fromDoc any
	docErr := self.Decode(&fromDoc)
	require.Error(t, docErr)
	require.EqualError(t, selfExpanded.Decode(&fromCopy), docErr.Error(), "a node that contains itself fails alike")

	var anchors map[string]any
	require.NoError(t, config.ExpandAliasKeys(parse("x: [&a max]\nm:\n  *a : 1\n  y: [&a tables]\n  *a : true\n")).Decode(&anchors))
	assert.Equal(t, map[string]any{"max": 1, "y": []any{"tables"}, "tables": true}, anchors["m"])
}

// TestRuleConfig_MergedKeysYieldToWrittenKeys covers YAML's precedence for
// merge keys, which a key brought in twice follows without an error: a key
// written in the mapping, even as an alias, replaces a merged one, and an
// earlier merged mapping replaces a later one.
func TestRuleConfig_MergedKeysYieldToWrittenKeys(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte(`ignore: [&length line_length]
base: &base
  line_length: 100
  tables: false
other: &other
  line_length: 60
  headings: false
rules:
  MD013:
    <<: *base
    line_length: 80
  MD010:
    <<: *base
    *length : 70
  MD009:
    <<: [*base, *other]
    options:
      max: 1
`))
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"line_length": 80, "tables": false}, cfg.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"line_length": 70, "tables": false}, cfg.Rules["MD010"].Options)
	assert.Equal(t, map[string]any{"line_length": 100, "tables": false, "headings": false, "max": 1},
		cfg.Rules["MD009"].Options)
}

// TestReadFlatOptions_ReadsOnlyAcceptedKeys covers decoding, which reads
// the settings and the options under options and leaves the keys written
// directly under the rule key alone, and ReadFlatOptions, which reads the
// keys that it is told the rule reads from the document, under rules and
// rule_defaults. A key that it is not told to read keeps its value
// unread, so a value that cannot be decoded, here one that holds the
// rule's own settings, does no harm.
func TestReadFlatOptions_ReadsOnlyAcceptedKeys(t *testing.T) {
	t.Parallel()

	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`rules:
  MD013: &length
    enabled: false
    options: {max: 100}
    metadata: *length
    line_length: 90
rule_defaults:
  MD009:
    br_spaces: 3
    note: 1
`), &doc))
	var cfg config.Config
	require.NoError(t, doc.Decode(&cfg))

	rule := cfg.Rules["MD013"]
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	assert.Equal(t, map[string]any{"max": 100}, rule.Options, "decoding leaves the flat keys alone")
	assert.Nil(t, cfg.RuleDefaults["MD009"].Options)

	var asked []string
	require.NoError(t, config.ReadFlatOptions(&doc, &cfg, func(rule, option string) bool {
		asked = append(asked, rule+"."+option)
		return option == "line_length" || option == "br_spaces"
	}))

	assert.ElementsMatch(t, []string{"MD013.line_length", "MD013.metadata", "MD009.br_spaces", "MD009.note"}, asked)
	assert.Equal(t, map[string]any{"max": 100, "line_length": 90}, cfg.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"br_spaces": 3}, cfg.RuleDefaults["MD009"].Options)
}

// TestReadFlatOptions_ReadsTheDocumentThatCfgWasDecodedFrom covers a caller
// that decodes a configuration from a parsed document without expanding
// its alias keys, then reads the flat options from the same document, as
// ReadFlatOptions describes. A rule key that an alias repeats decodes as
// yaml.v3 decodes it, to the later value, and the flat options come from
// that value alone.
func TestReadFlatOptions_ReadsTheDocumentThatCfgWasDecodedFrom(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, content string
		want          map[string]any
	}{
		{
			name:    "other options in each value",
			content: "rules:\n  &id MD013:\n    line_length: 100\n  *id :\n    tables: false\n",
			want:    map[string]any{"tables": false},
		},
		{
			name:    "one option in both values",
			content: "rules:\n  &id MD013:\n    line_length: 100\n  *id :\n    line_length: 90\n",
			want:    map[string]any{"line_length": 90},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var doc yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tt.content), &doc))
			var cfg config.Config
			require.NoError(t, doc.Decode(&cfg))
			require.NoError(t, config.ReadFlatOptions(&doc, &cfg, func(string, string) bool { return true }))
			assert.Equal(t, tt.want, cfg.Rules["MD013"].Options)
		})
	}
}

// TestRuleConfig_CloneKeepsEverythingDecoded covers Clone of a decoded
// configuration, under rules and rule_defaults, whether decoding read the
// flat options, as FromYAML does, or left them to ReadFlatOptions, as
// decoding with yaml.Unmarshal does: the copy holds exactly what the
// configuration holds, so nothing is waiting to be read that a copy could
// lose.
func TestRuleConfig_CloneKeepsEverythingDecoded(t *testing.T) {
	t.Parallel()

	content := []byte(`rules:
  MD013:
    severity: error
    line_length: 100
    options:
      tables: false
rule_defaults:
  MD009:
    br_spaces: 3
`)
	read, err := config.FromYAML(content)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"line_length": 100, "tables": false}, read.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"br_spaces": 3}, read.RuleDefaults["MD009"].Options)

	var decoded config.Config
	require.NoError(t, yaml.Unmarshal(content, &decoded))

	for name, cfg := range map[string]*config.Config{"FromYAML": read, "yaml.Unmarshal": &decoded} {
		clone := cfg.Clone()
		assert.Equal(t, settled(cfg.Rules), settled(clone.Rules), "%s: rules", name)
		assert.Equal(t, settled(cfg.RuleDefaults), settled(clone.RuleDefaults), "%s: rule_defaults", name)
	}
}

// settled returns a copy of rules in which empty options are nil: both set
// nothing, and Clone writes nil options as an empty mapping.
func settled(rules map[string]config.RuleConfig) map[string]config.RuleConfig {
	copied := make(map[string]config.RuleConfig, len(rules))
	for key, rule := range rules {
		if len(rule.Options) == 0 {
			rule.Options = nil
		}
		copied[key] = rule
	}
	return copied
}

// TestRuleConfig_FlatOptionsFollowYAMLKeySelection covers keys whose
// selection yaml.v3 decides in its own way: a numeric key that a written
// key and a merged key both spell, where yaml.v3 compares the merged key
// with the written one by type, and a null key, which decoding into a map
// of strings skips. An option written directly under the rule key is
// selected exactly as under options.
func TestRuleConfig_FlatOptionsFollowYAMLKeySelection(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte(`rules:
  MD013:
    <<: {'10': 80}
    10: 100
    ~: 5
  MD009:
    options:
      <<: {'10': 80}
      10: 100
      ~: 5
`))
	require.NoError(t, err)

	var selected map[string]any
	require.NoError(t, yaml.Unmarshal([]byte("<<: {'10': 80}\n10: 100\n~: 5\n"), &selected))
	assert.Equal(t, selected, cfg.Rules["MD009"].Options, "under options, yaml.v3 selects the keys")
	assert.Equal(t, selected, cfg.Rules["MD013"].Options, "directly under the rule key, the same keys")
}

// TestRuleConfig_FlatOptionsThroughMergesAndAliases covers flat options
// that a YAML merge key or an alias brings in, which decoding reads like
// options written in place.
func TestRuleConfig_FlatOptionsThroughMergesAndAliases(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte(`rule_defaults:
  MD013: &length
    line_length: 100
rules:
  MD013:
    <<: *length
    tables: false
  MD010: *length
`))
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"line_length": 100, "tables": false}, cfg.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"line_length": 100}, cfg.Rules["MD010"].Options)
	assert.Equal(t, map[string]any{"line_length": 100}, cfg.RuleDefaults["MD013"].Options)
}

// TestRuleConfig_FlatOptionKeysAsDecodingReadsThem covers keys whose name
// only decoding tells: an alias key reads as the scalar it names, and a
// quoted key as its text, so either may be a setting or an option.
func TestRuleConfig_FlatOptionKeysAsDecodingReadsThem(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte(`ignore: [&length line_length, &level severity]
rules:
  MD013:
    *length : 100
    *level : error
    "tables": false
    "enabled": false
`))
	require.NoError(t, err)

	rule := cfg.Rules["MD013"]
	assert.Equal(t, map[string]any{"line_length": 100, "tables": false}, rule.Options)
	require.NotNil(t, rule.Severity)
	assert.Equal(t, "error", *rule.Severity)
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
}

// TestRuleConfig_FlatOptionsWriteUnderOptions covers writing a
// configuration whose options were read flat: they go under options, the
// shape gomdlint writes, and read back the same, as Clone relies on.
func TestRuleConfig_FlatOptionsWriteUnderOptions(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte("rules:\n  MD013:\n    severity: error\n    line_length: 100\n"))
	require.NoError(t, err)

	written, err := cfg.ToYAML()
	require.NoError(t, err)
	var shape struct {
		Rules map[string]map[string]any `yaml:"rules"`
	}
	require.NoError(t, yaml.Unmarshal(written, &shape))
	assert.Equal(t, map[string]any{"line_length": 100}, shape.Rules["MD013"]["options"], string(written))
	assert.NotContains(t, shape.Rules["MD013"], "line_length", string(written))

	clone := cfg.Clone()
	assert.Equal(t, map[string]any{"line_length": 100}, clone.Rules["MD013"].Options)
	require.NotNil(t, clone.Rules["MD013"].Severity)
	assert.Equal(t, "error", *clone.Rules["MD013"].Severity)
}
