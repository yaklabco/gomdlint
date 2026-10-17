package configloader

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// maxSuggestionDistance is the largest edit distance at which an unknown key
// is reported as a likely typo of a known one.
const maxSuggestionDistance = 2

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

// unknownKeyWarnings reports keys in a configuration file that gomdlint does
// not read. YAML decoding drops them silently, so a typo such as "flavour"
// would otherwise leave the default in force without a trace.
func unknownKeyWarnings(path string, content []byte) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil || len(doc.Content) == 0 {
		return nil // Decoding errors are reported when the file is decoded.
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
// Other node kinds have no keys.
func forEachKey(node *yaml.Node, visit func(key, value *yaml.Node)) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		visit(node.Content[idx], node.Content[idx+1])
	}
}

// closestKey returns the known key nearest to name by edit distance, if any
// is close enough to be a likely typo.
func closestKey(name string, known []string) (string, bool) {
	best, bestDistance := "", maxSuggestionDistance+1
	for _, candidate := range known {
		if distance := editDistance(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best, best != ""
}

// editDistance returns the Levenshtein distance between two strings.
func editDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for col := range previous {
		previous[col] = col
	}
	for row := 1; row <= len(left); row++ {
		current[0] = row
		for col := 1; col <= len(right); col++ {
			cost := 1
			if left[row-1] == right[col-1] {
				cost = 0
			}
			current[col] = min(previous[col]+1, current[col-1]+1, previous[col-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}
