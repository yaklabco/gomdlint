package lint

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// inlineDirective is a markdownlint comment, such as
// <!-- markdownlint-disable MD009 -->.
type inlineDirective struct {
	line      int
	offset    int
	action    string
	parameter string

	// rules holds the IDs of the rules that the comment names, for an
	// action that takes rules (see takesRules): every rule when the
	// parameter has no words.
	rules []string

	// notices lists the problems with the words of the comment, in the
	// order of the words, for an action that takes rules, and with the keys
	// of a configure-file comment (see configureFile).
	notices []Notice
}

// kind names the comment's kind, in lower case, as in markdownlint-disable.
func (d *inlineDirective) kind() string {
	return "markdownlint-" + d.action
}

// takesRules reports whether the parameter of a comment with action names
// rules. markdownlint ignores the parameter of capture and restore, and
// configure-file takes a JSON object.
func takesRules(action string) bool {
	switch action {
	case "capture", "restore", "configure-file":
		return false
	default:
		return true
	}
}

var directivePattern = regexp.MustCompile(`(?is)<!--\s*markdownlint-` +
	`(configure-file|disable-next-line|disable-line|disable-file|enable-file|disable|enable|capture|restore)\b(.*?)-->`)

// collectDirectives returns the markdownlint comments of file in source
// order. Each comment that takes rules carries the IDs of the rules it
// names and the notices about its words.
func collectDirectives(file *mdast.FileSnapshot, registry *Registry) []inlineDirective {
	nodes := mdast.FindByKind(file.Root, mdast.NodeHTMLBlock)
	nodes = append(nodes, mdast.FindByKind(file.Root, mdast.NodeHTMLInline)...)
	directives := make([]inlineDirective, 0, len(nodes))
	resolver := &wordResolver{registry: registry}
	for _, node := range nodes {
		rng := node.SourceRange()
		content := node.Text()
		for _, match := range directivePattern.FindAllSubmatchIndex(content, -1) {
			offset := rng.StartOffset + match[0]
			line, _ := file.LineAt(offset)
			directive := inlineDirective{
				line: line, offset: offset,
				action:    strings.ToLower(string(content[match[2]:match[3]])),
				parameter: string(content[match[4]:match[5]]),
			}
			if takesRules(directive.action) {
				directive.rules, directive.notices = resolver.ruleList(&directive)
			}
			directives = append(directives, directive)
		}
	}
	slices.SortFunc(directives, func(a, b inlineDirective) int { return a.offset - b.offset })
	return directives
}

// configureFile applies the markdownlint-configure-file comments among
// directives to cfg and returns the result, which is cfg itself when there
// are none. A key that names no rule or tag is an error. Each key that is a
// deprecated tag gets a notice, which configureFile records on its comment.
func configureFile(directives []inlineDirective, registry *Registry, cfg *config.Config) (*config.Config, error) {
	var result *config.Config
	for idx := range directives {
		directive := &directives[idx]
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
			rules, meaning := configureFileRules(key, registry)
			if meaning == unknownWord {
				return nil, fmt.Errorf("markdownlint-configure-file at line %d: %s", directive.line, registry.UnknownKeyText(key))
			}
			if meaning == formerTag {
				directive.notices = append(directive.notices, deprecatedTagNotice(directive, key, rules))
			}
			for _, id := range sortedIDs(rules) {
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

// configureFileRules returns the registered rules that key, a key of a
// markdownlint-configure-file object, names, and how it names them. Like
// markdownlint, which looks each key up whole after only upper-casing it,
// it reads the key as one word: unlike the parameter of a
// markdownlint-disable comment, a key of several words names no rules, and
// an empty key names none rather than all.
func configureFileRules(key string, registry *Registry) ([]Rule, wordMeaning) {
	if !IsSingleWord(key) {
		return nil, unknownWord
	}
	return directiveRules(key, registry)
}

// wordMeaning says how a word of a markdownlint comment names rules.
type wordMeaning int

const (
	// unknownWord names no rule and no tag.
	unknownWord wordMeaning = iota

	// knownWord names rules, or rules that are not registered, without a
	// problem: a rule ID, name or alias, a tag of a catalog entry, or a tag
	// that a rule reports itself that is not one of gomdlint's former tags.
	knownWord

	// formerTag is one of gomdlint's former tags (see isFormerTag). It
	// names the rules that report it themselves, through Rule.Tags, but is
	// deprecated.
	formerTag
)

// isFormerTag reports whether word is one of gomdlint's former tags, in any
// case: the tags that its built-in rules report themselves that
// markdownlint does not have. Directives resolved tags through Rule.Tags
// before they used markdownlint's tags, so these still work there, with a
// notice, until a later release.
func isFormerTag(word string) bool {
	switch foldKey(word) {
	case "style", "lists", "layout", "metadata", "gfm", "tables":
		return true
	default:
		return false
	}
}

// directiveRules returns the registered rules that one word of a
// markdownlint comment names, and how it names them. The word names the
// rule it identifies and the rules its tag covers. Tags come from the
// rules' catalog entries, which carry markdownlint's tags. A word that is an
// identifier or a tag in the catalog names only those rules, even when none
// of them is registered, so that a registry holding some of the rules
// resolves it as the full one does. Any other word falls back to the tags
// that rules report themselves through Rule.Tags, so that gomdlint's former
// tags, such as style, keep working for now, as do the tags of rules
// without catalog entries.
func directiveRules(word string, registry *Registry) ([]Rule, wordMeaning) {
	rules := registry.RulesByTag(word)
	rule, isRule := registry.Lookup(word)
	if isRule {
		rules = append(rules, rule)
	}
	if isRule || registry.inCatalog(word) {
		return rules, knownWord
	}

	rules = registry.rulesByOwnTag(word)
	switch {
	case len(rules) == 0:
		return nil, unknownWord
	case isFormerTag(word):
		return rules, formerTag
	default:
		return rules, knownWord
	}
}

// sortedIDs returns the IDs of rules, sorted and without duplicates.
func sortedIDs(rules []Rule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID())
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

// wordResolver resolves the words of the markdownlint comments of one file.
// It remembers the suggestions it offers for each unknown word, which a file
// may repeat in many comments.
type wordResolver struct {
	registry *Registry

	// closest maps a lower-cased unknown word to Registry.Closest's
	// suggestions for it. It is nil until a comment has an unknown word.
	closest map[string][]Suggestion
}

// ruleList returns the IDs of the rules that the words of directive's
// parameter name, in ID order, or every rule when it has no words. Each word
// is a rule ID, name or alias, or a tag, in any case, as in markdownlint.
// Words that are not known are ignored. It also returns, in the order of
// the words, a notice for each word that names no rule or tag and for each
// deprecated tag, once for a word that the comment repeats in any case.
func (w *wordResolver) ruleList(directive *inlineDirective) ([]string, []Notice) {
	words := strings.Fields(directive.parameter)
	if len(words) == 0 {
		return w.registry.IDs(), nil
	}

	var named []Rule
	var notices []Notice
	var noticed map[string]bool // Lower-cased words with a notice, once one has.
	for _, word := range words {
		rules, meaning := directiveRules(word, w.registry)
		named = append(named, rules...)
		if meaning == knownWord {
			continue
		}

		key := foldKey(word)
		if noticed[key] {
			continue
		}
		if noticed == nil {
			noticed = make(map[string]bool)
		}
		noticed[key] = true
		if meaning == formerTag {
			notices = append(notices, deprecatedTagNotice(directive, word, rules))
			continue
		}
		suggestions, ok := w.closest[key]
		if !ok {
			suggestions = w.registry.Closest(key)
			if w.closest == nil {
				w.closest = make(map[string][]Suggestion)
			}
			w.closest[key] = suggestions
		}
		notices = append(notices, unknownRuleNotice(directive, word, suggestions))
	}
	return sortedIDs(named), notices
}

// unknownRuleNotice is the notice for word, a word of directive that names
// no rule or tag, which the comment ignores. It offers suggestions, the
// rules and tags that word most likely means.
func unknownRuleNotice(directive *inlineDirective, word string, suggestions []Suggestion) Notice {
	var names []string // None without suggestions, rather than an empty list.
	if len(suggestions) > 0 {
		names = make([]string, len(suggestions))
		for idx, suggestion := range suggestions {
			names[idx] = suggestion.Name
		}
	}
	kind := directive.kind()
	return Notice{
		Kind:        NoticeUnknownRule,
		Line:        directive.line,
		Directive:   kind,
		Name:        word,
		Suggestions: names,
		Message:     kind + ": " + UnknownRuleText(word, suggestions) + "; it is ignored",
	}
}

// deprecatedTagNotice is the notice for word, a word of directive that is
// one of gomdlint's former tags and names rules, in rule ID order. It names
// the rules, which the comment can name instead.
func deprecatedTagNotice(directive *inlineDirective, word string, rules []Rule) Notice {
	names := make([]string, len(rules))
	for idx, rule := range rules {
		names[idx] = cmp.Or(rule.Name(), rule.ID())
	}
	kind := directive.kind()
	return Notice{
		Kind:      NoticeDeprecatedTag,
		Line:      directive.line,
		Directive: kind,
		Name:      word,
		Rules:     names,
		Message:   kind + ": " + deprecatedTagText(word, names),
	}
}

// directiveNotices returns the notices of directives, in source order.
func directiveNotices(directives []inlineDirective) []Notice {
	var notices []Notice
	for idx := range directives {
		notices = append(notices, directives[idx].notices...)
	}
	return notices
}

// directiveStates computes rule enablement by original source line. File directives
// apply first, persistent directives second, and line overrides last.
func directiveStates(file *mdast.FileSnapshot, directives []inlineDirective, registry *Registry, cfg *config.Config) map[string][]bool {
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
