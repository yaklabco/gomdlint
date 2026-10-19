package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// The rules and explain tests run in a fresh project directory, so that
// the listing cannot pick up a configuration from the environment.

// inRulesProject runs the test from a copy of testdata/rules-project, whose
// configuration turns rules off and changes severities, and returns its
// directory.
func inRulesProject(t *testing.T) string {
	t.Helper()

	return inProject(t, readProject(t, testdataPath("rules-project")))
}

// TestRulesGolden lists the rules of the fixture project with each filter,
// rule format and output format, and compares the transcript of each with
// testdata/golden/<name>.golden.
func TestRulesGolden(t *testing.T) {
	skipUnlessSlashPaths(t)

	tests := []struct {
		name string
		args []string
	}{
		// Every rule, by category, with its status under the project's
		// configuration.
		{name: "rules"},
		{name: "rules-defaults", args: []string{"--defaults"}},
		{name: "rules-rule-format-id", args: []string{"--rule-format", "id", "--category", "lists"}},
		{name: "rules-rule-format-combined", args: []string{"--rule-format", "combined", "--category", "lists"}},
		// Words, each of which a rule must hold somewhere, in any case.
		{name: "rules-query", args: []string{"table"}},
		{name: "rules-query-words", args: []string{"heading", "BLANK"}},
		{name: "rules-query-alias", args: []string{"ul-style"}},
		// Repeated, or in a comma-separated list, a filter matches any of
		// its values.
		{name: "rules-category", args: []string{"--category", "blockquotes", "--category", "Emphasis"}},
		{name: "rules-category-list", args: []string{"--category", "html, spelling"}},
		{name: "rules-tag", args: []string{"--tag", "whitespace"}},
		{name: "rules-tag-list", args: []string{"--tag", "hr, Line_Length"}},
		{name: "rules-fixable", args: []string{"--fixable"}},
		{name: "rules-enabled", args: []string{"--enabled", "--category", "layout"}},
		{name: "rules-disabled", args: []string{"--disabled"}},
		{name: "rules-disabled-defaults", args: []string{"--disabled", "--defaults"}},
		// Different filters must all match.
		{name: "rules-filters-combine", args: []string{"--tag", "links", "--fixable", "url"}},
		{name: "rules-no-match", args: []string{"--category", "html", "--fixable"}},
		{name: "rules-json", args: []string{"--format", "json", "--category", "layout"}},
		{name: "rules-markdown", args: []string{"--format", "markdown"}},
		{name: "rules-markdown-no-match", args: []string{"--format", "markdown", "frobnicate"}},
		{name: "rules-unknown-category", args: []string{"--category", "heading"}},
		{name: "rules-category-is-tag", args: []string{"--category", "table"}},
		{name: "rules-unknown-tag", args: []string{"--tag", "whitepsace"}},
		{name: "rules-tag-is-category", args: []string{"--tag", "lists"}},
		{name: "rules-enabled-and-disabled", args: []string{"--enabled", "--disabled"}},
		{name: "rules-invalid-format", args: []string{"--format", "sarif"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := inRulesProject(t)

			got := execute(t, append([]string{"rules", "--color", "never"}, tt.args...)...)

			assertGolden(t, testdataPath("golden", tt.name+".golden"), transcript(t, got, dir))
		})
	}
}

// TestRulesReadTheConfigFileNamed covers --config, which names the file
// whose settings the listing shows instead of the project's.
func TestRulesReadTheConfigFileNamed(t *testing.T) {
	inProject(t, map[string]string{
		".gomdlint.yml": "rules:\n  line-length: false\n",
		"other.yml":     "rules:\n  no-hard-tabs: false\n",
	})

	got := execute(t, "rules", "--color", "never", "--disabled", "--config", "other.yml")

	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Contains(t, got.stdout, "no-hard-tabs ")
	assert.NotContains(t, got.stdout, "line-length", "the project's configuration is not read")
	assert.Contains(t, got.stdout, "1 of 55 rules, 0 on")
}

// TestRulesReportAConfigurationThatDoesNotLoad covers a project whose
// configuration lint could not load: the listing fails in the same way,
// unless --defaults asks for the built-in settings.
func TestRulesReportAConfigurationThatDoesNotLoad(t *testing.T) {
	inProject(t, map[string]string{".gomdlint.yml": "rules:\n  MD013: false\n  line-length: false\n"})

	got := execute(t, "rules", "--color", "never")

	assert.Equal(t, cli.ExitConfigError, got.code)
	assert.Contains(t, got.stderr, `"MD013" and "line-length" both configure line-length (MD013)`)
	assert.Empty(t, got.stdout)

	got = execute(t, "rules", "--color", "never", "--defaults")

	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Contains(t, got.stdout, "55 rules, 55 on")
}

// TestRulesListShowsCatalogDescriptions covers the DESCRIPTION column of
// the rules listing, which shows the one-line description from each rule's
// catalog entry.
func TestRulesListShowsCatalogDescriptions(t *testing.T) {
	inProject(t, nil)

	got := execute(t, "rules", "--color", "never", "--rule-format", "id")
	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)

	lines := strings.Split(strings.TrimSuffix(got.stdout, "\n"), "\n")
	require.True(t, strings.HasPrefix(lines[0], "ID "), "the listing starts with its header: %q", lines[0])
	rows := make(map[string]string, len(lines)) // rule ID -> row
	for _, line := range lines[1:] {
		if row, isRule := strings.CutPrefix(line, "  "); isRule {
			id, _, _ := strings.Cut(row, " ")
			rows[id] = line
		}
	}

	rules := lint.DefaultRegistry.Rules()
	assert.Len(t, rows, len(rules), "one row per rule")
	for _, rule := range rules {
		meta, ok := lint.DefaultRegistry.Meta(rule.ID())
		require.True(t, ok, "%s has a catalog entry", rule.ID())
		require.NotEmpty(t, meta.Description, "%s has a description", rule.ID())
		assert.True(t, strings.HasSuffix(rows[rule.ID()], "  "+meta.Description),
			"%s: the row ends with the catalog description %q: %q", rule.ID(), meta.Description, rows[rule.ID()])
	}
}

// TestRulesListDescribesTheRulesOfItsRegistry covers where the rules
// listing finds each description: in the rule's catalog entry, or, for a
// rule without an entry or with an entry that has no description, in the
// rule itself. The listing shows the rules of the registry the command
// line runs with. Rules without a category come last.
func TestRulesListDescribesTheRulesOfItsRegistry(t *testing.T) {
	inProject(t, nil)

	registry := lint.NewRegistry()
	described := lint.NewBaseRule("XX001", "described-rule", "The rule's own description", nil, true)
	undescribed := lint.NewBaseRule("XX002", "undescribed-rule", "Only the rule describes it", nil, false)
	blank := lint.NewBaseRule("XX003", "blank-entry-rule", "The entry leaves this to the rule", nil, false)
	registry.Register(&described)
	registry.Register(&undescribed)
	registry.Register(&blank)
	registry.Describe(lint.RuleMeta{
		ID: "XX001", Name: "described-rule", Category: lint.CategoryLists, Description: "Text in the catalog wins.",
	})
	registry.Describe(lint.RuleMeta{ID: "XX003", Name: "blank-entry-rule", Rationale: "Without a description, the rule's own text stays."})

	var stdout, stderr bytes.Buffer
	code := cli.ExecuteWithRegistry(context.Background(), registry, []string{"rules", "--color", "never"}, &stdout, &stderr)

	require.Equal(t, cli.ExitSuccess, code, stderr.String())
	assert.Equal(t, ""+
		"NAME                ID     FIX  STATUS   DESCRIPTION\n"+
		"\n"+
		"Lists\n"+
		"  described-rule    XX001  yes  warning  Text in the catalog wins.\n"+
		"\n"+
		"Other\n"+
		"  blank-entry-rule  XX003  no   warning  The entry leaves this to the rule\n"+
		"  undescribed-rule  XX002  no   warning  Only the rule describes it\n"+
		"\n"+
		"3 rules, 3 on, 1 fixable. Run 'gomdlint explain <rule>' for details.\n",
		stdout.String())
}

// rulesJSON is the JSON output of the rules command.
type rulesJSON struct {
	Version string `json:"version"`
	Rules   []struct {
		ID              string   `json:"id"`
		Name            string   `json:"name"`
		Aliases         []string `json:"aliases"`
		Category        string   `json:"category"`
		Tags            []string `json:"tags"`
		Description     string   `json:"description"`
		Rationale       string   `json:"rationale"`
		Fix             string   `json:"fix"`
		Fixable         bool     `json:"fixable"`
		Enabled         bool     `json:"enabled"`
		Severity        string   `json:"severity"`
		DefaultEnabled  bool     `json:"defaultEnabled"`
		DefaultSeverity string   `json:"defaultSeverity"`
		Options         []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Default any    `json:"default"`
		} `json:"options"`
	} `json:"rules"`
}

// TestRulesJSONDescribesEveryRule covers rules --format json, which
// describes every rule from its catalog entry, with each option the rule
// reads and its default, in the order of the text listing: by category,
// then by name.
func TestRulesJSONDescribesEveryRule(t *testing.T) {
	inRulesProject(t)

	got := execute(t, "rules", "--format", "json")
	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)

	var output rulesJSON
	require.NoError(t, json.Unmarshal([]byte(got.stdout), &output))
	assert.Equal(t, "1.0.0", output.Version)
	require.Len(t, output.Rules, len(lint.DefaultRegistry.Rules()))

	order := lint.Categories()
	for idx, rule := range output.Rules {
		meta, ok := lint.DefaultRegistry.Meta(rule.ID)
		require.True(t, ok, "%s is a registered rule with a catalog entry", rule.ID)
		assert.Equal(t, meta.Name, rule.Name)
		assert.Equal(t, string(meta.Category), rule.Category, "%s", rule.ID)
		assert.Equal(t, meta.Description, rule.Description, "%s", rule.ID)
		assert.Equal(t, meta.Rationale, rule.Rationale, "%s", rule.ID)
		assert.Equal(t, meta.Fix, rule.Fix, "%s", rule.ID)
		assert.Equal(t, meta.Fixable, rule.Fixable, "%s", rule.ID)
		assert.Equal(t, meta.Tags, rule.Tags, "%s", rule.ID)
		assert.Equal(t, append([]string{}, meta.Aliases...), rule.Aliases, "%s", rule.ID)
		require.Len(t, rule.Options, len(meta.Options), "%s", rule.ID)
		for optionIdx, option := range rule.Options {
			spec := meta.Options[optionIdx]
			assert.Equal(t, spec.Name, option.Name, "%s", rule.ID)
			assert.Equal(t, string(spec.Type), option.Type, "%s %s", rule.ID, spec.Name)
			want, err := json.Marshal(spec.Default)
			require.NoError(t, err)
			have, err := json.Marshal(option.Default)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(have), "%s %s default", rule.ID, spec.Name)
		}

		if idx > 0 {
			previous := output.Rules[idx-1]
			here, before := slices.Index(order, lint.Category(rule.Category)), slices.Index(order, lint.Category(previous.Category))
			assert.True(t, before < here || (before == here && previous.Name < rule.Name),
				"%s (%s) follows %s (%s)", rule.Name, rule.Category, previous.Name, previous.Category)
		}
	}

	states := make(map[string][2]any)
	for _, rule := range output.Rules {
		states[rule.Name] = [2]any{rule.Enabled, rule.Severity}
		assert.True(t, rule.DefaultEnabled, "%s is on by default", rule.Name)
	}
	assert.Equal(t, [2]any{false, "warning"}, states["line-length"], "off, with the severity it would report at")
	assert.Equal(t, [2]any{true, "error"}, states["no-trailing-spaces"])
	assert.Equal(t, [2]any{true, "info"}, states["unordered-list-style"])
}

// TestRulesRuleFormatFromEnvironment covers GOMDLINT_RULE_FORMAT, which
// sets how the text listing names rules when --rule-format is not given,
// as for lint. The flag wins over the variable, and an invalid value is a
// configuration error.
func TestRulesRuleFormatFromEnvironment(t *testing.T) {
	for _, defaults := range []bool{false, true} {
		args := []string{"rules", "--color", "never", "--category", "lists"}
		if defaults {
			args = append(args, "--defaults")
		}

		inRulesProject(t)
		t.Setenv("GOMDLINT_RULE_FORMAT", "id")

		got := execute(t, args...)
		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.True(t, strings.HasPrefix(got.stdout, "ID "), "IDs first, --defaults %v: %q", defaults, got.stdout)

		got = execute(t, append(args, "--rule-format", "name")...)
		require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
		assert.True(t, strings.HasPrefix(got.stdout, "NAME "), "the flag wins, --defaults %v: %q", defaults, got.stdout)

		t.Setenv("GOMDLINT_RULE_FORMAT", "ids")
		got = execute(t, args...)
		assert.Equal(t, cli.ExitConfigError, got.code, "--defaults %v", defaults)
		assert.Contains(t, got.stderr, `invalid rule format "ids"; must be one of: name, id, combined`)
		assert.Empty(t, got.stdout)

		got = execute(t, append(args, "--rule-format", "combined")...)
		require.Equal(t, cli.ExitSuccess, got.code, "the flag wins over an invalid value, --defaults %v: %s", defaults, got.stderr)
		assert.True(t, strings.HasPrefix(got.stdout, "RULE "), "--defaults %v: %q", defaults, got.stdout)
	}
}
