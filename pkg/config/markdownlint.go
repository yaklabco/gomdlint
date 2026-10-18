package config

import (
	"math"
	"reflect"
)

const markdownlintLineLength = 80

// MarkdownlintFrontMatterTitlePattern matches conventional metadata title keys.
const MarkdownlintFrontMatterTitlePattern = `^\s*"?title"?\s*[:=]`

// markdownlintWarning is the value that makes a rule report warnings in a
// markdownlint configuration.
const markdownlintWarning = string(SeverityWarning)

// MarkdownlintDefaultRuleConfig converts the value of markdownlint's default
// key to the settings that it gives the rule with the given ID, as
// getEffectiveConfig in markdownlint 0.41.1 reads it: the rule is on when
// the value is truthy in JavaScript, everything but false, null, zero, NaN
// and the empty string, and it reports warnings for "warning" and errors
// otherwise. Nothing else comes from the value, so an object turns the rule
// on whatever it holds, and the rule keeps markdownlint's option defaults.
func MarkdownlintDefaultRuleConfig(ruleID string, value any) RuleConfig {
	switch text, isText := value.(string); {
	case isText && text == markdownlintWarning:
		return MarkdownlintRuleConfig(ruleID, markdownlintWarning)
	case markdownlintTruthy(value):
		return MarkdownlintRuleConfig(ruleID, true)
	default:
		return MarkdownlintRuleConfig(ruleID, false)
	}
}

// markdownlintTruthy reports whether value, decoded from JSON or YAML, is
// truthy in JavaScript, as markdownlint tests the values of its
// configuration: everything but false, null, zero, NaN and the empty
// string.
func markdownlintTruthy(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Bool:
		return reflected.Bool()
	case reflect.String:
		return reflected.Len() > 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflected.Uint() != 0
	case reflect.Float32, reflect.Float64:
		number := reflected.Float()
		return number != 0 && !math.IsNaN(number)
	default:
		return true
	}
}

// MarkdownlintRuleConfig converts a markdownlint rule value to explicit gomdlint
// options. Native gomdlint defaults remain independent of migrated settings.
// As in getEffectiveConfig in markdownlint 0.41.1, a value that is falsy in
// JavaScript turns the rule off, and an object turns it on unless its
// enabled key is present and falsy.
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
	case string:
		*cfg.Enabled = typed != ""
		if typed == markdownlintWarning {
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
				*cfg.Enabled = markdownlintTruthy(option)
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
	default:
		// Booleans, null, numbers and arrays, as JavaScript tests them.
		*cfg.Enabled = markdownlintTruthy(value)
	}
	return cfg
}
