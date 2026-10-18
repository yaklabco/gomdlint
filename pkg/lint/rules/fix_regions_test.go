package rules

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// regionSnippets are pieces of Markdown whose parse reaches past their own
// lines, or that the fixing rules change: front matter, reference
// definitions and links through them, headings with repeated anchors and
// fragment links, raw HTML blocks of every kind, tight and loose lists,
// nested lists, list items that start with link reference definitions,
// lazy continuation lines, fences and HTML blocks left open, setext
// headings, tables, tabs and trailing spaces.
var regionSnippets = []string{
	"# Heading one\n",
	"## Heading  two ##\n",
	"## Heading one\n",
	"# Foo\tBar\n",
	"### Trailing punctuation.\n",
	"Setext heading\n===\n",
	"Setext two\n---\n",
	"Paragraph with trailing spaces   \nand a second line.  \nand a hard break.\n",
	"A very long line that goes on and on past the limit for sure, with words.\n",
	"Text with *emphasis* and `code  span` and a [link](#heading-one).\n",
	"See [the docs][docs] and [other][] and [shortcut].\n",
	"[docs]: https://example.com/docs\n",
	"[other]: </a b> \"Title  here\"\n",
	"[shortcut]: /short\n",
	"[docs]: https://example.com/duplicate\n",
	"[label\n over two lines]: /url\n",
	"<div>\nblock html  \n</div>\n",
	"<!-- comment\n\nstill comment  \n-->\n",
	"<pre>\npre  text\t\n</pre>\n",
	"<br>\n",
	"<custom-tag>\ntext\n",
	"- tight one\n- tight two\n- tight three\n",
	"- loose one\n\n- loose two\n\n- loose three\n",
	"-  wide marker\n-  wide again\n-  and again\n-  more\n",
	"* star list\n* star list\n* star list\n* star list\n",
	"1. one\n2. two\n3. three\n4. four\n5. five\n",
	"3. three\n4. four\n5. five\n6. six\n",
	"- parent\n   - child at three\n      - grandchild\n- next\n- last\n",
	"- item\n\n  continued paragraph\n- other\n- more\n- end\n",
	"- item\nlazy continuation line\n- other\n- more\n",
	"- a\n- b\n\n  para in b\n- c\n- d\n- e\n",
	"- [ ] task\n- [x] done\n- [ ] more\n- [ ] end\n",
	"> quote line\nlazy quote line\n",
	"> - quoted item\n>   - nested\n",
	"```\nunclosed fence\n",
	"~~~js\ncode\n~~~\n",
	"    indented code\n\n    more code\n",
	"| a | b |\n| - | - |\n| 1 | 2 |\n",
	"***\n",
	"Text right before a list\n- item\n- item two\n",
	"\n\n",
	"Final line without newline",
	"Escaped \\# hash and &#35; refs.\n",
	// goldmark 1.8 gives each definition a node of its own and reads a list
	// as loose when an item starts with definitions followed by text after a
	// blank line, where CommonMark reads it as tight.
	"- [d1]: /one\n  text after a definition\n- second\n- third\n- fourth\n",
	"- [d2]: /two\n  [d3]: /three\n  text after two\n- second\n- third\n- fourth\n",
	"- first\n- [d4]: /four\n  \"Title\"\n  text after a title\n- third\n- fourth\n",
	"- [d5]: /five\n- after a definition alone\n- third\n- fourth\n",
	"1. [d6]: /six\n   text\n2. two\n3. three\n4. four\n",
	"- [d7]: /seven\n  Setext in an item\n  ===\n- second\n- third\n",
	"- [d8]: /eight\n\n  text after a blank line\n- second\n- third\n",
	"Intro paragraph\n- [d9]: /nine\n  text\n- b\n- c\n- d\n",
	"> - [d10]: /ten\n>   quoted text\n> - b\n> - c\n",
	// goldmark can take a definition from a line after one that stays in
	// the paragraph: a title with a nested parenthesis, or a quoted title
	// that a definition follows on its closing line.
	"- a\n- [g1]: /g1\n  (x (y\n  [g2]: /g2\n- c   \n- d\n\n- e\n- f\n",
	"- a\n- [g3]: /g3\n  \"multi\n  line\" [g4]: /g4\n  text\n- c   \n- d\n- e\n",
	// Blocks that follow a closed block with no blank line between them,
	// where goldmark opens the next block before it closes the last.
	"Paragraph text   \nAnother short line   \n***\n",
	"Text before a heading   \n## Heading after text\n",
	"***\nText right after a break   \n",
	"## Heading one\nText right after a heading   \n",
	"<!-- one line comment -->\nText after a comment   \n",
	"```\nfenced\n```\nText after a fence   \n",
	"Setext heading\n---\nText right after a setext heading   \n",
	"    indented code\nText after indented code   \n",
	"<div>\nhtml six\n</div>\nText after html\n",
	"[docs]: /closed\n***\n- [d16]: /sixteen\n  text\n- b\n",
	// A GFM table made from an item's first paragraph comes before the setext
	// heading made from the same paragraph, which keeps its blank-line flag.
	"***\n- a\n  b|c\n  -|-\n  ===\n",
	"## Heading\n- [t1]: /t1\n  a\n  b|c\n  -|-\n  ===\n",
	"- x\n- a\n  b|c\n  -|-\n  ===\n- y   \n- z\n- w\n",
	// Headings whose slugs collide with numbered anchors: "a-" and "a- b"
	// become "a" and "a-b" when their spaces go, and "a" must pass "a-1" and
	// "a-2" to find a free number.
	"# a\n\n# a-1\n\n# a-2\n\n# `a `\n\n# `a `\n",
	"## a-b\n## a-\tb\n## `a ` b\n",
}

// regionEditTexts are the texts random edits insert.
var regionEditTexts = []string{
	"", "", " ", "  ", "   ", "\t", "\n", "\n\n", "x", "- ", "* ", "1. ", "2) ", "> ", "#", "# ",
	"```", "~~~", "<div>", "</div>", "<!--", "-->", "---", "===", "|", "[docs]: /changed\n", "    ",
	"\\", "[", "]", "*", "_", "\xef\xbb\xbf", "---\n", "+++", "{\n",
}

// randomRegionDocument joins random snippets, sometimes after front matter
// or a byte order mark, sometimes with CRLF line endings.
func randomRegionDocument(rng *rand.Rand) string {
	var doc strings.Builder
	switch rng.IntN(10) {
	case 0:
		doc.WriteString("---\ntitle: x\n---\n")
	case 1:
		doc.WriteString("---\nopen: front matter\n")
	}
	for range 2 + rng.IntN(8) {
		doc.WriteString(regionSnippets[rng.IntN(len(regionSnippets))])
		switch rng.IntN(3) {
		case 0:
			doc.WriteString("\n")
		case 1:
			doc.WriteString("\n\n")
		}
	}
	text := doc.String()
	if rng.IntN(10) == 0 {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if rng.IntN(15) == 0 {
		text = "\xef\xbb\xbf" + text
	}
	return text
}

// randomKeepingEdit returns an edit of the kinds the fixing rules make,
// which often keep the rendering: removing the spaces or tabs that end a
// line, adding or removing a blank line, narrowing the space after a list
// marker, or renumbering an ordered item. ok is false when the chosen line
// offers none.
func randomKeepingEdit(rng *rand.Rand, text string) (fix.TextEdit, bool) {
	lineStart := strings.LastIndexByte(text[:rng.IntN(len(text)+1)], '\n') + 1
	lineEnd := len(text)
	if end := strings.IndexByte(text[lineStart:], '\n'); end >= 0 {
		lineEnd = lineStart + end
	}
	line := strings.TrimSuffix(text[lineStart:lineEnd], "\r")
	trimmed := strings.TrimRight(line, " \t")
	content := strings.TrimLeft(line, " \t>")
	switch rng.IntN(5) {
	case 0:
		if len(trimmed) < len(line) {
			return fix.TextEdit{StartOffset: lineStart + len(trimmed), EndOffset: lineStart + len(line)}, true
		}
	case 1:
		return fix.TextEdit{StartOffset: lineStart, EndOffset: lineStart, NewText: "\n"}, true
	case 2:
		if strings.TrimSpace(line) == "" && lineEnd < len(text) {
			return fix.TextEdit{StartOffset: lineStart, EndOffset: lineEnd + 1}, true
		}
	case 3:
		if marker := len(line) - len(content); len(content) > 2 && strings.ContainsRune("-*+", rune(content[0])) &&
			strings.HasPrefix(content[1:], "  ") {
			return fix.TextEdit{StartOffset: lineStart + marker + 1, EndOffset: lineStart + marker + 2}, true
		}
	case 4:
		if marker := len(line) - len(content); len(content) > 2 && content[0] >= '0' && content[0] <= '9' && content[1] == '.' {
			return fix.TextEdit{StartOffset: lineStart + marker, EndOffset: lineStart + marker + 1, NewText: string(rune('1' + rng.IntN(9)))}, true
		}
	}
	return fix.TextEdit{}, false
}

// randomRegionEdits returns one to four edits: with keeping set, edits of
// the kinds the fixing rules make, and otherwise random edits or those.
func randomRegionEdits(rng *rand.Rand, text string, keeping bool) []fix.TextEdit {
	var edits []fix.TextEdit
	for attempt := 0; len(edits) < 1+rng.IntN(4) && attempt < 20; attempt++ {
		if keeping || rng.IntN(2) == 0 {
			if edit, ok := randomKeepingEdit(rng, text); ok {
				edits = append(edits, edit)
			}
			continue
		}
		edits = append(edits, randomRegionEdit(rng, text))
	}
	return edits
}

// randomRegionEdit returns an edit at a random place of text, most often at
// the start or end of a line.
func randomRegionEdit(rng *rand.Rand, text string) fix.TextEdit {
	start := rng.IntN(len(text) + 1)
	switch rng.IntN(3) {
	case 0:
		start = strings.LastIndexByte(text[:start], '\n') + 1
	case 1:
		if end := strings.IndexByte(text[start:], '\n'); end >= 0 {
			start += end
		}
	}
	end := start
	if rng.IntN(2) == 0 {
		end = min(len(text), start+rng.IntN(6))
	}
	return fix.TextEdit{StartOffset: start, EndOffset: end, NewText: regionEditTexts[rng.IntN(len(regionEditTexts))]}
}

// TestRegionChecksDecideAsWholeDocument checks region checks against the
// whole-document check they stand for: on random documents and random
// edits, alone and in sets, every decision region checks make equals the
// decision of comparing the signatures of the whole documents.
func TestRegionChecksDecideAsWholeDocument(t *testing.T) {
	documents := 1500
	if testing.Short() || raceEnabled {
		documents = 200
	}
	rng := rand.New(rand.NewPCG(20261017, 7))
	decided, total, kept := 0, 0, 0
	for range documents {
		text := randomRegionDocument(rng)
		if text == "" {
			continue
		}
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			snapshot := parseForCheckWith(t, flavor, text)
			check := newFixCheck(snapshot, flavor)
			for trial := range 12 {
				edits := randomRegionEdits(rng, text, trial%2 == 0)
				prepared, err := fix.PrepareEdits(edits, len(text))
				if err != nil || len(prepared) == 0 {
					continue
				}
				total++
				want := check.unchanged(fix.ApplyEdits(snapshot.Content, prepared))
				ok, got := check.regionsUnchanged(prepared)
				if !ok {
					continue
				}
				decided++
				if want {
					kept++
				}
				require.Equal(t, want, got, "flavor %s, document %q, edits %q, fixed %q",
					flavor, text, prepared, fix.ApplyEdits(snapshot.Content, prepared))
			}
		}
	}
	t.Logf("%d edit sets, %d decided by region checks, %d of them keep the document", total, decided, kept)
	assert.Greater(t, decided, total/2, "region checks decide most edit sets")
	assert.Positive(t, kept, "some edit sets keep the document")
}

// wholeKeptInOrder keeps fixes one at a time in order, each when it and the
// fixes kept before it leave the whole document's signatures unchanged, as
// parsing the whole document finds: the loop that keptInOrder speeds up.
func wholeKeptInOrder(check *fixCheck, diags []lint.Diagnostic, kept []int) []int {
	if !check.htmlBenign() {
		return nil
	}
	var together []int
	for _, idx := range kept {
		var edits []fix.TextEdit
		for _, other := range append(slices.Clone(together), idx) {
			edits = append(edits, diags[other].FixEdits...)
		}
		accepted, _, _, err := fix.PrepareEditsFiltered(edits, len(check.content))
		if err == nil && check.unchanged(fix.ApplyEdits(check.content, accepted)) {
			together = append(together, idx)
		}
	}
	return together
}

// TestKeptInOrderMatchesWholeDocument checks that keeping fixes one at a time
// with region states keeps the fixes that checking the whole document at
// each step keeps, on random documents and fixes, some of which overlap.
func TestKeptInOrderMatchesWholeDocument(t *testing.T) {
	documents := 400
	if testing.Short() || raceEnabled {
		documents = 60
	}
	rng := rand.New(rand.NewPCG(20261017, 11))
	steps, decided := 0, 0
	for tested := 0; tested < documents; {
		text := randomRegionDocument(rng) + randomRegionDocument(rng)
		// Documents whose raw HTML is not benign keep no fix at all.
		if !newFixCheck(parseForCheck(t, text), config.FlavorGFM).htmlBenign() {
			continue
		}
		tested++
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			snapshot := parseForCheckWith(t, flavor, text)
			for trial := range 3 {
				var diags []lint.Diagnostic
				var kept []int
				for range 3 + rng.IntN(8) {
					edits := randomRegionEdits(rng, text, trial != 2)
					if len(edits) == 0 {
						continue
					}
					diags = append(diags, lint.Diagnostic{RuleID: "T1", FixEdits: edits})
					kept = append(kept, len(diags)-1)
				}
				want := wholeKeptInOrder(newFixCheck(snapshot, flavor), diags, kept)
				got, statesDecided := newFixCheck(snapshot, flavor).keptInOrder(diags, kept)
				require.Equal(t, want, got, "flavor %s, document %q, fixes %v", flavor, text, diags)
				steps += len(kept)
				decided += statesDecided
			}
		}
	}
	t.Logf("%d steps, %d decided by region states", steps, decided)
	assert.Greater(t, decided, steps/2, "region states decide most steps")
}

// TestKeptInOrderWithRegionsThatOffset checks a fix whose edits fall in two
// regions that offset each other: it adds a paragraph at the top and deletes
// a paragraph with the same text at the bottom, so the signature stays the
// same though each region's part changes.
func TestKeptInOrderWithRegionsThatOffset(t *testing.T) {
	const paragraph = "Same text.\n\n"
	head := strings.Repeat(paragraph, 2)
	input := head + "Same text.\n"
	diags := []lint.Diagnostic{{RuleID: "T1", FixEdits: []fix.TextEdit{
		{StartOffset: 0, EndOffset: 0, NewText: paragraph},
		{StartOffset: len(head), EndOffset: len(input)},
	}}}
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		snapshot := parseForCheckWith(t, flavor, input)
		want := wholeKeptInOrder(newFixCheck(snapshot, flavor), diags, []int{0})
		require.Equal(t, []int{0}, want, "the fix keeps the document")
		got, _ := newFixCheck(snapshot, flavor).keptInOrder(diags, []int{0})
		assert.Equal(t, want, got, "flavor %s", flavor)
	}
}

// TestRegionChecksOnLongLists checks list regions against the whole-document
// check on edits inside long lists: edits that loosen or tighten the list,
// split or join items, nest them, end the list early or leave its items as
// they render.
func TestRegionChecksOnLongLists(t *testing.T) {
	var list strings.Builder
	list.WriteString("# Title\n\nIntro.\n\n")
	for idx := range 12 {
		fmt.Fprintf(&list, "- item %d with trailing spaces   \n", idx)
		if idx == 5 {
			list.WriteString("  continued text of item five\n")
		}
	}
	list.WriteString("\nAfter the list.\n")
	tight := list.String()
	loose := strings.ReplaceAll(tight, "   \n- item", "   \n\n- item")
	ordered := "# Title\n\n1. one\n2. two\n3. three\n4. four\n5. five\n6. six\n7. seven\n8. eight\n\nEnd.\n"
	edits := []struct {
		name     string
		old, new string
	}{
		{name: "trailing spaces", old: "item 4 with trailing spaces   ", new: "item 4 with trailing spaces"},
		{name: "blank line between items", old: "spaces   \n- item 6", new: "spaces   \n\n- item 6"},
		{name: "joined items", old: "spaces   \n- item 7", new: "spaces   item 7"},
		{name: "nested item", old: "\n- item 8", new: "\n  - item 8"},
		{name: "item ended by a paragraph", old: "\n- item 9", new: "\n\nitem 9"},
		{name: "lazy continuation", old: "  continued text", new: "continued text"},
		{name: "split item", old: "item 3 with", new: "item 3\n- with"},
		{name: "code in an item", old: "  continued text", new: "\n      continued text"},
		{name: "heading in an item", old: "- item 4 with", new: "- # item 4 with"},
		{name: "blank line removed", old: "spaces   \n\n- item 6", new: "spaces   \n- item 6"},
		{name: "renumbered item", old: "4. four", new: "9. four"},
		{name: "other delimiter", old: "5. five", new: "5) five"},
		{name: "fence left open", old: "- item 5 with", new: "- ```item 5 with"},
	}
	cases, decided := 0, 0
	for docName, doc := range map[string]string{"tight": tight, "loose": loose, "ordered": ordered} {
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			snapshot := parseForCheckWith(t, flavor, doc)
			check := newFixCheck(snapshot, flavor)
			for _, tt := range edits {
				if strings.Count(doc, tt.old) != 1 {
					continue
				}
				cases++
				t.Run(docName+"/"+string(flavor)+"/"+tt.name, func(t *testing.T) {
					edit := replaceOnce(t, doc, tt.old, tt.new)
					want := check.unchanged(fix.ApplyEdits(snapshot.Content, []fix.TextEdit{edit}))
					ok, got := check.regionsUnchanged([]fix.TextEdit{edit})
					if ok {
						decided++
						assert.Equal(t, want, got)
					}
				})
			}
		}
	}
	// Edits in long lists are what list regions are for.
	assert.Equal(t, cases, decided, "region checks decide every case")
}

// TestRegionChecksOnListsThatStartWithDefinitions checks list regions on
// lists whose items start with link reference definitions followed by text.
// goldmark gives each definition a node of its own and reads such a list as
// loose when the item follows a blank line, where CommonMark reads it as
// tight, and a list region parses its first item on its first line, as if a
// blank line came before it. Both region checks decide every edit that keeps
// the definitions, as the whole-document checks do.
func TestRegionChecksOnListsThatStartWithDefinitions(t *testing.T) {
	var list strings.Builder
	list.WriteString("# Title\n\nIntro.\n\n")
	for idx := range 12 {
		if idx%3 == 1 {
			fmt.Fprintf(&list, "- [ref%d]: /url%d\n  text of item %d   \n", idx, idx, idx)
			continue
		}
		fmt.Fprintf(&list, "- item %d with trailing spaces   \n", idx)
	}
	list.WriteString("\nAfter the list.\n")
	inside := list.String()
	docs := map[string]string{
		"definitions inside": inside,
		"definition first": strings.Replace(inside, "- item 0 with trailing spaces   \n",
			"- [ref0]: /url0\n  text of item 0\n", 1),
		"loose": strings.ReplaceAll(inside, "   \n- ", "   \n\n- "),
		// A definition after a line its paragraph keeps, in the item before
		// the edited ones.
		"kept line":       "# Title\n\n- a\n- [a]: /a\n  (x (y\n  [b]: /b\n- c\n\n- d\n- e\n- f\n",
		"kept line tight": "# Title\n\n- a\n- [a]: /a\n  (x (y\n  [b]: /b\n- c   \n- d\n- e\n- f\n",
	}
	edits := []struct {
		name     string
		old, new string
	}{
		{name: "trailing spaces after a definition item", old: "item 2 with trailing spaces   ", new: "item 2 with trailing spaces"},
		{name: "trailing spaces after a definition", old: "text of item 4   ", new: "text of item 4"},
		{name: "trailing spaces in the second item", old: "text of item 1   ", new: "text of item 1"},
		{name: "blank line before a definition item", old: "spaces   \n- [ref7]", new: "spaces   \n\n- [ref7]"},
		{name: "blank line after a definition", old: "/url4\n  text", new: "/url4\n\n  text"},
		{name: "blank line removed", old: "spaces   \n\n- [ref7]", new: "spaces   \n- [ref7]"},
		{name: "item split after a definition", old: "text of item 10", new: "text\n- of item 10"},
		{name: "definition item nested", old: "\n- [ref7]", new: "\n  - [ref7]"},
		{name: "blank line removed after an item", old: "- c\n\n- d", new: "- c\n- d"},
		{name: "trailing spaces after a kept line", old: "- c   ", new: "- c"},
	}
	cases, decided := 0, 0
	for docName, doc := range docs {
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			snapshot := parseForCheckWith(t, flavor, doc)
			check := newFixCheck(snapshot, flavor)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), fixAllowance{})
			model := verifier.regions()
			for _, tt := range edits {
				if strings.Count(doc, tt.old) != 1 {
					continue
				}
				cases++
				t.Run(docName+"/"+string(flavor)+"/"+tt.name, func(t *testing.T) {
					change := []fix.TextEdit{replaceOnce(t, doc, tt.old, tt.new)}
					want := check.unchanged(fix.ApplyEdits(snapshot.Content, change))
					ok, got := check.regionsUnchanged(change)
					outlineWant := verifier.acceptsWhole(change)
					outlineOK, outlineGot := false, false
					if model != nil {
						outlineOK, outlineGot = model.checkEdits(change)
					}
					if ok && outlineOK {
						decided++
					}
					if ok {
						assert.Equal(t, want, got, "render signature")
					}
					if outlineOK {
						assert.Equal(t, outlineWant, outlineGot, "outline")
					}
				})
			}
		}
	}
	assert.Equal(t, cases, decided, "both region checks decide every case")
}

// TestKeptInOrderOnFirstLines checks keeping fixes one at a time with region
// states against checking the whole document at each step, where fixes join
// the first lines, so that later fixes fall on the first line, and where the
// joined first line opens front matter or gains a byte order mark.
func TestKeptInOrderOnFirstLines(t *testing.T) {
	documents := 600
	if testing.Short() || raceEnabled {
		documents = 100
	}
	pieces := []string{
		"-", "--", "---", "+", "++", "+++", "{", "x", "  ", "", "", "# t", "\xef\xbb\xbf", "  ---", "  +++", " {",
	}
	texts := []string{"", "", "-", "+", "{", "\xef\xbb\xbf", "\n", "x", " "}
	closers := []string{"", "---\n", "+++\n", "}\n"}
	// Joining the empty first line with "  ---" keeps the document, and the
	// first line does not open front matter while it is indented. Taking
	// the spaces away then makes it open front matter, which runs to the
	// "---" below, though the fix starts after the first line.
	opener := "\n  ---\n\nText   \n\n---\n\nMore text   \n"
	openerFixes := []lint.Diagnostic{
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1}}},
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: 1, EndOffset: 3}}},
	}
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		snapshot := parseForCheckWith(t, flavor, opener)
		require.Equal(t, []int{0}, wholeKeptInOrder(newFixCheck(snapshot, flavor), openerFixes, []int{0, 1}))
		got, statesDecided := newFixCheck(snapshot, flavor).keptInOrder(openerFixes, []int{0, 1})
		assert.Equal(t, []int{0}, got, "flavor %s", flavor)
		assert.Equal(t, 1, statesDecided, "the states decide the join, and leave the opener to the whole check")
	}
	rng := rand.New(rand.NewPCG(20261018, 5))
	steps, decided := 0, 0
	for range documents {
		var head strings.Builder
		head.WriteString([]string{"", "", "  ", "x"}[rng.IntN(4)] + "\n")
		for range 1 + rng.IntN(4) {
			head.WriteString(pieces[rng.IntN(len(pieces))] + "\n")
		}
		text := head.String() + "\nText   \n\n" + closers[rng.IntN(len(closers))] + "\nMore text   \n"
		var newlines, indents []int
		for idx := range head.Len() {
			switch {
			case text[idx] == '\n':
				newlines = append(newlines, idx)
			case text[idx] == ' ' && (idx == 0 || text[idx-1] == '\n'):
				indents = append(indents, idx)
			}
		}
		var diags []lint.Diagnostic
		for range 2 + rng.IntN(6) {
			var edit fix.TextEdit
			switch rng.IntN(4) {
			case 0:
				at := newlines[rng.IntN(len(newlines))]
				edit = fix.TextEdit{StartOffset: at, EndOffset: at + 1}
			case 1:
				at := rng.IntN(head.Len() + 1)
				edit = fix.TextEdit{StartOffset: at, EndOffset: at, NewText: texts[rng.IntN(len(texts))]}
			case 2:
				if len(indents) == 0 {
					continue
				}
				// Remove the spaces that start a line.
				at := indents[rng.IntN(len(indents))]
				end := at
				for text[end] == ' ' {
					end++
				}
				edit = fix.TextEdit{StartOffset: at, EndOffset: end}
			default:
				edits := trailingSpaceEdits(text)
				edit = edits[rng.IntN(len(edits))]
			}
			diags = append(diags, lint.Diagnostic{RuleID: "T1", FixEdits: []fix.TextEdit{edit}})
		}
		slices.SortStableFunc(diags, func(a, b lint.Diagnostic) int {
			return a.FixEdits[0].StartOffset - b.FixEdits[0].StartOffset
		})
		kept := make([]int, len(diags))
		for idx := range kept {
			kept[idx] = idx
		}
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			snapshot := parseForCheckWith(t, flavor, text)
			want := wholeKeptInOrder(newFixCheck(snapshot, flavor), diags, kept)
			got, statesDecided := newFixCheck(snapshot, flavor).keptInOrder(diags, kept)
			require.Equal(t, want, got, "flavor %s, document %q, fixes %v", flavor, text, diags)
			steps += len(kept)
			decided += statesDecided
		}
	}
	t.Logf("%d steps, %d decided by region states", steps, decided)
	assert.Greater(t, decided, steps/4, "region states decide many steps")
}

// leafRunDocument returns a document of n paragraphs of two lines that end
// in trailing spaces, each followed, with no blank line, by the line closing.
func leafRunDocument(n int, closing string) string {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for range n {
		doc.WriteString("Paragraph text   \nAnother short line   \n" + closing + "\n")
	}
	return doc.String()
}

// trailingSpaceEdits returns, for each line of text that ends in spaces, the
// edit that removes them, as MD009 fixes them.
func trailingSpaceEdits(text string) []fix.TextEdit {
	var edits []fix.TextEdit
	start := 0
	for start < len(text) {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text) - start
		}
		line := text[start : start+end]
		if trimmed := strings.TrimRight(line, " "); len(trimmed) < len(line) {
			edits = append(edits, fix.TextEdit{StartOffset: start + len(trimmed), EndOffset: start + len(line)})
		}
		start += end + 1
	}
	return edits
}

// TestRegionParsesGrowWithTheDocument checks that region checks parse text
// that grows with the document, not with its square, when the document is
// many small blocks, each closed by a thematic break, an ATX heading, a
// one-line HTML block or a fence with no blank line before the next.
// goldmark opens each paragraph while the block before it is still open,
// which made the document one group, so every check parsed it all. Each fix
// is checked on its own, as fixes that share a block are; four times the
// paragraphs must take about four times the bytes, not sixteen.
func TestRegionParsesGrowWithTheDocument(t *testing.T) {
	const small, factor = 50, 4
	for _, closing := range []string{"***", "## Heading", "<!-- comment -->", "```\ncode\n```"} {
		t.Run(closing, func(t *testing.T) {
			parsed := func(n int) int {
				input := leafRunDocument(n, closing)
				edits := trailingSpaceEdits(input)
				require.Len(t, edits, 2*n)
				total := 0
				for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
					snapshot := parseForCheckWith(t, flavor, input)
					check := newFixCheck(snapshot, flavor)
					cfg := config.NewConfig()
					cfg.Flavor = flavor
					verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), fixAllowance{})
					outline := verifier.regions()
					require.NotNil(t, outline)
					for _, edit := range edits {
						decided, _ := check.regionsUnchanged([]fix.TextEdit{edit})
						require.True(t, decided, "render signature, edit %+v", edit)
						decided, _ = outline.checkEdits([]fix.TextEdit{edit})
						require.True(t, decided, "outline, edit %+v", edit)
					}
					for _, model := range check.models {
						require.NotNil(t, model)
						total += model.work.parsed
					}
					total += outline.work.parsed
				}
				return total
			}
			short, long := parsed(small), parsed(factor*small)
			ratio := float64(long) / float64(short)
			t.Logf("%d bytes parsed for %d paragraphs, %d for %d, ratio %.1f", short, small, long, factor*small, ratio)
			assert.Less(t, ratio, 1.5*factor, "region parses grow faster than the document")
		})
	}
}

// mixedLeafDocument returns a document of n paragraphs, each followed, with
// no blank line, by a thematic break, an ATX heading, a one-line HTML comment
// or a fenced code block in turn.
func mixedLeafDocument(n int) string {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range n {
		fmt.Fprintf(&doc, "Paragraph %d text   \nAnother short line   \n", idx)
		switch idx % 4 {
		case 0:
			doc.WriteString("***\n")
		case 1:
			fmt.Fprintf(&doc, "## Heading %d\n", idx)
		case 2:
			fmt.Fprintf(&doc, "<!-- comment %d -->\n", idx)
		default:
			fmt.Fprintf(&doc, "```\ncode %d\n```\n", idx)
		}
	}
	return doc.String()
}

// separateListsDocument returns a document of n lists of six items, each
// list followed by a paragraph, with three trailing spaces after each list's
// third item.
func separateListsDocument(n int) string {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range n {
		fmt.Fprintf(&doc, "- one %d\n- two\n- three   \n- four\n- five\n- six\n\nParagraph %d.\n\n", idx, idx)
	}
	return doc.String()
}

// breaksBeforeTextDocument returns a document of n thematic breaks, each
// followed with no blank line by a paragraph of one line that ends in three
// spaces and a blank line, and the edits that put a blank line after each
// break. Each edit changes how the group after its break starts, so its
// region must grow by that group.
func breaksBeforeTextDocument(n int) (string, []fix.TextEdit) {
	var doc strings.Builder
	var edits []fix.TextEdit
	doc.WriteString("# Title\n\n")
	for idx := range n {
		doc.WriteString("***")
		edits = append(edits, fix.TextEdit{StartOffset: doc.Len(), EndOffset: doc.Len(), NewText: "\n"})
		fmt.Fprintf(&doc, "\nText %d   \n\n", idx)
	}
	return doc.String(), edits
}

// regionWorkOf checks edits together with both region checks, as a rule's
// batch of fixes is checked, requiring both to decide, and returns the work
// their layouts counted.
func regionWorkOf(t *testing.T, input string, edits []fix.TextEdit) regionWork {
	t.Helper()
	var total regionWork
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		snapshot := parseForCheckWith(t, flavor, input)
		check := newFixCheck(snapshot, flavor)
		decided, _ := check.regionsUnchanged(edits)
		require.True(t, decided, "render signature")
		cfg := config.NewConfig()
		cfg.Flavor = flavor
		verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), fixAllowance{})
		outline := verifier.regions()
		require.NotNil(t, outline)
		decided, _ = outline.checkEdits(edits)
		require.True(t, decided, "outline")
		works := []regionWork{outline.work}
		for _, model := range check.models {
			require.NotNil(t, model)
			works = append(works, model.work)
		}
		for _, work := range works {
			total.parsed += work.parsed
			total.merged += work.merged
			total.tightVisits += work.tightVisits
			total.placing += work.placing
		}
	}
	return total
}

// TestRegionWorkGrowsWithTheDocument checks that combining the regions of
// one check takes work that grows with the document, not with its square:
// list tightness over many separate lists, each with one edit, and the
// merging of regions that grow, one for each edit, with other regions
// between them or with a region in every group, next to one another. Four
// times the document must take about four times the work, not sixteen.
func TestRegionWorkGrowsWithTheDocument(t *testing.T) {
	const small, factor = 50, 4
	ratio := func(short, long int) float64 { return float64(long) / float64(short) }
	t.Run("separate lists", func(t *testing.T) {
		work := func(lists int) regionWork {
			input := separateListsDocument(lists)
			edits := trailingSpaceEdits(input)
			require.Len(t, edits, lists)
			return regionWorkOf(t, input, edits)
		}
		short, long := work(small), work(factor*small)
		t.Logf("tightness visits %d for %d lists, %d for %d", short.tightVisits, small, long.tightVisits, factor*small)
		assert.Less(t, ratio(short.tightVisits, long.tightVisits), 1.5*factor, "tightness work grows faster than the document")
	})
	t.Run("growing regions next to one another", func(t *testing.T) {
		work := func(breaks int) regionWork {
			input, edits := breaksBeforeTextDocument(breaks)
			edits = append(edits, trailingSpaceEdits(input)...)
			prepared, err := fix.PrepareEdits(edits, len(input))
			require.NoError(t, err)
			return regionWorkOf(t, input, prepared)
		}
		short, long := work(small), work(factor*small)
		t.Logf("regions merged %d for %d breaks, %d for %d", short.merged, small, long.merged, factor*small)
		assert.Less(t, ratio(short.merged, long.merged), 1.5*factor, "merge work grows faster than the document")
	})
	t.Run("growing regions", func(t *testing.T) {
		work := func(breaks int) regionWork {
			input, edits := breaksBeforeTextDocument(breaks)
			return regionWorkOf(t, input, edits)
		}
		short, long := work(small), work(factor*small)
		t.Logf("regions merged %d for %d breaks, %d for %d", short.merged, small, long.merged, factor*small)
		assert.Less(t, ratio(short.merged, long.merged), 1.5*factor, "merge work grows faster than the document")
	})
	t.Run("many edits on one line", func(t *testing.T) {
		work := func(spans int) regionWork {
			input := "# Title\n\n" + strings.Repeat("word ` a` ", spans) + "\n"
			var edits []fix.TextEdit
			for idx := range len(input) {
				if strings.HasPrefix(input[idx:], "` a`") {
					edits = append(edits, fix.TextEdit{StartOffset: idx + 1, EndOffset: idx + 2})
				}
			}
			require.Len(t, edits, spans)
			return regionWorkOf(t, input, edits)
		}
		short, long := work(small), work(factor*small)
		t.Logf("bytes read to place edits %d for %d edits, %d for %d", short.placing, small, long.placing, factor*small)
		assert.Less(t, ratio(short.placing, long.placing), 1.5*factor, "placing edits grows faster than the document")
	})
	t.Run("fixes after a long first line with a fix", func(t *testing.T) {
		work := func(n int) int {
			input := longFirstLineDocument(n)
			edits := trailingSpaceEdits(input)
			require.Len(t, edits, n+1)
			total := 0
			for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
				diags := make([]lint.Diagnostic, len(edits))
				kept := make([]int, len(edits))
				for idx, edit := range edits {
					diags[idx] = lint.Diagnostic{RuleID: "MD009", FixEdits: []fix.TextEdit{edit}}
					kept[idx] = idx
				}
				check := newFixCheck(parseForCheckWith(t, flavor, input), flavor)
				got, decided := check.keptInOrder(diags, kept)
				require.Equal(t, kept, got, "every fix keeps the document")
				require.Equal(t, len(kept), decided, "region states decide every step")
				for _, model := range check.models {
					total += model.work.firstLine
				}
			}
			return total
		}
		short, long := work(small), work(factor*small)
		t.Logf("first line bytes read %d for %d fixes, %d for %d", short, small, long, factor*small)
		assert.Less(t, ratio(short, long), 1.5*factor, "first line work grows faster than the document")
	})
}

// longFirstLineDocument returns a document whose first line is a paragraph
// of 8n words that ends in trailing spaces, followed by n paragraphs that
// end in trailing spaces. Once the fix of the first line is kept, a check of
// a later fix need not read the first line again.
func longFirstLineDocument(n int) string {
	var doc strings.Builder
	doc.WriteString(strings.Repeat("word ", 8*n) + "  \n\n")
	for idx := range n {
		fmt.Fprintf(&doc, "Text %d   \n\n", idx)
	}
	return doc.String()
}

// stubOutcome is the outcome of a region in TestDecomposeRegionsMatchesFullMerges,
// which holds or not as a function of the region alone.
type stubOutcome struct {
	reg                region
	holds, cannotCheck bool
}

func (o stubOutcome) verdict() (bool, bool) { return o.holds, o.cannotCheck }

// decomposeWithFullMerges is decomposeRegions as it was before regions grew
// by windows: after each growth it merges all the regions again and goes on
// from the first that changed.
func decomposeWithFullMerges(layout *docLayout, placed []placedEdit, evaluate func(region, []placedEdit) stubOutcome) ([]region, []stubOutcome, bool) {
	regions := make([]region, len(placed))
	for idx, edit := range placed {
		regions[idx] = layout.regionFor(edit)
	}
	regions = layout.mergeRegions(regions)
	outcomes := make([]stubOutcome, 0, len(regions))
	for idx := 0; idx < len(regions); {
		from, to := layout.editSpan(regions[idx])
		outcome := evaluate(regions[idx], regionEdits(from, to, placed))
		if outcome.cannotCheck {
			return nil, nil, false
		}
		if !outcome.holds {
			grown := layout.grow(regions[idx])
			if grown == regions[idx] {
				return nil, nil, false
			}
			before := slices.Clone(regions)
			regions[idx] = grown
			regions = layout.mergeRegions(regions)
			idx = firstDifference(before, regions)
			outcomes = outcomes[:idx]
			continue
		}
		outcomes = append(outcomes, outcome)
		idx++
	}
	return regions, outcomes, true
}

// firstDifference returns the index of the first region that differs
// between two sorted lists, or the length of the shorter.
func firstDifference(a, b []region) int {
	idx := 0
	for idx < len(a) && idx < len(b) && a[idx] == b[idx] {
		idx++
	}
	return idx
}

// TestDecomposeRegionsMatchesFullMerges checks that growing regions by
// windows gives the regions and outcomes that merging all the regions after
// each growth gives, on random edits in random documents with lists, under
// both flavors, with regions that hold as a function of the region alone, so
// that regions grow again and again and lists become group regions.
func TestDecomposeRegionsMatchesFullMerges(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 23))
	gfm, commonMark := recordingInstances()
	checks, growths := 0, 0
	for trial := range 1500 {
		text := randomRegionDocument(rng) + randomRegionDocument(rng) + randomRegionDocument(rng)
		if trial%3 == 0 {
			// Long lists give list regions that grow and become groups.
			text += string(longListDocument(8+rng.IntN(20))) + separateListsDocument(2+rng.IntN(4))
		}
		for _, markdown := range []goldmark.Markdown{gfm, commonMark} {
			layout := newDocLayout(markdown, []byte(text))
			if layout == nil || len(layout.source) == 0 {
				continue
			}
			var edits []fix.TextEdit
			for range 1 + rng.IntN(16) {
				at := rng.IntN(len(layout.source) + 1)
				edits = append(edits, fix.TextEdit{StartOffset: at, EndOffset: min(len(layout.source), at+rng.IntN(4))})
			}
			prepared, err := fix.PrepareEdits(edits, len(layout.source))
			if err != nil {
				continue
			}
			placed := make([]placedEdit, len(prepared))
			for idx, edit := range prepared {
				placed[idx] = layout.placeEdit(edit)
			}
			seed := rng.Uint64()
			stub := func(calls *[]region) func(region, []placedEdit) stubOutcome {
				return func(reg region, _ []placedEdit) stubOutcome {
					*calls = append(*calls, reg)
					key := uint64(reg.list+2)*1000003 + uint64(reg.first)*1009 + uint64(reg.last)*7 + seed
					return stubOutcome{reg: reg, holds: key%5 < 3, cannotCheck: key%50 == 7}
				}
			}
			var wantCalls, gotCalls []region
			wantRegions, wantOutcomes, wantOK := decomposeWithFullMerges(layout, placed, stub(&wantCalls))
			gotRegions, gotOutcomes, gotOK := decomposeRegions(layout, placed, stub(&gotCalls))
			require.Equal(t, wantOK, gotOK, "document %q, edits %v", text, prepared)
			require.Equal(t, wantCalls, gotCalls, "document %q, edits %v", text, prepared)
			if wantOK {
				require.Equal(t, wantRegions, gotRegions, "document %q, edits %v", text, prepared)
				require.Equal(t, wantOutcomes, gotOutcomes, "document %q, edits %v", text, prepared)
			}
			growths += len(gotCalls) - len(gotOutcomes)
			checks++
		}
	}
	t.Logf("%d decompositions, %d regions that did not hold", checks, growths)
	assert.Positive(t, growths)
}

// TestRegionChecksAfterClosedBlocks checks region checks where groups start
// after a thematic break, an ATX heading or a one-line HTML block with no
// blank line before the next block, against the whole-document checks: edits
// that keep those blocks, and edits that break them, so that a paragraph runs
// on into the next, an HTML block or a fence runs to the end, or a thematic
// break becomes a setext underline, where a region must grow.
func TestRegionChecksAfterClosedBlocks(t *testing.T) {
	doc := mixedLeafDocument(12)
	edits := []struct {
		name     string
		old, new string
	}{
		{name: "trailing spaces after a break", old: "Paragraph 5 text   ", new: "Paragraph 5 text"},
		{name: "trailing spaces after a heading", old: "Paragraph 6 text   ", new: "Paragraph 6 text"},
		{name: "trailing spaces after a comment", old: "Paragraph 7 text   ", new: "Paragraph 7 text"},
		{name: "trailing spaces after a fence", old: "Paragraph 8 text   ", new: "Paragraph 8 text"},
		{name: "break removed", old: "line   \n***\nParagraph 5", new: "line   \nParagraph 5"},
		{name: "break broken", old: "***\nParagraph 5", new: "**\nParagraph 5"},
		{name: "break made a setext underline", old: "***\nParagraph 5", new: "---\nParagraph 5"},
		{name: "blank line after a break", old: "***\nParagraph 5", new: "***\n\nParagraph 5"},
		{name: "text after a break", old: "***\nParagraph 5", new: "*** x\nParagraph 5"},
		{name: "heading broken", old: "## Heading 5\n", new: "##Heading 5\n"},
		{name: "heading level", old: "## Heading 5\n", new: "### Heading 5\n"},
		{name: "heading removed", old: "## Heading 5\n", new: ""},
		{name: "heading made a setext underline", old: "## Heading 5\n", new: "===\n"},
		{name: "comment left open", old: "<!-- comment 6 -->", new: "<!-- comment 6"},
		{name: "comment closed later", old: "<!-- comment 6 -->\n", new: "<!-- comment 6\n-->\n"},
		{name: "fence left open", old: "```\ncode 7\n```\n", new: "```\ncode 7\n"},
		{name: "paragraph indented after a break", old: "***\nParagraph 9", new: "***\n    Paragraph 9"},
		{name: "blank line inserted before the next paragraph", old: "## Heading 5\n", new: "## Heading 5\n\n"},
		{name: "break line ending doubled", old: "line   \n***\nParagraph 9", new: "line   \n***\n\nParagraph 9"},
	}
	cases, decided := 0, 0
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		snapshot := parseForCheckWith(t, flavor, doc)
		check := newFixCheck(snapshot, flavor)
		cfg := config.NewConfig()
		cfg.Flavor = flavor
		verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), fixAllowance{})
		model := verifier.regions()
		require.NotNil(t, model)
		for _, tt := range edits {
			cases++
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				change := []fix.TextEdit{replaceOnce(t, doc, tt.old, tt.new)}
				want := check.unchanged(fix.ApplyEdits(snapshot.Content, change))
				ok, got := check.regionsUnchanged(change)
				if ok {
					assert.Equal(t, want, got, "render signature")
				}
				outlineOK, outlineGot := model.checkEdits(change)
				if outlineOK {
					assert.Equal(t, verifier.acceptsWhole(change), outlineGot, "outline")
				}
				if ok && outlineOK {
					decided++
				}
			})
		}
	}
	assert.Equal(t, cases, decided, "both region checks decide every case")
}

// TestRegionChecksOnTablesBeforeSetextHeadings checks region checks on list
// items whose first paragraph becomes a GFM table and a setext heading: the
// table comes first and the heading keeps the blank-line flag of the item's
// first line, which differs where a region starts after a closed block or
// at the item before a list region. Both region checks decide as the
// whole-document checks do.
func TestRegionChecksOnTablesBeforeSetextHeadings(t *testing.T) {
	tests := []struct {
		name, doc, old, new string
	}{
		{"blank line inserted after a break", "# Title\n\n***\n- a\n  b|c\n  -|-\n  ===\n", "***", "***\n"},
		{"blank line after a break removed", "# Title\n\n***\n\n- a\n  b|c\n  -|-\n  ===\n", "***\n", "***"},
		{"trailing spaces in the table", "# Title\n\n***\n- a\n  b|c   \n  -|-\n  ===\n", "b|c   ", "b|c"},
		{"trailing spaces after the item", "# Title\n\n- x\n- a\n  b|c\n  -|-\n  ===\n- y   \n- z\n- w\n- v\n", "- y   ", "- y"},
		{"blank line removed in the list", "# Title\n\n- x\n- a\n  b|c\n  -|-\n  ===\n- y\n\n- z\n- w\n- v\n", "- y\n\n- z", "- y\n- z"},
	}
	for _, tt := range tests {
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			t.Run(tt.name+"/"+string(flavor), func(t *testing.T) {
				snapshot := parseForCheckWith(t, flavor, tt.doc)
				check := newFixCheck(snapshot, flavor)
				change := []fix.TextEdit{replaceOnce(t, tt.doc, tt.old, tt.new)}
				want := check.unchanged(fix.ApplyEdits(snapshot.Content, change))
				ok, got := check.regionsUnchanged(change)
				require.True(t, ok, "region checks decide")
				assert.Equal(t, want, got, "render signature")
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), fixAllowance{})
				model := verifier.regions()
				require.NotNil(t, model)
				ok, got = model.checkEdits(change)
				require.True(t, ok, "outline region checks decide")
				assert.Equal(t, verifier.acceptsWhole(change), got, "outline")
			})
		}
	}
}

// TestGroupRegionsEndAtTheOriginalKind checks that a group region holds only
// when the group after it starts as the original's next group does: after a
// closed block, where its first blocks follow no blank line, or with no
// block open.
func TestGroupRegionsEndAtTheOriginalKind(t *testing.T) {
	input := "# Title\n\n***   \nText\n"
	gfm, _ := recordingInstances()
	layout := newDocLayout(gfm, []byte(input))
	require.NotNil(t, layout)
	require.Equal(t, []bool{false, false, true}, layout.afterClosed)
	reg := region{list: -1, first: 1, last: 1}
	at := strings.Index(input, "***   ") + len("***   ")
	// A blank line after the break makes the next group start with no block
	// open, though the edit does not reach its line.
	assert.False(t, layout.parseGroupRegion(reg, []fix.TextEdit{{StartOffset: at, EndOffset: at, NewText: "\n"}}).holds)
	// Removing the spaces keeps the break and the kind of the next start.
	assert.True(t, layout.parseGroupRegion(reg, []fix.TextEdit{{StartOffset: at - 3, EndOffset: at}}).holds)
}

// TestGroupsStartAfterClosedBlocks checks where docLayout's groups start
// when blocks follow one another with no blank line: after a top-level
// thematic break, ATX heading or one-line HTML block, and after a closed
// fence, but not after a setext heading, an indented code block, an HTML
// block that runs on, or a closed block inside a container.
func TestGroupsStartAfterClosedBlocks(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		starts      []string
		afterClosed []bool
	}{
		{"thematic break", "a\n***\nb\n", []string{"a", "b"}, []bool{false, true}},
		{"ATX heading", "# H\nb\n", []string{"# H", "b"}, []bool{false, true}},
		{"one-line HTML block", "<!-- c -->\nb\n", []string{"<!-- c -->", "b"}, []bool{false, true}},
		{"one-line script", "<script>x</script>\nb\n", []string{"<script>x</script>", "b"}, []bool{false, true}},
		{"one-line processing instruction", "<?php x ?>\nb\n", []string{"<?php x ?>", "b"}, []bool{false, true}},
		{"one-line declaration", "<!DOCTYPE html>\nb\n", []string{"<!DOCTYPE html>", "b"}, []bool{false, true}},
		{"one-line CDATA", "<![CDATA[ x ]]>\nb\n", []string{"<![CDATA[ x ]]>", "b"}, []bool{false, true}},
		{"thematic break with CRLF", "a\r\n***\r\nb\r\n", []string{"a\r", "b\r"}, []bool{false, true}},
		{"break before a blank line", "a\n***\n\nb\n", []string{"a", "b"}, []bool{false, false}},
		{"closed fence", "```\nx\n```\nb\n", []string{"```", "b"}, []bool{false, false}},
		{"heading after a paragraph", "a\n# H\nb\n", []string{"a", "b"}, []bool{false, true}},
		{"break after a list", "- a\n***\nb\n", []string{"- a", "b"}, []bool{false, true}},
		{"setext heading", "H\n---\nb\n", []string{"H"}, []bool{false}},
		{"indented code", "    x\nb\n", []string{"    x"}, []bool{false}},
		{"HTML block that runs on", "<div>\nx\n</div>\nb\n", []string{"<div>"}, []bool{false}},
		{"heading in a quote", "> # H\nb\n", []string{"> # H"}, []bool{false}},
	}
	gfm, commonMark := recordingInstances()
	markdowns := []struct {
		name     string
		markdown goldmark.Markdown
	}{{"gfm", gfm}, {"commonmark", commonMark}}
	for _, tt := range tests {
		for _, md := range markdowns {
			t.Run(tt.name+"/"+md.name, func(t *testing.T) {
				layout := newDocLayout(md.markdown, []byte(tt.input))
				require.NotNil(t, layout)
				var starts []string
				for _, offset := range layout.bounds[:len(layout.bounds)-1] {
					line, _, _ := strings.Cut(tt.input[offset:], "\n")
					starts = append(starts, line)
				}
				assert.Equal(t, tt.starts, starts)
				assert.Equal(t, tt.afterClosed, layout.afterClosed)
			})
		}
	}
}

// TestPlaceEditsFindsTheirLines checks that placing sorted edits each from
// the one before it finds the lines that searching back from each finds.
func TestPlaceEditsFindsTheirLines(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 6))
	checked := 0
	for range 300 {
		text := randomRegionDocument(rng)
		if text == "" || strings.HasPrefix(text, "---") || strings.HasPrefix(text, "\xef\xbb\xbf") {
			continue
		}
		layout := newDocLayout(regionMarkdowns(config.FlavorGFM)[0], []byte(text))
		if layout == nil {
			continue
		}
		edits, err := fix.PrepareEdits(randomRegionEdits(rng, text, false), len(text))
		if err != nil {
			continue
		}
		placed, _, ok := layout.placeEdits(edits)
		if !ok {
			continue
		}
		for _, edit := range placed {
			last := edit.edit.StartOffset
			if edit.edit.EndOffset > edit.edit.StartOffset {
				last = edit.edit.EndOffset - 1
				if layout.source[last] == '\n' {
					last = edit.edit.EndOffset
				}
			}
			require.Equal(t, lineStartIn(layout.source, edit.edit.StartOffset), edit.firstLine, "text %q, edit %+v", text, edit.edit)
			require.Equal(t, lineStartIn(layout.source, last), edit.lastLine, "text %q, edit %+v", text, edit.edit)
			checked++
		}
	}
	assert.Greater(t, checked, 300, "many edits are placed")
}

// randomHeadEdits returns sorted edits that do not overlap, at random places
// of the first lines of text (before limit): insertions, deletions and
// replacements, of texts that make or break front matter openers and line
// endings.
func randomHeadEdits(rng *rand.Rand, layout *docLayout, limit int, texts []string) []placedEdit {
	var edits []placedEdit
	pos := 0
	for range rng.IntN(4) {
		if pos > limit {
			break
		}
		start := pos + rng.IntN(limit-pos+1)
		end := min(len(layout.source), start+rng.IntN(3))
		edit := fix.TextEdit{StartOffset: start, EndOffset: end, NewText: texts[rng.IntN(len(texts))]}
		edits = append(edits, layout.placeEdit(edit))
		pos = end + rng.IntN(2)
		if end == start {
			pos = end + 1
		}
	}
	return edits
}

// TestFirstLineEndBoundsTheFirstLine checks the claim that tryAdd's skip
// rests on: firstLineKeeps reads no edit that starts after firstLineEnd of
// the other edits, so with such edits it decides as without them. Edits
// that start right after the bound, and replacements whose text holds the
// line ending, which edits at their start come before, are made often.
func TestFirstLineEndBoundsTheFirstLine(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 8))
	lines := []string{"-", "--", "  ---", "", "x", "{", "+++", "# t", "  "}
	texts := []string{"", "-", "--", "---", "+", "{", "x", " ", "\n", "x\n", "---\n", "\n-", "\xef\xbb\xbf"}
	checked := 0
	for range 4000 {
		var doc strings.Builder
		for range 2 + rng.IntN(4) {
			doc.WriteString(lines[rng.IntN(len(lines))] + "\n")
		}
		head := doc.Len()
		doc.WriteString("\ntext\n")
		layout := newDocLayout(regionMarkdowns(config.FlavorGFM)[0], []byte(doc.String()))
		if layout == nil || !layout.firstLineFree {
			continue
		}
		state := randomHeadEdits(rng, layout, head, texts)
		end := layout.firstLineEnd(state)
		var more []placedEdit
		at := end + 1 + rng.IntN(2)
		for range 1 + rng.IntN(2) {
			if at > len(layout.source) {
				break
			}
			edit := fix.TextEdit{StartOffset: at, EndOffset: at, NewText: texts[rng.IntN(len(texts))]}
			clash := false
			for _, other := range state {
				inside := edit.StartOffset > other.edit.StartOffset && edit.StartOffset < other.edit.EndOffset
				sameRange := other.edit.StartOffset == at && other.edit.EndOffset == at
				clash = clash || inside || sameRange
			}
			if !clash {
				more = append(more, layout.placeEdit(edit))
			}
			at += 1 + rng.IntN(3)
		}
		if len(more) == 0 {
			continue
		}
		require.Equal(t, layout.firstLineKeeps(state, nil), layout.firstLineKeeps(state, more),
			"source %q, edits %+v, more %+v, end %d", layout.source, state, more, end)
		checked++
	}
	assert.Greater(t, checked, 2000, "most cases are checked")
}

// TestFirstLineKeepsOnALongLine checks that clearing edits on the first line
// takes time that grows with the line, not with its square: a document can
// be one long line.
func TestFirstLineKeepsOnALongLine(t *testing.T) {
	content := []byte(strings.Repeat("word ", 1<<18) + "\n\nnext\n")
	layout := newDocLayout(regionMarkdowns(config.FlavorGFM)[0], content)
	require.NotNil(t, layout)
	require.True(t, layout.firstLineFree)
	start := time.Now()
	edits := []placedEdit{layout.placeEdit(fix.TextEdit{StartOffset: 4, EndOffset: 5, NewText: " "})}
	assert.True(t, layout.firstLineKeeps(edits, nil))
	assert.False(t, layout.firstLineKeeps([]placedEdit{layout.placeEdit(fix.TextEdit{
		StartOffset: 0, EndOffset: len(content) - len("\n\nnext\n"), NewText: "---",
	})}, nil), "a first line made an opener")
	assert.Less(t, time.Since(start), time.Second)
}

// TestRegionParsesShareDefinitions checks that a region parse resolves links
// through the document's definitions without adding them all again, which
// made the checks of a document with many definitions and many fixes take
// time that grows with their product.
func TestRegionParsesShareDefinitions(t *testing.T) {
	const definitions, paragraphs = 5000, 2000
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range definitions {
		fmt.Fprintf(&doc, "[label %d]: /target/%d\n", idx, idx)
	}
	for idx := range paragraphs {
		fmt.Fprintf(&doc, "\nParagraph %d links to [label %d].\n", idx, idx)
	}
	layout := newDocLayout(regionMarkdowns(config.FlavorGFM)[0], []byte(doc.String()))
	require.NotNil(t, layout)
	groups := len(layout.bounds) - 1
	require.Greater(t, groups, paragraphs)
	start := time.Now()
	for group := groups - paragraphs; group < groups; group++ {
		parse := layout.parseRegion(region{list: -1, first: group, last: group}, nil)
		require.True(t, parse.holds)
		require.True(t, parse.refsSame)
		require.Len(t, parse.blocks, 1)
		link, ok := parse.blocks[0].FirstChild().NextSibling().(*gmast.Link)
		require.True(t, ok, "the paragraph links")
		assert.Equal(t, fmt.Sprintf("/target/%d", group-(groups-paragraphs)), string(link.Destination))
	}
	assert.Less(t, time.Since(start), time.Second)
}

// TestRegionOutcomeCacheIsBounded checks that the outcomes a model caches
// stay within a bound that follows the document, even when every check
// covers a block the size of the document, as a long blockquote is.
func TestRegionOutcomeCacheIsBounded(t *testing.T) {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range 2000 {
		fmt.Fprintf(&doc, "> quoted line %d\n", idx)
	}
	input := doc.String()
	check := newFixCheck(parseForCheck(t, input), config.FlavorGFM)
	check.ensureModels()
	model := check.models[0]
	require.NotNil(t, model)
	for idx := range 500 {
		// Each edit changes the text of another line, so no check reuses
		// another's outcome.
		at := strings.Index(input, fmt.Sprintf("line %d\n", idx))
		decided, unchanged := model.checkEdits([]fix.TextEdit{{StartOffset: at, EndOffset: at + 4, NewText: "LINE"}})
		require.True(t, decided)
		require.False(t, unchanged)
	}
	cached := 0
	for key, outcome := range model.outcomes {
		cached += len(key)
		if !outcome.same {
			cached += len(outcome.text)
		}
	}
	assert.LessOrEqual(t, cached, 8*len(model.sig)+1<<20)
}

// TestRegionModelMatchesSignature checks that the recording parser gives
// the signature of the plain parser on every snippet, so that region checks
// are used for them.
func TestRegionModelMatchesSignature(t *testing.T) {
	for _, snippet := range regionSnippets {
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			check := newFixCheck(parseForCheckWith(t, flavor, snippet), flavor)
			check.ensureModels()
			for idx, model := range check.models {
				require.NotNil(t, model, "%q with parser %d", snippet, idx)
				assert.Equal(t, check.baseline[idx], model.sig)
			}
		}
	}
}

// TestReplacementsKeepTheirOrder checks that replacements that start at one
// offset are decided in order, an empty one first, however they come: the
// outline part of a group of definitions alone is empty and starts where the
// next part does, and a list token can be added after it. Sorted by start
// alone, more than twelve replacements could put the token first and make
// the comparison slice backwards.
func TestReplacementsKeepTheirOrder(t *testing.T) {
	input := "# Title\n\n[a]: /x   \n\nB0 text   \n\n- i0\n- i1\n- i2\n- i3\n- i4\n- i5\n- i6\n\nAfter the list.\n\n" +
		"A0 text   \n\nA1 text   \n\nA2 text   \n\nA3 text   \n\nA4 text   \n\nA5 text   \n\nA6 text   \n\nA7 text   \n\nPZ textX\n"
	edits := trailingSpaceEdits(input)
	at := strings.Index(input, "- i3")
	edits = append(edits, fix.TextEdit{StartOffset: at, EndOffset: at, NewText: "\n"})
	at = strings.Index(input, "textX") + len("text")
	edits = append(edits, fix.TextEdit{StartOffset: at, EndOffset: at + 1})
	prepared, err := fix.PrepareEdits(edits, len(input))
	require.NoError(t, err)
	snapshot := parseForCheckWith(t, config.FlavorGFM, input)
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM
	verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), fixAllowance{})
	model := verifier.regions()
	require.NotNil(t, model)
	decided, unchanged := model.checkEdits(prepared)
	require.True(t, decided)
	assert.Equal(t, verifier.acceptsWhole(prepared), unchanged)
	assert.False(t, unchanged, "the list turns loose")

	// The decision does not depend on the order replacements come in.
	orig := strings.Repeat("abcdefghij", 4)
	reps := []replacement{{start: 5, end: 5, text: "Q"}, {start: 5, end: 9, text: "fghi"}}
	for idx := range 14 {
		reps = append(reps, replacement{start: 10 + 2*idx, end: 11 + 2*idx, text: orig[10+2*idx : 11+2*idx]})
	}
	_, want := decideReplacements(orig, slices.Clone(reps))
	rng := rand.New(rand.NewPCG(20261018, 5))
	for range 200 {
		shuffled := slices.Clone(reps)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		_, got := decideReplacements(orig, shuffled)
		require.Equal(t, want, got)
	}
}

func TestSameAfterReplacements(t *testing.T) {
	const orig = "P(a)P(b)P(c)"
	tests := []struct {
		name string
		reps []replacement
		want bool
	}{
		{name: "no replacement", want: true},
		{name: "same text", reps: []replacement{{start: 4, end: 8, text: "P(b)"}}, want: true},
		{name: "other text", reps: []replacement{{start: 4, end: 8, text: "P(x)"}}, want: false},
		{name: "longer text", reps: []replacement{{start: 4, end: 8, text: "P(b)P(x)"}}, want: false},
		{
			// A paragraph moves from the first region to the second.
			name: "shifts that make up for each other",
			reps: []replacement{{start: 0, end: 4, text: "P(a)P(b)"}, {start: 8, end: 12, text: ""}},
			want: false,
		},
		{
			name: "shifts that give the same text",
			reps: []replacement{{start: 0, end: 4, text: ""}, {start: 12, end: 12, text: "P(c)"}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replaced := orig
			for idx := len(tt.reps) - 1; idx >= 0; idx-- {
				rep := tt.reps[idx]
				replaced = replaced[:rep.start] + rep.text + replaced[rep.end:]
			}
			assert.Equal(t, tt.want, replaced == orig, "the test case itself")
			assert.Equal(t, tt.want, sameAfterReplacements(orig, tt.reps))
		})
	}

	t.Run("periodic text that compensates", func(t *testing.T) {
		const periodic = "P(x)P(x)P(x)P(x)"
		// Removing the first paragraph and adding one at the end gives the
		// same signature.
		reps := []replacement{{start: 0, end: 4, text: ""}, {start: 16, end: 16, text: "P(x)"}}
		assert.True(t, sameAfterReplacements(periodic, reps))
	})
}

func TestFrontMatterLimit(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{name: "no front matter", content: "# A\n\ntext\n", want: 4},
		{name: "front matter", content: "---\na: b\n---\ntext\n", want: 13},
		{name: "byte order mark", content: "\xef\xbb\xbf# A\ntext\n", want: 7},
		{name: "unclosed front matter", content: "---\na: b\n\ntext\n", want: 16},
		{name: "one line", content: "text", want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := 0
			if strings.HasPrefix(tt.content, "\xef\xbb\xbf") {
				base = 3
			}
			assert.Equal(t, tt.want, frontMatterLimit([]byte(tt.content), base, frontMatterLength([]byte(tt.content)[base:])))
		})
	}
}
