package rules

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestFixCheckKeepsNoFixBesideFootnotesWithTags checks documents that hold
// a "[^" and a "<" that may start a tag. The check's parsers do not read
// footnotes: they read "[^1]: <code>" as a link reference definition and an
// indented footnote paragraph as code, and see no raw HTML there, while
// GitHub writes a footnote's raw HTML after the document, where the browser
// opens the element again for the footnotes after it. goldmark and
// cmark-gfm, which GitHub uses, also disagree on which footnote definitions
// there are and where they end. So such a document keeps no fix, wherever
// the "[^" and the "<" are, even when all its raw HTML is benign.
func TestFixCheckKeepsNoFixBesideFootnotesWithTags(t *testing.T) {
	const later = "\n\n[^b]: alpha beta gamma\n"
	cases := []fixCheckCase{
		{name: "footnote after a footnote with raw HTML", input: "See [^1] and [^b].\n\n[^1]: <code>" + later},
		{
			name:   "footnote after a footnote with raw HTML in CommonMark",
			flavor: config.FlavorCommonMark,
			input:  "See [^1] and [^b].\n\n[^1]: <code>" + later,
		},
		{name: "paragraph after a footnote with raw HTML", input: "See [^1].\n\n[^1]: <code>\n\nalpha beta gamma\n"},
		{
			name:  "footnote after a footnote whose code block holds raw HTML",
			input: "See [^1] and [^b].\n\n[^1]: First.\n\n    Second <code>open" + later,
		},
		{
			// GitHub folds the case of footnote labels, as goldmark does not.
			name:  "footnote after a footnote with raw HTML whose label differs in case",
			input: "See [^a] and [^b].\n\n[^A]: <code>" + later,
		},
		{name: "paragraph after an unreferenced footnote with raw HTML", input: "Text.\n\n[^1]: <code>\n\nalpha beta gamma\n"},
		{name: "footnote beside a footnote with benign HTML", input: "See [^1] and [^b].\n\n[^1]: a<br>b <!-- note -->" + later},
		{name: "paragraph after a footnote with benign HTML", input: "See [^1].\n\n[^1]: a<br>b\n\nalpha beta gamma\n"},
		{name: "footnote that starts with an autolink, beside benign HTML", input: "See [^a] and [^b].\n\nIntro a<br>b.\n\n[^a]: <https://example.com>" + later},
		{
			name: "label with a bracket in backticks, beside benign HTML",
			input: "Intro a<br>b.\n\nSee [^a`[`] and [^b].\n\n[^a`[`]: First.\n\n    Second <code>open" +
				later,
		},
		{name: "label with a bracket in backticks", input: "See [^a`[`] and [^b].\n\n[^a`[`]: First.\n\n    Second <code>open" + later},
		{name: "label with an escaped bracket", input: "See [^a\\]] and [^b].\n\n[^a\\]]: First.\n\n    Second <code>open" + later},
		{name: "label with spaces", input: "See [^a b] and [^b].\n\n[^a b]: First.\n\n    Second <code>open" + later},
		{name: "label that spans two lines", input: "See [^b].\n\n[^a\nc]: First.\n\n    Second <code>open" + later},
		{name: "definition in a blockquote", input: "See [^a`[`] and [^b].\n\n> [^a`[`]: First.\n>\n>     Second <code>open" + later},
		{name: "definition in a list item", input: "See [^a`[`] and [^b].\n\n- [^a`[`]: First.\n\n      Second <code>open" + later},
		{
			name:  "definition inside another one",
			input: "See [^x], [^a`[`] and [^b].\n\n[^x]: [^a`[`]: First.\n\n        Second <code>open" + later,
		},
		{name: "definitions after nested ones", input: "See [^x], [^c] and [^b].\n\n[^x]: [^y]: z\n\n[^c]: <code>" + later},
		{name: "definition whose text follows four spaces", input: "See [^a] and [^b].\n\n[^a]:     x<code>open" + later},
		{name: "definition whose text follows tabs", input: "See [^a] and [^b].\n\n[^a]:\t\tx<code>open" + later},
		{name: "definition that starts an ordered list", input: "See [^a] and [^b].\n\n[^a]: 10. x\n\n        <code>open" + later},
		{name: "definition whose label holds an escaped bracket", input: "See [^a\\] and [^b].\n\n[^a\\]: <code>]: text" + later},
		{name: "definition after a tab in a blockquote", input: "See [^a] and [^b].\n\n>\t[^a]: <![CDATA[" + later},
		{name: "definition after a tab in a list item", input: "See [^a] and [^b].\n\n- item\n\t[^a]: <div title=\"x" + later},
		{name: "comment in a definition split by a line of spaces", input: "See [^a] and [^b].\n\n[^a]: <!--\n  \n    -->" + later},
		{
			name:  "comment in a definition in a blockquote split by an empty quote line",
			input: "See [^a] and [^b].\n\n> [^a]: x\n>     <!--\n>\n>     -->" + later,
		},
		{
			// goldmark reads a table that cmark-gfm does not, as its delimiter
			// row has another width, and so a code span that hides the tag.
			name:  "table in a definition that only goldmark reads",
			input: "See [^a] and [^b].\n\n[^a]: `span\n    | - | - |\n    `<code>`" + later,
		},
		{
			name:  "comment in a definition in a list item split by a line of spaces, with CRLF line endings",
			input: "See [^a] and [^b].\r\n\r\n- [^a]: <!--\r\n  \r\n      -->\r\n\r\n[^b]: alpha beta gamma\r\n",
		},
		{
			name: "label with a bracket in backticks, with CRLF line endings",
			input: strings.ReplaceAll("Intro a<br>b.\n\nSee [^a`[`] and [^b].\n\n[^a`[`]: First.\n\n    Second <code>open"+
				later, "\n", "\r\n"),
		},
		{name: "footnote with raw HTML, with CR line endings", input: "See [^1] and [^b].\r\r[^1]: <code>\r\r[^b]: alpha beta gamma\r"},
		{
			// The reference is no definition, and no renderer reads one here,
			// but any "[^" counts.
			name:  "undefined reference beside a link reference definition",
			input: "Intro a<br>b.\n\n[^a] is an undefined reference.\n[x]: /url\n\nalpha beta gamma\n",
		},
		{name: "definition shown in a code block", input: "Intro a<br>b.\n\n```markdown\n[^a]: Note.\n```\n\nalpha beta gamma\n"},
		{name: "footnote syntax in a code span", input: "Write `[^1]` for a note, a<br>b.\n\nalpha beta gamma\n"},
	}
	starts := map[string]string{
		"a link":           "[Source](https://example.com)",
		"a number":         "2019 study",
		"a code span":      "`code` here",
		"a heading":        "# Heading",
		"a fence":          "```go",
		"a table row":      "a | b",
		"a paragraph":      "  text",
		"a list":           "- x",
		"a blockquote":     "> x",
		"an HTML block":    "<div>",
		"a thematic break": "___",
	}
	for name, start := range starts {
		cases = append(cases, fixCheckCase{
			name:  "footnote that starts with " + name + ", beside benign HTML",
			input: "See [^a] and [^b].\n\nIntro a<br>b.\n\n[^a]: " + start + later,
		})
	}
	for idx := range cases {
		cases[idx].old, cases[idx].new = "beta gamma", "beta\ngamma"
	}
	runFixCheckCases(t, cases)
}

// TestFixCheckKeepsFixesBesideFootnotesWithoutTags checks that a document
// with a "[^" but no "<" that may start a tag is checked as on main, and so
// keeps the fixes that leave its rendering unchanged.
func TestFixCheckKeepsFixesBesideFootnotesWithoutTags(t *testing.T) {
	const later = "\n\n[^b]: alpha beta gamma\n"
	runFixCheckCases(t, []fixCheckCase{
		{
			name:  "wrapping a footnote in a document without HTML",
			input: "See [^1] and [^b].\n\n[^1]: First.\n\n    Second code" + later,
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			name:  "wrapping beside a definition goldmark does not read",
			input: "See [^a`[`] and [^b].\n\n[^a`[`]: First.\n\n    Second code" + later,
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			// No "<" starts a tag, so no renderer reads raw HTML anywhere.
			name:  "wrapping a footnote in a document whose only < starts no tag",
			input: "See [^1] and [^b], where 1 < 2, a <- b and {{< x >}}.\n\n[^1]: First." + later,
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			name:   "wrapping a footnote in CommonMark",
			flavor: config.FlavorCommonMark,
			input:  "See [^1] and [^b].\n\n[^1]: First." + later,
			old:    "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			// An autolink is no raw HTML.
			name:  "wrapping a footnote in a document whose only < start autolinks",
			input: "See [^1] and [^b], <https://example.com> and <user@example.com>.\n\n[^1]: First." + later,
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			// The check still withholds a fix that changes the rendering.
			name:  "splitting a footnote paragraph",
			input: "See [^1] and [^b].\n\n[^1]: First." + later,
			old:   "beta gamma", new: "beta\n\ngamma",
			want: false,
		},
	})
}

// TestFixCheckKeepsFixesBesideTagsWithoutFootnotes checks that a document
// with a "<" that may start a tag but no "[^" is checked as on main: benign
// raw HTML leaves the fixes to the check, and other raw HTML withholds them.
func TestFixCheckKeepsFixesBesideTagsWithoutFootnotes(t *testing.T) {
	runFixCheckCases(t, []fixCheckCase{
		{
			name:  "wrapping beside benign HTML",
			input: "Line<br> and <img src=\"a.png\">, x^2 and [a ^b](c).\n\nalpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			name:  "wrapping beside a caret after a bracket with a space",
			input: "See [ ^1] and a<br>b.\n\nalpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			name:  "wrapping beside other HTML",
			input: "Press <kbd>x</kbd>.\n\nalpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
	})
}

// TestFixCheckKeepsNoFixBesideHTMLInFrontMatter checks front matter that
// the check's parsers blank but GitHub renders as Markdown: a document with
// TOML or JSON front matter and a "<" that may start a tag anywhere keeps
// no fix, since raw HTML in the front matter may open an element that holds
// the rest of the page, and a block the front matter leaves open may take
// in raw HTML after it. GitHub shows YAML front matter as a table, so a "<"
// there leaves the fixes to the check.
func TestFixCheckKeepsNoFixBesideHTMLInFrontMatter(t *testing.T) {
	const body = "\nalpha beta gamma\n"
	cases := []fixCheckCase{
		{name: "TOML front matter", input: "+++\n<code>open\n+++\n" + body},
		{name: "JSON front matter", input: "{\n\"a\": \"<code>open\"\n}\n" + body},
		{name: "TOML front matter after a byte order mark", input: "\uFEFF+++\n<code>open\n+++\n" + body},
		{name: "TOML front matter with a footnote", input: "+++\n[^a]: <code>open\n+++\n\nSee [^a].\n" + body},
		// The indented line continues the paragraph that the front matter is
		// for GitHub, where goldmark reads code.
		{name: "TOML front matter before an indented line", input: "+++\ntitle = \"x\"\n+++\n    <code>\n" + body},
		{name: "JSON front matter before an indented line", input: "{\n\"a\": 1\n}\n    <code>\n" + body},
		{name: "TOML front matter that opens a list", input: "+++\n- x\n+++\n\n    <code>\n" + body},
		{name: "TOML front matter with an end tag", input: "+++\ntitle = \"a</b>\"\n+++\n" + body},
		{name: "TOML front matter with a comment", input: "+++\n+++\n\n<!-- x -->\n" + body},
		{name: "TOML front matter with a processing instruction", input: "+++\n<?x?>\n+++\n" + body},
		{name: "TOML front matter with CRLF line endings", input: "+++\r\n<code>open\r\n+++\r\n" + body},
		{name: "JSON front matter after a byte order mark", input: "\uFEFF{\n\"a\": \"<code>open\"\n}\n" + body},
		{name: "TOML front matter whose fences have trailing spaces", input: "+++  \n<code>open\n+++  \n" + body},
		// An autolink is no raw HTML.
		{name: "TOML front matter before an autolink", input: "+++\ntitle = \"x\"\n+++\n\nSee <https://example.com>.\n" + body, want: true},
		// No "<" starts a tag, so no renderer reads raw HTML anywhere.
		{name: "TOML front matter before a Hugo shortcode", input: "+++\ntitle = \"x\"\n+++\n\n{{< figure src=\"a.png\" >}}\n" + body, want: true},
		{name: "TOML front matter with a comparison", input: "+++\ntitle = \"1 < 2\"\n+++\n\nWhen 1 <= 2 or a <- b.\n" + body, want: true},
		{name: "YAML front matter", input: "---\ntitle: a <code> b\n---\n" + body, want: true},
		{name: "TOML front matter without HTML", input: "+++\ntitle = \"a\"\n+++\n" + body, want: true},
		{name: "JSON front matter without HTML", input: "{\n\"a\": 1\n}\n" + body, want: true},
	}
	for idx := range cases {
		cases[idx].old, cases[idx].new = "beta gamma", "beta\ngamma"
	}
	runFixCheckCases(t, cases)
}

// TestHasTagStart checks which "<" may start raw HTML.
func TestHasTagStart(t *testing.T) {
	tests := map[string]bool{
		"":                   false,
		"1 < 2":              false,
		"a <= b, c <- d":     false,
		"{{< x >}}":          false,
		"ends with <":        false,
		"<3 and <<":          false,
		"<b>":                true,
		"x</b>":              true,
		"<!-- c -->":         true,
		"<?x?>":              true,
		"<https://x>":        false,
		"<user@example.com>": false,
		"<a_b>":              false,
		"<\u00e9>":           false,
		"<1>":                false,
		"1 < 2 <Z":           true,
		"<div":               true,
		"<a\nhref=x>":        true,
		"<a\fb>":             true,
		"<code\v>":           true,
		"<br/>":              true,
		"</b >":              true,
		"<a-b>":              true,
		"<![CDATA[x]]>":      true,
		"<!DOCTYPE html>":    true,
	}
	for input, want := range tests {
		t.Run(strconv.Quote(input), func(t *testing.T) {
			assert.Equal(t, want, hasTagStart([]byte(input)))
		})
	}
}
