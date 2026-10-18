package lint_test

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// lintDefault lints input with every built-in rule at its default settings,
// computing fixes as --fix would.
func lintDefault(t *testing.T, input string) *lint.FileResult {
	t.Helper()
	cfg := config.NewConfig()
	cfg.Fix = true
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
		context.Background(), "doc.md", []byte(input), cfg)
	require.NoError(t, err)
	return result
}

// withoutDirectives turns every markdownlint comment in input into a plain
// HTML comment of the same length, so the document keeps its structure and
// byte offsets but configures nothing.
func withoutDirectives(input string) string {
	return strings.ReplaceAll(input, "markdownlint-", "notdirective-")
}

// TestEngineUnknownDirectiveNameOnlyWarns covers a typo in a markdownlint
// comment. The comment does nothing for the word, as in markdownlint, so
// the findings and their fixes are those of the same document without the
// comment. The file gets a notice that names the closest rule.
func TestEngineUnknownDirectiveNameOnlyWarns(t *testing.T) {
	input := "# Title\n\n<!-- markdownlint-disable no-trailling-spaces -->\n\nTrailing space \n"

	got := lintDefault(t, input)
	want := lintDefault(t, withoutDirectives(input))

	require.NotEmpty(t, want.Diagnostics, "the document must have findings")
	assert.Equal(t, want.Diagnostics, got.Diagnostics, "the comment must not change the findings")
	assert.Equal(t, want.Edits, got.Edits, "the comment must not change the fixes")
	assert.Empty(t, want.Notices)
	assert.Equal(t, []lint.Notice{{
		Kind:        lint.NoticeUnknownRule,
		Line:        3,
		Directive:   "markdownlint-disable",
		Name:        "no-trailling-spaces",
		Suggestions: []string{"no-trailing-spaces"},
		Message: `markdownlint-disable: unknown rule "no-trailling-spaces" ` +
			`(did you mean "no-trailing-spaces"?); it is ignored`,
	}}, got.Notices)
}

// TestEngineFormerDirectiveTagsWarnAndWork covers gomdlint's former tags,
// which markdownlint does not have. They still silence the rules they
// cover, through the tags that rules report themselves, and each use gets a
// notice that names those rules and says the tag will stop working.
func TestEngineFormerDirectiveTagsWarnAndWork(t *testing.T) {
	const (
		mixedBullets = "- one\n\n* two\n"
		extraBlank   = "Text.\n\n\nMore.\n"
		shortTable   = "| A | B |\n| - | - |\n| 1 | 2 |\n"
	)
	tests := []struct {
		tag, ruleID, body string
		enable            []string
		covers            []string
	}{
		// The rules are named as markdownlint names them, such as ul-style
		// for unordered-list-style, so that the comment works in both tools.
		// gomdlint's own rules have only their own names.
		{
			tag: "style", ruleID: "MD004", body: mixedBullets,
			covers: []string{"heading-style", "ul-style", "code-block-style", "code-fence-style", "link-destination-style"},
		},
		{tag: "lists", ruleID: "MD004", body: mixedBullets, covers: []string{"ul-style"}},
		{tag: "layout", ruleID: "MD012", body: extraBlank, covers: []string{"no-multiple-blanks"}},
		{tag: "metadata", ruleID: "MD041", body: "Text.\n", enable: []string{"MD041"}, covers: []string{"first-line-heading"}},
		{tag: "gfm", ruleID: "MDL003", body: shortTable, covers: []string{"table-alignment"}},
		{tag: "tables", ruleID: "MDL003", body: shortTable, covers: []string{"table-alignment"}},
		{tag: "Style", ruleID: "MD004", body: mixedBullets,
			covers: []string{"heading-style", "ul-style", "code-block-style", "code-fence-style", "link-destination-style"}},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			require.NotEmpty(t, reportedLines(t, tt.body, tt.ruleID, tt.enable...), "the body must make %s report", tt.ruleID)

			input := "<!-- markdownlint-disable " + tt.tag + " -->\n\n" + tt.body
			assert.Empty(t, reportedLines(t, input, tt.ruleID, tt.enable...), "markdownlint-disable %s must still silence %s", tt.tag, tt.ruleID)

			cfg := config.NewConfig()
			cfg.EnableRules = tt.enable
			result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
				context.Background(), "doc.md", []byte(input), cfg)
			require.NoError(t, err)
			assert.Equal(t, []lint.Notice{{
				Kind:      lint.NoticeDeprecatedTag,
				Line:      1,
				Directive: "markdownlint-disable",
				Name:      tt.tag,
				Rules:     tt.covers,
				Message: "markdownlint-disable: deprecated tag \"" + tt.tag + "\" (write its rules instead: " +
					strings.Join(tt.covers, ", ") + "); it will stop working in a later release",
			}}, result.Notices)
		})
	}
}

// formerTags are gomdlint's former tags, which markdownlint does not have.
var formerTags = []string{"gfm", "layout", "lists", "metadata", "style", "tables"}

// TestEngineFormerTagsAreTheBuiltInTagsOutsideTheCatalog ties the former
// tags to the built-in rules: they are the tags that the rules report
// themselves and the catalog does not know, and each gets a notice.
func TestEngineFormerTagsAreTheBuiltInTagsOutsideTheCatalog(t *testing.T) {
	known := make(map[string]bool)
	for _, word := range catalogWords() {
		known[strings.ToLower(word)] = true
	}
	outside := make(map[string]bool)
	for _, rule := range lint.DefaultRegistry.Rules() {
		for _, tag := range rule.Tags() {
			if !known[strings.ToLower(tag)] {
				outside[strings.ToLower(tag)] = true
			}
		}
	}
	assert.ElementsMatch(t, formerTags, slices.Collect(maps.Keys(outside)))

	for _, tag := range formerTags {
		result := lintDefault(t, "# Title\n\n<!-- markdownlint-disable "+tag+" -->\n")
		require.Len(t, result.Notices, 1, tag)
		assert.Equal(t, lint.NoticeDeprecatedTag, result.Notices[0].Kind, tag)
	}
}

// taggedRule reports line 3 of every file and tags itself with tags.
type taggedRule struct {
	lint.BaseRule
}

func (r *taggedRule) Apply(*lint.RuleContext) ([]lint.Diagnostic, error) {
	return []lint.Diagnostic{{RuleID: r.ID(), Message: "found", StartLine: 3, StartColumn: 1}}, nil
}

// TestEngineOnlyFormerTagsAreDeprecated covers words that resolve through
// the tags that rules report themselves but are not gomdlint's former tags:
// markdownlint's tags in a registry without catalog entries, and the tag of
// a rule outside the catalog. They name their rules, as before, and get no
// notice. A former tag gets one in any registry.
func TestEngineOnlyFormerTagsAreDeprecated(t *testing.T) {
	lintWith := func(t *testing.T, registry *lint.Registry, input string) *lint.FileResult {
		t.Helper()
		result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
			context.Background(), "doc.md", []byte(input), config.NewConfig())
		require.NoError(t, err)
		return result
	}

	t.Run("markdownlint tag without the catalog", func(t *testing.T) {
		registry := lint.NewRegistry()
		registry.Register(rules.NewTrailingWhitespaceRule())

		result := lintWith(t, registry, "<!-- markdownlint-disable whitespace -->\n\nbad \n")

		assert.Empty(t, result.Diagnostics, "whitespace still silences no-trailing-spaces")
		assert.Empty(t, result.Notices)
	})

	t.Run("former tag without the catalog", func(t *testing.T) {
		registry := lint.NewRegistry()
		registry.Register(rules.NewUnorderedListStyleRule())

		result := lintWith(t, registry, "<!-- markdownlint-disable style -->\n\n- one\n\n* two\n")

		assert.Empty(t, result.Diagnostics, "style still silences unordered-list-style")
		require.Len(t, result.Notices, 1)
		assert.Equal(t, lint.NoticeDeprecatedTag, result.Notices[0].Kind)
		assert.Equal(t, []string{"unordered-list-style"}, result.Notices[0].Rules,
			"without a catalog entry, a rule has only its own name")
	})

	t.Run("tag of a rule outside the catalog", func(t *testing.T) {
		registry := lint.NewRegistry()
		registry.Register(&taggedRule{BaseRule: lint.NewBaseRule("ACME001", "acme-rule", "", []string{"acme"}, false)})
		describeCatalog(registry)
		require.Len(t, lintWith(t, registry, "# Title\n\nText.\n").Diagnostics, 1)

		result := lintWith(t, registry, "<!-- markdownlint-disable acme -->\n\nText.\n")

		assert.Empty(t, result.Diagnostics, "acme still silences acme-rule")
		assert.Empty(t, result.Notices)
	})
}

// catalogWords returns every rule ID, name and alias and every tag of the
// built-in rules' catalog entries.
func catalogWords() []string {
	var words []string
	for _, meta := range rules.Catalog() {
		words = append(words, meta.ID, meta.Name)
		words = append(words, meta.Aliases...)
		words = append(words, meta.Tags...)
	}
	return words
}

// TestEngineKnownDirectiveNamesNeedNoNotice lints comments of every kind
// that name every rule identifier and tag that markdownlint and gomdlint
// share, in any case. None of them gets a notice, and neither do comments
// without words or the words that capture and restore ignore.
func TestEngineKnownDirectiveNamesNeedNoNotice(t *testing.T) {
	words := catalogWords()
	everyWord := strings.Join(words, " ") + " " + strings.ToUpper(strings.Join(words, " ")) + " " +
		strings.ToLower(strings.Join(words, " "))

	var input strings.Builder
	input.WriteString("# Title\n\n")
	for _, kind := range []string{"disable", "enable", "disable-line", "disable-next-line", "disable-file", "enable-file"} {
		input.WriteString("<!-- markdownlint-" + kind + " " + everyWord + " -->\n\n")
		input.WriteString("<!-- markdownlint-" + kind + " -->\n\n")
	}
	input.WriteString("<!-- markdownlint-capture no-trailling-spaces style -->\n\n")
	input.WriteString("<!-- markdownlint-restore no-trailling-spaces style -->\n\n")
	input.WriteString("<!-- markdownlint-configure-file {\"default\": true, \"MD009\": true, \"ul-style\": true, \"whitespace\": true} -->\n")

	result := lintDefault(t, input.String())

	assert.Empty(t, result.Notices)
}

// TestEngineDirectiveNoticesForEveryKind covers every kind of markdownlint
// comment that names rules. Each names the comment and its line, and the
// word changes nothing about what the comment does to the other rule it
// names. The configure-file comment rejects unknown keys as an error, so
// only its former tags get notices.
func TestEngineDirectiveNoticesForEveryKind(t *testing.T) {
	tests := []struct {
		directive, word string
		kind            lint.NoticeKind
	}{
		{"markdownlint-disable", "frobnicate", lint.NoticeUnknownRule},
		{"markdownlint-enable", "frobnicate", lint.NoticeUnknownRule},
		{"markdownlint-disable-line", "frobnicate", lint.NoticeUnknownRule},
		{"markdownlint-disable-next-line", "frobnicate", lint.NoticeUnknownRule},
		{"markdownlint-disable-file", "frobnicate", lint.NoticeUnknownRule},
		{"markdownlint-enable-file", "frobnicate", lint.NoticeUnknownRule},
		{"markdownlint-disable", "lists", lint.NoticeDeprecatedTag},
		{"markdownlint-enable", "lists", lint.NoticeDeprecatedTag},
		{"markdownlint-disable-line", "lists", lint.NoticeDeprecatedTag},
		{"markdownlint-disable-next-line", "lists", lint.NoticeDeprecatedTag},
		{"markdownlint-disable-file", "lists", lint.NoticeDeprecatedTag},
		{"markdownlint-enable-file", "lists", lint.NoticeDeprecatedTag},
	}
	// The comment on line 5 and lines 6 and 8 end with a space, which
	// no-trailing-spaces (MD009) reports unless the comment silences it.
	document := func(directive, words string) string {
		// Written in upper case, as markdownlint also accepts.
		return "# Title\n\nText.\n\n<!-- " + strings.ToUpper(directive) + " " + words + " --> \n" +
			"Trailing space \n\nMore trailing \n"
	}
	for _, tt := range tests {
		t.Run(tt.directive+" "+tt.word, func(t *testing.T) {
			result := lintDefault(t, document(tt.directive, "MD009 "+tt.word))

			require.Len(t, result.Notices, 1)
			notice := result.Notices[0]
			assert.Equal(t, tt.kind, notice.Kind)
			assert.Equal(t, 5, notice.Line)
			assert.Equal(t, tt.directive, notice.Directive, "the comment's kind in canonical case")
			assert.Equal(t, tt.word, notice.Name)
			assert.True(t, strings.HasPrefix(notice.Message, tt.directive+": "), notice.Message)

			want := reportedLines(t, document(tt.directive, "MD009"), "MD009")
			assert.Equal(t, want, reportedLines(t, document(tt.directive, "MD009 "+tt.word), "MD009"),
				"the word must not change what the comment does to MD009")
			if strings.Contains(tt.directive, "disable") {
				plain := reportedLines(t, document("plain-comment", "MD009"), "MD009")
				assert.NotEqual(t, want, plain, "the comment must silence MD009 somewhere")
			}
		})
	}

	t.Run("markdownlint-configure-file former tag", func(t *testing.T) {
		result := lintDefault(t, "# Title\n\n<!-- markdownlint-configure-file {\"MD009\": true, \"lists\": false} -->\n")

		assert.Equal(t, []lint.Notice{{
			Kind:      lint.NoticeDeprecatedTag,
			Line:      3,
			Directive: "markdownlint-configure-file",
			Name:      "lists",
			Rules:     []string{"ul-style"},
			Message: `markdownlint-configure-file: deprecated tag "lists" ` +
				`(write its rules instead: ul-style); it will stop working in a later release`,
		}}, result.Notices)
	})
}

// TestEngineDirectiveNoticesFollowTheComments covers the order and lines of
// notices: one per word as written, once per comment even when the word
// repeats in any case, in the order of the comments and their words, at the
// line where each comment starts.
func TestEngineDirectiveNoticesFollowTheComments(t *testing.T) {
	input := "# Title\n\n" +
		"Text <!-- markdownlint-disable-line frobnicate --> inline.\n\n" +
		"<!-- markdownlint-disable\n  MD009 wibble WIBBLE style wibble\n-->\n\n" +
		"<!-- markdownlint-disable first --><!-- markdownlint-enable second -->\n\n" +
		"<!-- markdownlint-configure-file {\"lists\": false} --><!-- markdownlint-disable frobnicate -->\n"

	result := lintDefault(t, input)

	type placed struct {
		directive string
		line      int
		name      string
	}
	var got []placed
	for _, notice := range result.Notices {
		got = append(got, placed{notice.Directive, notice.Line, notice.Name})
	}
	assert.Equal(t, []placed{
		{"markdownlint-disable-line", 3, "frobnicate"},
		{"markdownlint-disable", 5, "wibble"},
		{"markdownlint-disable", 5, "style"},
		{"markdownlint-disable", 9, "first"},
		{"markdownlint-enable", 9, "second"},
		{"markdownlint-configure-file", 11, "lists"},
		{"markdownlint-disable", 11, "frobnicate"},
	}, got)
}

// TestEngineNestedDirectiveNotices covers a comment whose parameter holds
// the start of another comment, as markdownlint reads it: the outer
// comment reads every word up to the shared "-->", the inner one only its
// own. Each word gets one notice, on the last comment before it that takes
// rules: a capture comment takes none, so the outer comment gets the
// notices for its words.
func TestEngineNestedDirectiveNotices(t *testing.T) {
	type placed struct {
		directive, name string
	}
	tests := []struct {
		name, comment string
		want          []placed
	}{
		{
			name:    "inner comment that takes rules",
			comment: "<!-- markdownlint-disable <!-- markdownlint-enable frobnicate -->",
			want: []placed{
				{"markdownlint-disable", "<!--"}, {"markdownlint-disable", "markdownlint-enable"},
				{"markdownlint-enable", "frobnicate"},
			},
		},
		{
			name:    "inner comment that takes none",
			comment: "<!-- markdownlint-disable <!-- markdownlint-capture MD009 frobnicate -->",
			want: []placed{
				{"markdownlint-disable", "<!--"}, {"markdownlint-disable", "markdownlint-capture"},
				{"markdownlint-disable", "frobnicate"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lintDefault(t, "# Title\n\n"+tt.comment+"\n")

			var got []placed
			for _, notice := range result.Notices {
				assert.Equal(t, 3, notice.Line)
				got = append(got, placed{notice.Directive, notice.Name})
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestEngineDirectiveNoticeSuggestions covers the suggestions that notices
// offer: the closest rules and tags, as unknown keys elsewhere get them, and
// none when too many are as close. A rule is offered by the name that
// markdownlint lists first for it, which markdownlint reads in the same
// comment, even when the word looks like gomdlint's own name for the rule.
func TestEngineDirectiveNoticeSuggestions(t *testing.T) {
	tests := []struct {
		word        string
		suggestions []string
		message     string
	}{
		{
			word: "whitespce", suggestions: []string{"whitespace"},
			message: `markdownlint-disable: unknown rule "whitespce" (did you mean the tag "whitespace"?); it is ignored`,
		},
		{
			// A mistyped ID is as close to too many rules to name any.
			word: "MD002", message: `markdownlint-disable: unknown rule "MD002"; it is ignored`,
		},
		{
			word: "ul-styel", suggestions: []string{"ul-style"},
			message: `markdownlint-disable: unknown rule "ul-styel" (did you mean "ul-style"?); it is ignored`,
		},
		{word: "unordered-list-styel", suggestions: []string{"ul-style"}},
		{word: "no-multiple-blank", suggestions: []string{"no-multiple-blanks"}},
		{word: "heading-blank-line", suggestions: []string{"blanks-around-headings"}},
		{word: "single-h11", suggestions: []string{"single-title"}},
		// first-line-heading is markdownlint's first name for MD041 too.
		{word: "first-line-headin", suggestions: []string{"first-line-heading"}},
		// markdownlint lacks gomdlint's own rules, so they keep their names.
		{word: "table-alignmnt", suggestions: []string{"table-alignment"}},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := lintDefault(t, "# Title\n\n<!-- markdownlint-disable "+tt.word+" -->\n")

			require.Len(t, result.Notices, 1)
			assert.Equal(t, tt.suggestions, result.Notices[0].Suggestions)
			if tt.message != "" {
				assert.Equal(t, tt.message, result.Notices[0].Message)
			}
		})
	}
}

// TestEngineConfigureFileSuggestsMarkdownlintNames covers the error for an
// unknown key of a configure-file comment, which offers the rule by
// markdownlint's name, as notices in markdownlint comments do.
func TestEngineConfigureFileSuggestsMarkdownlintNames(t *testing.T) {
	input := "<!-- markdownlint-configure-file {\"ul-styel\": false} -->\n- one\n"

	_, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
		context.Background(), "test.md", []byte(input), config.NewConfig())

	require.ErrorContains(t, err, `markdownlint-configure-file at line 1: unknown rule "ul-styel" (did you mean "ul-style"?)`)
}

// TestEngineDirectiveNoticesInPartialRegistries lints with a registry that
// holds the whole catalog but only one rule. Words that the catalog knows
// name rules that are not registered, which is no mistake, so they get no
// notice.
func TestEngineDirectiveNoticesInPartialRegistries(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	describeCatalog(registry)

	input := "<!-- markdownlint-disable MD010 no-hard-tabs hard_tab ul-style bullet frobnicate -->\n\nbad \n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "doc.md", []byte(input), config.NewConfig())
	require.NoError(t, err)

	require.Len(t, result.Notices, 1)
	assert.Equal(t, "frobnicate", result.Notices[0].Name)
}

// TestPipelineDirectiveNoticesDescribeTheFixedContent covers --fix: like
// the findings, notices describe the content the fixes leave, so their
// lines match the fixed file.
func TestPipelineDirectiveNoticesDescribeTheFixedContent(t *testing.T) {
	input := "# Title\n\n\n\n<!-- markdownlint-disable frobnicate -->\n"
	pipeline := lint.NewPipeline(lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry))
	opts := lint.DefaultPipelineOptions()

	linted, err := pipeline.ProcessContent(context.Background(), "doc.md", []byte(input), config.NewConfig(), opts)
	require.NoError(t, err)
	require.Len(t, linted.Notices, 1)
	assert.Equal(t, 5, linted.Notices[0].Line)

	cfg := config.NewConfig()
	cfg.Fix = true
	opts.Fix = true
	fixed, err := pipeline.ProcessContent(context.Background(), "doc.md", []byte(input), cfg, opts)
	require.NoError(t, err)
	require.Equal(t, "# Title\n\n<!-- markdownlint-disable frobnicate -->\n", string(fixed.ModifiedContent))
	require.Len(t, fixed.Notices, 1)
	assert.Equal(t, 3, fixed.Notices[0].Line)
}
