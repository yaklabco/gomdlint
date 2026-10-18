package rules

import (
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// randomAnchorSlug returns a slug likely to collide with others and with
// their numbered anchors: a short slug, often numbered once or twice, and
// sometimes with a number no count gives, such as 0 or 01.
func randomAnchorSlug(rng *rand.Rand) string {
	bases := []string{"a", "a", "a", "b", "", "a-", "x-y"}
	slug := bases[rng.IntN(len(bases))]
	for range rng.IntN(3) {
		switch rng.IntN(8) {
		case 0:
			slug += "-0" + strconv.Itoa(1+rng.IntN(3))
		case 1:
			slug += "-0"
		default:
			slug += "-" + strconv.Itoa(1+rng.IntN(9))
		}
	}
	return slug
}

// randomAnchorSlugs returns up to most random slugs, sometimes after a run
// of numbered slugs in shuffled order, which a later slug must pass.
func randomAnchorSlugs(rng *rand.Rand, most int) []string {
	var slugs []string
	if rng.IntN(3) == 0 {
		run := make([]string, 1+rng.IntN(30))
		for idx := range run {
			run[idx] = "a-" + strconv.Itoa(1+idx)
		}
		rng.Shuffle(len(run), func(i, j int) { run[i], run[j] = run[j], run[i] })
		slugs = append(slugs, run[:1+rng.IntN(len(run))]...)
	}
	for range rng.IntN(most + 1) {
		slugs = append(slugs, randomAnchorSlug(rng))
	}
	rng.Shuffle(len(slugs), func(i, j int) {
		if rng.IntN(4) == 0 {
			slugs[i], slugs[j] = slugs[j], slugs[i]
		}
	})
	return slugs
}

// probingReplay works out the anchors of slugs after the first `before`
// headings of a history as the signature writer does, trying one number at
// a time, with the counts they set.
func probingReplay(history *anchorHistory, slugs []string, before int) ([]string, map[string]int) {
	counts := make(map[string]int)
	get := func(key string) (int, bool) {
		if value, found := counts[key]; found {
			return value, true
		}
		return history.lookup(key, before)
	}
	anchors := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		unique := slug
		for {
			if _, taken := get(unique); !taken {
				break
			}
			value, _ := get(slug)
			counts[slug] = value + 1
			unique = slug + "-" + strconv.Itoa(value+1)
		}
		counts[unique] = 0
		anchors = append(anchors, unique)
	}
	return anchors, counts
}

// TestMinHeapDropsInOrder checks that the heap holds its least item first
// as pushes and drops interleave.
func TestMinHeapDropsInOrder(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 7))
	for range 500 {
		numbers := minHeap[int]{less: func(a, b int) bool { return a < b }}
		var held []int
		for range rng.IntN(60) {
			if len(held) > 0 && rng.IntN(3) == 0 {
				slices.Sort(held)
				require.Equal(t, held[0], numbers.items[0])
				numbers.drop()
				held = held[1:]
				continue
			}
			number := rng.IntN(20)
			numbers.push(number)
			held = append(held, number)
		}
		slices.Sort(held)
		for _, want := range held {
			require.Equal(t, want, numbers.items[0])
			numbers.drop()
		}
		require.Empty(t, numbers.items)
	}
}

// TestAnchorHistoryCountsByOne checks what replay and the anchor walk rely
// on: each key's values in a history are 0, 1, 2 and so on, the first set
// by the heading that takes it as its anchor.
func TestAnchorHistoryCountsByOne(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 1))
	for range 2000 {
		history := recordAnchors(randomAnchorSlugs(rng, 40))
		for key, writes := range history.writes {
			for idx, write := range writes {
				require.Equal(t, idx, write.value, "key %q", key)
			}
			require.Equal(t, key, history.anchors[writes[0].heading], "key %q", key)
		}
	}
}

// TestAnchorReplayMatchesProbing checks replay, which passes runs of taken
// numbers at once, against trying one number at a time, on random slugs
// after random places of random histories.
func TestAnchorReplayMatchesProbing(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 2))
	for range 20000 {
		slugs := randomAnchorSlugs(rng, 40)
		history := recordAnchors(slugs)
		before := rng.IntN(len(slugs) + 1)
		replayed := randomAnchorSlugs(rng, 8)
		wantAnchors, wantCounts := probingReplay(&history, replayed, before)
		gotAnchors, gotCounts := history.replay(replayed, before)
		require.Equal(t, wantAnchors, gotAnchors, "slugs %q before %d, replayed %q", slugs, before, replayed)
		require.Equal(t, wantCounts, gotCounts, "slugs %q before %d, replayed %q", slugs, before, replayed)
	}
}

// randomAnchorChanges returns sorted changes that do not overlap, each
// replacing a run of the headings of slugs, perhaps none, with random slugs,
// with the slugs of the run changed in one place, or with the run's anchors
// as slugs, which give the same anchors but count differently.
func randomAnchorChanges(rng *rand.Rand, history *anchorHistory) []anchorChange {
	var changes []anchorChange
	from := 0
	for range rng.IntN(5) {
		if from > len(history.slugs) {
			break
		}
		start := from + rng.IntN(len(history.slugs)-from+1)
		end := start + rng.IntN(min(3, len(history.slugs)-start)+1)
		var slugs []string
		switch rng.IntN(4) {
		case 0:
			slugs = randomAnchorSlugs(rng, 3)
		case 1:
			slugs = slices.Clone(history.slugs[start:end])
			if len(slugs) > 0 {
				slugs[rng.IntN(len(slugs))] = randomAnchorSlug(rng)
			}
		case 2:
			slugs = slices.Clone(history.anchors[start:end])
		default:
			slugs = slices.Clone(history.slugs[start:end])
			if rng.IntN(2) == 0 && len(slugs) > 0 {
				slugs = slugs[1:]
			} else {
				slugs = append(slugs, randomAnchorSlug(rng))
			}
		}
		changes = append(changes, anchorChange{from: start, to: end, slugs: slugs})
		from = end + rng.IntN(2)
	}
	return changes
}

// TestAnchorsKeptMatchesRebuild checks the anchor walk against working out
// every anchor of the changed document and joining them as the outline
// does, on random changes to random histories.
func TestAnchorsKeptMatchesRebuild(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261018, 3))
	kept := 0
	for range 40000 {
		slugs := randomAnchorSlugs(rng, 30)
		history := recordAnchors(slugs)
		changes := randomAnchorChanges(rng, &history)
		var edited []string
		next := 0
		for _, change := range changes {
			edited = append(append(edited, slugs[next:change.from]...), change.slugs...)
			next = change.to
		}
		edited = append(edited, slugs[next:]...)
		want := strings.Join(recordAnchors(edited).anchors, "\x00") == strings.Join(history.anchors, "\x00")
		if want {
			kept++
		}
		require.Equal(t, want, history.keeps(changes), "slugs %q, changes %+v", slugs, changes)
	}
	t.Logf("%d change sets keep the anchors", kept)
	assert.Greater(t, kept, 1000, "many change sets keep the anchors")
}

// rebuiltAnchorsKept reports whether the document with the given slugs and
// changes gives its headings the document's anchors, by numbering every
// heading of the changed document.
func rebuiltAnchorsKept(slugs []string, changes []anchorChange) bool {
	var edited []string
	next := 0
	for _, change := range changes {
		edited = append(append(edited, slugs[next:change.from]...), change.slugs...)
		next = change.to
	}
	edited = append(edited, slugs[next:]...)
	return strings.Join(recordAnchors(edited).anchors, "\x00") == strings.Join(recordAnchors(slugs).anchors, "\x00")
}

// TestAnchorsKeptOnMovedHeadings checks the anchor walk against numbering
// every heading on every list of up to four slugs from a small set whose
// numbered slugs collide, with one or two headings taken out and as many
// put in elsewhere, before or after. A moved heading shifts the headings
// between, whose anchors must then each change into the anchor at their new
// place, which is where the walk's events and the keys they leave behind
// matter.
func TestAnchorsKeptOnMovedHeadings(t *testing.T) {
	alphabet := []string{"a", "a-1", "a-2", "b"}
	lists := [][]string{nil}
	frontier := [][]string{nil}
	for range 4 {
		var longer [][]string
		for _, list := range frontier {
			for _, slug := range alphabet {
				longer = append(longer, append(slices.Clone(list), slug))
			}
		}
		lists = append(lists, longer...)
		frontier = longer
	}
	require.Len(t, lists, 1+4+16+64+256)
	var inserts [][]string
	for _, first := range alphabet {
		inserts = append(inserts, []string{first})
		for _, second := range alphabet {
			inserts = append(inserts, []string{first, second})
		}
	}
	cases, kept := 0, 0
	check := func(slugs []string, changes []anchorChange) {
		history := recordAnchors(slugs)
		want := rebuiltAnchorsKept(slugs, changes)
		require.Equal(t, want, history.keeps(changes), "slugs %q, changes %+v", slugs, changes)
		cases++
		if want {
			kept++
		}
	}
	for _, slugs := range lists {
		for _, insert := range inserts {
			moved := len(insert)
			for from := 0; from+moved <= len(slugs); from++ {
				for at := 0; at <= len(slugs); at++ {
					switch {
					case at >= from+moved:
						check(slugs, []anchorChange{{from: from, to: from + moved}, {from: at, to: at, slugs: insert}})
					case at <= from:
						check(slugs, []anchorChange{{from: at, to: at, slugs: insert}, {from: from, to: from + moved}})
					}
				}
			}
		}
	}
	t.Logf("%d moves, %d keep the anchors", cases, kept)
	assert.Greater(t, kept, cases/50, "many moves keep the anchors")
}

// collidingHeadings are headings whose slugs collide with one another and
// with numbered anchors once the code and link rules fix them: reversed
// links whose slugs lose a number, and code spans whose slugs lose a hyphen.
var collidingHeadings = []string{
	"# x\n", "# x\n", "# x-1\n", "# x-2\n", "# x-1-1\n", "# (x)[-1]\n", "# (x)[-2]\n", "## (x)[-1-1]\n",
	"# (x-1)[-1]\n", "# a\n", "# a-1\n", "# `a `\n", "# ` a`\n", "> # (x)[-1]\n", "- # (x)[-1]\n- b\n", "Text.\n",
}

// collidingFixes returns the edits of one to three fixes of reversed links
// and padded code spans in text.
func collidingFixes(rng *rand.Rand, text string) []fix.TextEdit {
	reversed := regexp.MustCompile(`\(([^()\n]+)\)\[([^\]\n]+)\]`)
	padded := regexp.MustCompile("`( a|a )`")
	var sites []fix.TextEdit
	for _, match := range reversed.FindAllStringSubmatchIndex(text, -1) {
		sites = append(sites, fix.TextEdit{
			StartOffset: match[0], EndOffset: match[1],
			NewText: "[" + text[match[2]:match[3]] + "](" + text[match[4]:match[5]] + ")",
		})
	}
	for _, match := range padded.FindAllStringIndex(text, -1) {
		at := match[0] + 1 + strings.IndexByte(text[match[0]+1:match[1]], ' ')
		sites = append(sites, fix.TextEdit{StartOffset: at, EndOffset: at + 1})
	}
	if len(sites) == 0 {
		return nil
	}
	var edits []fix.TextEdit
	for range 1 + rng.IntN(3) {
		edits = append(edits, sites[rng.IntN(len(sites))])
	}
	return edits
}

// TestOutlineAnchorsDecideAsWholeDocument checks the outline's region checks
// against parsing the whole document on documents of colliding headings,
// where fixes keep or change anchors, change the counts after them, and
// move the anchors of later headings.
func TestOutlineAnchorsDecideAsWholeDocument(t *testing.T) {
	documents := 600
	if testing.Short() || raceEnabled {
		documents = 100
	}
	rng := rand.New(rand.NewPCG(20261018, 4))
	allow := fixAllowance{reversedLinks: true, codeSpanSpace: true}
	total, decided, kept := 0, 0, 0
	for range documents {
		var doc strings.Builder
		for range 3 + rng.IntN(10) {
			doc.WriteString(collidingHeadings[rng.IntN(len(collidingHeadings))] + "\n")
		}
		text := doc.String()
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			snapshot := parseForCheckWith(t, flavor, text)
			verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), allow)
			model := verifier.regions()
			require.NotNil(t, model, "document %q", text)
			for range 8 {
				prepared, err := fix.PrepareEdits(collidingFixes(rng, text), len(text))
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
				require.Equal(t, want, got, "flavor %s, document %q, edits %q", flavor, text, prepared)
			}
		}
	}
	t.Logf("%d edit sets, %d decided by region checks, %d of them keep the document", total, decided, kept)
	assert.Greater(t, decided, total*9/10, "region checks decide almost every edit set")
	assert.Greater(t, kept, total/50, "some edit sets keep the document")
}

// numberedSlugsDocument returns a document of the headings "a" and "a-1" up
// to "a-" and count, then count headings that hold the code span "a " and so
// have the slug "a-", with blank lines between them, and the edits that
// remove the space from each code span, as MD038 fixes it. A fix gives its
// heading the slug "a", whose next free number comes after all the numbered
// headings.
func numberedSlugsDocument(count int) (string, []fix.TextEdit) {
	var doc strings.Builder
	doc.WriteString("# a\n\n")
	for idx := 1; idx <= count; idx++ {
		fmt.Fprintf(&doc, "# a-%d\n\n", idx)
	}
	var edits []fix.TextEdit
	for range count {
		doc.WriteString("# `a")
		edits = append(edits, fix.TextEdit{StartOffset: doc.Len(), EndOffset: doc.Len() + 1})
		doc.WriteString(" `\n\n")
	}
	return doc.String(), edits
}

// reversedLinkSlugsDocument returns a document of n pairs of headings, "xi"
// and the reversed link "(xi)[-1]", whose slug is "xi-1", and the edits that
// turn each reversed link into a link, as MD011 fixes it. A fix gives its
// heading the slug "xi", which keeps the anchor "xi-1" but counts "xi" once
// more, so the counts after the heading change while its anchor does not.
func reversedLinkSlugsDocument(n int) (string, []fix.TextEdit) {
	var doc strings.Builder
	var edits []fix.TextEdit
	for idx := range n {
		fmt.Fprintf(&doc, "# x%d\n\n# ", idx)
		reversed := fmt.Sprintf("(x%d)[-1]", idx)
		edits = append(edits, fix.TextEdit{
			StartOffset: doc.Len(), EndOffset: doc.Len() + len(reversed), NewText: fmt.Sprintf("[x%d](-1)", idx),
		})
		doc.WriteString(reversed + "\n\n")
	}
	return doc.String(), edits
}

// anchorLookupsOf checks each edit on its own with both region checks, the
// outline's under allow, requiring both to decide, and returns the anchor
// counts and numbers the render signature's histories and the outline's
// looked up.
func anchorLookupsOf(t *testing.T, input string, edits []fix.TextEdit, allow fixAllowance) (int, int) {
	t.Helper()
	render, outlined := 0, 0
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		snapshot := parseForCheckWith(t, flavor, input)
		check := newFixCheck(snapshot, flavor)
		cfg := config.NewConfig()
		cfg.Flavor = flavor
		verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), allow)
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
			render += model.history.lookups
		}
		outlined += outline.history.lookups
	}
	return render, outlined
}

// TestAnchorWorkGrowsWithTheDocument checks that working out the anchors of
// a check takes work that grows with the document, not with its square: for
// headings whose new slug must pass many numbered slugs to find its number,
// and for headings whose new slug keeps its anchor but changes the counts
// after it. Each fix is checked on its own; in each checker, four times the
// headings must take about four times the lookups, not sixteen, and some:
// work done outside the history, such as numbering every heading again,
// would not be counted.
func TestAnchorWorkGrowsWithTheDocument(t *testing.T) {
	const small, factor = 50, 4
	cases := []struct {
		name  string
		doc   func(int) (string, []fix.TextEdit)
		allow fixAllowance
	}{
		{"numbered slugs", numberedSlugsDocument, fixAllowance{codeSpanSpace: true}},
		{"slugs that keep their anchors", reversedLinkSlugsDocument, fixAllowance{reversedLinks: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookups := func(headings int) (int, int) {
				input, edits := tc.doc(headings)
				require.Len(t, edits, headings)
				return anchorLookupsOf(t, input, edits, tc.allow)
			}
			shortRender, shortOutline := lookups(small)
			longRender, longOutline := lookups(factor * small)
			for _, checker := range []struct {
				name        string
				short, long int
			}{{"render signature", shortRender, longRender}, {"outline", shortOutline, longOutline}} {
				require.Positive(t, checker.short, "%s lookups", checker.name)
				ratio := float64(checker.long) / float64(checker.short)
				t.Logf("%s: %d lookups for %d headings, %d for %d, ratio %.1f",
					checker.name, checker.short, small, checker.long, factor*small, ratio)
				assert.Less(t, ratio, 1.5*factor, "%s anchor work grows faster than the document", checker.name)
			}
		})
	}
}
