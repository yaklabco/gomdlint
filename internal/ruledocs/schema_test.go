package ruledocs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// compileSchema compiles the generated JSON Schema of the configuration.
func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, ruledocs.WriteSchema(&out, ruledocs.Entries(lint.DefaultRegistry, nil)))
	doc, err := jsonschema.UnmarshalJSON(&out)
	require.NoError(t, err)

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource(ruledocs.SchemaURL, doc))
	schema, err := compiler.Compile(ruledocs.SchemaURL)
	require.NoError(t, err)
	return schema
}

// validateYAML validates a configuration file's YAML against schema, as an
// editor with yaml-language-server does.
func validateYAML(t *testing.T, schema *jsonschema.Schema, data []byte) error {
	t.Helper()

	var value any // An empty file, like one that holds only null, is null.
	require.NoError(t, yaml.Unmarshal(data, &value))
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

// TestSchemaAcceptsTheConfigsGomdlintWrites validates the configurations
// that gomdlint writes against the schema: the templates of gomdlint init,
// in YAML and JSON, minimal and full, and a markdownlint configuration that
// gomdlint migrate translates, which keys rules by name.
func TestSchemaAcceptsTheConfigsGomdlintWrites(t *testing.T) {
	schema := compileSchema(t)

	entries := ruledocs.Entries(lint.DefaultRegistry, nil)
	for _, full := range []bool{false, true} {
		for _, asJSON := range []bool{false, true} {
			content, err := ruledocs.ConfigTemplate(entries, ruledocs.TemplateOptions{Full: full, JSON: asJSON})
			require.NoError(t, err)
			assert.NoError(t, validateYAML(t, schema, content), "init, full %t, JSON %t", full, asJSON)
		}
	}

	source := filepath.Join(t.TempDir(), ".markdownlint.json")
	require.NoError(t, os.WriteFile(source, []byte(`{"default": true, "MD013": {"line_length": 100}, "MD033": false, "no-hard-tabs": "error"}`), 0o600))
	migrated, err := configloader.ConvertMarkdownlintConfig(source)
	require.NoError(t, err)
	for _, target := range configNames {
		content, err := migrated.FileContent(target)
		require.NoError(t, err)
		assert.NoError(t, validateYAML(t, schema, content), "migrate to %s:\n%s", target, content)
	}
}

// TestSchemaAgreesWithTheLoader validates configurations against the
// schema and loads them as gomdlint does: the schema accepts each one that
// gomdlint loads without an error or a warning, and rejects the others,
// except the mistakes that a schema cannot see, which it accepts.
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
		{name: "only null", yaml: "null\n", valid: true},
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

		{name: "a misspelled rule", yaml: "rules:\n  line-lengt: false\n"},
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
		{name: "an option and its other spelling", yaml: "rules:\n  line-length:\n    options:\n      max: 90\n      line_length: 100\n", schemaValid: true},
		{name: "an option flat and under options", yaml: "rules:\n  line-length:\n    max: 90\n    options:\n      max: 100\n", schemaValid: true},
		{name: "an invalid glob", yaml: "ignore: [\"[\"]\n", schemaValid: true},
		{name: "an empty ignore pattern", yaml: "ignore: [\"\"]\n"},

		// yaml.v3 reads a null as the zero value, so gomdlint loads every
		// null setting as unset.
		{name: "null settings", yaml: "flavor:\nseverity_default:\nrules:\nrule_defaults:\nignore:\n" +
			"gitignore:\ndefault_ignores:\nbackups:\n", valid: true},
		{name: "null backup settings", yaml: "backups:\n  enabled:\n  mode:\n", valid: true},
		{name: "a null rule", yaml: "rules:\n  MD013:\n", valid: true},
		{name: "null rule settings", yaml: "rules:\n  MD013:\n    enabled: null\n    severity: null\n" +
			"    auto_fix: null\n    reset_options: null\n    options: null\n", valid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			schemaErr := validateYAML(t, schema, []byte(tt.yaml))
			loaded := loadsCleanly(t, ".gomdlint.yml", tt.yaml)

			assert.Equal(t, tt.valid, loaded, "gomdlint loads it cleanly")
			if tt.valid || tt.schemaValid {
				assert.NoError(t, schemaErr)
			} else {
				assert.Error(t, schemaErr)
			}
		})
	}
}

// configNames are the names of a YAML and a JSON configuration file.
var configNames = []string{".gomdlint.yml", ".gomdlint.json"}

// TestSchemaAgreesWithTheLoaderOnEverySetting sets each setting that a
// configuration file reads, at the top level, under backups, and for rules,
// named by ID, name and markdownlint name, under rules and rule_defaults,
// with their settings and some options, to each of many values, in a YAML
// and a JSON file: null, empty strings, the spellings of true and false
// that yaml.v3 reads into a boolean setting, strings that look like
// booleans, numbers, lists, mappings, and the values of the enumerated
// settings. Whole files that are no mapping, and plain and tagged YAML
// scalars, which a JSON document cannot write, are checked too. The schema
// accepts each file that gomdlint loads without an error or a warning, and
// rejects the others.
func TestSchemaAgreesWithTheLoaderOnEverySetting(t *testing.T) {
	schema := compileSchema(t)

	paths := [][]string{
		{"flavor"}, {"severity_default"}, {"rules"}, {"rule_defaults"}, {"ignore"},
		{"gitignore"}, {"default_ignores"}, {"backups"}, {"backups", "enabled"}, {"backups", "mode"},
		{"rules", "MD013"}, {"rules", "line-length"}, {"rules", "ul-style"},
		{"rules", "MD013", "enabled"}, {"rules", "MD013", "severity"},
		{"rules", "MD013", "auto_fix"}, {"rules", "MD013", "reset_options"}, {"rules", "MD013", "options"},
		{"rules", "line-length", "enabled"}, {"rules", "ul-style", "severity"},
		{"rule_defaults", "MD013"}, {"rule_defaults", "MD013", "enabled"}, {"rule_defaults", "MD013", "severity"},
		{"rule_defaults", "MD013", "auto_fix"}, {"rule_defaults", "MD013", "reset_options"},
		{"rule_defaults", "MD013", "options"},
		{"rules", "MD013", "options", "max"}, {"rules", "MD013", "options", "ignore_tables"},
		{"rules", "MD013", "strict"}, {"rules", "MD003", "options", "style"},
		{"rules", "MD044", "options", "names"}, {"rules", "MD044", "names"},
	}
	values := []any{
		nil, "", " ", "yes", "Yes", "YES", "y", "Y", "on", "On", "ON", "no", "No", "NO", "n", "N", "off", "Off", "OFF",
		"yES", "true", "false", "True", "1", "0", true, false, 0, 1, 80, -1, 2.5,
		"error", "warning", "info", "Error", "gfm", "commonmark", "GFM", "sidecar", "none", "consistent", "atx",
		[]any{}, []any{"a"}, []any{""}, []any{1}, []any{true}, []any{nil}, map[string]any{}, map[string]any{"a": 1},
	}

	var docs []string
	for _, path := range paths {
		for _, value := range values {
			doc := value
			for idx := len(path) - 1; idx >= 0; idx-- {
				doc = map[string]any{path[idx]: doc}
			}
			content, err := json.Marshal(doc)
			require.NoError(t, err)
			docs = append(docs, string(content))
		}
	}
	docs = append(docs, "null", "[]", "0", `""`, "true")

	for _, content := range docs {
		schemaErr := validateYAML(t, schema, []byte(content))
		for _, name := range configNames {
			if loaded := loadsCleanly(t, name, content); loaded != (schemaErr == nil) {
				t.Errorf("%s %s: gomdlint loads it cleanly: %t, the schema accepts it: %t (%v)",
					name, content, loaded, schemaErr == nil, schemaErr)
			}
		}
	}

	for _, content := range []string{
		"gitignore: yes\n", "gitignore: on\n", "gitignore: Off\n", "gitignore: ~\n", "gitignore: !!str on\n",
		"gitignore: True\n", "gitignore: 1\n", "flavor: ~\n", "flavor: GFM\n", "severity_default: ~\n",
		"ignore: [0x10, ~, 1.5]\n", "ignore: ['']\n", "backups:\n  enabled: NO\n  mode: ''\n",
		"rules:\n  MD013:\n    enabled: Y\n    auto_fix: off\n    reset_options: !!str yes\n",
		"rules:\n  MD013:\n    options:\n      ignore_tables: yes\n", "rules:\n  MD013: on\n", "rules:\n  MD013: ~\n",
		"rules:\n  MD013:\n    severity: ''\n",
	} {
		schemaErr := validateYAML(t, schema, []byte(content))
		if loaded := loadsCleanly(t, ".gomdlint.yml", content); loaded != (schemaErr == nil) {
			t.Errorf("%q: gomdlint loads it cleanly: %t, the schema accepts it: %t (%v)",
				content, loaded, schemaErr == nil, schemaErr)
		}
	}
}

// loadsCleanly reports whether gomdlint loads content, a configuration
// file named name, without an error or a warning.
func loadsCleanly(t *testing.T, name, content string) bool {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
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
	assert.ElementsMatch(t, topKeys, slices.Collect(maps.Keys(doc.Properties)), "the settings that a configuration file reads")
	assert.ElementsMatch(t, topKeys, configKeys(t), "the keys of config.Config")
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

// configKeys returns the keys that a configuration file can set, as
// config.Config's YAML encoding writes them all.
func configKeys(t *testing.T) []string {
	t.Helper()

	cfg := config.NewConfig()
	cfg.SeverityDefault = string(config.SeverityInfo)
	cfg.RuleDefaults = map[string]config.RuleConfig{"MD001": {}}
	cfg.Gitignore = config.Bool(true)
	cfg.DefaultIgnores = config.Bool(true)
	data, err := cfg.ToYAML()
	require.NoError(t, err)
	var top map[string]any
	require.NoError(t, yaml.Unmarshal(data, &top))
	return slices.Collect(maps.Keys(top))
}
