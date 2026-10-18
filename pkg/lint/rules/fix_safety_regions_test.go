package rules

import (
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// outlineSnippets carry what the code, link, reference and table rules fix:
// bare URLs, padded link text and code spans, reversed links, shell prompts,
// fences without info strings or blank lines around them, tilde fences,
// tables, references and headings with links.
var outlineSnippets = []string{
	"Visit https://example.com/path and www.example.com now.\n",
	"See [ padded link ](https://example.com) here.\n",
	"Run ` padded code` and `code ` here.\n",
	"A (reversed)[https://example.com] link.\n",
	"```\n$ echo hi\n$ ls\n```\n",
	"```bash\n$ date\nSat Oct 17\n```\n",
	"~~~\ntilde fence\n~~~\n",
	"Text before\n```\ncode\n```\nText after\n",
	"| a | b |\n|---|---|\n| 1 | 2 |\n",
	"|a|b|\n|-|-|\n|1|2|\n",
	"[link][missing] and [gone][]\n",
	"[frag](#heading-one) and [other](#title)\n",
	"## Heading with https://example.com/h link\n",
	"## Heading with [ padded ](https://example.com/p)\n",
	"- Item with https://example.com/i and ` code`\n- Item two [ x ](https://example.com/x)\n- Item three\n- Item four\n",
	"- item\n\n  ```\n  $ make\n  ```\n\n- next\n\n  ```\n  $ make test\n  ```\n\n- last\n- end\n",
	// Headings whose text gives no anchor, an empty slug, slugs that collide
	// with numbered anchors, and headings in list items and quotes.
	"#\n",
	"# ![a](i.png)\n",
	"## !!!\n",
	"# Heading one 1\n",
	"- ## Heading in item ` code`\n- second\n- third\n- fourth\n",
	"> ## Quoted heading https://example.com/q\n",
}

// outlineEditPatterns find the places the code and link rules edit.
var outlineEditPatterns = []*regexp.Regexp{
	regexp.MustCompile(`https?://[^\s)>\]]+`),
	regexp.MustCompile(`\[ |  ?\]\(`),
	regexp.MustCompile("` | `"),
	regexp.MustCompile(`(?m)^\s*\$ `),
	regexp.MustCompile("(?m)^\\s*(```|~~~)"),
}

// randomOutlineEdits returns the edits of a fix like those the code and link
// rules make, or random edits.
func randomOutlineEdits(rng *rand.Rand, text string) []fix.TextEdit {
	pattern := outlineEditPatterns[rng.IntN(len(outlineEditPatterns))]
	matches := pattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 || rng.IntN(4) == 0 {
		return randomRegionEdits(rng, text, rng.IntN(2) == 0)
	}
	match := matches[rng.IntN(len(matches))]
	found := text[match[0]:match[1]]
	switch {
	case strings.HasPrefix(found, "http"):
		// Wrap a bare URL in angle brackets.
		return []fix.TextEdit{
			{StartOffset: match[0], EndOffset: match[0], NewText: "<"},
			{StartOffset: match[1], EndOffset: match[1], NewText: ">"},
		}
	case strings.HasSuffix(found, "$ "):
		return []fix.TextEdit{{StartOffset: match[1] - 2, EndOffset: match[1]}}
	case strings.HasSuffix(found, "```") || strings.HasSuffix(found, "~~~"):
		if rng.IntN(2) == 0 {
			return []fix.TextEdit{{StartOffset: match[1], EndOffset: match[1], NewText: "bash"}}
		}
		return []fix.TextEdit{{StartOffset: match[1] - 3, EndOffset: match[1], NewText: "```"}}
	default:
		// Remove the space that pads link text or a code span.
		at := match[0] + strings.IndexByte(found, ' ')
		return []fix.TextEdit{{StartOffset: at, EndOffset: at + 1}}
	}
}

// outlineAllowances are the allowances the rules give their checks, and all
// of them together.
var outlineAllowances = []fixAllowance{
	{}, {linkTextSpace: true}, {codeSpanSpace: true}, {autolinks: true},
	{codeInfo: true}, {prompts: true}, {reversedLinks: true},
	{linkTextSpace: true, codeSpanSpace: true, autolinks: true, codeInfo: true, prompts: true, reversedLinks: true},
}

// TestOutlineRegionsDecideAsWholeDocument checks the region checks of the
// outline against parsing the whole document: on random documents and
// edits, every decision region checks make equals the whole-document
// decision.
func TestOutlineRegionsDecideAsWholeDocument(t *testing.T) {
	documents := 900
	if testing.Short() || raceEnabled {
		documents = 120
	}
	rng := rand.New(rand.NewPCG(20261017, 41))
	total, decided, kept := 0, 0, 0
	for range documents {
		var doc strings.Builder
		for range 2 + rng.IntN(8) {
			if rng.IntN(2) == 0 {
				doc.WriteString(outlineSnippets[rng.IntN(len(outlineSnippets))])
			} else {
				doc.WriteString(regionSnippets[rng.IntN(len(regionSnippets))])
			}
			if rng.IntN(2) == 0 {
				doc.WriteString("\n")
			}
		}
		text := doc.String()
		switch rng.IntN(10) {
		case 0:
			text = "---\ntitle: x\n---\n" + text
		case 1:
			text = "---\nopen: front matter\n" + text
		}
		if rng.IntN(10) == 0 {
			text = strings.ReplaceAll(text, "\n", "\r\n")
		}
		if rng.IntN(8) == 0 {
			text = "\xef\xbb\xbf" + text
		}
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			snapshot := parseForCheckWith(t, flavor, text)
			allow := outlineAllowances[rng.IntN(len(outlineAllowances))]
			verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), allow)
			require.True(t, verifier.parsed)
			model := verifier.regions()
			if model == nil {
				continue
			}
			for range 10 {
				edits := randomOutlineEdits(rng, text)
				if rng.IntN(3) == 0 {
					edits = append(edits, randomOutlineEdits(rng, text)...)
				}
				prepared, err := fix.PrepareEdits(edits, len(text))
				if err != nil || len(prepared) == 0 {
					continue
				}
				total++
				want := verifier.acceptsWhole(prepared)
				ok, got := model.checkEdits(prepared)
				if !ok {
					continue
				}
				decided++
				if want {
					kept++
				}
				require.Equal(t, want, got, "flavor %s, allow %+v, document %q, edits %q, fixed %q",
					flavor, allow, text, prepared, fix.ApplyEdits(snapshot.Content, prepared))
			}
		}
	}
	t.Logf("%d edit sets, %d decided by region checks, %d of them keep the document", total, decided, kept)
	assert.Greater(t, decided, total/2, "region checks decide most edit sets")
	assert.Positive(t, kept, "some edit sets keep the document")
}

// TestOutlineCacheIsBounded checks that the outcomes an outline model caches,
// with the slugs of their headings, stay within a bound that follows the
// document, even when every check covers a block the size of the document
// and changes the slugs of its headings, as fixes of padded code spans in
// the headings of a long blockquote do.
func TestOutlineCacheIsBounded(t *testing.T) {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range 600 {
		fmt.Fprintf(&doc, "> ## Section %d ` code`\n>\n", idx)
	}
	input := doc.String()
	cfg := config.NewConfig()
	verifier := newFixVerifier(lint.NewRuleContext(context.Background(), parseForCheck(t, input), cfg, nil),
		fixAllowance{codeSpanSpace: true})
	model := verifier.regions()
	require.NotNil(t, model)
	for idx := range 300 {
		at := strings.Index(input, fmt.Sprintf("Section %d ` code`", idx)) + len(fmt.Sprintf("Section %d `", idx))
		decided, _ := model.checkEdits([]fix.TextEdit{{StartOffset: at, EndOffset: at + 1}})
		require.True(t, decided)
	}
	cached := 0
	for key, outcome := range model.outcomes {
		cached += len(key)
		if !outcome.same {
			cached += len(outcome.text)
		}
		for _, slug := range outcome.slugs {
			cached += len(slug) + slugOverhead
		}
	}
	assert.LessOrEqual(t, cached, model.cacheBudget())
	assert.Less(t, len(model.outcomes), 300, "the budget stops the cache")
}

// TestOutlineModelMatchesOutline checks that the outline written in parts is
// the verifier's on every snippet, so that region checks are used for them.
func TestOutlineModelMatchesOutline(t *testing.T) {
	for _, snippet := range append(append([]string{}, outlineSnippets...), regionSnippets...) {
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			for _, allow := range outlineAllowances {
				verifier := newFixVerifier(lint.NewRuleContext(context.Background(),
					parseForCheckWith(t, flavor, snippet), cfg, nil), allow)
				assert.NotNil(t, verifier.regions(), "%q under %s with %+v", snippet, flavor, allow)
			}
		}
	}
}
