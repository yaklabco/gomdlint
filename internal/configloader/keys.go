package configloader

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/editdistance"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

const (
	// maxSuggestionDistance is the largest edit distance at which an unknown
	// key is reported as a likely typo of a known one.
	maxSuggestionDistance = 2

	// mergeTag is the YAML tag of a merge key (<<).
	mergeTag = "!!merge"
)

// commandLineOnlyKeys maps settings that exist only as command-line flags to
// those flags. Config files cannot set them, but users often try.
//
//nolint:gochecknoglobals // Read-only lookup table.
var commandLineOnlyKeys = map[string]string{
	"fix":           "--fix",
	"dry_run":       "--dry-run",
	"jobs":          "--jobs",
	"format":        "--format",
	"rule_format":   "--rule-format",
	"enable_rules":  "--enable",
	"disable_rules": "--disable",
	"fix_rules":     "--fix-rules",
	"no_backups":    "--no-backups",
}

// yamlKeys returns the YAML keys a struct type reads, from its yaml tags.
func yamlKeys(value any) []string {
	typ := reflect.TypeOf(value)
	keys := make([]string, 0, typ.NumField())
	for idx := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(idx).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	slices.Sort(keys)
	return keys
}

// unknownKeyWarnings reports keys in the parsed document of a configuration
// file that gomdlint does not read. YAML decoding drops them silently, so a
// typo such as "flavour" would otherwise leave the default in force without
// a trace. Keys under rules and rule_defaults that name no rule in registry
// are reported too.
func unknownKeyWarnings(path string, doc *yaml.Node, registry *lint.Registry) []string {
	if len(doc.Content) == 0 {
		return nil // An empty file has no keys.
	}

	topKeys := yamlKeys(config.Config{})
	backupKeys := yamlKeys(config.BackupsConfig{})
	ruleKeys := yamlKeys(config.RuleConfig{})

	var warnings []string
	warn := func(key *yaml.Node, message string) {
		warnings = append(warnings, fmt.Sprintf("%s:%d: %s", path, key.Line, message))
	}

	forEachKey(doc.Content[0], func(key, value *yaml.Node) {
		name := key.Value
		if flag, ok := commandLineOnlyKeys[name]; ok {
			hint := "use the " + flag + " flag"
			if envVar := GetEnvVarName(name); envVar != "" {
				// The environment sets a lasting default, as a config file would.
				hint += " or the " + envVar + " environment variable"
			}
			warn(key, fmt.Sprintf("%q cannot be set in a config file; %s", name, hint))
			return
		}
		if !slices.Contains(topKeys, name) {
			warn(key, unknownKeyMessage("", name, topKeys))
			return
		}

		switch name {
		case "backups":
			forEachKey(value, func(nested, _ *yaml.Node) {
				if !slices.Contains(backupKeys, nested.Value) {
					warn(nested, unknownKeyMessage("backups.", nested.Value, backupKeys))
				}
			})
		case "rules", "rule_defaults":
			forEachKey(value, func(rule, settings *yaml.Node) {
				if _, known := registry.Lookup(rule.Value); !known {
					warn(rule, unknownRuleMessage(registry, rule.Value))
				}
				prefix := name + "." + rule.Value + "."
				forEachKey(settings, func(nested, _ *yaml.Node) {
					if !slices.Contains(ruleKeys, nested.Value) {
						warn(nested, fmt.Sprintf("unknown key %q; rule options belong under %q",
							prefix+nested.Value, prefix+"options"))
					}
				})
			})
		}
	})

	return warnings
}

// unknownRuleMessage describes key, a key under rules or rule_defaults that
// names no rule, offering the closest rules. Those keys take rules, not
// tags, so a tag gets the names of the rules it covers instead.
func unknownRuleMessage(registry *lint.Registry, key string) string {
	tagged := registry.RulesByTag(key)
	if len(tagged) == 0 {
		return lint.UnknownRuleText(key, registry.ClosestRules(key)) + "; it is ignored"
	}

	names := make([]string, len(tagged))
	for idx, rule := range tagged {
		names[idx] = rule.Name()
	}
	return fmt.Sprintf("unknown rule %q (it is a tag; configure its rules instead: %s); it is ignored",
		key, strings.Join(names, ", "))
}

// unknownKeyMessage describes an unknown key, suggesting the closest known
// key when the unknown one looks like a typo of it.
func unknownKeyMessage(prefix, name string, known []string) string {
	message := fmt.Sprintf("unknown key %q", prefix+name)
	if suggestion, ok := closestKey(name, known); ok {
		message += fmt.Sprintf(" (did you mean %q?)", prefix+suggestion)
	}
	return message + "; it is ignored"
}

// forEachKey calls visit for each key and value of a YAML mapping node.
// Other node kinds have no keys. A merge key (<<) is skipped: it is not a
// key of its own but brings in the keys of an anchored mapping, which are
// checked where the anchor is defined.
func forEachKey(node *yaml.Node, visit func(key, value *yaml.Node)) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		if key := node.Content[idx]; key.ShortTag() != mergeTag {
			visit(key, node.Content[idx+1])
		}
	}
}

// closestKey returns the known key nearest to name by edit distance, if any
// is close enough to be a likely typo.
func closestKey(name string, known []string) (string, bool) {
	best, bestDistance := "", maxSuggestionDistance+1
	for _, candidate := range known {
		if distance := editdistance.Levenshtein(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best, best != ""
}
