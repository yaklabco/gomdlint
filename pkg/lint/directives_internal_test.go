package lint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// resolveOne resolves the words of <!-- markdownlint-disable parameter -->,
// a comment on line 7, as collectDirectives resolves them.
func resolveOne(registry *Registry, parameter string) inlineDirective {
	text := "<!-- markdownlint-disable " + parameter + " -->"
	matches := findDirectives(text)
	directives := make([]inlineDirective, len(matches))
	for idx, match := range matches {
		directives[idx] = inlineDirective{line: 7, action: match.kind, parameter: text[match.kindEnd:match.close]}
	}
	(&wordResolver{registry: registry}).resolveComments(text, matches, directives)
	return directives[0]
}

// TestWordResolverRuleList pins how the words of a markdownlint comment
// resolve to rules, as before notices existed, and which words get notices.
func TestWordResolverRuleList(t *testing.T) {
	// withCustomTag adds a rule that tags itself acme, outside the catalog.
	withCustomTag := func() *Registry {
		reg := describedRegistry()
		reg.Register(&mockRule{id: "ACME001", name: "acme-rule", tags: []string{"acme"}})
		return reg
	}
	// withPhantomBeyondASCII adds a catalog entry, for a rule that is not
	// registered, whose alias holds a letter beyond ASCII.
	withPhantomBeyondASCII := func() *Registry {
		reg := describedRegistry()
		reg.Describe(RuleMeta{ID: "MD997", Name: "phantom-two", Aliases: []string{"fant\u00f4me"}})
		return reg
	}
	// withoutCatalog holds rules that tag themselves, without catalog
	// entries.
	withoutCatalog := func() *Registry {
		reg := NewRegistry()
		reg.Register(&mockRule{id: "MD004", name: "unordered-list-style", tags: []string{"lists", "style"}})
		reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces", tags: []string{"whitespace"}})
		return reg
	}
	every := []string{"MD004", "MD009", "MD026", "MD043", "MD047", "MD055"}

	type noticed struct {
		kind  NoticeKind
		name  string
		rules []string
	}
	tests := []struct {
		name      string
		registry  func() *Registry
		parameter string
		rules     []string
		notices   []noticed
	}{
		{name: "no words name every rule", registry: describedRegistry, parameter: "", rules: every},
		{name: "whitespace names every rule", registry: describedRegistry, parameter: " \t\n ", rules: every},
		{
			name: "an unknown word names nothing", registry: describedRegistry, parameter: " frobnicate ",
			notices: []noticed{{kind: NoticeUnknownRule, name: "frobnicate"}},
		},
		{
			name: "an unknown word leaves the others", registry: describedRegistry, parameter: "frobnicate MD009",
			rules: []string{"MD009"}, notices: []noticed{{kind: NoticeUnknownRule, name: "frobnicate"}},
		},
		{
			name: "repeats in any case", registry: describedRegistry, parameter: "md009 No-Trailing-Spaces MD009 FROB frob",
			rules: []string{"MD009"}, notices: []noticed{{kind: NoticeUnknownRule, name: "FROB"}},
		},
		{name: "an alias and a catalog tag", registry: describedRegistry, parameter: "ul-style bullet", rules: []string{"MD004"}},
		{name: "catalog words of a rule that is not registered", registry: describedRegistry, parameter: "phantom-alias MD998 phantom-rule"},
		{
			name: "a catalog alias beyond ASCII of a rule that is not registered", registry: withPhantomBeyondASCII,
			parameter: "fant\u00f4me FANT\u00d4ME",
		},
		{
			name: "a former tag", registry: describedRegistry, parameter: "lists LISTS",
			rules:   []string{"MD004"},
			notices: []noticed{{kind: NoticeDeprecatedTag, name: "lists", rules: []string{"unordered-list-style"}}},
		},
		{name: "the tag of a rule outside the catalog", registry: withCustomTag, parameter: "acme", rules: []string{"ACME001"}},
		{name: "a markdownlint tag without the catalog", registry: withoutCatalog, parameter: "whitespace", rules: []string{"MD009"}},
		{
			name: "a former tag without the catalog", registry: withoutCatalog, parameter: "style",
			rules:   []string{"MD004"},
			notices: []noticed{{kind: NoticeDeprecatedTag, name: "style", rules: []string{"unordered-list-style"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directive := resolveOne(tt.registry(), tt.parameter)

			if len(tt.rules) == 0 {
				assert.Empty(t, directive.rules)
			} else {
				assert.Equal(t, tt.rules, directive.rules)
			}
			var got []noticed
			for _, notice := range directive.notices {
				assert.Equal(t, 7, notice.Line)
				assert.Equal(t, "markdownlint-disable", notice.Directive)
				got = append(got, noticed{kind: notice.Kind, name: notice.Name, rules: notice.Rules})
			}
			assert.Equal(t, tt.notices, got)
		})
	}
}

// identityRegistry holds three rules with catalog entries, two of which
// markdownlint knows by other names, four rules without, whose names and
// tag hold letters beyond ASCII, and one whose catalog alias and tag do.
func identityRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(&mockRule{id: "MD004", name: "unordered-list-style"})
	reg.Register(&mockRule{id: "MD009", name: "no-trailing-spaces"})
	reg.Register(&mockRule{id: "MD012", name: "no-multiple-blank-lines"})
	reg.Register(&mockRule{id: "X001", name: "caf\u00e9-check", tags: []string{"t\u00e2che"}})
	reg.Register(&mockRule{id: "X002", name: "\u212aelvin-check"})
	reg.Register(&mockRule{id: "X003", name: "\u0130nfo-check"})
	reg.Register(&mockRule{id: "X004", name: "stra\u00dfe-check"})
	reg.Register(&mockRule{id: "X005", name: "x-five"})
	reg.Describe(RuleMeta{ID: "X005", Name: "x-five", Aliases: []string{"cr\u00e8me"}, Tags: []string{"\u00e9t\u00e9"}})
	reg.Describe(RuleMeta{
		ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"}, MarkdownlintName: "ul-style",
		Tags: []string{"bullet", "ul"},
	})
	reg.Describe(RuleMeta{ID: "MD009", Name: "no-trailing-spaces", Tags: []string{"whitespace"}})
	reg.Describe(RuleMeta{
		ID: "MD012", Name: "no-multiple-blank-lines", Aliases: []string{"no-multiple-blanks"}, MarkdownlintName: "no-multiple-blanks",
		Tags: []string{"whitespace", "blank_lines"},
	})
	return reg
}

// TestKeyIdentityAgrees runs spellings of a rule's name or tag through the
// three places that compare the words of markdownlint comments and the keys
// of configure-file objects: resolution, the order of configure-file keys
// and the deduplication of notices. They use one key identity,
// markdownlint's, so they agree on whether each spelling means the same as
// the one the rule was registered with, ASCII or not.
func TestKeyIdentityAgrees(t *testing.T) {
	registry := identityRegistry()
	tests := []struct {
		name, base, spelling string
		same                 bool
		tag                  bool // base is a tag, which no configure-file key orders as a rule.
	}{
		{name: "ASCII case", base: "no-trailing-spaces", spelling: "No-Trailing-SPACES", same: true},
		{name: "long s", base: "no-trailing-spaces", spelling: "no-trailing-\u017fpaces", same: true},
		{name: "dotless i", base: "no-trailing-spaces", spelling: "no-tra\u0131ling-spaces", same: true},
		{name: "Kelvin sign", base: "no-multiple-blanks", spelling: "no-multiple-blan\u212as"},
		{name: "dotted capital I", base: "no-trailing-spaces", spelling: "NO-TRA\u0130LING-SPACES"},
		{name: "custom name beyond ASCII", base: "caf\u00e9-check", spelling: "CAF\u00c9-CHECK", same: true},
		{name: "custom tag beyond ASCII", base: "t\u00e2che", spelling: "T\u00c2CHE", same: true, tag: true},
		{name: "k for a Kelvin sign", base: "\u212aelvin-check", spelling: "kelvin-check"},
		{name: "i for a dotted capital I", base: "\u0130nfo-check", spelling: "info-check"},
		{name: "sharp s", base: "stra\u00dfe-check", spelling: "STRASSE-CHECK", same: true},
		{name: "catalog alias beyond ASCII", base: "cr\u00e8me", spelling: "CR\u00c8ME", same: true},
		{name: "catalog tag beyond ASCII", base: "\u00e9t\u00e9", spelling: "\u00c9T\u00c9", same: true, tag: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseRules, baseMeaning := lookupWord(tt.base, registry)
			require.Equal(t, knownWord, baseMeaning)
			rules, meaning := lookupWord(tt.spelling, registry)
			if tt.same {
				assert.Equal(t, sortedIDs(baseRules), sortedIDs(rules), "resolution")
			} else {
				assert.Equal(t, unknownWord, meaning, "resolution")
			}

			require.Equal(t, !tt.tag, ruleKey(tt.base, registry))
			assert.Equal(t, tt.same && !tt.tag, ruleKey(tt.spelling, registry), "the order of configure-file keys")

			wantNotices := 2
			if tt.same {
				wantNotices = 1
			}
			directive := resolveOne(registry, tt.base+"-x "+tt.spelling+"-x")
			assert.Len(t, directive.notices, wantNotices, "deduplication of notices")
		})
	}
}

// TestMarkdownlintSuggestionsNeverOfferTheWord covers the suggestions for a
// word: never a rule or tag whose name has the word's key, which is the
// word itself as markdownlint reads it, and tags as their catalog entries
// write them, which markdownlint reads too.
func TestMarkdownlintSuggestionsNeverOfferTheWord(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockRule{id: "X001", name: "caf\u00e9-check"})
	registry.Register(&mockRule{id: "X004", name: "tagged-rule"})
	registry.Describe(RuleMeta{ID: "X004", Name: "tagged-rule", Tags: []string{"\u212aelvin-tag"}})
	// A catalog entry whose markdownlint name is not one of its aliases,
	// against RuleMeta's contract, so the name names nothing.
	registry.Register(&mockRule{id: "X006", name: "acme-four"})
	registry.Describe(RuleMeta{ID: "X006", Name: "acme-four", MarkdownlintName: "acme-fours"})

	for _, word := range []string{"caf\u00e9-check", "CAF\u00c9-CHECK", "\u212aelvin-tag", "\u212aELVIN-TAG", "acme-fours"} {
		assert.Empty(t, registry.markdownlintSuggestions(word), word)
	}
	assert.Equal(t, []Suggestion{{Name: "caf\u00e9-check", RuleID: "X001"}}, registry.markdownlintSuggestions("cafe-check"))
	assert.Equal(t, []Suggestion{{Name: "\u212aelvin-tag"}}, registry.markdownlintSuggestions("kelvin-tag"))

	// Where one catalog entry writes a tag in capitals and another in lower
	// case, the suggestion takes the lower case, whichever comes first.
	registry.Register(&mockRule{id: "ACME1", name: "acme-one"})
	registry.Describe(RuleMeta{ID: "ACME1", Name: "acme-one", Tags: []string{"BULLET"}})
	registry.Register(&mockRule{id: "MD004", name: "unordered-list-style"})
	registry.Describe(RuleMeta{ID: "MD004", Name: "unordered-list-style", Tags: []string{"bullet"}})
	assert.Equal(t, []Suggestion{{Name: "bullet"}}, registry.markdownlintSuggestions("bullt"))
}

// TestMarkdownlintTerm covers how a remedy writes a word for a markdownlint
// comment: a rule named by gomdlint's own name, in any spelling that means
// it, becomes markdownlint's name for the rule, and every other word stays
// as written.
func TestMarkdownlintTerm(t *testing.T) {
	registry := identityRegistry()
	tests := map[string]string{
		"unordered-list-style":         "ul-style",
		"Unordered-List-\u017ftyle":    "ul-style",
		"no-multiple-blank-lines":      "no-multiple-blanks",
		"UL-STYLE":                     "UL-STYLE",
		"md012":                        "md012",
		"No-Trailing-Spaces":           "No-Trailing-Spaces",
		"whitespace":                   "whitespace",
		"frobnicate":                   "frobnicate",
		"unordered-list-\u212a":        "unordered-list-\u212a",
		"no-multiple-blan\u212a-lines": "no-multiple-blan\u212a-lines",
	}
	for word, want := range tests {
		t.Run(word, func(t *testing.T) {
			assert.Equal(t, want, registry.markdownlintTerm(word))
		})
	}
}

// TestMarkdownlintNameFollowsTheRuleID covers how notices name a rule in
// hand, as a former tag lists it or a suggestion offers it: by
// markdownlint's name for the rule's ID, even when the rule registered
// under that ID has a name of its own, which markdownlint does not know.
// A rule without a catalog entry keeps its own name, and a rule without a
// name is named by its ID.
func TestMarkdownlintNameFollowsTheRuleID(t *testing.T) {
	registry := NewRegistry()
	listStyle := &mockRule{id: "MD004", name: "my-list-style"}
	custom := &mockRule{id: "X001", name: "custom-rule"}
	nameless := &mockRule{id: "X002"}
	for _, rule := range []Rule{listStyle, custom, nameless} {
		registry.Register(rule)
	}
	registry.Describe(RuleMeta{
		ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"}, MarkdownlintName: "ul-style",
	})

	assert.Equal(t, "ul-style", registry.markdownlintName(listStyle))
	assert.Equal(t, "custom-rule", registry.markdownlintName(custom))
	assert.Equal(t, "X002", registry.markdownlintName(nameless))

	suggestions := registry.markdownlintSuggestions("my-list-styel")
	require.Len(t, suggestions, 1)
	assert.Equal(t, Suggestion{Name: "ul-style", RuleID: "MD004"}, suggestions[0])
}

// TestFindDirectives covers where comments start, their kinds, and where
// their parameters end: at the next "-->" after the kind, which a comment
// inside another comment's parameter shares. The search resumes right after
// the kind, so it finds that inner comment, and it stops at a comment
// without a closing "-->".
func TestFindDirectives(t *testing.T) {
	text := "x <!-- markdownlint-disable a <!-- MARKDOWNLINT-ENABLE b --> y <!-- markdownlint-capture--> " +
		"<!-- markdownlint-disablex --> <!-- markdownlint-restore z"

	var got []string
	for _, match := range findDirectives(text) {
		got = append(got, fmt.Sprintf("%d %s %q", match.start, match.kind, text[match.kindEnd:match.close]))
	}

	assert.Equal(t, []string{
		`2 disable " a <!-- MARKDOWNLINT-ENABLE b "`,
		`30 enable " b "`,
		`63 capture ""`,
	}, got)
}

// TestJSWhitespaceAgreesWithItsPattern checks that isJSWhitespace and the
// jsWhitespace pattern hold the same characters, every one of them.
func TestJSWhitespaceAgreesWithItsPattern(t *testing.T) {
	pattern := regexp.MustCompile(`^` + jsWhitespace + `$`)
	for char := rune(0); char <= utf8.MaxRune; char++ {
		if !utf8.ValidRune(char) {
			continue
		}
		if pattern.MatchString(string(char)) != isJSWhitespace(char) {
			t.Fatalf("U+%04X: the pattern says %v", char, pattern.MatchString(string(char)))
		}
	}
}

// TestNestedDirectivesCostLinearWork reads a comment that holds many
// comment starts before its one "-->". Each start is a comment whose
// parameter runs to that "-->", so the parameters overlap. Reading them
// must cost memory in proportion to the text, not to the square of the
// number of comments.
func TestNestedDirectivesCostLinearWork(t *testing.T) {
	registry := describedRegistry()
	allocated := func(starts int) uint64 {
		content := "# T\n\n" + strings.Repeat("<!-- markdownlint-disable a\n", starts) + "-->\n"
		snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "doc.md", []byte(content))
		require.NoError(t, err)

		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		directives := collectDirectives(snapshot, registry)
		runtime.ReadMemStats(&after)

		require.Len(t, directives, starts)
		return after.TotalAlloc - before.TotalAlloc
	}

	small, large := allocated(1000), allocated(4000)

	assert.Less(t, large, 8*small, "four times the comments may cost about four times the memory, not sixteen")
}

// blankLineRule removes one blank line from the first run of two or more.
type blankLineRule struct {
	BaseRule
}

func (r *blankLineRule) Apply(ctx *RuleContext) ([]Diagnostic, error) {
	idx := bytes.Index(ctx.File.Content, []byte("\n\n\n"))
	if idx < 0 {
		return nil, nil
	}
	return []Diagnostic{{
		RuleID: r.ID(), Message: "blank lines", StartLine: bytes.Count(ctx.File.Content[:idx], []byte("\n")) + 2, StartColumn: 1,
		FixEdits: []fix.TextEdit{{StartOffset: idx, EndOffset: idx + 1}},
	}}, nil
}

// TestPipeline_RefusedWriteReportsTheNoticesOfTheFileAsItIs covers a fix
// that is not written because another program saved the file first. The
// result describes the file as it is, so its notices carry the lines of
// the content that was read, not of the fixed content.
func TestPipeline_RefusedWriteReportsTheNoticesOfTheFileAsItIs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	content := "# Title\n\n\n\n<!-- markdownlint-disable frobnicate -->\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	registry := NewRegistry()
	registry.Register(&blankLineRule{BaseRule: NewBaseRule("TEST010", "blank-lines", "", nil, true)})
	pipeline := NewPipeline(NewEngine(goldmark.New(goldmark.FlavorGFM), registry))
	pipeline.beforeWrite = func() {
		require.NoError(t, os.WriteFile(path, []byte(content+"Saved meanwhile.\n"), 0o644))
	}
	cfg := config.NewConfig()
	cfg.Fix = true

	result, err := pipeline.ProcessFile(context.Background(), path, cfg, withBackups(root))
	require.NoError(t, err)

	requireRefused(t, result)
	require.Len(t, result.Notices, 1)
	assert.Equal(t, 5, result.Notices[0].Line, "the comment's line in the file as it is; the fixes would move it to line 3")
}
