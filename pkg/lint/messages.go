package lint

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// RuleLabel names a rule in a message that someone may quote in a bug
// report or look up in markdownlint's documentation: its name, then its ID
// in parentheses, as in "no-trailing-spaces (MD009)". A rule without a name
// is named by its ID alone.
func RuleLabel(id, name string) string {
	if name == "" {
		return id
	}
	return name + " (" + id + ")"
}

// UnknownRuleText describes key, which names no rule, for an error or a
// warning: `unknown rule "key"`, followed in parentheses by the rules and
// tags that key most likely means, as in `unknown rule "tables" (did you
// mean the tag "table"?)`. Pass the suggestions of Closest or ClosestRules.
func UnknownRuleText(key string, suggestions []Suggestion) string {
	text := fmt.Sprintf("unknown rule %q", key)
	if len(suggestions) == 0 {
		return text
	}

	choices := make([]string, len(suggestions))
	for idx, suggestion := range suggestions {
		choices[idx] = strconv.Quote(suggestion.Name)
		if suggestion.IsTag() {
			choices[idx] = "the tag " + choices[idx]
		}
	}
	return text + " (did you mean " + joinChoices(choices, "or") + "?)"
}

// IsSingleWord reports whether key is one word: not empty, and free of
// whitespace. Only such a key can name a rule or a tag in a markdownlint
// configuration object, such as a .markdownlint.json file or a
// markdownlint-configure-file comment: markdownlint looks those keys up
// whole, ignoring only case, and no rule identifier or tag holds
// whitespace. Lookup and RulesByTag ignore surrounding whitespace, so check
// a key with IsSingleWord before resolving it as markdownlint would.
func IsSingleWord(key string) bool {
	return key != "" && !strings.ContainsFunc(key, unicode.IsSpace)
}

// UnknownKeyText describes key, a key of a markdownlint configuration
// object that names no rule or tag, for an error or a warning. Like
// UnknownRuleText, it offers the closest rules and tags. A key of several
// words that each name a rule or tag gets a note instead: such a key names
// nothing, because markdownlint reads it as one name.
func (r *Registry) UnknownKeyText(key string) string {
	words := strings.Fields(key)
	if len(words) < 2 || !r.allKnown(words) {
		return UnknownRuleText(key, r.Closest(key))
	}

	quoted := make([]string, len(words))
	for idx, word := range words {
		quoted[idx] = strconv.Quote(word)
	}
	return fmt.Sprintf("unknown rule %q (a key names one rule or tag; write %s as separate keys)",
		key, joinChoices(quoted, "and"))
}

// allKnown reports whether every word is a rule identifier or a tag: an
// identifier of a registered rule, or any identifier or tag of the catalog.
func (r *Registry) allKnown(words []string) bool {
	for _, word := range words {
		if _, isRule := r.Lookup(word); !isRule && !r.inCatalog(word) {
			return false
		}
	}
	return true
}

// joinChoices joins words into a list in prose, with conjunction before the
// last word: "a", "a or b", "a, b or c".
func joinChoices(words []string, conjunction string) string {
	last := len(words) - 1
	if last < 1 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:last], ", ") + " " + conjunction + " " + words[last]
}
