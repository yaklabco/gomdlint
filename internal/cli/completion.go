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
		return ruleCompletions(registry, listed, partial, false), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeExplain returns the completion of the argument of explain: a rule
// by name, ID or alias, a tag, or a category, as completeRules offers them
// for one value, and with the categories, which explain accepts too. Only
// the first argument completes.
func completeExplain(registry *lint.Registry) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return ruleCompletions(registry, "", toComplete, true), cobra.ShellCompDirectiveNoFileComp
	}
}

// ruleCompletions returns the rules of registry by name, ID and alias, and
// the markdownlint tags of their catalog entries, that start with partial,
// ignoring case, each after listed and with its description. An empty
// partial offers each rule once, by name, and the tags. With categories,
// it offers the categories of the rules too.
func ruleCompletions(registry *lint.Registry, listed, partial string, categories bool) []cobra.Completion {
	prefix := strings.ToLower(partial)

	var completions []cobra.Completion
	offer := func(value, description string) {
		if strings.HasPrefix(strings.ToLower(value), prefix) {
			completions = append(completions, cobra.CompletionWithDesc(listed+value, description))
		}
	}

	for _, rule := range registry.Rules() {
		meta, _ := registry.Meta(rule.ID())

		description := lint.RuleDescription(rule, meta)
		offer(rule.Name(), description)
		if partial == "" {
			continue
		}
		offer(rule.ID(), description)
		for _, alias := range meta.Aliases {
			offer(alias, description)
		}
	}
	for _, tag := range registryTags(registry) {
		offer(tag, tagDescription(registry, tag))
	}
	if categories {
		for _, category := range knownCategories(registry) {
			// A category that is also a tag is offered once, as the tag,
			// as explain reads it.
			if len(registry.RulesByTag(string(category))) == 0 {
				offer(string(category), categoryDescription(category))
			}
		}
	}

	slices.Sort(completions)
	return completions
}

// registerRulesCompletions makes the shell complete the filters of the
// rules command cmd: --category with the categories of the rules of
// registry, and --tag with their tags. Each takes a comma-separated list.
func registerRulesCompletions(cmd *cobra.Command, registry *lint.Registry) {
	completeList := func(values func() []cobra.Completion) cobra.CompletionFunc {
		return func(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			listed, partial := "", toComplete
			if last := strings.LastIndexByte(toComplete, ','); last >= 0 {
				listed, partial = toComplete[:last+1], toComplete[last+1:]
			}
			var completions []cobra.Completion
			for _, value := range values() {
				if strings.HasPrefix(strings.ToLower(value), strings.ToLower(partial)) {
					completions = append(completions, listed+value)
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		}
	}

	categories := func() []cobra.Completion {
		known := knownCategories(registry)
		completions := make([]cobra.Completion, 0, len(known))
		for _, category := range known {
			completions = append(completions, cobra.CompletionWithDesc(string(category), categoryDescription(category)))
		}
		return completions
	}
	tags := func() []cobra.Completion {
		listed := registryTags(registry)
		completions := make([]cobra.Completion, 0, len(listed))
		for _, tag := range listed {
			completions = append(completions, cobra.CompletionWithDesc(tag, tagDescription(registry, tag)))
		}
		return completions
	}

	for name, values := range map[string]func() []cobra.Completion{"category": categories, "tag": tags} {
		if err := cmd.RegisterFlagCompletionFunc(name, completeList(values)); err != nil {
			// Only a flag that does not exist, or has a completion already,
			// fails: a mistake in this file, which the tests catch.
			panic(fmt.Sprintf("complete --%s: %v", name, err))
		}
	}
}

// categoryDescription describes a category by its title.
func categoryDescription(category lint.Category) string {
	return "Category: " + category.Title()
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
