package ruledocs_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// ruleKeysHeader is the line of a generated configuration that says how
// it names rules.
const ruleKeysHeader = "# Rule keys are the names that 'gomdlint rules' lists. IDs such as MD013 also work.\n"

// schemaModeline is the first line of a generated configuration in YAML,
// which points editors at the configuration's JSON Schema.
const schemaModeline = "# yaml-language-server: $schema=" + ruledocs.SchemaURL + "\n"

// configTemplate returns the configuration template that init writes for
// opts, documenting the built-in rules.
func configTemplate(t *testing.T, opts ruledocs.TemplateOptions) string {
	t.Helper()

	content, err := ruledocs.ConfigTemplate(ruledocs.Entries(lint.DefaultRegistry, nil), opts)
	require.NoError(t, err)
	return string(content)
}

// TestConfigTemplateMinimal covers the minimal template of init: a header
// that names the schema and says how rules are named, and a commented
// example that keys rules by name and options by their canonical names.
// Uncommented, the example loads without a mistake: every key names a
// rule, and every option is one that its rule reads.
func TestConfigTemplateMinimal(t *testing.T) {
	content := configTemplate(t, ruledocs.TemplateOptions{})

	assert.True(t, strings.HasPrefix(content, schemaModeline+"# gomdlint configuration\n"+ruleKeysHeader), content)
	assert.NotRegexp(t, `MD\d{3}:`, content, "the template keys no rule by ID")

	var example []string
	inExample := false
	for line := range strings.Lines(content) {
		if line == "# rules:\n" {
			inExample = true
		}
		if inExample && strings.HasPrefix(line, "#") {
			example = append(example, strings.TrimPrefix(strings.TrimPrefix(line, "#"), " "))
		}
	}
	require.NotEmpty(t, example, "the template has a commented rules example")
	cfg, err := config.FromYAML([]byte(strings.Join(example, "")))
	require.NoError(t, err)
	assert.Len(t, cfg.Rules, 3)
	for key, settings := range cfg.Rules {
		rule, found := lint.DefaultRegistry.GetByName(key)
		require.True(t, found, "%q is the name of a rule", key)
		_, issues, _ := lint.DefaultRegistry.NormalizeOptions(rule.ID(), settings.Options)
		assert.Empty(t, issues, "%s reads every option of the example", key)
		for option := range settings.Options {
			assert.True(t, slices.ContainsFunc(declaredOptions(t, rule.ID()), func(spec lint.OptionSpec) bool {
				return spec.Name == option
			}), "%s option %q is its canonical name", key, option)
		}
	}
}

// declaredOptions returns the options that the catalog declares for the
// built-in rule with the given ID.
func declaredOptions(t *testing.T, id string) []lint.OptionSpec {
	t.Helper()

	meta, found := lint.DefaultRegistry.Meta(id)
	require.True(t, found, id)
	return meta.Options
}

// TestConfigTemplateMinimalJSON covers the minimal JSON template, which
// holds what the minimal YAML template sets: the flavor alone.
func TestConfigTemplateMinimalJSON(t *testing.T) {
	content := configTemplate(t, ruledocs.TemplateOptions{JSON: true})

	assert.JSONEq(t, `{"flavor": "gfm"}`, content)
}

// TestConfigTemplateFull covers the full template of init: every rule,
// grouped under a comment for its category in the order that rules lists
// them, keyed by its name with its ID in a trailing comment, after its
// description from the catalog. Its default state is a setting, and its
// severity and options, with their defaults, are comments, so the file
// changes nothing until it is edited.
func TestConfigTemplateFull(t *testing.T) {
	content := configTemplate(t, ruledocs.TemplateOptions{Full: true})
	lines := strings.Split(content, "\n")

	assert.True(t, strings.HasPrefix(content, schemaModeline+"# gomdlint configuration\n"+ruleKeysHeader), content)
	assert.NotRegexp(t, `(?m)^\s*MD\d{3}:`, content, "the template keys no rule by ID")

	var parsed struct {
		Rules map[string]map[string]any `yaml:"rules"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(content), &parsed))
	entries := ruledocs.Entries(lint.DefaultRegistry, nil)
	assert.Len(t, parsed.Rules, len(entries), "every rule is listed once")

	for _, entry := range entries {
		assert.Equal(t, map[string]any{"enabled": entry.Default.Enabled}, parsed.Rules[entry.Name], entry.Name)

		at := slices.IndexFunc(lines, func(line string) bool {
			return strings.HasPrefix(line, "  "+entry.Name+":") && strings.HasSuffix(line, "# "+entry.ID)
		})
		if !assert.GreaterOrEqual(t, at, 0, "%s is keyed by name with its ID as a comment", entry.Name) {
			continue
		}
		assert.Equal(t, entry.Description, commentAbove(lines, at), "%s is described from the catalog", entry.Name)
		require.Less(t, at+2, len(lines), entry.Name)
		assert.Equal(t, "    # severity: "+string(entry.Default.Severity), lines[at+2], entry.Name)
		for _, spec := range entry.Options {
			assert.Contains(t, content, "\n    #   "+spec.Name+":", "%s documents option %s", entry.Name, spec.Name)
		}

		header := slices.Index(lines, "  # "+entry.Category.Title())
		assert.Positive(t, header, "%s: its category has a header", entry.Name)
		assert.Less(t, header, at, "%s follows its category's header", entry.Name)
		assert.Less(t, at, nextHeader(lines, header), "%s comes before the next category's header", entry.Name)
	}

	var headers []int
	for _, category := range lint.Categories() {
		if at := slices.Index(lines, "  # "+category.Title()); at >= 0 {
			headers = append(headers, at)
		}
	}
	assert.True(t, slices.IsSorted(headers), "categories come in the order of the listing")

	assert.Regexp(t, `\n  heading-style: +# MD003\n`+
		`    enabled: true\n`+
		`    # severity: warning\n`+
		`    # options:\n`+
		`    #   style: consistent  # consistent, atx, atx_closed, setext, setext_with_atx, setext_with_atx_closed\n`+
		`    #   require_closing_atx: false\n`, content)
	assert.Contains(t, content, "    #   heading_max:  # defaults to the value of max\n",
		"an option that follows another shows no value, which would stop it following")
}

// commentAbove returns the text of the comment lines that end just above
// the line at index at, joined by spaces, without their comment markers.
func commentAbove(lines []string, at int) string {
	var words []string
	for idx := at - 1; idx >= 0 && strings.HasPrefix(lines[idx], "  # "); idx-- {
		words = append([]string{strings.TrimPrefix(lines[idx], "  # ")}, words...)
	}
	return strings.Join(words, " ")
}

// TestConfigTemplateFullJSON covers the full JSON template, which holds
// what the full YAML template sets: every rule by name with its default
// state, and the settings the YAML template sets outside rules.
func TestConfigTemplateFullJSON(t *testing.T) {
	content := configTemplate(t, ruledocs.TemplateOptions{Full: true, JSON: true})

	rules := map[string]any{}
	for _, entry := range ruledocs.Entries(lint.DefaultRegistry, nil) {
		rules[entry.Name] = map[string]any{"enabled": entry.Default.Enabled}
	}
	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(content), &settings), content)
	assert.Equal(t, map[string]any{
		"flavor":          "gfm",
		"backups":         map[string]any{"enabled": true, "mode": "sidecar"},
		"gitignore":       true,
		"default_ignores": true,
		"rules":           rules,
	}, settings, "the JSON template holds the settings the YAML template sets")
	assert.True(t, strings.HasSuffix(content, "}\n"), "the file ends with a newline")
}

// TestConfigTemplateFullWithoutRules covers a full template of no rules,
// which has an empty rules mapping, in YAML and in JSON.
func TestConfigTemplateFullWithoutRules(t *testing.T) {
	content, err := ruledocs.ConfigTemplate(nil, ruledocs.TemplateOptions{Full: true})
	require.NoError(t, err)
	assert.Contains(t, string(content), "\nrules: {}\n")

	content, err = ruledocs.ConfigTemplate(nil, ruledocs.TemplateOptions{Full: true, JSON: true})
	require.NoError(t, err)
	var settings map[string]any
	require.NoError(t, json.Unmarshal(content, &settings))
	assert.Equal(t, map[string]any{}, settings["rules"])
}

// nextHeader returns the index of the first category header after the line
// at index header, a category header, or the number of lines when there is
// none. A header is a comment line between two blank lines.
func nextHeader(lines []string, header int) int {
	for idx := header + 1; idx+1 < len(lines); idx++ {
		if lines[idx-1] == "" && strings.HasPrefix(lines[idx], "  # ") && lines[idx+1] == "" {
			return idx
		}
	}
	return len(lines)
}
