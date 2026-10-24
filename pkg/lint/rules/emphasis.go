package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// NoEmphasisAsHeadingRule checks for emphasis used instead of headings.
type NoEmphasisAsHeadingRule struct {
	lint.BaseRule
}

// NewNoEmphasisAsHeadingRule creates a new no-emphasis-as-heading rule.
func NewNoEmphasisAsHeadingRule() *NoEmphasisAsHeadingRule {
	return &NoEmphasisAsHeadingRule{
		BaseRule: lint.NewBaseRule(
			"MD036",
			"no-emphasis-as-heading",
			"Emphasis used instead of a heading",
			[]string{"emphasis", "headings"},
			true, // Auto-fixable - infers heading level from context.
		),
	}
}

// defaultEmphasisPunctuation is markdownlint's default punctuation that
// indicates emphasis is not a heading: normal and full-width.
const defaultEmphasisPunctuation = ".,;:!?\u3002\uFF0C\uFF1B\uFF1A\uFF01\uFF1F"

// escapeOrReferencePattern matches a backslash escape or a character
// reference. markdownlint's parser splits emphasized text at these, so it
// does not treat such emphasis as plain text.
var escapeOrReferencePattern = regexp.MustCompile(
	"\\\\[!-/:-@\\[-`{-~]|&(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[a-zA-Z][a-zA-Z0-9]{1,31});")

// Apply checks for emphasis used instead of headings.
//
// Like markdownlint, it reports a top-level paragraph whose only content is
// emphasis or strong emphasis around a single line of plain text that does
// not end in punctuation. Paragraphs in blockquotes and lists, multi-line
// emphasis, and emphasis containing code, links, escapes or other markup are
// not reported. Inline HTML (such as a trailing comment) is ignored.
//
// The fix is a gomdlint extension: a bold paragraph on a line of its own is
// replaced by an ATX heading one level below the preceding heading. It is
// withheld whenever the conversion could change the text.
func (r *NoEmphasisAsHeadingRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	punctuation := ctx.OptionString("punctuation", defaultEmphasisPunctuation)

	paragraphs := ctx.Paragraphs()
	diags := make([]lint.Diagnostic, 0, len(paragraphs))
	checker := newFixChecker(ctx)

	for _, para := range paragraphs {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}
		if para.Parent == nil || para.Parent.Kind != mdast.NodeDocument {
			continue
		}

		emphasis := soleEmphasisChild(para)
		if emphasis == nil {
			continue
		}
		text := plainEmphasisText(ctx.File.Content, emphasis)
		if text == nil {
			continue
		}

		// Text ending with punctuation reads as a sentence, not a heading.
		if lastRune, _ := utf8.DecodeLastRune(text); strings.ContainsRune(punctuation, lastRune) {
			continue
		}

		diagBuilder := lint.NewDiagnostic(r.ID(), para,
			"Emphasis used instead of a heading").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a heading instead of emphasis for section titles")

		if builder := r.buildHeadingFix(ctx, checker, para, emphasis, text); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}

		diags = append(diags, diagBuilder.Build())
	}

	return checker.filter(diags), nil
}

// soleEmphasisChild returns the paragraph's emphasis or strong child when it
// is the paragraph's only meaningful content (inline HTML and whitespace are
// ignored), or nil.
func soleEmphasisChild(para *mdast.Node) *mdast.Node {
	var emphasis *mdast.Node
	for child := para.FirstChild; child != nil; child = child.Next {
		switch {
		case child.Kind == mdast.NodeHTMLInline:
			continue
		case child.Kind == mdast.NodeText && child.Inline != nil && len(bytes.TrimSpace(child.Inline.Text)) == 0:
			continue
		case emphasis == nil && (child.Kind == mdast.NodeEmphasis || child.Kind == mdast.NodeStrong):
			emphasis = child
		default:
			return nil
		}
	}
	return emphasis
}

// plainEmphasisText returns the raw text of emphasis that holds a single
// line of plain text, or nil. markdownlint parses no strikethrough, so a
// strikethrough of plain text, tildes and all, is plain text too (see
// markdownlintChildren).
func plainEmphasisText(content []byte, emphasis *mdast.Node) []byte {
	children := markdownlintChildren(emphasis)
	if len(children) == 0 {
		return nil
	}
	for _, child := range children {
		if child.Kind != mdast.NodeText || child.Inline == nil {
			return nil
		}
	}
	text := children[0].Inline.Text
	if len(children) > 1 || children[0].Parent != emphasis {
		span := emphasis.ContentRange()
		if span.StartOffset < 0 || span.EndOffset > len(content) || span.StartOffset >= span.EndOffset {
			return nil
		}
		text = content[span.StartOffset:span.EndOffset]
	}
	if len(text) == 0 || bytes.ContainsAny(text, "\r\n") || escapeOrReferencePattern.Match(text) {
		return nil
	}
	return text
}

// buildHeadingFix replaces a bold paragraph with an ATX heading holding the
// same source text. It returns nil when the paragraph's line holds anything
// besides the bold text, or when the text ends with a hash, which the heading
// would read as a closing sequence.
func (r *NoEmphasisAsHeadingRule) buildHeadingFix(
	ctx *lint.RuleContext,
	checker *fixChecker,
	para, emphasis *mdast.Node,
	text []byte,
) *fix.EditBuilder {
	if emphasis.Kind != mdast.NodeStrong || bytes.HasSuffix(text, []byte("#")) {
		return nil
	}
	pos := para.SourcePosition()
	if !pos.IsValid() || pos.StartLine != pos.EndLine {
		return nil
	}
	line := ctx.File.Lines[pos.StartLine-1]
	source := bytes.TrimSpace(ctx.File.Content[line.StartOffset:line.NewlineStart])
	matchesDelimiters := func(delimiter string) bool {
		return bytes.Equal(source, []byte(delimiter+string(text)+delimiter))
	}
	if !matchesDelimiters("**") && !matchesDelimiters("__") {
		return nil
	}

	level := r.inferHeadingLevel(ctx, para)
	builder := fix.NewEditBuilder()
	builder.ReplaceRange(line.StartOffset, line.NewlineStart, strings.Repeat("#", level)+" "+string(text))
	full, content := emphasis.SourceRange(), emphasis.ContentRange()
	return checker.offer(builder, becomes(para, mdast.NodeHeading, headingAttrs(level, StyleATX),
		textSub{rng: mdast.SourceRange{StartOffset: full.StartOffset, EndOffset: content.StartOffset}},
		textSub{rng: mdast.SourceRange{StartOffset: content.EndOffset, EndOffset: full.EndOffset}}).withLocal(true))
}

// inferHeadingLevel determines the appropriate heading level for an emphasis paragraph.
// It scans backwards from the paragraph to find the nearest preceding heading,
// returns that heading's level + 1, caps at H6, and defaults to H2 if no heading found.
func (r *NoEmphasisAsHeadingRule) inferHeadingLevel(ctx *lint.RuleContext, para *mdast.Node) int {
	const (
		defaultLevel = 2
		maxLevel     = 6
	)

	// The headings are in document order, so the nearest one before the
	// paragraph is found by binary search.
	headings := ctx.Headings()
	paraStart := para.SourceRange().StartOffset
	nearest := sort.Search(len(headings), func(i int) bool {
		return headings[i].SourceRange().StartOffset >= paraStart
	}) - 1
	if nearest < 0 {
		return defaultLevel
	}

	// Use the nearest heading's level + 1, capped at maxLevel.
	level := lint.HeadingLevel(headings[nearest]) + 1
	if level > maxLevel {
		level = maxLevel
	}

	return level
}

// NoSpaceInEmphasisRule checks for spaces inside emphasis markers.
type NoSpaceInEmphasisRule struct {
	lint.BaseRule
}

// NewNoSpaceInEmphasisRule creates a new no-space-in-emphasis rule.
func NewNoSpaceInEmphasisRule() *NoSpaceInEmphasisRule {
	return &NoSpaceInEmphasisRule{
		BaseRule: lint.NewBaseRule(
			"MD037",
			"no-space-in-emphasis",
			"Spaces inside emphasis markers",
			[]string{"emphasis", "whitespace"},
			true,
		),
	}
}

// maxEmphasisRun is the longest delimiter run MD037 considers (***).
const maxEmphasisRun = 3

// delimiterRun is a run of emphasis markers that the parser left unmatched.
type delimiterRun struct {
	start, end int // Byte offsets of the run.
	marker     string
}

// Apply checks for spaces inside emphasis markers.
//
// Like markdownlint, it collects the delimiter runs (*, **, ***, _, __, ___)
// that the parser could not match in each block's inline content, pairs runs
// of the same marker in order, and reports a pair whose opening run is
// followed by whitespace or whose closing run is preceded by whitespace on its
// line. Runs inside emphasis, link or image text are not checked.
//
// The fix deletes the whitespace inside the markers. It is withheld when the
// content between them has no letter, such as "2 * 3 * 4", or when both
// markers are single asterisks with whitespace on both sides, such as
// "a * b * c": removing the spaces would turn arithmetic into emphasis.
func (r *NoSpaceInEmphasisRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	checker := newFixChecker(ctx)
	//nolint:errcheck // The visitor only returns cancellation errors, checked below.
	_ = mdast.Walk(ctx.Root, func(node *mdast.Node) error {
		if ctx.Cancelled() {
			return ctx.Ctx.Err()
		}
		if node.IsInline() {
			return nil
		}
		byMarker := make(map[string][]delimiterRun)
		var markers []string
		for _, run := range unmatchedDelimiterRuns(ctx.File, node) {
			if _, seen := byMarker[run.marker]; !seen {
				markers = append(markers, run.marker)
			}
			byMarker[run.marker] = append(byMarker[run.marker], run)
		}
		for _, marker := range markers {
			runs := byMarker[marker]
			for i := 0; i+1 < len(runs); i += 2 {
				diags = append(diags, r.checkPair(ctx, checker, node, runs[i], runs[i+1])...)
			}
		}
		return nil
	})
	if ctx.Cancelled() {
		return checker.filter(diags), ctx.Ctx.Err()
	}

	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].StartLine != diags[j].StartLine {
			return diags[i].StartLine < diags[j].StartLine
		}
		return diags[i].StartColumn < diags[j].StartColumn
	})
	return checker.filter(diags), nil
}

// checkPair reports a pair of runs in block with whitespace just inside them.
// Like markdownlint, the spaces on each side are a finding of their own,
// which covers them. Spaces deleted on one side alone would leave the runs
// literal, so each finding carries the fix that deletes the spaces on both.
func (r *NoSpaceInEmphasisRule) checkPair(
	ctx *lint.RuleContext,
	checker *fixChecker,
	block *mdast.Node,
	open, closing delimiterRun,
) []lint.Diagnostic {
	file := ctx.File
	openSpaces, openBad := spacesAfterRun(file, open)
	closeSpaces, closeBad := spacesBeforeRun(file, closing)
	if !openBad && !closeBad {
		return nil
	}

	// The fix turns the runs into emphasis around the text between them.
	builder := fix.NewEditBuilder()
	subs := []textSub{{rng: open.rng(), text: emphasisTokens(len(open.marker), false)}}
	var reported []mdast.SourceRange
	if openBad {
		builder.Delete(openSpaces.StartOffset, openSpaces.EndOffset)
		subs = append(subs, textSub{rng: openSpaces})
		reported = append(reported, openSpaces)
	}
	if closeBad {
		builder.Delete(closeSpaces.StartOffset, closeSpaces.EndOffset)
		subs = append(subs, textSub{rng: closeSpaces})
		reported = append(reported, closeSpaces)
	}
	subs = append(subs, textSub{rng: closing.rng(), text: emphasisTokens(len(closing.marker), true)})

	// Inside square brackets the new emphasis could change a link label, so
	// that the text would match a definition it did not match before.
	var offered *fix.EditBuilder
	if containsLetter(file.Content[open.end:closing.start]) && !spacedSingleAsterisks(file, open, closing) &&
		!insideBrackets(file, block, open.start, closing.end) {
		offered = checker.offer(builder, changeText(block, subs...).withLocal(true))
	}
	diags := make([]lint.Diagnostic, 0, len(reported))
	for _, spaces := range reported {
		diag := lint.NewDiagnosticAt(r.ID(), file.Path, rangePosition(file, spaces), "Spaces inside emphasis markers").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove spaces from inside emphasis markers")
		if offered != nil {
			diag = diag.WithFix(offered)
		}
		diags = append(diags, diag.Build())
	}
	return diags
}

// inLinkLabel reports whether inline node could be part of a link label: it
// is inside the text of a collapsed or shortcut reference link, which is the
// label, or inside square brackets that are not a link but could become one.
func inLinkLabel(ctx *lint.RuleContext, node *mdast.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeLink || parent.Kind == mdast.NodeImage {
			if parent.Inline == nil || parent.Inline.Link == nil {
				return true
			}
			style := parent.Inline.Link.ReferenceStyle
			return style == mdast.RefStyleCollapsed || style == mdast.RefStyleShortcut
		}
	}
	rng := node.SourceRange()
	return insideBrackets(ctx.File, leafBlockAt(ctx.Root, rng.StartOffset), rng.StartOffset, rng.EndOffset)
}

// insideBrackets reports whether the source from start to end lies inside
// a pair of square brackets in block, counting brackets that a backslash does
// not escape. A nil block counts as brackets.
func insideBrackets(file *mdast.FileSnapshot, block *mdast.Node, start, end int) bool {
	if block == nil {
		return true
	}
	rng := block.SourceRange()
	if rng.StartOffset < 0 || rng.EndOffset > len(file.Content) || start < rng.StartOffset || end > rng.EndOffset {
		return true
	}
	content := file.Content
	depth := 0
	for pos := rng.StartOffset; pos < start; pos++ {
		switch {
		case content[pos] == '\\':
			pos++
		case content[pos] == '[':
			depth++
		case content[pos] == ']' && depth > 0:
			depth--
		}
	}
	if depth == 0 {
		return false
	}
	open := depth
	for pos := end; pos < rng.EndOffset; pos++ {
		switch content[pos] {
		case '\\':
			pos++
		case '[':
			open++
		case ']':
			open--
			if open < depth {
				return true
			}
		}
	}
	return false
}

// rng returns the source range of a run.
func (run delimiterRun) rng() mdast.SourceRange {
	return mdast.SourceRange{StartOffset: run.start, EndOffset: run.end}
}

// emphasisTokens returns the summary tokens of the emphasis that a delimiter
// run of length opens (or, with closing, closes): one character emphasis,
// two strong emphasis, and three both, with the emphasis outside.
func emphasisTokens(length int, closing bool) string {
	var kinds []mdast.NodeKind
	switch length {
	case 1:
		kinds = []mdast.NodeKind{mdast.NodeEmphasis}
	case 2:
		kinds = []mdast.NodeKind{mdast.NodeStrong}
	default:
		kinds = []mdast.NodeKind{mdast.NodeEmphasis, mdast.NodeStrong}
	}
	if closing {
		slices.Reverse(kinds)
	}
	var tokens strings.Builder
	for _, kind := range kinds {
		tokens.WriteString(markupToken(kind, "", closing))
	}
	return tokens.String()
}

// spacedSingleAsterisks reports whether two runs are single asterisks, each
// with whitespace or the edge of the line on both sides, as in "a * b * c",
// which reads as multiplication as much as emphasis.
func spacedSingleAsterisks(file *mdast.FileSnapshot, open, closing delimiterRun) bool {
	return open.marker == "*" && closing.marker == "*" &&
		spacedOnBothSides(file, open) && spacedOnBothSides(file, closing)
}

// spacedOnBothSides reports whether a run has whitespace or the edge of its
// line on both sides.
func spacedOnBothSides(file *mdast.FileSnapshot, run delimiterRun) bool {
	line := lineOf(file, run.start)
	if line == nil {
		return false
	}
	before, _ := utf8.DecodeLastRune(file.Content[line.StartOffset:run.start])
	after, _ := utf8.DecodeRune(file.Content[run.end:line.NewlineStart])
	return (before == utf8.RuneError || unicode.IsSpace(before)) && (after == utf8.RuneError || unicode.IsSpace(after))
}

// unmatchedDelimiterRuns returns the emphasis delimiter runs in the direct
// text children of a block, which the parser left as literal text.
func unmatchedDelimiterRuns(file *mdast.FileSnapshot, block *mdast.Node) []delimiterRun {
	var runs []delimiterRun
	for child := block.FirstChild; child != nil; child = child.Next {
		switch child.Kind {
		case mdast.NodeText, mdast.NodeSoftBreak, mdast.NodeHardBreak:
		default:
			continue
		}
		start, ok := textNodeStart(file, child)
		if !ok {
			continue
		}
		text := child.Inline.Text
		for idx := 0; idx < len(text); {
			char := text[idx]
			if char == '\\' && idx+1 < len(text) && isASCIIPunctuation(text[idx+1]) {
				idx += 2
				continue
			}
			if char != '*' && char != '_' {
				idx++
				continue
			}
			runEnd := idx
			for runEnd < len(text) && text[runEnd] == char {
				runEnd++
			}
			if runEnd-idx <= maxEmphasisRun {
				runs = append(runs, delimiterRun{start: start + idx, end: start + runEnd, marker: string(text[idx:runEnd])})
			}
			idx = runEnd
		}
	}
	return runs
}

// textNodeStart returns the source offset of a node's raw inline text, or
// false when the node's source range does not hold exactly that text.
func textNodeStart(file *mdast.FileSnapshot, node *mdast.Node) (int, bool) {
	if node.Inline == nil || len(node.Inline.Text) == 0 {
		return 0, false
	}
	rng := node.SourceRange()
	if rng.StartOffset < 0 || rng.EndOffset > len(file.Content) || rng.Len() != len(node.Inline.Text) ||
		!bytes.Equal(file.Content[rng.StartOffset:rng.EndOffset], node.Inline.Text) {
		return 0, false
	}
	return rng.StartOffset, true
}

// spacesAfterRun returns the whitespace that follows a run when non-space
// text follows it on the same line.
func spacesAfterRun(file *mdast.FileSnapshot, run delimiterRun) (mdast.SourceRange, bool) {
	line := lineOf(file, run.start)
	if line == nil {
		return mdast.SourceRange{}, false
	}
	end := run.end
	for end < line.NewlineStart {
		char, size := utf8.DecodeRune(file.Content[end:line.NewlineStart])
		if !unicode.IsSpace(char) {
			break
		}
		end += size
	}
	if end == run.end || end == line.NewlineStart {
		return mdast.SourceRange{}, false
	}
	return mdast.SourceRange{StartOffset: run.end, EndOffset: end}, true
}

// spacesBeforeRun returns the whitespace that precedes a run when non-space
// text precedes it on the same line.
func spacesBeforeRun(file *mdast.FileSnapshot, run delimiterRun) (mdast.SourceRange, bool) {
	line := lineOf(file, run.start)
	if line == nil {
		return mdast.SourceRange{}, false
	}
	start := run.start
	for start > line.StartOffset {
		char, size := utf8.DecodeLastRune(file.Content[line.StartOffset:start])
		if !unicode.IsSpace(char) {
			break
		}
		start -= size
	}
	if start == run.start || start == line.StartOffset {
		return mdast.SourceRange{}, false
	}
	return mdast.SourceRange{StartOffset: start, EndOffset: run.start}, true
}

// containsLetter reports whether text contains a Unicode letter.
func containsLetter(text []byte) bool {
	return bytes.IndexFunc(text, unicode.IsLetter) >= 0
}

// isASCIIPunctuation reports whether b is an ASCII punctuation character,
// which a backslash can escape.
func isASCIIPunctuation(b byte) bool {
	return (b >= '!' && b <= '/') || (b >= ':' && b <= '@') || (b >= '[' && b <= '`') || (b >= '{' && b <= '~')
}

// EmphasisStyleRule checks for consistent emphasis style.
type EmphasisStyleRule struct {
	lint.BaseRule
}

// NewEmphasisStyleRule creates a new emphasis-style rule.
func NewEmphasisStyleRule() *EmphasisStyleRule {
	return &EmphasisStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD049",
			"emphasis-style",
			"Emphasis style should be consistent",
			[]string{"emphasis"},
			true,
		),
	}
}

// Emphasis marker styles for MD049 and MD050.
const (
	emphasisStyleAsterisk   = "asterisk"
	emphasisStyleUnderscore = "underscore"
)

// Apply checks for consistent emphasis style.
func (r *EmphasisStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return checkEmphasisStyle(ctx, r.ID(), ctx.EmphasisNodes(), 1, "Emphasis", "emphasis")
}

// StrongStyleRule checks for consistent strong (bold) style.
type StrongStyleRule struct {
	lint.BaseRule
}

// NewStrongStyleRule creates a new strong-style rule.
func NewStrongStyleRule() *StrongStyleRule {
	return &StrongStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD050",
			"strong-style",
			"Strong style should be consistent",
			[]string{"emphasis"},
			true,
		),
	}
}

// Apply checks for consistent strong style.
func (r *StrongStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return checkEmphasisStyle(ctx, r.ID(), ctx.StrongNodes(), 2, "Strong", "strong emphasis")
}

// checkEmphasisStyle implements MD049 (markerLength 1) and MD050
// (markerLength 2) like markdownlint: the style option is asterisk,
// underscore or consistent (the style of the first emphasis), and emphasis
// inside a word is never reported for the underscore style, because
// underscores cannot mark emphasis there. Each delimiter run of an emphasis
// in the wrong style is a finding of its own.
//
// The fix replaces both delimiter runs, and both findings of an emphasis
// carry it, so the engine applies it once. It is withheld when the new markers
// could pair differently: when the emphasized text holds the target marker,
// when a target marker is adjacent outside, or when an underscore would sit
// next to a letter or digit.
func checkEmphasisStyle(
	ctx *lint.RuleContext,
	ruleID string,
	nodes []*mdast.Node,
	markerLength int,
	label, suggestionNoun string,
) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	style := ctx.OptionString("style", styleConsistent)
	var diags []lint.Diagnostic
	checker := newFixChecker(ctx)

	for _, node := range nodes {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}
		open, closing, ok := emphasisDelimiters(ctx.File, node, markerLength)
		if !ok {
			continue
		}

		actual := emphasisStyleUnderscore
		if ctx.File.Content[open.StartOffset] == '*' {
			actual = emphasisStyleAsterisk
		}
		if style == styleConsistent {
			style = actual
		}
		if actual == style {
			continue
		}
		if style == emphasisStyleUnderscore && isIntraword(ctx.File.Content, open, closing) {
			continue
		}

		// New markers in a link label would change the label, so no fix is
		// offered for emphasis that could be part of one.
		var builder *fix.EditBuilder
		if !inLinkLabel(ctx, node) {
			builder = checker.offer(emphasisStyleFix(ctx.File, node, open, closing, style), fixIntent{local: true})
		}
		// Like markdownlint, each marker is reported on its own. Neither can
		// change alone, so both findings carry the fix that changes both.
		for _, marker := range []mdast.SourceRange{open, closing} {
			diagBuilder := lint.NewDiagnosticAt(ruleID, ctx.File.Path, rangePosition(ctx.File, marker),
				fmt.Sprintf("%s style %q does not match expected %q", label, actual, style)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %q for all %s", style, suggestionNoun))
			if builder != nil {
				diagBuilder = diagBuilder.WithFix(builder)
			}
			diags = append(diags, diagBuilder.Build())
		}
	}

	sortDiagnosticsByPosition(diags)
	return checker.filter(diags), nil
}

// emphasisDelimiters returns the opening and closing delimiter runs of an
// emphasis node: the parts of its full span outside its content span. ok is
// false unless both runs have markerLength copies of the same marker.
func emphasisDelimiters(
	file *mdast.FileSnapshot,
	node *mdast.Node,
	markerLength int,
) (mdast.SourceRange, mdast.SourceRange, bool) {
	full, content := node.SourceRange(), node.ContentRange()
	open := mdast.SourceRange{StartOffset: full.StartOffset, EndOffset: content.StartOffset}
	closing := mdast.SourceRange{StartOffset: content.EndOffset, EndOffset: full.EndOffset}
	if open.StartOffset < 0 || closing.EndOffset > len(file.Content) || open.EndOffset > closing.StartOffset ||
		open.Len() != markerLength || closing.Len() != markerLength {
		return mdast.SourceRange{}, mdast.SourceRange{}, false
	}
	marker := file.Content[open.StartOffset]
	if marker != '*' && marker != '_' {
		return mdast.SourceRange{}, mdast.SourceRange{}, false
	}
	for _, rng := range []mdast.SourceRange{open, closing} {
		for _, char := range file.Content[rng.StartOffset:rng.EndOffset] {
			if char != marker {
				return mdast.SourceRange{}, mdast.SourceRange{}, false
			}
		}
	}
	return open, closing, true
}

// isIntraword reports whether emphasis sits inside a word, judged like
// markdownlint by an ASCII word character just outside either delimiter.
func isIntraword(content []byte, open, closing mdast.SourceRange) bool {
	return (open.StartOffset > 0 && isASCIIWordChar(content[open.StartOffset-1])) ||
		(closing.EndOffset < len(content) && isASCIIWordChar(content[closing.EndOffset]))
}

// isASCIIWordChar matches the JavaScript \w class.
func isASCIIWordChar(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// emphasisStyleFix replaces both delimiter runs with the style's marker. It
// returns nil when the new markers could be parsed differently.
func emphasisStyleFix(
	file *mdast.FileSnapshot,
	node *mdast.Node,
	open, closing mdast.SourceRange,
	style string,
) *fix.EditBuilder {
	marker := byte('_')
	if style == emphasisStyleAsterisk {
		marker = '*'
	} else if style != emphasisStyleUnderscore {
		return nil
	}
	content := file.Content
	inner := content[open.EndOffset:closing.StartOffset]
	if len(inner) == 0 || inner[0] == marker || inner[len(inner)-1] == marker {
		return nil
	}
	if (open.StartOffset > 0 && content[open.StartOffset-1] == marker) ||
		(closing.EndOffset < len(content) && content[closing.EndOffset] == marker) {
		return nil
	}
	if marker == '_' {
		// An underscore opens and closes emphasis only next to whitespace,
		// punctuation or the edge of the line (CommonMark flanking rules),
		// so not after a letter, digit or combining mark.
		before, _ := utf8.DecodeLastRune(content[:open.StartOffset])
		after, _ := utf8.DecodeRune(content[closing.EndOffset:])
		if !isFlankingBoundary(before) || !isFlankingBoundary(after) {
			return nil
		}
	}
	literal := false
	//nolint:errcheck // Walk visitor never returns error.
	_ = mdast.Walk(node, func(child *mdast.Node) error {
		if child.Kind == mdast.NodeText && child.Inline != nil && bytes.IndexByte(child.Inline.Text, marker) >= 0 {
			literal = true
		}
		return nil
	})
	if literal {
		return nil
	}

	replacement := strings.Repeat(string(marker), open.Len())
	builder := fix.NewEditBuilder()
	builder.ReplaceRange(open.StartOffset, open.EndOffset, replacement)
	builder.ReplaceRange(closing.StartOffset, closing.EndOffset, replacement)
	return builder
}

// isFlankingBoundary reports whether r, the character next to a delimiter
// run, lets an underscore run open or close emphasis: the edge of the text
// (utf8.RuneError), whitespace, or punctuation, which CommonMark and the
// parser take to include symbols.
func isFlankingBoundary(r rune) bool {
	return r == utf8.RuneError || unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
}
