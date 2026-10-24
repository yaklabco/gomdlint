package rules

import "testing"

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
