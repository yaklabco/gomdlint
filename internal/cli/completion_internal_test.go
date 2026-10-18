package cli

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	_ "github.com/yaklabco/gomdlint/pkg/lint/rules" // Register built-in rules
)

// completedValues returns the values that completeRules offers for word,
// without their descriptions.
func completedValues(t *testing.T, registry *lint.Registry, word string) []string {
	t.Helper()

	completions, directive := completeRules(registry)(nil, nil, word)
	require.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	values := make([]string, len(completions))
	for idx, completion := range completions {
		values[idx], _, _ = strings.Cut(completion, "\t")
	}
	return values
}

// TestCompleteRulesOffersWhatTheFlagsAccept checks that completion offers
// every rule ID, name and alias, and every tag, of the built-in rules, and
// that the rule flags accept every value it offers.
func TestCompleteRulesOffersWhatTheFlagsAccept(t *testing.T) {
	registry := lint.DefaultRegistry

	want := make(map[string]bool)
	for _, rule := range registry.Rules() {
		meta, ok := registry.Meta(rule.ID())
		require.True(t, ok, "%s has a catalog entry", rule.ID())
		for _, word := range slices.Concat([]string{rule.ID(), rule.Name()}, meta.Aliases, meta.Tags) {
			want[word] = true
		}
	}

	// IDs and aliases come once a word starts them, so ask for every first
	// character too.
	offered := make(map[string]bool)
	for word := range want {
		for _, prefix := range []string{"", word[:1]} {
			for _, value := range completedValues(t, registry, prefix) {
				offered[value] = true
			}
		}
	}

	assert.Equal(t, slices.Sorted(maps.Keys(want)), slices.Sorted(maps.Keys(offered)))

	ids, problems := resolveRuleList(registry, "--disable", slices.Sorted(maps.Keys(offered)), nil)
	assert.Empty(t, problems)
	assert.Len(t, ids, len(registry.Rules()))
}

// TestCompleteRulesDescribesRulesFromTheCatalog checks that every rule ID,
// name and alias completes with the rule's one-line catalog description,
// as gomdlint rules shows it.
func TestCompleteRulesDescribesRulesFromTheCatalog(t *testing.T) {
	registry := lint.DefaultRegistry

	descriptions := make(map[string]string) // completed value -> description
	for _, prefix := range []string{"", "m", "M", "u", "s", "f", "b", "n"} {
		completions, _ := completeRules(registry)(nil, nil, prefix)
		for _, completion := range completions {
			value, description, _ := strings.Cut(completion, "\t")
			descriptions[value] = description
		}
	}

	for _, rule := range registry.Rules() {
		meta, ok := registry.Meta(rule.ID())
		require.True(t, ok, "%s has a catalog entry", rule.ID())
		require.NotEmpty(t, meta.Description, "%s has a description", rule.ID())
		for _, value := range slices.Concat([]string{rule.ID(), rule.Name()}, meta.Aliases) {
			if description, offered := descriptions[value]; offered {
				assert.Equal(t, meta.Description, description, "%s completes with the catalog description", value)
			}
		}
		assert.Contains(t, descriptions, rule.Name(), "%s completes", rule.Name())
	}
}

// TestCompleteRulesIgnoresCase covers words typed in another case. Values
// keep their own spelling; shells apply their own rules for case.
func TestCompleteRulesIgnoresCase(t *testing.T) {
	assert.Equal(t, []string{"ul", "ul-indent", "ul-style"}, completedValues(t, lint.DefaultRegistry, "UL"))

	values := completedValues(t, lint.DefaultRegistry, "md009,md01")
	require.NotEmpty(t, values)
	assert.Equal(t, "md009,MD010", values[0], "the items before the last stay as typed")
	for _, value := range values {
		assert.True(t, strings.HasPrefix(value, "md009,MD01"), value)
	}
}
