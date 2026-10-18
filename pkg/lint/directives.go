package lint

import (
	"cmp"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

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

// jsWhitespace matches one character that JavaScript's \s matches, as in
// markdownlint's regular expressions: tab, line feed, vertical tab, form
// feed, carriage return, the line and paragraph separators, the byte order
// mark and every space separator, such as the no-break space.
const jsWhitespace = `[\t\n\v\f\r\x{2028}\x{2029}\x{feff}\p{Zs}]`

// isJSWhitespace reports whether char is one that jsWhitespace matches.
// Unlike unicode.IsSpace, it reports true for the byte order mark and false
// for the next-line character U+0085.
func isJSWhitespace(char rune) bool {
	switch char {
	case '\t', '\n', '\v', '\f', '\r', '\u2028', '\u2029', '\ufeff':
		return true
	default:
		return unicode.Is(unicode.Zs, char)
	}
}

// directiveStart matches the start of a markdownlint comment as markdownlint
// 0.41.1's inlineCommentStartRe (helpers/helpers.cjs) does: "<!--", any
// whitespace, "markdownlint-", the comment's kind, and whitespace or "-->"
// right after the kind. A comment whose kind is followed by anything else,
// such as <!-- markdownlint-disablex --> or
// <!-- markdownlint-disable-next-lines -->, is plain text.
var directiveStart = regexp.MustCompile(directiveStartPattern())

// directiveStartPattern returns the pattern of directiveStart. Like a
// JavaScript regular expression with the i flag and without the u flag, it
// ignores the case of ASCII letters only.
func directiveStartPattern() string {
	kinds := []string{
		"configure-file", "disable-next-line", "disable-line", "disable-file", "enable-file",
		"disable", "enable", "capture", "restore",
	}
	for idx, kind := range kinds {
		kinds[idx] = caseless(kind)
	}
	return `<!--` + jsWhitespace + `*` + caseless("markdownlint-") +
		`(` + strings.Join(kinds, "|") + `)(?:` + jsWhitespace + `|-->)`
}

// caseless returns a pattern that matches word, made of lower-case ASCII
// letters and hyphens, with each letter in either case. Go's (?i) would
// also match the long s for s and the Kelvin sign for k, which
// JavaScript's i flag does not.
func caseless(word string) string {
	var pattern strings.Builder
	for _, char := range []byte(word) {
		if 'a' <= char && char <= 'z' {
			pattern.Write([]byte{'[', char, char - 'a' + 'A', ']'})
		} else {
			pattern.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	return pattern.String()
}

// directiveMatch is a markdownlint comment found in a piece of text, by
// offsets into it.
type directiveMatch struct {
	start   int    // The offset of "<!--".
	kind    string // The comment's kind in lower case, such as disable.
	kindEnd int    // The offset right after the kind, where the parameter starts.
	close   int    // The offset of the "-->" that ends the parameter.
}

// directiveMatches yields the markdownlint comments in text, in order, read
// as markdownlint 0.41.1 reads them (handleInlineConfig in
// lib/markdownlint.mjs): a comment starts where directiveStart matches, and
// its parameter runs from the end of its kind to the next "-->". The search
// for the next comment resumes right after the whitespace or "-->" that
// ends the kind, so a comment within the parameter of another counts too,
// and it stops at a comment without a closing "-->". Comments within one
// parameter share its "-->", which is looked for once, so the search takes
// time in proportion to the length of text.
func directiveMatches(text string) iter.Seq[directiveMatch] {
	return func(yield func(directiveMatch) bool) {
		closing := -1 // The first "-->" at or after the kind of the last comment.
		for pos := 0; pos < len(text); {
			loc := directiveStart.FindStringSubmatchIndex(text[pos:])
			if loc == nil {
				return
			}
			kindStart, kindEnd := pos+loc[2], pos+loc[3]
			if closing < kindEnd {
				next := strings.Index(text[kindEnd:], "-->")
				if next < 0 {
					return
				}
				closing = kindEnd + next
			}
			match := directiveMatch{
				start:   pos + loc[0],
				kind:    strings.ToLower(text[kindStart:kindEnd]),
				kindEnd: kindEnd,
				close:   closing,
			}
			if !yield(match) {
				return
			}
			pos += loc[1]
		}
	}
}

// findDirectives returns the comments that directiveMatches yields.
func findDirectives(text string) []directiveMatch {
	return slices.Collect(directiveMatches(text))
}

// HasInlineDirective reports whether text holds a markdownlint comment that
// turns rules on or off, read as the engine reads comments: one of any kind
// but configure-file. A rule whose fixes must not rewrite lines that such a
// comment exempts can look for one in the HTML of a file with it.
func HasInlineDirective(text []byte) bool {
	if !directiveStart.Match(text) {
		return false
	}
	for match := range directiveMatches(string(text)) {
		if match.kind != "configure-file" {
			return true
		}
	}
	return false
}

// collectDirectives returns the markdownlint comments of file in source
// order. Each comment that takes rules carries the IDs of the rules it
// names and the notices about its words. Only HTML nodes hold comments, so
// comments in code and front matter do not count.
func collectDirectives(file *mdast.FileSnapshot, registry *Registry) []inlineDirective {
	nodes := mdast.FindByKind(file.Root, mdast.NodeHTMLBlock)
	nodes = append(nodes, mdast.FindByKind(file.Root, mdast.NodeHTMLInline)...)
	directives := make([]inlineDirective, 0, len(nodes))
	resolver := &wordResolver{registry: registry}
	for _, node := range nodes {
		content := node.Text()
		if !directiveStart.Match(content) {
			continue
		}
		text := string(content) // The comments' parameters are slices of it.
		matches := findDirectives(text)
		found := make([]inlineDirective, len(matches))
		for idx, match := range matches {
			offset := node.SourceRange().StartOffset + match.start
			line, _ := file.LineAt(offset)
			found[idx] = inlineDirective{
				line: line, offset: offset,
				action:    match.kind,
				parameter: text[match.kindEnd:match.close],
			}
		}
		resolver.resolveComments(text, matches, found)
		directives = append(directives, found...)
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
			rules, meaning := lookupWord(key, registry)
			if meaning == unknownWord {
				suggestions := registry.markdownlintSuggestions(registry.Closest(key))
				return nil, fmt.Errorf("markdownlint-configure-file at line %d: %s", directive.line, registry.unknownKeyText(key, suggestions))
			}
			if meaning == formerTag {
				directive.notices = append(directive.notices, deprecatedTagNotice(registry, directive, key, rules))
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

// lookupWord returns the registered rules that word, one word of a
// markdownlint comment or a key of a markdownlint-configure-file object,
// names, and how it names them. markdownlint looks each up whole, after it
// upper-cases it with JavaScript's toUpperCase (see markdownlintKey). A word
// that is empty or holds whitespace names no rules: so a configure-file key
// of several words names none, and an empty key none rather than all. A
// comment's parameter splits into words at JavaScript's whitespace, so its
// words can hold only the whitespace that Go counts and JavaScript does not,
// the next-line character U+0085.
func lookupWord(word string, registry *Registry) ([]Rule, wordMeaning) {
	key, ok := markdownlintKey(word)
	if !ok || !IsSingleWord(word) {
		return nil, unknownWord
	}
	return directiveRules(key, registry)
}

// markdownlintKey returns word in lower case as markdownlint compares it
// with rule names, aliases and tags, which are all ASCII. markdownlint
// upper-cases the word with JavaScript's toUpperCase, which turns ten
// characters beyond ASCII into ASCII letters (see asciiUpperCase) and
// leaves the others, such as the Kelvin sign or the dotted capital I, as
// they are, unlike Go's case folding. It reports false for a word that
// holds such a character, which names nothing.
func markdownlintKey(word string) (string, bool) {
	ascii := true
	for idx := 0; idx < len(word) && ascii; idx++ {
		ascii = word[idx] < utf8.RuneSelf
	}
	if ascii {
		return strings.ToLower(word), true
	}

	var key strings.Builder
	for _, char := range word {
		if char < utf8.RuneSelf {
			key.WriteRune(unicode.ToLower(char))
			continue
		}
		letters, ok := asciiUpperCase(char)
		if !ok {
			return "", false
		}
		key.WriteString(letters)
	}
	return key.String(), true
}

// asciiUpperCase returns, in lower case, the ASCII letters that JavaScript's
// toUpperCase turns char into, for the ten characters beyond ASCII that it
// turns into nothing but ASCII letters: the sharp s, the dotless i, the long
// s and the Latin ligatures U+FB00 to U+FB06.
func asciiUpperCase(char rune) (string, bool) {
	switch char {
	case '\u00df':
		return "ss", true
	case '\u0131':
		return "i", true
	case '\u017f':
		return "s", true
	case '\ufb00':
		return "ff", true
	case '\ufb01':
		return "fi", true
	case '\ufb02':
		return "fl", true
	case '\ufb03':
		return "ffi", true
	case '\ufb04':
		return "ffl", true
	case '\ufb05', '\ufb06':
		return "st", true
	default:
		return "", false
	}
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

// markdownlintName returns the name that a markdownlint comment should use
// for rule, which markdownlint reads too: the name markdownlint lists first
// for the rule (RuleMeta.MarkdownlintName), or the rule's own name when that
// is markdownlint's first name, when markdownlint lacks the rule, or when
// the rule has no catalog entry.
func (r *Registry) markdownlintName(rule Rule) string {
	id, own := rule.ID(), rule.Name() // Outside the lock: a rule may use the registry.
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cmp.Or(r.meta[id].MarkdownlintName, own, id)
}

// markdownlintSuggestions returns suggestions, from Closest, with each rule
// named as a markdownlint comment should name it (see markdownlintName).
// Tags keep their names, which are markdownlint's.
func (r *Registry) markdownlintSuggestions(suggestions []Suggestion) []Suggestion {
	if len(suggestions) == 0 {
		return nil
	}
	named := make([]Suggestion, len(suggestions))
	r.mu.RLock()
	defer r.mu.RUnlock()
	for idx, suggestion := range suggestions {
		if !suggestion.IsTag() {
			suggestion.Name = cmp.Or(r.meta[suggestion.RuleID].MarkdownlintName, suggestion.Name)
		}
		named[idx] = suggestion
	}
	return named
}

// wordResolver resolves the words of the markdownlint comments of one file.
// It remembers the suggestions it offers for each unknown word, which a file
// may repeat in many comments.
type wordResolver struct {
	registry *Registry

	// closest maps a lower-cased unknown word to the suggestions for it:
	// Registry.Closest's, named as markdownlint names the rules. It is nil
	// until a comment has an unknown word.
	closest map[string][]Suggestion
}

// resolvedWord is a word of a markdownlint comment and what it names.
type resolvedWord struct {
	word    string
	rules   []Rule
	meaning wordMeaning
}

// resolveComments resolves the words of the comments of one piece of text,
// whose matches are in order, and records on each comment that takes rules
// the IDs of the rules it names, in ID order, or every rule when its
// parameter has no words, and the notices about its words. As in
// markdownlint, the words are the parts of a parameter between runs of
// JavaScript's whitespace, each a rule ID, name or alias, or a tag, in any
// case (see lookupWord), and words that are not known are ignored.
//
// A comment whose parameter holds the start of the next comment shares that
// comment's "-->", so its parameter is its own segment, up to the end of the
// next comment's kind, followed by the next comment's parameter. Walking
// backwards, each segment is split and resolved once: a comment names the
// rules of its segment and those of the next comment's parameter. A word
// that names no rule or tag, or that is a former tag, gets one notice, on
// the last comment before it that takes rules, once for a word that the
// comment holds more than once in any case. So the work and the notices grow
// with the length of text rather than with the square of the number of
// nested comments.
func (w *wordResolver) resolveComments(text string, matches []directiveMatch, directives []inlineDirective) {
	var (
		named   []string         // The IDs that the next comment's parameter names, sorted.
		pending [][]resolvedWord // Segments of later comments that take no rules, last first.
	)
	for idx := len(matches) - 1; idx >= 0; idx-- {
		match := matches[idx]
		end, shares := match.close, idx+1 < len(matches) && matches[idx+1].kindEnd <= match.close
		if shares {
			end = matches[idx+1].kindEnd
		} else {
			named, pending = nil, nil
		}
		segment := w.resolve(strings.FieldsFunc(text[match.kindEnd:end], isJSWhitespace))
		named = mergeIDs(named, segment)

		directive := &directives[idx]
		if !takesRules(directive.action) {
			pending = append(pending, segment)
			continue
		}
		directive.rules = named
		if !shares && len(segment) == 0 {
			directive.rules = w.registry.IDs()
		}
		directive.notices = w.notices(directive, segment, pending)
		pending = nil
	}
}

// resolve returns what each of words names.
func (w *wordResolver) resolve(words []string) []resolvedWord {
	resolved := make([]resolvedWord, len(words))
	for idx, word := range words {
		rules, meaning := lookupWord(word, w.registry)
		resolved[idx] = resolvedWord{word: word, rules: rules, meaning: meaning}
	}
	return resolved
}

// mergeIDs returns ids, which is sorted, together with the IDs of the rules
// that words name, sorted and without duplicates. It returns ids itself
// when words name no rule.
func mergeIDs(ids []string, words []resolvedWord) []string {
	var added []string
	for _, word := range words {
		for _, rule := range word.rules {
			added = append(added, rule.ID())
		}
	}
	if len(added) == 0 {
		return ids
	}
	merged := append(slices.Clone(ids), added...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// notices returns the notices of directive about the words of its segment,
// then about the words of the segments in pending, which hold them last
// first: one for each word that names no rule or tag and for each former
// tag, once for a word that they hold more than once in any case.
func (w *wordResolver) notices(directive *inlineDirective, segment []resolvedWord, pending [][]resolvedWord) []Notice {
	var notices []Notice
	var noticed map[string]bool // Lower-cased words with a notice, once one has.
	add := func(word resolvedWord) {
		if word.meaning == knownWord {
			return
		}
		key := foldKey(word.word)
		if noticed[key] {
			return
		}
		if noticed == nil {
			noticed = make(map[string]bool)
		}
		noticed[key] = true
		if word.meaning == formerTag {
			notices = append(notices, deprecatedTagNotice(w.registry, directive, word.word, word.rules))
			return
		}
		notices = append(notices, unknownRuleNotice(directive, word.word, w.suggestions(key)))
	}
	for _, word := range segment {
		add(word)
	}
	for idx := len(pending) - 1; idx >= 0; idx-- {
		for _, word := range pending[idx] {
			add(word)
		}
	}
	return notices
}

// suggestions returns the rules and tags that key, a lower-cased unknown
// word, most likely means, named as markdownlint names them, computing them
// once per word.
func (w *wordResolver) suggestions(key string) []Suggestion {
	suggestions, ok := w.closest[key]
	if !ok {
		suggestions = w.registry.markdownlintSuggestions(w.registry.Closest(key))
		if w.closest == nil {
			w.closest = make(map[string][]Suggestion)
		}
		w.closest[key] = suggestions
	}
	return suggestions
}

// unknownRuleNotice is the notice for word, a word of directive that names
// no rule or tag, which the comment ignores. It offers suggestions, the
// rules and tags that word most likely means, named as markdownlint names
// them.
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
// the rules, which the comment can name instead, as markdownlint names them
// (see Registry.markdownlintName), so that markdownlint reads them too.
func deprecatedTagNotice(registry *Registry, directive *inlineDirective, word string, rules []Rule) Notice {
	names := make([]string, len(rules))
	for idx, rule := range rules {
		names[idx] = registry.markdownlintName(rule)
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
