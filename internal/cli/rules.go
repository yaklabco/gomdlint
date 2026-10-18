package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// rulesTablePadding separates the columns of the rules listing.
const rulesTablePadding = 2

type rulesFlags struct {
	ruleFormat string
}

// newRulesCommand creates the rules command, which lists the rules of
// registry.
func newRulesCommand(registry *lint.Registry) *cobra.Command {
	flags := &rulesFlags{}

	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List available lint rules",
		Long: `List all available lint rules with their IDs, descriptions,
default severity, and whether they support auto-fixing.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ruleFormat := config.RuleFormat(flags.ruleFormat)
			if !ruleFormat.IsValid() {
				return usageError("invalid rule format %q; valid formats: name, id, combined", flags.ruleFormat)
			}

			if err := writeRules(cmd.OutOrStdout(), registry, ruleFormat); err != nil {
				return withExitCode(ExitIOError, err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.ruleFormat, "rule-format", "name",
		"rule identifier format in output: name, id, or combined")

	return cmd
}

// writeRules writes the rules of registry as a plain table, one rule per
// line, in rule ID order. Each rule is described by its catalog entry, or
// by itself when the entry is missing or has no description.
func writeRules(out io.Writer, registry *lint.Registry, ruleFormat config.RuleFormat) error {
	table := tabwriter.NewWriter(out, 0, 0, rulesTablePadding, ' ', 0)
	fmt.Fprintln(table, "RULE\tSEVERITY\tFIXABLE\tDESCRIPTION")
	for _, rule := range registry.Rules() {
		fixable := "no"
		if rule.CanFix() {
			fixable = "yes"
		}
		meta, _ := registry.Meta(rule.ID()) // A rule without an entry gets the zero entry.
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n",
			config.FormatRuleID(ruleFormat, rule.ID(), rule.Name()),
			rule.DefaultSeverity(), fixable, lint.RuleDescription(rule, meta))
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("write rules: %w", err)
	}
	return nil
}
