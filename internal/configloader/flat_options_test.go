package configloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestLoad_FlatOptions checks that a config file may write rule options
// directly under the rule key, as markdownlint does, in YAML and in JSON,
// under rules and rule_defaults. They load without warnings, and
// markdownlint's spellings load as gomdlint's canonical names, an inverted
// spelling with the opposite value, as they do under options.
func TestLoad_FlatOptions(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		".gomdlint.yml": `rules:
  line-length:
    line_length: 100
    code_blocks: false
  MD012:
    maximum: 2
rule_defaults:
  no-trailing-spaces:
    code_blocks: true
    br_spaces: 3
`,
		".gomdlint.json": `{
  "rules": {
    "line-length": {"line_length": 100, "code_blocks": false},
    "MD012": {"maximum": 2}
  },
  "rule_defaults": {
    "no-trailing-spaces": {"code_blocks": true, "br_spaces": 3}
  }
}
`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result, err := loadProjectFile(t, name, content)
			require.NoError(t, err)

			assert.Empty(t, result.Warnings)
			assert.Equal(t, map[string]any{"max": 100, "ignore_code_blocks": true}, result.Config.Rules["MD013"].Options)
			assert.Equal(t, map[string]any{"max_consecutive": 2}, result.Config.Rules["MD012"].Options)
			assert.Equal(t, map[string]any{"ignore_code_blocks": false, "br_spaces": 3},
				result.Config.RuleDefaults["MD009"].Options)
		})
	}
}

// TestLoad_FlatOptionsBesideSettings checks a rule entry that holds rule
// settings, flat options and an options mapping at once: the settings
// apply, and the flat options and the options mapping merge.
func TestLoad_FlatOptionsBesideSettings(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, `rules:
  MD013:
    severity: error
    auto_fix: false
    line_length: 100
    options:
      tables: false
`)
	require.NoError(t, err)

	assert.Empty(t, result.Warnings)
	rule := result.Config.Rules["MD013"]
	require.NotNil(t, rule.Severity)
	assert.Equal(t, "error", *rule.Severity)
	require.NotNil(t, rule.AutoFix)
	assert.False(t, *rule.AutoFix)
	assert.Equal(t, map[string]any{"max": 100, "ignore_tables": true}, rule.Options)
}

// TestLoad_FlatOptionsMergeByOption checks that configuration layers merge
// option by option whatever shape each writes: a project config that sets
// an option overrides that option of a user config and keeps the others.
func TestLoad_FlatOptionsMergeByOption(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, user, project string
	}{
		{
			name:    "flat over options",
			user:    "rules:\n  MD013:\n    options:\n      max: 90\n      strict: true\n",
			project: "rules:\n  line-length:\n    line_length: 100\n",
		},
		{
			name:    "options over flat",
			user:    "rules:\n  MD013:\n    line_length: 90\n    strict: true\n",
			project: "rules:\n  line-length:\n    options:\n      max: 100\n",
		},
		{
			name:    "flat over flat",
			user:    "rules:\n  MD013:\n    line_length: 90\n    strict: true\n",
			project: "rules:\n  line-length:\n    line_length: 100\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			user := filepath.Join(dir, "user.yml")
			project := filepath.Join(dir, "project.yml")
			require.NoError(t, os.WriteFile(user, []byte(tt.user), 0o600))
			require.NoError(t, os.WriteFile(project, []byte(tt.project), 0o600))

			merged := mergeFiles(t, user, project)
			assert.Equal(t, map[string]any{"max": 100, "strict": true}, merged.Rules["MD013"].Options)

			// rule_defaults merges the same way.
			userDefaults := filepath.Join(dir, "user-defaults.yml")
			projectDefaults := filepath.Join(dir, "project-defaults.yml")
			require.NoError(t, os.WriteFile(userDefaults, []byte(strings.Replace(tt.user, "rules:", "rule_defaults:", 1)), 0o600))
			require.NoError(t, os.WriteFile(projectDefaults,
				[]byte(strings.Replace(tt.project, "rules:", "rule_defaults:", 1)), 0o600))

			merged = mergeFiles(t, userDefaults, projectDefaults)
			assert.Equal(t, map[string]any{"max": 100, "strict": true}, merged.RuleDefaults["MD013"].Options)
		})
	}
}

// TestMergeAll_KeepsFlatOptions checks that merging configurations that
// FromYAML read, under rules and rule_defaults, keeps the options that each
// writes directly under a rule key, option by option.
func TestMergeAll_KeepsFlatOptions(t *testing.T) {
	t.Parallel()

	base, err := config.FromYAML([]byte(
		"rules:\n  MD013:\n    line_length: 90\n    strict: true\nrule_defaults:\n  MD009:\n    br_spaces: 3\n    strict: true\n"))
	require.NoError(t, err)
	override, err := config.FromYAML([]byte(
		"rules:\n  MD013:\n    line_length: 100\nrule_defaults:\n  MD009:\n    br_spaces: 4\n"))
	require.NoError(t, err)

	merged := MergeAll(base, override)
	assert.Equal(t, map[string]any{"line_length": 100, "strict": true}, merged.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"br_spaces": 4, "strict": true}, merged.RuleDefaults["MD009"].Options)
}

// TestLoad_CloneKeepsFlatOptions checks that a copy of a loaded
// configuration, such as the one that linting makes for a file with a
// markdownlint-configure-file comment, keeps the options that the file
// writes directly under rule keys, under rules and rule_defaults.
func TestLoad_CloneKeepsFlatOptions(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "rules:\n  line-length:\n    line_length: 100\n"+
		"rule_defaults:\n  no-trailing-spaces:\n    br_spaces: 3\n")
	require.NoError(t, err)

	clone := result.Config.Clone()
	assert.Equal(t, map[string]any{"max": 100}, clone.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"br_spaces": 3}, clone.RuleDefaults["MD009"].Options)
	assert.Equal(t, result.Config.Rules, clone.Rules)
	assert.Equal(t, result.Config.RuleDefaults, clone.RuleDefaults)
}

// TestLoad_RejectsAnOptionSetFlatAndUnderOptions checks an option set both
// directly under the rule key and under options. Two spellings of it are
// the error that two spellings under options are, and the same spelling
// twice is that error with the line of the flat key: either value could
// win.
func TestLoad_RejectsAnOptionSetFlatAndUnderOptions(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, content, want string
	}{
		{
			name:    "two spellings",
			content: "rules:\n  line-length:\n    line_length: 100\n    options:\n      max: 90\n",
			want:    `.gomdlint.yml: rules: "line_length" and "max" both set line-length option "max"; keep one of them`,
		},
		{
			name:    "two spellings, both flat",
			content: "rules:\n  line-length:\n    line_length: 100\n    max: 90\n",
			want:    `.gomdlint.yml: rules: "line_length" and "max" both set line-length option "max"; keep one of them`,
		},
		{
			name:    "inverted spelling",
			content: "rule_defaults:\n  MD009:\n    code_blocks: true\n    options:\n      ignore_code_blocks: false\n",
			want: `.gomdlint.yml: rule_defaults: "code_blocks" and "ignore_code_blocks" both set ` +
				`no-trailing-spaces option "ignore_code_blocks"; keep one of them`,
		},
		{
			name:    "same spelling",
			content: "rules:\n  MD013:\n    max: 100\n    options:\n      max: 90\n",
			want:    `.gomdlint.yml: rules: line 3: "max" and "options.max" both set line-length option "max"; keep one of them`,
		},
		{
			name:    "same markdownlint spelling",
			content: "rule_defaults:\n  line-length:\n    line_length: 100\n    options:\n      line_length: 90\n",
			want: `.gomdlint.yml: rule_defaults: line 3: "line_length" and "options.line_length" both set ` +
				`line-length option "max"; keep one of them`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadProject(t, tt.content)
			require.Error(t, err)
			assert.True(t, strings.HasSuffix(err.Error(), tt.want), err.Error())
		})
	}
}

// TestLoad_WarnsAboutFlatOptions checks the warnings about flat options,
// which are those about options under options, with the file and line: an
// unknown option, with the closest spelling, a value of the wrong type and
// a value the option does not allow. A key that is close to a rule setting
// is likely a typo of it, and its warning says so.
func TestLoad_WarnsAboutFlatOptions(t *testing.T) {
	t.Parallel()

	content := `rules:
  line-length:
    lin_length: 100
    max: 100px
    severity: error
  unordered-list-style:
    style: dashes
  list-indent:
    indent: 2
  MD009:
    severty: error
    option:
      br_spaces: 3
rule_defaults:
  frobnicate:
    anything: 1
`
	result, err := loadProject(t, content)
	require.NoError(t, err, "option problems do not fail loading")

	assert.Equal(t, []string{
		`.gomdlint.yml:3: unknown option "lin_length" for line-length (did you mean "line_length"?); it is ignored`,
		`.gomdlint.yml:4: line-length option "max" must be an integer, not "100px"; it is ignored`,
		`.gomdlint.yml:7: unordered-list-style option "style" must be one of "consistent", "asterisk", "plus", "dash" or ` +
			`"sublist", not "dashes" (did you mean "dash"?)`,
		`.gomdlint.yml:9: unknown option "indent" for list-indent (it has no options); it is ignored`,
		`.gomdlint.yml:11: unknown key "rules.MD009.severty" (did you mean "rules.MD009.severity"?); it is ignored`,
		`.gomdlint.yml:12: unknown key "rules.MD009.option" (did you mean "rules.MD009.options"?); it is ignored`,
		`.gomdlint.yml:15: unknown rule "frobnicate"; it is ignored`,
	}, relativeWarnings(result.Warnings, ".gomdlint.yml"))

	rule := result.Config.Rules["MD013"]
	require.NotNil(t, rule.Severity)
	assert.Equal(t, "error", *rule.Severity)
	assert.Empty(t, rule.Options, "the unknown option is never read and the value of the wrong type is dropped")
	assert.Equal(t, map[string]any{"style": "dashes"}, result.Config.Rules["MD004"].Options,
		"a value outside the allowed ones is kept for the rule")
	assert.Empty(t, result.Config.Rules["MD009"].Options, "keys that misspell settings are no options")
}

// TestLoad_WarnsAboutFlatOptionsInJSON checks the same warnings for a JSON
// config file, with the lines of its keys.
func TestLoad_WarnsAboutFlatOptionsInJSON(t *testing.T) {
	t.Parallel()

	content := `{
  "rules": {
    "MD013": {"line_length": 100, "heading_length": 60},
    "MD009": {
      "enable": false
    }
  }
}
`
	result, err := loadProjectFile(t, ".gomdlint.json", content)
	require.NoError(t, err)

	assert.Equal(t, []string{
		`.gomdlint.json:3: unknown option "heading_length" for line-length; it is ignored`,
		`.gomdlint.json:5: unknown key "rules.MD009.enable" (did you mean "rules.MD009.enabled"?); it is ignored`,
	}, relativeWarnings(result.Warnings, ".gomdlint.json"))
	assert.Equal(t, map[string]any{"max": 100}, result.Config.Rules["MD013"].Options)
}

// TestLoad_NativeSettingsBesideAKeyThatHoldsThem checks a config that main
// loads: native settings, and an unknown key whose value is an alias of the
// rule's own settings. Only the rule's declared options are read from the
// keys directly under the rule key, so the unknown key's value, which
// contains itself, is never decoded. The file loads with the same settings
// as on main, and the unknown key gets a warning.
func TestLoad_NativeSettingsBesideAKeyThatHoldsThem(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, `rules:
  MD013: &length
    enabled: false
    severity: error
    options: {max: 100}
    metadata: *length
`)
	require.NoError(t, err)

	severity := "error"
	assert.Equal(t, config.RuleConfig{Enabled: config.Bool(false), Severity: &severity, Options: map[string]any{"max": 100}},
		result.Config.Rules["MD013"])
	assert.Equal(t, []string{`.gomdlint.yml:6: unknown option "metadata" for line-length; it is ignored`},
		relativeWarnings(result.Warnings, ".gomdlint.yml"))
}

// TestLoad_RejectsADeclaredFlatOptionThatCannotDecode checks a key
// written directly under the rule key that spells an option of the rule,
// whose value holds the rule's own settings. The option is read, so its
// value must decode, and loading fails with an error that names the rule,
// the line and the option.
func TestLoad_RejectsADeclaredFlatOptionThatCannotDecode(t *testing.T) {
	t.Parallel()

	_, err := loadProject(t, "rules:\n  MD013: &length\n    line_length: *length\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		`.gomdlint.yml: rules.MD013: line 3: option "line_length": yaml: anchor 'length' value contains itself`)
}

// TestLoad_UnknownOptionWrittenTwiceOnlyWarns checks an unknown option
// written both directly under the rule key and under options. Neither is
// read, so they do not conflict, and each gets a warning.
func TestLoad_UnknownOptionWrittenTwiceOnlyWarns(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "rules:\n  MD013:\n    foo: 1\n    options:\n      foo: 2\n")
	require.NoError(t, err)

	assert.Equal(t, []string{
		`.gomdlint.yml:3: unknown option "foo" for line-length; it is ignored`,
		`.gomdlint.yml:5: unknown option "foo" for line-length; it is ignored`,
	}, relativeWarnings(result.Warnings, ".gomdlint.yml"))
}

// TestLoad_RejectsAKeyWrittenTwiceThroughAnAlias checks two keys in one
// mapping that read as one name through an alias: a rule key under rules
// or rule_defaults, a rule setting, an option directly under the rule key
// and an option under options. Decoding would keep one of the values
// silently, or fail with an error that names a type of gomdlint's, so
// loading fails instead with the error that the key written out twice
// gets, YAML's error at the line of the later key.
func TestLoad_RejectsAKeyWrittenTwiceThroughAnAlias(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, literal, alias, want string
	}{
		{
			name:    "rule key",
			literal: "rules:\n  MD013: false\n  MD013: true\n",
			alias:   "rules:\n  &id MD013: false\n  *id : true\n",
			want:    `line 3: mapping key "MD013" already defined at line 2`,
		},
		{
			name:    "rule key under rule_defaults",
			literal: "rule_defaults:\n  MD013:\n    line_length: 100\n  MD013:\n    line_length: 90\n",
			alias:   "rule_defaults:\n  &id MD013:\n    line_length: 100\n  *id :\n    line_length: 90\n",
			want:    `line 4: mapping key "MD013" already defined at line 2`,
		},
		{
			name:    "rule key in a merged mapping",
			literal: "base: &base\n  MD013: false\n  MD013: true\nrules:\n  <<: *base\n",
			alias:   "base: &base\n  &id MD013: false\n  *id : true\nrules:\n  <<: *base\n",
			want:    `line 3: mapping key "MD013" already defined at line 2`,
		},
		{
			name:    "setting",
			literal: "rules:\n  MD013:\n    severity: error\n    severity: warning\n",
			alias:   "rules:\n  MD013:\n    &set severity: error\n    *set : warning\n",
			want:    `line 4: mapping key "severity" already defined at line 3`,
		},
		{
			name:    "flat option",
			literal: "rules:\n  MD013:\n    line_length: 100\n    line_length: 20\n",
			alias:   "rules:\n  MD013:\n    &opt line_length: 100\n    *opt : 20\n",
			want:    `line 4: mapping key "line_length" already defined at line 3`,
		},
		{
			name:    "under options",
			literal: "rules:\n  MD013:\n    options:\n      max: 100\n      max: 20\n",
			alias:   "rules:\n  MD013:\n    options:\n      &opt max: 100\n      *opt : 20\n",
			want:    `line 5: mapping key "max" already defined at line 4`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, literalErr := loadProject(t, tt.literal)
			_, aliasErr := loadProject(t, tt.alias)

			require.Error(t, literalErr)
			require.Error(t, aliasErr)
			// Each project lives in a directory of its own, which the
			// errors name, so they are compared after the file's name.
			assert.Equal(t, "parse YAML: yaml: unmarshal errors:\n  "+tt.want, afterConfigFile(t, literalErr))
			assert.Equal(t, afterConfigFile(t, literalErr), afterConfigFile(t, aliasErr))
		})
	}
}

// TestLoad_RejectsAnAliasKeyOfItsOwnMapping checks a config file in which
// an alias key names the mapping that holds it. Loading fails with the
// error for a key that is no scalar, at the alias's line, before anything
// decodes the file. Decoding used to report that the node contains itself,
// and an expansion of the key would make a cycle that decoding never
// leaves.
func TestLoad_RejectsAnAliasKeyOfItsOwnMapping(t *testing.T) {
	t.Parallel()

	_, err := loadProject(t, "rules:\n  MD013:\n    options:\n      x: &m {*m : 1}\n")

	require.Error(t, err)
	assert.Equal(t, "parse YAML: yaml: unmarshal errors:\n  line 4: mapping key must be a scalar", afterConfigFile(t, err))
}

// afterConfigFile returns the message of err, an error of loading a
// project's .gomdlint.yml, after the path of the file.
func afterConfigFile(t *testing.T, err error) string {
	t.Helper()

	_, after, found := strings.Cut(err.Error(), string(filepath.Separator)+".gomdlint.yml: ")
	require.True(t, found, "the error names the file: %v", err)
	return after
}

// TestLoad_MergedFlatOptionsYieldToWrittenOnes checks YAML's precedence for
// merge keys under a rule: an option written in the rule's settings
// replaces the one that a merge key brings in, without an error.
func TestLoad_MergedFlatOptionsYieldToWrittenOnes(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, `rule_defaults:
  MD013: &base
    line_length: 100
    tables: false
rules:
  line-length:
    <<: *base
    line_length: 80
`)
	require.NoError(t, err)

	assert.Empty(t, result.Warnings)
	assert.Equal(t, map[string]any{"max": 80, "ignore_tables": true}, result.Config.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"max": 100, "ignore_tables": true}, result.Config.RuleDefaults["MD013"].Options)
}

// settingNamedOptionsRule is a rule whose options are spelled like rule
// settings, which no built-in rule's are: severity, and level, whose other
// spelling is enabled.
type settingNamedOptionsRule struct {
	lint.BaseRule
}

func settingNamedOptionsRegistry() *lint.Registry {
	registry := lint.NewRegistry()
	registry.Register(&settingNamedOptionsRule{
		BaseRule: lint.NewBaseRule("MD999", "custom-rule", "A rule with options named like settings.", nil, false),
	})
	registry.Describe(lint.RuleMeta{ID: "MD999", Name: "custom-rule", OptionsDeclared: true, Options: []lint.OptionSpec{
		{Name: "severity", Type: lint.OptionString, Description: "An option named like a setting."},
		{
			Name: "level", Aliases: []lint.OptionAlias{{Name: "enabled"}}, Type: lint.OptionBoolean,
			Description: "An option with a spelling named like a setting.",
		},
	}})
	return registry
}

// TestUnknownKeyWarnings_FlatKeySpelledLikeAnOption checks a key under a
// rule that is both a rule setting and a spelling of one of the rule's
// options. The setting wins, since a setting key is never read as an
// option, and a warning names the clash and where the option goes. Under
// options the spelling is unambiguous, so it gets no warning.
func TestUnknownKeyWarnings_FlatKeySpelledLikeAnOption(t *testing.T) {
	t.Parallel()

	content := "rules:\n  custom-rule:\n    severity: error\n    enabled: false\n    options:\n      severity: high\n"
	doc, err := parseYAMLDocument([]byte(content))
	require.NoError(t, err)

	assert.Equal(t, []string{
		`x.yml:3: "rules.custom-rule.severity" is a rule setting, not custom-rule option "severity"; ` +
			`to set the option, write it under "rules.custom-rule.options"`,
		`x.yml:4: "rules.custom-rule.enabled" is a rule setting, not custom-rule option "level"; ` +
			`to set the option, write it under "rules.custom-rule.options"`,
	}, unknownKeyWarnings("x.yml", doc, settingNamedOptionsRegistry()))

	cfg, err := config.FromYAML([]byte(content))
	require.NoError(t, err)
	rule := cfg.Rules["custom-rule"]
	require.NotNil(t, rule.Severity)
	assert.Equal(t, "error", *rule.Severity, "the setting wins")
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled, "the setting wins")
	assert.Equal(t, map[string]any{"severity": "high"}, rule.Options)
}
