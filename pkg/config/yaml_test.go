package config_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
)

func TestConfigClone(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		var c *config.Config
		clone := c.Clone()
		assert.Nil(t, clone)
	})

	t.Run("empty config", func(t *testing.T) {
		c := &config.Config{}
		clone := c.Clone()
		require.NotNil(t, clone)
		assert.NotSame(t, c, clone)
	})

	t.Run("deep copies Rules map", func(t *testing.T) {
		enabled := true
		severity := "error"
		original := &config.Config{
			Rules: map[string]config.RuleConfig{
				"MD001": {
					Enabled:  &enabled,
					Severity: &severity,
					Options: map[string]any{
						"style": "dash",
					},
				},
			},
		}

		clone := original.Clone()
		require.NotNil(t, clone)

		// Verify the Rules map is a different instance
		assert.NotSame(t, &original.Rules, &clone.Rules)

		// Verify the rule config values are copied
		require.Contains(t, clone.Rules, "MD001")
		assert.True(t, *clone.Rules["MD001"].Enabled)
		assert.Equal(t, "error", *clone.Rules["MD001"].Severity)

		// Verify modifying clone doesn't affect original
		newSeverity := "warning"
		clone.Rules["MD001"] = config.RuleConfig{Severity: &newSeverity}
		assert.Equal(t, "error", *original.Rules["MD001"].Severity)
	})

	t.Run("deep copies Ignore slice", func(t *testing.T) {
		original := &config.Config{
			Ignore: []string{"*.md", "vendor/**"},
		}

		clone := original.Clone()
		require.NotNil(t, clone)

		// Verify the slice is a different instance
		assert.Equal(t, original.Ignore, clone.Ignore)

		// Verify modifying clone doesn't affect original
		clone.Ignore[0] = "changed"
		assert.Equal(t, "*.md", original.Ignore[0])
	})

	t.Run("preserves all fields", func(t *testing.T) {
		enabled := true
		original := &config.Config{
			Flavor:          config.FlavorGFM,
			SeverityDefault: "warning",
			Rules: map[string]config.RuleConfig{
				"MD001": {Enabled: &enabled},
			},
			Ignore:       []string{"*.bak"},
			Backups:      config.BackupsConfig{Enabled: config.Bool(true), Mode: "sidecar"},
			Fix:          true,
			DryRun:       true,
			Format:       config.FormatJSON,
			RuleFormat:   config.RuleFormatCombined,
			Jobs:         4,
			EnableRules:  []string{"MD001", "MD002"},
			DisableRules: []string{"MD003"},
			FixRules:     []string{"MD001"},
			NoBackups:    true,
		}

		clone := original.Clone()
		require.NotNil(t, clone)

		assert.Equal(t, original.Flavor, clone.Flavor)
		assert.Equal(t, original.SeverityDefault, clone.SeverityDefault)
		assert.Equal(t, original.Backups, clone.Backups)
		assert.Equal(t, original.Fix, clone.Fix)
		assert.Equal(t, original.DryRun, clone.DryRun)
		assert.Equal(t, original.Format, clone.Format)
		assert.Equal(t, original.RuleFormat, clone.RuleFormat)
		assert.Equal(t, original.Jobs, clone.Jobs)
		assert.Equal(t, original.NoBackups, clone.NoBackups)

		// Verify slices are copied
		assert.Equal(t, original.EnableRules, clone.EnableRules)
		assert.Equal(t, original.DisableRules, clone.DisableRules)
		assert.Equal(t, original.FixRules, clone.FixRules)

		// The clone owns its backups setting.
		*clone.Backups.Enabled = false
		assert.True(t, original.Backups.IsEnabled())
	})
}

func TestBackupsConfigIsEnabled(t *testing.T) {
	t.Parallel()

	assert.False(t, config.BackupsConfig{}.IsEnabled(), "unset")
	assert.False(t, config.BackupsConfig{Enabled: config.Bool(false)}.IsEnabled())
	assert.True(t, config.BackupsConfig{Enabled: config.Bool(true)}.IsEnabled())
	assert.True(t, config.NewConfig().Backups.IsEnabled(), "default")
}

func TestBackupsDisabledSurvivesYAML(t *testing.T) {
	t.Parallel()

	cfg, err := config.FromYAML([]byte("backups:\n  enabled: false\n"))
	require.NoError(t, err)
	require.NotNil(t, cfg.Backups.Enabled)
	assert.False(t, cfg.Backups.IsEnabled())

	data, err := cfg.ToYAML()
	require.NoError(t, err)
	assert.Contains(t, string(data), "enabled: false")
}

func TestConfigToYAML(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		var cfg *config.Config
		data, err := cfg.ToYAML()
		require.NoError(t, err)
		assert.Nil(t, data)
	})

	t.Run("basic config serializes", func(t *testing.T) {
		cfg := &config.Config{
			Flavor:          config.FlavorGFM,
			SeverityDefault: "warning",
		}

		data, err := cfg.ToYAML()
		require.NoError(t, err)
		assert.Contains(t, string(data), "flavor: gfm")
		assert.Contains(t, string(data), "severity_default: warning")
	})
}

// TestConfigToJSON covers the JSON form of a configuration: the settings
// ToYAML writes, under the same keys, so the file reads back the same.
func TestConfigToJSON(t *testing.T) {
	enabled := false
	cfg := &config.Config{
		Flavor: config.FlavorCommonMark,
		Ignore: []string{"vendor/**"},
		Rules: map[string]config.RuleConfig{
			"MD013": {Enabled: &enabled, Options: map[string]any{"line_length": 100, "tables": "no"}},
		},
	}

	data, err := cfg.ToJSON()
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed), string(data))
	assert.Equal(t, "commonmark", parsed["flavor"])
	assert.Equal(t, []any{"vendor/**"}, parsed["ignore"])
	rules, ok := parsed["rules"].(map[string]any)
	require.True(t, ok, string(data))
	rule, ok := rules["MD013"].(map[string]any)
	require.True(t, ok, string(data))
	assert.Equal(t, false, rule["enabled"])
	assert.Equal(t, map[string]any{"line_length": float64(100), "tables": "no"}, rule["options"])
	assert.Nil(t, rule["severity"], "an unset severity stays unset")
	assert.True(t, strings.HasSuffix(string(data), "}\n"), "the file ends with a newline")

	t.Run("nil config", func(t *testing.T) {
		var empty *config.Config
		data, err := empty.ToJSON()
		require.NoError(t, err)
		assert.Nil(t, data)
	})

	// People edit config files by hand, so markup stays readable.
	t.Run("keeps markup characters", func(t *testing.T) {
		withMarkup := &config.Config{Ignore: []string{"<draft> & co/**"}}
		data, err := withMarkup.ToJSON()
		require.NoError(t, err)
		assert.Contains(t, string(data), `"<draft> & co/**"`)
	})
}

func TestFromYAML(t *testing.T) {
	t.Run("parses valid YAML", func(t *testing.T) {
		yaml := []byte(`
flavor: gfm
severity_default: error
rules:
  MD001:
    enabled: true
`)
		cfg, err := config.FromYAML(yaml)
		require.NoError(t, err)
		assert.Equal(t, config.FlavorGFM, cfg.Flavor)
		assert.Equal(t, "error", cfg.SeverityDefault)
		require.Contains(t, cfg.Rules, "MD001")
		assert.True(t, *cfg.Rules["MD001"].Enabled)
	})

	t.Run("initializes empty Rules map", func(t *testing.T) {
		yaml := []byte(`flavor: commonmark`)
		cfg, err := config.FromYAML(yaml)
		require.NoError(t, err)
		assert.NotNil(t, cfg.Rules)
	})
}

// TestYAMLToJSONEscapesYAMLLineBreaks covers strings that hold characters
// that YAML reads as line breaks, NEL and the line and paragraph
// separators: the JSON escapes them, so that YAML, which reads JSON too,
// reads it back the same.
func TestYAMLToJSONEscapesYAMLLineBreaks(t *testing.T) {
	t.Parallel()

	content, err := config.YAMLToJSON([]byte("key: \"a\\Lb\\Pc\\Nd\"\n"))
	require.NoError(t, err)

	assert.Contains(t, string(content), `"a\u2028b\u2029c\u0085d"`)
	var read map[string]string
	require.NoError(t, yamlv3.Unmarshal(content, &read), string(content))
	assert.Equal(t, map[string]string{"key": "a\u2028b\u2029c\u0085d"}, read)
}
