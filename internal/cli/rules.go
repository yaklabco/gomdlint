package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

type rulesFlags struct {
	ruleFormat string
	format     string
}

const formatJSON = "json"

// ruleInfo represents a rule in JSON output.
type ruleInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Fixable     bool   `json:"fixable"`
}

func newRulesCommand() *cobra.Command {
	flags := &rulesFlags{}

	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List available lint rules",
		Long: `List all available lint rules with their IDs, descriptions,
default severity, and whether they support auto-fixing.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			rules := lint.DefaultRegistry.Rules()

			// Handle JSON output format.
			if flags.format == formatJSON {
				return outputRulesJSON(rules)
			}

			// Default to text output.
			logger := logging.NewInteractive()

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
	cmd.Flags().StringVar(&flags.format, "format", "text",
		"output format: text, json")

	return cmd
}

// outputRulesJSON outputs rules as a JSON array.
func outputRulesJSON(rules []lint.Rule) error {
	infos := make([]ruleInfo, 0, len(rules))
	for _, rule := range rules {
		infos = append(infos, ruleInfo{
			ID:          rule.ID(),
			Name:        rule.Name(),
			Description: rule.Description(),
			Severity:    string(rule.DefaultSeverity()),
			Fixable:     rule.CanFix(),
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(infos); err != nil {
		return fmt.Errorf("encoding rules: %w", err)
	}
	return nil
}
