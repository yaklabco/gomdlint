package lint_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// lengthSpecs declares options in the shape of line-length's: a renamed
// integer, an inverted boolean, a string with allowed values, lists, an
// integer or list, and a pattern that false turns off.
func lengthSpecs() []lint.OptionSpec {
	return []lint.OptionSpec{
		{Name: "max", Aliases: []lint.OptionAlias{{Name: "line_length"}}, Type: lint.OptionInteger, Default: 80},
		{
			Name: "ignore_tables", Aliases: []lint.OptionAlias{{Name: "tables", Inverted: true}},
			Type: lint.OptionBoolean, Default: false,
		},
		{Name: "style", Type: lint.OptionString, Default: "consistent", Values: []string{"consistent", "dash", "plus"}},
		{Name: "names", Type: lint.OptionStringList, Default: []string{}},
		{Name: "lines_above", Type: lint.OptionIntegerOrList, Default: 1},
		{Name: "front_matter_title", Type: lint.OptionStringOrFalse, Default: "^title"},
	}
}

func TestOptionTypeAccepts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		typ      lint.OptionType
		accepted []any
		rejected []any
	}{
		{
			typ:      lint.OptionBoolean,
			accepted: []any{true, false},
			rejected: []any{"true", 1, []any{true}},
		},
		{
			typ:      lint.OptionInteger,
			accepted: []any{0, 80, -1, float64(100)},
			rejected: []any{2.5, "80", true, []any{80}, math.Inf(1), math.NaN(), int64(80), 1e300},
		},
		{
			typ:      lint.OptionString,
			accepted: []any{"", "consistent"},
			rejected: []any{false, 1, []any{"a"}},
		},
		{
			typ:      lint.OptionStringList,
			accepted: []any{[]any{}, []any{"a", "b"}, []string{"a"}},
			rejected: []any{"a", 1, true, map[string]any{"a": 1}, []any{1}, []any{"a", 2}, []any{nil}, []int{1}},
		},
		{
			typ:      lint.OptionIntegerOrList,
			accepted: []any{1, -1, float64(2), []any{}, []any{1, float64(2)}, []int{1}},
			rejected: []any{"1", 1.5, true, []any{1, 2.5}, []any{"1"}, []any{nil}, []string{"1"}},
		},
		{
			typ:      lint.OptionStringOrFalse,
			accepted: []any{"", "^title", false},
			rejected: []any{true, 1, []any{"a"}},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			t.Parallel()
			for _, value := range tt.accepted {
				assert.True(t, tt.typ.Accepts(value), "%s accepts %#v", tt.typ, value)
			}
			for _, value := range tt.rejected {
				assert.False(t, tt.typ.Accepts(value), "%s rejects %#v", tt.typ, value)
			}
			assert.True(t, tt.typ.Accepts(nil), "null leaves an option unset, whatever its type")
		})
	}
}

func TestNormalizeOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options map[string]any
		want    map[string]any
		issues  []lint.OptionIssue
	}{
		{
			name:    "canonical spellings stay",
			options: map[string]any{"max": 100, "ignore_tables": true, "style": "dash"},
			want:    map[string]any{"max": 100, "ignore_tables": true, "style": "dash"},
		},
		{
			name:    "markdownlint's spelling becomes the canonical one",
			options: map[string]any{"line_length": 100},
			want:    map[string]any{"max": 100},
		},
		{
			name:    "an inverted spelling holds the opposite value",
			options: map[string]any{"tables": false},
			want:    map[string]any{"ignore_tables": true},
		},
		{
			name:    "an inverted spelling set to true",
			options: map[string]any{"tables": true},
			want:    map[string]any{"ignore_tables": false},
		},
		{
			name:    "a whole number written as a float is an integer",
			options: map[string]any{"max": float64(100)},
			want:    map[string]any{"max": float64(100)},
		},
		{
			name:    "a typed list becomes a list of values, as a config file holds",
			options: map[string]any{"names": []string{"JavaScript"}, "lines_above": []int{1, 2}},
			want:    map[string]any{"names": []any{"JavaScript"}, "lines_above": []any{1, 2}},
		},
		{
			name:    "false turns a pattern off",
			options: map[string]any{"front_matter_title": false},
			want:    map[string]any{"front_matter_title": false},
		},
		{
			name:    "null leaves an option unset",
			options: map[string]any{"max": nil},
			want:    map[string]any{"max": nil},
		},
		{
			name:    "a value of the wrong type is dropped",
			options: map[string]any{"max": "100px", "style": "dash"},
			want:    map[string]any{"style": "dash"},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueWrongType, Key: "max", Value: "100px", Option: lengthSpecs()[0],
			}},
		},
		{
			name:    "a fraction is not an integer",
			options: map[string]any{"line_length": 2.5},
			want:    map[string]any{},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueWrongType, Key: "line_length", Value: 2.5, Option: lengthSpecs()[0],
			}},
		},
		{
			name:    "an inverted spelling takes only true or false",
			options: map[string]any{"tables": "no"},
			want:    map[string]any{},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueWrongType, Key: "tables", Value: "no", Option: lengthSpecs()[1],
			}},
		},
		{
			name:    "a value outside the allowed ones is kept for the rule",
			options: map[string]any{"style": "dashes"},
			want:    map[string]any{"style": "dashes"},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueNotAllowed, Key: "style", Value: "dashes", Option: lengthSpecs()[2],
				Suggestions: []string{"dash"},
			}},
		},
		{
			name:    "an unknown option is kept and gets the closest spelling",
			options: map[string]any{"lin_length": 100},
			want:    map[string]any{"lin_length": 100},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueUnknown, Key: "lin_length", Value: 100, Suggestions: []string{"line_length"},
			}},
		},
		{
			name:    "spellings are matched exactly",
			options: map[string]any{"Max": 100},
			want:    map[string]any{"Max": 100},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueUnknown, Key: "Max", Value: 100, Suggestions: []string{"max"},
			}},
		},
		{
			name:    "two spellings of one option conflict, and the canonical one wins",
			options: map[string]any{"max": 90, "line_length": 100},
			want:    map[string]any{"max": 90},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueConflict, Key: "line_length", Other: "max", Value: 100, Option: lengthSpecs()[0],
			}},
		},
		{
			name:    "an inverted spelling conflicts with the canonical one",
			options: map[string]any{"tables": false, "ignore_tables": true},
			want:    map[string]any{"ignore_tables": true},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueConflict, Key: "ignore_tables", Other: "tables", Value: true, Option: lengthSpecs()[1],
			}},
		},
		{
			name:    "an item of the wrong type drops the whole list, as a scalar of the wrong type is dropped",
			options: map[string]any{"names": []any{"Go", 42}, "style": "dash"},
			want:    map[string]any{"style": "dash"},
			issues: []lint.OptionIssue{{
				Kind: lint.OptionIssueWrongType, Key: "names", Value: 42, Item: 2, Option: lengthSpecs()[3],
			}},
		},
		{
			name:    "each item of the wrong type is reported",
			options: map[string]any{"lines_above": []any{1, 2.5, "3"}},
			want:    map[string]any{},
			issues: []lint.OptionIssue{
				{Kind: lint.OptionIssueWrongType, Key: "lines_above", Value: 2.5, Item: 2, Option: lengthSpecs()[4]},
				{Kind: lint.OptionIssueWrongType, Key: "lines_above", Value: "3", Item: 3, Option: lengthSpecs()[4]},
			},
		},
		{
			name:    "a whole number written as a float is an integer item",
			options: map[string]any{"lines_above": []any{1, float64(2)}},
			want:    map[string]any{"lines_above": []any{1, float64(2)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before := cloneOptions(tt.options)
			got, issues := lint.NormalizeOptions(lengthSpecs(), tt.options)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.issues, issues)
			assert.Equal(t, before, tt.options, "the options given are left alone")
		})
	}
}

// cloneOptions returns a shallow copy of options.
func cloneOptions(options map[string]any) map[string]any {
	clone := make(map[string]any, len(options))
	for key, value := range options {
		clone[key] = value
	}
	return clone
}

func TestNormalizeOptions_NoOptions(t *testing.T) {
	t.Parallel()

	got, issues := lint.NormalizeOptions(lengthSpecs(), nil)
	assert.Nil(t, got)
	assert.Empty(t, issues)

	got, issues = lint.NormalizeOptions(nil, map[string]any{"style": "dash"})
	assert.Equal(t, map[string]any{"style": "dash"}, got)
	assert.Equal(t, []lint.OptionIssue{{
		Kind: lint.OptionIssueUnknown, Key: "style", Value: "dash", NoOptions: true,
	}}, issues, "every option of a rule that reads none is unknown")
}

func TestNormalizeOptions_Suggestions(t *testing.T) {
	t.Parallel()

	specs := []lint.OptionSpec{
		{Name: "max", Aliases: []lint.OptionAlias{{Name: "line_length"}}, Type: lint.OptionInteger},
		{Name: "heading_max", Aliases: []lint.OptionAlias{{Name: "heading_line_length"}}, Type: lint.OptionInteger},
		{Name: "code_block_max", Aliases: []lint.OptionAlias{{Name: "code_block_line_length"}}, Type: lint.OptionInteger},
		{Name: "strict", Type: lint.OptionBoolean},
		{Name: "stern", Type: lint.OptionBoolean},
	}

	tests := []struct {
		key  string
		want []string
	}{
		{key: "stirct", want: []string{"strict"}},
		{key: "heading_length", want: nil},
		{key: "length", want: []string{"code_block_line_length", "heading_line_length", "line_length"}},
		{key: "_max", want: []string{"max"}},
		{key: "frobnicate", want: nil},
		{key: "stren", want: []string{"stern"}},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			_, issues := lint.NormalizeOptions(specs, map[string]any{tt.key: true})
			require.Len(t, issues, 1)
			assert.Equal(t, tt.want, issues[0].Suggestions)
		})
	}
}

func TestOptionIssueText(t *testing.T) {
	t.Parallel()

	specs := lengthSpecs()
	tests := []struct {
		name  string
		issue lint.OptionIssue
		want  string
	}{
		{
			name:  "unknown with a suggestion",
			issue: lint.OptionIssue{Kind: lint.OptionIssueUnknown, Key: "lin_length", Suggestions: []string{"line_length"}},
			want:  `unknown option "lin_length" for line-length (did you mean "line_length"?); it is ignored`,
		},
		{
			name:  "unknown with several suggestions",
			issue: lint.OptionIssue{Kind: lint.OptionIssueUnknown, Key: "length", Suggestions: []string{"a", "b", "c"}},
			want:  `unknown option "length" for line-length (did you mean "a", "b" or "c"?); it is ignored`,
		},
		{
			name:  "unknown without a suggestion",
			issue: lint.OptionIssue{Kind: lint.OptionIssueUnknown, Key: "frobnicate"},
			want:  `unknown option "frobnicate" for line-length; it is ignored`,
		},
		{
			name:  "unknown for a rule that reads no options",
			issue: lint.OptionIssue{Kind: lint.OptionIssueUnknown, Key: "style", NoOptions: true},
			want:  `unknown option "style" for line-length (it has no options); it is ignored`,
		},
		{
			name:  "an integer",
			issue: lint.OptionIssue{Kind: lint.OptionIssueWrongType, Key: "line_length", Value: "100px", Option: specs[0]},
			want:  `line-length option "line_length" must be an integer, not "100px"; it is ignored`,
		},
		{
			name:  "a boolean",
			issue: lint.OptionIssue{Kind: lint.OptionIssueWrongType, Key: "tables", Value: 0, Option: specs[1]},
			want:  `line-length option "tables" must be true or false, not 0; it is ignored`,
		},
		{
			name:  "a string",
			issue: lint.OptionIssue{Kind: lint.OptionIssueWrongType, Key: "style", Value: []any{"dash"}, Option: specs[2]},
			want:  `line-length option "style" must be a string, not ["dash"]; it is ignored`,
		},
		{
			name:  "a list",
			issue: lint.OptionIssue{Kind: lint.OptionIssueWrongType, Key: "names", Value: "JavaScript", Option: specs[3]},
			want:  `line-length option "names" must be a list of strings, not "JavaScript"; it is ignored`,
		},
		{
			name:  "an integer or a list",
			issue: lint.OptionIssue{Kind: lint.OptionIssueWrongType, Key: "lines_above", Value: true, Option: specs[4]},
			want:  `line-length option "lines_above" must be an integer or a list of integers, not true; it is ignored`,
		},
		{
			name: "a pattern",
			issue: lint.OptionIssue{
				Kind: lint.OptionIssueWrongType, Key: "front_matter_title", Value: map[string]any{"a": 1}, Option: specs[5],
			},
			want: `line-length option "front_matter_title" must be a string or false, not {"a":1}; it is ignored`,
		},
		{
			name: "a value outside the allowed ones",
			issue: lint.OptionIssue{
				Kind: lint.OptionIssueNotAllowed, Key: "style", Value: "dashes", Option: specs[2], Suggestions: []string{"dash"},
			},
			want: `line-length option "style" must be one of "consistent", "dash" or "plus", not "dashes" (did you mean "dash"?)`,
		},
		{
			name:  "a value outside the allowed ones, unlike any",
			issue: lint.OptionIssue{Kind: lint.OptionIssueNotAllowed, Key: "style", Value: "zigzag", Option: specs[2]},
			want:  `line-length option "style" must be one of "consistent", "dash" or "plus", not "zigzag"`,
		},
		{
			name:  "two spellings",
			issue: lint.OptionIssue{Kind: lint.OptionIssueConflict, Key: "line_length", Other: "max", Option: specs[0]},
			want:  `"line_length" and "max" both set line-length option "max"; keep one of them`,
		},
		{
			name:  "an item of a list of strings",
			issue: lint.OptionIssue{Kind: lint.OptionIssueWrongType, Key: "names", Value: 42, Item: 2, Option: specs[3]},
			want:  `line-length option "names" item 2 must be a string, not 42; the option is ignored`,
		},
		{
			name:  "an item of a list of integers",
			issue: lint.OptionIssue{Kind: lint.OptionIssueWrongType, Key: "lines_above", Value: 2.5, Item: 1, Option: specs[4]},
			want:  `line-length option "lines_above" item 1 must be an integer, not 2.5; the option is ignored`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.issue.Text("line-length"))
		})
	}
}

// optionsRegistry registers a rule whose catalog entry declares the
// options of lengthSpecs, and one without an entry.
func optionsRegistry() *lint.Registry {
	registry := lint.NewRegistry()
	registry.Register(&optionRule{id: "MD013", name: "line-length"})
	registry.Register(&optionRule{id: "MD005", name: "list-indent"})
	registry.Register(&optionRule{id: "MD999", name: "custom"})
	registry.Describe(lint.RuleMeta{ID: "MD013", Name: "line-length", Options: lengthSpecs(), OptionsDeclared: true})
	registry.Describe(lint.RuleMeta{ID: "MD005", Name: "list-indent", OptionsDeclared: true})
	return registry
}

// optionRule is a rule that reads nothing and reports nothing.
type optionRule struct {
	id, name string
}

func (r *optionRule) ID() string                                         { return r.id }
func (r *optionRule) Name() string                                       { return r.name }
func (r *optionRule) Description() string                                { return "test rule" }
func (r *optionRule) DefaultEnabled() bool                               { return true }
func (r *optionRule) DefaultSeverity() config.Severity                   { return config.SeverityWarning }
func (r *optionRule) Tags() []string                                     { return nil }
func (r *optionRule) CanFix() bool                                       { return false }
func (r *optionRule) Apply(*lint.RuleContext) ([]lint.Diagnostic, error) { return nil, nil }

func TestRegistryNormalizeOptions(t *testing.T) {
	t.Parallel()

	registry := optionsRegistry()

	got, issues, declared := registry.NormalizeOptions("MD013", map[string]any{"line_length": 100, "tables": false})
	assert.True(t, declared)
	assert.Equal(t, map[string]any{"max": 100, "ignore_tables": true}, got)
	assert.Empty(t, issues)

	got, issues, declared = registry.NormalizeOptions("MD005", map[string]any{"indent": 2})
	assert.True(t, declared, "an entry may declare that its rule reads no options")
	assert.Equal(t, map[string]any{"indent": 2}, got)
	require.Len(t, issues, 1)
	assert.True(t, issues[0].NoOptions)

	options := map[string]any{"line_length": 100}
	for _, id := range []string{"MD999", "MD404"} {
		got, issues, declared = registry.NormalizeOptions(id, options)
		assert.False(t, declared, "%s has no entry that declares its options", id)
		assert.Equal(t, options, got, "%s: the options stay as they are", id)
		assert.Empty(t, issues, id)
	}
}

// TestRegistryNormalizeOptions_IssuesShareNothing checks that the option
// in an issue is a copy: changing its aliases, allowed values or list
// default leaves the registry's declaration alone.
func TestRegistryNormalizeOptions_IssuesShareNothing(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(&optionRule{id: "MD044", name: "proper-names"})
	registry.Describe(lint.RuleMeta{ID: "MD044", Name: "proper-names", OptionsDeclared: true, Options: []lint.OptionSpec{
		{
			Name: "style", Aliases: []lint.OptionAlias{{Name: "old_style"}}, Type: lint.OptionString,
			Default: "consistent", Values: []string{"consistent", "dash"},
		},
		{Name: "names", Type: lint.OptionStringList, Default: []string{"Go"}},
	}})
	options := map[string]any{"old_style": "zigzag", "names": "Go"}

	const changed = "changed"
	_, issues, declared := registry.NormalizeOptions("MD044", options)
	require.True(t, declared)
	require.Len(t, issues, 2)
	for _, issue := range issues {
		if len(issue.Option.Aliases) > 0 {
			issue.Option.Aliases[0].Name = changed
		}
		if len(issue.Option.Values) > 0 {
			issue.Option.Values[0] = changed
		}
		if names, ok := issue.Option.Default.([]string); ok {
			names[0] = changed
		}
	}

	meta, ok := registry.Meta("MD044")
	require.True(t, ok)
	assert.Equal(t, "old_style", meta.Options[0].Aliases[0].Name)
	assert.Equal(t, []string{"consistent", "dash"}, meta.Options[0].Values)
	assert.Equal(t, []string{"Go"}, meta.Options[1].Default)

	_, again, _ := registry.NormalizeOptions("MD044", options)
	assert.Equal(t, issues[0].Key, again[0].Key)
	assert.Equal(t, []string{"consistent", "dash"}, again[1].Option.Values, "a later issue sees the declaration as it was")
}

func TestRegistryNormalizeOptions_UndeclaredEntry(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(&optionRule{id: "MD013", name: "line-length"})
	registry.Describe(lint.RuleMeta{ID: "MD013", Name: "line-length", Options: lengthSpecs()})

	options := map[string]any{"line_length": 100}
	got, issues, declared := registry.NormalizeOptions("MD013", options)
	assert.False(t, declared, "an entry that does not declare its options checks nothing")
	assert.Equal(t, options, got)
	assert.Empty(t, issues)
}

// TestRegistryMarkdownlintRuleConfig covers the translation of a rule's
// setting in a markdownlint configuration through its declared options:
// markdownlint's spellings become canonical, and options that the rule
// cannot use are left out, as markdownlint ignores them.
func TestRegistryMarkdownlintRuleConfig(t *testing.T) {
	t.Parallel()

	registry := optionsRegistry()

	cfg, issues := registry.MarkdownlintRuleConfig("MD013", map[string]any{
		"line_length": 100, "tables": false, "lin_length": 1, "style": 3, "severity": "warning",
	})
	assert.Equal(t, map[string]any{"max": 100, "ignore_tables": true}, cfg.Options)
	require.NotNil(t, cfg.Severity)
	assert.Equal(t, "warning", *cfg.Severity)
	require.Len(t, issues, 2)
	assert.Equal(t, lint.OptionIssueUnknown, issues[0].Kind)
	assert.Equal(t, "lin_length", issues[0].Key)
	assert.Equal(t, lint.OptionIssueWrongType, issues[1].Kind)
	assert.Equal(t, "style", issues[1].Key)

	cfg, issues = registry.MarkdownlintRuleConfig("MD013", map[string]any{"line_length": 100, "max": 90})
	assert.Equal(t, map[string]any{"max": 90}, cfg.Options, "the canonical spelling wins")
	require.Len(t, issues, 1)
	assert.Equal(t, lint.OptionIssueConflict, issues[0].Kind)

	cfg, issues = registry.MarkdownlintRuleConfig("MD005", map[string]any{"indent": 2})
	assert.Nil(t, cfg.Options, "a rule without options keeps none")
	require.Len(t, issues, 1)
	assert.True(t, issues[0].NoOptions)

	cfg, issues = registry.MarkdownlintRuleConfig("MD999", map[string]any{"any": 1})
	assert.Equal(t, map[string]any{"any": 1}, cfg.Options, "a rule without declared options keeps what is written")
	assert.Empty(t, issues)

	cfg, issues = registry.MarkdownlintRuleConfig("MD013", false)
	require.NotNil(t, cfg.Enabled)
	assert.False(t, *cfg.Enabled)
	assert.Nil(t, cfg.Options)
	assert.Empty(t, issues)
}

// TestRegistryMarkdownlintRuleConfigWritesPinnedOptions checks the options
// that every markdownlint setting of a rule writes out: the defaults of the
// options that its declaration marks MarkdownlintPinned, whatever the value
// and under the default key too, with the options that a setting gives
// over them, each setting with its own copy of the defaults.
func TestRegistryMarkdownlintRuleConfigWritesPinnedOptions(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(&optionRule{id: "MD033", name: "no-inline-html"})
	registry.Describe(lint.RuleMeta{ID: "MD033", Name: "no-inline-html", OptionsDeclared: true, Options: []lint.OptionSpec{
		{Name: "allowed_elements", Type: lint.OptionStringList, Default: []string{"br"}, MarkdownlintPinned: true},
		{Name: "table_allowed_elements", Type: lint.OptionStringList, Default: []string{}},
	}})
	pinned := map[string]any{"allowed_elements": []string{"br"}}

	values := []any{true, false, nil, "warning", "", float64(0), map[string]any{}, map[string]any{"enabled": false}}
	for _, value := range values {
		cfg, issues := registry.MarkdownlintRuleConfig("MD033", value)
		assert.Equal(t, pinned, cfg.Options, "%#v", value)
		assert.Empty(t, issues, "%#v", value)
		assert.Equal(t, pinned, registry.MarkdownlintDefaultRuleConfig("MD033", value).Options, "default key %#v", value)
	}

	cfg, issues := registry.MarkdownlintRuleConfig("MD033", map[string]any{
		"allowed_elements": []any{"img"}, "table_allowed_elements": []any{"td"},
	})
	assert.Equal(t, map[string]any{"allowed_elements": []any{"img"}, "table_allowed_elements": []any{"td"}}, cfg.Options,
		"the options of the setting win")
	assert.Empty(t, issues)

	cfg, _ = registry.MarkdownlintRuleConfig("MD033", true)
	cfg.Options["allowed_elements"].([]string)[0] = "hr"
	assert.Equal(t, pinned, registry.MarkdownlintDefaultRuleConfig("MD033", true).Options, "each setting has its own copy")
	meta, ok := registry.Meta("MD033")
	require.True(t, ok)
	assert.Equal(t, []string{"br"}, meta.Options[0].Default, "the declaration keeps its default")

	assert.Nil(t, registry.MarkdownlintDefaultRuleConfig("MD404", true).Options, "an unknown rule writes no options")
}

// TestResolveRulesNormalizesOptions covers the options that rules see: the
// canonical spellings, whatever spelling the configuration used, with
// rules overriding rule_defaults option by option.
func TestResolveRulesNormalizesOptions(t *testing.T) {
	t.Parallel()

	registry := optionsRegistry()
	baseline := map[string]any{"max": 80, "tables": false, "style": "plus"}
	ruleOptions := map[string]any{"line_length": 100, "names": []string{"Go"}, "lines_above": "2"}
	cfg := config.NewConfig()
	cfg.RuleDefaults = map[string]config.RuleConfig{"MD013": {Options: baseline}}
	cfg.Rules = map[string]config.RuleConfig{"MD013": {Options: ruleOptions}}

	var got map[string]any
	for _, resolved := range lint.ResolveRules(registry, cfg) {
		if resolved.Rule.ID() == "MD013" {
			got = resolved.Config.Options
		}
	}

	assert.Equal(t, map[string]any{
		"max": 100, "ignore_tables": true, "style": "plus", "names": []any{"Go"},
	}, got, "rules win over rule_defaults by option, and a value of the wrong type is dropped")
	assert.Equal(t, map[string]any{"max": 80, "tables": false, "style": "plus"}, baseline, "rule_defaults are left alone")
	assert.Equal(t, map[string]any{"line_length": 100, "names": []string{"Go"}, "lines_above": "2"}, ruleOptions,
		"rules are left alone")
}

func TestResolveRulesKeepsCanonicalOptions(t *testing.T) {
	t.Parallel()

	registry := optionsRegistry()
	options := map[string]any{"max": 100}
	cfg := config.NewConfig()
	cfg.Rules = map[string]config.RuleConfig{"MD013": {Options: options}, "MD999": {Options: map[string]any{"any": 1}}}

	for _, resolved := range lint.ResolveRules(registry, cfg) {
		switch resolved.Rule.ID() {
		case "MD013":
			assert.Equal(t, options, resolved.Config.Options)
		case "MD999":
			assert.Equal(t, map[string]any{"any": 1}, resolved.Config.Options, "a rule without declared options gets them as given")
		}
	}
}
