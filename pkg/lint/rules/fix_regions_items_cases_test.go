package rules

import (
	"context"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// lineAt returns the line of text that starts at offset, without its line
// ending.
func lineAt(text string, offset int) string {
	line, _, _ := strings.Cut(text[offset:], "\n")
	return line
}

// TestItemGroupsStart checks where docLayout's groups of a list item start:
// on the marker line, after a blank line, after a block that closes alone,
// which it marks, and after a closed fence, whose next block follows no
// blank line, in a list at the top level or in a blockquote; but not where a
// block stays open, as a nested list, a paragraph that a block interrupts or
// a setext heading does.
func TestItemGroupsStart(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		starts      []string
		afterClosed []bool
		blank       []bool
	}{
		{"blank lines", "- a\n\n  b\n\n  c\n", []string{"- a", "  b", "  c"}, []bool{false, false, false}, []bool{true, true, true}},
		{"thematic break", "- a\n\n  ***\n  b\n", []string{"- a", "  ***", "  b"}, []bool{false, false, true}, []bool{true, true, false}},
		{"ATX heading", "- a\n\n  # H\n  b\n", []string{"- a", "  # H", "  b"}, []bool{false, false, true}, []bool{true, true, false}},
		{"closed fence", "- a\n\n  ```\n  x\n  ```\n  b\n", []string{"- a", "  ```", "  b"}, []bool{false, false, false}, []bool{true, true, false}},
		{"lazy line", "- a\nlazy\n\n  b\n", []string{"- a", "  b"}, []bool{false, false}, []bool{true, true}},
		{"in a blockquote", "> - a\n>\n>   b\n", []string{"> - a", ">   b"}, []bool{false, false}, []bool{true, true}},
		{"interrupted paragraph", "- a\n  # H\n", []string{"- a"}, nil, nil},
		{"nested list stays open", "- a\n  - b\n\n  c\n", []string{"- a"}, nil, nil},
		{"setext heading", "- H\n  ---\n  b\n", []string{"- H"}, nil, nil},
	}
	gfm, commonMark := recordingInstances()
	for _, tt := range tests {
		for _, md := range []struct {
			name     string
			markdown goldmark.Markdown
		}{{"gfm", gfm}, {"commonmark", commonMark}} {
			t.Run(tt.name+"/"+md.name, func(t *testing.T) {
				layout := newDocLayout(md.markdown, []byte(tt.input))
				require.NotNil(t, layout)
				require.NotEmpty(t, layout.items)
				item := layout.items[0]
				var starts []string
				for _, offset := range item.bounds {
					starts = append(starts, lineAt(tt.input, offset))
				}
				assert.Equal(t, tt.starts, starts)
				assert.Equal(t, len(tt.starts) > 1, item.regions)
				if item.regions {
					assert.Equal(t, tt.afterClosed, item.afterClosed)
					assert.Equal(t, tt.blank, item.blank)
				}
			})
		}
	}
}

// TestItemPrefixes checks the prefix that opens an item with the lists and
// items around it: its marker line up to its content, after the blockquote
// marker of a list in a blockquote and after the prefix of the item that
// holds a nested list, and none for an item whose content starts on a later
// line or whose marker line holds a tab before its content.
func TestItemPrefixes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		item   int
		prefix string
	}{
		{"bullet", "- a\n", 0, "- "},
		{"wide marker", "-   a\n", 0, "-   "},
		{"ordered", "10) a\n", 0, "10) "},
		{"indented list", "  * a\n", 0, "  * "},
		{"in a blockquote", "> - a\n", 0, "> - "},
		{"nested", "- a\n\n  1. b\n", 1, "- 1. "},
		{"nested on the marker line", "- - a\n", 1, "- - "},
		{"nested in a blockquote", "> - a\n>\n>   - b\n", 1, "> - - "},
		{"content on a later line", "-\n  a\n", 0, ""},
		{"tab", "-\ta\n", 0, ""},
	}
	gfm, _ := recordingInstances()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := newDocLayout(gfm, []byte(tt.input))
			require.NotNil(t, layout)
			require.Greater(t, len(layout.items), tt.item)
			assert.Equal(t, tt.prefix, string(layout.items[tt.item].prefix))
		})
	}
}

// TestItemOpeners checks the opener of an item region that starts at a group
// of an item: the rest of the marker line when that is the line before the
// group, an empty ATX heading before a blank line or a block that closes
// alone, a fence that the line before closes after a closed fence, and the
// opening of an HTML block that the line before closes, of the kind whose
// opener holds.
func TestItemOpeners(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		group  int
		opener string
	}{
		{"after the marker line", "- # H\n  b\n\n  c\n", 1, "- # H\n"},
		{"after a blank line", "- a\n\n  b\n\n  c\n", 1, "- #\n\n"},
		{"after a heading", "- a\n\n  # H\n  b\n\n  c\n", 2, "- #\n  # H\n"},
		{"after a closed fence", "- a\n  ```\n  x\n  ```\n  b\n\n  c\n", 1, "- ```\n  ```\n"},
		{"in a blockquote", "> - a\n>\n>   b\n>\n>   c\n", 1, "> - #\n>\n"},
		{"after a comment", "- a\n  <!--\n  x\n  -->\n  b\n\n  c\n", 1, "- <!--\n  -->\n"},
		{"after a processing instruction", "- a\n  <?\n  x\n  ?>\n  b\n\n  c\n", 1, "- <?\n  ?>\n"},
		{"after a pre block", "- a\n  <pre>\n  x\n  </PRE>\n  b\n\n  c\n", 1, "- <pre>\n  </PRE>\n"},
	}
	gfm, _ := recordingInstances()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := newDocLayout(gfm, []byte(tt.input))
			require.NotNil(t, layout)
			item := layout.items[0]
			require.True(t, item.regions)
			require.Less(t, tt.group, len(item.bounds))
			opener := layout.itemOpener(0, tt.group)
			assert.Equal(t, tt.opener, string(opener.text), "opener of the group at %q", lineAt(tt.input, item.bounds[tt.group]))
		})
	}
}

// TestKeptInOrderMatchesWholeDocumentInItems checks, as
// TestKeptInOrderMatchesWholeDocument does, that keeping fixes one at a time
// with region states keeps the fixes that checking the whole document at
// each step keeps, on documents of lists whose items hold many groups, with
// blank lines inserted and random edits, so that item regions join and leave
// the states and change the tightness of their lists.
func TestKeptInOrderMatchesWholeDocumentInItems(t *testing.T) {
	documents := 150
	if testing.Short() || raceEnabled {
		documents = 30
	}
	rng := rand.New(rand.NewPCG(20261025, 88))
	steps, decided := 0, 0
	for tested := 0; tested < documents; {
		text := randomItemDocument(rng)
		// Documents whose raw HTML is not benign keep no fix at all.
		if !newFixCheck(parseForCheck(t, text), config.FlavorGFM).htmlBenign() {
			continue
		}
		tested++
		validator := fix.NewValidator([]byte(text))
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			snapshot := parseForCheckWith(t, flavor, text)
			for trial := range 3 {
				var diags []lint.Diagnostic
				var kept []int
				for range 3 + rng.IntN(8) {
					edits := []fix.TextEdit{blankLineEdit(rng, text)}
					if trial == 2 {
						edits = randomRegionEdits(rng, text, rng.IntN(2) == 0)
					}
					if _, err := validator.Validate(edits); err != nil || len(edits) == 0 {
						continue
					}
					diags = append(diags, lint.Diagnostic{RuleID: "T1", FixEdits: checkedEdits(edits)})
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

// TestItemRegionChecks checks fixes inside list items with both region
// checks against the whole-document checks, and that item regions decide
// them: blank lines that loosen a tight list or leave a loose one as it is,
// in items at the top level, in a blockquote and nested in another item,
// edits after a closed fence, an edit that takes a line out of its item, a
// definition that the line after the region starts, and a table that text
// runs into.
func TestItemRegionChecks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// The edit replaces remove bytes at the first at with text.
		at      string
		remove  int
		text    string
		changed bool
		// tables reports a case about a table, which CommonMark reads as
		// a paragraph that a blank line splits.
		tables bool
	}{
		{"blank line in a loose item", "- a\n\n  b\n  ***\n  c\n\n  d\n- e\n", "  c", 0, "\n", false, false},
		{"blank line that loosens a tight list", "- a\n  ***\n  b\n  ***\n  c\n- d\n", "  b", 0, "\n", true, false},
		{"blank line after a closed fence", "- a\n\n  t\n  ```\n  x\n  ```\n  b\n\n  c\n", "  b", 0, "\n", false, false},
		{"trailing spaces in a quoted item", "> - a\n>\n>   b   \n>\n>   c\n>\n>   d\n", "   \n>\n>   c", 3, "", false, false},
		{"nested item", "- a\n\n  - b\n\n    c\n\n    d   \n\n    e\n\n  - f\n", "   \n\n    e", 3, "", false, false},
		{"line taken out of its item", "- a\n\n  b\n\n  c\n\n  d\n", "  c", 0, "\n\nx", true, false},
		{"definitions after the region", "- a\n\n  b   \n\n  [r]: /u\n\n  [r]\n", "   \n\n  [r]:", 3, "", false, false},
		{"table that text runs into", "- a\n\n  text\n  | a | b |\n  | - | - |\n\n  more\n", "  | a | b |", 0, "\n", false, true},
	}
	for _, tt := range tests {
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			t.Run(tt.name+"/"+string(flavor), func(t *testing.T) {
				at := strings.Index(tt.input, tt.at)
				require.GreaterOrEqual(t, at, 0)
				edits := []fix.TextEdit{{StartOffset: at, EndOffset: at + tt.remove, NewText: tt.text}}
				checks := newBlockChecks(t, flavor, tt.input, fixAllowance{})
				require.NotNil(t, checks.outline)
				ok, outlineOK := checks.decide(t, edits, tt.name)
				require.True(t, ok)
				require.True(t, outlineOK)
				_, got := checks.check.regionsUnchanged(edits)
				assert.Equal(t, !tt.changed && (!tt.tables || flavor == config.FlavorGFM), got, "render signature")
				assert.Positive(t, checks.kinds()[itemRegion], "item regions decide the edit")
			})
		}
	}
}

// TestDecomposedRegionsDoNotOverlap checks that the regions a check needs
// stay apart when an item region grows into the list region of its item,
// and that list region meets an item region of the next item: their union
// does not fit, so the region that stands for it holds the whole list,
// with the list region of another item that already held.
func TestDecomposedRegionsDoNotOverlap(t *testing.T) {
	input := "# T\n\n- a\n- b   \n- c\n- d\n- e\n\n  p1\n\n  p2\n- f\n\n  q1   \n\n  q2\n- g\n"
	snapshot := parseForCheckWith(t, config.FlavorGFM, input)
	check := newFixCheck(snapshot, config.FlavorGFM)
	check.ensureModels()
	model := check.models[0]
	require.NotNil(t, model)
	var edits []fix.TextEdit
	for _, at := range []string{"   \n- c", "\n\n  p2", "   \n\n  q2"} {
		start := strings.Index(input, at)
		require.GreaterOrEqual(t, start, 0)
		end := start + 3
		if at[0] == '\n' {
			end = start + 1
		}
		edits = append(edits, fix.TextEdit{StartOffset: start, EndOffset: end})
	}
	placed, _, ok := model.placeEdits(edits)
	require.True(t, ok)
	regions, _, ok := model.decompose(placed)
	require.True(t, ok)
	for idx := 1; idx < len(regions); idx++ {
		_, prevEnd := model.span(regions[idx-1])
		start, _ := model.span(regions[idx])
		assert.LessOrEqual(t, prevEnd, start, "regions %v and %v overlap", regions[idx-1], regions[idx])
	}
	checks := newBlockChecks(t, config.FlavorGFM, input, fixAllowance{})
	prepared, err := fix.PrepareEdits(edits, len(input))
	require.NoError(t, err)
	checks.decide(t, prepared, "growth past a list region")
}

// TestItemWindowsKeepRuleFixes lints random documents of lists, whose items
// hold headings, emphasis and thematic breaks that the heading, emphasis and
// horizontal-rule rules fix, with item windows for every fix and with none,
// and checks that every rule reports the same findings with the same fixes:
// the item window decides with the intents the rules give their fixes as
// the window of the blocks around a fix does.
func TestItemWindowsKeepRuleFixes(t *testing.T) {
	documents := 150
	if testing.Short() || raceEnabled {
		documents = 30
	}
	saved := itemWindowMin
	t.Cleanup(func() { itemWindowMin = saved })
	extra := []string{
		"#Heading without a space\n", "## Heading with punctuation.\n", "**Emphasis as a heading**\n",
		"* * *\n", "Text *with  emphasis* here\n", "  ## Indented heading\n", "##  Two spaces\n",
	}
	rng := rand.New(rand.NewPCG(20261025, 22))
	registry := lint.NewRegistry()
	RegisterAll(registry)
	compared, fixes := 0, 0
	for range documents {
		text := randomItemDocument(rng)
		// Put rule findings into items, indented to the first item's content.
		if at := strings.Index(text, "\n\n"); at >= 0 && rng.IntN(2) == 0 {
			text = text[:at] + "\n\n" + extra[rng.IntN(len(extra))] + text[at+2:]
		}
		lines := strings.SplitAfter(text, "\n")
		for idx := range lines {
			if idx > 0 && strings.HasPrefix(lines[idx], "  ") && rng.IntN(3) == 0 {
				lines[idx] = "  " + extra[rng.IntN(len(extra))] + lines[idx]
			}
		}
		text = strings.Join(lines, "")
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			engine := lint.NewEngine(gmparser.New(string(flavor)), registry)
			lintWith := func(threshold int) []lint.Diagnostic {
				itemWindowMin = threshold
				result, err := engine.LintFile(context.Background(), "doc.md", []byte(text), cfg)
				require.NoError(t, err)
				return result.Diagnostics
			}
			narrow, wide := lintWith(0), lintWith(math.MaxInt)
			require.Len(t, narrow, len(wide), "flavor %s, document %q", flavor, text)
			for idx := range wide {
				require.Equal(t, wide[idx].RuleID, narrow[idx].RuleID)
				require.Equal(t, wide[idx].FixEdits, narrow[idx].FixEdits, "flavor %s, %s at line %d, document %q",
					flavor, wide[idx].RuleID, wide[idx].StartLine, text)
				if len(wide[idx].FixEdits) > 0 {
					fixes++
				}
			}
			compared += len(wide)
		}
	}
	t.Logf("%d findings compared, %d with fixes", compared, fixes)
	assert.Positive(t, fixes)
}

// TestItemWindowChecks checks the heading rules' item window against the
// whole-document check: a blank line in a loose item keeps the document, one
// that loosens a tight list does not, and a list that a table after a blank
// line makes loose, as gomdlint's parser reads it, stays loose, so a blank
// line in it keeps the document.
func TestItemWindowChecks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		at    string
		kept  bool
	}{
		{"loose item", "- a\n\n  b\n  ***\n  c\n\n  d\n", "  c", true},
		{"tight list", "- a\n  ***\n  b\n  ***\n  c\n- d\n", "  b", false},
		{"loose by a table", "2) a\n   ```\n   ```\n   <x-tag>\n\n   | h1 | h2 |\n   | -- | -- |\n", "   <x-tag>", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := strings.Index(tt.input, tt.at)
			require.GreaterOrEqual(t, at, 0)
			edits := []fix.TextEdit{{StartOffset: at, EndOffset: at, NewText: "\n"}}
			snapshot := parseForCheckWith(t, config.FlavorGFM, tt.input)
			checker := newFixChecker(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
			intents := []fixIntent{{}}
			require.Equal(t, tt.kept, checker.documentKept(edits, intents), "whole document")
			window, decided, kept := checker.itemWindow(edits, intents)
			require.True(t, decided, "the item window decides")
			if kept {
				kept = slices.EqualFunc(window.want, window.after, sameBlock)
			}
			assert.Equal(t, tt.kept, kept)
		})
	}
}
