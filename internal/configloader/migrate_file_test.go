package configloader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// namedSettings is a markdownlint config that names rules by ID, by
// markdownlint's names and by a tag, as section 4.1 of the rule-name
// ergonomics plan shows.
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

	assert.True(t, strings.HasPrefix(text, GenerateMigrationHeader(converted.SourcePath)), text)
	assert.Contains(t, GenerateMigrationHeader(converted.SourcePath),
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
	assert.Equal(t, withoutEmptyOptions(converted.Config.Rules), withoutEmptyOptions(loaded.Rules))
	assert.Equal(t, withoutEmptyOptions(converted.Config.RuleDefaults), withoutEmptyOptions(loaded.RuleDefaults))
}

// withoutEmptyOptions returns a copy of settings in which empty options
// are none, and numbers are float64, as JSON reads them, so that settings
// read from YAML and JSON compare equal.
func withoutEmptyOptions(settings map[string]config.RuleConfig) map[string]config.RuleConfig {
	copied := make(map[string]config.RuleConfig, len(settings))
	for id, rule := range settings {
		if len(rule.Options) == 0 {
			rule.Options = nil
		} else {
			encoded, err := json.Marshal(rule.Options)
			if err != nil {
				panic(err)
			}
			rule.Options = nil
			if err := json.Unmarshal(encoded, &rule.Options); err != nil {
				panic(err)
			}
		}
		copied[id] = rule
	}
	return copied
}
