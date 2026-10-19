package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

type explainFlags struct {
	format   string
	defaults bool
}

// newExplainCommand creates the explain command, which describes a rule of
// registry, or lists the rules of a tag or a category.
func newExplainCommand(registry *lint.Registry) *cobra.Command {
	flags := &explainFlags{}

	cmd := &cobra.Command{
		Use:               "explain <rule>",
		Short:             "Explain what a rule checks and how to configure it",
		Long:              explainLongDescription,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeExplain(registry),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExplain(cmd, args[0], flags, registry)
		},
	}

	cmd.Flags().StringVar(&flags.format, "format", docsFormatText, "output format: text, json, markdown")
	cmd.Flags().BoolVar(&flags.defaults, "defaults", false,
		"show the built-in settings instead of the configuration's")

	return cmd
}

const explainLongDescription = `Explain what a rule checks, why it matters, its options and their
defaults, what its fix does, and how to configure it and silence it.

The argument is a rule ID, name or markdownlint name, in any case, such as
MD009, no-trailing-spaces or ul-style. A markdownlint tag, such as
whitespace, or a category, such as layout, lists the rules it covers.

The page says whether the rule runs, and at what severity, under the
configuration that 'gomdlint lint' would load here, as 'gomdlint rules'
does. --defaults shows the built-in settings instead. --format markdown
prints the rule's documentation page, which does not read the
configuration, and --format json prints the rule as
'gomdlint rules --format json' does.

Examples:
  gomdlint explain line-length          # By name
  gomdlint explain MD009                # By ID
  gomdlint explain ul-style             # By markdownlint's name
  gomdlint explain whitespace           # The rules of a tag
  gomdlint explain line-length --format markdown

` + docsExitCodeHelp

// explainTopic is what the argument of explain names: a rule, a tag, a
// category, or a tag and a category of the same name.
type explainTopic struct {
	// rule is the rule the argument names, or nil.
	rule lint.Rule

	// tag is the tag the argument names, as the catalog writes it, or "".
	tag string

	// category is the category the argument names, or "".
	category lint.Category
}

// runExplain explains the rule, tag or category that arg names.
func runExplain(cmd *cobra.Command, arg string, flags *explainFlags, registry *lint.Registry) error {
	if err := validateDocsFormat(flags.format); err != nil {
		return err
	}
	topic, err := findExplainTopic(registry, arg)
	if err != nil {
		return err
	}

	// Markdown output does not depend on the rules' states, so it reads no
	// configuration. Only the text listing of a tag or a category names
	// rules in a rule format.
	var cfg *config.Config // nil for the built-in settings
	if !flags.defaults && flags.format != docsFormatMarkdown {
		usesRuleFormat := flags.format == docsFormatText && topic.rule == nil
		if cfg, err = loadRuleSettings(cmd, listingSettings("", usesRuleFormat)); err != nil {
			return err
		}
	}
	entries := ruledocs.Entries(registry, cfg)
	out := cmd.OutOrStdout()

	if topic.rule != nil {
		idx := slices.IndexFunc(entries, func(entry ruledocs.Entry) bool { return entry.ID == topic.rule.ID() })
		if idx < 0 {
			return withExitCode(ExitInternalError, fmt.Errorf("rule %s is not registered", topic.rule.ID()))
		}
		switch flags.format {
		case docsFormatJSON:
			err = ruledocs.WriteJSON(out, entries[idx:idx+1])
		case docsFormatMarkdown:
			err = ruledocs.WriteMarkdownPage(out, entries[idx])
		default:
			err = ruledocs.WriteTextPage(out, entries[idx], arg)
		}
		return withExitCode(ExitIOError, err)
	}

	return explainGroup(cmd, topic, flags.format, entries, cfg)
}

// explainGroup lists the rules of topic, a tag or a category, in format.
// The text listing names the rules command that lists them too, and names
// rules as GOMDLINT_RULE_FORMAT says. cfg is the loaded configuration, or
// nil under --defaults. For a text listing, it must be loaded with
// listingSettings reporting that the output uses the rule format, so that
// it holds the variable's setting. When topic is both a tag and a category,
// it lists the rules of the tag.
func explainGroup(cmd *cobra.Command, topic explainTopic, format string, entries []ruledocs.Entry, cfg *config.Config) error {
	filter := ruledocs.Filter{Categories: []lint.Category{topic.category}}
	title := "Rules in the category " + string(topic.category)
	lead := string(topic.category) + " is a category. It holds these rules:"
	command := "gomdlint rules --category " + string(topic.category)
	if topic.tag != "" {
		filter = ruledocs.Filter{Tags: []string{topic.tag}}
		title = "Rules with the tag " + topic.tag
		lead = topic.tag + " is a markdownlint tag. It covers these rules:"
		command = "gomdlint rules --tag " + topic.tag
	}

	out := cmd.OutOrStdout()
	listed := filter.Select(entries)
	if format != docsFormatText {
		return writeRuleList(out, format, title, listed, ruledocs.ListOptions{})
	}
	ruleFormat, err := listingRuleFormat(cmd, "", cfg)
	if err != nil {
		return err
	}

	var text strings.Builder
	text.WriteString(lead + "\n\n")
	if err := ruledocs.WriteText(&text, listed, ruledocs.ListOptions{RuleFormat: ruleFormat, Total: len(entries)}); err != nil {
		return withExitCode(ExitInternalError, err)
	}
	fmt.Fprintf(&text, "Run '%s' to list them with more filters.\n", command)
	if topic.tag != "" && topic.category != "" {
		fmt.Fprintf(&text, "%s is also a category. Run 'gomdlint rules --category %s' to list its rules.\n",
			topic.category, topic.category)
	}
	if _, err := fmt.Fprint(out, text.String()); err != nil {
		return withExitCode(ExitIOError, fmt.Errorf("write rules: %w", err))
	}
	return nil
}

// findExplainTopic returns what arg names: a rule, by ID, name or alias, or
// else a tag, a category, or both. Matching ignores case. An argument that
// names none of them is a usage error, which offers the closest rules,
// tags and categories.
func findExplainTopic(registry *lint.Registry, arg string) (explainTopic, error) {
	if rule, ok := registry.Lookup(arg); ok {
		return explainTopic{rule: rule}, nil
	}

	var topic explainTopic
	if tagged := registry.RulesByTag(arg); len(tagged) > 0 {
		topic.tag = catalogTag(registry, tagged[0], arg)
	}
	categories := knownCategories(registry)
	topic.category, _ = findCategory(categories, arg)
	if topic.tag == "" && topic.category == "" {
		return explainTopic{}, &ExitError{
			Code: ExitInvalidUsage,
			Err:  errors.New(unknownTopicText(registry, categories, arg) + "\n" + listRulesHint),
		}
	}
	return topic, nil
}

// unknownTopicText describes arg, an argument of explain that names no
// rule, tag or category, as lint.UnknownRuleText describes a rule flag's
// value, and offers the closest categories after the closest rules and
// tags. A category that is offered as a tag already is not offered again.
func unknownTopicText(registry *lint.Registry, categories []lint.Category, arg string) string {
	var choices []string
	offered := make(map[string]bool)
	for _, suggestion := range registry.Closest(arg) {
		choice := strconv.Quote(suggestion.Name)
		if suggestion.IsTag() {
			choice = "the tag " + choice
			offered[suggestion.Name] = true
		}
		choices = append(choices, choice)
	}

	keys := make([]string, len(categories))
	for idx, category := range categories {
		keys[idx] = string(category)
	}
	for _, key := range closestWords(keys, arg) {
		if !offered[key] {
			choices = append(choices, "the category "+strconv.Quote(key))
		}
	}
	return fmt.Sprintf("unknown rule %q", arg) + didYouMean(choices)
}

// catalogTag returns tag as the catalog entry of rule, one of the rules the
// tag covers, writes it.
func catalogTag(registry *lint.Registry, rule lint.Rule, tag string) string {
	meta, _ := registry.Meta(rule.ID()) // RulesByTag finds tags in the catalog entries.
	for _, own := range meta.Tags {
		if strings.EqualFold(own, strings.TrimSpace(tag)) {
			return own
		}
	}
	return strings.TrimSpace(tag)
}
