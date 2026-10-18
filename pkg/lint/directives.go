package lint

import (
	"cmp"
	"errors"
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

// configureFileAction is the action of a markdownlint-configure-file
// comment, whose parameter is a JSON object rather than rule names.
const configureFileAction = "configure-file"

// takesRules reports whether the parameter of a comment with action names
// rules. markdownlint ignores the parameter of capture and restore, and
// configure-file takes a JSON object.
func takesRules(action string) bool {
	switch action {
	case "capture", "restore", configureFileAction:
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
		configureFileAction, "disable-next-line", "disable-line", "disable-file", "enable-file",
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
		if match.kind != configureFileAction {
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
// are none. As in markdownlint 0.41.1, the comments act as one
// configuration object (see MarkdownlintSettings), which each comment's
// keys join in the order they are written: a key that the object holds
// already keeps its place and takes the new value. The object then applies
// to cfg. Its default key, the first key named default in any case, gives
// every rule a baseline under RuleDefaults. Its other keys apply in order,
// each replacing the settings under Rules of the rules it names, so a later
// key wins over an earlier one for the rules both name.
//
// markdownlint spreads the comments over its own configuration object, in
// which a key that its configuration holds already keeps the
// configuration's place. That holds for the default key of the object
// {"default": true}, which markdownlint assumes without a configuration
// file, so there a comment's Default key, written in another case, loses to
// it. gomdlint applies the comments over its settings, which are no such
// object, so their keys apply in the order the comments write them.
//
// A key that names no rule or tag is an error. A key that is a deprecated
// tag gets a notice, which configureFile records on its comment, once for a
// tag that the comment's keys repeat in any spelling that markdownlint reads
// as the same word.
func configureFile(directives []inlineDirective, registry *Registry, cfg *config.Config) (*config.Config, error) {
	var settings MarkdownlintSettings
	lines := make(map[string]int) // key -> the line of the comment that last set it
	configured := false
	for idx := range directives {
		directive := &directives[idx]
		if directive.action != configureFileAction {
			continue
		}
		configured = true
		parsed, err := decodeSettings([]byte(directive.parameter), false)
		if errors.Is(err, ErrNotJSONObject) {
			return nil, fmt.Errorf("markdownlint-configure-file at line %d must contain a JSON object", directive.line)
		}
		if err != nil {
			return nil, fmt.Errorf("markdownlint-configure-file at line %d: %w", directive.line, err)
		}
		var noticed wordSet // The deprecated tags with a notice.
		for _, setting := range parsed.settings {
			if !IsMarkdownlintDefaultKey(setting.Key) {
				rules, meaning := lookupWord(setting.Key, registry)
				if meaning == unknownWord {
					return nil, fmt.Errorf("markdownlint-configure-file at line %d: %s",
						directive.line, registry.configureFileKeyText(setting.Key))
				}
				if meaning == formerTag && noticed.add(setting.Key) {
					directive.notices = append(directive.notices, deprecatedTagNotice(registry, directive, setting.Key, rules))
				}
			}
			settings.Set(setting.Key, setting.Value)
			lines[setting.Key] = directive.line
		}
	}
	if !configured {
		return cfg, nil
	}

	result := cfg.Clone()
	if result == nil {
		result = config.NewConfig()
	}
	if value, ok := settings.Default(); ok {
		if result.RuleDefaults == nil {
			result.RuleDefaults = make(map[string]config.RuleConfig)
		}
		for _, id := range registry.IDs() {
			baseline := value
			if IsGomdlintOnlyRule(id) {
				baseline = false
			}
			result.RuleDefaults[id] = registry.MarkdownlintDefaultRuleConfig(id, baseline)
		}
	}
	if result.Rules == nil {
		result.Rules = make(map[string]config.RuleConfig)
	}
	for _, setting := range settings.settings {
		if IsMarkdownlintDefaultKey(setting.Key) {
			continue
		}
		rules, _ := lookupWord(setting.Key, registry)
		for _, id := range sortedIDs(rules) {
			converted, err := configureFileRule(registry, lines[setting.Key], id, setting.Value)
			if err != nil {
				return nil, err
			}
			converted.AutoFix = result.Rules[id].AutoFix
			result.Rules[id] = converted
		}
	}
	return result, nil
}

// configureFileRule converts value, the setting of the rule with the given
// ID in a markdownlint-configure-file comment on the given line, as a
// markdownlint configuration file's is converted. Options that the rule
// does not read, or whose values have the wrong type, change nothing, as
// in markdownlint, but two spellings of one option are an error, as in a
// config file.
func configureFileRule(registry *Registry, line int, id string, value any) (config.RuleConfig, error) {
	converted, issues := registry.MarkdownlintRuleConfig(id, value)
	for _, issue := range issues {
		if issue.Kind != OptionIssueConflict {
			continue
		}
		name := id
		if rule, ok := registry.GetByID(id); ok {
			name = rule.Name()
		}
		return config.RuleConfig{}, fmt.Errorf("markdownlint-configure-file at line %d: %s", line, issue.Text(name))
	}
	return converted, nil
}

// markdownlintKey returns the key by which markdownlint knows word, a word
// of a markdownlint comment or a key of a markdownlint configuration object,
// in a configuration file or a markdownlint-configure-file comment.
// markdownlint upper-cases each word and key with JavaScript's
// toUpperCase before it compares them, so two words mean the same exactly
// when their upper case is the same. The key is that upper case with its
// ASCII letters written in lower case, as the registry keeps identifiers.
// Beyond ASCII, toUpperCase turns ten characters into ASCII letters (see
// asciiUpperCase) and leaves others, such as the Kelvin sign and the dotted
// capital I, as they are, where strings.ToLower, which the registry's
// foldKey uses, turns them into k and i. The key writes every other
// character in its simple upper case, as toUpperCase does, except for the
// few characters whose full upper case is longer than one character and not
// ASCII, such as U+0149, which toUpperCase expands. Go's Unicode tables can
// also lag the JavaScript engine's, so a character that only a newer
// version of Unicode gives an upper case, such as U+0264, keeps its own. In
// both cases two spellings can have different keys where markdownlint reads
// one word. Such a character is beyond ASCII, so this matters only for a
// custom rule whose name or tag holds one: a word written as the rule
// writes it still names the rule, as does any spelling that differs from it
// only in the case of characters that Go and JavaScript upper-case alike.
//
// Everything that compares the words of comments or the keys of
// markdownlint configuration objects goes through this key: their
// resolution (lookupWord, and Registry.MarkdownlintConfigRuleIDs for the
// keys of configuration files such as .markdownlint.json, against the
// registry's index of every identifier and tag by this key), the default
// key (IsMarkdownlintDefaultKey), the deduplication of notices (wordSet),
// the suggestions for unknown words (Registry.markdownlintSuggestions and
// Registry.UnknownKeyText) and the words of remedies
// (Registry.markdownlintTerm). Registry.Closest, which ranks the
// suggestions, lower-cases the key once more with foldKey. That decides no
// word's meaning; it only lets a word with a Kelvin sign be offered its
// ASCII spelling, and an ASCII word a spelling with a Kelvin sign or a
// dotted capital I.
func markdownlintKey(word string) string {
	if isASCII(word) {
		return strings.ToLower(word)
	}

	var key strings.Builder
	for _, char := range word {
		letters, special := asciiUpperCase(char)
		switch {
		case char < utf8.RuneSelf:
			key.WriteRune(unicode.ToLower(char))
		case special:
			key.WriteString(letters)
		default:
			key.WriteRune(unicode.ToUpper(char))
		}
	}
	return key.String()
}

// wordSet holds words of markdownlint comments, or keys of configure-file
// objects, by their keys (see markdownlintKey), so that it holds a word once
// in every spelling that markdownlint reads as the same word.
type wordSet map[string]bool

// add adds word to the set, making the set when it is nil, and reports
// whether the set lacked word.
func (s *wordSet) add(word string) bool {
	key := markdownlintKey(word)
	if (*s)[key] {
		return false
	}
	if *s == nil {
		*s = make(wordSet)
	}
	(*s)[key] = true
	return true
}

// isASCII reports whether text holds nothing but ASCII.
func isASCII(text string) bool {
	for idx := range len(text) {
		if text[idx] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// lookupWord returns the registered rules that word, one word of a
// markdownlint comment or a key of a markdownlint-configure-file object,
// names, and how it names them. markdownlint looks each up whole, so a word
// names an identifier or tag exactly when the two have the same key (see
// markdownlintKey), beyond ASCII too. An empty word names no rules, and so
// does a configure-file key of several words, unless a rule's name or tag
// holds the same whitespace between its words.
func lookupWord(word string, registry *Registry) ([]Rule, wordMeaning) {
	return directiveRules(markdownlintKey(word), registry)
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

// isFormerTag reports whether key, the key of a word (see
// markdownlintKey), is one of gomdlint's former tags: the tags that its
// built-in rules report themselves that markdownlint does not have.
// Directives resolved tags through Rule.Tags before they used markdownlint's
// tags, so these still work there, with a notice, until a later release.
func isFormerTag(key string) bool {
	switch key {
	case "style", "lists", "layout", "metadata", "gfm", "tables":
		return true
	default:
		return false
	}
}

// directiveRules returns the registered rules that key, the key of one
// word of a markdownlint comment (see markdownlintKey), names, and how it
// names them. The word names the rule it identifies and the rules its tag
// covers. Tags come from the rules' catalog entries, which carry
// markdownlint's tags. A word that is an identifier or a tag in the catalog
// names only those rules, even when none of them is registered, so that a
// registry holding some of the rules resolves it as the full one does. Any
// other word falls back to the tags that rules report themselves through
// Rule.Tags, so that gomdlint's former tags, such as style, keep working for
// now, as do the tags of rules without catalog entries.
func directiveRules(key string, registry *Registry) ([]Rule, wordMeaning) {
	word := registry.lookups().words[key]
	switch {
	case word == nil:
		return nil, unknownWord
	case word.rule != nil || word.catalog:
		rules := slices.Clone(word.tagged)
		if word.rule != nil {
			rules = append(rules, word.rule)
		}
		return rules, knownWord
	case isFormerTag(key):
		return slices.Clone(word.owned), formerTag
	default:
		// Every other entry is a tag that rules report themselves.
		return slices.Clone(word.owned), knownWord
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

// markdownlintTerm returns how a markdownlint comment, or a key of a
// configure-file object, should write word, a word that names a rule or a
// tag, so that markdownlint reads it too: as written, unless it names a
// rule by gomdlint's own name for it where that is not the name
// markdownlint lists first, and then by the name markdownlint lists first
// (RuleMeta.MarkdownlintName). Words compare by their keys (see
// markdownlintKey), and the catalog answers for every rule it describes,
// registered or not. Every message that says what to write in a
// markdownlint comment names rules through it or through
// markdownlintRuleName, its counterpart for rules in hand: the remedies of
// configure-file errors through it, and suggestions and the rules of a
// former tag through markdownlintRuleName.
func (r *Registry) markdownlintTerm(word string) string {
	return cmp.Or(r.lookups().markdownlintNames[markdownlintKey(word)], word)
}

// markdownlintRuleName returns the name that a markdownlint comment should
// use for the registered rule with the given ID and name: the name that
// markdownlint lists first for that ID (RuleMeta.MarkdownlintName), or else
// its name as markdownlintTerm writes it, without surrounding whitespace,
// which no comment word can hold, or its ID when it has no name.
// Both ways give the same name for a built-in rule, whose name is its
// catalog entry's. The ID comes first for a library's rule registered under
// a built-in ID with a name of its own, which markdownlint does not know.
func (r *Registry) markdownlintRuleName(id, name string) string {
	r.mu.RLock()
	term := r.meta[id].MarkdownlintName
	r.mu.RUnlock()
	if term != "" {
		return term
	}
	return cmp.Or(r.markdownlintTerm(strings.TrimSpace(name)), id)
}

// markdownlintName is markdownlintRuleName for rule.
func (r *Registry) markdownlintName(rule Rule) string {
	return r.markdownlintRuleName(rule.ID(), rule.Name()) // Outside the lock: a rule may use the registry.
}

// markdownlintSuggestions returns the rules and tags that word, a word of a
// markdownlint comment or a key of a configure-file object that names
// nothing, most likely means: those that Closest offers for the word as
// markdownlint reads it, its key (see markdownlintKey), with each rule named
// as a markdownlint comment should name it (see markdownlintRuleName) and
// each tag as its catalog entry writes it, which markdownlint reads too. It
// never offers the word itself: a name with the word's key, which the word
// would name if it named anything.
func (r *Registry) markdownlintSuggestions(word string) []Suggestion {
	key := markdownlintKey(word)
	suggestions := r.Closest(key)
	tagSpellings := r.lookups().tagSpellings
	var named []Suggestion
	for _, suggestion := range suggestions {
		if suggestion.IsTag() {
			suggestion.Name = cmp.Or(tagSpellings[suggestion.Name], suggestion.Name)
		} else {
			suggestion.Name = r.markdownlintRuleName(suggestion.RuleID, suggestion.Name)
		}
		if markdownlintKey(suggestion.Name) != key {
			named = append(named, suggestion)
		}
	}
	return named
}

// configureFileKeyText describes key, a key of a markdownlint-configure-file
// object that names no rule or tag, for an error, as UnknownKeyText does
// for configuration files, but in the terms of a markdownlint comment. A
// key of several words that each name a rule or tag gets a note that says
// to write them as separate keys, and any other key the closest rules and
// tags; both name each rule as markdownlint names it (see markdownlintTerm).
func (r *Registry) configureFileKeyText(key string) string {
	words := strings.FieldsFunc(key, isJSWhitespace)
	terms := make([]string, len(words))
	for idx, word := range words {
		if _, meaning := lookupWord(word, r); meaning != knownWord {
			terms = nil
			break
		}
		terms[idx] = r.markdownlintTerm(word)
	}
	if len(terms) >= 2 {
		return severalWordsKeyText(key, terms)
	}
	return UnknownRuleText(key, r.markdownlintSuggestions(key))
}

// wordResolver resolves the words of the markdownlint comments of one file.
// It remembers the suggestions it offers for each unknown word, which a file
// may repeat in many comments.
type wordResolver struct {
	registry *Registry

	// closest maps the key of an unknown word (see markdownlintKey) to the
	// suggestions for it: Registry.Closest's, named as markdownlint names
	// the rules. It is nil until a comment has an unknown word.
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
// spelling that markdownlint reads as it (see lookupWord), and words that
// are not known are ignored.
//
// A comment whose parameter holds the start of the next comment shares that
// comment's "-->", so its parameter is its own segment, up to the end of the
// next comment's kind, followed by the next comment's parameter. Walking
// backwards, each segment is split and resolved once: a comment names the
// rules of its segment and those of the next comment's parameter. A word
// that names no rule or tag, or that is a former tag, gets one notice, on
// the last comment before it that takes rules, once for a word that the
// comment holds more than once (see wordSet). So the work and the notices
// grow with the length of text rather than with the square of the number of
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
// tag, once for a word that they hold more than once in any spelling that
// markdownlint reads as the same word (see wordSet).
func (w *wordResolver) notices(directive *inlineDirective, segment []resolvedWord, pending [][]resolvedWord) []Notice {
	var notices []Notice
	var noticed wordSet // The words with a notice.
	add := func(word resolvedWord) {
		if word.meaning == knownWord || !noticed.add(word.word) {
			return
		}
		if word.meaning == formerTag {
			notices = append(notices, deprecatedTagNotice(w.registry, directive, word.word, word.rules))
			return
		}
		notices = append(notices, unknownRuleNotice(directive, word.word, w.suggestions(word.word)))
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

// suggestions returns the rules and tags that word, an unknown word, most
// likely means (see Registry.markdownlintSuggestions), computing them once
// per key.
func (w *wordResolver) suggestions(word string) []Suggestion {
	key := markdownlintKey(word)
	suggestions, ok := w.closest[key]
	if !ok {
		suggestions = w.registry.markdownlintSuggestions(key)
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
		state[rule.ID()] = resolveRule(rule, cfg, nil).Enabled
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
