package lint

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/yaklabco/gomdlint/internal/editdistance"
)

const (
	// maxSuggestionDistance is the largest edit distance at which Suggest
	// offers a rule or tag as a likely typo of a key.
	maxSuggestionDistance = 2

	// substringRank ranks a substring match after every typo.
	substringRank = maxSuggestionDistance + 1

	// noMatch is the rank of a word that a key does not match.
	noMatch = -1

	// minSubstringLength is the shortest key that Suggest looks for within
	// names, aliases and tags.
	minSubstringLength = 3

	// maxClosest is the most suggestions that Closest offers. A key that is
	// as close to more rules and tags is ambiguous.
	maxClosest = 3
)

// Registry holds the registered lint rules and their catalog entries, and
// resolves the identifiers and tags that name them.
type Registry struct {
	mu     sync.RWMutex
	byID   map[string]Rule
	byName map[string]Rule
	meta   map[string]RuleMeta // rule ID -> catalog entry

	// index serves every lookup of identifiers, tags and names. Every
	// change sets it to nil, and the next lookup builds a new one. A built
	// index never changes, so a reader may use it after releasing the lock.
	index *ruleIndex
}

// ruleIndex maps the identifiers and tags of the registered rules, and the
// words of the catalog, to the rules: lower-cased for Lookup, RulesByTag
// and Suggest, and by markdownlint's key for the words of markdownlint
// comments and the keys of markdownlint configuration objects, in
// configure-file comments and in configuration files.
type ruleIndex struct {
	// keys maps the IDs, names and aliases of the rules to the rules. An ID
	// takes precedence over a name, and a name over an alias.
	keys map[string]Rule

	// tags maps the tags of the rules' catalog entries to the rules.
	tags map[string][]Rule

	// tagSpellings maps each key of tags to the tag as a catalog entry
	// writes it: in lower case when an entry writes it so, else as the
	// first entry that lists it, in rule ID order, does.
	tagSpellings map[string]string

	// words maps the key (see markdownlintKey) of every ID, name, alias and
	// tag of the registered rules and of the catalog to what a word with
	// that key names in a markdownlint comment, or as a key of a
	// markdownlint configuration object.
	// Unlike the lower-cased maps, it compares identifiers beyond ASCII as
	// markdownlint does, by their upper case. Like them, it ignores
	// whitespace around an identifier, which no built-in identifier has and
	// no comment word can hold.
	words map[string]*directiveWord

	// markdownlintNames maps the key (see markdownlintKey) of a catalog
	// entry's own name to markdownlint's name for its rule
	// (RuleMeta.MarkdownlintName), for every entry that has one, whether or
	// not its rule is registered.
	markdownlintNames map[string]string

	// tagNames lists the keys of tags in order.
	tagNames []string

	// rules lists the rules with their identifiers, for Suggest.
	rules []indexedRule

	// options maps the ID of each registered rule whose catalog entry
	// declares its options (RuleMeta.OptionsDeclared) to those options.
	options map[string]*optionTable
}

// directiveWord says what a word of a markdownlint comment, or a key of a
// configure-file object, names: an entry of ruleIndex.words.
type directiveWord struct {
	// rule is the registered rule whose ID, name or catalog alias the word
	// is. An ID takes precedence over a name, and a name over an alias.
	rule Rule

	// tagged lists the registered rules whose catalog entries list the word
	// as a tag, in rule ID order.
	tagged []Rule

	// owned lists the registered rules that report the word as a tag
	// themselves, through Rule.Tags, in rule ID order.
	owned []Rule

	// catalog reports whether the word is the ID, name, an alias or a tag
	// of any catalog entry, whether or not its rule is registered.
	catalog bool
}

// indexedRule is a registered rule with its lower-cased identifiers.
type indexedRule struct {
	rule Rule
	id   string
	// names holds the rule's name and aliases.
	names []string
}

// NewRegistry creates an empty rule registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:   make(map[string]Rule),
		byName: make(map[string]Rule),
		meta:   make(map[string]RuleMeta),
	}
}

// Register adds a rule to the registry.
// If a rule with the same ID already exists, it is replaced, and the name
// of the replaced rule no longer resolves unless the new rule uses it too.
func (r *Registry) Register(rule Rule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if old, ok := r.byID[rule.ID()]; ok {
		// Only drop the old name if it still points at the replaced rule;
		// another rule may have registered the same name since.
		if named, ok := r.byName[old.Name()]; ok && named.ID() == old.ID() {
			delete(r.byName, old.Name())
		}
	}

	r.byID[rule.ID()] = rule
	r.byName[rule.Name()] = rule
	r.index = nil
}

// Describe records meta as the catalog entry of the rule with ID meta.ID,
// replacing any earlier entry for that rule. Lookup resolves the entry's
// aliases, and RulesByTag its tags, while a rule with that ID is
// registered. The registry keeps its own copy of meta.
func (r *Registry) Describe(meta RuleMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.meta[meta.ID] = meta.clone()
	r.index = nil
}

// Meta returns a copy of the catalog entry of the registered rule with the
// given ID. It reports false for an ID that no registered rule has, given
// exactly as registered, and for a rule without an entry.
func (r *Registry) Meta(id string) (RuleMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, registered := r.byID[id]; !registered {
		return RuleMeta{}, false
	}
	meta, ok := r.meta[id]
	if !ok {
		return RuleMeta{}, false
	}
	return meta.clone(), true
}

// Get retrieves a rule by ID or name.
// It tries ID first, then falls back to name lookup.
func (r *Registry) Get(key string) (Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try ID first
	if rule, ok := r.byID[key]; ok {
		return rule, true
	}
	// Fall back to name
	if rule, ok := r.byName[key]; ok {
		return rule, true
	}
	return nil, false
}

// GetByID retrieves a rule by its ID only.
func (r *Registry) GetByID(id string) (Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.byID[id]
	return rule, ok
}

// GetByName retrieves a rule by its name only.
func (r *Registry) GetByName(name string) (Rule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rule, ok := r.byName[name]
	return rule, ok
}

// Lookup returns the registered rule that key identifies: its ID, its name
// or one of the aliases in its catalog entry. Matching ignores case, as
// markdownlint's does, and surrounding whitespace, so "md009" and
// " No-Trailing-Spaces " both find MD009. An ID takes precedence over a
// name, and a name over an alias. Tags do not resolve; see RulesByTag.
func (r *Registry) Lookup(key string) (Rule, bool) {
	rule, ok := r.lookups().keys[foldKey(key)]
	return rule, ok
}

// Resolve returns the ID of the registered rule that key identifies,
// together with the rule, as Lookup finds it.
func (r *Registry) Resolve(key string) (string, Rule, bool) {
	rule, ok := r.Lookup(key)
	if !ok {
		return "", nil, false
	}
	return rule.ID(), rule, true
}

// RulesByTag returns the registered rules whose catalog entries list tag,
// in rule ID order. Matching ignores case and surrounding whitespace. The
// tags that rules report themselves through Rule.Tags do not count.
func (r *Registry) RulesByTag(tag string) []Rule {
	return slices.Clone(r.lookups().tags[foldKey(tag)])
}

// NormalizeOptions rewrites options, the options of the registered rule
// with the given ID, to the canonical names of the options that its
// catalog entry declares, and reports the problems it finds, as the
// function NormalizeOptions does. It reports false, and returns options as
// they are without problems, when no registered rule has the ID, given
// exactly as registered, or when the rule's entry does not declare its
// options (RuleMeta.OptionsDeclared).
func (r *Registry) NormalizeOptions(id string, options map[string]any) (map[string]any, []OptionIssue, bool) {
	table := r.lookups().options[id]
	if table == nil {
		return options, nil, false
	}
	normalized, issues := table.normalize(options)
	return normalized, issues, true
}

// Suggestion is a rule or a tag that Suggest offers in place of a key.
type Suggestion struct {
	// Name is the name to show for the rule, or the tag. Suggest and
	// Closest give a rule's canonical name; suggestions for markdownlint
	// comments give markdownlint's name for it instead.
	Name string

	// RuleID is the ID of the suggested rule. It is empty for a tag.
	RuleID string
}

// IsTag reports whether the suggestion is a tag rather than a rule.
func (s Suggestion) IsTag() bool {
	return s.RuleID == ""
}

// Suggest returns the registered rules and the catalog tags that key most
// likely means, best first, for a key that names neither. Matching ignores
// case and surrounding whitespace. A rule or tag qualifies when key is at
// most two edits from its ID, name, an alias or the tag, and needs fewer
// edits than that word has characters, or when key, at least three
// characters long, appears within its name, an alias or the tag. Typos come
// first, closest first, then substring matches; matches of equal rank are
// in order of name. A rule is offered by its canonical name, whichever
// identifier matched.
func (r *Registry) Suggest(key string) []Suggestion {
	ranked := r.rankSuggestions(key)
	if len(ranked) == 0 {
		return nil
	}
	suggestions := make([]Suggestion, len(ranked))
	for idx, entry := range ranked {
		suggestions[idx] = entry.Suggestion
	}
	return suggestions
}

// Closest returns the rules and tags that key most likely means, for a
// message about a key that names neither: those that Suggest ranks best, in
// its order. A key that is as close to more than three rules and tags gets
// none, because naming some of them would mislead. Rule IDs are dense, so a
// mistyped ID such as MD002 is one edit away from eleven rules.
func (r *Registry) Closest(key string) []Suggestion {
	return closest(r.rankSuggestions(key), true)
}

// ClosestRules is Closest for a key in a place that takes rules but not
// tags: it leaves tags out before it picks the best suggestions.
func (r *Registry) ClosestRules(key string) []Suggestion {
	return closest(r.rankSuggestions(key), false)
}

// rankedSuggestion is a suggestion with its rank: the edit distance of a
// typo, or substringRank.
type rankedSuggestion struct {
	Suggestion

	rank int
}

// rankSuggestions returns the suggestions for key, in Suggest's order, with
// their ranks.
func (r *Registry) rankSuggestions(key string) []rankedSuggestion {
	key = foldKey(key)
	if key == "" {
		return nil
	}
	index := r.lookups()

	ranks := make(map[Suggestion]int)
	offer := func(suggestion Suggestion, rank int) {
		if previous, seen := ranks[suggestion]; rank != noMatch && (!seen || rank < previous) {
			ranks[suggestion] = rank
		}
	}
	for _, entry := range index.rules {
		suggestion := Suggestion{Name: entry.rule.Name(), RuleID: entry.rule.ID()}
		offer(suggestion, matchRank(key, entry.id, false))
		for _, name := range entry.names {
			offer(suggestion, matchRank(key, name, true))
		}
	}
	for _, tag := range index.tagNames {
		offer(Suggestion{Name: tag}, matchRank(key, tag, true))
	}

	ranked := make([]rankedSuggestion, 0, len(ranks))
	for suggestion, rank := range ranks {
		ranked = append(ranked, rankedSuggestion{Suggestion: suggestion, rank: rank})
	}
	slices.SortFunc(ranked, func(left, right rankedSuggestion) int {
		return cmp.Or(
			cmp.Compare(left.rank, right.rank),
			cmp.Compare(left.Name, right.Name),
			cmp.Compare(left.RuleID, right.RuleID),
		)
	})
	return ranked
}

// closest returns the suggestions of the best rank in ranked, which is in
// Suggest's order, leaving tags out unless withTags is set. It returns none
// when more than maxClosest share that rank.
func closest(ranked []rankedSuggestion, withTags bool) []Suggestion {
	var best []Suggestion
	bestRank := noMatch
	for _, entry := range ranked {
		if entry.IsTag() && !withTags {
			continue
		}
		if bestRank == noMatch {
			bestRank = entry.rank
		} else if entry.rank != bestRank {
			break // The rest rank worse.
		}
		best = append(best, entry.Suggestion)
	}
	if len(best) > maxClosest {
		return nil
	}
	return best
}

// matchRank ranks how well key matches word, both lower-cased: by edit
// distance when key is close enough to be a typo of word, which must be
// longer than the distance, or with substringRank when substrings is set
// and key appears within word. It returns noMatch otherwise.
func matchRank(key, word string, substrings bool) int {
	if lengthGap := len(key) - len(word); lengthGap <= maxSuggestionDistance && -lengthGap <= maxSuggestionDistance {
		if distance := editdistance.Levenshtein(key, word); distance <= maxSuggestionDistance && distance < len(word) {
			return distance
		}
	}
	if substrings && len(key) >= minSubstringLength && strings.Contains(word, key) {
		return substringRank
	}
	return noMatch
}

// Rules returns all registered rules.
func (r *Registry) Rules() []Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Rule, 0, len(r.byID))
	for _, rule := range r.byID {
		result = append(result, rule)
	}

	// Sort by rule ID for consistent, deterministic output.
	slices.SortFunc(result, func(a, b Rule) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	return result
}

// IDs returns all registered rule IDs in sorted order.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.byID))
	for id := range r.byID {
		result = append(result, id)
	}

	slices.Sort(result)
	return result
}

// lookups returns the index of the registered rules, building it first if
// a change has made it stale.
func (r *Registry) lookups() *ruleIndex {
	r.mu.RLock()
	index := r.index
	r.mu.RUnlock()
	if index != nil {
		return index
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.index == nil {
		r.index = r.buildIndex()
	}
	return r.index
}

// buildIndex indexes the registered rules. The caller holds the write lock.
func (r *Registry) buildIndex() *ruleIndex {
	ids := slices.Sorted(maps.Keys(r.byID))
	build := indexBuilder{&ruleIndex{
		keys:  make(map[string]Rule, 2*len(ids)), // An ID and a name per rule, before aliases.
		tags:  make(map[string][]Rule),
		rules: make([]indexedRule, 0, len(ids)),

		tagSpellings:      make(map[string]string),
		words:             make(map[string]*directiveWord),
		markdownlintNames: make(map[string]string),
		options:           make(map[string]*optionTable),
	}}

	// The catalog's words come from every entry, registered or not, so that
	// a word means the same in a registry that holds only some of the rules.
	for _, id := range slices.Sorted(maps.Keys(r.meta)) {
		build.addCatalogEntry(r.meta[id])
	}

	// The first claim on a key wins, so claim IDs, then names, then aliases.
	for _, id := range ids {
		build.claim(id, r.byID[id])
	}
	for _, name := range slices.Sorted(maps.Keys(r.byName)) {
		build.claim(name, r.byName[name])
	}
	for _, id := range ids {
		rule := r.byID[id]
		meta := r.meta[id]
		indexed := indexedRule{rule: rule, id: foldKey(id), names: []string{foldKey(rule.Name())}}
		for _, alias := range meta.Aliases {
			build.claim(alias, rule)
			indexed.names = append(indexed.names, foldKey(alias))
		}
		build.rules = append(build.rules, indexed)
		build.addTags(rule, meta)
		if meta.OptionsDeclared {
			build.options[id] = newOptionTable(meta.Options)
		}
	}

	build.tagNames = slices.Sorted(maps.Keys(build.tags))
	return build.ruleIndex
}

// indexBuilder fills a ruleIndex, which never changes once buildIndex
// returns it.
type indexBuilder struct {
	*ruleIndex
}

// entry returns the entry of words for word, adding an empty one when there
// is none, or nil for a word whose key is empty.
func (b indexBuilder) entry(word string) *directiveWord {
	key := markdownlintKey(strings.TrimSpace(word))
	if key == "" {
		return nil
	}
	found := b.words[key]
	if found == nil {
		found = &directiveWord{}
		b.words[key] = found
	}
	return found
}

// addCatalogEntry indexes the words of meta, a catalog entry, whose rule may
// not be registered. Entries arrive in ID order, and the first claim on a
// name wins.
func (b indexBuilder) addCatalogEntry(meta RuleMeta) {
	for _, group := range [][]string{{meta.ID, meta.Name}, meta.Aliases, meta.Tags} {
		for _, word := range group {
			if found := b.entry(word); found != nil {
				found.catalog = true
			}
		}
	}
	key := markdownlintKey(strings.TrimSpace(meta.Name))
	if _, taken := b.markdownlintNames[key]; key != "" && !taken && meta.MarkdownlintName != "" {
		b.markdownlintNames[key] = meta.MarkdownlintName
	}
}

// claim makes identifier, an ID, name or alias of rule, name rule, unless
// another rule has claimed it already.
func (b indexBuilder) claim(identifier string, rule Rule) {
	if key := foldKey(identifier); key != "" {
		if _, taken := b.keys[key]; !taken {
			b.keys[key] = rule
		}
	}
	if found := b.entry(identifier); found != nil && found.rule == nil {
		found.rule = rule
	}
}

// addTags indexes the tags of rule, a registered rule: those of its catalog
// entry meta and those it reports itself. Rules arrive in ID order.
func (b indexBuilder) addTags(rule Rule, meta RuleMeta) {
	addTagged(b.tags, meta.Tags, rule)
	for _, tag := range meta.Tags {
		if key := foldKey(tag); key != "" {
			if spelling, ok := b.tagSpellings[key]; !ok || (spelling != key && tag == key) {
				b.tagSpellings[key] = tag
			}
		}
		if found := b.entry(tag); found != nil {
			found.tagged = appendOnce(found.tagged, rule)
		}
	}
	for _, tag := range rule.Tags() {
		if found := b.entry(tag); found != nil {
			found.owned = appendOnce(found.owned, rule)
		}
	}
}

// addTagged adds rule to the list of each of its tags in byTag, once.
func addTagged(byTag map[string][]Rule, tags []string, rule Rule) {
	for _, tag := range tags {
		if key := foldKey(tag); key != "" {
			byTag[key] = appendOnce(byTag[key], rule)
		}
	}
}

// appendOnce appends rule to rules unless it is the last of them already,
// as when a rule lists a tag twice. Rules arrive in ID order, so each list
// stays in ID order.
func appendOnce(rules []Rule, rule Rule) []Rule {
	if last := len(rules) - 1; last >= 0 && rules[last].ID() == rule.ID() {
		return rules
	}
	return append(rules, rule)
}

// foldKey normalizes a rule identifier or tag for matching: lower case,
// without surrounding whitespace.
func foldKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// DefaultRegistry is the global registry for built-in rules.
// Rules register themselves during init().
//
//nolint:gochecknoglobals // Global registry is intentional for rule registration
var DefaultRegistry = NewRegistry()
