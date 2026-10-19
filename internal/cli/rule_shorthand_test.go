package cli_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// shorthandDoc has findings for line-length (MD013), no-inline-html (MD033)
// and no-trailing-spaces (MD009), which are all on by default.
const shorthandDoc = "# Title\n\n" +
	"This line is long enough to go past the default line length limit, whether that limit is eighty " +
	"or one hundred and twenty characters.\n\n" +
	"Some <b>bold</b> text. \n"

// userConfigWithoutHTML is a user config, at the path where inProject's
// XDG_CONFIG_HOME puts it, that turns no-inline-html off, so that "true"
// in a project config has a rule to turn back on.
var userConfigWithoutHTML = map[string]string{".xdg/gomdlint/config.yaml": "rules:\n  no-inline-html: false\n"}

// reportedRules returns the rule IDs that a JSON report names.
func reportedRules(t *testing.T, report string) map[string]bool {
	t.Helper()

	var parsed struct {
		Files []struct {
			Diagnostics []struct {
				RuleID string `json:"ruleId"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	rules := make(map[string]bool)
	for _, file := range parsed.Files {
		for _, diag := range file.Diagnostics {
			rules[diag.RuleID] = true
		}
	}
	return rules
}

// TestLint_RuleBooleanShorthand covers rules set to true or false in a
// gomdlint config, as markdownlint allows: false turns a rule off, true
// turns one on, in YAML and in JSON. Every rule these tests use is on by
// default, so a user config turns no-inline-html off for the project
// config to turn back on.
func TestLint_RuleBooleanShorthand(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		config string
	}{
		{name: "YAML by name", file: ".gomdlint.yml", config: "rules:\n  line-length: false\n  no-inline-html: true\n"},
		{name: "YAML by ID", file: ".gomdlint.yml", config: "rules:\n  MD013: false\n  MD033: true\n"},
		{name: "JSON", file: "gomdlint.json", config: `{"rules": {"line-length": false, "MD033": true}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"doc.md": shorthandDoc, tt.file: tt.config}
			maps.Copy(files, userConfigWithoutHTML)
			inProject(t, files)

			got := execute(t, "lint", "--config", tt.file, "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, map[string]bool{"MD009": true, "MD033": true}, reportedRules(t, got.stdout))
		})
	}

	t.Run("without the config the defaults apply", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc})

		got := execute(t, "lint", "--format", "json", "doc.md")

		assert.Equal(t, map[string]bool{"MD009": true, "MD013": true, "MD033": true}, reportedRules(t, got.stdout))
	})

	t.Run("the user config alone turns no-inline-html off", func(t *testing.T) {
		files := map[string]string{"doc.md": shorthandDoc}
		maps.Copy(files, userConfigWithoutHTML)
		inProject(t, files)

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		assert.Equal(t, map[string]bool{"MD009": true, "MD013": true}, reportedRules(t, got.stdout))
	})
}

// severityDoc has findings for line-length (MD013), no-inline-html (MD033),
// no-trailing-spaces (MD009) and, on lines 7 and 8,
// no-multiple-blank-lines (MD012), which all report warnings by default.
const severityDoc = shorthandDoc + "\n\n\nText after blank lines.\n"

// layeredDoc has two lines longer than line-length's default of 80
// characters: line 3, of 99 characters, and line 5, of 134.
var layeredDoc = "# Title\n\n" + strings.Repeat("word ", 19) + "word\n\n" + strings.Repeat("word ", 26) + "word\n"

// finding is a finding of a JSON report: its rule, line and severity.
type finding struct {
	rule     string
	line     int
	severity string
}

// reportedFindings returns the findings that a JSON report lists.
func reportedFindings(t *testing.T, report string) []finding {
	t.Helper()

	var parsed struct {
		Files []struct {
			Diagnostics []struct {
				RuleID    string `json:"ruleId"`
				StartLine int    `json:"startLine"`
				Severity  string `json:"severity"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	var findings []finding
	for _, file := range parsed.Files {
		for _, diag := range file.Diagnostics {
			findings = append(findings, finding{rule: diag.RuleID, line: diag.StartLine, severity: diag.Severity})
		}
	}
	return findings
}

// ruleSeverities returns the severity of each rule's findings in a JSON
// report, by rule ID.
func ruleSeverities(t *testing.T, report string) map[string]string {
	t.Helper()

	severities := make(map[string]string)
	for _, found := range reportedFindings(t, report) {
		if previous, seen := severities[found.rule]; seen {
			require.Equal(t, previous, found.severity, "every finding of %s has one severity", found.rule)
		}
		severities[found.rule] = found.severity
	}
	return severities
}

// TestLint_RuleSeverityShorthand covers rules set to a severity in a
// gomdlint config, as markdownlint allows with error and warning, and with
// gomdlint's info: the rule's findings take the severity, whichever key
// names the rule, in YAML and in JSON, under rules and under
// rule_defaults. Error findings fail the run; info and warnings do not.
func TestLint_RuleSeverityShorthand(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		config string
		want   map[string]string
		code   int
	}{
		{
			name: "YAML by name", file: ".gomdlint.yml",
			config: "rules:\n  line-length: error\n  no-inline-html: info\n",
			want:   map[string]string{"MD009": "warning", "MD012": "warning", "MD013": "error", "MD033": "info"},
			code:   cli.ExitLintErrors,
		},
		{
			name: "YAML by ID and markdownlint name", file: ".gomdlint.yml",
			config: "rules:\n  MD009: info\n  no-multiple-blanks: info\n  MD013: info\n  MD033: warning\n",
			want:   map[string]string{"MD009": "info", "MD012": "info", "MD013": "info", "MD033": "warning"},
			code:   cli.ExitSuccess,
		},
		{
			name: "JSON", file: "gomdlint.json",
			config: `{"rules": {"no-trailing-spaces": "error", "MD033": "info"}}`,
			want:   map[string]string{"MD009": "error", "MD012": "warning", "MD013": "warning", "MD033": "info"},
			code:   cli.ExitLintErrors,
		},
		{
			name: "rule_defaults", file: ".gomdlint.yml",
			config: "rule_defaults:\n  no-multiple-blank-lines: error\n",
			want:   map[string]string{"MD009": "warning", "MD012": "error", "MD013": "warning", "MD033": "warning"},
			code:   cli.ExitLintErrors,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": severityDoc, tt.file: tt.config})

			got := execute(t, "lint", "--config", tt.file, "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, tt.code, got.code)
			assert.Equal(t, tt.want, ruleSeverities(t, got.stdout))
		})
	}

	t.Run("without the config every rule reports warnings", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": severityDoc})

		got := execute(t, "lint", "--format", "json", "doc.md")

		assert.Equal(t, cli.ExitSuccess, got.code)
		assert.Equal(t, map[string]string{"MD009": "warning", "MD012": "warning", "MD013": "warning", "MD033": "warning"},
			ruleSeverities(t, got.stdout))
	})
}

// TestLint_RuleSeverityShorthandKeepsLowerLayerOptions covers a project
// config that gives line-length a severity, over a user config, or the
// project config's own rule_defaults, that sets its line length to 120.
// The shorthand turns the rule on at that severity, as "enabled: true" and
// "severity: error" do, and keeps the line length of the layer below: of
// layeredDoc's two long lines, only the longer one is reported, as an
// error.
func TestLint_RuleSeverityShorthandKeepsLowerLayerOptions(t *testing.T) {
	const lineLength = "line-length:\n    line_length: 120\n"
	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "user config",
			files: map[string]string{
				".xdg/gomdlint/config.yaml": "rules:\n  " + lineLength,
				".gomdlint.yml":             "rules:\n  line-length: error\n",
			},
		},
		{
			name:  "rule_defaults",
			files: map[string]string{".gomdlint.yml": "rule_defaults:\n  " + lineLength + "rules:\n  line-length: error\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"doc.md": layeredDoc}
			maps.Copy(files, tt.files)
			inProject(t, files)

			got := execute(t, "lint", "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, cli.ExitLintErrors, got.code)
			assert.Equal(t, []finding{{rule: "MD013", line: 5, severity: "error"}}, reportedFindings(t, got.stdout))

			// The same files with the shorthand written out lint alike.
			project := strings.Replace(tt.files[".gomdlint.yml"], "line-length: error",
				"line-length:\n    enabled: true\n    severity: error", 1)
			require.NoError(t, os.WriteFile(".gomdlint.yml", []byte(project), 0o644))
			written := execute(t, "lint", "--format", "json", "doc.md")
			assert.Equal(t, got, written)
		})
	}
}

// TestLint_RuleSeverityShorthandUnderRuleDefaults covers the shorthand
// under rule_defaults, where it is a default like any other: a rule that
// a lower layer turns off under rules stays off. The shorthand under rules
// in turn turns on a rule that a higher layer turns off under
// rule_defaults, as a migrated markdownlint config does for a rule that
// its default: false turns off. Each case also lints with the project
// config alone, which shows that its rule_defaults take effect, so the
// user config is what changes the result.
func TestLint_RuleSeverityShorthandUnderRuleDefaults(t *testing.T) {
	longLines := []finding{{rule: "MD013", line: 3, severity: "error"}, {rule: "MD013", line: 5, severity: "error"}}
	tests := []struct {
		name, user, project string
		want, projectAlone  []finding
	}{
		{
			name:         "a rule that a user config turns off stays off",
			user:         "rules:\n  line-length: false\n",
			project:      "rule_defaults:\n  line-length: error\n",
			projectAlone: longLines,
		},
		{
			name:    "a user config turns on a rule that the project config turns off",
			user:    "rules:\n  line-length: error\n",
			project: "rule_defaults:\n  line-length: false\n",
			want:    longLines,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inProject(t, map[string]string{
				"doc.md":                    layeredDoc,
				".xdg/gomdlint/config.yaml": tt.user,
				".gomdlint.yml":             tt.project,
			})

			got := execute(t, "lint", "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, tt.want, reportedFindings(t, got.stdout))

			require.NoError(t, os.Remove(filepath.Join(".xdg", "gomdlint", "config.yaml")))
			alone := execute(t, "lint", "--format", "json", "doc.md")
			require.Empty(t, alone.stderr)
			assert.Equal(t, tt.projectAlone, reportedFindings(t, alone.stdout), "the project config alone")
		})
	}
}

// TestLint_RuleSeverityShorthandTurnsTheRuleOn covers a severity written as
// a rule's value over settings that turn the rule off: a user config, the
// project config's own rule_defaults, and a user config that also sets
// the line length. As in markdownlint, the shorthand turns the rule on at
// that severity, and the user's line length still applies. The severity
// setting alone leaves the rule off.
func TestLint_RuleSeverityShorthandTurnsTheRuleOn(t *testing.T) {
	longLines := []finding{{rule: "MD013", line: 3, severity: "warning"}, {rule: "MD013", line: 5, severity: "warning"}}
	tests := []struct {
		name     string
		lower    string
		project  string
		longForm string
		want     []finding
	}{
		{
			name:     "over a user config that turns the rule off",
			lower:    "rules:\n  line-length: false\n",
			project:  "rules:\n  line-length: warning\n",
			longForm: "rules:\n  line-length:\n    severity: warning\n",
			want:     longLines,
		},
		{
			name:     "over rule_defaults that turn the rule off",
			project:  "rule_defaults:\n  line-length: false\nrules:\n  line-length: error\n",
			longForm: "rule_defaults:\n  line-length: false\nrules:\n  line-length:\n    severity: error\n",
			want:     []finding{{rule: "MD013", line: 3, severity: "error"}, {rule: "MD013", line: 5, severity: "error"}},
		},
		{
			name:     "keeping the line length of a user config that turns the rule off",
			lower:    "rules:\n  line-length:\n    enabled: false\n    line_length: 120\n",
			project:  "rules:\n  line-length: error\n",
			longForm: "rules:\n  line-length:\n    severity: error\n",
			want:     []finding{{rule: "MD013", line: 5, severity: "error"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"doc.md": layeredDoc, ".gomdlint.yml": tt.project}
			if tt.lower != "" {
				files[".xdg/gomdlint/config.yaml"] = tt.lower
			}
			inProject(t, files)

			got := execute(t, "lint", "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, tt.want, reportedFindings(t, got.stdout))

			require.NoError(t, os.WriteFile(".gomdlint.yml", []byte(tt.longForm), 0o644))
			longForm := execute(t, "lint", "--format", "json", "doc.md")
			require.Empty(t, longForm.stderr)
			assert.Empty(t, reportedFindings(t, longForm.stdout), "the severity setting alone leaves the rule off")
		})
	}
}

// TestLint_InvalidSeverityShorthand covers a rule set to a string that is
// no severity. The run fails as it does for the same string under
// severity, with the same error, which names the rule and lists the
// severities.
func TestLint_InvalidSeverityShorthand(t *testing.T) {
	for _, value := range []string{"bogus", "Warning", "off"} {
		t.Run(value, func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": severityDoc, ".gomdlint.yml": "rules:\n  line-length: " + value + "\n"})
			shorthand := execute(t, "lint", "doc.md")

			inProject(t, map[string]string{"doc.md": severityDoc, ".gomdlint.yml": "rules:\n  line-length:\n    severity: " + value + "\n"})
			setting := execute(t, "lint", "doc.md")

			assert.Equal(t, cli.ExitConfigError, setting.code)
			assert.Equal(t, "Error: load configuration: rules.MD013.severity: invalid severity \""+value+
				"\"; must be one of: error, warning, info\n", setting.stderr)
			assert.Equal(t, setting, shorthand)
		})
	}
}

// TestLint_RejectsKeysRepeatedThroughAliases covers a gomdlint config in
// which a YAML alias writes a key of a mapping a second time: a rule key,
// a rule setting, an option directly under the rule key or an option under
// options. The run fails as it does for the key written out twice, with
// the same error.
func TestLint_RejectsKeysRepeatedThroughAliases(t *testing.T) {
	tests := []struct {
		name, literal, alias string
	}{
		{
			name:    "rule key",
			literal: "rules:\n  line-length: false\n  line-length: true\n",
			alias:   "rules:\n  &rule line-length: false\n  *rule : true\n",
		},
		{
			name:    "rule setting",
			literal: "rules:\n  MD013:\n    enabled: false\n    enabled: true\n",
			alias:   "rules:\n  MD013:\n    &set enabled: false\n    *set : true\n",
		},
		{
			name:    "option under the rule key",
			literal: "rules:\n  MD013:\n    line_length: 200\n    line_length: 20\n",
			alias:   "rules:\n  MD013:\n    &opt line_length: 200\n    *opt : 20\n",
		},
		{
			name:    "option under options",
			literal: "rules:\n  MD013:\n    options:\n      max: 200\n      max: 20\n",
			alias:   "rules:\n  MD013:\n    options:\n      &opt max: 200\n      *opt : 20\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := inProject(t, map[string]string{"doc.md": severityDoc, ".gomdlint.yml": tt.literal})
			literal := transcript(t, execute(t, "lint", "doc.md"), dir)

			dir = inProject(t, map[string]string{"doc.md": severityDoc, ".gomdlint.yml": tt.alias})
			alias := transcript(t, execute(t, "lint", "doc.md"), dir)

			assert.Contains(t, literal, "exit code 65\n")
			assert.Contains(t, literal, "already defined at line")
			assert.Equal(t, literal, alias)
		})
	}
}

// TestRuleShorthandMessagesGolden runs command lines whose gomdlint config
// sets a rule to a string that is no severity, or writes a rule key twice
// through a YAML alias, and compares the transcript of each with
// testdata/golden/<name>.golden.
func TestRuleShorthandMessagesGolden(t *testing.T) {
	skipUnlessSlashPaths(t)

	tests := []struct {
		name   string
		config string
	}{
		{name: "lint-config-severity-shorthand-invalid", config: "rules:\n  line-length: warn\n"},
		{name: "lint-config-rule-repeated-through-alias", config: "rules:\n  &rule line-length: false\n  *rule : error\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := inProject(t, map[string]string{"doc.md": severityDoc, ".gomdlint.yml": tt.config})

			got := execute(t, "lint", "doc.md")

			assertGolden(t, testdataPath("golden", tt.name+".golden"), transcript(t, got, dir))
		})
	}
}

// TestLint_RuleShorthandAgreesWithMigration covers the same shorthand in a
// markdownlint config: applied for the run, migrated with "gomdlint
// migrate", or written in a gomdlint config, it turns the same rules on and
// off.
func TestLint_RuleShorthandAgreesWithMigration(t *testing.T) {
	const shorthand = "line-length: false\nno-inline-html: true\n"
	ruleStates := func(rules map[string]bool) [2]bool { return [2]bool{rules["MD013"], rules["MD033"]} }
	want := [2]bool{false, true}

	t.Run("markdownlint config applied for the run", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc, ".markdownlint.yaml": shorthand})

		got := execute(t, "lint", "--format", "json", "doc.md")

		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})

	t.Run("migrated config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc, ".markdownlint.yaml": shorthand})

		migrated := execute(t, "migrate", ".markdownlint.yaml", "--output", "migrated.yml")
		require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
		got := execute(t, "lint", "--config", "migrated.yml", "--format", "json", "doc.md")

		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})

	// A .json config must hold JSON, so migrate writes JSON for one.
	t.Run("config migrated to a JSON file", func(t *testing.T) {
		dir := inProject(t, map[string]string{"doc.md": shorthandDoc, ".markdownlint.yaml": shorthand})

		migrated := execute(t, "migrate", ".markdownlint.yaml", "--output", ".gomdlint.json")
		require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
		assert.True(t, json.Valid([]byte(readFile(t, filepath.Join(dir, ".gomdlint.json")))), "migrate writes JSON")
		// As migrate advises, the markdownlint file goes, so only the
		// JSON file can set the rules.
		require.NoError(t, os.Remove(filepath.Join(dir, ".markdownlint.yaml")))
		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr, "lint reads the JSON file without complaint")
		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})

	t.Run("gomdlint config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": shorthandDoc, ".gomdlint.yml": "rules:\n  line-length: false\n  no-inline-html: true\n"})

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		assert.Equal(t, want, ruleStates(reportedRules(t, got.stdout)))
	})
}
