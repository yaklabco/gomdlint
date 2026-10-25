package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/reporter"
)

// TestLint_SARIFDescribesRulesWithHelp covers the help that gomdlint lint
// gives each SARIF rule descriptor: the page that explain prints, as text
// and Markdown, and the rule's generated page.
func TestLint_SARIFDescribesRulesWithHelp(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Title\n\nTrailing \n"), 0o644))
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: gfm\n"), 0o644))

	cmd := cli.NewRootCommand(cli.BuildInfo{Version: "9.8.7"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"lint", "--config", cfgFile, "--format", "sarif", "--color", "never", mdFile})

	_ = cmd.Execute() //nolint:errcheck // Findings make Execute fail; only the report matters here.

	var log reporter.SARIFOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &log), "stdout: %s\nstderr: %s", stdout.String(), stderr.String())
	require.Len(t, log.Runs, 1)
	descriptors := log.Runs[0].Tool.Driver.Rules
	require.Len(t, descriptors, 1, "stdout: %s", stdout.String())
	assert.Equal(t, "MD009", descriptors[0].ID)
	require.NotNil(t, descriptors[0].Help)
	assert.True(t, strings.HasPrefix(descriptors[0].Help.Text, "no-trailing-spaces (MD009)"), descriptors[0].Help.Text)
	assert.True(t, strings.HasPrefix(descriptors[0].Help.Markdown, "# no-trailing-spaces\n"), descriptors[0].Help.Markdown)
	assert.Equal(t, "https://github.com/yaklabco/gomdlint/blob/main/docs/rules/no-trailing-spaces.md", descriptors[0].HelpURI)
	assert.Contains(t, descriptors[0].Help.Text, "Options, under rules.no-trailing-spaces.options:",
		"keyed by name, as explain and generated configurations key the rule")
}

// companyPolicyRule is a custom rule that reports the first line of every
// file.
type companyPolicyRule struct {
	lint.BaseRule
}

// Apply reports the first line.
func (r *companyPolicyRule) Apply(*lint.RuleContext) ([]lint.Diagnostic, error) {
	return []lint.Diagnostic{{RuleID: r.ID(), RuleName: r.Name(), Message: "Policy finding", StartLine: 1, StartColumn: 1}}, nil
}

// TestLint_SARIFDescribesACustomRuleUnderABuiltinID covers gomdlint lint
// with a custom rule that replaces MD009 in a registry of the built-in
// rules: its SARIF descriptor describes the custom rule, not the built-in
// one whose catalog entry the registry still holds for MD009.
func TestLint_SARIFDescribesACustomRuleUnderABuiltinID(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	registry.Register(&companyPolicyRule{
		BaseRule: lint.NewBaseRule("MD009", "company-policy", "Company policy.", []string{"policy"}, false),
	})

	tmpDir := t.TempDir()
	mdFile := filepath.Join(tmpDir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Title\n\nText.\n"), 0o644))
	cfgFile := filepath.Join(tmpDir, ".gomdlint.yml")
	require.NoError(t, os.WriteFile(cfgFile, []byte("flavor: gfm\n"), 0o644))

	var stdout, stderr bytes.Buffer
	args := []string{"lint", "--config", cfgFile, "--format", "sarif", "--color", "never", mdFile}
	cli.ExecuteWithRegistry(context.Background(), registry, args, &stdout, &stderr)

	var log reporter.SARIFOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &log), "stdout: %s\nstderr: %s", stdout.String(), stderr.String())
	require.Len(t, log.Runs, 1)
	descriptors := log.Runs[0].Tool.Driver.Rules
	require.Len(t, descriptors, 1, "stdout: %s", stdout.String())
	descriptor := descriptors[0]
	assert.Equal(t, "company-policy", descriptor.Name)
	require.NotNil(t, descriptor.ShortDescription)
	assert.Equal(t, "Company policy.", descriptor.ShortDescription.Text)
	assert.Nil(t, descriptor.Help)
	assert.Empty(t, descriptor.HelpURI)
}
