package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

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

// defaultEmphasisPunctuation is the default punctuation that indicates emphasis is not a heading.
const defaultEmphasisPunctuation = ".,;:!?"

// emphasisSpaceMatchGroups is the minimum submatch indices for the emphasisSpacePattern.
const emphasisSpaceMatchGroups = 8

// Apply checks for emphasis used instead of headings.
func (r *NoEmphasisAsHeadingRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	punctuation := ctx.OptionString("punctuation", defaultEmphasisPunctuation)

	paragraphs := ctx.Paragraphs()
	var diags []lint.Diagnostic

	for _, para := range paragraphs {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		// Check if this paragraph is just a single emphasized element.
		if !r.isEmphasisOnlyParagraph(para) {
			continue
		}

		// Get the text content.
		text := extractTextFromNode(para)
		if text == "" {
			continue
		}

		// Check if it ends with punctuation.
		lastRune := []rune(text)[len([]rune(text))-1]
		if strings.ContainsRune(punctuation, lastRune) {
			continue
		}

		// Check if the child is bold (NodeStrong) for autofix.
		// We only autofix bold paragraphs, not italic ones.
		child := para.FirstChild
		isBold := child != nil && child.Kind == mdast.NodeStrong

		diagBuilder := lint.NewDiagnostic(r.ID(), para,
			"Emphasis used instead of a heading").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a heading instead of emphasis for section titles")

		// Add fix only for bold paragraphs.
		if isBold {
			sourceRange := para.SourceRange()
			if sourceRange.StartOffset >= 0 && sourceRange.EndOffset > sourceRange.StartOffset {
				level := r.inferHeadingLevel(ctx, para)
				innerText := extractTextFromNode(child)
				replacement := strings.Repeat("#", level) + " " + innerText

				builder := fix.NewEditBuilder()
				builder.ReplaceRange(sourceRange.StartOffset, sourceRange.EndOffset, replacement)
				diagBuilder = diagBuilder.WithFix(builder)
			}
		}

		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
}

func (r *NoEmphasisAsHeadingRule) isEmphasisOnlyParagraph(para *mdast.Node) bool {
	if para == nil || para.Kind != mdast.NodeParagraph {
		return false
	}

	// Check if paragraph has exactly one child that is emphasis or strong.
	childCount := 0
	hasEmphasis := false

	for child := para.FirstChild; child != nil; child = child.Next {
		childCount++
		if child.Kind == mdast.NodeEmphasis || child.Kind == mdast.NodeStrong {
			hasEmphasis = true
		}
	}

	return childCount == 1 && hasEmphasis
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

// emphasisSpacePattern matches emphasis with spaces inside.
var emphasisSpacePattern = regexp.MustCompile(`(\*{1,2}|_{1,2})\s+([^*_]+)\s+(\*{1,2}|_{1,2})`)

// Apply checks for spaces inside emphasis markers.
func (r *NoSpaceInEmphasisRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		// Skip lines in code blocks.
		if lint.IsLineInCodeBlock(ctx.File, ctx.Root, lineNum) {
			continue
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		matches := emphasisSpacePattern.FindAllSubmatchIndex(lineContent, -1)

		for _, match := range matches {
			if len(match) < emphasisSpaceMatchGroups {
				continue
			}

			// Extract the matched groups.
			start, end := match[0], match[1]
			openMarker := string(lineContent[match[2]:match[3]])
			content := string(lineContent[match[4]:match[5]])
			closeMarker := string(lineContent[match[6]:match[7]])

			// Markers should match.
			if openMarker != closeMarker {
				continue
			}

			line := ctx.File.Lines[lineNum-1]

			// Build fix.
			builder := fix.NewEditBuilder()
			fixedEmphasis := openMarker + strings.TrimSpace(content) + closeMarker
			builder.ReplaceRange(line.StartOffset+start, line.StartOffset+end, fixedEmphasis)

			diagPos := mdast.SourcePosition{
				StartLine:   lineNum,
				StartColumn: start + 1,
				EndLine:     lineNum,
				EndColumn:   end + 1,
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos,
				"Spaces inside emphasis markers").
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Remove spaces from inside emphasis markers").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
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
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
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
		style := r.detectEmphasisStyle(ctx.File, pos)
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

func (r *EmphasisStyleRule) detectEmphasisStyle(file *mdast.FileSnapshot, pos mdast.SourcePosition) string {
	lineContent := lint.LineContent(file, pos.StartLine)
	if pos.StartColumn < 1 || pos.StartColumn > len(lineContent) {
		return ""
	}

	ch := lineContent[pos.StartColumn-1]
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
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		pos := st.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		// Detect style from the source.
		style := r.detectStrongStyle(ctx.File, pos)
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

func (r *StrongStyleRule) detectStrongStyle(file *mdast.FileSnapshot, pos mdast.SourcePosition) string {
	lineContent := lint.LineContent(file, pos.StartLine)
	if pos.StartColumn < 1 || pos.StartColumn+1 > len(lineContent) {
		return ""
	}

	ch := lineContent[pos.StartColumn-1]
	if ch == '*' && lineContent[pos.StartColumn] == '*' {
		return "asterisk"
	} else if ch == '_' && lineContent[pos.StartColumn] == '_' {
		return "underscore"
	}
	return ""
}

func (r *StrongStyleRule) buildStyleFix(_ *mdast.FileSnapshot, _ mdast.SourcePosition, _, _ string) *fix.EditBuilder {
	// For now, return nil - auto-fix would need more work.
	return nil
}

// extractTextFromNode extracts all text content from a node's descendants.
func extractTextFromNode(node *mdast.Node) string {
	if node == nil {
		return ""
	}
	var buf bytes.Buffer
	//nolint:errcheck // Walk visitor never returns error.
	_ = mdast.Walk(node, func(n *mdast.Node) error {
		if n.Kind == mdast.NodeText && n.Inline != nil {
			buf.Write(n.Inline.Text)
		}
		return nil
	})
	return buf.String()
}
