package rules

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// NoMultipleSpaceBlockquoteRule checks for multiple spaces after blockquote symbol.
type NoMultipleSpaceBlockquoteRule struct {
	lint.BaseRule
}

// NewNoMultipleSpaceBlockquoteRule creates a new no-multiple-space-blockquote rule.
func NewNoMultipleSpaceBlockquoteRule() *NoMultipleSpaceBlockquoteRule {
	return &NoMultipleSpaceBlockquoteRule{
		BaseRule: lint.NewBaseRule(
			"MD027",
			"no-multiple-space-blockquote",
			"Multiple spaces after blockquote symbol",
			[]string{"blockquote", "indentation", "whitespace"},
			true,
		),
	}
}

// quoteSpace is whitespace that follows a blockquote's own prefix on a line.
type quoteSpace struct {
	quote   *mdast.Node
	lineNum int
	start   int // offset of the first extra whitespace byte
	end     int // offset just past the whitespace
	spaces  int // whitespace after the '>' marker, including the optional space
	item    *mdast.Node
	inList  bool
	// unsafe is true when the whitespace indents a code fence or an HTML
	// block, whose indentation is part of the content, or when it keeps a
	// paragraph line that looks like a block start from starting a block.
	unsafe bool
	// startsBlock is true when the text after the whitespace looks like the
	// start of a block.
	startsBlock bool
}

// Apply checks for multiple spaces after blockquote symbol.
//
// Like markdownlint, the rule reports whitespace that directly follows a
// blockquote's '>' marker and its optional space, on every line that
// continues the blockquote, including nested and indented blockquotes.
// Whitespace that a list item claims as its content indentation is not
// reported, and neither is code. The fix removes the extra whitespace; before
// a list marker it moves the item, so it goes through the list shift plan,
// and it is withheld on fence and HTML block lines, whose indentation is
// part of their content.
func (r *NoMultipleSpaceBlockquoteRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	includeListItems := ctx.OptionBool("list_items", true)
	codeLines := lint.CodeContentLines(ctx.CodeBlocks())
	plan := newListShiftPlan(ctx.File)

	var findings []quoteSpace
	for _, quote := range ctx.Blockquotes() {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		pos := quote.SourcePosition()
		if !pos.IsValid() || quote.FirstChild == nil {
			continue
		}
		items := quoteListItems(quote)
		unsafeLines := quoteVerbatimLines(quote)
		continuation := quoteContinuationLines(quote)
		for lineNum := pos.StartLine; lineNum <= nodeLastLine(quote); lineNum++ {
			if codeLines[lineNum] {
				continue
			}
			finding, ok := extraQuoteSpace(ctx.File, quote, items, lineNum)
			if !ok || (!includeListItems && (finding.item != nil || finding.inList)) {
				continue
			}
			finding.unsafe = unsafeLines[lineNum] || (continuation[lineNum] && finding.startsBlock)
			if finding.item != nil {
				proposeQuoteSpaceShift(plan, finding)
			}
			findings = append(findings, finding)
		}
	}
	safeShifts := plan.resolve()

	sort.Slice(findings, func(i, j int) bool { return findings[i].start < findings[j].start })
	diags := make([]lint.Diagnostic, 0, len(findings))
	for _, finding := range findings {
		line := ctx.File.Lines[finding.lineNum-1]
		pos := mdast.SourcePosition{
			StartLine:   finding.lineNum,
			StartColumn: finding.start - line.StartOffset + 1,
			EndLine:     finding.lineNum,
			EndColumn:   finding.end - line.StartOffset + 1,
		}
		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Multiple spaces (%d) after blockquote symbol", finding.spaces)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a single space after the blockquote symbol")
		if quoteSpaceFixable(finding, safeShifts) {
			builder := fix.NewEditBuilder()
			builder.Delete(finding.start, finding.end)
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
}

// extraQuoteSpace finds the whitespace after a blockquote's prefix on a line.
// It reports false when the line does not continue the blockquote, holds
// nothing after the whitespace, or when a list item inside the blockquote
// claims the whitespace as its content indentation.
func extraQuoteSpace(file *mdast.FileSnapshot, quote *mdast.Node, items []*mdast.Node, lineNum int) (quoteSpace, bool) {
	prefix := lint.ContainerPrefix(file, quote.FirstChild, lineNum)
	if strings.Count(string(prefix), ">") < blockquoteDepth(quote) {
		return quoteSpace{}, false
	}
	content := lint.LineContent(file, lineNum)
	start := len(prefix)
	end := start
	for end < len(content) && (content[end] == ' ' || content[end] == '\t') {
		end++
	}
	if end == start || end == len(content) {
		return quoteSpace{}, false
	}

	finding := quoteSpace{
		quote:       quote,
		lineNum:     lineNum,
		start:       file.Lines[lineNum-1].StartOffset + start,
		end:         file.Lines[lineNum-1].StartOffset + end,
		spaces:      end - start,
		startsBlock: wrapContinuationStartsBlock(string(content[end:])),
	}
	if prefix[len(prefix)-1] == ' ' || prefix[len(prefix)-1] == '\t' {
		finding.spaces++
	}

	for _, item := range items {
		itemPos := item.SourcePosition()
		switch {
		case itemPos.StartLine == lineNum && item.SourceRange().StartOffset == finding.end:
			finding.item = item
		case itemPos.StartLine < lineNum && lineNum <= nodeLastLine(item):
			finding.inList = true
			if item.FirstChild != nil && len(lint.ContainerPrefix(file, item.FirstChild, lineNum)) > start {
				// The line continues the item, whose indentation absorbs
				// the whitespace.
				return quoteSpace{}, false
			}
		}
	}
	return finding, true
}

// quoteListItems returns the list items whose nearest blockquote is quote.
func quoteListItems(quote *mdast.Node) []*mdast.Node {
	var items []*mdast.Node
	_ = mdast.Walk(quote, func(node *mdast.Node) error { //nolint:errcheck // Visitor never returns an error.
		if node.Kind == mdast.NodeListItem && nearestBlockquote(node) == quote {
			items = append(items, node)
		}
		return nil
	})
	return items
}

// blockquoteDepth counts a blockquote and the blockquotes enclosing it.
func blockquoteDepth(quote *mdast.Node) int {
	depth := 0
	for node := quote; node != nil; node = node.Parent {
		if node.Kind == mdast.NodeBlockquote {
			depth++
		}
	}
	return depth
}

// proposeQuoteSpaceShift proposes moving the list item that follows the
// extra whitespace to the start of the blockquote content.
func proposeQuoteSpaceShift(plan *listShiftPlan, finding quoteSpace) {
	layout, ok := plan.layout(finding.item)
	if !ok {
		return
	}
	width := finding.end - finding.start
	plan.propose(finding.item, listShift{markerCol: layout.markerCol - width, contentCol: layout.contentCol - width})
}

// quoteContinuationLines returns the lines of a blockquote that continue a
// paragraph rather than start one.
func quoteContinuationLines(quote *mdast.Node) map[int]bool {
	lines := make(map[int]bool)
	_ = mdast.Walk(quote, func(node *mdast.Node) error { //nolint:errcheck // Visitor never returns an error.
		if node.Kind != mdast.NodeParagraph || nearestBlockquote(node) != quote {
			return nil
		}
		pos := node.SourcePosition()
		for line := pos.StartLine + 1; pos.IsValid() && line <= nodeLastLine(node); line++ {
			lines[line] = true
		}
		return nil
	})
	return lines
}

// quoteVerbatimLines returns the lines of a blockquote whose indentation is
// part of the content: the lines of HTML blocks and the fence lines of fenced
// code blocks.
func quoteVerbatimLines(quote *mdast.Node) map[int]bool {
	lines := make(map[int]bool)
	_ = mdast.Walk(quote, func(node *mdast.Node) error { //nolint:errcheck // Visitor never returns an error.
		pos := node.SourcePosition()
		if !pos.IsValid() || nearestBlockquote(node) != quote {
			return nil
		}
		switch {
		case node.Kind == mdast.NodeHTMLBlock:
			for line := pos.StartLine; line <= nodeLastLine(node); line++ {
				lines[line] = true
			}
		case lint.IsFencedCodeBlock(node):
			lines[pos.StartLine] = true
			lines[nodeLastLine(node)] = true
		}
		return nil
	})
	return lines
}

// quoteSpaceFixable reports whether removing the whitespace keeps the
// document structure: a list item must have been accepted by the shift plan,
// and the whitespace must not indent a code fence or an HTML block.
func quoteSpaceFixable(finding quoteSpace, safeShifts map[*mdast.Node]listShift) bool {
	if finding.item != nil {
		_, ok := safeShifts[finding.item]
		return ok
	}
	return !finding.unsafe
}

// NoBlanksBlockquoteRule checks for blank lines inside blockquotes.
type NoBlanksBlockquoteRule struct {
	lint.BaseRule
}

// NewNoBlanksBlockquoteRule creates a new no-blanks-blockquote rule.
func NewNoBlanksBlockquoteRule() *NoBlanksBlockquoteRule {
	return &NoBlanksBlockquoteRule{
		BaseRule: lint.NewBaseRule(
			"MD028",
			"no-blanks-blockquote",
			"Blank line inside blockquote",
			[]string{"blockquote", "whitespace"},
			false, // Not auto-fixable - requires human decision.
		),
	}
}

// Apply checks for blank lines separating blockquotes.
func (r *NoBlanksBlockquoteRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || len(ctx.File.Lines) < 3 {
		return nil, nil
	}

	var diags []lint.Diagnostic
	var inBlockquote bool
	var lastBlockquoteLine int
	// The fence lines of quoted code start with '>' too, so only the code
	// itself is skipped.
	codeLines := lint.CodeContentLines(ctx.CodeBlocks())

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		lineContent := lint.LineContent(ctx.File, lineNum)

		// Skip lines of code.
		if codeLines[lineNum] {
			continue
		}

		isBlockquoteLine := len(lineContent) > 0 && lineContent[0] == '>'
		isBlankLine := len(bytes.TrimSpace(lineContent)) == 0

		if isBlockquoteLine {
			// Check if we're returning to a blockquote after a gap.
			if inBlockquote && lastBlockquoteLine > 0 && lastBlockquoteLine < lineNum-1 {
				// There was a gap - check if it was just blank lines.
				allBlank := true
				for checkLine := lastBlockquoteLine + 1; checkLine < lineNum; checkLine++ {
					checkContent := lint.LineContent(ctx.File, checkLine)
					if len(bytes.TrimSpace(checkContent)) > 0 {
						allBlank = false
						break
					}
				}

				if allBlank {
					pos := mdast.SourcePosition{
						StartLine:   lastBlockquoteLine + 1,
						StartColumn: 1,
						EndLine:     lineNum - 1,
						EndColumn:   1,
					}

					diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
						"Blank line inside blockquote separates it into multiple blockquotes").
						WithSeverity(config.SeverityWarning).
						WithSuggestion("Add text between blockquotes or use '>' on blank lines").
						Build()
					diags = append(diags, diag)
				}
			}

			inBlockquote = true
			lastBlockquoteLine = lineNum
		} else if !isBlankLine {
			// Non-blank, non-blockquote line resets the tracking.
			inBlockquote = false
			lastBlockquoteLine = 0
		}
		// Blank lines don't reset tracking - we want to find gaps.
	}

	return diags, nil
}
