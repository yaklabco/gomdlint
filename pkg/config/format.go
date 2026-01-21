package config

// FormatRuleID formats a rule identifier based on the given format.
// Falls back to ID if name is empty.
func FormatRuleID(format RuleFormat, ruleID, ruleName string) string {
	// Fall back to ID if name is empty
	if ruleName == "" {
		return ruleID
	}

	switch format {
	case RuleFormatID:
		return ruleID
	case RuleFormatCombined:
		return ruleID + "/" + ruleName
	case RuleFormatName:
		return ruleName
	default:
		// Default to name format
		return ruleName
	}
}
