package rules

import (
	"strings"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// TestFixCheckKeepsNoFixBesideFootnoteHTML checks raw HTML in GFM footnotes.
// The check's parsers do not read footnotes: they read "[^1]: <code>" as a
// link reference definition and an indented footnote paragraph as code, and
// see no raw HTML there. GitHub writes a footnote's raw HTML after the
// document, where the browser opens the element again for the footnotes
// after it, so a document with raw HTML in a footnote that is not benign
// keeps no fix. Benign HTML in a footnote, and footnotes without HTML,
// leave the fixes to the check.
func TestFixCheckKeepsNoFixBesideFootnoteHTML(t *testing.T) {
	runFixCheckCases(t, []fixCheckCase{
		{
			name:  "wrapping a footnote after a footnote with raw HTML",
			input: "See [^1] and [^2].\n\n[^1]: <code>\n\n[^2]: alpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			name:   "wrapping a footnote after a footnote with raw HTML in CommonMark",
			flavor: config.FlavorCommonMark,
			input:  "See [^1] and [^2].\n\n[^1]: <code>\n\n[^2]: alpha beta gamma\n",
			old:    "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			name:  "wrapping a paragraph after a footnote with raw HTML",
			input: "See [^1].\n\n[^1]: <code>\n\nalpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			name:  "wrapping a footnote after a footnote whose code block holds raw HTML",
			input: "See [^1] and [^2].\n\n[^1]: First.\n\n    Second <code>open\n\n[^2]: alpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			// GitHub folds the case of footnote labels, as goldmark does not,
			// so it writes this footnote and the code element in it.
			name:  "wrapping a footnote after a footnote with raw HTML whose label differs in case",
			input: "See [^a] and [^b].\n\n[^A]: <code>\n\n[^b]: alpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			// GitHub drops a footnote without a reference, but the check does
			// not match references, so it reads every footnote.
			name:  "wrapping a paragraph after an unreferenced footnote with raw HTML",
			input: "Text.\n\n[^1]: <code>\n\nalpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			name:  "wrapping a paragraph after a footnote with benign HTML",
			input: "See [^1].\n\n[^1]: a<br>b\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			name:  "wrapping a footnote beside a footnote with benign HTML",
			input: "See [^1] and [^2].\n\n[^1]: a<br>b <!-- note -->\n\n[^2]: alpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			name:  "wrapping a footnote in a document without HTML",
			input: "See [^1] and [^2].\n\n[^1]: First.\n\n    Second code\n\n[^2]: alpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
		{
			name:  "wrapping a footnote in a document whose only < is text",
			input: "See [^1] and [^2], where 1 < 2.\n\n[^1]: First.\n\n[^2]: alpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: true,
		},
	})
}

// TestFixCheckKeepsNoFixBesideUnreadFootnotes checks documents with a line
// that may start a footnote definition that goldmark does not read as one,
// while cmark-gfm, which GitHub uses, may: goldmark rejects a "[" in a
// label, even in backticks, and reads the indented continuation as code,
// where it sees no raw HTML. Such a document keeps no fix when it holds a
// "<", and keeps its fixes otherwise.
func TestFixCheckKeepsNoFixBesideUnreadFootnotes(t *testing.T) {
	const later = "\n\n[^b]: alpha beta gamma\n"
	cases := []fixCheckCase{
		{
			name: "label with a bracket in backticks, beside benign HTML",
			input: "Intro a<br>b.\n\nSee [^a`[`] and [^b].\n\n[^a`[`]: First.\n\n    Second <code>open" +
				later,
		},
		{
			name:  "label with a bracket in backticks, with no other HTML",
			input: "See [^a`[`] and [^b].\n\n[^a`[`]: First.\n\n    Second <code>open" + later,
		},
		{
			name:  "label with an escaped bracket",
			input: "See [^a\\]] and [^b].\n\n[^a\\]]: First.\n\n    Second <code>open" + later,
		},
		{
			name:  "label with spaces",
			input: "See [^a b] and [^b].\n\n[^a b]: First.\n\n    Second <code>open" + later,
		},
		{
			name:  "definition in a blockquote",
			input: "See [^a`[`] and [^b].\n\n> [^a`[`]: First.\n>\n>     Second <code>open" + later,
		},
		{
			name:  "definition in a list item",
			input: "See [^a`[`] and [^b].\n\n- [^a`[`]: First.\n\n      Second <code>open" + later,
		},
		{
			name:  "label that spans two lines",
			input: "See [^b].\n\n[^a\nc]: First.\n\n    Second <code>open" + later,
		},
		{
			// cmark-gfm opens the second definition inside the first.
			name:  "definition that goldmark does not read inside one it reads",
			input: "See [^x], [^a`[`] and [^b].\n\n[^x]: [^a`[`]: First.\n\n        Second <code>open" + later,
		},
		{
			name:  "definition that goldmark does not read in a list inside one it reads",
			input: "See [^x], [^a`[`] and [^b].\n\n[^x]: - [^a`[`]: First.\n\n            Second <code>open" + later,
		},
		{
			// goldmark's footnote extension moves the definitions it closes
			// into a list beside the first one closed, which here lies inside
			// x, and so drops every later definition from the document.
			name:  "definitions after nested ones that goldmark reads",
			input: "See [^x], [^c] and [^b].\n\n[^x]: [^y]: z\n\n[^c]: <code>" + later,
		},
		{
			// cmark-gfm skips the spaces after the label, while goldmark reads
			// an indented code block there, and the other parsers a link
			// reference definition: none of them shows the code element.
			name:  "definition whose text follows four spaces",
			input: "See [^a] and [^b].\n\n[^a]:     x<code>open" + later,
		},
		{
			name:  "definition whose text follows tabs",
			input: "See [^a] and [^b].\n\n[^a]:\t\tx<code>open" + later,
		},
		{
			name:  "definition in a blockquote whose text follows four spaces",
			input: "See [^a] and [^b].\n\n> [^a]:     x<code>open" + later,
		},
		{
			name:  "more definitions nested on one line than the footnote parse reads",
			input: "See [^a].\n\n" + strings.Repeat("[^a]: ", maxFootnoteNesting+1) + "x <br>" + later,
		},
		{
			name:  "definition in a list item whose text follows four spaces",
			input: "See [^a] and [^b].\n\n- [^a]:     x<code>open" + later,
		},
		{
			// cmark-gfm drops the space before the list marker, so its item
			// holds the indented line as text, where goldmark reads code.
			name:  "definition that starts an ordered list",
			input: "See [^a] and [^b].\n\n[^a]: 10. x\n\n        <code>open" + later,
		},
		{
			name:  "definition that starts a bullet list after spaces",
			input: "See [^a] and [^b].\n\n[^a]:   - x\n\n        <code>open" + later,
		},
		{
			name:  "definition that starts an ordered list after spaces",
			input: "See [^a] and [^b].\n\n[^a]:   1. x\n\n         <code>open" + later,
		},
		{
			name:  "definition that starts a bullet list with spaces after its marker",
			input: "See [^a] and [^b].\n\n[^a]: -    x\n\n         <code>open" + later,
		},
		{
			// goldmark reads an escaped bracket in a label, cmark-gfm ends
			// the label there and reads the rest as text.
			name:  "definition whose label holds an escaped bracket",
			input: "See [^a\\] and [^b].\n\n[^a\\]: <code>]: text" + later,
		},
		{
			name: "label with a bracket in backticks, with CRLF line endings",
			input: strings.ReplaceAll("Intro a<br>b.\n\nSee [^a`[`] and [^b].\n\n[^a`[`]: First.\n\n    Second <code>open"+
				later, "\n", "\r\n"),
		},
	}
	for idx := range cases {
		cases[idx].old, cases[idx].new = "beta gamma", "beta\ngamma"
	}
	cases = append(cases, fixCheckCase{
		name:  "unread definition in a document with no HTML",
		input: "See [^a`[`].\n\n[^a`[`]: First.\n\n    Second code" + later,
		old:   "beta gamma", new: "beta\ngamma",
		want: true,
	}, fixCheckCase{
		// goldmark puts the definition after the spaces a tab leaves in the
		// list item, which still counts as reading it.
		name:  "definition after a tab in a list item, beside benign HTML",
		input: "See [^b].\n\n- a\n\n\t[^b]: Note.\n\na<br>b end.\n\nalpha beta gamma\n",
		old:   "beta gamma", new: "beta\ngamma",
		want: true,
	})
	runFixCheckCases(t, cases)
}

// TestFixCheckKeepsNoFixBesideHTMLInFrontMatter checks front matter that
// the check's parsers blank but GitHub renders as Markdown: a document with
// TOML or JSON front matter and a "<" anywhere keeps no fix, since raw HTML
// in the front matter may open an element that holds the rest of the page,
// and a block the front matter leaves open may take in raw HTML after it.
// GitHub shows YAML front matter as a table, so a "<" there leaves the fixes
// to the check.
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
		{name: "TOML front matter that defines a footnote", input: "+++\n[^a]: x\n+++\n\n    <code>\n\nSee [^a].\n" + body},
		{name: "YAML front matter", input: "---\ntitle: a <code> b\n---\n" + body, want: true},
		{name: "TOML front matter without HTML", input: "+++\ntitle = \"a\"\n+++\n" + body, want: true},
		{name: "JSON front matter without HTML", input: "{\n\"a\": 1\n}\n" + body, want: true},
	}
	for idx := range cases {
		cases[idx].old, cases[idx].new = "beta gamma", "beta\ngamma"
	}
	runFixCheckCases(t, cases)
}
