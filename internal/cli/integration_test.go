package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jamesainslie/gomdlint/internal/cli"
)

// testMarkdownWithTrailingSpaces is a test markdown file with trailing spaces on line 1.
// This triggers MD009/no-trailing-spaces rule.
const testMarkdownWithTrailingSpaces = "# Hello World   \n\nSome text.\n"

// TestIntegration_RuleFormatFlag tests the --rule-format flag with different formats.
func TestIntegration_RuleFormatFlag(t *testing.T) {
	t.Parallel()

	// Create a temp markdown file with trailing spaces (triggers MD009/no-trailing-spaces)
	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "test.md")
	// Content with trailing spaces on line 1
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	tests := []struct {
		name           string
		ruleFormat     string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:           "format name shows rule name only",
			ruleFormat:     "name",
			wantContains:   []string{"no-trailing-spaces"},
			wantNotContain: []string{"MD009/"},
		},
		{
			name:           "format id shows rule ID only",
			ruleFormat:     "id",
			wantContains:   []string{"MD009"},
			wantNotContain: []string{"no-trailing-spaces"},
		},
		{
			name:           "format combined shows both ID and name",
			ruleFormat:     "combined",
			wantContains:   []string{"MD009/no-trailing-spaces"},
			wantNotContain: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := cli.NewRootCommand(info)

			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			// Create a minimal config to override the project config
			cfgDir := t.TempDir()
			cfgFile := filepath.Join(cfgDir, ".gomdlint.yml")
			require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

			cmd.SetArgs([]string{
				"lint",
				"--config", cfgFile,
				"--rule-format", tt.ruleFormat,
				"--no-context",
				"--color", "never",
				mdFile,
			})

			_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues //nolint:errcheck // Ignore error - we expect lint issues

			output := stdout.String() + stderr.String()

			for _, want := range tt.wantContains {
				assert.Contains(t, output, want,
					"output should contain %q for rule-format=%s", want, tt.ruleFormat)
			}

			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, output, notWant,
					"output should not contain %q for rule-format=%s", notWant, tt.ruleFormat)
			}
		})
	}
}

// TestIntegration_ConfigWithRuleNames tests that config files can use rule names.
func TestIntegration_ConfigWithRuleNames(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create config file using rule name to disable the rule
	configContent := `
rules:
  no-trailing-spaces:
    enabled: false
`
	configFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", configFile,
		"--no-context",
		"--color", "never",
		mdFile,
	})

	err := cmd.Execute()

	output := stdout.String() + stderr.String()

	// The rule should be disabled, so no trailing-spaces error
	assert.NotContains(t, output, "no-trailing-spaces",
		"disabled rule should not appear in output")
	assert.NotContains(t, output, "MD009",
		"disabled rule should not appear in output")

	// If there are no other issues, command should succeed
	// Note: there might be other rules that trigger, so we just check
	// the specific rule is disabled
	_ = err // Command may or may not error depending on other rules
}

// TestIntegration_ConfigWithRuleID tests that config files still work with rule IDs.
func TestIntegration_ConfigWithRuleID(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create config file using rule ID to disable the rule
	configContent := `
rules:
  MD009:
    enabled: false
`
	configFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", configFile,
		"--no-context",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	output := stdout.String() + stderr.String()

	// The rule should be disabled, so no trailing-spaces error
	assert.NotContains(t, output, "no-trailing-spaces",
		"disabled rule should not appear in output")
	assert.NotContains(t, output, "MD009",
		"disabled rule should not appear in output")
}

// TestIntegration_DuplicateRuleWarning tests that duplicate rule configs emit a warning.
// The warning is emitted via the logging system when debug is enabled.
func TestIntegration_DuplicateRuleWarning(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file
	mdFile := filepath.Join(tmpDir, "test.md")
	content := "# Hello World\n\nSome text.\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create config file with both ID and name for the same rule
	configContent := `
rules:
  MD009:
    enabled: false
  no-trailing-spaces:
    enabled: true
`
	configFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", configFile,
		"--no-context",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	// The duplicate warning is already tested in configloader/loader_test.go
	// Here we just verify the config is loaded and last value wins (enabled: true)
	// Since no-trailing-spaces is enabled (last value), we should see it in output
	// if there were trailing spaces. Our test file has no trailing spaces, so no output.
	// This test primarily verifies that the duplicate config doesn't cause an error.
	output := stdout.String() + stderr.String()
	assert.NotContains(t, output, "error loading", "config with duplicate rules should load without error")
}

// TestIntegration_RulesCommandWithFormat tests that the rules command accepts --rule-format flag.
// Note: The rules command outputs to os.Stdout via logging, which is difficult to capture
// in tests. We verify the command runs without error with each format.
func TestIntegration_RulesCommandWithFormat(t *testing.T) {
	t.Parallel()

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	tests := []struct {
		name       string
		ruleFormat string
	}{
		{name: "format name", ruleFormat: "name"},
		{name: "format id", ruleFormat: "id"},
		{name: "format combined", ruleFormat: "combined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := cli.NewRootCommand(info)

			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{
				"rules",
				"--rule-format", tt.ruleFormat,
			})

			err := cmd.Execute()
			require.NoError(t, err, "rules command should succeed with --rule-format=%s", tt.ruleFormat)
		})
	}
}

// TestIntegration_DefaultRuleFormat tests that the default rule format is "name".
func TestIntegration_DefaultRuleFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create a minimal config to override the project config
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", cfgFile,
		"--no-context",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	output := stdout.String() + stderr.String()

	// Default should show rule name, not ID
	assert.Contains(t, output, "no-trailing-spaces",
		"default format should show rule name")
}

// TestIntegration_JSONOutputIncludesBothIDAndName tests that JSON output includes both.
func TestIntegration_JSONOutputIncludesBothIDAndName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create a minimal config to override the project config
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", cfgFile,
		"--format", "json",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	output := stdout.String()

	// JSON should include both ruleId and ruleName
	assert.Contains(t, output, `"ruleId"`,
		"JSON output should include ruleId field")
	assert.Contains(t, output, `"ruleName"`,
		"JSON output should include ruleName field")
	assert.Contains(t, output, `"MD009"`,
		"JSON output should include the rule ID value")
	assert.Contains(t, output, `"no-trailing-spaces"`,
		"JSON output should include the rule name value")
}

// TestIntegration_EnableDisableByID tests --enable and --disable flags with rule IDs.
// Note: The --enable/--disable flags currently only support rule IDs, not names.
// Rule name support would require updating pkg/lint/resolve.go's ResolveRules function.
func TestIntegration_EnableDisableByID(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	// Create a minimal config to override the project config
	cfgDir := t.TempDir()
	cfgFile := filepath.Join(cfgDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

	// Test --disable with rule ID
	t.Run("disable by ID", func(t *testing.T) {
		t.Parallel()

		cmd := cli.NewRootCommand(info)

		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{
			"lint",
			"--config", cfgFile,
			"--disable", "MD009",
			"--no-context",
			"--color", "never",
			mdFile,
		})

		_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

		output := stdout.String() + stderr.String()

		// Rule should be disabled
		assert.NotContains(t, output, "no-trailing-spaces",
			"disabled rule should not appear in output")
		assert.NotContains(t, output, "MD009",
			"disabled rule should not appear in output")
	})
}

// TestIntegration_MixedRuleFormatsInConfig tests config with mixed ID and name references.
func TestIntegration_MixedRuleFormatsInConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with issues
	mdFile := filepath.Join(tmpDir, "test.md")
	// This has trailing spaces (MD009) and hard tabs (MD010)
	content := "# Hello World   \n\n\tSome text with tab.\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create config file using mix of IDs and names
	configContent := `
rules:
  MD009:
    enabled: false
  no-hard-tabs:
    enabled: false
`
	configFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", configFile,
		"--no-context",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	output := stdout.String() + stderr.String()

	// Both rules should be disabled
	assert.NotContains(t, output, "no-trailing-spaces",
		"MD009 should be disabled")
	assert.NotContains(t, output, "MD009",
		"MD009 should be disabled")
	assert.NotContains(t, output, "no-hard-tabs",
		"MD010 should be disabled")
	assert.NotContains(t, output, "MD010",
		"MD010 should be disabled")
}

// TestIntegration_SummaryFormat tests that --format summary produces expected output.
func TestIntegration_SummaryFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create a minimal config to override the project config
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", cfgFile,
		"--format", "summary",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	output := stdout.String() + stderr.String()

	// Verify summary format output contains expected sections
	assert.Contains(t, output, "Rules Summary",
		"summary format should show Rules Summary table")
	assert.Contains(t, output, "Files Summary",
		"summary format should show Files Summary table")
	assert.Contains(t, output, "Total:",
		"summary format should show Total line")
}

// TestIntegration_SummaryFormatRulesFirst tests that default order shows rules first.
func TestIntegration_SummaryFormatRulesFirst(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create a minimal config to override the project config
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", cfgFile,
		"--format", "summary",
		"--summary-order", "rules",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	output := stdout.String() + stderr.String()

	// Verify Rules Summary appears before Files Summary
	rulesIdx := strings.Index(output, "Rules Summary")
	filesIdx := strings.Index(output, "Files Summary")

	assert.Greater(t, rulesIdx, -1, "output should contain Rules Summary")
	assert.Greater(t, filesIdx, -1, "output should contain Files Summary")
	assert.Less(t, rulesIdx, filesIdx,
		"with --summary-order rules, Rules Summary should appear before Files Summary")
}

// TestIntegration_SummaryFormatFilesFirst tests that --summary-order files shows files first.
func TestIntegration_SummaryFormatFilesFirst(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a markdown file with trailing spaces
	mdFile := filepath.Join(tmpDir, "test.md")
	content := testMarkdownWithTrailingSpaces
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create a minimal config to override the project config
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", cfgFile,
		"--format", "summary",
		"--summary-order", "files",
		"--color", "never",
		mdFile,
	})

	_ = cmd.Execute() //nolint:errcheck // Ignore error - we expect lint issues

	output := stdout.String() + stderr.String()

	// Verify Files Summary appears before Rules Summary
	rulesIdx := strings.Index(output, "Rules Summary")
	filesIdx := strings.Index(output, "Files Summary")

	assert.Greater(t, rulesIdx, -1, "output should contain Rules Summary")
	assert.Greater(t, filesIdx, -1, "output should contain Files Summary")
	assert.Less(t, filesIdx, rulesIdx,
		"with --summary-order files, Files Summary should appear before Rules Summary")
}

// TestIntegration_SummaryFormatNoIssues tests that summary format with no issues shows clean output.
func TestIntegration_SummaryFormatNoIssues(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create a clean markdown file (no issues)
	mdFile := filepath.Join(tmpDir, "clean.md")
	content := "# Hello World\n\nSome text without any issues.\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0644))

	// Create a minimal config to override the project config
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: commonmark\n"), 0644))

	info := cli.BuildInfo{
		Version: "test",
		Commit:  "test",
		Date:    "test",
	}

	cmd := cli.NewRootCommand(info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{
		"lint",
		"--config", cfgFile,
		"--format", "summary",
		"--color", "never",
		mdFile,
	})

	err := cmd.Execute()

	output := stdout.String() + stderr.String()

	// With no issues, command should succeed
	require.NoError(t, err, "lint command should succeed with no issues")

	// Verify clean output message
	assert.Contains(t, output, "No issues found",
		"summary format should show 'No issues found' when there are no issues")

	// Should NOT show the summary tables since there are no issues
	assert.NotContains(t, output, "Rules Summary",
		"summary format should not show Rules Summary when there are no issues")
	assert.NotContains(t, output, "Files Summary",
		"summary format should not show Files Summary when there are no issues")
}
