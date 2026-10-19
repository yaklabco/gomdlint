package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// The rules tests run in a fresh project directory, so that the listing
// cannot pick up a configuration from the environment once the rules
// command reads one.

// TestRulesOutputGolden lists the rules and compares the output with
// testdata/golden/rules.golden.
func TestRulesOutputGolden(t *testing.T) {
	inProject(t, nil)

	got := execute(t, "rules")

	assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
	assert.Empty(t, got.stderr)
	assertGolden(t, testdataPath("golden", "rules.golden"), got.stdout)
}

// TestRulesListShowsCatalogDescriptions covers the DESCRIPTION column of
// the rules listing, which shows the one-line description from each rule's
// catalog entry.
func TestRulesListShowsCatalogDescriptions(t *testing.T) {
	inProject(t, nil)

	got := execute(t, "rules", "--rule-format", "id")
	require.Equal(t, cli.ExitSuccess, got.code, got.stderr)

	lines := strings.Split(strings.TrimSuffix(got.stdout, "\n"), "\n")
	require.True(t, strings.HasPrefix(lines[0], "RULE "), "the listing starts with its header: %q", lines[0])
	rows := make(map[string]string, len(lines)) // rule ID -> row
	for _, line := range lines[1:] {
		id, _, _ := strings.Cut(line, " ")
		rows[id] = line
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
// line runs with.
func TestRulesListDescribesTheRulesOfItsRegistry(t *testing.T) {
	inProject(t, nil)

	registry := lint.NewRegistry()
	described := lint.NewBaseRule("XX001", "described-rule", "The rule's own description", nil, true)
	undescribed := lint.NewBaseRule("XX002", "undescribed-rule", "Only the rule describes it", nil, false)
	blank := lint.NewBaseRule("XX003", "blank-entry-rule", "The entry leaves this to the rule", nil, false)
	registry.Register(&described)
	registry.Register(&undescribed)
	registry.Register(&blank)
	registry.Describe(lint.RuleMeta{ID: "XX001", Name: "described-rule", Description: "Text in the catalog wins."})
	registry.Describe(lint.RuleMeta{ID: "XX003", Name: "blank-entry-rule", Rationale: "Without a description, the rule's own text stays."})

	var stdout, stderr bytes.Buffer
	code := cli.ExecuteWithRegistry(context.Background(), registry, []string{"rules"}, &stdout, &stderr)

	require.Equal(t, cli.ExitSuccess, code, stderr.String())
	assert.Equal(t, ""+
		"RULE              SEVERITY  FIXABLE  DESCRIPTION\n"+
		"described-rule    warning   yes      Text in the catalog wins.\n"+
		"undescribed-rule  warning   no       Only the rule describes it\n"+
		"blank-entry-rule  warning   no       The entry leaves this to the rule\n",
		stdout.String())
}
