package ruledocs_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
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
	assert.Regexp(t, `\n#   heading-increment: +# MD001\n#     severity: error\n#   line-length: +# MD013\n`+
		`#     options:\n#       max: 100\n#       ignore_tables: true\n#   no-inline-html: false +# MD033\n`, content,
		"each example rule has its ID in a comment")

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

	wantRules := map[string]any{}
	for _, entry := range ruledocs.Entries(lint.DefaultRegistry, nil) {
		wantRules[entry.Name] = map[string]any{"enabled": entry.Default.Enabled}
	}
	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(content), &settings), content)
	assert.Equal(t, map[string]any{
		"flavor":          "gfm",
		"backups":         map[string]any{"enabled": true, "mode": "sidecar"},
		"gitignore":       true,
		"default_ignores": true,
		"rules":           wantRules,
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

// collidingRegistry returns a registry of the built-in rules and a custom
// rule, MD900, named line-length, like MD013.
func collidingRegistry() *lint.Registry {
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	registry.Register(&quietRule{lint.NewBaseRule("MD900", "line-length", "A custom rule.", nil, false)})
	return registry
}

// TestConfigTemplateFullKeysCollidingNamesByID covers rules that share a
// name: the full template keys each of them by ID, in YAML and in JSON, so
// that no key is written twice and every key configures the rule it
// documents. Other rules keep their names.
func TestConfigTemplateFullKeysCollidingNamesByID(t *testing.T) {
	registry := collidingRegistry()
	entries := ruledocs.Entries(registry, nil)

	for _, asJSON := range []bool{false, true} {
		content, err := ruledocs.ConfigTemplate(entries, ruledocs.TemplateOptions{Full: true, JSON: asJSON})
		require.NoError(t, err, "JSON %t", asJSON)

		var parsed struct {
			Rules map[string]any `json:"rules" yaml:"rules"`
		}
		require.NoError(t, yaml.Unmarshal(content, &parsed), "JSON %t: no key is written twice", asJSON)
		assert.Len(t, parsed.Rules, len(entries), "JSON %t", asJSON)
		assert.Contains(t, parsed.Rules, "MD013", "JSON %t", asJSON)
		assert.Contains(t, parsed.Rules, "MD900", "JSON %t", asJSON)
		assert.Contains(t, parsed.Rules, "no-trailing-spaces", "JSON %t", asJSON)
		assert.NotContains(t, parsed.Rules, "line-length", "JSON %t", asJSON)
		resolved := make(map[string]bool, len(parsed.Rules))
		for key := range parsed.Rules {
			rule, found := registry.Lookup(key)
			require.True(t, found, key)
			resolved[rule.ID()] = true
		}
		assert.Len(t, resolved, len(entries), "JSON %t: each key configures another rule", asJSON)
		if !asJSON {
			assert.Regexp(t, `\n  MD013: +# MD013\n`, string(content))
		}
	}
}

// TestConfigTemplateMinimalKeysCollidingNamesByID covers the minimal
// template's example, which keys a rule whose name another rule shares by
// ID.
func TestConfigTemplateMinimalKeysCollidingNamesByID(t *testing.T) {
	content, err := ruledocs.ConfigTemplate(ruledocs.Entries(collidingRegistry(), nil), ruledocs.TemplateOptions{})
	require.NoError(t, err)

	assert.Contains(t, string(content), "\n#   MD013:")
	assert.NotContains(t, string(content), "\n#   line-length:")
	assert.Contains(t, string(content), "\n#   heading-increment:")
}

// TestConfigTemplateFullQuotesKeys covers custom rules whose names YAML
// would not read as written, with a colon, a # or spaces around them: the
// full template quotes their keys, so that it parses, in YAML and in JSON,
// and each key configures its rule.
func TestConfigTemplateFullQuotesKeys(t *testing.T) {
	registry := lint.NewRegistry()
	for idx, name := range []string{"has: colon", "#hash", " spaced "} {
		registry.Register(&quietRule{lint.NewBaseRule(fmt.Sprintf("XX%03d", idx+1), name, "A custom rule.", nil, false)})
	}

	for _, asJSON := range []bool{false, true} {
		content, err := ruledocs.ConfigTemplate(ruledocs.Entries(registry, nil), ruledocs.TemplateOptions{Full: true, JSON: asJSON})
		require.NoError(t, err, "JSON %t", asJSON)

		var parsed struct {
			Rules map[string]any `json:"rules" yaml:"rules"`
		}
		require.NoError(t, yaml.Unmarshal(content, &parsed), "JSON %t:\n%s", asJSON, content)
		assert.Len(t, parsed.Rules, 3, "JSON %t", asJSON)
		for key := range parsed.Rules {
			rule, found := registry.Lookup(key)
			require.True(t, found, "JSON %t: %q", asJSON, key)
			assert.Equal(t, strings.TrimSpace(key), strings.TrimSpace(rule.Name()), "JSON %t", asJSON)
		}
	}
}

// awkwardScalars are strings that YAML reads as something else, or cannot
// hold as written on one line: keywords, numbers, dates, indicators,
// spaces at either end, line breaks, tabs, quotes, backslashes, characters
// beyond ASCII and the empty string.
func awkwardScalars() []string {
	return []string{
		"null", "~", "true", "yes", "off", "123", "0x1F", "1e3", "2026-10-25", ".inf",
		"a: b", "#x", " lead", "trail ", "line\nbreak", "tab\there", `say "hi"`, "it's",
		`back\slash`, "ünïcødé", "auto_fix: false\nreset_options: true", "- item", "[flow]", "&anchor", "",
		strings.Repeat("a long words ", 20),
		"\na", "\ta\nb", "\n", "a\u2028b", "a\u2029b", "zero\u200bwidth", "nel\u0085x",
		strings.Repeat("a long words ", 20) + "\nand a break",
	}
}

// awkwardRule is a custom rule whose name, option defaults and an option's
// name are awkward scalars, with the scalar as an option's only allowed
// value and as the option that another follows.
type awkwardRule struct {
	lint.BaseRule
	value string
}

func (r *awkwardRule) DeclaredOptions() []lint.OptionSpec {
	return []lint.OptionSpec{
		{Name: "value", Type: lint.OptionString, Default: r.value, Values: []string{r.value}},
		{Name: "unset", Type: lint.OptionString},
		{Name: "follows", Type: lint.OptionString, Default: r.value, DefaultFrom: r.value},
		{Name: r.value, Type: lint.OptionStringList, Default: []string{r.value, "plain"}},
	}
}

// awkwardRegistry returns a registry of one custom rule for each awkward
// scalar, named by it, with it as its options' defaults, its category and
// its description.
func awkwardRegistry() *lint.Registry {
	registry := lint.NewRegistry()
	for idx, scalar := range awkwardScalars() {
		id := fmt.Sprintf("XX%03d", idx+1)
		registry.Register(&awkwardRule{BaseRule: lint.NewBaseRule(id, scalar, scalar, nil, false), value: scalar})
		registry.Describe(lint.RuleMeta{ID: id, Name: scalar, Description: scalar, Category: lint.Category(scalar)})
	}
	return registry
}

// TestConfigTemplateFullWritesAwkwardScalars covers rules whose names and
// option defaults YAML would misread if written as they are: the full
// template parses, in YAML and in JSON, with one key per rule that names
// that rule, and uncommenting every commented setting gives each rule its
// default severity and options, and nothing else.
func TestConfigTemplateFullWritesAwkwardScalars(t *testing.T) {
	registry := awkwardRegistry()
	entries := ruledocs.Entries(registry, nil)

	for _, asJSON := range []bool{false, true} {
		content, err := ruledocs.ConfigTemplate(entries, ruledocs.TemplateOptions{Full: true, JSON: asJSON})
		require.NoError(t, err, "JSON %t", asJSON)

		var parsed struct {
			Rules map[string]map[string]any `json:"rules" yaml:"rules"`
		}
		require.NoError(t, yaml.Unmarshal(content, &parsed), "JSON %t:\n%s", asJSON, content)
		assertKeysNameTheirRules(t, registry, parsed.Rules, fmt.Sprintf("JSON %t", asJSON))
		for key, settings := range parsed.Rules {
			assert.Equal(t, map[string]any{"enabled": true}, settings, "JSON %t: %q", asJSON, key)
		}
	}

	content, err := ruledocs.ConfigTemplate(entries, ruledocs.TemplateOptions{Full: true})
	require.NoError(t, err)
	uncommented := uncommentRuleSettings(string(content))
	cfg, err := config.FromYAML([]byte(uncommented))
	require.NoError(t, err, uncommented)
	require.Len(t, cfg.Rules, len(entries), uncommented)
	for key, settings := range cfg.Rules {
		rule, found := registry.Lookup(key)
		require.True(t, found, "%q", key)
		awkward, isAwkward := rule.(*awkwardRule)
		require.True(t, isAwkward, "%q", key)
		want := map[string]any{"value": awkward.value, awkward.value: []any{awkward.value, "plain"}}
		if awkward.value == "" {
			want["follows"] = "" // An empty DefaultFrom follows no option.
		}
		assert.Equal(t, want, withoutNulls(settings.Options), "%q", key)
		require.NotNil(t, settings.Severity, "%q", key)
		assert.Equal(t, string(config.SeverityWarning), *settings.Severity, "%q", key)
		assert.Nil(t, settings.AutoFix, "%q: a default adds no setting", key)
		assert.False(t, settings.ResetOptions, "%q: a default adds no setting", key)
	}
}

// assertKeysNameTheirRules checks that keys, the rule keys of a generated
// configuration, hold one key for each rule of registry, and that each key
// names a different rule.
func assertKeysNameTheirRules[V any](t *testing.T, registry *lint.Registry, keys map[string]V, what string) {
	t.Helper()

	assert.Len(t, keys, len(registry.IDs()), "%s: one key per rule", what)
	named := make(map[string]bool, len(keys))
	for key := range keys {
		rule, found := registry.Lookup(key)
		if assert.True(t, found, "%s: %q names a rule", what, key) {
			named[rule.ID()] = true
		}
	}
	assert.Len(t, named, len(registry.IDs()), "%s: each key names another rule", what)
}

// uncommentRuleSettings returns content, a full template, with the
// commented settings of each rule, its severity and options, uncommented.
func uncommentRuleSettings(content string) string {
	var out strings.Builder
	for line := range strings.Lines(content) {
		if rest, ok := strings.CutPrefix(line, "    # "); ok {
			line = "    " + rest
		}
		out.WriteString(line)
	}
	return out.String()
}

// withoutNulls returns options without the options that are null, which
// leave the option unset.
func withoutNulls(options map[string]any) map[string]any {
	kept := make(map[string]any, len(options))
	for name, value := range options {
		if value != nil {
			kept[name] = value
		}
	}
	return kept
}

// TestConfigTemplateMinimalWritesAwkwardNames covers the minimal
// template's example when its rules have names that YAML would misread,
// such as null: uncommented, the example loads, and each key names its
// rule.
func TestConfigTemplateMinimalWritesAwkwardNames(t *testing.T) {
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	for id, name := range map[string]string{"MD001": "null", "MD013": "a: b", "MD033": "line\nbreak"} {
		registry.Register(&quietRule{lint.NewBaseRule(id, name, "", nil, false)})
	}
	content, err := ruledocs.ConfigTemplate(ruledocs.Entries(registry, nil), ruledocs.TemplateOptions{})
	require.NoError(t, err)

	var example strings.Builder
	_, rest, found := strings.Cut(string(content), "\n# rules:\n")
	require.True(t, found)
	example.WriteString("rules:\n")
	for line := range strings.Lines(rest) {
		example.WriteString(strings.TrimPrefix(line, "# "))
	}
	cfg, err := config.FromYAML([]byte(example.String()))
	require.NoError(t, err, example.String())
	named := make(map[string]bool)
	for key := range cfg.Rules {
		rule, found := registry.Lookup(key)
		require.True(t, found, "%q", key)
		named[rule.ID()] = true
	}
	assert.Equal(t, map[string]bool{"MD001": true, "MD013": true, "MD033": true}, named)
}
