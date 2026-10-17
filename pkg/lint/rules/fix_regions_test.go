package rules

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// regionSnippets are pieces of Markdown whose parse reaches past their own
// lines, or that the fixing rules change: front matter, reference
// definitions and links through them, headings with repeated anchors and
// fragment links, raw HTML blocks of every kind, tight and loose lists,
// nested lists, lazy continuation lines, fences and HTML blocks left open,
// setext headings, tables, tabs and trailing spaces.
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
	for _, doc := range []string{tight, loose, ordered} {
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			snapshot := parseForCheckWith(t, flavor, doc)
			check := newFixCheck(snapshot, flavor)
			for _, tt := range edits {
				if strings.Count(doc, tt.old) != 1 {
					continue
				}
				edit := replaceOnce(t, doc, tt.old, tt.new)
				want := check.unchanged(fix.ApplyEdits(snapshot.Content, []fix.TextEdit{edit}))
				decided, got := check.regionsUnchanged([]fix.TextEdit{edit})
				if decided {
					assert.Equal(t, want, got, "%s in %q", tt.name, doc)
				}
			}
		}
	}
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
