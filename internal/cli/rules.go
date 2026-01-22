package cli

import (
	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

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
		RunE: func(_ *cobra.Command, _ []string) error {
			logger := logging.NewInteractive()

			rules := lint.DefaultRegistry.Rules()

			if len(rules) == 0 {
				logger.Info("no rules registered yet")
				logger.Info("rules will be added in a future release")
				return nil
			}

			logger.Info("available rules")

			ruleFormat := config.RuleFormat(flags.ruleFormat)

			for _, rule := range rules {
				fixable := "-"
				if rule.CanFix() {
					fixable = "yes"
				}

				ruleIdentifier := config.FormatRuleID(ruleFormat, rule.ID(), rule.Name())

				logger.Info(ruleIdentifier,
					logging.FieldSeverity, rule.DefaultSeverity(),
					logging.FieldFixable, fixable,
					logging.FieldDescription, rule.Description(),
				)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&flags.ruleFormat, "rule-format", "name",
		"rule identifier format in output: name, id, or combined")

	return cmd
}
