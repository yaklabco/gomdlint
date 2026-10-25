package configloader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// namedSettings is a markdownlint config that names rules by ID, by
// markdownlint's names and by a tag.
const namedSettings = `{
  "default": true,
  "MD013": {"line_length": 120, "tables": false},
  "no-hard-tabs": false,
  "whitespace": false,
  "single-title": {"level": 1},
  "ul-style": "warning"
}`

// TestMigrationFileKeysRulesByName covers the YAML file that migrate
// writes: a header that names the source and says how rules are keyed,
// rules keyed by name with their IDs in trailing comments, a comment above
// the rules that a markdownlint tag sets and above gomdlint's own rules,
// and no unset or empty settings. It loads to the converted settings.
func TestMigrationFileKeysRulesByName(t *testing.T) {
	converted := convertSettings(t, namedSettings)
	path := filepath.Join(t.TempDir(), ".gomdlint.yml")

	content, err := converted.FileContent(path)
	require.NoError(t, err)
	text := string(content)

	assert.True(t, strings.HasPrefix(text, migrationHeader(converted.SourcePath)), text)
	assert.Contains(t, migrationHeader(converted.SourcePath),
		"# gomdlint configuration\n# Migrated from: .markdownlint.json\n"+
			"# Rule keys are the names that 'gomdlint rules' lists. IDs such as MD013 also work.\n")
	assert.NotRegexp(t, `(?m)^\s*MDL?\d{3}:`, text, "no rule is keyed by ID")
	assert.NotContains(t, text, "null", "unset settings are left out")
	assert.NotContains(t, text, "{}", "empty options are left out")
	for _, want := range []string{
		"\n  line-length:", "# MD013\n",
		"\n  unordered-list-style:", "# MD004\n",
		"\n  single-h1:", "# MD025\n",
		"\n\n  # \"whitespace\", a markdownlint tag, sets these rules:\n  list-marker-space:",
		"\n\n  # gomdlint's own rules, which markdownlint does not have:\n  link-destination-style:",
	} {
		assert.Contains(t, text, want)
	}
	assert.Regexp(t, `\n  line-length: +# MD013\n    enabled: true\n    severity: error\n    reset_options: true\n`+
		`    options:\n      ignore_tables: true\n      max: 120\n`, text)
	assert.Regexp(t, `\nrule_defaults:\n  heading-increment: +# MD001\n    enabled: true\n    severity: error\n`, text)

	assertLoadsAsConverted(t, converted, path, content)
}

// TestMigrationJSONFileKeysRulesByName covers the JSON file that migrate
// writes for a .json name: rules keyed by name, without unset or empty
// settings, which loads to the converted settings.
func TestMigrationJSONFileKeysRulesByName(t *testing.T) {
	converted := convertSettings(t, namedSettings)
	path := filepath.Join(t.TempDir(), ".gomdlint.json")

	content, err := converted.FileContent(path)
	require.NoError(t, err)

	var file map[string]map[string]map[string]any
	require.NoError(t, json.Unmarshal(content, &file), string(content))
	assert.Contains(t, file["rules"], "line-length")
	assert.Contains(t, file["rule_defaults"], "heading-increment")
	assert.NotContains(t, file["rules"], "MD013")
	assert.NotContains(t, string(content), "null")
	assert.NotContains(t, string(content), "{}")

	assertLoadsAsConverted(t, converted, path, content)
}

// assertLoadsAsConverted writes content, the file that migrate writes for
// converted, to path, and checks that it loads without a warning to the
// converted settings. Empty options are the same as none.
func assertLoadsAsConverted(t *testing.T, converted *MigrationResult, path string, content []byte) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, content, 0o600))
	loaded, warnings, err := loadConfigFile(path)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, withoutEmptyOptions(t, converted.Config.Rules), withoutEmptyOptions(t, loaded.Rules))
	assert.Equal(t, withoutEmptyOptions(t, converted.Config.RuleDefaults), withoutEmptyOptions(t, loaded.RuleDefaults))
}

// withoutEmptyOptions returns a copy of settings in which empty options
// are none, and numbers are float64, as JSON reads them, so that settings
// read from YAML and JSON compare equal.
func withoutEmptyOptions(t *testing.T, settings map[string]config.RuleConfig) map[string]config.RuleConfig {
	t.Helper()

	copied := make(map[string]config.RuleConfig, len(settings))
	for id, rule := range settings {
		if len(rule.Options) == 0 {
			rule.Options = nil
		} else {
			encoded, err := json.Marshal(rule.Options)
			require.NoError(t, err)
			rule.Options = nil
			require.NoError(t, json.Unmarshal(encoded, &rule.Options))
		}
		copied[id] = rule
	}
	return copied
}

// TestWrittenSettingsWriteEveryRuleSetting keeps the settings that a
// migrated config writes in step with the settings that a rule has: a
// setting that writtenSettings lacks would be lost on migration.
func TestWrittenSettingsWriteEveryRuleSetting(t *testing.T) {
	fields := reflect.TypeFor[writtenSettings]()
	keys := make([]string, 0, fields.NumField())
	for idx := range fields.NumField() {
		field := fields.Field(idx)
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		keys = append(keys, name)
	}

	assert.Equal(t, config.RuleSettingKeys(), keys)
}

// lengthRule is a custom rule named line-length, like MD013.
type lengthRule struct {
	lint.BaseRule
}

func (*lengthRule) Apply(*lint.RuleContext) ([]lint.Diagnostic, error) { return nil, nil }

// TestMigrationFileKeysCollidingNamesByID covers a registry in which a
// custom rule, MD900, is named line-length, like MD013: the file that
// migrate writes keys MD013 by its ID, in YAML and in JSON, so that the
// file configures MD013 and not MD900 when the registry loads it.
func TestMigrationFileKeysCollidingNamesByID(t *testing.T) {
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	registry.Register(&lengthRule{lint.NewBaseRule("MD900", "line-length", "", nil, false)})
	converted := convertSettings(t, `{"MD013": false}`)
	converted.keyRegistry = registry

	for _, name := range []string{".gomdlint.yml", ".gomdlint.json"} {
		content, err := converted.FileContent(filepath.Join(t.TempDir(), name))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "line-length", name)
		if name == ".gomdlint.yml" {
			assert.Regexp(t, `\nrules:\n  MD013: +# MD013\n    enabled: false\n`, string(content))
		}

		loaded, err := config.FromYAML(content)
		require.NoError(t, err, name)
		require.NoError(t, normalizeRuleKeys(loaded, registry), name)
		assert.Equal(t, map[string]bool{"MD013": false}, enabledStates(loaded.Rules), name)
		assert.Contains(t, loaded.RuleDefaults, "MD013", name)
		assert.NotContains(t, loaded.RuleDefaults, "MD900", name)
		assert.Len(t, loaded.RuleDefaults, len(converted.Config.RuleDefaults), name)
	}
}
