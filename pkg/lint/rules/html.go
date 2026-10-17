package rules

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// InlineHTMLRule restricts the use of raw HTML in Markdown.
type InlineHTMLRule struct {
	lint.BaseRule
}

// NewInlineHTMLRule creates a new inline HTML rule.
func NewInlineHTMLRule() *InlineHTMLRule {
	return &InlineHTMLRule{
		BaseRule: lint.NewBaseRule(
			"MD033",
			"no-inline-html",
			"Inline HTML should be avoided or restricted to allowed elements",
			[]string{"html"},
			false, // Not auto-fixable.
		),
	}
}

// DefaultEnabled returns false - this rule is opt-in.
func (r *InlineHTMLRule) DefaultEnabled() bool {
	return false
}

// Apply reports every opening HTML element that is not allowed. HTML blocks
// are scanned for each element they contain, like markdownlint, which
// re-parses HTML blocks as inline content. Closing tags, comments,
// declarations, and CDATA sections are never reported.
func (r *InlineHTMLRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// markdownlint defaults allowed_elements to [], and table_allowed_elements
	// falls back to allowed_elements when it is not set.
	allowedElements := ctx.OptionStringSlice("allowed_elements", nil)
	allowed := lowercaseSet(allowedElements)
	tableAllowed := lowercaseSet(ctx.OptionStringSlice("table_allowed_elements", allowedElements))

	// The table lines and the masked copy of the file are worked out once
	// for the file, not again for each element.
	tables := newTableLineRanges(ctx.Tables())
	sources := htmlSources{file: ctx.File}

	var diags []lint.Diagnostic
	report := func(construct mdscan.HTMLConstruct, allowedSet map[string]bool) {
		if !construct.IsOpeningElement() || allowedSet[strings.ToLower(construct.Name)] {
			return
		}
		diags = append(diags, r.elementDiagnostic(ctx, construct, allowedSet))
	}

	for _, block := range ctx.HTMLBlocks() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		span := block.SourceRange()
		sources.with(block, func(source []byte) {
			for _, construct := range mdscan.ScanHTML(source, span.StartOffset, span.EndOffset) {
				report(construct, allowed)
			}
		})
	}

	for _, inline := range ctx.HTMLInlines() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		construct, ok := sources.construct(inline)
		if !ok {
			continue
		}
		allowedSet := allowed
		if tables.contains(inline.SourcePosition().StartLine) {
			allowedSet = tableAllowed
		}
		report(construct, allowedSet)
	}

	sortDiagnosticsByPosition(diags)
	return diags, nil
}

// elementDiagnostic builds the diagnostic for a disallowed element. The range
// covers the element's opening tag up to the end of its first line.
func (r *InlineHTMLRule) elementDiagnostic(
	ctx *lint.RuleContext,
	construct mdscan.HTMLConstruct,
	allowedSet map[string]bool,
) lint.Diagnostic {
	end := construct.End
	if newline := bytes.IndexAny(construct.Text(ctx.File.Content), "\r\n"); newline >= 0 {
		end = construct.Start + newline
	}
	startLine, startColumn := ctx.File.LineAt(construct.Start)
	endLine, endColumn := ctx.File.LineAt(end)
	pos := mdast.SourcePosition{StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn}

	suggestion := "Remove HTML or use Markdown syntax"
	if len(allowedSet) > 0 {
		names := make([]string, 0, len(allowedSet))
		for name := range allowedSet {
			names = append(names, name)
		}
		sort.Strings(names)
		suggestion = "Allowed elements: " + strings.Join(names, ", ")
	}

	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
		fmt.Sprintf("HTML element '%s' is not allowed", construct.Name)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(suggestion).
		Build()
}

// htmlNodeConstruct returns the single raw HTML construct of an inline HTML
// node, read from source, the node's file content as htmlSources gives it.
func htmlNodeConstruct(source []byte, node *mdast.Node) (mdscan.HTMLConstruct, bool) {
	if node == nil {
		return mdscan.HTMLConstruct{}, false
	}
	span := node.SourceRange()
	if span.StartOffset < 0 || span.EndOffset > len(source) || span.IsEmpty() {
		return mdscan.HTMLConstruct{}, false
	}
	return mdscan.MatchHTML(source[:span.EndOffset], span.StartOffset)
}

// quotedContinuationPrefixes returns the byte ranges of the container
// prefixes that htmlSources masks: on each of node's lines after its first,
// the prefix when it holds a blockquote marker.
func quotedContinuationPrefixes(file *mdast.FileSnapshot, node *mdast.Node) []mdast.SourceRange {
	pos := node.SourcePosition()
	if !pos.IsValid() {
		return nil
	}
	var prefixes []mdast.SourceRange
	for line := pos.StartLine + 1; line <= pos.EndLine && line <= len(file.Lines); line++ {
		prefix := lint.ContainerPrefix(file, node, line)
		if !bytes.ContainsRune(prefix, '>') {
			continue
		}
		start := file.Lines[line-1].StartOffset
		prefixes = append(prefixes, mdast.SourceRange{StartOffset: start, EndOffset: start + len(prefix)})
	}
	return prefixes
}

// fillWithSpaces overwrites every byte of text with a space.
func fillWithSpaces(text []byte) {
	for idx := range text {
		text[idx] = ' '
	}
}

// htmlSources gives each HTML node the source it is read from: the file's
// content with the blockquote prefix of each of the node's lines after its
// first replaced by spaces, at unchanged offsets. Raw HTML that spans lines
// inside a blockquote is then read the way the parser read it, without a
// line's '>' marker ending a tag early. A node without such lines reads the
// file itself. The nodes that need masked lines share one copy of the file,
// which is masked for a node and restored after it, so the file is copied at
// most once, however many such nodes it holds.
type htmlSources struct {
	file    *mdast.FileSnapshot
	scratch []byte
}

// construct returns the single raw HTML construct of an inline HTML node,
// read from the node's source.
func (s *htmlSources) construct(node *mdast.Node) (mdscan.HTMLConstruct, bool) {
	var (
		construct mdscan.HTMLConstruct
		ok        bool
	)
	s.with(node, func(source []byte) {
		construct, ok = htmlNodeConstruct(source, node)
	})
	return construct, ok
}

// with calls use with the source of node. The source is valid only during
// the call.
func (s *htmlSources) with(node *mdast.Node, use func(source []byte)) {
	prefixes := quotedContinuationPrefixes(s.file, node)
	if len(prefixes) == 0 {
		use(s.file.Content)
		return
	}
	if s.scratch == nil {
		s.scratch = bytes.Clone(s.file.Content)
	}
	for _, prefix := range prefixes {
		fillWithSpaces(s.scratch[prefix.StartOffset:prefix.EndOffset])
	}
	use(s.scratch)
	for _, prefix := range prefixes {
		copy(s.scratch[prefix.StartOffset:prefix.EndOffset], s.file.Content[prefix.StartOffset:prefix.EndOffset])
	}
}

// tableLineRanges holds the first and last lines of a file's tables, sorted
// and merged, so that finding whether a line lies in a table is a binary
// search instead of a walk over the document.
type tableLineRanges [][2]int

// newTableLineRanges returns the line ranges of tables.
func newTableLineRanges(tables []*mdast.Node) tableLineRanges {
	ranges := make(tableLineRanges, 0, len(tables))
	for _, table := range tables {
		if pos := table.SourcePosition(); pos.IsValid() {
			ranges = append(ranges, [2]int{pos.StartLine, pos.EndLine})
		}
	}
	return ranges.merged()
}

// merged sorts the ranges by first line and joins the ones that overlap. It
// reuses the receiver's storage.
func (r tableLineRanges) merged() tableLineRanges {
	slices.SortFunc(r, func(a, b [2]int) int { return cmp.Compare(a[0], b[0]) })
	merged := r[:0]
	for _, lines := range r {
		if last := len(merged) - 1; last >= 0 && lines[0] <= merged[last][1] {
			merged[last][1] = max(merged[last][1], lines[1])
			continue
		}
		merged = append(merged, lines)
	}
	return merged
}

// contains reports whether line lies in one of the tables, as
// lint.IsLineInTable does.
func (r tableLineRanges) contains(line int) bool {
	next := sort.Search(len(r), func(idx int) bool { return r[idx][0] > line })
	return next > 0 && line <= r[next-1][1]
}

// lowercaseSet returns the set of lowercased values.
func lowercaseSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[strings.ToLower(value)] = true
	}
	return set
}
