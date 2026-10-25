package rules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gmast "github.com/yuin/goldmark/ast"
	goldparser "github.com/yuin/goldmark/parser"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// sharedChecksDoc has findings with fixes of several rules that check their
// fixes with a fixChecker (MD019, MD023, MD026, MD049, MD050) and of
// several that check them with a fixVerifier (MD011, MD034, MD038, MD039).
const sharedChecksDoc = "# Title\n\n" +
	"##  Two spaces\n\n" +
	"  ## Indented\n\n" +
	"## Trailing punctuation.\n\n" +
	"Some *em* and _em_ text, and **strong** and __strong__ text.\n\n" +
	"A `code ` span and a [ link ](https://example.com/a) here.\n\n" +
	"A (reversed)[https://example.com/b] link.\n\n" +
	"Visit https://example.com/c for more.\n\n" +
	"- item one\n" +
	"- item two\n"

// TestFixChecksParseOriginalOncePerLint checks that the fix checks of all
// the rules of one lint of a file parse the whole original document once,
// for their outlines, and build its region layout once, however many rules
// check fixes.
func TestFixChecksParseOriginalOncePerLint(t *testing.T) {
	t.Parallel()

	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		t.Run(string(flavor), func(t *testing.T) {
			t.Parallel()

			cfg := config.NewConfig()
			cfg.Flavor = flavor
			registry := lint.NewRegistry()
			RegisterAll(registry)
			counts := &originalParses{}
			ctx := context.WithValue(context.Background(), originalParsesKey{}, counts)
			result, err := lint.NewEngine(goldmark.New(string(flavor)), registry).
				LintFile(ctx, "doc.md", []byte(sharedChecksDoc), cfg)
			require.NoError(t, err)
			require.Empty(t, result.RuleErrors)

			fixed := make(map[string]bool)
			for _, diag := range result.Diagnostics {
				if diag.HasFix() {
					fixed[diag.RuleID] = true
				}
			}
			for _, id := range []string{"MD019", "MD023", "MD026", "MD049", "MD050", "MD011", "MD034", "MD038", "MD039"} {
				assert.True(t, fixed[id], "%s offers a fix", id)
			}
			assert.Equal(t, 1, counts.checks, "parses of the original with a check's parser")
			assert.Equal(t, 1, counts.layouts, "region layouts of the original")
		})
	}
}

// allowances lists every allowance that a rule checks its fixes with.
var allowances = []fixAllowance{
	{}, {linkTextSpace: true}, {codeSpanSpace: true}, {autolinks: true},
	{codeInfo: true}, {prompts: true}, {reversedLinks: true},
}

// sharedProbe is a rule that runs after every built-in rule, since rules
// run in order of ID, and checks each value that the fix checks of the lint
// shared against the value built again the way each rule built it before
// the rules shared it. Since it runs last, a value that a rule changed after
// it was built differs too.
type sharedProbe struct {
	lint.BaseRule
	t *testing.T
	// compared counts the shared values compared, by kind.
	compared map[string]int
	// flavorsDiffer counts the files whose outlines differ by flavor.
	flavorsDiffer int
}

func newSharedProbe(t *testing.T) *sharedProbe {
	return &sharedProbe{
		BaseRule: lint.NewBaseRule("ZZ999", "shared-probe", "", nil, false),
		t:        t,
		compared: make(map[string]int),
	}
}

// cached returns the value of key that a rule before the probe shared, and
// false when no rule shared one.
func cached[T any](ctx *lint.RuleContext, key any) (T, bool) {
	built := false
	value := lint.SharedValue(ctx, key, func() T {
		built = true
		var zero T
		return zero
	})
	return value, !built
}

func (p *sharedProbe) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	t := p.t
	t.Helper()
	flavor := checkFlavor(ctx)
	content := checkedContent(ctx.File.Content)
	parseCtx := context.Background()

	fresh, err := goldmark.New(flavor).Parse(parseCtx, ctx.File.Path, content)
	require.NoError(t, err)
	// Before, fixChecker parsed the original without a path.
	unnamed, ok := reparse(ctx, content)
	require.True(t, ok)
	freshOutline := outlineOf(unnamed)
	assert.Equal(t, freshOutline, outlineOf(fresh), "the path of a parse does not change its outline")

	if parsed, ok := cached[parsedDocument](ctx, originalKey{flavor}); ok {
		p.compared["parse"]++
		assert.True(t, parsed.ok)
		assert.True(t, reflect.DeepEqual(fresh, parsed.file), "shared parse of the original")
	}
	if shared, ok := cached[outlineOfOriginal](ctx, checkerOutlineKey{flavor}); ok {
		p.compared["checker outline"]++
		assert.True(t, shared.ok)
		assert.True(t, reflect.DeepEqual(freshOutline, shared.outline), "shared fixChecker outline")
	}
	for _, allow := range allowances {
		if shared, ok := cached[string](ctx, verifierOutlineKey{flavor: flavor, allow: allow}); ok {
			p.compared["verifier outline"]++
			assert.Equal(t, documentOutline(fresh, allow), shared, "shared fixVerifier outline for %+v", allow)
		}
	}
	freshLayout := newDocLayoutFlags(recordingMarkdown(flavor), content, true)
	if base, ok := cached[*docLayout](ctx, layoutKey{flavor}); ok {
		p.compared["layout"]++
		assertSameLayout(t, freshLayout, base)
		if base != nil {
			assert.Nil(t, base.units, "no check builds units on the shared layout")
			assert.Nil(t, base.openers, "no check finds item openers on the shared layout")
			assert.Nil(t, base.labels, "no check builds labels on the shared layout")
			assert.Equal(t, regionWork{}, base.work, "no check counts work on the shared layout")
		}
		// An outline model built on a view of the shared layout is the one
		// built on a layout of its own, and leaves the shared layout as it was.
		for _, allow := range allowances {
			verifier := newFixVerifier(ctx, allow)
			shared := buildOutlineModel(verifier, fresh, originalLayout(ctx))
			own := buildOutlineModel(verifier, fresh, newDocLayoutFlags(recordingMarkdown(flavor), content, true))
			assertSameModel(t, own, shared)
			p.compared["model"]++
		}
		assertSameLayout(t, freshLayout, base)
	}

	// A value shared for the file under one flavor is not served for the
	// other.
	other := *ctx
	otherCfg := config.NewConfig()
	otherCfg.Flavor = config.FlavorGFM
	if flavor == goldmark.FlavorGFM {
		otherCfg.Flavor = config.FlavorCommonMark
	}
	other.Config = otherCfg
	otherFresh, ok := reparse(&other, content)
	require.True(t, ok)
	otherOutline, ok := originalOutline(&other)
	require.True(t, ok)
	assert.True(t, reflect.DeepEqual(outlineOf(otherFresh), otherOutline), "outline under the other flavor")
	assertSameLayout(t, newDocLayoutFlags(recordingMarkdown(string(otherCfg.Flavor)), content, true), originalLayout(&other))
	if !reflect.DeepEqual(outlineOf(otherFresh), freshOutline) {
		p.flavorsDiffer++
	}
	return nil, nil
}

// TestFixChecksShareValuesBuiltAsBefore lints the golden inputs, the
// documents above and the content their fixes make, with every rule and a
// probe after them (see sharedProbe), and checks that each value the fix
// checks shared is the value each rule built for itself before.
func TestFixChecksShareValuesBuiltAsBefore(t *testing.T) {
	t.Parallel()

	inputs, err := filepath.Glob(filepath.Join(testdataDir(t), "*", "*.input.md"))
	require.NoError(t, err)
	require.NotEmpty(t, inputs)
	docs := []string{sharedChecksDoc, itemHeadingsDoc(40), "---\ntitle: x\n---\n# T\n\n#No space\n"}
	for _, path := range inputs {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		docs = append(docs, string(data))
	}

	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		t.Run(string(flavor), func(t *testing.T) {
			t.Parallel()

			cfg := config.NewConfig()
			cfg.Flavor = flavor
			registry := lint.NewRegistry()
			RegisterAll(registry)
			probe := newSharedProbe(t)
			registry.Register(probe)
			engine := lint.NewEngine(goldmark.New(string(flavor)), registry)
			for _, doc := range docs {
				content := []byte(doc)
				for pass := 0; pass < 2; pass++ {
					result, err := engine.LintFile(context.Background(), "doc.md", content, cfg)
					require.NoError(t, err)
					require.Empty(t, result.RuleErrors)
					if len(result.Edits) == 0 {
						break
					}
					content = fix.ApplyEdits(content, result.Edits)
				}
			}
			for _, kind := range []string{"parse", "checker outline", "verifier outline", "layout", "model"} {
				assert.Positive(t, probe.compared[kind], "shared values compared: %s", kind)
			}
			assert.Positive(t, probe.flavorsDiffer, "files whose outlines differ by flavor")
		})
	}
}

// itemHeadingsDoc returns a document with one long list item that holds n
// headings without blank lines around them, so that MD022 checks its fixes
// on the item's groups, with the region layout.
func itemHeadingsDoc(n int) string {
	var out strings.Builder
	out.WriteString("# T\n\n- outer\n\n")
	for idx := range n {
		fmt.Fprintf(&out, "  text %d\n  ## Heading %d\n  after %d\n\n", idx, idx, idx)
	}
	return out.String()
}

// assertSameLayout checks that got is the region layout want is, but for
// the units, label index and work counts that checks build on a view.
func assertSameLayout(t *testing.T, want, got *docLayout) {
	t.Helper()
	if want == nil || got == nil {
		assert.Equal(t, want == nil, got == nil, "layout built")
		return
	}
	assert.True(t, want.markdown == got.markdown, "markdown")
	assert.Equal(t, want.source, got.source, "source")
	assert.Equal(t, [5]any{want.base, want.frontMatter, want.frontLimit, want.firstLineFree, want.tableFlags},
		[5]any{got.base, got.frontMatter, got.frontLimit, got.firstLineFree, got.tableFlags}, "front matter")
	assert.Equal(t, want.bounds, got.bounds, "bounds")
	assert.Equal(t, want.afterClosed, got.afterClosed, "afterClosed")
	assert.Equal(t, want.groupNodes, got.groupNodes, "groupNodes")
	assert.Equal(t, nodeTrees(want.nodes), nodeTrees(got.nodes), "nodes")
	assert.Equal(t, want.lists, got.lists, "lists")
	assert.Equal(t, want.tables, got.tables, "tables")
	require.Len(t, got.quotes, len(want.quotes), "quotes")
	for idx := range want.quotes {
		wantQuote, gotQuote := *want.quotes[idx], *got.quotes[idx]
		assert.Equal(t, nodeTrees(wantQuote.nodes), nodeTrees(gotQuote.nodes), "quote %d nodes", idx)
		wantQuote.nodes, gotQuote.nodes = nil, nil
		assert.Equal(t, wantQuote, gotQuote, "quote %d", idx)
	}
	require.Len(t, got.items, len(want.items), "items")
	for idx := range want.items {
		wantItem, gotItem := *want.items[idx], *got.items[idx]
		assert.Equal(t, nodeTrees(wantItem.nodes), nodeTrees(gotItem.nodes), "item %d nodes", idx)
		wantItem.nodes, gotItem.nodes = nil, nil
		assert.Equal(t, wantItem, gotItem, "item %d", idx)
	}
	for name, maps := range map[string][2]map[gmast.Node]int{
		"lists": {want.listIndex, got.listIndex}, "quotes": {want.quoteIndex, got.quoteIndex},
		"items": {want.itemIndex, got.itemIndex}, "tables": {want.tableIndex, got.tableIndex},
	} {
		assert.Equal(t, indexByPath(maps[0]), indexByPath(maps[1]), "index of %s", name)
	}
	assert.Equal(t, refTexts(want.refs), refTexts(got.refs), "refs")
	wantIndex, gotIndex := make(map[string]string), make(map[string]string)
	for label, ref := range want.refIndex {
		wantIndex[label] = refText(ref)
	}
	for label, ref := range got.refIndex {
		gotIndex[label] = refText(ref)
	}
	assert.Equal(t, wantIndex, gotIndex, "refIndex")
	assert.Equal(t, want.lookups, got.lookups, "lookups")
}

// assertSameModel checks that got is the outline model want is.
func assertSameModel(t *testing.T, want, got *outlineModel) {
	t.Helper()
	if want == nil || got == nil {
		assert.Equal(t, want == nil, got == nil, "model built")
		return
	}
	assertSameLayout(t, want.docLayout, got.docLayout)
	assert.Equal(t, want.body, got.body, "body")
	assert.Equal(t, want.groups, got.groups, "groups")
	assert.Equal(t, want.listParts, got.listParts, "listParts")
	assert.Equal(t, want.quoteParts, got.quoteParts, "quoteParts")
	assert.Equal(t, want.tableParts, got.tableParts, "tableParts")
	assert.Equal(t, want.itemParts, got.itemParts, "itemParts")
	assert.Equal(t, want.slugs, got.slugs, "slugs")
	assert.Equal(t, want.history, got.history, "history")
	assert.Equal(t, want.anchors, got.anchors, "anchors")
}

// nodeTrees describes goldmark nodes and their descendants: kinds, line
// segments and text segments.
func nodeTrees(nodes []gmast.Node) []string {
	trees := make([]string, len(nodes))
	for idx, node := range nodes {
		var out strings.Builder
		writeNodeTree(&out, node)
		trees[idx] = out.String()
	}
	return trees
}

func writeNodeTree(out *strings.Builder, node gmast.Node) {
	fmt.Fprintf(out, "(%s", node.Kind())
	if node.Type() == gmast.TypeBlock {
		lines := node.Lines()
		for idx := range lines.Len() {
			segment := lines.At(idx)
			fmt.Fprintf(out, " %d-%d", segment.Start, segment.Stop)
		}
	}
	if text, ok := node.(*gmast.Text); ok {
		fmt.Fprintf(out, " text %d-%d", text.Segment.Start, text.Segment.Stop)
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		out.WriteByte(' ')
		writeNodeTree(out, child)
	}
	out.WriteByte(')')
}

// indexByPath returns index keyed by the path of each node from the root
// of its tree.
func indexByPath(index map[gmast.Node]int) map[string]int {
	byPath := make(map[string]int, len(index))
	for node, at := range index {
		var path []string
		for current := node; current != nil; current = current.Parent() {
			position := 0
			for sibling := current.PreviousSibling(); sibling != nil; sibling = sibling.PreviousSibling() {
				position++
			}
			path = append(path, fmt.Sprintf("%s#%d", current.Kind(), position))
		}
		slices.Reverse(path)
		byPath[strings.Join(path, "/")] = at
	}
	return byPath
}

// refTexts describes recorded definitions.
func refTexts(refs []recordedRef) []string {
	texts := make([]string, len(refs))
	for idx, ref := range refs {
		texts[idx] = fmt.Sprintf("%d %s", ref.offset, refText(ref.ref))
	}
	return texts
}

// refText describes a definition.
func refText(ref goldparser.Reference) string {
	return fmt.Sprintf("%q %q %q", ref.Label(), ref.Destination(), ref.Title())
}
