package rules

import (
	"fmt"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// HeadingIncrementRule checks that heading levels increment by one.
type HeadingIncrementRule struct {
	lint.BaseRule
}

// NewHeadingIncrementRule creates a new heading increment rule.
func NewHeadingIncrementRule() *HeadingIncrementRule {
	return &HeadingIncrementRule{
		BaseRule: lint.NewBaseRule(
			"MD001",
			"heading-increment",
			"Heading levels should only increment by one level at a time",
			[]string{"headings"},
			false,
		),
	}
}

// Apply checks that heading levels increment by at most one.
func (r *HeadingIncrementRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	headings := ctx.Headings()
	if len(headings) == 0 {
		return nil, nil
	}

	var diags []lint.Diagnostic
	var prevLevel int
	// A front matter title acts as the H1, so the first heading must be H1 or H2.
	if hasFrontMatterTitle(ctx) {
		prevLevel = 1
	}

	for _, heading := range headings {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		level := lint.HeadingLevel(heading)
		if level == 0 {
			continue
		}

		// First heading can be any level.
		if prevLevel > 0 && level > prevLevel+1 {
			diag := lint.NewDiagnostic(r.ID(), heading,
				fmt.Sprintf("Heading level jumped from H%d to H%d", prevLevel, level)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use H%d instead", prevLevel+1)).
				Build()
			diags = append(diags, diag)
		}

		prevLevel = level
	}

	return diags, nil
}

// SingleH1Rule checks that there is at most one H1 heading.
type SingleH1Rule struct {
	lint.BaseRule
}

// NewSingleH1Rule creates a new single H1 rule.
func NewSingleH1Rule() *SingleH1Rule {
	return &SingleH1Rule{
		BaseRule: lint.NewBaseRule(
			"MD025",
			"single-h1",
			"Multiple top-level headings in the same document",
			[]string{"headings"},
			false,
		),
	}
}

// Apply checks that there is at most one H1 heading.
//
// Like markdownlint, a later H1 is only a violation when the document has a
// title: either a front matter title or an H1 that is the first content. The
// title_only option (default true) can be disabled to flag every H1 after the
// first regardless of where the first one appears.
func (r *SingleH1Rule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	allowNoH1 := ctx.OptionBool("allow_no_h1", true)
	level := ctx.OptionInt("level", 1)
	titleOnly := ctx.OptionBool("title_only", true)

	headings := ctx.Headings()
	var h1Headings []*mdast.Node

	for _, heading := range headings {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}

		if lint.HeadingLevel(heading) == level {
			h1Headings = append(h1Headings, heading)
		}
	}

	var diags []lint.Diagnostic
	frontMatterTitle := hasFrontMatterTitle(ctx)
	// Headings before firstDuplicate are allowed; a front matter title counts
	// as the first top-level heading, so then every matching heading is flagged.
	firstDuplicate := 1
	switch {
	case frontMatterTitle:
		firstDuplicate = 0
	case titleOnly && len(h1Headings) > 0 && !headingIsFirstContent(ctx, h1Headings[0], firstDefinitionLine(ctx)):
		return nil, nil
	}

	// Check for missing H1.
	if !allowNoH1 && len(h1Headings) == 0 && !frontMatterTitle {
		pos := mdast.SourcePosition{
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   1,
		}
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			"Document should have an H1 heading").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add an H1 heading at the beginning of the document").
			Build()
		diags = append(diags, diag)
	}

	// Flag all H1s after the first.
	for i := firstDuplicate; i < len(h1Headings); i++ {
		heading := h1Headings[i]
		number := i + 2 - firstDuplicate
		diag := lint.NewDiagnostic(r.ID(), heading,
			fmt.Sprintf("Multiple H%d headings found (this is H%d #%d)", level, level, number)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use H2 or lower for subsequent headings").
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// HeadingStyleRule enforces consistent heading style.
type HeadingStyleRule struct {
	lint.BaseRule
}

// NewHeadingStyleRule creates a new heading style rule.
func NewHeadingStyleRule() *HeadingStyleRule {
	return &HeadingStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD003",
			"heading-style",
			"Heading style should be consistent",
			[]string{"headings", "style"},
			true,
		),
	}
}

// HeadingStyle represents the style of a heading.
type HeadingStyle string

const (
	// StyleATX is the ATX style (# Heading).
	StyleATX HeadingStyle = "atx"
	// StyleATXClosed is the ATX style with closing hashes (# Heading #).
	StyleATXClosed HeadingStyle = "atx_closed"
	// StyleSetext is the setext style (underlined).
	StyleSetext HeadingStyle = "setext"
	// StyleSetextWithATX uses setext for levels 1 and 2 and ATX for deeper levels.
	StyleSetextWithATX HeadingStyle = "setext_with_atx"
	// StyleSetextWithATXClosed uses setext for levels 1 and 2 and closed ATX for deeper levels.
	StyleSetextWithATXClosed HeadingStyle = "setext_with_atx_closed"
	// StyleConsistent means use whatever style is first encountered.
	StyleConsistent HeadingStyle = "consistent"
)

// maxSetextLevel is the deepest heading level setext syntax can express.
const maxSetextLevel = 2

// Apply checks that all headings use the configured style.
//
// The style option follows markdownlint: consistent (the default) takes the
// style of the first heading, and setext_with_atx / setext_with_atx_closed
// expect setext for levels 1 and 2 and (closed) ATX for deeper levels. The
// gomdlint-only require_closing_atx option turns every expected open ATX
// heading into a closed one.
func (r *HeadingStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	style := HeadingStyle(ctx.OptionString("style", string(StyleConsistent)))
	requireClosingATX := ctx.OptionBool("require_closing_atx", false)

	headings := ctx.Headings()
	var diags []lint.Diagnostic
	checker := newFixChecker(ctx)

	for _, heading := range headings {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}

		detectedStyle := detectHeadingStyle(ctx.File, heading)
		if detectedStyle == "" {
			continue
		}

		// The first heading decides the style in consistent mode.
		if style == StyleConsistent {
			style = detectedStyle
		}

		expected := expectedHeadingStyle(style, lint.HeadingLevel(heading), requireClosingATX)
		if detectedStyle != expected {
			diags = append(diags, r.createStyleDiagnostic(ctx, checker, heading, detectedStyle, expected))
		}
	}

	return checker.filter(diags), nil
}

// expectedHeadingStyle returns the style a heading of the given level must use.
func expectedHeadingStyle(style HeadingStyle, level int, requireClosingATX bool) HeadingStyle {
	expected := style
	switch style {
	case StyleSetextWithATX:
		expected = StyleATX
		if level <= maxSetextLevel {
			expected = StyleSetext
		}
	case StyleSetextWithATXClosed:
		expected = StyleATXClosed
		if level <= maxSetextLevel {
			expected = StyleSetext
		}
	}
	if requireClosingATX && expected == StyleATX {
		expected = StyleATXClosed
	}
	return expected
}

func (r *HeadingStyleRule) createStyleDiagnostic(
	ctx *lint.RuleContext,
	checker *fixChecker,
	heading *mdast.Node,
	detected, expected HeadingStyle,
) lint.Diagnostic {
	msg := fmt.Sprintf("Heading style '%s' does not match expected style '%s'", detected, expected)

	builder := lint.NewDiagnostic(r.ID(), heading, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use %s style headings", expected))

	// Only auto-fix ATX style changes (not setext conversions).
	if canAutoFix(detected, expected) {
		fixBuilder := buildHeadingStyleFix(ctx, checker, heading, detected, expected)
		if fixBuilder != nil {
			builder = builder.WithFix(fixBuilder)
		}
	}

	return builder.Build()
}

// headingContentRange returns the byte range of a heading's inline content
// with trailing whitespace removed: for ATX headings the text between the
// opening sequence and the optional closing sequence, for setext headings the
// text lines without the underline. ok is false for empty headings and for
// headings without a usable source range.
func headingContentRange(file *mdast.FileSnapshot, heading *mdast.Node) (mdast.SourceRange, bool) {
	if file == nil || heading == nil {
		return mdast.SourceRange{}, false
	}
	rng := heading.ContentRange()
	if rng.StartOffset < 0 || rng.EndOffset > len(file.Content) || rng.StartOffset >= rng.EndOffset {
		return mdast.SourceRange{}, false
	}
	end := rng.EndOffset
	for end > rng.StartOffset && (file.Content[end-1] == ' ' || file.Content[end-1] == '\t') {
		end--
	}
	if end == rng.StartOffset {
		return mdast.SourceRange{}, false
	}
	return mdast.SourceRange{StartOffset: rng.StartOffset, EndOffset: end}, true
}

// lineOf returns the line that contains offset, or nil when it is out of range.
func lineOf(file *mdast.FileSnapshot, offset int) *mdast.LineInfo {
	lineNum, _ := file.LineAt(offset)
	if lineNum < 1 || lineNum > len(file.Lines) {
		return nil
	}
	return &file.Lines[lineNum-1]
}

// atxParts holds the syntax parts of an ATX heading line as byte ranges.
type atxParts struct {
	open       mdast.SourceRange // The opening hash sequence.
	openSpace  mdast.SourceRange // Whitespace after the opening sequence.
	content    mdast.SourceRange // The heading text; empty when there is none.
	closeSpace mdast.SourceRange // Whitespace between the text and the closing sequence.
	close      mdast.SourceRange // The closing hash sequence; empty when not closed.
}

// isSetextHeading reports whether a heading is a setext heading, whose full
// span runs from its text through its underline over more than one line.
func isSetextHeading(file *mdast.FileSnapshot, heading *mdast.Node) bool {
	full := heading.SourceRange()
	if full.IsEmpty() || full.EndOffset > len(file.Content) {
		return false
	}
	first, _ := file.LineAt(full.StartOffset)
	last, _ := file.LineAt(full.EndOffset - 1)
	return last > first
}

// atxHeadingParts splits an ATX heading's full span into its parts, so that
// empty headings such as "#" or "## ##" are classified like any other. ok is
// false for setext headings and headings without a usable span.
func atxHeadingParts(file *mdast.FileSnapshot, heading *mdast.Node) (atxParts, bool) {
	full := heading.SourceRange()
	if full.StartOffset < 0 || full.EndOffset > len(file.Content) || full.IsEmpty() ||
		file.Content[full.StartOffset] != '#' || isSetextHeading(file, heading) {
		return atxParts{}, false
	}
	content := file.Content
	isSpace := func(b byte) bool { return b == ' ' || b == '\t' }
	openEnd := full.StartOffset
	for openEnd < full.EndOffset && content[openEnd] == '#' {
		openEnd++
	}
	spaceEnd := openEnd
	for spaceEnd < full.EndOffset && isSpace(content[spaceEnd]) {
		spaceEnd++
	}
	end := full.EndOffset
	for end > spaceEnd && isSpace(content[end-1]) {
		end--
	}
	parts := atxParts{
		open:      mdast.SourceRange{StartOffset: full.StartOffset, EndOffset: openEnd},
		openSpace: mdast.SourceRange{StartOffset: openEnd, EndOffset: spaceEnd},
		content:   mdast.SourceRange{StartOffset: spaceEnd, EndOffset: spaceEnd},
	}
	if text, ok := headingContentRange(file, heading); ok {
		parts.content = text
	}

	// A closing sequence is a run of hashes that ends the line and follows
	// whitespace: after the text, or after the opening run of an empty heading.
	closeStart := parts.content.EndOffset
	for closeStart < end && isSpace(content[closeStart]) {
		closeStart++
	}
	closeEnd := closeStart
	for closeEnd < end && content[closeEnd] == '#' {
		closeEnd++
	}
	afterSpace := closeStart > parts.content.EndOffset || (parts.content.IsEmpty() && spaceEnd > openEnd)
	if closeStart < end && closeEnd == end && afterSpace {
		parts.closeSpace = mdast.SourceRange{StartOffset: parts.content.EndOffset, EndOffset: closeStart}
		parts.close = mdast.SourceRange{StartOffset: closeStart, EndOffset: end}
	}
	return parts, true
}

// detectHeadingStyle determines the style of a heading from its full span:
// setext headings span their underline, and closed ATX headings end with a
// closing hash sequence, which an escaped hash (\#) is not.
func detectHeadingStyle(file *mdast.FileSnapshot, heading *mdast.Node) HeadingStyle {
	if file == nil || heading == nil {
		return ""
	}
	if isSetextHeading(file, heading) {
		return StyleSetext
	}
	parts, ok := atxHeadingParts(file, heading)
	switch {
	case !ok:
		return ""
	case !parts.close.IsEmpty():
		return StyleATXClosed
	default:
		return StyleATX
	}
}

// allSameChar returns true if all bytes in b are the same as c.
func allSameChar(b []byte, c byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, ch := range b {
		if ch != c {
			return false
		}
	}
	return true
}

// canAutoFix returns true if we can auto-fix between these styles.
func canAutoFix(from, to HeadingStyle) bool {
	// Only fix ATX <-> ATX_closed, not setext conversions.
	return (from == StyleATX && to == StyleATXClosed) || (from == StyleATXClosed && to == StyleATX)
}

// buildHeadingStyleFix converts between open and closed ATX headings. It edits
// only the text after the heading content, so indentation and container
// prefixes such as blockquote markers are preserved. It returns nil unless
// the fix changes only the heading's style, which it does not when, for
// example, removing the closing sequence of "# Text # #" would make the
// literal "#" a closing sequence.
func buildHeadingStyleFix(
	ctx *lint.RuleContext,
	checker *fixChecker,
	heading *mdast.Node,
	from, to HeadingStyle,
) *fix.EditBuilder {
	parts, ok := atxHeadingParts(ctx.File, heading)
	level := lint.HeadingLevel(heading)
	line := lineOf(ctx.File, heading.SourceRange().StartOffset)
	if !ok || level == 0 || line == nil {
		return nil
	}
	anchor := parts.content.EndOffset
	if parts.content.IsEmpty() {
		anchor = parts.open.EndOffset
	}

	var edit fix.TextEdit
	switch {
	case from == StyleATX && to == StyleATXClosed:
		edit = fix.TextEdit{StartOffset: anchor, EndOffset: line.NewlineStart, NewText: " " + strings.Repeat("#", level)}
	case from == StyleATXClosed && to == StyleATX:
		edit = fix.TextEdit{StartOffset: anchor, EndOffset: line.NewlineStart}
	default:
		return nil
	}
	builder := fix.NewEditBuilder()
	builder.ReplaceRange(edit.StartOffset, edit.EndOffset, edit.NewText)
	return checker.offer(builder, becomes(heading, mdast.NodeHeading, headingAttrs(level, to)).withLocal(true))
}
