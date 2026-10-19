package configloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestLoad_RuleBooleanShorthand covers rules set to true or false, as
// markdownlint allows, by name, alias or ID. They load without warnings,
// and false disables a rule while true enables one.
func TestLoad_RuleBooleanShorthand(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "rules:\n  line-length: false\n  MD033: true\n  ul-style: false\n")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}

	for id, want := range map[string]bool{"MD013": false, "MD033": true, "MD004": false} {
		rule, ok := result.Config.Rules[id]
		if !ok || rule.Enabled == nil {
			t.Errorf("%s: not configured: %+v", id, result.Config.Rules)
			continue
		}
		if *rule.Enabled != want {
			t.Errorf("%s: enabled = %v, want %v", id, *rule.Enabled, want)
		}
	}
}

// TestLoad_RuleBooleanShorthandValidatesLikeTheMapping covers validation of
// the shorthand: an unknown rule is a warning either way, and a value that
// is neither a boolean, a string nor a mapping is an error that names its
// line. A string is a severity, which validates as the severity setting
// does (see TestLoad_RuleSeverityShorthandValidatesLikeTheSetting).
func TestLoad_RuleBooleanShorthandValidatesLikeTheMapping(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"rules:\n  no-such-rule: false\n",
		"rules:\n  no-such-rule:\n    enabled: false\n",
		"rules:\n  no-such-rule: warning\n",
	} {
		result, err := loadProject(t, content)
		if err != nil {
			t.Fatalf("Load(%q) error = %v", content, err)
		}
		if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], `unknown rule "no-such-rule"`) {
			t.Errorf("Load(%q) warnings = %v, want one unknown rule warning", content, result.Warnings)
		}
	}

	for content, line := range map[string]string{
		"rules:\n  MD013: 80\n":                  "line 2",
		"rules:\n  MD013: [off]\n":               "line 2",
		"rules:\n  MD013:\n    enabled: maybe\n": "line 3",
	} {
		_, err := loadProject(t, content)
		if err == nil || !strings.Contains(err.Error(), line) {
			t.Errorf("Load(%q) error = %v, want an error at %s", content, err, line)
		}
	}
}

// TestLoad_RuleBooleanShorthandInJSON covers a JSON config file given with
// --config.
func TestLoad_RuleBooleanShorthandInJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "gomdlint.json")
	if err := os.WriteFile(path, []byte(`{"rules": {"line-length": false, "MD033": true}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := Load(context.Background(), LoadOptions{
		WorkingDir:         dir,
		ExplicitPath:       path,
		IgnoreSystemConfig: true,
		IgnoreUserConfig:   true,
		IgnoreMarkdownlint: true,
		IgnoreEnv:          true,
		NonInteractive:     true,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if rule := result.Config.Rules["MD013"]; rule.Enabled == nil || *rule.Enabled {
		t.Errorf("MD013 = %+v, want disabled", rule)
	}
	if rule := result.Config.Rules["MD033"]; rule.Enabled == nil || !*rule.Enabled {
		t.Errorf("MD033 = %+v, want enabled", rule)
	}
}

// TestLoad_RuleBooleanShorthandKeepsLowerLayerOptions covers a project
// config that turns on a rule a user config configured: true only sets
// enabled, so the user's options still apply, as with "enabled: true".
func TestLoad_RuleBooleanShorthandKeepsLowerLayerOptions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	user := filepath.Join(dir, "user.yaml")
	project := filepath.Join(dir, "project.yml")
	if err := os.WriteFile(user, []byte("rules:\n  MD013:\n    enabled: false\n    options:\n      max: 120\n"), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
	if err := os.WriteFile(project, []byte("rules:\n  line-length: true\n"), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg := mergeFiles(t, user, project)

	rule := cfg.Rules["MD013"]
	if rule.Enabled == nil || !*rule.Enabled {
		t.Errorf("MD013 = %+v, want enabled", rule)
	}
	if rule.Options["max"] != 120 {
		t.Errorf("MD013 options = %v, want the user's max of 120", rule.Options)
	}
}

// TestLoad_RuleSeverityShorthand covers rules set to a severity, as
// markdownlint allows with error and warning, and with gomdlint's info, by
// ID, name or markdownlint name, under rules and rule_defaults, in YAML and
// in JSON. Each turns the rule on at that severity, as markdownlint reads
// it, and loads without warnings.
func TestLoad_RuleSeverityShorthand(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, file, content string }{
		{
			name: "YAML", file: ".gomdlint.yml",
			content: "rules:\n  line-length: warning\n  MD033: info\n  ul-style: error\n" +
				"rule_defaults:\n  no-multiple-blanks: info\n",
		},
		{
			name: "JSON", file: ".gomdlint.json",
			content: `{"rules": {"line-length": "warning", "MD033": "info", "ul-style": "error"},` +
				` "rule_defaults": {"no-multiple-blanks": "info"}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := loadProjectFile(t, tt.file, tt.content)
			require.NoError(t, err)
			assert.Empty(t, result.Warnings)

			severity := func(value string) config.RuleConfig {
				return config.RuleConfig{Enabled: config.Bool(true), Severity: &value}
			}
			assert.Equal(t, map[string]config.RuleConfig{
				"MD013": severity("warning"), "MD033": severity("info"), "MD004": severity("error"),
			}, result.Config.Rules)
			assert.Equal(t, map[string]config.RuleConfig{"MD012": severity("info")}, result.Config.RuleDefaults)
		})
	}
}

// TestLoad_RuleSeverityShorthandValidatesLikeTheSetting covers a rule value
// that is a string but no severity that gomdlint knows. Loading fails with
// the error that the severity setting gets for the string: the validation
// error that names the rule's ID and lists the severities, whichever key
// names the rule.
func TestLoad_RuleSeverityShorthandValidatesLikeTheSetting(t *testing.T) {
	t.Parallel()

	const severities = "; must be one of: error, warning, info"
	for _, tt := range []struct {
		name, shorthand, setting, want string
	}{
		{
			name:      "unknown word",
			shorthand: "rules:\n  MD013: bogus\n",
			setting:   "rules:\n  MD013:\n    severity: bogus\n",
			want:      `rules.MD013.severity: invalid severity "bogus"` + severities,
		},
		{
			name:      "another case",
			shorthand: "rules:\n  line-length: Warning\n",
			setting:   "rules:\n  line-length:\n    severity: Warning\n",
			want:      `rules.MD013.severity: invalid severity "Warning"` + severities,
		},
		{
			name:      "YAML 1.1 boolean",
			shorthand: "rules:\n  ul-style: off\n",
			setting:   "rules:\n  ul-style:\n    severity: off\n",
			want:      `rules.MD004.severity: invalid severity "off"` + severities,
		},
		{
			name:      "quoted boolean",
			shorthand: "rules:\n  MD013: \"false\"\n",
			setting:   "rules:\n  MD013:\n    severity: \"false\"\n",
			want:      `rules.MD013.severity: invalid severity "false"` + severities,
		},
		{
			name:      "empty",
			shorthand: "rules:\n  MD013: ''\n",
			setting:   "rules:\n  MD013:\n    severity: ''\n",
			want:      `rules.MD013.severity: invalid severity ""` + severities,
		},
		{
			name:      "rule_defaults",
			shorthand: "rule_defaults:\n  no-trailing-spaces: fatal\n",
			setting:   "rule_defaults:\n  no-trailing-spaces:\n    severity: fatal\n",
			want:      `rule_defaults.MD009.severity: invalid severity "fatal"` + severities,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, shorthandErr := loadProject(t, tt.shorthand)
			_, settingErr := loadProject(t, tt.setting)

			require.Error(t, settingErr)
			require.Error(t, shorthandErr)
			assert.Equal(t, tt.want, settingErr.Error())
			assert.Equal(t, settingErr.Error(), shorthandErr.Error())

			var shorthandInvalid, settingInvalid *ValidationError
			require.ErrorAs(t, shorthandErr, &shorthandInvalid, "the shorthand fails validation, as the setting does")
			require.ErrorAs(t, settingErr, &settingInvalid)
			assert.Equal(t, settingInvalid, shorthandInvalid)
		})
	}
}

// TestLoad_RuleSeverityShorthandKeepsLowerLayerSettings covers a project
// config that gives a severity to a rule that a user config configures
// and turns off. As in markdownlint, the shorthand turns the rule on at
// that severity, so it merges as enabled: true with the severity does:
// the user's options, here written directly under the rule key and under
// options, still apply, and so does its auto_fix. The severity setting
// alone leaves the rule off. A later layer that sets options leaves the
// shorthand's severity and enabled setting alone in turn.
func TestLoad_RuleSeverityShorthandKeepsLowerLayerSettings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return path
	}
	user := write("user.yaml",
		"rules:\n  MD013:\n    enabled: false\n    auto_fix: false\n    line_length: 120\n    options:\n      tables: false\n")
	shorthand := write("shorthand.yml", "rules:\n  line-length: error\n")
	turnedOn := write("on.yml", "rules:\n  line-length:\n    enabled: true\n    severity: error\n")
	setting := write("setting.yml", "rules:\n  line-length:\n    severity: error\n")

	got := mergeFiles(t, user, shorthand)

	assert.Equal(t, mergeFiles(t, user, turnedOn).Rules, got.Rules, "the shorthand merges as enabled: true with the severity")
	rule := got.Rules["MD013"]
	require.NotNil(t, rule.Severity)
	assert.Equal(t, "error", *rule.Severity)
	assert.Equal(t, map[string]any{"max": 120, "ignore_tables": true}, rule.Options, "the user's options apply")
	require.NotNil(t, rule.AutoFix)
	assert.False(t, *rule.AutoFix, "the user's auto_fix applies")
	require.NotNil(t, rule.Enabled)
	assert.True(t, *rule.Enabled, "the shorthand turns the rule back on")

	longForm := mergeFiles(t, user, setting).Rules["MD013"]
	require.NotNil(t, longForm.Enabled)
	assert.False(t, *longForm.Enabled, "the severity setting alone leaves the rule off")

	lower := write("lower.yml", "rules:\n  MD013: warning\n")
	options := write("options.yml", "rules:\n  MD013:\n    options:\n      max: 100\n")
	layered := mergeFiles(t, lower, options).Rules["MD013"]
	require.NotNil(t, layered.Severity)
	assert.Equal(t, "warning", *layered.Severity, "a later layer's options leave the severity alone")
	require.NotNil(t, layered.Enabled)
	assert.True(t, *layered.Enabled)
	assert.Equal(t, map[string]any{"max": 100}, layered.Options)
}

// mergeFiles loads config files in order of increasing precedence, as Load
// merges its layers.
func mergeFiles(t *testing.T, paths ...string) *config.Config {
	t.Helper()

	var cfg *config.Config
	for _, path := range paths {
		fileCfg, warnings, err := loadConfigFile(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if len(warnings) != 0 {
			t.Errorf("%s: unexpected warnings: %v", path, warnings)
		}
		cfg = merge(cfg, fileCfg)
	}
	return cfg
}
