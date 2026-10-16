package rules

import (
	"bytes"
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
	case titleOnly && len(h1Headings) > 0 && !headingIsFirstContent(ctx, h1Headings[0]):
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

	for _, heading := range headings {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
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
			diags = append(diags, r.createStyleDiagnostic(ctx, heading, detectedStyle, expected))
		}
	}

	return diags, nil
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
	heading *mdast.Node,
	detected, expected HeadingStyle,
) lint.Diagnostic {
	msg := fmt.Sprintf("Heading style '%s' does not match expected style '%s'", detected, expected)

	builder := lint.NewDiagnostic(r.ID(), heading, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use %s style headings", expected))

	// Only auto-fix ATX style changes (not setext conversions).
	if canAutoFix(detected, expected) {
		fixBuilder := buildHeadingStyleFix(ctx.File, heading, detected, expected)
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

// atxClosingSequence returns the range of the closing hash sequence that
// follows an ATX heading's content; ok is false when the heading is not closed.
func atxClosingSequence(file *mdast.FileSnapshot, content mdast.SourceRange) (mdast.SourceRange, bool) {
	line := lineOf(file, content.EndOffset-1)
	if line == nil {
		return mdast.SourceRange{}, false
	}
	rest := file.Content[content.EndOffset:line.NewlineStart]
	trimmed := bytes.TrimLeft(rest, " \t")
	if len(trimmed) == len(rest) {
		// The closing sequence must be preceded by whitespace.
		return mdast.SourceRange{}, false
	}
	hashes := len(trimmed) - len(bytes.TrimLeft(trimmed, "#"))
	if hashes == 0 || len(bytes.TrimSpace(trimmed[hashes:])) > 0 {
		return mdast.SourceRange{}, false
	}
	start := line.NewlineStart - len(trimmed)
	return mdast.SourceRange{StartOffset: start, EndOffset: start + hashes}, true
}

// atxOpeningWhitespace returns the whitespace between an ATX heading's
// opening hash sequence and its content, and the offset where the opening
// sequence starts.
func atxOpeningWhitespace(file *mdast.FileSnapshot, content mdast.SourceRange) (mdast.SourceRange, int) {
	wsStart := content.StartOffset
	for wsStart > 0 && (file.Content[wsStart-1] == ' ' || file.Content[wsStart-1] == '\t') {
		wsStart--
	}
	seqStart := wsStart
	for seqStart > 0 && file.Content[seqStart-1] == '#' {
		seqStart--
	}
	return mdast.SourceRange{StartOffset: wsStart, EndOffset: content.StartOffset}, seqStart
}

// detectHeadingStyle determines the style of a heading from its source.
//
// An ATX heading's content is preceded on its line by the opening hash
// sequence; a setext heading's content is preceded only by indentation and
// container markers. Closed ATX headings have a closing hash sequence after
// the content, which an escaped hash (\#) is not.
func detectHeadingStyle(file *mdast.FileSnapshot, heading *mdast.Node) HeadingStyle {
	content, ok := headingContentRange(file, heading)
	if !ok {
		return ""
	}
	line := lineOf(file, content.StartOffset)
	if line == nil {
		return ""
	}
	before := bytes.TrimRight(file.Content[line.StartOffset:content.StartOffset], " \t")
	if !bytes.HasSuffix(before, []byte("#")) {
		return StyleSetext
	}
	if _, closed := atxClosingSequence(file, content); closed {
		return StyleATXClosed
	}
	return StyleATX
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
// prefixes such as blockquote markers are preserved.
func buildHeadingStyleFix(
	file *mdast.FileSnapshot,
	heading *mdast.Node,
	from, to HeadingStyle,
) *fix.EditBuilder {
	content, ok := headingContentRange(file, heading)
	level := lint.HeadingLevel(heading)
	if !ok || level == 0 {
		return nil
	}
	line := lineOf(file, content.EndOffset-1)
	if line == nil {
		return nil
	}

	builder := fix.NewEditBuilder()
	switch {
	case from == StyleATX && to == StyleATXClosed:
		builder.ReplaceRange(content.EndOffset, line.NewlineStart, " "+strings.Repeat("#", level))
	case from == StyleATXClosed && to == StyleATX:
		if _, closed := atxClosingSequence(file, content); !closed {
			return nil
		}
		builder.Delete(content.EndOffset, line.NewlineStart)
	default:
		return nil
	}
	return builder
}
