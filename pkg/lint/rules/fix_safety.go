package rules

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// fixAllowance names the changes a rule's fix is meant to make to the parsed
// document. Everything else in the document's outline must stay the same.
type fixAllowance struct {
	// linkTextSpace lets the text of a link lose whitespace at its ends (MD039).
	linkTextSpace bool
	// codeSpanSpace lets the content of a code span lose whitespace at its ends (MD038).
	codeSpanSpace bool
	// autolinks lets text become an autolink that shows the same address (MD034).
	autolinks bool
	// codeInfo lets the info string of a code block change (MD040).
	codeInfo bool
	// prompts lets a line of code lose a leading "$" prompt (MD014).
	prompts bool
	// reversedLinks lets the text "(text)[destination]" become a link with
	// that text and destination (MD011).
	reversedLinks bool
}

// Markers that delimit the parts of an outline. They are control characters,
// so text in the document cannot be mistaken for them.
const (
	outlineOpen  = "\x01"
	outlineClose = "\x02"
)

// verifyFixes withholds every fix in diags that would change the document
// beyond what allow permits, keeping the diagnostic itself. A fix is applied
// to a copy of the content, which is parsed again with the configured flavor;
// its outline (block structure, nesting, list tightness, and text) must equal
// the original document's. All fixes are tried together first. When that
// fails, the set is split in halves until the failing fixes are found, and a
// fix is kept, in document order, only if it passes together with the fixes
// kept before it.
func verifyFixes(ctx *lint.RuleContext, diags []lint.Diagnostic, allow fixAllowance) []lint.Diagnostic {
	var fixable []int
	for idx := range diags {
		if diags[idx].HasFix() {
			fixable = append(fixable, idx)
		}
	}
	if len(fixable) == 0 || ctx.File == nil {
		return diags
	}
	newFixVerifier(ctx, allow).settle(nil, diags, fixable)
	return diags
}

// fixVerifier compares the outline of the document with candidate fixes
// applied against the original document's outline.
type fixVerifier struct {
	ctx      context.Context
	file     *mdast.FileSnapshot
	parser   *goldmark.Parser
	allow    fixAllowance
	original string
	parsed   bool // whether the original content parsed
}

func newFixVerifier(ruleCtx *lint.RuleContext, allow fixAllowance) *fixVerifier {
	flavor := ""
	if ruleCtx.Config != nil {
		flavor = string(ruleCtx.Config.Flavor)
	}
	ctx := ruleCtx.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	verifier := &fixVerifier{ctx: ctx, file: ruleCtx.File, parser: goldmark.New(flavor), allow: allow}
	// The original is parsed again with the same parser as the fixed copies,
	// so both outlines come from the same flavor.
	if original, err := verifier.parser.Parse(ctx, ruleCtx.File.Path, ruleCtx.File.Content); err == nil {
		verifier.original, verifier.parsed = documentOutline(original, allow), true
	}
	return verifier
}

// settle keeps the fixes of diags[idxs] that pass together with the edits
// already kept, withholds the others, and returns the edits kept.
func (v *fixVerifier) settle(kept []fix.TextEdit, diags []lint.Diagnostic, idxs []int) []fix.TextEdit {
	if len(idxs) == 0 {
		return kept
	}
	candidate := slices.Clone(kept)
	for _, idx := range idxs {
		candidate = append(candidate, diags[idx].FixEdits...)
	}
	if v.accepts(candidate) {
		return candidate
	}
	if len(idxs) == 1 {
		diags[idxs[0]].FixEdits = nil
		return kept
	}
	mid := len(idxs) / 2
	kept = v.settle(kept, diags, idxs[:mid])
	return v.settle(kept, diags, idxs[mid:])
}

// accepts reports whether the content with edits applied has the original
// document's outline.
func (v *fixVerifier) accepts(edits []fix.TextEdit) bool {
	if !v.parsed {
		return false
	}
	prepared, err := fix.PrepareEdits(edits, len(v.file.Content))
	if err != nil {
		return false
	}
	fixed, err := v.parser.Parse(v.ctx, v.file.Path, fix.ApplyEdits(v.file.Content, prepared))
	if err != nil {
		return false
	}
	return documentOutline(fixed, v.allow) == v.original
}

// documentOutline returns a canonical description of a parsed document: each
// block's kind, its rendering attributes (heading level, list kind, start and
// tightness, code block kind, info and code, table alignments) and children,
// and the text of its inline content. Source positions, markers and spacing
// that do not change the rendered document are left out, so two documents
// that render the same have the same outline.
func documentOutline(file *mdast.FileSnapshot, allow fixAllowance) string {
	var out strings.Builder
	writeOutline(&out, file, file.Root, allow)
	return out.String()
}

func writeOutline(out *strings.Builder, file *mdast.FileSnapshot, node *mdast.Node, allow fixAllowance) {
	switch node.Kind {
	case mdast.NodeText:
		if node.Inline != nil {
			out.Write(node.Inline.Text)
		} else {
			out.Write(node.Text())
		}
	case mdast.NodeSoftBreak:
		out.WriteString("\n")
	case mdast.NodeHardBreak:
		writeOutlineItem(out, "br", "")
	case mdast.NodeCodeSpan:
		text := ""
		if node.Inline != nil {
			text = string(node.Inline.Text)
		}
		if allow.codeSpanSpace {
			text = strings.TrimSpace(text)
		}
		writeOutlineItem(out, "code", text)
	case mdast.NodeHTMLInline, mdast.NodeTaskCheckbox:
		writeOutlineItem(out, node.Kind.String(), sourceWithoutPrefixes(file, node))
	case mdast.NodeLink, mdast.NodeImage:
		writeLinkOutline(out, file, node, allow)
	default:
		writeOutlineItem(out, node.Kind.String(), blockOutlineAttrs(file, node, allow))
		for child := node.FirstChild; child != nil; child = child.Next {
			writeOutline(out, file, child, allow)
		}
		out.WriteString(outlineClose)
	}
}

// writeOutlineItem writes the start of an outline item: its name and
// attributes. Items without children are complete; the others are closed by
// outlineClose after their children.
func writeOutlineItem(out *strings.Builder, name, attrs string) {
	fmt.Fprintf(out, "%s%s %q%s", outlineOpen, name, attrs, outlineClose)
}

// writeLinkOutline writes a link or image: its destination, title, and text.
func writeLinkOutline(out *strings.Builder, file *mdast.FileSnapshot, node *mdast.Node, allow fixAllowance) {
	if allow.autolinks && isAutolinkNode(node) {
		// An autolink shows its address, so it reads as that text.
		for child := node.FirstChild; child != nil; child = child.Next {
			writeOutline(out, file, child, allow)
		}
		return
	}
	var inner strings.Builder
	for child := node.FirstChild; child != nil; child = child.Next {
		writeOutline(&inner, file, child, allow)
	}
	text := inner.String()
	if allow.linkTextSpace {
		text = strings.TrimSpace(text)
	}
	destination, title := "", ""
	if node.Inline != nil && node.Inline.Link != nil {
		destination, title = node.Inline.Link.Destination, node.Inline.Link.Title
	}
	if allow.reversedLinks && node.Kind == mdast.NodeLink && title == "" {
		// Read a link the way its reversed form was written as text.
		fmt.Fprintf(out, "(%s)[%s]", text, destination)
		return
	}
	writeOutlineItem(out, node.Kind.String(), destination+"\x00"+title)
	out.WriteString(text)
	out.WriteString(outlineClose)
}

// blockOutlineAttrs returns the attributes of a block that change how it renders.
func blockOutlineAttrs(file *mdast.FileSnapshot, node *mdast.Node, allow fixAllowance) string {
	block := node.Block
	switch node.Kind {
	case mdast.NodeHeading:
		if block != nil {
			return fmt.Sprintf("level=%d", block.HeadingLevel)
		}
	case mdast.NodeList:
		if block != nil && block.List != nil {
			list := block.List
			start := 0
			if list.Ordered {
				start = list.StartNumber
			}
			return fmt.Sprintf("ordered=%t start=%d tight=%t", list.Ordered, start, list.Tight)
		}
	case mdast.NodeCodeBlock:
		return codeBlockOutline(file, node, allow)
	case mdast.NodeHTMLBlock, mdast.NodeRaw:
		return sourceWithoutPrefixes(file, node)
	case mdast.NodeTable:
		if block != nil && block.Table != nil {
			names := make([]string, 0, len(block.Table.Alignments))
			for _, alignment := range block.Table.Alignments {
				names = append(names, alignment.String())
			}
			return strings.Join(names, ",")
		}
	case mdast.NodeTableRow, mdast.NodeTableCell:
		if block != nil && block.Table != nil {
			return fmt.Sprintf("header=%t align=%s", block.Table.Header, block.Table.Alignment)
		}
	}
	return ""
}

// codeBlockOutline returns a code block's kind, info string, and code: each
// line without its container prefix and the block's own indentation.
func codeBlockOutline(file *mdast.FileSnapshot, node *mdast.Node, allow fixAllowance) string {
	kind := "fenced"
	if lint.IsIndentedCodeBlock(node) {
		kind = "indented"
	}
	info := lint.CodeBlockInfo(node)
	if allow.codeInfo {
		info = ""
	}
	indent := codeIndent(file, node)
	var code strings.Builder
	for _, line := range codeBlockContentLines(file, node) {
		text := stripColumns(line.content, indent)
		if allow.prompts {
			text = dollarCommandPattern.ReplaceAllString(text, "${1}")
		}
		code.WriteString(text)
		code.WriteString("\n")
	}
	return kind + "\x00" + info + "\x00" + code.String()
}

// stripColumns removes leading spaces and tabs from a line, up to the given
// width in columns, keeping the rest of a tab that reaches past it as spaces.
func stripColumns(line string, columns int) string {
	column := 0
	for idx := range len(line) {
		next := column
		switch line[idx] {
		case ' ':
			next++
		case '\t':
			next += codeTabWidth - column%codeTabWidth
		default:
			return line[idx:]
		}
		if next > columns {
			return strings.Repeat(" ", next-columns) + line[idx+1:]
		}
		column = next
	}
	return ""
}

// sourceWithoutPrefixes returns a node's source with the container prefix
// of each line after its first removed.
func sourceWithoutPrefixes(file *mdast.FileSnapshot, node *mdast.Node) string {
	span := node.SourceRange()
	pos := node.SourcePosition()
	if span.IsEmpty() || span.EndOffset > len(file.Content) || !pos.IsValid() {
		return ""
	}
	var text strings.Builder
	start := span.StartOffset
	for line := pos.StartLine; line <= pos.EndLine && line <= len(file.Lines); line++ {
		info := file.Lines[line-1]
		if line > pos.StartLine {
			start = info.StartOffset + len(lint.ContainerPrefix(file, node, line))
			text.WriteString("\n")
		}
		end := min(info.NewlineStart, span.EndOffset)
		if start < end {
			text.Write(file.Content[start:end])
		}
	}
	return text.String()
}
