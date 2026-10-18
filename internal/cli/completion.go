package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// ruleListFlags are the flags that take a list of rules.
//
//nolint:gochecknoglobals // Read-only list.
var ruleListFlags = []string{"enable", "disable", "fix-rules"}

// registerRuleCompletions makes the shell complete the rule list flags of
// cmd with the rules of registry and their tags. Cobra's completion
// scripts, which the completion command prints, ask for them through its
// hidden __complete command.
func registerRuleCompletions(cmd *cobra.Command, registry *lint.Registry) {
	complete := completeRules(registry)
	for _, name := range ruleListFlags {
		if err := cmd.RegisterFlagCompletionFunc(name, complete); err != nil {
			// Only a flag that does not exist, or has a completion already,
			// fails: a mistake in this file, which the tests catch.
			panic(fmt.Sprintf("complete --%s: %v", name, err))
		}
	}
}

// completeRules returns the completion of a rule list flag: the rules of
// registry by name, ID and alias, and the markdownlint tags of their
// catalog entries, as the flag accepts them. Each value comes with the
// rule's description, or for a tag the names of its rules.
//
// Values match the word being completed by prefix, ignoring case. An empty
// word offers each rule once, by name, and the tags; IDs and aliases are
// offered once the word starts one. The values are comma-separated, so
// only the last item of the word is completed, and the items before it
// stay in front of each value.
func completeRules(registry *lint.Registry) cobra.CompletionFunc {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		listed, partial := "", toComplete
		if last := strings.LastIndexByte(toComplete, ','); last >= 0 {
			listed, partial = toComplete[:last+1], toComplete[last+1:]
		}
		prefix := strings.ToLower(partial)

		var completions []cobra.Completion
		offer := func(value, description string) {
			if strings.HasPrefix(strings.ToLower(value), prefix) {
				completions = append(completions, cobra.CompletionWithDesc(listed+value, description))
			}
		}

		var tags []string
		for _, rule := range registry.Rules() {
			meta, _ := registry.Meta(rule.ID())
			tags = append(tags, meta.Tags...)

			offer(rule.Name(), rule.Description())
			if partial == "" {
				continue
			}
			offer(rule.ID(), rule.Description())
			for _, alias := range meta.Aliases {
				offer(alias, rule.Description())
			}
		}

		slices.Sort(tags)
		for _, tag := range slices.Compact(tags) {
			offer(tag, tagDescription(registry, tag))
		}

		slices.Sort(completions)
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

// tagDescription describes a tag by the names of the rules it covers.
func tagDescription(registry *lint.Registry, tag string) string {
	tagged := registry.RulesByTag(tag)
	names := make([]string, len(tagged))
	for idx, rule := range tagged {
		names[idx] = rule.Name()
	}
	return "Tag: " + strings.Join(names, ", ")
}
