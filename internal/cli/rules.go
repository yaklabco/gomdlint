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

func newRulesCommand() *cobra.Command {
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

			if err := writeRules(cmd.OutOrStdout(), lint.DefaultRegistry.Rules(), ruleFormat); err != nil {
				return withExitCode(ExitIOError, err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&flags.ruleFormat, "rule-format", "name",
		"rule identifier format in output: name, id, or combined")

	return cmd
}

// writeRules writes the rules as a plain table, one rule per line.
func writeRules(out io.Writer, rules []lint.Rule, ruleFormat config.RuleFormat) error {
	table := tabwriter.NewWriter(out, 0, 0, rulesTablePadding, ' ', 0)
	fmt.Fprintln(table, "RULE\tSEVERITY\tFIXABLE\tDESCRIPTION")
	for _, rule := range rules {
		fixable := "no"
		if rule.CanFix() {
			fixable = "yes"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n",
			config.FormatRuleID(ruleFormat, rule.ID(), rule.Name()),
			rule.DefaultSeverity(), fixable, rule.Description())
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("write rules: %w", err)
	}
	return nil
}
