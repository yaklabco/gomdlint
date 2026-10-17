package rules

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	gmextast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// fixCheckOptions lists the changes a rule's fixes are meant to make beyond
// whitespace in text, which renderers do not show.
type fixCheckOptions struct {
	// codeWhitespace allows whitespace changes inside code blocks and code
	// spans, for rules that replace tabs or trailing whitespace in code.
	codeWhitespace bool
}

// keepSafeFixes removes the fix from every diagnostic whose fix, applied on
// its own to the file, would change what a renderer produces. It is the last
// step of every fixing rule in this file set, so that a gap in a rule's own
// safety checks cannot corrupt a document.
func keepSafeFixes(ctx *lint.RuleContext, diags []lint.Diagnostic, opts fixCheckOptions) []lint.Diagnostic {
	var check *fixCheck
	for idx := range diags {
		if len(diags[idx].FixEdits) == 0 {
			continue
		}
		if check == nil {
			flavor := config.FlavorCommonMark
			if ctx.Config != nil && ctx.Config.Flavor != "" {
				flavor = ctx.Config.Flavor
			}
			check = newFixCheck(ctx.File, flavor, opts)
		}
		if !check.keeps(diags[idx].FixEdits) {
			diags[idx].FixEdits = nil
		}
	}
	return diags
}

// fixCheck verifies candidate fixes by applying each one to a copy of the
// file, parsing the changed part again with goldmark in the configured
// flavor, and comparing render signatures (see goldmarkSignature) before and
// after.
//
// Only the top-level blocks around a fix are parsed again: the blocks it
// touches and one more on each side, so that a change that joins blocks or
// pulls a following block in is seen. Reference links whose definitions lie
// outside that region read as text in both parses.
type fixCheck struct {
	file      *mdast.FileSnapshot
	opts      fixCheckOptions
	markdown  goldmark.Markdown
	blocks    []lineSpan
	wholeFile bool
	baselines map[lineSpan]string
}

// lineSpan is a range of lines, both ends included.
type lineSpan struct {
	first int
	last  int
}

func newFixCheck(file *mdast.FileSnapshot, flavor config.Flavor, opts fixCheckOptions) *fixCheck {
	markdown := goldmark.New()
	if flavor == config.FlavorGFM {
		markdown = goldmark.New(goldmark.WithExtensions(gmext.GFM))
	}
	check := &fixCheck{
		file:      file,
		opts:      opts,
		markdown:  markdown,
		baselines: make(map[lineSpan]string),
	}
	if file.Root == nil {
		check.wholeFile = true
		return check
	}
	for block := file.Root.FirstChild; block != nil; block = block.Next {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			// Without a position the blocks around a fix cannot be found.
			check.wholeFile = true
			return check
		}
		check.blocks = append(check.blocks, lineSpan{first: pos.StartLine, last: nodeLastLine(block)})
	}
	return check
}

// keeps reports whether applying edits alone leaves the render signature of
// the document unchanged.
func (c *fixCheck) keeps(edits []fix.TextEdit) bool {
	if len(c.file.Lines) == 0 || len(edits) == 0 {
		return false
	}
	sorted := append([]fix.TextEdit(nil), edits...)
	fix.SortEdits(sorted)
	region := c.region(sorted[0].StartOffset, sorted[len(sorted)-1].EndOffset)
	start := c.file.Lines[region.first-1].StartOffset
	end := len(c.file.Content)
	if region.last < len(c.file.Lines) {
		end = c.file.Lines[region.last].StartOffset
	}

	shifted := make([]fix.TextEdit, len(sorted))
	for idx, edit := range sorted {
		if edit.StartOffset < start || edit.EndOffset > end || edit.StartOffset > edit.EndOffset ||
			(idx > 0 && edit.StartOffset < sorted[idx-1].EndOffset) {
			return false
		}
		shifted[idx] = fix.TextEdit{StartOffset: edit.StartOffset - start, EndOffset: edit.EndOffset - start, NewText: edit.NewText}
	}
	original := c.file.Content[start:end]
	changed := fix.ApplyEdits(original, shifted)
	if region.first == 1 {
		original, changed = c.withoutPreamble(original), c.withoutPreamble(changed)
	}

	baseline, ok := c.baselines[region]
	if !ok {
		baseline = c.signature(original)
		c.baselines[region] = baseline
	}
	return c.signature(changed) == baseline
}

// withoutPreamble blanks what the parser leaves out at the start of a file:
// a byte order mark and the front matter lines.
func (c *fixCheck) withoutPreamble(text []byte) []byte {
	text = bytes.TrimPrefix(text, []byte("\xEF\xBB\xBF"))
	if c.file.FrontMatterLines == 0 {
		return text
	}
	blanked := bytes.Clone(text)
	lines := 0
	for idx := 0; idx < len(blanked) && lines < c.file.FrontMatterLines; idx++ {
		switch blanked[idx] {
		case '\n':
			lines++
		case '\r':
		default:
			blanked[idx] = ' '
		}
	}
	return blanked
}

// signature parses text and returns its render signature.
func (c *fixCheck) signature(text []byte) string {
	doc := c.markdown.Parser().Parse(gmtext.NewReader(text))
	return goldmarkSignature(doc, text, c.opts)
}

// region returns the lines to parse again for edits between two offsets:
// the top-level blocks they touch, or the blocks around them when they fall
// between blocks, and one more block on each side. A region that reaches the
// first or last block extends to the start or end of the file.
func (c *fixCheck) region(startOffset, endOffset int) lineSpan {
	whole := lineSpan{first: 1, last: len(c.file.Lines)}
	if c.wholeFile || len(c.blocks) == 0 {
		return whole
	}
	firstLine, _ := c.file.LineAt(startOffset)
	lastLine, _ := c.file.LineAt(max(startOffset, endOffset-1))
	if firstLine < 1 || lastLine < 1 {
		return whole
	}
	// The blocks from the first one that ends on or after the first line to
	// the last one that starts on or before the last line. When the edits
	// fall between two blocks, the search finds them the other way round,
	// and the region starts from the two blocks around the edits.
	lo := sort.Search(len(c.blocks), func(idx int) bool { return c.blocks[idx].last >= firstLine })
	hi := sort.Search(len(c.blocks), func(idx int) bool { return c.blocks[idx].first > lastLine }) - 1
	if lo > hi {
		lo, hi = hi, lo
	}
	lo--
	hi++

	region := whole
	if lo >= 0 {
		region.first = c.blocks[lo].first
	}
	if hi < len(c.blocks) {
		region.last = c.blocks[hi].last
	}
	return region
}

// goldmarkSignature describes what a renderer produces for a document parsed
// by goldmark: block kinds and nesting, heading levels, list kinds, start
// numbers and tightness, code block info strings and content, raw HTML,
// table cell alignment, and inline content with its hard breaks, emphasis,
// links, images, code spans and task checkboxes. Runs of whitespace in text
// and raw HTML collapse to one space, as renderers show them; code is kept
// as written, unless opts allow whitespace changes in code.
func goldmarkSignature(doc gmast.Node, source []byte, opts fixCheckOptions) string {
	var out strings.Builder
	sig := signatureWriter{source: source, opts: opts, out: &out}
	sig.blocks(doc)
	return out.String()
}

// signatureWriter builds a render signature. Inline text is buffered so
// that whitespace runs spanning text nodes and soft breaks collapse to one
// space.
type signatureWriter struct {
	source []byte
	opts   fixCheckOptions
	out    *strings.Builder
	text   []byte
}

func (s *signatureWriter) blocks(parent gmast.Node) {
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		s.block(node)
	}
}

func (s *signatureWriter) block(node gmast.Node) {
	switch node := node.(type) {
	case *gmast.Paragraph, *gmast.TextBlock:
		s.out.WriteString("P(")
		s.inlineContent(node)
	case *gmast.Heading:
		fmt.Fprintf(s.out, "H%d(", node.Level)
		s.inlineContent(node)
	case *gmast.List:
		kind := "UL"
		if node.IsOrdered() {
			kind = fmt.Sprintf("OL%d", node.Start)
		}
		fmt.Fprintf(s.out, "%s%t(", kind, node.IsTight)
		s.blocks(node)
	case *gmast.FencedCodeBlock:
		info := ""
		if node.Info != nil {
			info = string(node.Info.Segment.Value(s.source))
		}
		fmt.Fprintf(s.out, "C%q(%q", info, s.code(node.Lines()))
	case *gmast.CodeBlock:
		fmt.Fprintf(s.out, "C(%q", s.code(node.Lines()))
	case *gmast.HTMLBlock:
		raw := linesText(node.Lines(), s.source)
		if node.HasClosure() {
			raw += "\n" + strings.TrimRight(string(node.ClosureLine.Value(s.source)), "\r\n")
		}
		fmt.Fprintf(s.out, "B(%q", collapseLineWhitespace(raw))
	case *gmextast.TableCell:
		fmt.Fprintf(s.out, "D%d(", node.Alignment)
		s.inlineContent(node)
	default:
		s.out.WriteString(node.Kind().String() + "(")
		s.blocks(node)
	}
	s.out.WriteString(")")
}

// code returns the content of a code block from its lines, with whitespace
// collapsed when opts allow whitespace changes in code.
func (s *signatureWriter) code(lines *gmtext.Segments) string {
	code := linesText(lines, s.source)
	if s.opts.codeWhitespace {
		return collapseLineWhitespace(code)
	}
	return code
}

// linesText joins the text of block lines, including the spaces that stand
// for the part of a tab that the indentation did not use. The line ending
// of the last line is left out, because a block that ends the file has none
// until a final newline is added.
func linesText(lines *gmtext.Segments, source []byte) string {
	var text strings.Builder
	for idx := range lines.Len() {
		segment := lines.At(idx)
		text.WriteString(strings.Repeat(" ", segment.Padding))
		text.Write(segment.Value(source))
	}
	return strings.TrimSuffix(strings.TrimSuffix(text.String(), "\n"), "\r")
}

// inlineContent writes the inline content of a block, trimmed.
func (s *signatureWriter) inlineContent(parent gmast.Node) {
	outer := s.out
	var inner strings.Builder
	s.out = &inner
	s.inlines(parent)
	s.flush()
	s.out = outer
	s.out.WriteString(strings.TrimSpace(inner.String()))
}

func (s *signatureWriter) inlines(parent gmast.Node) {
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		s.inline(node)
	}
}

func (s *signatureWriter) inline(node gmast.Node) {
	switch node := node.(type) {
	case *gmast.Text:
		s.text = append(s.text, node.Value(s.source)...)
		switch {
		case node.HardLineBreak():
			s.token("\x00br")
		case node.SoftLineBreak():
			s.text = append(s.text, ' ')
		}
	case *gmast.String:
		s.text = append(s.text, node.Value...)
	case *gmast.CodeSpan:
		var code strings.Builder
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if text, ok := child.(*gmast.Text); ok {
				code.Write(text.Value(s.source))
				if text.SoftLineBreak() || text.HardLineBreak() {
					code.WriteByte(' ')
				}
			}
		}
		content := strings.NewReplacer("\r\n", " ", "\n", " ").Replace(code.String())
		if s.opts.codeWhitespace {
			content = collapseWhitespace(content)
		}
		s.token(fmt.Sprintf("\x00code%q", content))
	case *gmast.RawHTML:
		var raw strings.Builder
		for idx := range node.Segments.Len() {
			segment := node.Segments.At(idx)
			raw.Write(segment.Value(s.source))
		}
		s.token(fmt.Sprintf("\x00html%q", collapseWhitespace(raw.String())))
	case *gmast.AutoLink:
		s.token(fmt.Sprintf("\x00autolink%q", node.URL(s.source)))
	case *gmast.Link:
		s.token(fmt.Sprintf("\x00link%q%q(", node.Destination, collapseWhitespace(string(node.Title))))
		s.inlines(node)
		s.token(")")
	case *gmast.Image:
		s.token(fmt.Sprintf("\x00image%q%q(", node.Destination, collapseWhitespace(string(node.Title))))
		s.inlines(node)
		s.token(")")
	case *gmast.Emphasis:
		s.token(fmt.Sprintf("\x00em%d(", node.Level))
		s.inlines(node)
		s.token(")")
	case *gmextast.TaskCheckBox:
		s.token(fmt.Sprintf("\x00task%t", node.IsChecked))
	default:
		s.token("\x00" + node.Kind().String() + "(")
		s.inlines(node)
		s.token(")")
	}
}

// token writes buffered text, collapsed, and then a token.
func (s *signatureWriter) token(token string) {
	s.flush()
	s.out.WriteString(token)
}

func (s *signatureWriter) flush() {
	if len(s.text) == 0 {
		return
	}
	text := string(s.text)
	collapsed := collapseWhitespace(text)
	if collapsed != "" && strings.TrimLeft(text, " \t\r\n") != text {
		collapsed = " " + collapsed
	}
	if strings.TrimRight(text, " \t\r\n") != text {
		collapsed += " "
	}
	if collapsed == "" {
		collapsed = " "
	}
	s.out.WriteString(collapsed)
	s.text = s.text[:0]
}

// collapseWhitespace replaces every run of spaces, tabs and line endings
// with one space and trims the ends.
func collapseWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// collapseLineWhitespace collapses runs of spaces and tabs within each line
// and trims each line, keeping the lines.
func collapseLineWhitespace(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for idx, line := range lines {
		lines[idx] = collapseWhitespace(line)
	}
	return strings.Join(lines, "\n")
}
