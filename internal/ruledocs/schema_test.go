package ruledocs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/configloader"
	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// schemaURL is where the schema says it lives, which compiles it.
const schemaURL = "https://raw.githubusercontent.com/yaklabco/gomdlint/main/docs/schema/gomdlint.schema.json"

// compileSchema compiles the generated JSON Schema of the configuration.
func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, ruledocs.WriteSchema(&out, ruledocs.Entries(lint.DefaultRegistry, nil)))
	doc, err := jsonschema.UnmarshalJSON(&out)
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource(schemaURL, doc))
	schema, err := compiler.Compile(schemaURL)
	require.NoError(t, err)
	return schema
}

// validateYAML validates a configuration file's YAML against schema, as an
// editor with yaml-language-server does.
func validateYAML(t *testing.T, schema *jsonschema.Schema, data []byte) error {
	t.Helper()

	var value any
	require.NoError(t, yaml.Unmarshal(data, &value))
	if value == nil {
		value = map[string]any{} // An empty file sets nothing.
	}
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	require.NoError(t, err)
	return schema.Validate(instance)
}

// TestSchemaAcceptsTheRepositoryConfigs validates the repository's own
// configuration and every fixture configuration that the CLI tests and
// the golden tests read against the schema.
func TestSchemaAcceptsTheRepositoryConfigs(t *testing.T) {
	root := repoRoot(t)
	schema := compileSchema(t)

	paths := []string{filepath.Join(root, ".gomdlint.yml")}
	for _, pattern := range []string{
		"internal/cli/testdata/*/.gomdlint.yml",
		"pkg/lint/rules/testdata/*/*.config.yml",
	} {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		require.NoError(t, err)
		require.NotEmpty(t, matches, pattern)
		paths = append(paths, matches...)
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.NoError(t, validateYAML(t, schema, data), "%s", path)
	}
}

// TestSchemaAgreesWithTheLoader validates configurations against the
// schema and loads them as gomdlint does: the schema accepts each one that
// gomdlint loads without an error or a warning, and rejects the others.
func TestSchemaAgreesWithTheLoader(t *testing.T) {
	schema := compileSchema(t)

	for _, tt := range []struct {
		name  string
		yaml  string
		valid bool

		// schemaValid marks a configuration that gomdlint rejects for a
		// reason that a JSON Schema cannot express, which the schema
		// accepts.
		schemaValid bool
	}{
		{name: "empty", yaml: "", valid: true},
		{name: "a rule by name", yaml: "rules:\n  line-length: false\n", valid: true},
		{name: "a rule by ID", yaml: "rules:\n  MD013: false\n", valid: true},
		{name: "a rule by markdownlint's name", yaml: "rules:\n  ul-style: error\n", valid: true},
		{name: "a rule by another markdownlint name", yaml: "rules:\n  first-line-h1: true\n", valid: true},
		{name: "rules in other cases", yaml: "rules:\n  md013:\n    enabled: true\n  Heading-Increment:\n    severity: info\n", valid: true},
		{name: "a rule with spaces around it", yaml: "rules:\n  \" MD013 \": false\n", valid: true},
		{name: "settings and options", yaml: "rules:\n  line-length:\n    enabled: true\n    severity: error\n" +
			"    auto_fix: false\n    reset_options: true\n    options:\n      max: 100\n      ignore_tables: true\n", valid: true},
		{name: "markdownlint's option spelling", yaml: "rules:\n  line-length:\n    options:\n      line_length: 100\n      code_blocks: false\n", valid: true},
		{name: "flat options", yaml: "rules:\n  line-length:\n    line_length: 100\n    strict: true\n", valid: true},
		{name: "rule defaults", yaml: "rule_defaults:\n  MD013:\n    options:\n      max: 120\n", valid: true},
		{name: "an allowed value", yaml: "rules:\n  heading-style:\n    options:\n      style: setext_with_atx\n", valid: true},
		{name: "a string or false", yaml: "rules:\n  first-line-heading:\n    options:\n      front_matter_title: false\n", valid: true},
		{name: "a list of strings", yaml: "rules:\n  MD043:\n    options:\n      headings: [\"# Title\", \"*\"]\n", valid: true},
		{name: "a number or a list", yaml: "rules:\n  MD030:\n    options:\n      ul_single: 1\n  MD024:\n    options:\n      siblings_only: true\n", valid: true},
		{name: "an unset option", yaml: "rules:\n  MD047:\n    options:\n      max_trailing_blank_lines: null\n", valid: true},
		{name: "every other setting", yaml: "flavor: commonmark\nseverity_default: info\nignore: [\"vendor/**\"]\n" +
			"gitignore: false\ndefault_ignores: true\nbackups:\n  enabled: true\n  mode: sidecar\n", valid: true},

		{name: "a misspelled rule", yaml: "rules:\n  line-lenght: false\n"},
		{name: "a tag", yaml: "rules:\n  whitespace: false\n"},
		{name: "a category", yaml: "rules:\n  layout: false\n"},
		{name: "an unknown option", yaml: "rules:\n  line-length:\n    options:\n      maxx: 100\n"},
		{name: "an unknown flat option", yaml: "rules:\n  line-length:\n    maxx: 100\n"},
		{name: "an option of the wrong type", yaml: "rules:\n  line-length:\n    options:\n      max: long\n"},
		{name: "an unknown severity", yaml: "rules:\n  line-length: loud\n"},
		{name: "an unknown setting", yaml: "flavour: gfm\n"},
		{name: "a setting only flags set", yaml: "fix: true\n"},
		{name: "an unknown flavor", yaml: "flavor: markdown\n"},
		{name: "an unknown backup mode", yaml: "backups:\n  mode: zip\n"},
		{name: "an unknown backup setting", yaml: "backups:\n  keep: 3\n"},

		{name: "two keys for one rule", yaml: "rules:\n  MD013: false\n  line-length: true\n", schemaValid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			schemaErr := validateYAML(t, schema, []byte(tt.yaml))
			loaded := loadsCleanly(t, tt.yaml)

			assert.Equal(t, tt.valid, loaded, "gomdlint loads it cleanly")
			if tt.valid || tt.schemaValid {
				assert.NoError(t, schemaErr)
			} else {
				assert.Error(t, schemaErr)
			}
		})
	}
}

// loadsCleanly reports whether gomdlint loads content, a configuration
// file, without an error or a warning.
func loadsCleanly(t *testing.T, content string) bool {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".gomdlint.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	result, err := configloader.Load(context.Background(), configloader.LoadOptions{
		WorkingDir:         filepath.Dir(path),
		ExplicitPath:       path,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreEnv:          true,
		IgnoreMarkdownlint: true,
		NonInteractive:     true,
	})
	if err != nil {
		t.Logf("load: %v", err)
		return false
	}
	if len(result.Warnings) > 0 {
		t.Logf("warnings: %s", strings.Join(result.Warnings, "; "))
		return false
	}
	return true
}

// TestSchemaDescribesEveryRule checks that the schema names every rule by
// its name, its ID and its aliases, with the rule's description, and
// describes every option.
func TestSchemaDescribesEveryRule(t *testing.T) {
	var out bytes.Buffer
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)
	require.NoError(t, ruledocs.WriteSchema(&out, entries))

	var doc struct {
		Comment     string `json:"$comment"`
		Definitions map[string]struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		} `json:"definitions"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	assert.Contains(t, doc.Comment, "go test ./internal/ruledocs -update")

	rulesSchema := doc.Definitions["rules"].Properties
	for _, entry := range entries {
		for _, key := range append([]string{entry.Name, entry.ID}, entry.Aliases...) {
			if assert.Contains(t, rulesSchema, key, "%s", entry.ID) {
				assert.Contains(t, rulesSchema[key].Description, entry.Description, "%s", key)
			}
		}
	}

	topKeys := []string{"backups", "default_ignores", "flavor", "gitignore", "ignore", "rule_defaults", "rules", "severity_default"}
	assert.ElementsMatch(t, topKeys, keysOf(doc.Properties), "the settings that a configuration file reads")
	assert.ElementsMatch(t, topKeys, configKeys(), "the keys of config.Config")
}

// TestSchemaOptionDefaults covers the defaults that the schema gives
// options for editors to fill in: the option's own, and the opposite one
// for a spelling that holds the opposite value.
func TestSchemaOptionDefaults(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, ruledocs.WriteSchema(&out, ruledocs.Entries(lint.DefaultRegistry, nil)))

	var doc struct {
		Definitions map[string]struct {
			AnyOf []struct {
				Properties map[string]struct {
					Default    any `json:"default"`
					Properties map[string]struct {
						Default any `json:"default"`
					} `json:"properties"`
				} `json:"properties"`
			} `json:"anyOf"`
		} `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	settings := doc.Definitions["rule-MD013"].AnyOf[2].Properties

	assert.InDelta(t, 80, settings["max"].Default, 0)
	assert.Equal(t, false, settings["ignore_code_blocks"].Default)
	assert.Equal(t, true, settings["code_blocks"].Default, "code_blocks is ignore_code_blocks inverted")
	assert.Equal(t, true, settings["options"].Properties["code_blocks"].Default)
	assert.Nil(t, settings["heading_max"].Default, "heading_max defaults to the value of max")
}

// keysOf returns the keys of properties.
func keysOf(properties map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	return keys
}

// configKeys returns the keys that a configuration file can set, as
// config.Config's YAML encoding writes them all.
func configKeys() []string {
	cfg := config.NewConfig()
	cfg.SeverityDefault = string(config.SeverityInfo)
	cfg.RuleDefaults = map[string]config.RuleConfig{"MD001": {}}
	cfg.Gitignore = config.Bool(true)
	cfg.DefaultIgnores = config.Bool(true)
	data, err := cfg.ToYAML()
	if err != nil {
		return nil
	}
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return nil
	}
	keys := make([]string, 0, len(top))
	for key := range top {
		keys = append(keys, key)
	}
	return keys
}
