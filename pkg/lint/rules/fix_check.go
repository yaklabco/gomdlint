package rules

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

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
	// codeTabSpaces, when positive, lets a tab in a code block or code span
	// become that many spaces, for MD010, which replaces tabs in code like
	// markdownlint does.
	codeTabSpaces int
}

// individualFixChecks is the number of fixes a rule may report in a file up
// to which each fix is checked on its own before they are checked together.
// Past it, fixes are checked in batches that are split until each batch
// keeps the document unchanged, which keeps the cost of a file with many
// fixes close to one parse per batch.
const individualFixChecks = 16

// keepSafeFixes removes the fix from every diagnostic whose fix would change
// what a renderer produces, applied on its own or together with the other
// fixes the rule keeps. It is the last step of every fixing rule in this file
// set, so that a gap in a rule's own safety checks cannot corrupt a document.
//
// When a rule reports few fixes, each is checked on its own; with many, they
// are checked in batches, halved until a batch passes or holds one fix. Then
// the kept fixes are checked together, applied the way the fix engine applies
// them; if they change the document together, they are kept one at a time
// in document order, each only when the fixes kept before it and it still
// leave the document unchanged.
func keepSafeFixes(ctx *lint.RuleContext, diags []lint.Diagnostic, opts fixCheckOptions) []lint.Diagnostic {
	var candidates []int
	for idx := range diags {
		if len(diags[idx].FixEdits) > 0 {
			candidates = append(candidates, idx)
		}
	}
	if len(candidates) == 0 {
		return diags
	}
	flavor := config.FlavorCommonMark
	if ctx.Config != nil && ctx.Config.Flavor != "" {
		flavor = ctx.Config.Flavor
	}
	check := newFixCheck(ctx.File, flavor, opts)

	var kept []int
	if len(candidates) <= individualFixChecks {
		for _, idx := range candidates {
			if check.keeps(diags[idx].FixEdits) {
				kept = append(kept, idx)
			}
		}
	} else {
		kept = check.keptInBatches(diags, candidates)
	}
	if len(kept) > 1 && !check.keepsTogether(diags, kept) {
		var together []int
		for _, idx := range kept {
			if check.keepsTogether(diags, append(together, idx)) {
				together = append(together, idx)
			}
		}
		kept = together
	}

	keep := make(map[int]bool, len(kept))
	for _, idx := range kept {
		keep[idx] = true
	}
	for _, idx := range candidates {
		if !keep[idx] {
			diags[idx].FixEdits = nil
		}
	}
	return diags
}

// keptInBatches returns the fixes of a batch of diagnostics that keep the
// document unchanged: the whole batch when it passes together, and
// otherwise the fixes kept from each half, down to single fixes.
func (c *fixCheck) keptInBatches(diags []lint.Diagnostic, batch []int) []int {
	if c.keepsTogether(diags, batch) {
		return batch
	}
	if len(batch) == 1 {
		return nil
	}
	half := len(batch) / 2
	// The halves share their array with the batch, so the result is a copy.
	return slices.Concat(c.keptInBatches(diags, batch[:half]), c.keptInBatches(diags, batch[half:]))
}

// fixCheck verifies candidate fixes by applying them to a copy of the whole
// file, parsing it again with goldmark, and comparing render signatures (see
// fixCheck.signatures) before and after. The file is parsed in the
// configured flavor and, when that is CommonMark, in GFM too, because most
// Markdown is also read on GitHub.
//
// The check reads the complete document, so it sees the effects a fix has
// far from the text it changes: a link reference definition that links
// elsewhere use, the anchor of a heading that a fragment link points to,
// front matter that starts or ends.
type fixCheck struct {
	content   []byte
	opts      fixCheckOptions
	markdowns []goldmark.Markdown
	baseline  []string
}

func newFixCheck(file *mdast.FileSnapshot, flavor config.Flavor, opts fixCheckOptions) *fixCheck {
	markdowns := []goldmark.Markdown{goldmark.New(goldmark.WithExtensions(gmext.GFM))}
	if flavor != config.FlavorGFM {
		markdowns = append(markdowns, goldmark.New())
	}
	return &fixCheck{content: file.Content, opts: opts, markdowns: markdowns}
}

// keeps reports whether applying edits leaves the render signatures of the
// document unchanged. The edits must not overlap.
func (c *fixCheck) keeps(edits []fix.TextEdit) bool {
	if len(edits) == 0 {
		return false
	}
	prepared, err := fix.PrepareEdits(edits, len(c.content))
	if err != nil {
		return false
	}
	return c.unchanged(fix.ApplyEdits(c.content, prepared))
}

// keepsTogether reports whether the fixes of the given diagnostics, applied
// together the way the fix engine applies them, leave the render signatures
// of the document unchanged.
func (c *fixCheck) keepsTogether(diags []lint.Diagnostic, indexes []int) bool {
	var edits []fix.TextEdit
	for _, idx := range indexes {
		edits = append(edits, diags[idx].FixEdits...)
	}
	accepted, _, _, err := fix.PrepareEditsFiltered(edits, len(c.content))
	if err != nil {
		return false
	}
	return c.unchanged(fix.ApplyEdits(c.content, accepted))
}

// unchanged reports whether changed renders like the original content.
func (c *fixCheck) unchanged(changed []byte) bool {
	if c.baseline == nil {
		c.baseline = c.signatures(c.content)
	}
	candidate := c.signatures(changed)
	for idx := range candidate {
		if candidate[idx] != c.baseline[idx] {
			return false
		}
	}
	return true
}

// signatures returns the render signature of content for each parser: its
// front matter, kept as written, and the signature of the Markdown after it
// (see goldmarkSignature). A byte order mark is left out, as the parser
// leaves it out, and the front matter lines are blanked before parsing.
func (c *fixCheck) signatures(content []byte) []string {
	content = bytes.TrimPrefix(content, []byte(mdast.ByteOrderMark))
	frontMatter := frontMatterLength(content)
	source := bytes.Clone(content)
	for idx := range frontMatter {
		if source[idx] != '\n' && source[idx] != '\r' {
			source[idx] = ' '
		}
	}
	signatures := make([]string, len(c.markdowns))
	for idx, markdown := range c.markdowns {
		doc := markdown.Parser().Parse(gmtext.NewReader(source))
		signatures[idx] = fmt.Sprintf("FM%q", content[:frontMatter]) + goldmarkSignature(doc, source, c.opts)
	}
	return signatures
}

// The lines that open and close YAML and TOML front matter.
const (
	yamlFrontMatterFence = "---"
	tomlFrontMatterFence = "+++"
)

// frontMatterLength returns the number of bytes of a complete leading YAML
// or TOML metadata block, the way the parser recognizes one: a first line of
// "---", "+++" or "{", up to a closing "---" or "..." ("+++" or "}" for the
// other two), with trailing spaces allowed on both. It returns 0 when the
// content has no front matter.
func frontMatterLength(content []byte) int {
	lines := bytes.SplitAfter(content, []byte("\n"))
	if len(lines) < 2 {
		return 0
	}
	trimmed := func(line []byte) string {
		return string(bytes.TrimRight(line, " \t\r\n"))
	}
	opening := trimmed(lines[0])
	closing := opening
	switch opening {
	case yamlFrontMatterFence, tomlFrontMatterFence:
	case "{":
		closing = "}"
	default:
		return 0
	}
	length := len(lines[0])
	for _, line := range lines[1:] {
		length += len(line)
		text := trimmed(line)
		if text == closing || (opening != "{" && text == "...") {
			return length
		}
	}
	return 0
}

// goldmarkSignature describes what a renderer produces for a document parsed
// by goldmark: block kinds and nesting, heading levels and the anchors GitHub
// gives headings, list kinds, start numbers and tightness, code block info
// strings and content, table cell alignment, and inline content with its
// hard breaks, emphasis, links and images with their resolved destinations
// and titles, code spans and task checkboxes. Raw HTML, code, link titles
// and image descriptions are kept as written; runs of the whitespace HTML
// collapses (spaces, tabs, line endings and form feeds) collapse to one
// space in other text. Code may only differ in tabs that opts allow to
// become spaces.
func goldmarkSignature(doc gmast.Node, source []byte, opts fixCheckOptions) string {
	var out strings.Builder
	sig := signatureWriter{source: source, opts: opts, out: &out, anchors: make(map[string]int)}
	sig.blocks(doc)
	return out.String()
}

// signatureWriter builds a render signature. Inline text is buffered so
// that whitespace runs spanning text nodes and soft breaks collapse to one
// space.
type signatureWriter struct {
	source  []byte
	opts    fixCheckOptions
	out     *strings.Builder
	text    []byte
	anchors map[string]int
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
		fmt.Fprintf(s.out, "H%d#%s(", node.Level, s.anchor(headingText(node, s.source)))
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
		fmt.Fprintf(s.out, "C%q(%q", info, s.code(linesText(node.Lines(), s.source)))
	case *gmast.CodeBlock:
		fmt.Fprintf(s.out, "C(%q", s.code(linesText(node.Lines(), s.source)))
	case *gmast.HTMLBlock:
		raw := linesText(node.Lines(), s.source)
		if node.HasClosure() {
			raw += "\n" + strings.TrimRight(string(node.ClosureLine.Value(s.source)), "\r\n")
		}
		fmt.Fprintf(s.out, "B(%q", raw)
	case *gmextast.TableCell:
		fmt.Fprintf(s.out, "D%d(", node.Alignment)
		s.inlineContent(node)
	default:
		s.out.WriteString(node.Kind().String() + "(")
		s.blocks(node)
	}
	s.out.WriteString(")")
}

// anchor returns the anchor GitHub gives a heading with the given text: the
// slug of the text, with "-1", "-2" and so on added to repeated slugs, the
// way github-slugger counts them.
func (s *signatureWriter) anchor(text string) string {
	slug := githubSlug(text)
	unique := slug
	for {
		if _, taken := s.anchors[unique]; !taken {
			break
		}
		s.anchors[slug]++
		unique = slug + "-" + strconv.Itoa(s.anchors[slug])
	}
	s.anchors[unique] = 0
	return unique
}

// githubSlug returns the github-slugger slug of heading text: lowercased,
// with every character other than letters, marks, numbers, connector
// punctuation, spaces and hyphens removed, and each space turned into a
// hyphen.
func githubSlug(text string) string {
	var slug strings.Builder
	for _, char := range strings.ToLower(text) {
		switch {
		case char == ' ':
			slug.WriteByte('-')
		case char == '-' || unicode.IsLetter(char) || unicode.IsMark(char) || unicode.IsNumber(char) ||
			unicode.Is(unicode.Pc, char):
			slug.WriteRune(char)
		}
	}
	return slug.String()
}

// headingText returns the text content of a heading as a renderer produces
// it, the text that GitHub derives the anchor from: the text and code of
// its inline content without markup, with a line break inside the heading
// read as a line ending.
func headingText(heading gmast.Node, source []byte) string {
	var text strings.Builder
	var walk func(node gmast.Node)
	walk = func(parent gmast.Node) {
		for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
			switch node := node.(type) {
			case *gmast.Text:
				text.Write(node.Value(source))
				if node.SoftLineBreak() || node.HardLineBreak() {
					text.WriteByte('\n')
				}
			case *gmast.String:
				text.Write(node.Value)
			case *gmast.RawHTML, *gmast.Image:
			default:
				walk(node)
			}
		}
	}
	walk(heading)
	return text.String()
}

// code returns the content of a code block or code span, with the tabs that
// opts allow to change replaced by the spaces they may become.
func (s *signatureWriter) code(code string) string {
	if s.opts.codeTabSpaces > 0 {
		return strings.ReplaceAll(code, "\t", strings.Repeat(" ", s.opts.codeTabSpaces))
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
	s.out.WriteString(strings.Trim(inner.String(), " "))
}

func (s *signatureWriter) inlines(parent gmast.Node) {
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		s.inline(node)
	}
}

func (s *signatureWriter) inline(node gmast.Node) {
	switch node := node.(type) {
	case *gmast.Text:
		value := node.Value(s.source)
		switch {
		case node.HardLineBreak():
			// Renderers drop the spaces before a line break; goldmark
			// leaves some of them in the text with GFM extensions.
			s.text = bytes.TrimRight(append(s.text, value...), " \t")
			s.token("\x00br")
		case node.SoftLineBreak():
			s.text = append(append(s.text, value...), ' ')
		default:
			s.text = append(s.text, value...)
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
		s.token(fmt.Sprintf("\x00code%q", s.code(content)))
	case *gmast.RawHTML:
		var raw strings.Builder
		for idx := range node.Segments.Len() {
			segment := node.Segments.At(idx)
			raw.Write(segment.Value(s.source))
		}
		s.token(fmt.Sprintf("\x00html%q", raw.String()))
	case *gmast.AutoLink:
		s.token(fmt.Sprintf("\x00autolink%q", node.URL(s.source)))
	case *gmast.Link:
		s.token(fmt.Sprintf("\x00link%q%q(", node.Destination, node.Title))
		s.inlines(node)
		s.token(")")
	case *gmast.Image:
		s.token(fmt.Sprintf("\x00image%q%q%q", node.Destination, node.Title, exactText(node, s.source)))
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

// exactText returns the source text of the inline content of a node, such
// as the description of an image, which renderers write into an attribute
// as it is, with each line break kept.
func exactText(parent gmast.Node, source []byte) string {
	var text strings.Builder
	var walk func(node gmast.Node)
	walk = func(parent gmast.Node) {
		for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
			switch node := node.(type) {
			case *gmast.Text:
				text.Write(node.Value(source))
				if node.SoftLineBreak() || node.HardLineBreak() {
					text.WriteByte('\n')
				}
			case *gmast.String:
				text.Write(node.Value)
			default:
				fmt.Fprintf(&text, "\x00%s(", node.Kind())
				walk(node)
				text.WriteString(")")
			}
		}
	}
	walk(parent)
	return text.String()
}

// token writes buffered text, collapsed, and then a token.
func (s *signatureWriter) token(token string) {
	s.flush()
	s.out.WriteString(token)
}

// flush writes buffered text with every run of HTML whitespace collapsed to
// one space.
func (s *signatureWriter) flush() {
	if len(s.text) == 0 {
		return
	}
	space := false
	for _, char := range s.text {
		if isHTMLWhitespace(char) {
			space = true
			continue
		}
		if space {
			s.out.WriteByte(' ')
			space = false
		}
		s.out.WriteByte(char)
	}
	if space {
		s.out.WriteByte(' ')
	}
	s.text = s.text[:0]
}

// isHTMLWhitespace reports whether a byte is whitespace that HTML collapses:
// a space, tab, line feed, form feed or carriage return. A no-break space is
// not.
func isHTMLWhitespace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\n' || char == '\f' || char == '\r'
}
