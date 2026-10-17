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
		case emphasis == nil && (child.Kind == mdast.NodeEmphasis || child.Kind == mdast.NodeStrong):
			emphasis = child
		default:
			return nil
		}
	}
	return emphasis
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
// line. Runs inside emphasis, link or image text are not checked.
//
// The fix deletes the whitespace inside the markers. It is withheld when the
// content between them has no letter, such as "2 * 3 * 4", where removing the
// spaces would turn arithmetic into emphasis.
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
	diag := lint.NewDiagnosticAt(r.ID(), file.Path, pos, "Spaces inside emphasis markers").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Remove spaces from inside emphasis markers")
	if containsLetter(file.Content[open.end:closing.start]) {
		diag = diag.WithFix(builder)
	}
	return diag.Build(), true
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
// underscores cannot mark emphasis there.
//
// The fix replaces both delimiter runs. It is withheld when the new markers
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

	for _, node := range nodes {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
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

		diagBuilder := lint.NewDiagnostic(ruleID, node,
			fmt.Sprintf("%s style %q does not match expected %q", label, actual, style)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Use %q for all %s", style, suggestionNoun))
		if builder := emphasisStyleFix(ctx.File, node, open, closing, style); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
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
