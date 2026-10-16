package rules

import (
	"bytes"
	"fmt"
	"regexp"
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
const defaultEmphasisPunctuation = ".,;:!?。，；：！？"

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

	for _, para := range paragraphs {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if para.Parent == nil || para.Parent.Kind != mdast.NodeDocument {
			continue
		}

		emphasis := soleEmphasisChild(para)
		if emphasis == nil {
			continue
		}
		text := plainEmphasisText(emphasis)
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

		if builder := r.buildHeadingFix(ctx, para, emphasis, text); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}

		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
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
		case emphasis == nil && (child.Kind == mdast.NodeEmphasis || child.Kind == mdast.NodeStrong) &&
			!isStrikethrough(child):
			emphasis = child
		default:
			return nil
		}
	}
	return emphasis
}

// isStrikethrough reports whether a node is GFM strikethrough, which the
// parser represents as emphasis.
func isStrikethrough(node *mdast.Node) bool {
	_, ok := node.Ext["strikethrough"]
	return ok
}

// plainEmphasisText returns the raw text of emphasis whose only child is a
// single line of plain text, or nil.
func plainEmphasisText(emphasis *mdast.Node) []byte {
	child := emphasis.FirstChild
	if child == nil || child.Next != nil || child.Kind != mdast.NodeText || child.Inline == nil {
		return nil
	}
	text := child.Inline.Text
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
	return builder
}

// inferHeadingLevel determines the appropriate heading level for an emphasis paragraph.
// It scans backwards from the paragraph to find the nearest preceding heading,
// returns that heading's level + 1, caps at H6, and defaults to H2 if no heading found.
func (r *NoEmphasisAsHeadingRule) inferHeadingLevel(ctx *lint.RuleContext, para *mdast.Node) int {
	const (
		defaultLevel = 2
		maxLevel     = 6
	)

	paraPos := para.SourcePosition()
	if !paraPos.IsValid() {
		return defaultLevel
	}

	headings := ctx.Headings()
	if len(headings) == 0 {
		return defaultLevel
	}

	// Find the nearest heading that appears before this paragraph.
	var nearestHeading *mdast.Node
	nearestLine := 0

	for _, heading := range headings {
		headingPos := heading.SourcePosition()
		if !headingPos.IsValid() {
			continue
		}

		// Only consider headings that come before this paragraph.
		if headingPos.StartLine < paraPos.StartLine && headingPos.StartLine > nearestLine {
			nearestHeading = heading
			nearestLine = headingPos.StartLine
		}
	}

	if nearestHeading == nil {
		return defaultLevel
	}

	// Use the nearest heading's level + 1, capped at maxLevel.
	level := lint.HeadingLevel(nearestHeading) + 1
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
// line. Runs inside emphasis, link or image text are not checked. Unlike
// markdownlint, a pair around content without any letter, such as
// "2 * 3 * 4", is read as arithmetic and not reported.
func (r *NoSpaceInEmphasisRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
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
				if diag, ok := r.checkPair(ctx.File, runs[i], runs[i+1]); ok {
					diags = append(diags, diag)
				}
			}
		}
		return nil
	})
	if ctx.Cancelled() {
		return diags, ctx.Ctx.Err()
	}

	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].StartLine != diags[j].StartLine {
			return diags[i].StartLine < diags[j].StartLine
		}
		return diags[i].StartColumn < diags[j].StartColumn
	})
	return diags, nil
}

// checkPair reports a pair of runs with whitespace just inside them.
func (r *NoSpaceInEmphasisRule) checkPair(
	file *mdast.FileSnapshot,
	open, closing delimiterRun,
) (lint.Diagnostic, bool) {
	openSpaces, openBad := spacesAfterRun(file, open)
	closeSpaces, closeBad := spacesBeforeRun(file, closing)
	if !openBad && !closeBad {
		return lint.Diagnostic{}, false
	}
	if !containsLetter(file.Content[open.end:closing.start]) {
		return lint.Diagnostic{}, false
	}

	builder := fix.NewEditBuilder()
	reported := closing
	if openBad {
		builder.Delete(openSpaces.StartOffset, openSpaces.EndOffset)
		reported = open
	}
	if closeBad {
		builder.Delete(closeSpaces.StartOffset, closeSpaces.EndOffset)
	}

	pos := rangePosition(file, mdast.SourceRange{StartOffset: reported.start, EndOffset: reported.end})
	return lint.NewDiagnosticAt(r.ID(), file.Path, pos, "Spaces inside emphasis markers").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Remove spaces from inside emphasis markers").
		WithFix(builder).
		Build(), true
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

// Apply checks for consistent emphasis style.
func (r *EmphasisStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := ctx.OptionString("style", "consistent")

	emphases := ctx.EmphasisNodes()
	if len(emphases) == 0 {
		return nil, nil
	}

	var diags []lint.Diagnostic
	var expectedStyle string

	if configStyle != "consistent" {
		expectedStyle = configStyle
	}

	for _, em := range emphases {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip strong emphasis.
		if em.Inline != nil && em.Inline.EmphasisLevel > 1 {
			continue
		}

		pos := em.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		// Detect style from the source.
		style := r.detectEmphasisStyle(ctx.File, em)
		if style == "" {
			continue
		}

		// Set expected style from first emphasis.
		if expectedStyle == "" {
			expectedStyle = style
			continue
		}

		// Check for style mismatch.
		if style != expectedStyle {
			builder := r.buildStyleFix(ctx.File, pos, style, expectedStyle)

			diag := lint.NewDiagnostic(r.ID(), em,
				fmt.Sprintf("Emphasis style %q does not match expected %q", style, expectedStyle)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %q for all emphasis", expectedStyle)).
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

func (r *EmphasisStyleRule) detectEmphasisStyle(file *mdast.FileSnapshot, em *mdast.Node) string {
	// The full span of emphasis starts with its opening marker.
	rng := em.SourceRange()
	if rng.Len() == 0 || rng.StartOffset >= len(file.Content) {
		return ""
	}

	ch := file.Content[rng.StartOffset]
	switch ch {
	case '*':
		return "asterisk"
	case '_':
		return "underscore"
	default:
		return ""
	}
}

func (r *EmphasisStyleRule) buildStyleFix(_ *mdast.FileSnapshot, _ mdast.SourcePosition, _, _ string) *fix.EditBuilder {
	// This is complex because we need to find the actual markers.
	// For now, return nil - auto-fix would need more work.
	return nil
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
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := ctx.OptionString("style", "consistent")

	strongs := ctx.StrongNodes()
	if len(strongs) == 0 {
		return nil, nil
	}

	var diags []lint.Diagnostic
	var expectedStyle string

	if configStyle != "consistent" {
		expectedStyle = configStyle
	}

	for _, st := range strongs {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		pos := st.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		// Detect style from the source.
		style := r.detectStrongStyle(ctx.File, st)
		if style == "" {
			continue
		}

		// Set expected style from first strong.
		if expectedStyle == "" {
			expectedStyle = style
			continue
		}

		// Check for style mismatch.
		if style != expectedStyle {
			builder := r.buildStyleFix(ctx.File, pos, style, expectedStyle)

			diag := lint.NewDiagnostic(r.ID(), st,
				fmt.Sprintf("Strong style %q does not match expected %q", style, expectedStyle)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %q for all strong emphasis", expectedStyle)).
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

func (r *StrongStyleRule) detectStrongStyle(file *mdast.FileSnapshot, st *mdast.Node) string {
	// The full span of strong emphasis starts with its two marker characters.
	rng := st.SourceRange()
	if rng.Len() < 2 || rng.StartOffset+2 > len(file.Content) {
		return ""
	}

	ch1 := file.Content[rng.StartOffset]
	ch2 := file.Content[rng.StartOffset+1]
	if ch1 == '*' && ch2 == '*' {
		return "asterisk"
	}
	if ch1 == '_' && ch2 == '_' {
		return "underscore"
	}
	return ""
}

func (r *StrongStyleRule) buildStyleFix(_ *mdast.FileSnapshot, _ mdast.SourcePosition, _, _ string) *fix.EditBuilder {
	// For now, return nil - auto-fix would need more work.
	return nil
}
