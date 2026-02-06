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
func (r *SingleH1Rule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	allowNoH1 := ctx.OptionBool("allow_no_h1", true)

	headings := ctx.Headings()
	var h1Headings []*mdast.Node

	for _, heading := range headings {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}

		if lint.HeadingLevel(heading) == 1 {
			h1Headings = append(h1Headings, heading)
		}
	}

	var diags []lint.Diagnostic

	// Check for missing H1.
	if !allowNoH1 && len(h1Headings) == 0 {
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
	for i := 1; i < len(h1Headings); i++ {
		heading := h1Headings[i]
		diag := lint.NewDiagnostic(r.ID(), heading,
			fmt.Sprintf("Multiple H1 headings found (this is H1 #%d)", i+1)).
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
	// StyleConsistent means use whatever style is first encountered.
	StyleConsistent HeadingStyle = "consistent"
)

// Apply checks that all headings use a consistent style.
func (r *HeadingStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := HeadingStyle(ctx.OptionString("style", string(StyleATX)))
	requireClosingATX := ctx.OptionBool("require_closing_atx", false)

	// Determine effective style.
	effectiveStyle := configStyle
	if configStyle == StyleConsistent {
		effectiveStyle = "" // Will be set from first heading.
	}

	// If requiring closing ATX, the effective style is atx_closed.
	if requireClosingATX && (configStyle == StyleATX || configStyle == StyleConsistent) {
		if configStyle != StyleConsistent {
			effectiveStyle = StyleATXClosed
		}
	}

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

		// Set consistent style from first heading.
		if effectiveStyle == "" {
			effectiveStyle = detectedStyle
			if requireClosingATX && effectiveStyle == StyleATX {
				effectiveStyle = StyleATXClosed
			}
			continue
		}

		// Check for style mismatch.
		if !stylesMatch(detectedStyle, effectiveStyle, requireClosingATX) {
			diag := r.createStyleDiagnostic(ctx, heading, detectedStyle, effectiveStyle, requireClosingATX)
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

func (r *HeadingStyleRule) createStyleDiagnostic(
	ctx *lint.RuleContext,
	heading *mdast.Node,
	detected, expected HeadingStyle,
	requireClosingATX bool,
) lint.Diagnostic {
	msg := fmt.Sprintf("Heading style '%s' does not match expected style '%s'", detected, expected)

	builder := lint.NewDiagnostic(r.ID(), heading, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use %s style headings", expected))

	// Only auto-fix ATX style changes (not setext conversions).
	if canAutoFix(detected, expected) {
		fixBuilder := buildHeadingStyleFix(ctx.File, heading, detected, expected, requireClosingATX)
		if fixBuilder != nil {
			builder = builder.WithFix(fixBuilder)
		}
	}

	return builder.Build()
}

// detectHeadingStyle determines the style of a heading from its source.
func detectHeadingStyle(file *mdast.FileSnapshot, heading *mdast.Node) HeadingStyle {
	if heading == nil || file == nil {
		return ""
	}

	pos := heading.SourcePosition()
	if pos.StartLine < 1 || pos.StartLine > len(file.Lines) {
		return ""
	}

	lineContent := lint.LineContent(file, pos.StartLine)
	if len(lineContent) == 0 {
		return ""
	}

	// Check if it starts with # (ATX style).
	trimmed := bytes.TrimLeft(lineContent, " \t")
	if len(trimmed) > 0 && trimmed[0] == '#' {
		// Check if it ends with # (closed ATX).
		trimmedLine := bytes.TrimSpace(lineContent)
		if len(trimmedLine) > 1 && trimmedLine[len(trimmedLine)-1] == '#' {
			// Find the content between opening and closing #s.
			// Remove leading #s and spaces.
			afterOpen := bytes.TrimLeft(trimmedLine, "#")
			afterOpen = bytes.TrimLeft(afterOpen, " \t")
			// Remove trailing #s and spaces.
			beforeClose := bytes.TrimRight(afterOpen, "#")
			beforeClose = bytes.TrimRight(beforeClose, " \t")
			// If there's content between, it's closed style.
			if len(beforeClose) > 0 {
				return StyleATXClosed
			}
		}
		return StyleATX
	}

	// Check for setext style (heading followed by === or ---).
	if pos.EndLine > pos.StartLine && pos.EndLine <= len(file.Lines) {
		underline := lint.LineContent(file, pos.EndLine)
		trimmedUnderline := bytes.TrimSpace(underline)
		if len(trimmedUnderline) > 0 {
			if allSameChar(trimmedUnderline, '=') || allSameChar(trimmedUnderline, '-') {
				return StyleSetext
			}
		}
	}

	// Default to ATX if we can't determine.
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

// stylesMatch checks if two styles are compatible.
func stylesMatch(detected, expected HeadingStyle, requireClosingATX bool) bool {
	if detected == expected {
		return true
	}

	// ATX and ATX_closed are compatible unless requireClosingATX is set.
	if !requireClosingATX {
		if (detected == StyleATX || detected == StyleATXClosed) &&
			(expected == StyleATX || expected == StyleATXClosed) {
			return true
		}
	}

	return false
}

// canAutoFix returns true if we can auto-fix between these styles.
func canAutoFix(from, to HeadingStyle) bool {
	// Only fix ATX <-> ATX_closed, not setext conversions.
	if from == StyleSetext || to == StyleSetext {
		return false
	}
	return true
}

// buildHeadingStyleFix creates an edit to fix heading style.
func buildHeadingStyleFix(
	file *mdast.FileSnapshot,
	heading *mdast.Node,
	from, to HeadingStyle,
	requireClosingATX bool,
) *fix.EditBuilder {
	if file == nil || heading == nil {
		return nil
	}

	pos := heading.SourcePosition()
	if pos.StartLine < 1 || pos.StartLine > len(file.Lines) {
		return nil
	}

	lineContent := lint.LineContent(file, pos.StartLine)
	level := lint.HeadingLevel(heading)
	if level == 0 {
		return nil
	}

	// Extract heading text (content without markers).
	headingText := extractHeadingText(lineContent, from)

	// Build new heading.
	var newHeading string
	if to == StyleATXClosed || (to == StyleATX && requireClosingATX) {
		newHeading = fmt.Sprintf("%s %s %s", strings.Repeat("#", level), headingText, strings.Repeat("#", level))
	} else {
		newHeading = fmt.Sprintf("%s %s", strings.Repeat("#", level), headingText)
	}

	line := file.Lines[pos.StartLine-1]
	builder := fix.NewEditBuilder()
	builder.ReplaceRange(line.StartOffset, line.NewlineStart, newHeading)

	return builder
}

// extractHeadingText extracts the text content from a heading line.
func extractHeadingText(lineContent []byte, style HeadingStyle) string {
	content := string(bytes.TrimSpace(lineContent))

	// Remove leading #s.
	content = strings.TrimLeft(content, "#")
	content = strings.TrimLeft(content, " \t")

	// Remove trailing #s if present.
	if style == StyleATXClosed {
		content = strings.TrimRight(content, "#")
		content = strings.TrimRight(content, " \t")
	}

	return content
}
