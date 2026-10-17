package rules

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestNoBareURLsRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "autolink URL",
			input:     "<https://example.com>\n",
			wantDiags: 0,
		},
		{
			name:      "bare URL",
			input:     "Visit https://example.com for info\n",
			wantDiags: 1,
		},
		{
			name:      "URL in code span",
			input:     "`https://example.com`\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no URLs",
			input:     "Just some text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

// TestNoBareURLsRule_AutolinkLiterals follows markdownlint MD034, which reports
// GFM autolink literals: trailing punctuation and unbalanced parentheses are
// not part of a URL, and URLs in link text, code, or inline HTML content are
// not bare.
func TestNoBareURLsRule_AutolinkLiterals(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  []position
		fixed string
		// gfmFixed is the fixed output with the GFM flavor when it differs:
		// the fix is withheld where the parser reads the literal differently.
		gfmFixed string
	}{
		{"trailing period", "Visit https://example.com/path.\n", []position{{1, 7}}, "Visit <https://example.com/path>.\n", ""},
		{"balanced parentheses", "See https://en.wikipedia.org/wiki/Foo_(bar) now\n", []position{{1, 5}},
			"See <https://en.wikipedia.org/wiki/Foo_(bar)> now\n", ""},
		{"inside parentheses", "Docs (see https://example.com/a) here\n", []position{{1, 11}},
			"Docs (see <https://example.com/a>) here\n", ""},
		{"unbalanced parenthesis before punctuation", "Docs (see https://example.com/a). here\n", []position{{1, 11}},
			"Docs (see <https://example.com/a>). here\n", "Docs (see https://example.com/a). here\n"},
		{"backslash in the path", "See https://example.com/a\\b here\n", []position{{1, 5}},
			"See https://example.com/a\\b here\n", ""},
		{"quote that ends the literal", "See https://example.com/\"q\" now\n", []position{{1, 5}},
			"See https://example.com/\"q\" now\n", ""},
		{"path that runs into emphasis", "See https://example.com/a*b* now\n", []position{{1, 5}},
			"See https://example.com/a*b* now\n", ""},
		{"address after mailto", "Write mailto:user@example.com now\n", []position{{1, 14}},
			"Write mailto:user@example.com now\n", ""},
		{"digit in the last domain label of an email", "Mail user@example.co1 now\n", nil, "", ""},
		{"underscore in the last two domain labels", "See https://example.a_b.com/a now\n", nil, "", ""},
		// GitHub renders with cmark-gfm, which finds emails in decoded text
		// and scans a URL to ASCII whitespace, so a literal keeps its fix only
		// where its neighbours are plain ASCII.
		{"character reference after an email", "Mail user@example.com&#97; now\n", []position{{1, 6}},
			"Mail user@example.com&#97; now\n", ""},
		{"hexadecimal character reference after an email", "Mail user@example.com&#x61; now\n", []position{{1, 6}},
			"Mail user@example.com&#x61; now\n", ""},
		{"named character reference after an email", "Mail user@example.com&amp; now\n", []position{{1, 6}},
			"Mail user@example.com&amp; now\n", ""},
		{"less-than reference after an email", "Mail user@example.com&lt; now\n", []position{{1, 6}},
			"Mail user@example.com&lt; now\n", ""},
		{"scheme colon written as a character reference", "Write xmpp&#58;user@example.com now\n", []position{{1, 16}},
			"Write xmpp&#58;user@example.com now\n", ""},
		{"no-break space in the path", "See https://example.com/a\u00a0b here\n", []position{{1, 5}},
			"See https://example.com/a\u00a0b here\n", ""},
		{"em space in the path", "See https://example.com/a\u2003b here\n", []position{{1, 5}},
			"See https://example.com/a\u2003b here\n", ""},
		{"ideographic space in the path", "See https://example.com/a\u3000b here\n", []position{{1, 5}},
			"See https://example.com/a\u3000b here\n", ""},
		{"next line character in the path", "See https://example.com/a\u0085b here\n", []position{{1, 5}},
			"See https://example.com/a\u0085b here\n", ""},
		{"byte order mark in the path", "See https://example.com/a\ufeffb here\n", []position{{1, 5}},
			"See https://example.com/a\ufeffb here\n", ""},
		{"no-break space before the url", "See\u00a0https://example.com here\n", []position{{1, 6}},
			"See\u00a0https://example.com here\n", ""},
		{"accented letter after an email", "Mail user@example.com\u00e9 now\n", []position{{1, 6}},
			"Mail user@example.com\u00e9 now\n", ""},
		{"closing bracket after the url", "See https://example.com/a] here\n", []position{{1, 5}},
			"See https://example.com/a] here\n", ""},
		{"underscore after a url without a path", "See https://example.com_ here\n", []position{{1, 5}},
			"See https://example.com_ here\n", ""},
		{"asterisk that would open emphasis after the fix", "See https://example.com*. and more*\n", []position{{1, 5}},
			"See https://example.com*. and more*\n", ""},
		{"email that starts with www", "Mail www.foo@example.com now\n", []position{{1, 6}},
			"Mail www.foo@example.com now\n", ""},
		{"email with an underscore after a period", "Mail foo._bar@example.com now\n", []position{{1, 6}},
			"Mail foo._bar@example.com now\n", ""},
		{"email in parentheses", "Mail (user@example.com) now\n", []position{{1, 7}}, "Mail (<user@example.com>) now\n", ""},
		{"url after a parenthesis at the line start", "(https://example.com/a) here\n", []position{{1, 2}},
			"(<https://example.com/a>) here\n", ""},
		{"blank line between the url and a table", "See https://example.com here\n\n| a | b |\n| - | - |\n",
			[]position{{1, 5}}, "See <https://example.com> here\n\n| a | b |\n| - | - |\n", ""},
		{"email with trailing period", "Mail user@example.com.\n", []position{{1, 6}}, "Mail <user@example.com>.\n", ""},
		{"entity after url", "Go to https://example.com/q?a=1&amp; now\n", []position{{1, 7}},
			"Go to https://example.com/q?a=1&amp; now\n", ""},
		{"inside emphasis", "Read *https://example.com* now\n", []position{{1, 7}}, "Read *https://example.com* now\n", ""},
		{"inside strikethrough", "Old ~~see https://example.com~~ now\n", []position{{1, 11}}, "Old ~~see <https://example.com>~~ now\n", ""},
		{"blockquote", "> Visit https://example.com\n", []position{{1, 9}}, "> Visit <https://example.com>\n", ""},
		{"multibyte text before url", "中文中文 https://example.com\n", []position{{1, 14}}, "中文中文 <https://example.com>\n", ""},
		{"url in link text", "[https://example.com](https://example.com)\n", nil, "", ""},
		{"url in brackets", "[https://example.com]\n", nil, "", ""},
		{"url after unclosed bracket", "[see https://example.com and more\n", nil, "", ""},
		{"url after closed bracket", "[note] https://example.com\n", []position{{1, 8}}, "[note] <https://example.com>\n", ""},
		{"url in code span", "Use `https://example.com` here\n", nil, "", ""},
		{"url in inline html content", "Use <span>https://example.com</span> here\n", nil, "", ""},
		{"url in html attribute", "Use <a href=\"https://example.com\">x</a> here\n", nil, "", ""},
		{"url in html block", "<div>\nhttps://example.com\n</div>\n", nil, "", ""},
		{"letter before protocol", "xhttps://example.com\n", nil, "", ""},
		{"autolink", "<https://example.com>\n", nil, "", ""},
		{"reference definition", "[ref]: https://example.com\n\n[ref]\n", nil, "", ""},
	}

	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		for _, tt := range tests {
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(tt.input))
				require.NoError(t, err)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
				require.NoError(t, err)
				require.Len(t, diags, len(tt.want), "diagnostics: %+v", diags)
				var edits []fix.TextEdit
				for idx, want := range tt.want {
					assert.Equal(t, want.line, diags[idx].StartLine, "diagnostic %d line", idx)
					assert.Equal(t, want.column, diags[idx].StartColumn, "diagnostic %d column", idx)
					edits = append(edits, diags[idx].FixEdits...)
				}
				if len(tt.want) == 0 {
					return
				}
				prepared, err := fix.PrepareEdits(edits, len(tt.input))
				require.NoError(t, err)
				want := tt.fixed
				if flavor == config.FlavorGFM && tt.gfmFixed != "" {
					want = tt.gfmFixed
				}
				assert.Equal(t, want, string(fix.ApplyEdits([]byte(tt.input), prepared)))
			})
		}
	}
}

// TestNoBareURLsRule_FixesAgreeWithTheParser runs MD034 over generated autolink
// literal shapes in several contexts, with both flavors. Every finding must be
// a GFM autolink literal. Every fix must meet the conditions under which
// GitHub, markdownlint, and the parser read the literal alike (see
// fixEligible), checked here on their own: the parser read the literal as
// plain text or as one autolink with exactly its span, the model of
// cmark-gfm links exactly the literal to the same address and nothing else
// changes with the fix, and the parser's autolink after the fix has the
// literal's address.
func TestNoBareURLsRule_FixesAgreeWithTheParser(t *testing.T) {
	shapes := []string{
		`https://example.com/a\b`, `https://example.com/a\)b`, `https://example.com/a\_b`,
		`https://example.com/a&amp;`, `https://example.com/a&amp;b`, `https://example.com/&copy;`,
		`https://example.com/a&b;c`, `https://example.com/a&;`, `https://example.com/a&#38;`,
		`https://example.com/(a(b)c)`, `https://example.com/a))`, `https://example.com/a(b))`,
		`https://example.com/a).`, `https://example.com/((a)`, `https://example.com/a)b)`, `https://example.com/((a).)`,
		`https://example.com/a?!.,`, `https://example.com/a...`, `https://example.com/a:;`, `https://example.com/a_~`,
		`https://example.com/a*`, `https://example.com*.`, `https://example.com_`, `https://example.com/a]`,
		`https://example.a_b.com/a`, `https://a_b.example.com/a`, `www.a_b.example.com`, `www.example.a_b.com`,
		`user@example.co1`, `https://example.co1/a`, `user@example.1co`,
		`www.example.com`, `www.example.com/a.`, `WWW.EXAMPLE.COM`,
		`http://example.com`, `https://example.com`, `HTTPS://EXAMPLE.COM/A`, `https://`, `https://.`,
		`mailto:user@example.com`, `xmpp:user@example.com/res`, `user@example.com`, `(user@example.com)`,
		`user.name+tag@sub.example.org.`, `user@example.com_`, `user@exa_mple.com`, `user@example.c_m`,
		`user@example.com**`, `user@example.com@x.yz`, `www.foo@example.com`, `foo_www.bar@example.com`,
		`foo._bar@example.com`, `first_last@example.com`,
		`user@example.com&#97;`, `user@example.com&#x61;`, `user@example.com&amp;`, `user@example.com&lt;`,
		`xmpp&#58;user@example.com`,
		"https://example.com/a\u00a0b", "https://example.com/a\u2003b", "https://example.com/a\u3000b",
		"https://example.com/a\u0085b", "https://example.com/a\ufeffb",
		"user@example.com\u00e9", "https://example.com/a\u00e9",
		`https://example.com/a<b`, `https://example.com/<x>`,
		`https://example.com/"q"`, `'https://example.com'`, `"https://example.com"`,
		`https://example.com/a*b*`, "https://example.com/a`b`",
	}
	layouts := []string{
		"See %s here\n", "%s\n", "> Quote %s here\n", "- Item %s here\n", "# Heading %s\n",
		"| %s | b |\n| - | - |\n", "Read *%s* now\n", "Docs (%s) here\n", "Say \"%s\" now\n",
		mdast.ByteOrderMark + "%s here\n", "See\u00a0%s here\n", "(%s) here\n", ">%s here\n",
		"|%s|b|\n|-|-|\n", "See %s\u00e9 here\n",
		"|x|%s |\n|-|-|-|\n", "| %s | b |\n|-|-|-|\n", "> | a | b |\n> |---|---|\n|%s|x |\n",
		"See %s here\n:--\n", "Intro %s\n|a|b|\n|-|-|-|\n",
		"> See %s here\n>\n> | a | b |\n> | - | - |\n", "See %s here\n    >\n|a|b|\n|-|-|-|\n",
	}
	fixes := 0
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		for _, layout := range layouts {
			for _, shape := range shapes {
				fixes += checkBareURLFixes(t, flavor, fmt.Sprintf(layout, shape))
			}
		}
	}
	// The sweep must exercise the checks of attached fixes, not only withhold.
	assert.Positive(t, fixes)
}

// checkBareURLFixes checks MD034's findings and fixes on one document and
// returns how many fixes it attached.
func checkBareURLFixes(t *testing.T, flavor config.Flavor, doc string) int {
	t.Helper()
	parse := func(source string) *mdast.FileSnapshot {
		snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(source))
		require.NoError(t, err)
		return snapshot
	}
	snapshot := parse(doc)
	cfg := config.NewConfig()
	cfg.Flavor = flavor
	diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
	require.NoError(t, err)
	textStart := 0
	if strings.HasPrefix(doc, mdast.ByteOrderMark) {
		textStart = len(mdast.ByteOrderMark)
	}
	fixes := 0
	for _, diag := range diags {
		start := snapshot.Lines[diag.StartLine-1].StartOffset + diag.StartColumn - 1
		end := snapshot.Lines[diag.EndLine-1].StartOffset + diag.EndColumn - 1
		label := fmt.Sprintf("%s %q", flavor, doc)
		contentStart, contentEnd := inlineContentAround(snapshot.Root, start, end)
		require.Less(t, contentStart, contentEnd, "%s: no inline content around %d", label, start)
		// Nothing comes before a literal at the start of the inline content.
		var prev byte
		if start > textStart && start != contentStart {
			prev = doc[start-1]
		}
		literalEnd, kind := mdscan.MatchAutolinkLiteral([]byte(doc[:contentEnd]), start, prev)
		require.NotEqual(t, mdscan.LiteralNone, kind, "%s: finding at %d is no GFM literal", label, start)
		require.Equal(t, literalEnd, end, "%s: finding is not the whole GFM literal", label)
		if !diag.HasFix() {
			continue
		}
		fixes++
		literal := doc[start:end]
		require.True(t, readAsText(snapshot.Root, start, end),
			"%s: fix for %q, which the parser reads differently", label, literal)
		checkFixConditions(t, label, doc, contentStart, start, end, kind)
		content := []byte(doc[contentStart:contentEnd])
		checkGitHubKeepsLinks(t, label, content, start-contentStart, end-contentStart, kind)
		require.Len(t, diag.FixEdits, 1)
		edit := diag.FixEdits[0]
		require.Equal(t, [2]int{start, end}, [2]int{edit.StartOffset, edit.EndOffset}, "%s: fix range", label)
		require.Equal(t, "<"+literal+">", edit.NewText)
		// The parser records an email autolink's address without "mailto:".
		fixed := parse(doc[:start] + edit.NewText + doc[end:])
		require.Equal(t, literal, autolinkDestinationAt(fixed.Root, start, end+2),
			"%s: the fix changes the address", label)
	}
	return fixes
}

// inlineContentAround returns the span of the inline content of the
// innermost paragraph, heading, or table cell that holds source[start:end].
func inlineContentAround(root *mdast.Node, start, end int) (int, int) {
	from, to := 0, 0
	_ = mdast.Walk(root, func(node *mdast.Node) error { //nolint:errcheck // visitor never returns error
		if !isTextContainer(node) {
			return nil
		}
		first, last := node.FirstChild.SourceRange().StartOffset, node.LastChild.SourceRange().EndOffset
		if first <= start && end <= last && first >= from {
			from, to = first, last
		}
		return nil
	})
	return from, to
}

// checkFixConditions checks the conditions under which MD034 attaches a fix,
// on their own, for the literal doc[ls:le] on a line whose inline content
// starts at doc[lineStart].
func checkFixConditions(t *testing.T, label, doc string, lineStart, ls, le int, kind mdscan.LiteralKind) {
	t.Helper()
	content := []byte(doc)
	literal := content[ls:le]
	for _, ch := range literal {
		require.True(t, ch > ' ' && ch < 0x7f && !strings.ContainsRune("&\\<>\"'`", rune(ch)),
			"%s: the literal holds %q", label, ch)
	}
	blank := func(pos int) bool {
		return strings.IndexByte(" \t\r\n", content[pos]) >= 0
	}
	runStart := ls
	if runStart > lineStart && content[runStart-1] == '(' {
		runStart--
	}
	require.True(t, runStart == lineStart || blank(runStart-1), "%s: text before the literal", label)
	stop := le
	for stop < len(content) && strings.IndexByte(".,:;!?*_~\"')", content[stop]) >= 0 {
		stop++
	}
	require.True(t, stop == len(content) || blank(stop), "%s: text after the literal", label)
	trail := content[le:stop]
	if bytes.IndexByte(trail, ')') >= 0 {
		require.LessOrEqual(t, bytes.Count(literal, []byte("(")), bytes.Count(literal, []byte(")")),
			"%s: a closing parenthesis after an unbalanced one", label)
	}
	if len(trail) > 0 && strings.IndexByte("*_~", trail[0]) >= 0 {
		require.Equal(t, len(trail), bytes.Count(trail, trail[:1]), "%s: a delimiter that could open", label)
	}
	// No line of the run around the literal, up to a line of nothing but
	// spaces and tabs, holds a pipe or could be a delimiter row, so no
	// renderer reads a table there.
	lines := strings.Split(doc, "\n")
	first := strings.Count(doc[:ls], "\n")
	last := first
	isBreak := func(line string) bool { return strings.Trim(line, " \t\r") == "" }
	for first > 0 && !isBreak(lines[first-1]) {
		first--
	}
	for last+1 < len(lines) && !isBreak(lines[last+1]) {
		last++
	}
	for _, line := range lines[first : last+1] {
		rest := strings.TrimLeft(strings.TrimPrefix(line, mdast.ByteOrderMark), " \t>")
		require.NotContains(t, line, "|", "%s: a pipe in the run of lines", label)
		require.False(t, strings.Contains(rest, "-") && strings.Trim(rest, "-:| \t\v\f\r") == "",
			"%s: a line that could be a delimiter row", label)
	}
	if kind != mdscan.LiteralEmail {
		return
	}
	lower := strings.ToLower(string(literal))
	require.False(t, strings.HasPrefix(lower, "www.") || strings.Contains(lower, "_www."), "%s: a www address", label)
	for idx, ch := range literal {
		if ch != '_' {
			continue
		}
		before, after := idx, idx
		for before > 0 && literal[before-1] == '_' {
			before--
		}
		for after < len(literal) && literal[after] == '_' {
			after++
		}
		require.True(t, before > 0 && after < len(literal) &&
			cmarkIsAlnum(literal[before-1]) && cmarkIsAlnum(literal[after]),
			"%s: an underscore that could open or close emphasis", label)
	}
}

// checkGitHubKeepsLinks checks with the model of cmark-gfm that GitHub links
// exactly the literal content[ls:le] to its address before the fix, and that
// wrapping it in angle brackets changes no other link.
func checkGitHubKeepsLinks(t *testing.T, label string, content []byte, ls, le int, kind mdscan.LiteralKind) {
	t.Helper()
	literal := string(content[ls:le])
	want := literal
	if kind == mdscan.LiteralEmail {
		want = "mailto:" + literal
	}
	var others []cmarkLink
	found := false
	for _, link := range cmarkAutolinks(content) {
		switch {
		case link.end <= ls || link.start >= le:
			if link.start >= le {
				link.start, link.end = link.start+2, link.end+2
			}
			others = append(others, link)
		case link.start == ls && link.end == le && link.url == want:
			found = true
		default:
			require.Failf(t, "GitHub reads the literal differently",
				"%s: GitHub links %q to %q", label, content[link.start:link.end], link.url)
		}
	}
	require.True(t, found, "%s: GitHub does not link %q", label, literal)
	fixed := make([]byte, 0, len(content)+2)
	fixed = append(fixed, content[:ls]...)
	fixed = append(fixed, "<"+literal+">"...)
	fixed = append(fixed, content[le:]...)
	require.Equal(t, others, cmarkAutolinks(fixed), "%s: the fix changes what GitHub links", label)
}

// readAsText reports whether the parser read source[start:end] as plain text
// or as one autolink with exactly that span: every inline node overlapping
// the range is text, that autolink, or a container holding the whole range.
func readAsText(root *mdast.Node, start, end int) bool {
	plain := true
	_ = mdast.Walk(root, func(node *mdast.Node) error { //nolint:errcheck // visitor never returns error
		span := node.SourceRange()
		if !node.IsInline() || span.EndOffset <= start || span.StartOffset >= end {
			return nil
		}
		switch {
		case node.Kind == mdast.NodeText:
		case isAutolinkNode(node) && span.StartOffset == start && span.EndOffset == end:
		case node.Kind != mdast.NodeLink && span.StartOffset <= start && span.EndOffset >= end:
		default:
			plain = false
		}
		return nil
	})
	return plain
}

// autolinkDestinationAt returns the destination of the autolink whose span is
// source[start:end], or "" when there is none.
func autolinkDestinationAt(root *mdast.Node, start, end int) string {
	destination := ""
	_ = mdast.Walk(root, func(node *mdast.Node) error { //nolint:errcheck // visitor never returns error
		if span := node.SourceRange(); isAutolinkNode(node) && span.StartOffset == start && span.EndOffset == end {
			destination = node.Inline.Link.Destination
		}
		return nil
	})
	return destination
}

// TestNoBareURLsRule_WWWLiterals reports GFM "www." autolink literals like
// markdownlint, without a fix: wrapping one in angle brackets does not make
// an autolink (it has no scheme) and would show the brackets.
func TestNoBareURLsRule_WWWLiterals(t *testing.T) {
	input := "Visit www.example.com/path. today\n\n" +
		"See (www.example.com) and *www.example.org* and xwww.example.net here\n\n" +
		"[www.example.com/shortcut]\n"
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run(string(flavor), func(t *testing.T) {
			snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
			require.NoError(t, err)
			require.Len(t, diags, 3, "diagnostics: %+v", diags)
			for idx, want := range [][2]int{{1, 7}, {3, 6}, {3, 28}} {
				assert.Equal(t, want[0], diags[idx].StartLine, "diagnostic %d line", idx)
				assert.Equal(t, want[1], diags[idx].StartColumn, "diagnostic %d column", idx)
				assert.False(t, diags[idx].HasFix(), "diagnostic %d must not have a fix", idx)
			}
			assert.Equal(t, 1+len("www.example.com/path"), diags[0].EndColumn-diags[0].StartColumn+1)
		})
	}
}

// TestNoBareURLsRule_Tables covers literals in and next to text that a parser
// may read as a table. GitHub splits cells only where it reads a table, which
// needs a delimiter row right after the header row, with as many cells, in
// the same paragraph; goldmark accepts more. So MD034 attaches no fix in a
// table or in a run of lines that holds a pipe or a line that could be a
// delimiter row.
//
// Detection follows goldmark's blocks, as every rule does: with GFM, MD034
// reads the cells of every table goldmark reads. Where goldmark reads a table
// that micromark refuses (a header row with fewer cells than the delimiter
// row) or takes a lazy row into a table, markdownlint reads those lines as a
// paragraph, so the rows named "known difference" pin gomdlint's result
// against markdownlint's. With CommonMark there are no tables, so the lines
// are paragraph text, where markdownlint, which always reads GFM tables, can
// differ too.
func TestNoBareURLsRule_Tables(t *testing.T) {
	type position struct{ line, column, length int }
	tests := []struct {
		name       string
		input      string
		gfm        []position
		commonMark []position
	}{
		// markdownlint: 1:2 length 27, the lines as one paragraph.
		{"known difference: goldmark reads a table whose header row has fewer cells",
			"|https://a.com|https://b.com |\n|-|-|-|\n",
			[]position{{1, 2, 13}, {1, 16, 13}}, []position{{1, 2, 27}}},
		{"literal across the cells of a table", "| https://a.com|https://b.com |\n|-|-|\n",
			[]position{{1, 3, 13}, {1, 17, 13}}, []position{{1, 3, 27}}},
		// markdownlint: 3:2 length 27, the lazy row as a paragraph after the blockquote.
		{"known difference: goldmark takes a lazy row into a table in a blockquote",
			"> | a | b |\n> |---|---|\n|https://a.com|https://b.com |\n",
			[]position{{3, 2, 13}, {3, 16, 13}}, []position{{3, 2, 27}}},
		{"padded cell of a table", "| https://example.com | b |\n| - | - |\n",
			[]position{{1, 3, 19}}, []position{{1, 3, 19}}},
		{"line before a delimiter-like line", "See https://example.com here\n:--\n",
			[]position{{1, 5, 19}}, []position{{1, 5, 19}}},
		{"paragraph line before a header line", "Intro https://example.com\nheader\n|-|\n",
			[]position{{1, 7, 19}}, []position{{1, 7, 19}}},
		{"paragraph before a table that GFM refuses", "Intro https://example.com\n|a|b|\n|-|-|-|\n",
			[]position{{1, 7, 19}}, []position{{1, 7, 19}}},
		// goldmark's table parse makes the "www." literal start a cell, where
		// micromark, which refuses the table, reads it after a pipe and
		// markdownlint reports nothing.
		{"known difference: a www literal starts a cell of a table that GFM refuses",
			"|x|www.example.com |\n|-|-|-|\n", []position{{1, 4, 15}}, nil},
		// markdownlint: 1:6 length 15 only, the lazy row as a paragraph.
		{"known difference: a www literal starts a cell of a lazy row",
			"> |x|www.example.com|\n> |-|-|\n|y|www.example.org|\n",
			[]position{{1, 6, 15}, {3, 4, 15}}, nil},
		// markdownlint reports the same three as gomdlint with GFM.
		{"valid table after a table that GFM refuses", "|a|b|\n|-|-|-|\n|x|www.example.com|\n|-|-|\n",
			[]position{{3, 4, 15}}, nil},
		{"code span that a lazy row opens", "> |a|b|\n> |-|-|\n`open\n> https://example.com |b|`\n",
			[]position{{4, 3, 19}}, nil},
		{"tabbed list item in a blockquote", "> -\t|a|b|\n>\t|-|-|\n>\t|x|www.example.com|\n",
			[]position{{3, 6, 15}}, nil},
		// Only a blank line ends a run. An indented ">" is paragraph text on
		// GitHub, where the code span runs on to the third line and holds the
		// URL; goldmark, which reads the last two lines as a table, leaves the
		// backtick open, so the URL is a known difference with GFM.
		{"code span over an indented > line", "`open https://example.com\n    >\n|a|b`|\n|-|-|-|\n",
			[]position{{1, 7, 19}}, nil},
		{"code span over a tab-indented > line", "`open https://example.com\n\t>\n|a|b`|\n|-|-|-|\n",
			[]position{{1, 7, 19}}, nil},
		{"paragraph that a > line separates from a table in a blockquote",
			"> See https://example.com here\n>\n> | a | b |\n> | - | - |\n",
			[]position{{1, 7, 19}}, []position{{1, 7, 19}}},
		{"paragraph that an unindented > line separates from a table",
			"See https://example.com here\n>\n| a | b |\n| - | - |\n",
			[]position{{1, 5, 19}}, []position{{1, 5, 19}}},
	}
	for _, tt := range tests {
		for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				diags, err := NewNoBareURLsRule().Apply(fixSafetyContext(t, flavor, tt.input))
				require.NoError(t, err)
				want := tt.gfm
				if flavor == config.FlavorCommonMark {
					want = tt.commonMark
				}
				got := make([]position, 0, len(diags))
				for _, diag := range diags {
					got = append(got, position{diag.StartLine, diag.StartColumn, diag.EndColumn - diag.StartColumn})
					assert.False(t, diag.HasFix(), "fix at %d:%d", diag.StartLine, diag.StartColumn)
				}
				if want == nil {
					want = []position{}
				}
				assert.Equal(t, want, got)
			})
		}
	}
}

// TestNoBareURLsRule_LiteralAtLineStart reports a "www." literal that starts
// a line's inline content right after a container marker, such as ">" with
// no space or a table cell's pipe. micromark reads the start of a line as
// the start of the text there, where a "www." literal may begin, whatever
// the source holds before it.
func TestNoBareURLsRule_LiteralAtLineStart(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name    string
		input   string
		want    []position
		flavors []config.Flavor
	}{
		{"blockquote without a space", ">www.example.com/a here\n", []position{{1, 2}},
			[]config.Flavor{config.FlavorCommonMark, config.FlavorGFM}},
		{"blockquote continuation line", "> text\n>www.example.com/a here\n", []position{{2, 2}},
			[]config.Flavor{config.FlavorCommonMark, config.FlavorGFM}},
		{"table cells without padding", "|www.example.com|b|\n|-|-|\n|c|www.example.org|\n", []position{{1, 2}, {3, 4}},
			[]config.Flavor{config.FlavorGFM}},
		{"after a byte order mark", mdast.ByteOrderMark + "www.example.com/a here\n", []position{{1, 4}},
			[]config.Flavor{config.FlavorCommonMark, config.FlavorGFM}},
	}
	for _, tt := range tests {
		for _, flavor := range tt.flavors {
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(tt.input))
				require.NoError(t, err)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
				require.NoError(t, err)
				got := make([]position, 0, len(diags))
				for _, diag := range diags {
					got = append(got, position{diag.StartLine, diag.StartColumn})
				}
				assert.Equal(t, tt.want, got)
			})
		}
	}
}

func TestNoBareURLsRule_Metadata(t *testing.T) {
	rule := NewNoBareURLsRule()

	assert.Equal(t, "MD034", rule.ID())
	assert.Equal(t, "no-bare-urls", rule.Name())
	assert.Contains(t, rule.Tags(), "links")
	assert.True(t, rule.CanFix())
}

func TestNoBareURLsRule_EmailEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		// Test various email patterns that might cause issues
		{
			name:      "email at start of line",
			input:     "user@example.com is my email\n",
			wantDiags: 1,
			wantFix:   "<user@example.com> is my email\n",
		},
		{
			name:      "email at end of line",
			input:     "Contact user@example.com\n",
			wantDiags: 1,
			wantFix:   "Contact <user@example.com>\n",
		},
		{
			name:      "email alone on line",
			input:     "user@example.com\n",
			wantDiags: 1,
			wantFix:   "<user@example.com>\n",
		},
		{
			name:      "multiple emails on same line",
			input:     "Contact alice@test.com or bob@test.com\n",
			wantDiags: 2,
			wantFix:   "Contact <alice@test.com> or <bob@test.com>\n",
		},
		{
			name:      "email with complex domain",
			input:     "Send to user@sub.domain.example.com please\n",
			wantDiags: 1,
			wantFix:   "Send to <user@sub.domain.example.com> please\n",
		},
		{
			name:      "email with plus addressing",
			input:     "Send to user+tag@example.com please\n",
			wantDiags: 1,
			wantFix:   "Send to <user+tag@example.com> please\n",
		},
		{
			name:      "already wrapped email - should not match",
			input:     "Contact <user@example.com> for help\n",
			wantDiags: 0,
			wantFix:   "Contact <user@example.com> for help\n",
		},
		{
			name:      "email next to URL on same line",
			input:     "Visit https://example.com or email admin@example.com\n",
			wantDiags: 2,
			wantFix:   "Visit <https://example.com> or email <admin@example.com>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))

			// CRITICAL: Verify idempotency — this is what would catch the infinite loop
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent — no re-detection after wrapping")
		})
	}
}

func TestNoBareURLsRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - already wrapped URL",
			input:     "Visit <https://example.com> for info\n",
			wantDiags: 0,
			wantFix:   "Visit <https://example.com> for info\n",
		},
		{
			name:      "single bare URL",
			input:     "Visit https://example.com for info\n",
			wantDiags: 1,
			wantFix:   "Visit <https://example.com> for info\n",
		},
		{
			name:      "multiple bare URLs same line",
			input:     "Check https://example.com and https://test.org today\n",
			wantDiags: 2,
			wantFix:   "Check <https://example.com> and <https://test.org> today\n",
		},
		{
			name:      "multiple bare URLs different lines",
			input:     "First https://example.com here\nSecond https://test.org there\n",
			wantDiags: 2,
			wantFix:   "First <https://example.com> here\nSecond <https://test.org> there\n",
		},
		{
			name:      "bare email",
			input:     "Contact a@b.co for help\n",
			wantDiags: 1,
			wantFix:   "Contact <a@b.co> for help\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "URL in code span - should be skipped",
			input:     "Use `https://example.com` in code\n",
			wantDiags: 0,
			wantFix:   "Use `https://example.com` in code\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))

			// Verify idempotency (always)
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent")
		})
	}
}

// TestNoBareURLsRule_WithholdsUnsafeFixes reports literals that angle brackets
// would not turn into the same autolink, without a fix: a URL holding '<',
// '>', or a backslash, and an email address whose domain an autolink does not
// accept.
func TestNoBareURLsRule_WithholdsUnsafeFixes(t *testing.T) {
	for _, input := range []string{
		"See https://example.com/a>b now.\n",
		"See \\[https://example.com\\] now.\n",
		"Mail foo@a_b.com now.\n",
	} {
		for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
			t.Run(string(flavor)+" "+input, func(t *testing.T) {
				snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
				require.NoError(t, err)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
				require.NoError(t, err)
				require.Len(t, diags, 1, "the literal is still reported: %+v", diags)
				assert.False(t, diags[0].HasFix(), "angle brackets would not make the same autolink")
			})
		}
	}
}

// TestNoBareURLsRule_ByteOrderMark treats the start of a file that begins
// with a byte order mark as the start of its text, as the parser does, so a
// "www." literal there is still reported.
func TestNoBareURLsRule_ByteOrderMark(t *testing.T) {
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run(string(flavor), func(t *testing.T) {
			input := "\xEF\xBB\xBFwww.example.com and https://example.com\n"
			snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
			require.NoError(t, err)
			require.Len(t, diags, 2, "diagnostics: %+v", diags)
			assert.Equal(t, 4, diags[0].StartColumn, "the www literal starts after the mark")
		})
	}
}

// TestNoBareURLsRule_EntityInURL withholds the fix when the URL holds an
// entity reference: as text the entity is decoded ("a©b"), but an
// autolink shows and links to it as written. GitHub decodes it in a GFM
// literal too, so the fix is withheld under both flavors.
func TestNoBareURLsRule_EntityInURL(t *testing.T) {
	input := "See https://example.com/a&copy;b now.\n"
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run(string(flavor), func(t *testing.T) {
			snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
			require.NoError(t, err)
			require.Len(t, diags, 1)
			assert.False(t, diags[0].HasFix(), "angle brackets would change the shown text and the destination")
		})
	}
}
