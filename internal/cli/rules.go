package cli

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/configloader"
	"github.com/yaklabco/gomdlint/internal/editdistance"
	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// The output formats of the rules and explain commands.
const (
	docsFormatText     = "text"
	docsFormatJSON     = "json"
	docsFormatMarkdown = "markdown"
)

// maxCategoryDistance is the largest edit distance at which a category is
// offered as a likely typo of a --category value.
const maxCategoryDistance = 2

type rulesFlags struct {
	ruleFormat string
	format     string
	categories []string
	tags       []string
	fixable    bool
	enabled    bool
	disabled   bool
	defaults   bool
}

// newRulesCommand creates the rules command, which lists the rules of
// registry.
func newRulesCommand(registry *lint.Registry) *cobra.Command {
	flags := &rulesFlags{}

	cmd := &cobra.Command{
		Use:               "rules [words...]",
		Short:             "List the lint rules by category",
		Long:              rulesLongDescription,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRules(cmd, args, flags, registry)
		},
	}

	cmd.Flags().StringVar(&flags.ruleFormat, "rule-format", "name",
		"how the text listing names rules: name, id, or combined")
	cmd.Flags().StringVar(&flags.format, "format", docsFormatText, "output format: text, json, markdown")
	cmd.Flags().StringSliceVar(&flags.categories, "category", nil,
		"list the rules of these categories, such as headings")
	cmd.Flags().StringSliceVar(&flags.tags, "tag", nil,
		"list the rules of these markdownlint tags, such as whitespace")
	cmd.Flags().BoolVar(&flags.fixable, "fixable", false, "list only the rules that can fix what they find")
	cmd.Flags().BoolVar(&flags.enabled, "enabled", false, "list only the rules that run")
	cmd.Flags().BoolVar(&flags.disabled, "disabled", false, "list only the rules that do not run")
	cmd.Flags().BoolVar(&flags.defaults, "defaults", false,
		"show the built-in settings instead of the configuration's")
	registerRulesCompletions(cmd, registry)

	return cmd
}

const rulesLongDescription = `List the lint rules by category, each with its ID, whether it can fix
what it finds, its status and what it checks. Within a category, rules are
sorted by name, or by ID when --rule-format puts the ID first.

STATUS is off, or the severity of the rule's findings, under the
configuration that 'gomdlint lint' would load here: the project config, or
the file that --config names, with GOMDLINT_* settings. --defaults shows the
built-in settings instead.

Words, and the filter flags, select rules. A rule must match each word,
ignoring case, in its name, ID, markdownlint name, category, tags or
description, and pass every filter. A filter flag that is given several
values, comma-separated or repeated, matches any of them.

Examples:
  gomdlint rules                          # Every rule, by category
  gomdlint rules table                    # Rules that mention tables
  gomdlint rules --tag whitespace         # The rules of a markdownlint tag
  gomdlint rules --category headings --fixable
  gomdlint rules --disabled               # The rules this project turns off
  gomdlint rules --format json            # Every rule and its options, as JSON
  gomdlint rules --format markdown        # A Markdown table of the rules

'gomdlint explain <rule>' shows what one rule checks, why, its options and
what its fix does.

` + docsExitCodeHelp

// runRules lists the rules of registry that pass the filters of args and
// flags.
func runRules(cmd *cobra.Command, args []string, flags *rulesFlags, registry *lint.Registry) error {
	if err := validateDocsFormat(flags.format); err != nil {
		return err
	}
	if !config.RuleFormat(flags.ruleFormat).IsValid() {
		return usageError("invalid rule format %q; valid formats: name, id, combined", flags.ruleFormat)
	}
	if flags.enabled && flags.disabled {
		return usageError("--enabled and --disabled exclude each other; give one of them")
	}
	filter, err := rulesFilter(args, flags, registry)
	if err != nil {
		return err
	}

	var cfg *config.Config // nil for the built-in settings
	if !flags.defaults {
		var cliCfg config.Config
		if cmd.Flags().Changed("rule-format") {
			cliCfg.RuleFormat = config.RuleFormat(flags.ruleFormat)
		}
		if cfg, err = loadRuleSettings(cmd, &cliCfg); err != nil {
			return err
		}
	}
	opts := ruledocs.ListOptions{}
	if flags.format == docsFormatText {
		if opts.RuleFormat, err = listingRuleFormat(cmd, flags.ruleFormat, cfg); err != nil {
			return err
		}
	}

	entries := ruledocs.Entries(registry, cfg)
	opts.Total = len(entries)
	return writeRuleList(cmd.OutOrStdout(), flags.format, "Rules", filter.Select(entries), opts)
}

// validateDocsFormat rejects an output format of the rules and explain
// commands other than text, json and markdown.
func validateDocsFormat(format string) error {
	switch format {
	case docsFormatText, docsFormatJSON, docsFormatMarkdown:
		return nil
	}
	return usageError("invalid format %q; valid formats: text, json, markdown", format)
}

// writeRuleList writes entries in format: a text table as opts says, JSON,
// or Markdown tables under a heading with title.
func writeRuleList(out io.Writer, format, title string, entries []ruledocs.Entry, opts ruledocs.ListOptions) error {
	var err error
	switch format {
	case docsFormatJSON:
		err = ruledocs.WriteJSON(out, entries)
	case docsFormatMarkdown:
		err = ruledocs.WriteMarkdown(out, title, entries)
	default:
		err = ruledocs.WriteText(out, entries, opts)
	}
	return withExitCode(ExitIOError, err)
}

// rulesFilter builds the filter of the rules command from its words and
// flags, checking that each category and tag exists. A value that names
// none is a usage error, which describes every such value, one per line.
func rulesFilter(args []string, flags *rulesFlags, registry *lint.Registry) (ruledocs.Filter, error) {
	filter := ruledocs.Filter{
		Words:    args,
		Fixable:  flags.fixable,
		Enabled:  flags.enabled,
		Disabled: flags.disabled,
	}

	categories := knownCategories(registry)
	var problems []string
	for _, value := range flags.categories {
		category, ok := findCategory(categories, value)
		if !ok {
			problems = append(problems, "--category: "+unknownCategoryText(registry, categories, value))
			continue
		}
		filter.Categories = append(filter.Categories, category)
	}
	for _, value := range flags.tags {
		tagged := registry.RulesByTag(value)
		if len(tagged) == 0 {
			problems = append(problems, "--tag: "+unknownTagText(registry, categories, value))
			continue
		}
		filter.Tags = append(filter.Tags, catalogTag(registry, tagged[0], value))
	}

	if len(problems) > 0 {
		return ruledocs.Filter{}, usageError("%s", strings.Join(problems, "\n"))
	}
	return filter, nil
}

// knownCategories returns every category that gomdlint defines, in the
// order of lint.Categories, then any others that the catalog entries of
// the rules of registry name, in order of key.
func knownCategories(registry *lint.Registry) []lint.Category {
	categories := lint.Categories()
	var others []lint.Category
	for _, id := range registry.IDs() {
		meta, _ := registry.Meta(id) // A rule without an entry has no category.
		if meta.Category != "" && !slices.Contains(categories, meta.Category) && !slices.Contains(others, meta.Category) {
			others = append(others, meta.Category)
		}
	}
	slices.Sort(others)
	return append(categories, others...)
}

// findCategory returns the category of categories that value names,
// ignoring case and surrounding whitespace.
func findCategory(categories []lint.Category, value string) (lint.Category, bool) {
	value = strings.TrimSpace(value)
	for _, category := range categories {
		if strings.EqualFold(string(category), value) {
			return category, true
		}
	}
	return "", false
}

// unknownCategoryText describes value, a --category value that names no
// category, with the closest categories, and --tag when value is a tag,
// and lists the categories.
func unknownCategoryText(registry *lint.Registry, categories []lint.Category, value string) string {
	keys := make([]string, len(categories))
	for idx, category := range categories {
		keys[idx] = string(category)
	}
	var choices []string
	for _, key := range closestWords(keys, value) {
		choices = append(choices, strconv.Quote(key))
	}
	if len(registry.RulesByTag(value)) > 0 {
		choices = append(choices, "--tag "+strings.TrimSpace(value))
	}
	return fmt.Sprintf("unknown category %q", value) + didYouMean(choices) + "; categories: " + strings.Join(keys, ", ")
}

// unknownTagText describes tag, a --tag value that no rule's catalog entry
// lists, with --category when tag is a category, or else the closest tags,
// and lists the tags.
func unknownTagText(registry *lint.Registry, categories []lint.Category, tag string) string {
	var choices []string
	if category, isCategory := findCategory(categories, tag); isCategory {
		choices = append(choices, "--category "+string(category))
	} else {
		for _, suggestion := range registry.ClosestTags(tag) {
			choices = append(choices, strconv.Quote(suggestion.Name))
		}
	}
	return fmt.Sprintf("unknown tag %q", tag) + didYouMean(choices) + "; tags: " + strings.Join(registryTags(registry), ", ")
}

// didYouMean offers choices in parentheses, as in ` (did you mean "a" or
// "b"?)`, or returns "" for none.
func didYouMean(choices []string) string {
	if len(choices) == 0 {
		return ""
	}
	last := len(choices) - 1
	offer := choices[last]
	if last > 0 {
		offer = strings.Join(choices[:last], ", ") + " or " + choices[last]
	}
	return " (did you mean " + offer + "?)"
}

// registryTags returns the tags that the catalog entries of the rules of
// registry list, in order.
func registryTags(registry *lint.Registry) []string {
	var tags []string
	for _, id := range registry.IDs() {
		meta, _ := registry.Meta(id) // A rule without an entry has no tags.
		tags = append(tags, meta.Tags...)
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}

// closestWords returns the words that value most likely means, as a typo:
// those at the smallest edit distance from it, at most
// maxCategoryDistance, ignoring case. A word must be longer than the
// distance.
func closestWords(words []string, value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	best := maxCategoryDistance
	closest := make([]string, 0, len(words))
	for _, word := range words {
		distance := editdistance.Levenshtein(value, strings.ToLower(word))
		switch {
		case distance > best || distance >= len(word):
			continue
		case distance < best:
			best, closest = distance, closest[:0]
		}
		closest = append(closest, word)
	}
	return closest
}

// loadRuleSettings returns the configuration whose rule settings the rules
// and explain commands show: the one that lint would load from the working
// directory, or from the file that --config names, with the environment
// and cliCfg, the settings of the command's flags, on top. Its notices and
// warnings go to the command's log, as for lint.
func loadRuleSettings(cmd *cobra.Command, cliCfg *config.Config) (*config.Config, error) {
	ctx := cmd.Context()

	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, withExitCode(ExitInternalError, fmt.Errorf("get config flag: %w", err))
	}
	workDir, err := os.Getwd()
	if err != nil {
		return nil, withExitCode(ExitInternalError, fmt.Errorf("get working directory: %w", err))
	}

	loadResult, err := configloader.Load(ctx, configloader.LoadOptions{
		WorkingDir:     workDir,
		ExplicitPath:   configPath,
		NonInteractive: true,
		Stdin:          cmd.InOrStdin(),
		Stderr:         cmd.ErrOrStderr(),
		CLIConfig:      cliCfg,
	})
	if err != nil {
		return nil, failure(ctx, ExitConfigError, fmt.Errorf("load configuration: %w", err))
	}

	logger := logging.FromContext(ctx)
	for _, notice := range loadResult.Notices {
		logger.Info(notice)
	}
	for _, warning := range loadResult.Warnings {
		logger.Warn(warning)
	}
	return loadResult.Config, nil
}

// listingRuleFormat returns how a text listing names rules: as
// --rule-format says when it is given, else as GOMDLINT_RULE_FORMAT says,
// else by name. cfg is the loaded configuration, which holds the
// variable's setting, already checked, or nil when none was loaded, under
// --defaults, which leaves the variable to be read here. As for lint, an
// invalid value of the variable is a configuration error, unless the flag
// is given.
func listingRuleFormat(cmd *cobra.Command, flag string, cfg *config.Config) (config.RuleFormat, error) {
	if cmd.Flags().Changed("rule-format") {
		return config.RuleFormat(flag), nil
	}
	if cfg != nil {
		return cmp.Or(cfg.RuleFormat, config.RuleFormatName), nil
	}
	format := config.RuleFormat(os.Getenv(ruleFormatVariable))
	if format != "" && !format.IsValid() {
		return "", withExitCode(ExitConfigError,
			fmt.Errorf("%s: invalid rule format %q; must be one of: name, id, combined", ruleFormatVariable, format))
	}
	return cmp.Or(format, config.RuleFormatName), nil
}

// ruleFormatVariable is the environment variable that sets how human
// output names rules when --rule-format is not given.
const ruleFormatVariable = "GOMDLINT_RULE_FORMAT"
