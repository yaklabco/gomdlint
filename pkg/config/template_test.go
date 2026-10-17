package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// withRuleInfo makes the templates of a test document rules. The provider
// is a package variable, so tests that set it do not run in parallel.
func withRuleInfo(t *testing.T, rules []RuleInfo) {
	t.Helper()

	previous := DefaultRuleInfoProvider
	DefaultRuleInfoProvider = func() []RuleInfo { return rules }
	t.Cleanup(func() { DefaultRuleInfoProvider = previous })
}

func generate(t *testing.T, opts TemplateOptions) string {
	t.Helper()

	content, err := GenerateTemplate(opts)
	require.NoError(t, err)
	return string(content)
}

var templateRules = []RuleInfo{
	{
		ID: "MD001", Name: "heading-increment", Description: "Heading levels go down one step at a time",
		Enabled: true, Severity: SeverityWarning, Tags: []string{"headings"},
	},
	{
		ID: "MD033", Name: "no-inline-html", Description: "Inline HTML",
		Enabled: false, Severity: SeverityError, CanFix: true,
	},
}

// TestGenerateTemplate_FullDocumentsProvidedRules covers the full
// templates: every provided rule with its default enabled state, and its
// default severity only as a comment in YAML, since a severity in the file
// would override severity_default.
func TestGenerateTemplate_FullDocumentsProvidedRules(t *testing.T) {
	withRuleInfo(t, templateRules)

	content := generate(t, TemplateOptions{Format: "yaml", Full: true})
	assert.Contains(t, content, "\n  # MD001: heading-increment\n"+
		"  # Heading levels go down one step at a time\n"+
		"  # Tags: headings\n"+
		"  MD001:\n"+
		"    enabled: true\n"+
		"    # severity: warning\n")
	assert.Contains(t, content, "\n  # MD033: no-inline-html\n"+
		"  # Inline HTML\n"+
		"  # Auto-fix: yes\n"+
		"  MD033:\n"+
		"    enabled: false\n"+
		"    # severity: error\n")

	var parsed struct {
		Rules map[string]map[string]any `yaml:"rules"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(content), &parsed))
	assert.Equal(t, map[string]map[string]any{
		"MD001": {"enabled": true},
		"MD033": {"enabled": false},
	}, parsed.Rules)

	jsonContent := generate(t, TemplateOptions{Format: "json", Full: true})
	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonContent), &settings), jsonContent)
	assert.Equal(t, map[string]any{
		"flavor":          "gfm",
		"backups":         map[string]any{"enabled": true, "mode": "sidecar"},
		"gitignore":       true,
		"default_ignores": true,
		"rules": map[string]any{
			"MD001": map[string]any{"enabled": true},
			"MD033": map[string]any{"enabled": false},
		},
	}, settings, "the JSON template holds the settings the YAML template sets")
	assert.True(t, strings.HasSuffix(jsonContent, "}\n"), "the file ends with a newline")
}

// TestGenerateTemplate_MinimalJSON covers the minimal JSON template, which
// sets what the minimal YAML template sets and no rules.
func TestGenerateTemplate_MinimalJSON(t *testing.T) {
	withRuleInfo(t, templateRules)

	var settings map[string]any
	content := generate(t, TemplateOptions{Format: "json"})
	require.NoError(t, json.Unmarshal([]byte(content), &settings), content)

	assert.Equal(t, map[string]any{"flavor": "gfm"}, settings)
}

// TestGenerateTemplate_FullWithoutRuleInfo covers a program that generates
// a full template without the lint package, which provides the rules: the
// template lists no rules rather than a stale list.
func TestGenerateTemplate_FullWithoutRuleInfo(t *testing.T) {
	withRuleInfo(t, nil)
	DefaultRuleInfoProvider = nil

	content := generate(t, TemplateOptions{Format: "yaml", Full: true})

	assert.Contains(t, content, "\nrules: {}\n")
	assert.NotContains(t, content, "MD0")

	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(generate(t, TemplateOptions{Format: "json", Full: true})), &settings))
	assert.Equal(t, map[string]any{}, settings["rules"])
}
