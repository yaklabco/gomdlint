// Package rules provides reference link/image tracking rules.
// These rules require document-wide analysis via the refs package.
package rules

import (
	"bytes"
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/refs"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// LinkFragmentsRule validates that link fragments reference valid anchors (MD051).
type LinkFragmentsRule struct {
	lint.BaseRule
}

// NewLinkFragmentsRule creates a new link fragments rule.
func NewLinkFragmentsRule() *LinkFragmentsRule {
	return &LinkFragmentsRule{
		BaseRule: lint.NewBaseRule(
			"MD051",
			"link-fragments",
			"Link fragments should be valid",
			[]string{"links"},
			false, // Not auto-fixable
		),
	}
}

// Apply checks that link fragments reference valid document anchors.
func (r *LinkFragmentsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	ignoreCase := ctx.OptionBool("ignore_case", false)
	ignoredPatternStr := ctx.OptionString("ignored_pattern", "")

	var ignoredPattern *regexp.Regexp
	if ignoredPatternStr != "" {
		var err error
		ignoredPattern, err = regexp.Compile(ignoredPatternStr)
		if err != nil {
			// Invalid pattern - ignore it
			ignoredPattern = nil
		}
	}

	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip if no fragment
		if usage.Fragment == "" {
			continue
		}

		// Only validate same-file fragments (e.g., "#heading").
		// External URLs have fragments pointing to external sites.
		// Local file paths (./file.md#anchor) would need cross-file validation.
		if !usage.IsSameFileFragment() {
			continue
		}

		fragment := strings.TrimPrefix(usage.Fragment, "#")

		// Skip if matches ignored pattern
		if ignoredPattern != nil && ignoredPattern.MatchString(fragment) {
			continue
		}

		// Validate fragment
		valid := refCtx.ValidateFragment(usage.Fragment)
		if !valid && ignoreCase {
			valid = refCtx.ValidateFragmentIgnoreCase(usage.Fragment)
		}

		if !valid {
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position,
				fmt.Sprintf("Link fragment '#%s' does not match any heading", fragment)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use a valid heading anchor").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// ReferenceLinkImagesRule validates that reference labels are defined (MD052).
type ReferenceLinkImagesRule struct {
	lint.BaseRule
}

// NewReferenceLinkImagesRule creates a new reference links/images rule.
func NewReferenceLinkImagesRule() *ReferenceLinkImagesRule {
	return &ReferenceLinkImagesRule{
		BaseRule: lint.NewBaseRule(
			"MD052",
			"reference-links-images",
			"Reference links and images should use defined labels",
			[]string{"links", "images"},
			false, // Not auto-fixable
		),
	}
}

// Apply reports full and collapsed references, and shortcut references when
// shortcut_syntax is set, whose label matches no definition. The parser turns
// every reference with a defined label into a link or image, so only the
// reference syntax it left as text can be undefined.
//
// Like markdownlint, MD052 reads GFM footnotes, which goldmark does not:
// "[^label]" calls a footnote when the document defines it with a line that
// starts "[^label]:", so it is no undefined reference. In "[text][^label]"
// the second pair calls the footnote, so the first is a shortcut reference.
func (r *ReferenceLinkImagesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	includeShortcut := ctx.OptionBool("shortcut_syntax", false)
	ignoredLabels := ctx.OptionStringSlice("ignored_labels", []string{"x"})

	// Build ignored set
	ignoredSet := make(map[string]bool)
	for _, label := range ignoredLabels {
		ignoredSet[refs.NormalizeLabel(label)] = true
	}

	var footnotes map[string]bool
	var diags []lint.Diagnostic
	math := &lazyMath{ctx: ctx}

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if usage.Label == "" {
			continue
		}

		label, normalized, style := usage.Label, usage.NormalizedLabel, usage.Style
		// A footnote definition is no link reference definition, so a
		// footnote label calls the footnote even where goldmark resolved it
		// as a link to a definition such as "[^n]: /url".
		footnote := false
		if strings.HasPrefix(label, "^") {
			if footnotes == nil {
				footnotes = gfmFootnoteLabels(ctx)
			}
			footnote = footnotes[normalized]
		}
		if !usage.Unresolved && !footnote {
			continue
		}

		// A defined label can still stay text, for example when the link
		// text contains another link, so only labels with no definition
		// are reported.
		defined := usage.ResolvedDefinition != nil
		if footnote {
			if style != refs.StyleFull || !validShortcutLabel(usage.Text) {
				continue
			}
			label, normalized, style = usage.Text, refs.NormalizeLabel(usage.Text), refs.StyleShortcut
			defined = refCtx.Definitions[normalized] != nil
		}
		if usageInMath(ctx.File, math, usage) {
			continue
		}

		// Skip shortcut unless configured to check
		if style == refs.StyleShortcut && !includeShortcut {
			continue
		}

		// Skip ignored labels
		if ignoredSet[normalized] || defined {
			continue
		}

		kind := "link"
		if usage.IsImage {
			kind = "image"
		}
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position,
			fmt.Sprintf("Reference %s uses undefined label [%s]", kind, displayLabel(label))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Define the reference or use inline syntax").
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// usageInMath reports whether a reference starts in math, which
// markdownlint parses and goldmark does not: on a line of a math block or
// in inline math.
func usageInMath(file *mdast.FileSnapshot, math *lazyMath, usage *refs.ReferenceUsage) bool {
	start, ok := file.Offset(usage.Position.StartLine, usage.Position.StartColumn)
	return ok && math.holds(usage.Position.StartLine, start, start+1)
}

// validShortcutLabel reports whether link text can be the label of a
// shortcut reference: it holds a character other than whitespace and no
// unescaped bracket.
func validShortcutLabel(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for idx := 0; idx < len(text); idx++ {
		switch text[idx] {
		case '\\':
			idx++
		case '[', ']':
			return false
		}
	}
	return true
}

// gfmFootnoteLabels returns the normalized labels, each with its "^", of
// the footnote definitions GFM reads in the file (see footnoteScan).
func gfmFootnoteLabels(ctx *lint.RuleContext) map[string]bool {
	labels := make(map[string]bool)
	first := 0
	for line := ctx.File.FrontMatterLines + 1; line <= len(ctx.File.Lines); line++ {
		if _, ok := footnoteDefinitionLabel(withoutContainerPrefix(lint.LineText(ctx.File, line))); ok {
			first = line
			break
		}
	}
	if first == 0 {
		return labels
	}
	scan := newFootnoteScan(ctx)
	for line := first; line <= len(ctx.File.Lines); line++ {
		scan.visit(line, labels)
	}
	return labels
}

// innermostContainers returns, for each line, the innermost blockquote or
// list item whose span holds it, or nil. The containers are written from
// the longest to the shortest, and from the outermost to the innermost
// among equally long ones, so an inner one overwrites the outer ones, in
// time that grows with the lines the containers hold.
func innermostContainers(ctx *lint.RuleContext) []*mdast.Node {
	type heldLines struct {
		node               *mdast.Node
		first, last, depth int
	}
	var spans []heldLines
	for _, containers := range [][]*mdast.Node{ctx.Blockquotes(), ctx.ListItems()} {
		for _, container := range containers {
			if pos := container.SourcePosition(); pos.IsValid() {
				depth := 0
				for parent := container.Parent; parent != nil; parent = parent.Parent {
					depth++
				}
				spans = append(spans, heldLines{container, pos.StartLine, pos.EndLine, depth})
			}
		}
	}
	slices.SortFunc(spans, func(a, b heldLines) int {
		return cmp.Or(cmp.Compare(b.last-b.first, a.last-a.first), cmp.Compare(a.depth, b.depth))
	})
	holders := make([]*mdast.Node, len(ctx.File.Lines)+1)
	for _, span := range spans {
		for line := max(span.first, 1); line <= span.last && line < len(holders); line++ {
			holders[line] = span.node
		}
	}
	return holders
}

// enclosingContainer returns the blockquote or list item around node, or
// nil when there is none.
func enclosingContainer(node *mdast.Node) *mdast.Node {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeBlockquote || parent.Kind == mdast.NodeListItem {
			return parent
		}
	}
	return nil
}

// maxContainerIndent is the most columns of space before the marker that
// starts a container, such as a blockquote or a footnote definition.
const maxContainerIndent = 3

// footnoteScan reads footnote definitions line by line, as
// micromark-extension-gfm-footnote reads them and goldmark does not. A
// footnote definition is a container: it starts after the prefixes of the
// containers its line continues explicitly (a lazy line is outside those it
// does not continue) and at most three columns of space, with
// "[^label]:", and its content lines are indented four columns, or are a
// lazy line of a paragraph in it. So a definition can start four to seven
// columns in, inside an open footnote, and a line after a blank line that
// is not indented as far closes the footnotes it does not reach. Indented
// code and fenced code in a footnote's content hold no definition, nor do
// goldmark's code and HTML blocks outside footnotes, or math blocks that a
// fence outside containers opens, at column 0 or indented
// (possibleMathLines).
//
// Where micromark may not read a definition, the scan counts none
// (countable): MD052 then resolves the footnote call as goldmark does, as
// main does, so counting fewer definitions only reports more calls, never
// fewer. That is where an earlier line of a footnote's content, or of a
// goldmark container around the definition, may open math or an HTML block
// over it, or where an ordered list item around it may not be a list item
// to micromark. A footnote closes after a blank line, at math that a fence
// outside containers opens, and at a less indented line that starts a block
// no lazy line continues.
type footnoteScan struct {
	ctx          *lint.RuleContext
	holders      []*mdast.Node
	htmlLines    map[int]bool
	math         mathRegions
	indentedCode map[int]bool
	// depth counts the footnote definitions that may be open, afterBlank
	// whether a blank line came after the last line read, and fence the
	// fenced code open in a footnote's content.
	depth      int
	afterBlank bool
	fence      footnoteFence
	// doubtIn tells, for each open footnote by depth from 1, whether a line
	// of its content may open math or an HTML block over what follows.
	// doubtIn[0] stands for no footnote, and len(doubtIn) is depth+1.
	doubtIn []bool
	// blockStarts holds the lines where goldmark starts a block other than
	// a paragraph, which no lazy line continues.
	blockStarts map[int]bool
	// containerMath holds, for each goldmark container, its first line that
	// opens math after the container's prefix (findContainerMath).
	containerMath map[*mdast.Node]int
}

// footnoteFence is a fenced code block in the content of the depth
// footnote definitions that hold it.
type footnoteFence struct {
	open  bool
	char  byte
	size  int
	depth int
}

// newFootnoteScan builds a scan of the file of ctx.
func newFootnoteScan(ctx *lint.RuleContext) *footnoteScan {
	scan := &footnoteScan{
		ctx:           ctx,
		holders:       innermostContainers(ctx),
		htmlLines:     htmlBlockLines(ctx),
		math:          mathRegions{lines: possibleMathLines(ctx)},
		indentedCode:  make(map[int]bool),
		doubtIn:       []bool{false},
		blockStarts:   make(map[int]bool),
		containerMath: make(map[*mdast.Node]int),
	}
	for _, blocks := range [][]*mdast.Node{
		ctx.Headings(), ctx.ThematicBreaks(), ctx.CodeBlocks(), ctx.HTMLBlocks(), ctx.ListItems(), ctx.Blockquotes(), ctx.Tables(),
	} {
		for _, block := range blocks {
			if pos := block.SourcePosition(); pos.IsValid() {
				scan.blockStarts[pos.StartLine] = true
			}
		}
	}
	scan.findContainerMath()
	for _, block := range ctx.CodeBlocks() {
		if !lint.IsFencedCodeBlock(block) {
			for line := range lint.CodeContentLines([]*mdast.Node{block}) {
				scan.indentedCode[line] = true
			}
		}
	}
	return scan
}

// visit reads one line, adding the label of a definition on it to labels.
func (s *footnoteScan) visit(line int, labels map[string]bool) {
	file := s.ctx.File
	if len(bytes.TrimSpace(withoutContainerPrefix(lint.LineText(file, line)))) == 0 {
		s.afterBlank = true
		return
	}
	if s.math.lines[line] {
		// Math that a fence outside containers opens holds no definition
		// and ends every footnote, since math allows no lazy line.
		s.closeFootnotes(0)
		s.afterBlank = false
		return
	}
	container := s.holders[line]
	for container != nil && !lint.ContinuesContainers(file, container, line) {
		container = enclosingContainer(container)
	}
	indent, start, ok := lint.IndentAfterContainers(file, container, line)
	if !ok {
		return
	}
	rest := lint.RawLineContent(file, line)[start:]
	if indent < tabStop*s.depth && (s.afterBlank || s.blockStarts[line] || breaksParagraph(rest, true)) {
		s.closeFootnotes(indent / tabStop)
	}
	s.afterBlank = false
	if s.fence.open && s.fence.depth <= s.depth {
		if indent-tabStop*s.fence.depth <= maxFenceIndent && s.fence.closedBy(rest) {
			s.fence.open = false
		}
		return
	}
	s.fence.open = false
	if s.depth > 0 && indent >= tabStop*s.depth && indent-tabStop*s.depth <= maxFenceIndent {
		if char, size, opens := footnoteFenceOpening(rest); opens {
			s.fence = footnoteFence{open: true, char: char, size: size, depth: s.depth}
			return
		}
	}
	s.noteDoubt(indent, rest)
	if s.excluded(line) {
		return
	}
	label, ok := footnoteDefinitionLabel(rest)
	if !ok || indent > maxContainerIndent+tabStop*s.depth || !s.countable(line, indent, container) {
		return
	}
	labels[refs.NormalizeLabel("^"+label)] = true
	s.closeFootnotes(indent / tabStop)
	s.depth++
	s.doubtIn = append(s.doubtIn, opensDoubt(bytes.TrimLeft(rest[len("[^")+len(label)+len("]:"):], " \t")))
}

// closeFootnotes closes the open footnotes past depth.
//
// micromark reads a line indented exactly four columns for each open
// footnote as one that continues all of them, so a fence or doubt in a
// footnote that the scan closes may go on in the one it leaves open: that
// one takes over the doubt.
func (s *footnoteScan) closeFootnotes(depth int) {
	held := s.fence.open && s.fence.depth > depth
	for _, doubt := range s.doubtIn[depth+1:] {
		held = held || doubt
	}
	s.depth = depth
	s.doubtIn = s.doubtIn[:depth+1]
	if held && depth > 0 {
		s.doubtIn[depth] = true
	}
}

// noteDoubt marks the open footnotes whose content holds a line, indent
// columns in and rest after its indentation, that may open math or an HTML
// block over the lines after it.
func (s *footnoteScan) noteDoubt(indent int, rest []byte) {
	if s.depth == 0 || !opensDoubt(rest) {
		return
	}
	for level := 1; level <= min(s.depth, indent/tabStop); level++ {
		s.doubtIn[level] = true
	}
}

// opensDoubt reports whether text, a line after its indentation, may open
// math or an HTML block in micromark.
func opensDoubt(text []byte) bool {
	if _, fence := mathFenceOpening(text); fence {
		return true
	}
	kind, _ := htmlBlockStart(text)
	return kind != htmlNone
}

// countable reports whether a footnote definition on line, indent columns
// past the prefixes of container and the containers around it, may count.
// It does not when a footnote whose content holds it has a line before it
// that may open math or an HTML block (noteDoubt); when a goldmark
// container around it has a line before it that opens math after the
// container's prefix (findContainerMath); or when a list item around it is
// in an ordered list that micromark may not read (orderedListNotFromOne).
func (s *footnoteScan) countable(line, indent int, container *mdast.Node) bool {
	for level := 1; level <= min(s.depth, indent/tabStop); level++ {
		if s.doubtIn[level] {
			return false
		}
	}
	for node := container; node != nil; node = enclosingContainer(node) {
		if node.Kind == mdast.NodeListItem && orderedListNotFromOne(s.ctx.File.Content, node) {
			return false
		}
		if first := s.containerMath[node]; first > 0 && first < line {
			return false
		}
	}
	return true
}

// findContainerMath records, for each goldmark container, its first line
// whose text after the container's prefix and at most three spaces opens
// math, in one pass over the lines that hold "$$". A line belongs to the
// innermost container it continues explicitly.
func (s *footnoteScan) findContainerMath() {
	file := s.ctx.File
	for line := file.FrontMatterLines + 1; line <= len(file.Lines); line++ {
		raw := lint.RawLineContent(file, line)
		if !bytes.Contains(raw, []byte("$$")) {
			continue
		}
		container := s.holders[line]
		for container != nil && !lint.ContinuesContainers(file, container, line) {
			container = enclosingContainer(container)
		}
		if container == nil || s.containerMath[container] > 0 {
			continue
		}
		indent, start, ok := lint.IndentAfterContainers(file, container, line)
		if _, fence := mathFenceOpening(raw[start:]); ok && fence && indent <= maxFenceIndent {
			s.containerMath[container] = line
		}
	}
}

// orderedListNotFromOne reports whether list item, read from content,
// belongs to an ordered list whose first marker is not "1." or "1)", the
// single digit 1. micromark may not start such a list after paragraph text
// or indented code, and starts every list whose first marker is. An item's
// span starts at its marker.
func orderedListNotFromOne(content []byte, item *mdast.Node) bool {
	list := item.Parent
	if list == nil || list.Block == nil || list.Block.List == nil || !list.Block.List.Ordered || list.FirstChild == nil {
		return false
	}
	at := list.FirstChild.SourceRange().StartOffset
	return at < 0 || at+1 >= len(content) || content[at] != '1' || (content[at+1] != '.' && content[at+1] != ')')
}

// excluded reports whether a line is in goldmark's code or HTML blocks, or
// in a math block, where no definition starts. In a footnote's content goldmark
// reads an indented line after a blank line as indented code, which the
// indentation rule judges instead.
func (s *footnoteScan) excluded(line int) bool {
	if s.htmlLines[line] || s.math.lines[line] {
		return true
	}
	return s.ctx.IsLineInCodeBlock(line) && (s.depth == 0 || !s.indentedCode[line])
}

// minFenceSize is the fewest backticks or tildes that open a code fence.
const minFenceSize = 3

// footnoteFenceOpening reports whether text opens a code fence, and returns
// its character and length: three or more backticks, with no backtick in
// the info string after them, or three or more tildes.
func footnoteFenceOpening(text []byte) (byte, int, bool) {
	if len(text) == 0 || (text[0] != '`' && text[0] != '~') {
		return 0, 0, false
	}
	char := text[0]
	size := 0
	for size < len(text) && text[size] == char {
		size++
	}
	if size < minFenceSize || (char == '`' && bytes.IndexByte(text[size:], '`') >= 0) {
		return 0, 0, false
	}
	return char, size, true
}

// closedBy reports whether text closes the fence: at least as many of its
// characters, then only spaces and tabs.
func (fence footnoteFence) closedBy(text []byte) bool {
	size := 0
	for size < len(text) && text[size] == fence.char {
		size++
	}
	return size >= fence.size && len(bytes.Trim(text[size:], " \t\r\n")) == 0
}

// withoutContainerPrefix returns text after the blockquote markers, list
// item markers and indentation that start it.
func withoutContainerPrefix(text []byte) []byte {
	for len(text) > 0 {
		switch ch := text[0]; {
		case ch == ' ' || ch == '\t' || ch == '>':
			text = text[1:]
		case (ch == '-' || ch == '+' || ch == '*') && len(text) > 1 && (text[1] == ' ' || text[1] == '\t'):
			text = text[2:]
		case ch >= '0' && ch <= '9':
			digits := 1
			for digits < len(text) && digits < 9 && text[digits] >= '0' && text[digits] <= '9' {
				digits++
			}
			if digits+1 >= len(text) || (text[digits] != '.' && text[digits] != ')') ||
				(text[digits+1] != ' ' && text[digits+1] != '\t') {
				return text
			}
			text = text[digits+2:]
		default:
			return text
		}
	}
	return text
}

// maxFootnoteLabel is the most characters GFM allows in a footnote label.
const maxFootnoteLabel = 999

// footnoteDefinitionLabel returns the label of a GFM footnote definition
// that starts text, "[^label]:", as written. As in micromark, the label is
// not empty, holds no space, tab, line ending or "[", and a backslash
// escapes a bracket or backslash after it.
func footnoteDefinitionLabel(text []byte) (string, bool) {
	if !bytes.HasPrefix(text, []byte("[^")) {
		return "", false
	}
	size := 0
	for idx := len("[^"); idx < len(text); {
		char, width := utf8.DecodeRune(text[idx:])
		switch {
		case size > maxFootnoteLabel:
			return "", false
		case char == ']':
			if size == 0 || idx+1 >= len(text) || text[idx+1] != ':' {
				return "", false
			}
			return string(text[len("[^"):idx]), true
		case char == '[' || char == ' ' || char == '\t' || char == '\r' || char == '\n':
			return "", false
		case char == '\\' && idx+1 < len(text) && (text[idx+1] == '[' || text[idx+1] == '\\' || text[idx+1] == ']'):
			idx += 2
			size += 2
			continue
		}
		idx += width
		size++
	}
	return "", false
}

// displayLabel returns a label for a message, with each run of whitespace,
// line breaks included, shown as one space.
func displayLabel(label string) string {
	return strings.Join(strings.Fields(label), " ")
}

// LinkImageRefDefsRule detects unused/duplicate reference definitions (MD053).
type LinkImageRefDefsRule struct {
	lint.BaseRule
}

// NewLinkImageRefDefsRule creates a new link/image reference definitions rule.
func NewLinkImageRefDefsRule() *LinkImageRefDefsRule {
	return &LinkImageRefDefsRule{
		BaseRule: lint.NewBaseRule(
			"MD053",
			"link-image-reference-definitions",
			"Link and image reference definitions should be needed",
			[]string{"links", "images"},
			true, // Auto-fixable (can remove unused/duplicate definitions)
		),
	}
}

// Apply checks for unused and duplicate reference definitions.
func (r *LinkImageRefDefsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	ignoredDefs := ctx.OptionStringSlice("ignored_definitions", []string{"//"})

	// Build ignored set
	ignoredSet := make(map[string]bool)
	for _, def := range ignoredDefs {
		ignoredSet[refs.NormalizeLabel(def)] = true
	}

	var diags []lint.Diagnostic
	removal := newDefinitionRemoval(ctx, refCtx.AllDefinitions)

	for _, def := range refCtx.AllDefinitions {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip ignored definitions
		if ignoredSet[def.NormalizedLabel] {
			continue
		}

		var msg, suggestion string
		switch {
		case def.IsDuplicate:
			msg, suggestion = "Duplicate reference definition [%s]", "Remove duplicate definition"
		case def.UsageCount == 0:
			msg, suggestion = "Unused reference definition [%s]", "Remove unused definition or add a reference"
		default:
			continue
		}
		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, def.Position, fmt.Sprintf(msg, displayLabel(def.Label))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(suggestion)
		if builder := removal.fix(def); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return verifyFixes(ctx, diags, fixAllowance{}), nil
}

// definitionRemoval builds the MD053 fix, which deletes a definition's lines.
type definitionRemoval struct {
	file *mdast.FileSnapshot
	// columns maps the first line of each definition to the column it starts at.
	columns map[int]int
	// absorbers are the lists and indented code blocks, which can take in
	// what follows them once a definition no longer separates them.
	absorbers []*mdast.Node
}

func newDefinitionRemoval(ctx *lint.RuleContext, defs []*refs.ReferenceDefinition) *definitionRemoval {
	columns := make(map[int]int, len(defs))
	for _, def := range defs {
		columns[def.Lines[0]] = definitionColumn(ctx.File, def)
	}
	return &definitionRemoval{
		file:    ctx.File,
		columns: columns,
		absorbers: mdast.FindAll(ctx.Root, func(node *mdast.Node) bool {
			return node.Kind == mdast.NodeList || lint.IsIndentedCodeBlock(node)
		}),
	}
}

// fix deletes the lines of a definition, line endings included, when nothing
// else on them would be lost and the blocks around them parse the same
// without them. A byte order mark before the first line stays.
func (d *definitionRemoval) fix(def *refs.ReferenceDefinition) *fix.EditBuilder {
	first, last := def.Lines[0], def.Lines[1]
	if !def.Removable || first < 1 || last < first || last > len(d.file.Lines) || !d.keepsStructure(def) {
		return nil
	}
	builder := fix.NewEditBuilder()
	builder.Delete(d.file.Lines[first-1].StartOffset+lint.LineTextOffset(d.file, first), d.file.Lines[last-1].EndOffset)
	return builder
}

// keepsStructure reports whether the blocks around a definition parse the
// same once its lines are deleted. The line after it continues the
// definition's paragraph, so it must be blank or start another definition at
// the same column: anything else could start a different block once it
// follows a blank line (indented code, an ordered list not starting at 1)
// or leave its blockquote as a lazy line. After a list or an indented code
// block, the next non-blank line must also start such a definition, or the
// document must end, because the list could take in a list item or
// indented text and two indented code blocks would merge into one.
func (d *definitionRemoval) keepsStructure(def *refs.ReferenceDefinition) bool {
	first, last := def.Lines[0], def.Lines[1]
	startsSibling := func(line int) bool {
		column, ok := d.columns[line]
		return ok && column == definitionColumn(d.file, def)
	}
	if next := last + 1; next <= len(d.file.Lines) && !d.isBlank(next) && !startsSibling(next) {
		return false
	}
	prev := first - 1
	for prev >= 1 && d.isBlank(prev) {
		prev--
	}
	if prev < 1 || !d.absorbs(prev, first) {
		return true
	}
	next := last + 1
	for next <= len(d.file.Lines) && d.isBlank(next) {
		next++
	}
	return next > len(d.file.Lines) || startsSibling(next)
}

// absorbs reports whether line prev belongs to a list or an indented code
// block that does not also hold line first.
func (d *definitionRemoval) absorbs(prev, first int) bool {
	for _, node := range d.absorbers {
		pos := node.SourcePosition()
		if !pos.IsValid() || prev < pos.StartLine || prev > pos.EndLine {
			continue
		}
		if first < pos.StartLine || first > pos.EndLine {
			return true
		}
	}
	return false
}

// definitionColumn returns the column a definition starts at, counted from
// the start of its line's text, so a byte order mark does not count.
func definitionColumn(file *mdast.FileSnapshot, def *refs.ReferenceDefinition) int {
	return def.Position.StartColumn - lint.LineTextOffset(file, def.Lines[0])
}

// isBlank reports whether a line holds nothing but spaces, tabs, and
// blockquote markers, so it is blank inside its blockquotes.
func (d *definitionRemoval) isBlank(line int) bool {
	return len(bytes.Trim(lint.LineContent(d.file, line), " \t\r>")) == 0
}

// LinkImageStyleRule enforces consistent link/image syntax styles (MD054).
type LinkImageStyleRule struct {
	lint.BaseRule
}

// NewLinkImageStyleRule creates a new link/image style rule.
func NewLinkImageStyleRule() *LinkImageStyleRule {
	return &LinkImageStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD054",
			"link-image-style",
			"Link and image style should be consistent",
			[]string{"links", "images"},
			false, // Not auto-fixable
		),
	}
}

// Apply checks link/image style consistency. Every style is allowed by
// default, as in markdownlint, so the rule reports nothing until an option
// disallows a style, and like markdownlint it then returns at once.
func (r *LinkImageStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Get allowed styles from config
	allowAutolink := ctx.OptionBool("autolink", true)
	allowInline := ctx.OptionBool("inline", true)
	allowFull := ctx.OptionBool("full", true)
	allowCollapsed := ctx.OptionBool("collapsed", true)
	allowShortcut := ctx.OptionBool("shortcut", true)
	allowURLInline := ctx.OptionBool("url_inline", true)
	if allowAutolink && allowInline && allowFull && allowCollapsed && allowShortcut && allowURLInline {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Reference syntax the parser left as text is not a link or image.
		if usage.Unresolved {
			continue
		}

		allowed := true
		var msg string

		switch usage.Style {
		case refs.StyleAutolink:
			if !allowAutolink {
				allowed = false
				msg = "Autolinks are not allowed"
			}
		case refs.StyleInline:
			if !allowInline {
				allowed = false
				msg = "Inline links/images are not allowed"
			} else if !allowURLInline && allowAutolink && isURLAsText(ctx.File.Content, usage) {
				allowed = false
				msg = "URL as inline link text is not allowed; use autolink syntax"
			}
		case refs.StyleFull:
			if !allowFull {
				allowed = false
				msg = "Full reference syntax is not allowed"
			}
		case refs.StyleCollapsed:
			if !allowCollapsed {
				allowed = false
				msg = "Collapsed reference syntax is not allowed"
			}
		case refs.StyleShortcut:
			if !allowShortcut {
				allowed = false
				msg = "Shortcut reference syntax is not allowed"
			}
		}

		if !allowed {
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use an allowed link/image style").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// isURLAsText reports whether an inline link could be written as an
// autolink, as markdownlint's url_inline decides it: a link, not an image,
// with no title, whose label is its destination, and whose destination an
// autolink can hold. The caller also requires autolinks to be allowed. Like
// markdownlint, it compares the label as written in content, between the
// brackets, with the destination as written, so a label with formatting or
// an escape that the destination lacks, which an autolink would lose, does
// not match.
func isURLAsText(content []byte, usage *refs.ReferenceUsage) bool {
	if usage.Style != refs.StyleInline || usage.IsImage {
		return false
	}
	node := usage.Node
	if node == nil || node.Inline == nil || node.Inline.Link == nil || node.Inline.Link.Title != "" {
		return false
	}
	start, end, ok := linkTextSpan(content, node)
	return ok && string(content[start:end]) == usage.Destination && isAutolinkDestination(usage.Destination)
}

// isAutolinkDestination reports whether a destination could be written as a
// CommonMark autolink: it starts with a scheme, as uriSchemePattern matches
// one in any case, and holds no spaces, ASCII control characters, "<" or
// ">". A protocol-relative //host/path has no scheme, so it cannot.
// markdownlint asks instead whether a browser parses the destination as a
// URL, which also accepts a one-letter scheme, such as c:x, that starts no
// autolink, and rejects http:// without a host, which an autolink can hold.
func isAutolinkDestination(dest string) bool {
	return uriSchemePattern.MatchString(dest) && !strings.ContainsFunc(dest, func(char rune) bool {
		return char == ' ' || char == '<' || char == '>' || char < 0x20 || char == 0x7f
	})
}

// DescriptiveLinkTextRule checks for generic link text (MD059).
type DescriptiveLinkTextRule struct {
	lint.BaseRule
}

// NewDescriptiveLinkTextRule creates a new descriptive link text rule.
func NewDescriptiveLinkTextRule() *DescriptiveLinkTextRule {
	return &DescriptiveLinkTextRule{
		BaseRule: lint.NewBaseRule(
			"MD059",
			"descriptive-link-text",
			"Link text should be descriptive",
			[]string{"links", "accessibility"},
			false, // Not auto-fixable
		),
	}
}

// getDefaultGenericPatterns returns markdownlint's default prohibited_texts.
func getDefaultGenericPatterns() []string {
	return []string{
		"click here",
		"here",
		"link",
		"more",
	}
}

// Apply checks for generic/non-descriptive link text. The prohibited texts
// come from patterns, which markdownlint calls prohibited_texts: the
// registry rewrites that spelling to patterns before the rule runs. With
// the default markdownlint_compatible, link text is compared like
// markdownlint: as written between the brackets, with punctuation and other
// characters that are not ASCII letters or digits ignored, and a link whose
// text holds a code span or raw HTML directly, not inside emphasis, is
// skipped. Autolinks are never checked. A finding points at the link text,
// one column after its opening bracket, as markdownlint's does.
func (r *DescriptiveLinkTextRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()

	// Get patterns from config, or use defaults
	patterns := ctx.OptionStringSlice("patterns", getDefaultGenericPatterns())
	compatible := ctx.OptionBool("markdownlint_compatible", true)

	// Build pattern set (lowercase for case-insensitive matching)
	patternSet := make(map[string]bool)
	for _, p := range patterns {
		if compatible {
			p = normalizeMarkdownlintLinkText(p)
		}
		patternSet[strings.ToLower(strings.TrimSpace(p))] = true
	}

	var diags []lint.Diagnostic
	math := &lazyMath{ctx: ctx}

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip images - they have alt text, not link text - autolinks, whose
		// text is their address, and reference syntax the parser left as
		// text, which is not a link.
		if usage.IsImage || usage.Unresolved || usage.Style == refs.StyleAutolink {
			continue
		}
		label, hasLabel := linkLabel(ctx.File, usage.Node)
		if compatible && holdsMarkdownlintChild(usage.Node, math, mdast.NodeCodeSpan, mdast.NodeHTMLInline) {
			continue
		}

		// Check if text is generic
		text := strings.ToLower(strings.TrimSpace(usage.Text))
		if compatible {
			source := usage.Text
			if hasLabel {
				source = string(ctx.File.Content[label.StartOffset:label.EndOffset])
			}
			text = normalizeMarkdownlintLinkText(source)
		}
		if text == "" {
			continue
		}

		if patternSet[text] {
			pos := usage.Position
			if hasLabel {
				pos = rangePosition(ctx.File, label)
			}
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				fmt.Sprintf("Link text '%s' is not descriptive", visibleLinkText(usage.Node))).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use descriptive text that explains the link destination").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// linkLabel returns the source range of a link's text, between its brackets.
func linkLabel(file *mdast.FileSnapshot, link *mdast.Node) (mdast.SourceRange, bool) {
	label := link.ContentRange()
	full := link.SourceRange()
	if file == nil || label.EndOffset <= label.StartOffset || label.StartOffset <= full.StartOffset ||
		label.EndOffset > len(file.Content) {
		return mdast.SourceRange{}, false
	}
	return label, true
}

// visibleLinkText returns the text a link shows, including the content of
// its code spans, for messages. A line break counts as a space.
func visibleLinkText(link *mdast.Node) string {
	var text strings.Builder
	var add func(parent *mdast.Node)
	add = func(parent *mdast.Node) {
		for child := parent.FirstChild; child != nil; child = child.Next {
			switch {
			case (child.Kind == mdast.NodeText || child.Kind == mdast.NodeCodeSpan) && child.Inline != nil:
				text.Write(child.Inline.Text)
			case child.Kind == mdast.NodeSoftBreak || child.Kind == mdast.NodeHardBreak:
				text.WriteByte(' ')
			default:
				add(child)
			}
		}
	}
	add(link)
	return text.String()
}

// markdownlintChildren returns the children of node that markdownlint's
// micromark tree gives it, in order: the node's children, with the children
// of a strikethrough in its place, because markdownlint parses no
// strikethrough.
func markdownlintChildren(node *mdast.Node) []*mdast.Node {
	var children []*mdast.Node
	for child := node.FirstChild; child != nil; child = child.Next {
		if child.Kind == mdast.NodeStrikethrough {
			children = append(children, markdownlintChildren(child)...)
			continue
		}
		children = append(children, child)
	}
	return children
}

// holdsMarkdownlintChild reports whether markdownlint's micromark tree gives
// node a child of one of kinds (see markdownlintChildren). A child in
// inline math does not count: markdownlint parses math and reads it as
// math, not as the code or HTML goldmark finds there.
func holdsMarkdownlintChild(node *mdast.Node, math *lazyMath, kinds ...mdast.NodeKind) bool {
	for _, child := range markdownlintChildren(node) {
		if !slices.Contains(kinds, child.Kind) {
			continue
		}
		if span := child.SourceRange(); !math.overlaps(span.StartOffset, span.EndOffset) {
			return true
		}
	}
	return false
}

func normalizeMarkdownlintLinkText(text string) string {
	text = strings.Map(func(char rune) rune {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			return char
		}
		return ' '
	}, text)
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
