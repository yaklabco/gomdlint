package config

const markdownlintLineLength = 80

// MarkdownlintFrontMatterTitlePattern matches conventional metadata title keys.
const MarkdownlintFrontMatterTitlePattern = `^\s*"?title"?\s*[:=]`

// MarkdownlintRuleConfig converts a markdownlint rule value to explicit gomdlint
// options. Native gomdlint defaults remain independent of migrated settings.
func MarkdownlintRuleConfig(ruleID string, value any) RuleConfig {
	enabled := true
	severity := string(SeverityError)
	cfg := RuleConfig{Enabled: &enabled, Severity: &severity}
	switch ruleID {
	case "MD001":
		cfg.Options = map[string]any{"front_matter_title": MarkdownlintFrontMatterTitlePattern}
	case "MD004", "MD046", "MD048":
		cfg.Options = map[string]any{"style": "consistent"}
	case "MD009":
		cfg.Options = map[string]any{"ignore_code_blocks": true, "br_spaces": 2}
	case "MD013":
		cfg.Options = map[string]any{
			"max": markdownlintLineLength, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true,
		}
	case "MD025":
		cfg.Options = map[string]any{"title_only": true}
	case "MD022", "MD060":
		cfg.Options = map[string]any{"markdownlint_compatible": true}
	case "MD029":
		cfg.Options = map[string]any{"style": "one_or_ordered"}
	case "MD033":
		cfg.Options = map[string]any{"allowed_elements": []string{}}
	case "MD059":
		cfg.Options = map[string]any{
			"patterns": []string{"click here", "here", "link", "more"}, "markdownlint_compatible": true,
		}
	}
	switch typed := value.(type) {
	case bool:
		*cfg.Enabled = typed
	case nil:
		*cfg.Enabled = false
	case string:
		if typed == "warning" {
			severity = typed
		}
	case map[string]any:
		if cfg.Options == nil {
			cfg.Options = make(map[string]any)
		}
		for key, option := range typed {
			if ruleID == "MD059" && key == "prohibited_texts" {
				key = "patterns"
			}
			if ruleID == "MD012" && key == "maximum" {
				key = "max_consecutive"
			}
			switch key {
			case "enabled":
				if state, ok := option.(bool); ok {
					*cfg.Enabled = state
				}
				continue
			case "severity":
				if explicitSeverity, ok := option.(string); ok {
					cfg.Severity = &explicitSeverity
				}
				continue
			}
			if ruleID == "MD013" {
				switch key {
				case "line_length":
					key = "max"
				case "heading_line_length":
					key = "heading_max"
				case "code_block_line_length":
					key = "code_block_max"
				case "code_blocks", "tables", "headings":
					if include, ok := option.(bool); ok {
						cfg.Options["ignore_"+key] = !include
						continue
					}
				}
			}
			if ruleID == "MD009" && key == "code_blocks" {
				if include, ok := option.(bool); ok {
					cfg.Options["ignore_code_blocks"] = !include
					continue
				}
			}
			cfg.Options[key] = option
		}
	}
	return cfg
}
