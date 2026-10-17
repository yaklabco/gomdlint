package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// MaxLineLengthRule checks that lines do not exceed a maximum length.
type MaxLineLengthRule struct {
	lint.BaseRule
}

// NewMaxLineLengthRule creates a new max line length rule.
func NewMaxLineLengthRule() *MaxLineLengthRule {
	return &MaxLineLengthRule{
		BaseRule: lint.NewBaseRule(
			"MD013",
			"line-length",
			"Line length should not exceed the configured maximum",
			[]string{"line_length"},
			true, // Auto-fixable via line wrapping.
		),
	}
}

// defaultMaxLineLength is markdownlint's default maximum line length.
const defaultMaxLineLength = 80

// tableDelimiterRowPattern matches a GFM table delimiter row with at least one
// column separator, after any blockquote markers and indentation.
var tableDelimiterRowPattern = regexp.MustCompile(`^[ \t>]*\|?[ \t]*:?-+:?[ \t]*(\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)

// lineLengthOptions holds the MD013 settings. Each gomdlint option also
// accepts its markdownlint name; when both are set the gomdlint name wins.
type lineLengthOptions struct {
	max              int
	headingMax       int
	codeBlockMax     int
	ignoreCodeBlocks bool
	ignoreTables     bool
	ignoreHeadings   bool
	ignoreURLs       bool
	compatible       bool
	strict           bool
	stern            bool
}

func readLineLengthOptions(ctx *lint.RuleContext) lineLengthOptions {
	maxLength := optionIntAlias(ctx, "max", "line_length", defaultMaxLineLength)
	return lineLengthOptions{
		max:              maxLength,
		headingMax:       optionIntAlias(ctx, "heading_max", "heading_line_length", maxLength),
		codeBlockMax:     optionIntAlias(ctx, "code_block_max", "code_block_line_length", maxLength),
		ignoreCodeBlocks: ignoreOptionAlias(ctx, "ignore_code_blocks", "code_blocks"),
		ignoreTables:     ignoreOptionAlias(ctx, "ignore_tables", "tables"),
		ignoreHeadings:   ignoreOptionAlias(ctx, "ignore_headings", "headings"),
		ignoreURLs:       ctx.OptionBool("ignore_urls", false),
		compatible:       ctx.OptionBool("markdownlint_compatible", true),
		strict:           ctx.OptionBool("strict", false),
		stern:            ctx.OptionBool("stern", false),
	}
}

// optionIntAlias reads an integer option, falling back to its markdownlint name.
func optionIntAlias(ctx *lint.RuleContext, name, alias string, defaultValue int) int {
	if ctx.Option(name, nil) != nil {
		return ctx.OptionInt(name, defaultValue)
	}
	return ctx.OptionInt(alias, defaultValue)
}

// ignoreOptionAlias reads an ignore_* option, falling back to the inverted
// markdownlint include option. Both default to including the lines.
func ignoreOptionAlias(ctx *lint.RuleContext, name, include string) bool {
	if ctx.Option(name, nil) != nil {
		return ctx.OptionBool(name, false)
	}
	return !ctx.OptionBool(include, true)
}

// Apply checks that no line exceeds the maximum length.
//
// The defaults follow markdownlint: a limit of 80 characters (UTF-16 code
// units, as markdownlint counts them) for every line including code blocks,
// tables and headings, with markdownlint's allowance for a final run without
// whitespace, link-only lines and reference definitions. The wrap fix is a
// gomdlint addition: it only breaks paragraph text, and never where the new
// line could start a different block or where the break would change inline
// content.
func (r *MaxLineLengthRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || len(ctx.File.Lines) == 0 {
		return nil, nil
	}

	opts := readLineLengthOptions(ctx)
	scan := newLineLengthScan(ctx, opts)

	var diags []lint.Diagnostic
	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		maxLength, checked := scan.lineLimit(lineNum)
		if !checked || maxLength <= 0 {
			continue
		}
		line := string(lint.LineText(ctx.File, lineNum))
		textOffset := lint.LineTextOffset(ctx.File, lineNum)
		length := markdownlintTextLength(line)
		if length <= maxLength {
			continue
		}
		if scan.exempt(line, lineNum, maxLength) {
			continue
		}
		if opts.ignoreURLs && lint.LineContainsURL(ctx.File, lineNum) {
			continue
		}

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: textOffset + markdownlintByteLimit(line, maxLength) + 1,
			EndLine:     lineNum,
			EndColumn:   textOffset + len(line) + 1,
		}

		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Line length %d exceeds maximum %d", length, maxLength)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Shorten the line to at most %d characters", maxLength))
		if fixer := scan.wrapFix(lineNum, maxLength); fixer != nil {
			diagBuilder = diagBuilder.WithFix(fixer)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return keepSafeFixes(ctx, diags, fixCheckOptions{}), nil
}

// offsetRange is a half-open byte range of the file.
type offsetRange struct {
	start, end int
}

// lineLengthScan holds the per-file line classification MD013 needs.
type lineLengthScan struct {
	ctx         *lint.RuleContext
	opts        lineLengthOptions
	code        map[int]bool
	headings    map[int]bool
	tables      map[int]bool
	definitions map[int]bool
	linkOnly    map[int]bool
	wrappable   map[int]*mdast.Node
	noBreak     map[int][]offsetRange
}

func newLineLengthScan(ctx *lint.RuleContext, opts lineLengthOptions) *lineLengthScan {
	scan := &lineLengthScan{
		ctx:         ctx,
		opts:        opts,
		code:        markdownlintCodeLines(ctx),
		headings:    nodeLineSet(ctx.Headings()),
		tables:      nodeLineSet(ctx.Tables()),
		definitions: make(map[int]bool),
		linkOnly:    make(map[int]bool),
		wrappable:   wrappableLines(ctx),
		noBreak:     noBreakRanges(ctx),
	}
	if opts.compatible {
		scan.linkOnly = markdownlintLinkOnlyLines(ctx)
		scan.definitions = markdownlintDefinitionLines(ctx)
	}
	return scan
}

// lineLimit returns the length limit for a line, or false when the line is
// not checked.
func (s *lineLengthScan) lineLimit(lineNum int) (int, bool) {
	switch {
	case s.code[lineNum]:
		return s.opts.codeBlockMax, !s.opts.ignoreCodeBlocks
	case s.opts.ignoreTables && s.tables[lineNum]:
		return 0, false
	case s.headings[lineNum]:
		return s.opts.headingMax, !s.opts.ignoreHeadings
	default:
		return s.opts.max, true
	}
}

// exempt reports whether a line that is longer than the limit is still
// allowed: in markdownlint mode reference definitions, link-only lines and,
// unless strict, a final run without whitespace may cross the limit.
func (s *lineLengthScan) exempt(line string, lineNum, maxLength int) bool {
	if !s.opts.compatible {
		return false
	}
	if s.definitions[lineNum] {
		return true
	}
	measured, exempt := markdownlintMeasuredLength(s.opts, line, s.linkOnly[lineNum])
	return exempt || measured <= maxLength
}

// exceeds reports whether text would still be reported as too long.
func (s *lineLengthScan) exceeds(text string, maxLength int) bool {
	if !s.opts.compatible {
		return markdownlintTextLength(text) > maxLength
	}
	measured, exempt := markdownlintMeasuredLength(s.opts, text, false)
	return !exempt && measured > maxLength
}

func markdownlintMeasuredLength(opts lineLengthOptions, line string, linkOnly bool) (int, bool) {
	if opts.strict {
		return markdownlintTextLength(line), false
	}
	if linkOnly {
		return 0, true
	}
	if opts.stern {
		return markdownlintTextLength(line), isUnwrappableMarkdownlintLine(line)
	}
	prefix := strings.TrimRightFunc(line, func(char rune) bool { return !unicode.IsSpace(char) })
	return markdownlintTextLength(prefix) + 1, false
}

func markdownlintTextLength(text string) int {
	length := 0
	for _, char := range text {
		length += utf16.RuneLen(char)
	}
	return length
}

func markdownlintByteLimit(text string, limit int) int {
	length := 0
	for offset, char := range text {
		length += utf16.RuneLen(char)
		if length > limit {
			return offset
		}
	}
	return len(text)
}

// markdownlintCodeLines returns the lines of code blocks, including the fence
// lines of fenced code blocks.
func markdownlintCodeLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, block := range ctx.CodeBlocks() {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		// The full span of a fenced code block includes both fences.
		for line := pos.StartLine; line <= pos.EndLine; line++ {
			lines[line] = true
		}
	}
	return lines
}

// markdownlintLinkOnlyLines returns the lines that hold a link, image or
// autolink and no text of a paragraph, which markdownlint exempts unless
// strict. Like markdownlint, only text that is directly in a paragraph
// counts, including the space between two links; text inside emphasis,
// links, code spans and HTML does not.
func markdownlintLinkOnlyLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, links := range [][]*mdast.Node{ctx.Links(), ctx.Images()} {
		for _, link := range links {
			pos := link.SourcePosition()
			for line := pos.StartLine; pos.IsValid() && line <= pos.EndLine; line++ {
				lines[line] = true
			}
		}
	}
	for _, paragraph := range ctx.Paragraphs() {
		for child := paragraph.FirstChild; child != nil; child = child.Next {
			if child.Kind != mdast.NodeText || child.Inline == nil || len(child.Inline.Text) == 0 {
				continue
			}
			pos := child.SourcePosition()
			for line := pos.StartLine; pos.IsValid() && line <= pos.EndLine; line++ {
				delete(lines, line)
			}
		}
	}
	return lines
}

// definitionStartPattern matches the start of a link reference definition
// once the container markers of its line are removed: the bracket that opens
// its label, which may go on over the next lines.
var definitionStartPattern = regexp.MustCompile(`^ {0,3}\[`)

// containerMarkersPattern matches the blockquote markers and list markers at
// the start of a line, with the whitespace around them.
var containerMarkersPattern = regexp.MustCompile(`^(?:[ \t]*(?:>[ \t]?|(?:[-+*]|[0-9]{1,9}[.)])(?:[ \t]+|$)))*`)

// markdownlintDefinitionLines returns every line of each link reference
// definition, which markdownlint never reports. The parser keeps no node for
// a definition, so its lines are the ones that no leaf block covers: a line
// whose text starts with the "[" of a label after its container markers,
// and the lines that continue it, such as the rest of a long label, the
// destination or the title, up to a blank line or a line that a block
// covers. The label must close with "]:" within 999 characters, as
// CommonMark and markdownlint require; goldmark also accepts longer labels,
// which markdownlint reads as paragraph text.
func markdownlintDefinitionLines(ctx *lint.RuleContext) map[int]bool {
	covered := leafBlockLines(ctx.Root)
	var rests [][]byte
	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		var rest []byte
		if lineNum > ctx.File.FrontMatterLines && !covered[lineNum] {
			text := lint.LineText(ctx.File, lineNum)
			rest = text[len(containerMarkersPattern.Find(text)):]
			if len(bytes.TrimSpace(rest)) == 0 {
				rest = nil
			}
		}
		rests = append(rests, rest)
	}

	lines := make(map[int]bool)
	inDefinition := false
	for idx, rest := range rests {
		if rest == nil {
			inDefinition = false
			continue
		}
		if definitionStartPattern.Match(rest) && definitionLabelCloses(rests[idx:]) {
			inDefinition = true
		}
		if inDefinition {
			lines[idx+1] = true
		}
	}
	return lines
}

// maxLinkLabelLength is the CommonMark limit on the characters between the
// brackets of a link label.
const maxLinkLabelLength = 999

// definitionLabelCloses reports whether the label that opens on the first of
// the given lines closes with "]:" within the label length limit, before a
// line that is blank or that a block covers (nil). The lines are the text
// after the container markers. A paragraph drops the indentation of the
// lines that continue it, and each line ending counts as a character.
func definitionLabelCloses(rests [][]byte) bool {
	length := 0
	blank := true
	for idx, rest := range rests {
		if rest == nil {
			return false
		}
		text := bytes.TrimLeft(rest, " \t")
		if idx == 0 {
			text = text[1:]
		} else {
			length++
		}
		for pos := 0; pos < len(text); pos++ {
			switch char := text[pos]; {
			case char == '\\' && pos+1 < len(text):
				pos++
				length++
				blank = false
			case char == '[':
				return false
			case char == ']':
				return !blank && length <= maxLinkLabelLength && pos+1 < len(text) && text[pos+1] == ':'
			case char != ' ' && char != '\t':
				blank = false
			}
			if utf8.RuneStart(text[pos]) {
				length++
			}
		}
		if length > maxLinkLabelLength {
			return false
		}
	}
	return false
}

// leafBlockLines returns the lines that leaf blocks cover: paragraphs,
// headings, code blocks, HTML blocks, thematic breaks and tables, including
// fences and setext underlines.
func leafBlockLines(root *mdast.Node) map[int]bool {
	lines := make(map[int]bool)
	_ = mdast.Walk(root, func(node *mdast.Node) error { //nolint:errcheck // Visitor never returns an error.
		switch node.Kind {
		case mdast.NodeParagraph, mdast.NodeHeading, mdast.NodeCodeBlock, mdast.NodeHTMLBlock,
			mdast.NodeThematicBreak, mdast.NodeTable:
			pos := node.SourcePosition()
			for line := pos.StartLine; pos.IsValid() && line <= nodeLastLine(node); line++ {
				lines[line] = true
			}
		default:
		}
		return nil
	})
	return lines
}

func isUnwrappableMarkdownlintLine(line string) bool {
	line = strings.TrimLeft(line, "#> \t")
	return !strings.ContainsFunc(line, unicode.IsSpace)
}

// nodeLineSet returns the lines covered by the given nodes.
func nodeLineSet(nodes []*mdast.Node) map[int]bool {
	lines := make(map[int]bool)
	for _, node := range nodes {
		pos := node.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		for line := pos.StartLine; line <= nodeLastLine(node); line++ {
			lines[line] = true
		}
	}
	return lines
}

// wrappableLines maps the lines of paragraph text that the wrap fix may
// break to their paragraph. Paragraphs holding a table delimiter row are left
// alone: wrapping them would break the table for GFM renderers.
func wrappableLines(ctx *lint.RuleContext) map[int]*mdast.Node {
	lines := make(map[int]*mdast.Node)
	for _, paragraph := range ctx.Paragraphs() {
		pos := paragraph.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		last := nodeLastLine(paragraph)
		tableLike := false
		for line := pos.StartLine; line <= last; line++ {
			content := lint.LineText(ctx.File, line)
			if strings.Contains(string(content), "|") && tableDelimiterRowPattern.Match(content) {
				tableLike = true
				break
			}
		}
		if tableLike {
			continue
		}
		for line := pos.StartLine; line <= last; line++ {
			lines[line] = paragraph
		}
	}
	return lines
}

// taskMarkerPattern matches a GFM task list checkbox at the start of the
// content of a list item.
var taskMarkerPattern = regexp.MustCompile(`^\[[ \txX]\]`)

// noBreakRanges returns, per line, the byte ranges where inserting a line
// break would change inline content: code spans (whose whitespace is
// literal), inline HTML and the text of the elements it opens (see
// inlineElementRanges), the destination and title of links and images, and
// GFM task list checkboxes, which a break between the brackets turns into
// bracket text. Checkboxes are found in the text in every flavor, because
// GFM renderers show them even when the file is linted as CommonMark.
func noBreakRanges(ctx *lint.RuleContext) map[int][]offsetRange {
	ranges := make(map[int][]offsetRange)
	add := func(start, end int) {
		if start < 0 || end <= start {
			return
		}
		startLine, _ := ctx.File.LineAt(start)
		endLine, _ := ctx.File.LineAt(end)
		for line := startLine; line <= endLine; line++ {
			ranges[line] = append(ranges[line], offsetRange{start: start, end: end})
		}
	}
	for _, span := range ctx.CodeSpans() {
		rng := span.SourceRange()
		add(rng.StartOffset, rng.EndOffset)
	}
	for _, html := range ctx.HTMLInlines() {
		rng := html.SourceRange()
		add(rng.StartOffset, rng.EndOffset)
	}
	for _, paragraph := range ctx.Paragraphs() {
		for _, rng := range inlineElementRanges(paragraph) {
			add(rng.start, rng.end)
		}
	}
	for _, links := range [][]*mdast.Node{ctx.Links(), ctx.Images()} {
		for _, link := range links {
			// Breaking inside the link text is fine; after it come the
			// destination, title or reference label.
			rng := link.SourceRange()
			textEnd := rng.StartOffset
			if link.Inline != nil && link.Inline.Link != nil && link.Inline.Link.ReferenceStyle != mdast.RefStyleAutolink {
				textEnd = link.ContentRange().EndOffset
			}
			add(textEnd, rng.EndOffset)
		}
	}
	for _, item := range ctx.ListItems() {
		marker, found := findListItemMarker(ctx.File, item)
		if found && marker.HasContent() && taskMarkerPattern.Match(ctx.File.Content[marker.ContentStart:marker.LineEnd]) {
			add(marker.ContentStart, marker.ContentStart+len("[ ]"))
		}
	}
	return ranges
}

// inlineElementRanges returns the ranges of the elements that inline raw
// HTML opens in a paragraph, from the start tag through the matching end
// tag, or through the end of the paragraph, which closes them, when the end
// tag is missing. CSS or the element itself can keep the whitespace of their
// text, so a line break there could show.
func inlineElementRanges(paragraph *mdast.Node) []offsetRange {
	type openTag struct {
		name  string
		start int
	}
	var open []openTag
	var ranges []offsetRange
	_ = mdast.Walk(paragraph, func(node *mdast.Node) error { //nolint:errcheck // Visitor never returns an error.
		if node.Kind != mdast.NodeHTMLInline {
			return nil
		}
		rng := node.SourceRange()
		for _, tag := range scanHTMLTags(string(node.Text())) {
			switch {
			case tag.closing:
				for idx := len(open) - 1; idx >= 0; idx-- {
					if open[idx].name == tag.name {
						ranges = append(ranges, offsetRange{start: open[idx].start, end: rng.EndOffset})
						open = open[:idx]
						break
					}
				}
			case !tag.selfClosing && !isVoidElement(tag.name):
				open = append(open, openTag{name: tag.name, start: rng.StartOffset})
			}
		}
		return nil
	})
	for _, tag := range open {
		ranges = append(ranges, offsetRange{start: tag.start, end: paragraph.SourceRange().EndOffset})
	}
	return ranges
}

// wrapFix builds a fix that breaks a long paragraph line at whitespace, as
// often as needed for every resulting line to fit. Each break replaces a run
// of whitespace with a line ending and the continuation prefix of the
// paragraph's containers (blockquote markers and list item content
// indentation). A break is skipped when the new line would
// start a block (a list item, heading, blockquote, fence, thematic break,
// HTML block or table row), when it would follow a backslash (a hard line
// break), or when it falls inside a code span, inline HTML or a link
// destination. It returns nil when no safe break brings the line closer to
// the limit.
func (s *lineLengthScan) wrapFix(lineNum, maxLength int) *fix.EditBuilder {
	paragraph := s.wrappable[lineNum]
	if paragraph == nil {
		return nil
	}
	info := s.ctx.File.Lines[lineNum-1]
	textOffset := lint.LineTextOffset(s.ctx.File, lineNum)
	textStart := info.StartOffset + textOffset
	content := string(s.ctx.File.Content[textStart:info.NewlineStart])
	if isTableLine(content) {
		return nil
	}
	containerEnd := len(lint.ContainerPrefix(s.ctx.File, paragraph, lineNum)) - textOffset
	contentStart := containerEnd
	for contentStart < len(content) && (content[contentStart] == ' ' || content[contentStart] == '\t') {
		contentStart++
	}
	// Continuation lines repeat the containers and the line's own indentation.
	prefix := lint.ContinuationPrefix(s.ctx.File, paragraph) + content[containerEnd:contentStart]
	runs := whitespaceRuns(content)

	builder := fix.NewEditBuilder()
	segment := 0
	for {
		current := content
		if segment > 0 {
			current = prefix + content[segment:]
		}
		if !s.exceeds(current, maxLength) {
			break
		}
		best := -1
		for idx, run := range runs {
			if run.start <= max(segment, contentStart) || run.end >= len(content) {
				continue
			}
			head := content[:run.start]
			if segment > 0 {
				head = prefix + content[segment:run.start]
			}
			if markdownlintTextLength(head) > maxLength {
				break
			}
			if s.safeBreak(lineNum, textStart, content, run) {
				best = idx
			}
		}
		if best < 0 {
			break
		}
		run := runs[best]
		builder.ReplaceRange(textStart+run.start, textStart+run.end, "\n"+prefix)
		segment = run.end
	}
	if len(builder.Edits) == 0 {
		return nil
	}
	return builder
}

// safeBreak reports whether a whitespace run may become a line break.
func (s *lineLengthScan) safeBreak(lineNum, lineStart int, content string, run offsetRange) bool {
	if content[run.start-1] == '\\' || wrapContinuationStartsBlock(content[run.end:]) {
		return false
	}
	offset := lineStart + run.start
	for _, rng := range s.noBreak[lineNum] {
		if rng.start <= offset && offset < rng.end {
			return false
		}
	}
	return true
}

// whitespaceRuns returns the runs of spaces and tabs in a line.
func whitespaceRuns(content string) []offsetRange {
	var runs []offsetRange
	for idx := 0; idx < len(content); idx++ {
		if content[idx] != ' ' && content[idx] != '\t' {
			continue
		}
		start := idx
		for idx < len(content) && (content[idx] == ' ' || content[idx] == '\t') {
			idx++
		}
		runs = append(runs, offsetRange{start: start, end: idx})
	}
	return runs
}

// wrapContinuationStartsBlock reports whether text placed at the start of a
// continuation line could be read as the start of a block instead of
// paragraph text.
func wrapContinuationStartsBlock(rest string) bool {
	if rest == "" {
		return true
	}
	first := rest[0]
	switch first {
	case '#', '>', '=', '-', '+', '|':
		return true
	case '*', '_':
		return len(rest) == 1 || rest[1] == ' ' || rest[1] == '\t' || rest[1] == first
	case '`', '~':
		return strings.HasPrefix(rest, strings.Repeat(string(first), 3))
	case '<':
		return len(rest) > 1 && (isASCIILetter(rest[1]) || rest[1] == '/' || rest[1] == '!' || rest[1] == '?')
	case ':':
		return len(rest) > 1 && rest[1] == '-'
	}
	digits := 0
	for digits < len(rest) && isASCIIDigit(rest[digits]) {
		digits++
	}
	if digits == 0 || digits > maxOrderedMarkerDigits || digits == len(rest) || (rest[digits] != '.' && rest[digits] != ')') {
		return false
	}
	return digits+1 == len(rest) || rest[digits+1] == ' ' || rest[digits+1] == '\t'
}

// linePrefix extracts the prefix for continuation lines.
// Returns the prefix string and the start position of actual content.
func linePrefix(line string) (string, int) {
	pos := 0
	lineLen := len(line)
	var prefixBuilder strings.Builder

	// Skip leading whitespace.
	for pos < lineLen && (line[pos] == ' ' || line[pos] == '\t') {
		_ = prefixBuilder.WriteByte(line[pos]) // strings.Builder.WriteByte never fails
		pos++
	}
	leadingSpace := prefixBuilder.String()
	prefixBuilder.Reset()
	prefixBuilder.WriteString(leadingSpace)

	// Check for blockquote prefix.
	if pos < lineLen && line[pos] == '>' {
		_ = prefixBuilder.WriteByte('>') // strings.Builder.WriteByte never fails
		pos++
		// Skip space after >.
		if pos < lineLen && line[pos] == ' ' {
			_ = prefixBuilder.WriteByte(' ') // strings.Builder.WriteByte never fails
			pos++
		}
		// Recursively check for nested structures.
		nestedPrefix, nestedStart := linePrefix(line[pos:])
		prefixBuilder.WriteString(nestedPrefix)
		return prefixBuilder.String(), pos + nestedStart
	}

	// Check for list markers (-, *, +, or number.).
	listStart := pos
	if pos < lineLen && (line[pos] == '-' || line[pos] == '*' || line[pos] == '+') {
		pos++
		if pos < lineLen && line[pos] == ' ' {
			// List item: continuation uses spaces to align.
			markerLen := pos - listStart + 1
			prefixBuilder.WriteString(strings.Repeat(" ", markerLen))
			pos++
			return prefixBuilder.String(), pos
		}
		pos = listStart // Not a list marker, reset.
	}

	// Check for numbered list (1. or 1)).
	if pos < lineLen && line[pos] >= '0' && line[pos] <= '9' {
		for pos < lineLen && line[pos] >= '0' && line[pos] <= '9' {
			pos++
		}
		if pos < lineLen && (line[pos] == '.' || line[pos] == ')') {
			pos++
			if pos < lineLen && line[pos] == ' ' {
				markerLen := pos - listStart + 1
				prefixBuilder.WriteString(strings.Repeat(" ", markerLen))
				pos++
				return prefixBuilder.String(), pos
			}
		}
		// Not a numbered list, fall through to return leading space.
	}

	// Plain paragraph - no special prefix needed for continuation.
	return leadingSpace, len(leadingSpace)
}

// isTableLine checks if a line is part of a table.
func isTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) > 0 && trimmed[0] == '|'
}
