package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklabco/gomdlint/pkg/config"
)

// mockRule for testing.
type mockRule struct {
	id   string
	name string
	tags []string
}

func (m *mockRule) ID() string                               { return m.id }
func (m *mockRule) Name() string                             { return m.name }
func (m *mockRule) Description() string                      { return "mock" }
func (m *mockRule) DefaultEnabled() bool                     { return true }
func (m *mockRule) DefaultSeverity() config.Severity         { return config.SeverityWarning }
func (m *mockRule) Tags() []string                           { return m.tags }
func (m *mockRule) CanFix() bool                             { return false }
func (m *mockRule) Apply(*RuleContext) ([]Diagnostic, error) { return nil, nil }

func TestRegistry_GetByName(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	got, ok := reg.GetByName("no-trailing-spaces")
	assert.True(t, ok)
	assert.Equal(t, "MD009", got.ID())
}

func TestRegistry_GetByName_NotFound(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.GetByName("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_Get_ByNameFallback(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	// Get should find by name when ID doesn't match
	got, ok := reg.Get("no-trailing-spaces")
	assert.True(t, ok)
	assert.Equal(t, "MD009", got.ID())
}

func TestRegistry_Resolve(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	tests := []struct {
		key    string
		wantID string
		wantOK bool
	}{
		{"MD009", "MD009", true},
		{"no-trailing-spaces", "MD009", true},
		{"nonexistent", "", false},
	}

	for _, tt := range tests {
		id, _, ok := reg.Resolve(tt.key)
		assert.Equal(t, tt.wantOK, ok, "key: %s", tt.key)
		if tt.wantOK {
			assert.Equal(t, tt.wantID, id, "key: %s", tt.key)
		}
	}
}

func TestRegistry_GetByID(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD009", name: "no-trailing-spaces"}
	reg.Register(rule)

	got, ok := reg.GetByID("MD009")
	assert.True(t, ok)
	assert.Equal(t, "MD009", got.ID())

	_, ok = reg.GetByID("nonexistent")
	assert.False(t, ok)
}

func TestRegistry_Register_And_Get(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD001", name: "heading-increment"}
	reg.Register(rule)

	// Should be retrievable by ID
	got, ok := reg.Get("MD001")
	assert.True(t, ok)
	assert.Equal(t, "MD001", got.ID())
	assert.Equal(t, "heading-increment", got.Name())
}

func TestRegistry_Rules(t *testing.T) {
	reg := NewRegistry()
	rule1 := &mockRule{id: "MD001", name: "heading-increment"}
	rule2 := &mockRule{id: "MD002", name: "first-heading-h1"}
	reg.Register(rule1)
	reg.Register(rule2)

	rules := reg.Rules()
	assert.Len(t, rules, 2)
	// Should be sorted by ID
	assert.Equal(t, "MD001", rules[0].ID())
	assert.Equal(t, "MD002", rules[1].ID())
}

func TestRegistry_IDs(t *testing.T) {
	reg := NewRegistry()
	rule1 := &mockRule{id: "MD002", name: "first-heading-h1"}
	rule2 := &mockRule{id: "MD001", name: "heading-increment"}
	reg.Register(rule1)
	reg.Register(rule2)

	ids := reg.IDs()
	assert.Equal(t, []string{"MD001", "MD002"}, ids)
}

func TestRegistry_DescribeAliases(t *testing.T) {
	reg := NewRegistry()
	rule := &mockRule{id: "MD041", name: "first-line-heading"}
	reg.Register(rule)
	reg.Describe(RuleMeta{ID: "MD041", Name: "first-line-heading", Aliases: []string{"single-h1", "first-line-h1"}})

	// Should resolve alias to rule
	id, r, ok := reg.Resolve("single-h1")
	assert.True(t, ok)
	assert.Equal(t, "MD041", id)
	assert.Equal(t, "first-line-heading", r.Name())

	// Should resolve other alias too
	id2, _, ok2 := reg.Resolve("first-line-h1")
	assert.True(t, ok2)
	assert.Equal(t, "MD041", id2)
}

func TestRegistry_ReRegisterUnderNewNameDropsOldName(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD009", name: "old-name"})
	reg.Register(&mockRule{id: "MD009", name: "new-name"})

	_, ok := reg.GetByName("old-name")
	assert.False(t, ok, "the old name must no longer resolve")
	_, _, ok = reg.Resolve("old-name")
	assert.False(t, ok)

	got, ok := reg.GetByName("new-name")
	assert.True(t, ok)
	assert.Equal(t, "MD009", got.ID())
	assert.Len(t, reg.Rules(), 1)
}

func TestRegistry_ReRegisterKeepsANameNowOwnedByAnotherRule(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD001", name: "shared"})
	reg.Register(&mockRule{id: "MD002", name: "shared"})
	reg.Register(&mockRule{id: "MD001", name: "renamed"})

	got, ok := reg.GetByName("shared")
	assert.True(t, ok, "renaming MD001 must not remove the name MD002 now has")
	assert.Equal(t, "MD002", got.ID())
}

func TestRegistry_DescribeAliases_UnknownRule(t *testing.T) {
	reg := NewRegistry()
	// Describing a rule that is not registered should not panic
	reg.Describe(RuleMeta{ID: "UNKNOWN", Aliases: []string{"some-alias"}})

	_, _, ok := reg.Resolve("some-alias")
	assert.False(t, ok)
}

// overwritten replaces an element of a slice that the registry must not share.
const overwritten = "overwritten"

// describedRegistry returns a registry of mock rules with catalog entries.
// The rules' own tags differ from their catalog tags on purpose.
func describedRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD047", name: "single-trailing-newline"})
	reg.Register(&mockRule{id: "MD004", name: "unordered-list-style", tags: []string{"lists", "style"}})
	reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces", tags: []string{"whitespace"}})
	reg.Register(&mockRule{id: "MD026", name: "no-trailing-punctuation"})
	reg.Register(&mockRule{id: "MD043", name: "required-headings"})
	reg.Register(&mockRule{id: "MD055", name: "table-pipe-style"})
	reg.Describe(RuleMeta{
		ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"},
		Category: CategoryLists, Tags: []string{"bullet", "ul"}, Fixable: true,
	})
	reg.Describe(RuleMeta{ID: "MD009", Name: "no-trailing-spaces", Category: CategoryLayout, Tags: []string{"whitespace"}, Fixable: true})
	reg.Describe(RuleMeta{ID: "MD026", Name: "no-trailing-punctuation", Category: CategoryHeadings, Tags: []string{"headings"}, Fixable: true})
	reg.Describe(RuleMeta{ID: "MD043", Name: "required-headings", Category: CategoryHeadings, Tags: []string{"headings"}})
	reg.Describe(RuleMeta{ID: "MD047", Name: "single-trailing-newline", Category: CategoryLayout, Tags: []string{"blank_lines"}, Fixable: true})
	reg.Describe(RuleMeta{ID: "MD055", Name: "table-pipe-style", Category: CategoryTables, Tags: []string{"table"}})
	// A catalog entry for a rule that is not registered stays invisible.
	reg.Describe(RuleMeta{ID: "MD998", Name: "phantom-rule", Aliases: []string{"phantom-alias"}, Tags: []string{"bullet"}})

	return reg
}

func ruleIDs(rules []Rule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID())
	}
	return ids
}

func TestRegistry_Lookup(t *testing.T) {
	reg := describedRegistry()

	for _, key := range []string{
		"MD004", "md004", "Md004", "unordered-list-style", "Unordered-List-Style",
		"ul-style", "UL-STYLE", " ul-style ", "\tMD004\n",
	} {
		rule, ok := reg.Lookup(key)
		if assert.True(t, ok, "key %q", key) {
			assert.Equal(t, "MD004", rule.ID(), "key %q", key)
		}
	}

	for _, key := range []string{"", "   ", "MD999", "no-such-rule", "ul-styles", "MD00 4", "bullet", "MD998", "phantom-rule", "phantom-alias"} {
		rule, ok := reg.Lookup(key)
		assert.False(t, ok, "key %q", key)
		assert.Nil(t, rule, "key %q", key)
	}
}

func TestRegistry_LookupPrefersIDsThenNamesThenAliases(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD001", name: "first"})
	reg.Register(&mockRule{id: "MD002", name: "md001"}) // a name that spells another rule's ID
	reg.Register(&mockRule{id: "MD003", name: "third"})
	reg.Describe(RuleMeta{ID: "MD003", Aliases: []string{"First", "md002", "third-alias"}})

	for key, want := range map[string]string{
		"MD001": "MD001", "md001": "MD001", "first": "MD001",
		"MD002": "MD002", "third": "MD003", "third-alias": "MD003",
	} {
		rule, ok := reg.Lookup(key)
		if assert.True(t, ok, "key %q", key) {
			assert.Equal(t, want, rule.ID(), "key %q", key)
		}
	}
}

func TestRegistry_LookupFollowsRegistrationChanges(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD009", name: "old-name"})
	_, ok := reg.Lookup("old-name")
	require.True(t, ok)

	reg.Register(&mockRule{id: "MD009", name: "new-name"})
	_, ok = reg.Lookup("old-name")
	assert.False(t, ok, "a replaced rule's old name must no longer resolve")
	rule, ok := reg.Lookup("New-Name")
	require.True(t, ok)
	assert.Equal(t, "new-name", rule.Name())

	reg.Describe(RuleMeta{ID: "MD010", Aliases: []string{"late-alias"}})
	_, ok = reg.Lookup("late-alias")
	assert.False(t, ok, "an alias resolves only once its rule is registered")
	reg.Register(&mockRule{id: "MD010", name: "late-rule"})
	rule, ok = reg.Lookup("late-alias")
	require.True(t, ok)
	assert.Equal(t, "MD010", rule.ID())
}

// TestRegistry_ReplacementUnderAnotherNameDropsTheEntry covers a rule that
// replaces a described rule under the same ID: the entry describes the
// replaced rule, so a replacement under another name has none, and the
// entry's aliases and tags no longer resolve to it. A replacement under
// the same name, such as another implementation of the rule, keeps it.
func TestRegistry_ReplacementUnderAnotherNameDropsTheEntry(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces"})
	reg.Describe(RuleMeta{
		ID: "MD009", Name: "no-trailing-spaces", Aliases: []string{"trailing-spaces"},
		Tags: []string{"whitespace"}, Description: "Lines do not end in spaces.",
	})

	reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces"})
	_, ok := reg.Meta("MD009")
	assert.True(t, ok, "a replacement under the same name keeps the entry")

	reg.Register(&mockRule{id: "MD009", name: "company-policy"})
	_, ok = reg.Meta("MD009")
	assert.False(t, ok, "the entry describes the replaced rule")
	_, ok = reg.Lookup("trailing-spaces")
	assert.False(t, ok, "the replaced rule's alias")
	assert.Empty(t, reg.RulesByTag("whitespace"), "the replaced rule's tag")
	rule, ok := reg.Lookup("company-policy")
	require.True(t, ok)
	assert.Equal(t, "MD009", rule.ID())

	// markdownlint still knows the replaced rule's words, so they are no
	// unknown words in markdownlint comments and configs.
	for _, word := range []string{"no-trailing-spaces", "trailing-spaces", "whitespace"} {
		_, known := reg.MarkdownlintConfigRuleIDs(word)
		assert.True(t, known, "%s stays a word that markdownlint knows", word)
	}
}

// TestRegistry_EntryDescribedBeforeAReplacement covers an entry described
// for a rule before it replaces a described rule under the same ID: the
// entry describes the new rule, which keeps it.
func TestRegistry_EntryDescribedBeforeAReplacement(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces"})
	reg.Describe(RuleMeta{ID: "MD009", Name: "no-trailing-spaces", Aliases: []string{"trailing-spaces"}})

	reg.Describe(RuleMeta{ID: "MD009", Name: "company-policy", Aliases: []string{"policy"}})
	reg.Register(&mockRule{id: "MD009", name: "company-policy"})

	meta, ok := reg.Meta("MD009")
	require.True(t, ok, "the entry describes the new rule")
	assert.Equal(t, "company-policy", meta.Name)
	rule, ok := reg.Lookup("policy")
	require.True(t, ok, "the new entry's alias")
	assert.Equal(t, "company-policy", rule.Name())
}

// otherImplementation is a rule of another type than mockRule, such as a
// custom implementation of a built-in rule.
type otherImplementation struct {
	mockRule
}

// describedMD009 returns a registry in which MD009 is described and then
// registered, as RegisterAll records the built-in rules.
func describedMD009() *Registry {
	reg := NewRegistry()
	reg.Describe(RuleMeta{
		ID: "MD009", Name: "no-trailing-spaces", Aliases: []string{"trailing-spaces"},
		Tags: []string{"whitespace"}, Description: "Lines do not end in spaces.",
		OptionsDeclared: true, Options: []OptionSpec{{Name: "br_spaces", Type: OptionInteger, Default: 2}},
	})
	reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces"})
	return reg
}

// TestRegistry_ReplacementOfAnotherTypeDropsTheEntry covers a rule of
// another type that replaces a described rule under its ID and name: the
// entry describes the replaced implementation, so the new rule has none
// unless an entry is described for it, before or after it registers. A
// replacement of the same type keeps the entry.
func TestRegistry_ReplacementOfAnotherTypeDropsTheEntry(t *testing.T) {
	t.Run("same type", func(t *testing.T) {
		reg := describedMD009()
		reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces"})
		_, ok := reg.Meta("MD009")
		assert.True(t, ok)
	})

	t.Run("another type", func(t *testing.T) {
		reg := describedMD009()
		reg.Register(&otherImplementation{mockRule{id: "MD009", name: "no-trailing-spaces"}})
		_, ok := reg.Meta("MD009")
		assert.False(t, ok, "the entry describes the replaced implementation")
		_, ok = reg.Lookup("trailing-spaces")
		assert.False(t, ok, "the replaced implementation's alias")
		_, issues, declared := reg.NormalizeOptions("MD009", map[string]any{"company_mode": true})
		assert.False(t, declared, "the replaced implementation's options: %v", issues)
		_, known := reg.MarkdownlintConfigRuleIDs("trailing-spaces")
		assert.True(t, known, "markdownlint still knows the word")
	})

	t.Run("described, then registered", func(t *testing.T) {
		reg := describedMD009()
		reg.Describe(RuleMeta{ID: "MD009", Name: "no-trailing-spaces", Description: "Company policy."})
		reg.Register(&otherImplementation{mockRule{id: "MD009", name: "no-trailing-spaces"}})
		meta, ok := reg.Meta("MD009")
		require.True(t, ok, "the entry described for the new rule")
		assert.Equal(t, "Company policy.", meta.Description)

		reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces"})
		_, ok = reg.Meta("MD009")
		assert.False(t, ok, "a later replacement of another type drops it")
	})

	t.Run("registered, then described", func(t *testing.T) {
		reg := describedMD009()
		reg.Register(&otherImplementation{mockRule{id: "MD009", name: "no-trailing-spaces"}})
		reg.Describe(RuleMeta{ID: "MD009", Name: "no-trailing-spaces", Description: "Company policy."})
		meta, ok := reg.Meta("MD009")
		require.True(t, ok)
		assert.Equal(t, "Company policy.", meta.Description)
	})
}

func TestRegistry_DescribeReplacesEntry(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD004", name: "unordered-list-style"})
	reg.Describe(RuleMeta{ID: "MD004", Aliases: []string{"old-alias"}, Tags: []string{"old-tag"}})

	aliases := []string{"ul-style"}
	reg.Describe(RuleMeta{ID: "MD004", Aliases: aliases, Tags: []string{"bullet"}})
	aliases[0] = overwritten // Describe keeps its own copy.

	_, ok := reg.Lookup("old-alias")
	assert.False(t, ok)
	assert.Empty(t, reg.RulesByTag("old-tag"))
	rule, ok := reg.Lookup("ul-style")
	require.True(t, ok)
	assert.Equal(t, "MD004", rule.ID())
	assert.Equal(t, []string{"MD004"}, ruleIDs(reg.RulesByTag("bullet")))
}

func TestRegistry_Meta(t *testing.T) {
	reg := describedRegistry()

	meta, ok := reg.Meta("MD004")
	require.True(t, ok)
	assert.Equal(t, RuleMeta{
		ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"},
		Category: CategoryLists, Tags: []string{"bullet", "ul"}, Fixable: true,
	}, meta)

	// The entry is a copy, so changing it leaves the registry alone.
	meta.Aliases[0] = overwritten
	meta.Tags[0] = overwritten
	again, ok := reg.Meta("MD004")
	require.True(t, ok)
	assert.Equal(t, []string{"ul-style"}, again.Aliases)
	assert.Equal(t, []string{"bullet", "ul"}, again.Tags)

	_, ok = reg.Meta("md004")
	assert.False(t, ok, "Meta takes a rule ID as registered")
	_, ok = reg.Meta("MD998")
	assert.False(t, ok, "an entry for a rule that is not registered stays hidden")
	reg.Register(&mockRule{id: "MD999", name: "undescribed"})
	_, ok = reg.Meta("MD999")
	assert.False(t, ok, "a rule without an entry has no metadata")
}

// TestRegistry_MetaCopiesRuleTags checks that an entry from Meta shares the
// tags that its rule reports itself with neither Describe's caller nor the
// registry.
func TestRegistry_MetaCopiesRuleTags(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD004", name: "unordered-list-style"})
	ruleTags := []string{"lists", "style"}
	reg.Describe(RuleMeta{ID: "MD004", Tags: []string{"bullet", "ul"}, RuleTags: ruleTags})
	ruleTags[0] = overwritten // Describe keeps its own copy.

	meta, ok := reg.Meta("MD004")
	require.True(t, ok)
	assert.Equal(t, []string{"lists", "style"}, meta.RuleTags)

	meta.RuleTags[0] = overwritten
	again, ok := reg.Meta("MD004")
	require.True(t, ok)
	assert.Equal(t, []string{"lists", "style"}, again.RuleTags, "the entry is a copy")
}

// TestRegistry_MetaCopiesOptions checks that the options of an entry from
// Meta share nothing with the registry's: not their aliases, their allowed
// values or a list default.
func TestRegistry_MetaCopiesOptions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD013", name: "line-length"})
	reg.Describe(RuleMeta{ID: "MD013", OptionsDeclared: true, Options: []OptionSpec{
		{
			Name: "style", Aliases: []OptionAlias{{Name: "old_style"}}, Type: OptionString,
			Default: "consistent", Values: []string{"consistent", "dash"},
		},
		{Name: "names", Type: OptionStringList, Default: []string{"Go"}},
		{Name: "lines", Type: OptionIntegerOrList, Default: []int{1, 2}},
	}})

	meta, ok := reg.Meta("MD013")
	require.True(t, ok)
	meta.Options[0].Aliases[0].Name = overwritten
	meta.Options[0].Values[0] = overwritten
	names, ok := meta.Options[1].Default.([]string)
	require.True(t, ok)
	names[0] = overwritten
	lines, ok := meta.Options[2].Default.([]int)
	require.True(t, ok)
	lines[0] = 9

	again, ok := reg.Meta("MD013")
	require.True(t, ok)
	assert.Equal(t, "old_style", again.Options[0].Aliases[0].Name)
	assert.Equal(t, []string{"consistent", "dash"}, again.Options[0].Values)
	assert.Equal(t, []string{"Go"}, again.Options[1].Default)
	assert.Equal(t, []int{1, 2}, again.Options[2].Default)
	assert.True(t, again.OptionsDeclared)
}

func TestRegistry_RulesByTag(t *testing.T) {
	reg := describedRegistry()

	assert.Equal(t, []string{"MD004"}, ruleIDs(reg.RulesByTag("bullet")), "only registered rules")
	assert.Equal(t, []string{"MD004"}, ruleIDs(reg.RulesByTag("UL")), "matching ignores case")
	assert.Equal(t, []string{"MD026", "MD043"}, ruleIDs(reg.RulesByTag("headings")), "rules in ID order")
	assert.Equal(t, []string{"MD009"}, ruleIDs(reg.RulesByTag(" whitespace ")))

	// Tags come from catalog entries only. MD004 tags itself lists and style.
	for _, tag := range []string{"lists", "style", "MD004", "ul-style", "", "no-such-tag"} {
		assert.Empty(t, reg.RulesByTag(tag), "tag %q", tag)
	}

	// The result belongs to the caller.
	tagged := reg.RulesByTag("headings")
	tagged[0] = nil
	assert.Equal(t, []string{"MD026", "MD043"}, ruleIDs(reg.RulesByTag("headings")))
}

// TestRegistry_MarkdownlintConfigKeys covers the keys of a markdownlint
// configuration file that markdownlint knows: every identifier and tag of
// every catalog entry, registered or not, and the IDs and names of the
// registered rules, in any case but whole, as markdownlint looks keys up.
// Only the rules that are registered get configured.
func TestRegistry_MarkdownlintConfigKeys(t *testing.T) {
	reg := describedRegistry()

	known := map[string][]string{
		"MD004": {"MD004"}, "md004": {"MD004"}, "unordered-list-style": {"MD004"}, "UL-STYLE": {"MD004"},
		"bullet": {"MD004"}, "ul": {"MD004"}, "headings": {"MD026", "MD043"},
		"MD998": nil, "phantom-rule": nil, "phantom-alias": nil,
	}
	for word, want := range known {
		ids, ok := reg.MarkdownlintConfigRuleIDs(word)
		assert.True(t, ok, "word %q", word)
		assert.Equal(t, want, ids, "word %q", word)
	}

	// The rules' own tags are not markdownlint's, and a key is read whole.
	for _, word := range []string{"lists", "style", "", " ul ", "no-such-word", "MD999"} {
		ids, ok := reg.MarkdownlintConfigRuleIDs(word)
		assert.False(t, ok, "word %q", word)
		assert.Empty(t, ids, "word %q", word)
	}

	// A registered rule without an entry is known by its ID and name, but
	// not by its own tags.
	reg.Register(&mockRule{id: "MD999", name: "undescribed", tags: []string{"custom"}})
	for _, word := range []string{"MD999", "Undescribed"} {
		ids, ok := reg.MarkdownlintConfigRuleIDs(word)
		assert.True(t, ok, "word %q", word)
		assert.Equal(t, []string{"MD999"}, ids, "word %q", word)
	}
	_, ok := reg.MarkdownlintConfigRuleIDs("custom")
	assert.False(t, ok)
}

func TestRegistry_SuggestTypos(t *testing.T) {
	reg := describedRegistry()

	for key, want := range map[string]Suggestion{
		"no-trailling-spaces": {Name: "no-trailing-spaces", RuleID: "MD009"},
		"NO-TRAILLING-SPACES": {Name: "no-trailing-spaces", RuleID: "MD009"},
		"ul-styel":            {Name: "unordered-list-style", RuleID: "MD004"}, // an alias's typo names the rule
		"unordered-list-styl": {Name: "unordered-list-style", RuleID: "MD004"},
		"MD0004":              {Name: "unordered-list-style", RuleID: "MD004"},
		"tables":              {Name: "table"},
		"bulet":               {Name: "bullet"},
	} {
		got := reg.Suggest(key)
		if assert.NotEmpty(t, got, "key %q", key) {
			assert.Equal(t, want, got[0], "key %q", key)
		}
	}

	tag := reg.Suggest("tables")[0]
	assert.True(t, tag.IsTag())
	assert.False(t, reg.Suggest("no-trailling-spaces")[0].IsTag())
}

func TestRegistry_SuggestSubstrings(t *testing.T) {
	reg := describedRegistry()

	assert.Equal(t, []Suggestion{
		{Name: "no-trailing-punctuation", RuleID: "MD026"},
		{Name: "no-trailing-spaces", RuleID: "MD009"},
		{Name: "single-trailing-newline", RuleID: "MD047"},
	}, reg.Suggest("trailing"))
	assert.Equal(t, []Suggestion{{Name: "table-pipe-style", RuleID: "MD055"}}, reg.Suggest("PIPE"))

	// Typos come before substrings: heading is one letter short of the tag
	// headings and part of the name required-headings.
	assert.Equal(t, []Suggestion{
		{Name: "headings"},
		{Name: "required-headings", RuleID: "MD043"},
	}, reg.Suggest("heading"))

	// A rule that matches through several identifiers appears once.
	assert.Equal(t, []Suggestion{{Name: "unordered-list-style", RuleID: "MD004"}}, reg.Suggest("ul-styl"))
}

func TestRegistry_SuggestNothing(t *testing.T) {
	reg := describedRegistry()

	for _, key := range []string{"", "   ", "zzzzzz", "xy", "phantom-alia", "phantom"} {
		assert.Empty(t, reg.Suggest(key), "key %q", key)
	}
}

func TestRegistry_ClosestOffersTheBestSuggestions(t *testing.T) {
	reg := describedRegistry()

	var (
		spaces      = Suggestion{Name: "no-trailing-spaces", RuleID: "MD009"}
		punctuation = Suggestion{Name: "no-trailing-punctuation", RuleID: "MD026"}
		newline     = Suggestion{Name: "single-trailing-newline", RuleID: "MD047"}
	)

	assert.Equal(t, []Suggestion{spaces}, reg.Closest("no-trailling-spaces"))
	assert.Equal(t, []Suggestion{{Name: "table"}}, reg.Closest("tables"), "a typo of a tag")
	assert.Equal(t, []Suggestion{punctuation, spaces, newline}, reg.Closest("trailing"), "substrings of equal rank")

	// Suggest also offers required-headings, which contains the key, but
	// the tag is one edit away and so closer.
	require.Len(t, reg.Suggest("heading"), 2)
	assert.Equal(t, []Suggestion{{Name: "headings"}}, reg.Closest("heading"))

	assert.Empty(t, reg.Closest("zzzzzz"))
	assert.Empty(t, reg.Closest(""))
}

// TestRegistry_ClosestSkipsAmbiguousKeys covers a key as close to many
// rules as to any. Rule IDs are dense, so a mistyped ID is often as close
// to several of them, and naming some would mislead.
func TestRegistry_ClosestSkipsAmbiguousKeys(t *testing.T) {
	reg := describedRegistry()

	require.Len(t, reg.Suggest("MD0"), 6, "two edits from each registered ID")
	assert.Empty(t, reg.Closest("MD0"))
	assert.Empty(t, reg.ClosestRules("MD0"))
}

func TestRegistry_ClosestTagsLeavesOutRules(t *testing.T) {
	reg := describedRegistry()

	assert.Equal(t, []Suggestion{{Name: "table"}}, reg.ClosestTags("tables"))
	assert.Equal(t, []Suggestion{{Name: "bullet"}}, reg.ClosestTags("BULET"))
	// Suggest also offers required-headings, which contains the key.
	assert.Equal(t, []Suggestion{{Name: "headings"}}, reg.ClosestTags("heading"))
	assert.Empty(t, reg.ClosestTags("no-trailling-spaces"), "only a rule is close")
	assert.Empty(t, reg.ClosestTags("ul-styel"), "only an alias is close")
	assert.Empty(t, reg.ClosestTags("zzzzzz"))
}

func TestRegistry_ClosestRulesLeavesOutTags(t *testing.T) {
	reg := describedRegistry()

	assert.Equal(t, []Suggestion{{Name: "no-trailing-spaces", RuleID: "MD009"}}, reg.ClosestRules("no-trailling-spaces"))
	// Without the tag headings, the rule that contains the key is closest.
	assert.Equal(t, []Suggestion{{Name: "required-headings", RuleID: "MD043"}}, reg.ClosestRules("heading"))
	assert.Empty(t, reg.ClosestRules("tables"), "only a tag is close")
	assert.Empty(t, reg.ClosestRules("bulet"))
}

// TestRegistry_ConfigKey covers the key under which a generated
// configuration names a rule: its name while the name identifies the rule
// alone, and its ID once another registered rule has the same name, uses
// it as an alias or has it as its ID, so that the key never configures
// another rule.
func TestRegistry_ConfigKey(t *testing.T) {
	reg := NewRegistry()
	for _, rule := range []*mockRule{
		{id: "MD009", name: "no-trailing-spaces"},
		{id: "MD010", name: "no-hard-tabs"},
		{id: "MD013", name: "line-length"},
		{id: "MD033", name: "no-inline-html"},
		{id: "MD900", name: "Line-Length"},
		{id: "MD901", name: "tabs"},
		{id: "MD902", name: "md033"},
		{id: "MD903", name: " spaced "},
		{id: "MD904", name: "old-name"},
		{id: "MD905", name: "own-alias"},
		{id: "MD906", name: "kept"},
	} {
		reg.Register(rule)
	}
	reg.Describe(RuleMeta{ID: "MD901", Name: "tabs", Aliases: []string{"no-hard-tabs"}})
	reg.Describe(RuleMeta{ID: "MD905", Name: "own-alias", Aliases: []string{"Own-Alias"}})
	reg.Describe(RuleMeta{ID: "MD999", Name: "phantom", Aliases: []string{"kept"}})
	_ = reg.ConfigKey("MD904") // Builds the index, which the next Register must rebuild.
	reg.Register(&mockRule{id: "MD904", name: "new-name"})

	tests := []struct {
		name, id, want string
	}{
		{name: "a name that one rule has", id: "MD009", want: "no-trailing-spaces"},
		{name: "a name that another rule has, in another case", id: "MD013", want: "MD013"},
		{name: "the other rule of that name", id: "MD900", want: "MD900"},
		{name: "a name that another rule has as an alias", id: "MD010", want: "MD010"},
		{name: "the rule with that alias", id: "MD901", want: "tabs"},
		{name: "a name that another rule has as its ID", id: "MD902", want: "MD902"},
		{name: "the rule with that ID keeps its name", id: "MD033", want: "no-inline-html"},
		{name: "a name with spaces around it", id: "MD903", want: " spaced "},
		{name: "a rule registered again under a new name", id: "MD904", want: "new-name"},
		{name: "a name that is also the rule's own alias", id: "MD905", want: "own-alias"},
		{name: "an alias of a catalog entry that is not registered", id: "MD906", want: "kept"},
		{name: "a rule that is not registered", id: "MD999", want: "MD999"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, reg.ConfigKey(tt.id), tt.name)
	}
	for _, id := range reg.IDs() {
		rule, found := reg.Lookup(reg.ConfigKey(id))
		require.True(t, found, id)
		assert.Equal(t, id, rule.ID(), "the key of %s names it", id)
	}
}

// TestRegistry_ConfigKeyIDsThatDifferInCase pins the limit of ConfigKey:
// an ID that another registered ID differs from only in case cannot name
// its rule either, since Lookup ignores case.
func TestRegistry_ConfigKeyIDsThatDifferInCase(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "X1", name: "b"})
	reg.Register(&mockRule{id: "x1", name: "b"})

	assert.Equal(t, "x1", reg.ConfigKey("x1"))
	rule, found := reg.Lookup("x1")
	require.True(t, found)
	assert.Equal(t, "X1", rule.ID(), "Lookup finds the first ID")
}
