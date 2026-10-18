package config_test

import (
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
			assert.Equal(t, config.FlatOptionConflictError{Name: "line_length", Line: 3}, *conflict)
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

// TestRuleConfig_RejectsAKeyWrittenTwiceThroughAnAlias covers two keys in
// one mapping that read as one name, where one of them is an alias of the
// other. YAML rejects a key written twice, but compares an alias key by its
// anchor, so decoding would keep the later value silently. Decoding fails
// instead, at the line of the later key, naming both keys as written, both
// for keys directly under the rule key and for keys under options.
func TestRuleConfig_RejectsAKeyWrittenTwiceThroughAnAlias(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, content, want string
	}{
		{
			name:    "flat",
			content: "rules:\n  MD013:\n    &opt line_length: 100\n    *opt : 20\n",
			want:    `line 4: "line_length" and "*opt" both set option "line_length"; keep one of them`,
		},
		{
			name:    "alias first",
			content: "ignore: [&opt line_length]\nrules:\n  MD013:\n    *opt : 20\n    line_length: 100\n",
			want:    `line 5: "*opt" and "line_length" both set option "line_length"; keep one of them`,
		},
		{
			name:    "under options",
			content: "rules:\n  MD013:\n    options:\n      &opt max: 100\n      *opt : 20\n",
			want:    `line 5: "options.max" and "options.*opt" both set option "max"; keep one of them`,
		},
		{
			name:    "in a merged mapping",
			content: "base: &base\n  &opt max: 100\n  *opt : 20\nrules:\n  MD013:\n    <<: *base\n",
			want:    `line 3: "max" and "*opt" both set option "max"; keep one of them`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg config.Config
			err := yaml.Unmarshal([]byte(tt.content), &cfg)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
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
	assert.Contains(t, err.Error(), `line 4: "line_length" and "*a" both set option "line_length"`)
	assert.Contains(t, err.Error(), `line 8: "options.br_spaces" and "options.*b" both set option "br_spaces"`)
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

// TestRuleConfig_FlatOptionsStayUnreadUntilResolved covers decoding, which
// keeps the options written directly under the rule key unread, so that
// only the options a rule declares need decoding. A flat key whose value
// cannot be decoded, here one that holds the rule's own settings, decodes,
// and ResolveFlatOptions drops it without reading it, while it reads the
// options that it is told the rule reads. FromYAML, which does not know
// the rules, reads every flat key as an option.
func TestRuleConfig_FlatOptionsStayUnreadUntilResolved(t *testing.T) {
	t.Parallel()

	var cfg config.Config
	require.NoError(t, yaml.Unmarshal([]byte(`rules:
  MD013: &length
    enabled: false
    options: {max: 100}
    metadata: *length
    line_length: 90
`), &cfg))

	rule := cfg.Rules["MD013"]
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	assert.Equal(t, map[string]any{"max": 100}, rule.Options, "flat options wait to be read")

	dropped, err := rule.ResolveFlatOptions(func(name string) bool { return name == "line_length" })
	require.NoError(t, err)
	assert.Equal(t, []string{"metadata"}, dropped)
	assert.Equal(t, map[string]any{"max": 100, "line_length": 90}, rule.Options)

	dropped, err = rule.ResolveFlatOptions(func(string) bool { return true })
	require.NoError(t, err)
	assert.Empty(t, dropped, "no flat option is left to read")
	assert.Equal(t, map[string]any{"max": 100, "line_length": 90}, rule.Options)
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
