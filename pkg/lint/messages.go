package lint

import (
	"fmt"
	"strconv"
	"strings"
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

// joinChoices joins words into a list in prose, with conjunction before the
// last word: "a", "a or b", "a, b or c".
func joinChoices(words []string, conjunction string) string {
	last := len(words) - 1
	if last < 1 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:last], ", ") + " " + conjunction + " " + words[last]
}
