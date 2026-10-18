package configloader

import (
	"cmp"
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
// are reported too, and so are the options of a rule that it cannot use,
// whether written under options or directly under the rule key (see
// ruleEntryWarnings). The keys checked are those that decoding reads, after
// YAML's merge keys and aliases; see mappingEntries.
func unknownKeyWarnings(path string, doc *yaml.Node, registry *lint.Registry) []string {
	if len(doc.Content) == 0 {
		return nil // An empty file has no keys.
	}

	topKeys := yamlKeys(config.Config{})
	backupKeys := yamlKeys(config.BackupsConfig{})

	var warnings []string
	warn := func(entry keyEntry, message string) {
		warnings = append(warnings, fmt.Sprintf("%s:%d: %s%s", path, entry.line, message, entry.through))
	}

	for _, entry := range mappingEntries(doc.Content[0], "") {
		name := entry.name
		if flag, ok := commandLineOnlyKeys[name]; ok {
			hint := "use the " + flag + " flag"
			if envVar := GetEnvVarName(name); envVar != "" {
				// The environment sets a lasting default, as a config file would.
				hint += " or the " + envVar + " environment variable"
			}
			warn(entry, fmt.Sprintf("%q cannot be set in a config file; %s", name, hint))
			continue
		}
		if !slices.Contains(topKeys, name) {
			warn(entry, unknownKeyMessage("", name, topKeys))
			continue
		}

		switch name {
		case "backups":
			for _, nested := range mappingEntries(entry.value, entry.through) {
				if !slices.Contains(backupKeys, nested.name) {
					warn(nested, unknownKeyMessage("backups.", nested.name, backupKeys))
				}
			}
		case "rules", "rule_defaults":
			for _, rule := range mappingEntries(entry.value, entry.through) {
				known, isRule := registry.Lookup(rule.name)
				if !isRule {
					// The settings of a key that names no rule configure
					// nothing, so there is nothing to check them against.
					warn(rule, unknownRuleMessage(registry, rule.name))
					continue
				}
				prefix := name + "." + rule.name + "."
				for _, nested := range mappingEntries(rule.value, rule.through) {
					for _, warning := range ruleEntryWarnings(registry, known, prefix, nested) {
						warn(warning.at, warning.message)
					}
				}
			}
		}
	}

	return warnings
}

// ruleEntryWarnings describes the problems of entry, a key under the
// settings of rule, which a warning names after prefix, such as
// "rules.line-length.". A rule setting gets a warning when the rule has an
// option spelled like it, since the setting wins (see
// config.RuleConfig.UnmarshalYAML), and the options under options get
// optionWarnings. Any other key is an option written directly under the
// rule key, which gets optionWarnings too, unless it looks like a typo of
// a rule setting rather than of an option (see misspelledSetting).
func ruleEntryWarnings(registry *lint.Registry, rule lint.Rule, prefix string, entry keyEntry) []optionWarning {
	settingKeys := config.RuleSettingKeys()
	if !slices.Contains(settingKeys, entry.name) {
		if misspelledSetting(registry, rule, entry.name) {
			return []optionWarning{{at: entry, message: unknownKeyMessage(prefix, entry.name, settingKeys)}}
		}
		return optionWarnings(registry, rule, entry)
	}

	var warnings []optionWarning
	if option, ok := declaredOption(registry, rule, entry.name); ok {
		warnings = append(warnings, optionWarning{at: entry, message: fmt.Sprintf(
			"%q is a rule setting, not %s option %q; to set the option, write it under %q",
			prefix+entry.name, rule.Name(), option, prefix+"options")})
	}
	if entry.name == "options" {
		for _, option := range mappingEntries(entry.value, entry.through) {
			warnings = append(warnings, optionWarnings(registry, rule, option)...)
		}
	}
	return warnings
}

// unknownOptionIssue returns the issue that name spells no option of rule,
// with the closest spellings, when the rule declares its options and none
// of them is spelled so. Only the name counts: the value asked about is
// null, so no value is ever decoded to tell.
func unknownOptionIssue(registry *lint.Registry, rule lint.Rule, name string) (lint.OptionIssue, bool) {
	_, issues, _ := registry.NormalizeOptions(rule.ID(), map[string]any{name: nil})
	for _, issue := range issues {
		if issue.Kind == lint.OptionIssueUnknown {
			return issue, true
		}
	}
	return lint.OptionIssue{}, false
}

// unknownOption reports whether rule declares its options and none of them
// is spelled name. A rule that declares none may read any name.
func unknownOption(registry *lint.Registry, rule lint.Rule, name string) bool {
	_, unknown := unknownOptionIssue(registry, rule, name)
	return unknown
}

// declaredOption returns the canonical name of the option of rule that
// spelling spells, by its name or an alias, when the rule declares its
// options and one of them is spelled so.
func declaredOption(registry *lint.Registry, rule lint.Rule, spelling string) (string, bool) {
	normalized, issues, declared := registry.NormalizeOptions(rule.ID(), map[string]any{spelling: nil})
	switch {
	case !declared:
		return "", false
	case len(issues) > 0:
		// One key brings at most one issue. A known spelling's issue
		// names its option.
		if issues[0].Kind == lint.OptionIssueUnknown {
			return "", false
		}
		return issues[0].Option.Name, true
	}
	for name := range normalized {
		return name, true
	}
	return "", false
}

// misspelledSetting reports whether name, a key written directly under the
// settings of rule, which reads as an option of rule, more likely misspells
// a rule setting, such as severty for severity: it spells no option of the
// rule and is close to none, but close to a setting. A rule that does not
// declare its options has none to tell a typo by.
func misspelledSetting(registry *lint.Registry, rule lint.Rule, name string) bool {
	issue, unknown := unknownOptionIssue(registry, rule, name)
	if !unknown || len(issue.Suggestions) > 0 {
		return false
	}
	_, near := closestKey(name, config.RuleSettingKeys())
	return near
}

// optionWarning is a warning about an option of a rule, with the entry
// whose place it names.
type optionWarning struct {
	at      keyEntry
	message string
}

// optionWarnings describes the problems of option, an option of rule,
// written under the rule's options or directly under the rule key: an
// option that the rule does not read, with the closest spellings, a value
// of the wrong type, or a list with items of the wrong type, which loading
// drops, as if the file did not set it, and a value that the option does
// not allow. A warning about an item of a list names the item's line. Two
// spellings of one option fail loading instead (see normalizeRuleOptions).
func optionWarnings(registry *lint.Registry, rule lint.Rule, option keyEntry) []optionWarning {
	// The name alone tells an unknown option. Loading never decodes the
	// value of an unknown key written directly under the rule key, which
	// may not decode, so its value is not read here either.
	if issue, unknown := unknownOptionIssue(registry, rule, option.name); unknown {
		return []optionWarning{{at: option, message: issue.Text(rule.Name())}}
	}
	var value any
	if err := option.value.Decode(&value); err != nil {
		return nil // Loading has decoded the value, so this does not happen.
	}
	_, issues, _ := registry.NormalizeOptions(rule.ID(), map[string]any{option.name: value})
	warnings := make([]optionWarning, 0, len(issues))
	for _, issue := range issues {
		at := option
		if line := itemLine(option.value, issue.Item); line > 0 {
			at.line = line
		}
		warnings = append(warnings, optionWarning{at: at, message: issue.Text(rule.Name())})
	}
	return warnings
}

// itemLine returns the line of item, counted from 1, of node, a sequence
// or an alias to one, or 0 when node holds no such item.
func itemLine(node *yaml.Node, item int) int {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node == nil || node.Kind != yaml.SequenceNode || item < 1 || item > len(node.Content) {
		return 0
	}
	return node.Content[item-1].Line
}

// keyEntry is a key that decoding reads from a mapping, with its value and
// the place where its entry is written.
type keyEntry struct {
	// name is the key as decoding reads it: an alias key reads as the
	// scalar it names.
	name string

	// value is the value that decoding takes for the key: the node of the
	// document when the entry is found, and otherwise decoding's copy.
	value *yaml.Node

	// line and column locate the key of the entry, or its value when the
	// entry is not found in the document.
	line, column int

	// through ends a warning about the key that a merge key or an alias
	// brought in, as in " (through the merge at line 4)". It is empty for a
	// key written in place, and for one whose way in is not known.
	through string
}

// mappingEntries returns the entries that decoding reads from node, a
// mapping or an alias to one, in the order in which they are written.
//
// Decoding itself settles which keys count: node is decoded into a map of
// nodes, which applies YAML's rules. A written key replaces a merged one, an
// earlier merged mapping replaces a later one, and an alias key reads as the
// scalar it names. The document only shows where each entry is written: the
// entry whose key reads as the name and whose value is the node decoding
// took. An entry that is not found is placed at its value, without a note on
// how it arrived.
//
// through says how node was reached, and applies to all its entries. When
// it is empty, an entry that a merge key of node brings in says so, and the
// entries of an alias say that. A mapping that decoding rejects, which
// loading has rejected already, yields the entries written in it.
func mappingEntries(node *yaml.Node, through string) []keyEntry {
	mapping := node
	if mapping != nil && mapping.Kind == yaml.AliasNode {
		if through == "" {
			through = fmt.Sprintf(" (through the alias at line %d)", mapping.Line)
		}
		mapping = mapping.Alias
	}
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}

	written := writtenEntries(mapping)
	var decoded map[string]yaml.Node
	if err := mapping.Decode(&decoded); err != nil {
		return ownEntries(written, through)
	}

	entries := make([]keyEntry, 0, len(decoded))
	for name, value := range decoded {
		entry := keyEntry{name: name, value: &value, line: value.Line, column: value.Column, through: through}
		if found := takenEntry(written, name, &value); found != nil {
			entry.value, entry.line, entry.column = found.value, found.key.Line, found.key.Column
			if entry.through == "" && found.merge != nil {
				entry.through = fmt.Sprintf(" (through the merge at line %d)", found.merge.Line)
			}
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(left, right keyEntry) int {
		return cmp.Or(
			cmp.Compare(left.line, right.line),
			cmp.Compare(left.column, right.column),
			cmp.Compare(left.name, right.name),
		)
	})
	return entries
}

// ownEntries returns the written entries of the mapping itself, without
// those that its merge keys bring in, as key entries.
func ownEntries(written []writtenEntry, through string) []keyEntry {
	var entries []keyEntry
	for _, entry := range written {
		if entry.merge == nil {
			entries = append(entries, keyEntry{
				name: entry.name, value: entry.value, line: entry.key.Line, column: entry.key.Column, through: through,
			})
		}
	}
	return entries
}

// writtenEntry is an entry written in a mapping, or in a mapping that one of
// its merge keys brings in, with its key read as decoding reads it.
type writtenEntry struct {
	name       string
	key, value *yaml.Node

	// merge is the merge key of the searched mapping through which the
	// entry arrives, or nil for an entry written in that mapping.
	merge *yaml.Node
}

// writtenEntries lists the entries written in mapping and in the mappings
// that its merge keys bring in: its own entries first, then each merged
// mapping in turn, with the mappings that it merges. They are only places
// to look for the entries that decoding takes. Each mapping is listed once,
// so a mapping that merges itself cannot loop.
func writtenEntries(mapping *yaml.Node) []writtenEntry {
	var entries []writtenEntry
	collectWrittenEntries(mapping, nil, make(map[*yaml.Node]bool), &entries)
	return entries
}

// collectWrittenEntries appends the entries written in node, a mapping or an
// alias to one, and in the mappings it merges, to entries, unless seen holds
// the mapping already. merge is the merge key that led to node, or nil.
func collectWrittenEntries(node, merge *yaml.Node, seen map[*yaml.Node]bool, entries *[]writtenEntry) {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node == nil || node.Kind != yaml.MappingNode || seen[node] {
		return
	}
	seen[node] = true

	var mergeKey, mergeValue *yaml.Node
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		key, value := node.Content[idx], node.Content[idx+1]
		if isMergeKey(key) {
			mergeKey, mergeValue = key, value
			continue
		}
		var name string
		if key.Decode(&name) == nil {
			*entries = append(*entries, writtenEntry{name: name, key: key, value: value, merge: merge})
		}
	}
	if mergeValue == nil {
		return
	}

	if merge == nil {
		merge = mergeKey
	}
	sources := []*yaml.Node{mergeValue}
	if mergeValue.Kind == yaml.SequenceNode {
		sources = mergeValue.Content
	}
	for _, source := range sources {
		collectWrittenEntries(source, merge, seen, entries)
	}
}

// takenEntry returns the written entry that decoding took for the key name:
// the one whose key reads as name and whose value is the node of which
// value is decoding's copy. It returns nil when no entry is.
func takenEntry(written []writtenEntry, name string, value *yaml.Node) *writtenEntry {
	for idx := range written {
		if entry := &written[idx]; entry.name == name && copyOf(value, entry.value) {
			return entry
		}
	}
	return nil
}

// copyOf reports whether decoded is decoding's copy of node. Decoding into a
// yaml.Node copies the node it reads, so the copy keeps its position, its
// tag and value, the target of an alias and the content of a collection.
func copyOf(decoded, node *yaml.Node) bool {
	return decoded.Kind == node.Kind && decoded.Line == node.Line && decoded.Column == node.Column &&
		decoded.Tag == node.Tag && decoded.Value == node.Value && decoded.Alias == node.Alias &&
		len(decoded.Content) == len(node.Content) && (len(node.Content) == 0 || &decoded.Content[0] == &node.Content[0])
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

// isMergeKey reports whether key is a merge key, as YAML decoding tells
// one: the plain scalar <<, or << tagged !!merge. A quoted "<<", like every
// key of a JSON config, is an ordinary key.
func isMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.Value == "<<" && key.ShortTag() == mergeTag
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
