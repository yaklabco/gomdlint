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
