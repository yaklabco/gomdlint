package config

import (
	"math"
	"reflect"
)

// MarkdownlintFrontMatterTitlePattern matches conventional metadata title keys.
const MarkdownlintFrontMatterTitlePattern = `^\s*"?title"?\s*[:=]`

// markdownlintWarning is the value that makes a rule report warnings in a
// markdownlint configuration.
const markdownlintWarning = string(SeverityWarning)

// OptionNormalizer returns the options that the rule with the given ID gets
// from a setting in a markdownlint configuration, given options, the
// options that the setting writes, which are none for a setting that is no
// object. The lint package's Registry supplies one from the rule catalog,
// which this package cannot import: for a rule key, it rewrites the options
// to the names that the rule reads, such as max for markdownlint's
// line_length (see lint.Registry.MarkdownlintRuleConfig), and for the
// default key, which writes none, it gives the options that the key writes
// out (see lint.Registry.MarkdownlintDefaultRuleConfig).
type OptionNormalizer func(ruleID string, options map[string]any) map[string]any

// MarkdownlintDefaultRuleConfig converts the value of markdownlint's default
// key to the settings that it gives the rule with the given ID, as
// getEffectiveConfig in markdownlint 0.41.1 reads it: the rule is on when
// the value is truthy in JavaScript, everything but false, null, zero, NaN
// and the empty string, and it reports warnings for "warning" and errors
// otherwise. Nothing else comes from the value, so an object turns the rule
// on whatever it holds, and the rule keeps its option defaults, which are
// markdownlint's. The settings reset no option of a lower layer (see
// RuleConfig.ResetOptions), so a rule that a lower layer gives options
// keeps those that the default key does not write out, unless a rule key
// turns the rule on. normalize, when not nil, gives the options that the
// default key writes out.
func MarkdownlintDefaultRuleConfig(ruleID string, value any, normalize OptionNormalizer) RuleConfig {
	var cfg RuleConfig
	switch text, isText := value.(string); {
	case isText && text == markdownlintWarning:
		cfg = MarkdownlintRuleConfig(ruleID, markdownlintWarning, normalize)
	case markdownlintTruthy(value):
		cfg = MarkdownlintRuleConfig(ruleID, true, normalize)
	default:
		cfg = MarkdownlintRuleConfig(ruleID, false, normalize)
	}
	cfg.ResetOptions = false
	return cfg
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

// MarkdownlintRuleConfig converts value, the setting of a rule in a
// markdownlint configuration, to gomdlint's settings for the rule with the
// given ID. markdownlint reports every finding as an error, so the rule
// reports errors unless value asks for warnings. As in getEffectiveConfig
// in markdownlint 0.41.1, a value that is falsy in JavaScript turns the
// rule off, a string turns it on, with warnings when it is "warning", and
// an object turns it on unless its enabled key is present and falsy. An
// object's severity key asks for warnings when it is "warning", and for
// errors otherwise, whatever it holds: gomdlint's info, a misspelling such
// as Warning, or a value that is not a string. The object holds the rule's
// options under its other keys.
//
// getEffectiveConfig gives a rule the options of the value that sets it
// and no others, so a value that turns the rule on resets the options that
// lower configuration layers give it (see RuleConfig.ResetOptions): every
// option that the value does not set takes its default. A value that turns
// the rule off resets nothing.
//
// normalize, when not nil, gives the options that the rule gets from the
// options that value writes, which it calls with none for a value that
// writes none (see OptionNormalizer). Without normalize, the settings hold
// the options as written.
func MarkdownlintRuleConfig(ruleID string, value any, normalize OptionNormalizer) RuleConfig {
	enabled := true
	severity := string(SeverityError)
	cfg := RuleConfig{Enabled: &enabled, Severity: &severity}
	options := make(map[string]any)
	switch typed := value.(type) {
	case string:
		*cfg.Enabled = typed != ""
		if typed == markdownlintWarning {
			severity = typed
		}
	case map[string]any:
		for key, option := range typed {
			switch key {
			case "enabled":
				*cfg.Enabled = markdownlintTruthy(option)
			case "severity":
				severity = string(SeverityError)
				if text, ok := option.(string); ok && text == markdownlintWarning {
					severity = markdownlintWarning
				}
			default:
				options[key] = option
			}
		}
	default:
		// Booleans, null, numbers and arrays, as JavaScript tests them.
		*cfg.Enabled = markdownlintTruthy(value)
	}
	if normalize != nil {
		options = normalize(ruleID, options)
	}
	if len(options) > 0 {
		cfg.Options = options
	}
	cfg.ResetOptions = *cfg.Enabled
	return cfg
}
