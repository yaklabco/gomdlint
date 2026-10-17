package lint

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

type inlineDirective struct {
	line      int
	offset    int
	action    string
	rules     []string
	parameter string
}

var directivePattern = regexp.MustCompile(`(?is)<!--\s*markdownlint-` +
	`(configure-file|disable-next-line|disable-line|disable-file|enable-file|disable|enable|capture|restore)\b(.*?)-->`)

func collectDirectives(file *mdast.FileSnapshot, registry *Registry) []inlineDirective {
	nodes := mdast.FindByKind(file.Root, mdast.NodeHTMLBlock)
	nodes = append(nodes, mdast.FindByKind(file.Root, mdast.NodeHTMLInline)...)
	directives := make([]inlineDirective, 0, len(nodes))
	for _, node := range nodes {
		rng := node.SourceRange()
		for _, match := range directivePattern.FindAllSubmatchIndex(node.Text(), -1) {
			content := node.Text()
			offset := rng.StartOffset + match[0]
			line, _ := file.LineAt(offset)
			parameter := string(content[match[4]:match[5]])
			ruleIDs, _ := directiveRuleIDs(parameter, registry)
			directives = append(directives, inlineDirective{
				line: line, offset: offset,
				action:    strings.ToLower(string(content[match[2]:match[3]])),
				rules:     ruleIDs,
				parameter: parameter,
			})
		}
	}
	slices.SortFunc(directives, func(a, b inlineDirective) int { return a.offset - b.offset })
	return directives
}

func configureFile(directives []inlineDirective, registry *Registry, cfg *config.Config) (*config.Config, error) {
	var result *config.Config
	for _, directive := range directives {
		if directive.action != "configure-file" {
			continue
		}
		if result == nil {
			result = cfg.Clone()
			if result == nil {
				result = config.NewConfig()
			}
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(directive.parameter), &raw); err != nil {
			return nil, fmt.Errorf("markdownlint-configure-file at line %d: %w", directive.line, err)
		}
		if raw == nil {
			return nil, fmt.Errorf("markdownlint-configure-file at line %d must contain a JSON object", directive.line)
		}
		keys := slices.Sorted(maps.Keys(raw))
		slices.SortStableFunc(keys, func(a, b string) int {
			_, aRule := registry.Lookup(a)
			_, bRule := registry.Lookup(b)
			if aRule == bRule {
				return 0
			}
			if aRule {
				return 1
			}
			return -1
		})
		for _, key := range keys {
			value := raw[key]
			if strings.EqualFold(key, "default") {
				if result.RuleDefaults == nil {
					result.RuleDefaults = make(map[string]config.RuleConfig)
				}
				for _, id := range registry.IDs() {
					baseline := value
					if IsGomdlintOnlyRule(id) {
						baseline = false
					}
					result.RuleDefaults[id] = config.MarkdownlintRuleConfig(id, baseline)
				}
				continue
			}
			ids, known := configureFileRuleIDs(key, registry)
			if !known {
				return nil, fmt.Errorf("markdownlint-configure-file at line %d: %s", directive.line, registry.UnknownKeyText(key))
			}
			for _, id := range ids {
				converted := config.MarkdownlintRuleConfig(id, value)
				converted.AutoFix = result.Rules[id].AutoFix
				result.Rules[id] = converted
			}
		}
	}
	if result == nil {
		return cfg, nil
	}
	return result, nil
}

// configureFileRuleIDs returns the IDs of the registered rules that key, a
// key of a markdownlint-configure-file object, names, and whether the key is
// known. Like markdownlint, which looks each key up whole after only
// upper-casing it, it reads the key as one word: unlike the parameter of a
// markdownlint-disable comment, a key of several words names no rules, and
// an empty key names none rather than all.
func configureFileRuleIDs(key string, registry *Registry) ([]string, bool) {
	if !IsSingleWord(key) {
		return nil, false
	}
	return directiveRuleIDs(key, registry)
}

// directiveRuleIDs returns the IDs of the registered rules that the words of
// parameter name, or of every rule when it has no words, and whether any
// word is known. Each word is a rule ID, name or alias, or a tag, in any
// case, as in markdownlint. Words that are not known are ignored.
func directiveRuleIDs(parameter string, registry *Registry) ([]string, bool) {
	items := strings.Fields(parameter)
	if len(items) == 0 {
		return registry.IDs(), true
	}
	ids := make(map[string]bool)
	known := false
	for _, item := range items {
		rules, itemKnown := directiveRules(item, registry)
		known = known || itemKnown
		for _, rule := range rules {
			ids[rule.ID()] = true
		}
	}
	return slices.Sorted(maps.Keys(ids)), known
}

// directiveRules returns the registered rules that one word of a
// markdownlint comment names, and whether the word is known. The word names
// the rule it identifies and the rules its tag covers. Tags come from the
// rules' catalog entries, which carry markdownlint's tags. A word that is an
// identifier or a tag in the catalog names only those rules, even when none
// of them is registered, so that a registry holding some of the rules
// resolves it as the full one does. Any other word falls back to the tags
// that rules report themselves through Rule.Tags, so that gomdlint's former
// tags, such as style, keep working for now.
func directiveRules(word string, registry *Registry) ([]Rule, bool) {
	rules := registry.RulesByTag(word)
	rule, isRule := registry.Lookup(word)
	if isRule {
		rules = append(rules, rule)
	}
	if isRule || registry.inCatalog(word) {
		return rules, true
	}

	rules = registry.rulesByOwnTag(word)
	return rules, len(rules) > 0
}

// directiveStates computes rule enablement by original source line. File directives
// apply first, persistent directives second, and line overrides last.
func directiveStates(file *mdast.FileSnapshot, registry *Registry, cfg *config.Config) map[string][]bool {
	directives := collectDirectives(file, registry)
	state := make(map[string]bool)
	states := make(map[string][]bool)
	for _, rule := range registry.Rules() {
		state[rule.ID()] = resolveRule(rule, cfg).Enabled
		states[rule.ID()] = make([]bool, len(file.Lines)+1)
	}
	set := func(directive inlineDirective) {
		for _, id := range directive.rules {
			state[id] = strings.HasPrefix(directive.action, "enable")
		}
	}
	for _, directive := range directives {
		if directive.action == "disable-file" || directive.action == "enable-file" {
			set(directive)
		}
	}
	captured := maps.Clone(state)
	next := 0
	for line := 1; line <= len(file.Lines); line++ {
		for next < len(directives) && directives[next].line == line {
			directive := directives[next]
			switch directive.action {
			case "disable", "enable":
				set(directive)
			case "capture":
				captured = maps.Clone(state)
			case "restore":
				state = maps.Clone(captured)
			}
			next++
		}
		for id, enabled := range state {
			states[id][line] = enabled && line > file.FrontMatterLines
		}
	}
	for _, directive := range directives {
		line := directive.line
		switch directive.action {
		case "disable-next-line":
			line++
		case "disable-line":
		default:
			continue
		}
		if line <= len(file.Lines) {
			for _, id := range directive.rules {
				states[id][line] = false
			}
		}
	}
	return states
}
