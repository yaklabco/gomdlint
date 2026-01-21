package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// FirstLineHeadingRule checks that files begin with a top-level heading.
type FirstLineHeadingRule struct {
	lint.BaseRule
}

// NewFirstLineHeadingRule creates a new first line heading rule.
func NewFirstLineHeadingRule() *FirstLineHeadingRule {
	return &FirstLineHeadingRule{
		BaseRule: lint.NewBaseRule(
			"MD041",
			"first-line-heading",
			"First line in a file should be a top-level heading",
			[]string{"headings", "metadata"},
			false, // Not auto-fixable.
		),
	}
}

// DefaultEnabled returns false - this rule is opt-in.
func (r *FirstLineHeadingRule) DefaultEnabled() bool {
	return false
}

// Apply checks that the first content in the file is a top-level heading.
func (r *FirstLineHeadingRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil || len(ctx.File.Content) == 0 {
		return nil, nil
	}

	requiredLevel := ctx.OptionInt("level", 1)
	frontMatterTitlePattern := ctx.OptionString("front_matter_title", "")

	// Skip front matter to find first content.
	firstContentLine := r.findFirstContentLine(ctx.File)
	if firstContentLine < 1 {
		return nil, nil
	}

	// Check for front matter title if configured.
	if frontMatterTitlePattern != "" {
		hasFrontMatterTitle, err := r.checkFrontMatterTitle(ctx.File, frontMatterTitlePattern)
		// If error or front matter has title, skip first heading check.
		if err == nil && hasFrontMatterTitle {
			return nil, nil
		}
		// Invalid regex is ignored - continue with default heading check behavior.
	}

	// Find the first block at or after the first content line.
	// This skips any front matter that goldmark may parse as blocks.
	firstBlock := r.findFirstBlockAfterLine(ctx.Root, firstContentLine)
	if firstBlock == nil {
		return nil, nil
	}

	// If first block is not a heading.
	if firstBlock.Kind != mdast.NodeHeading {
		pos := mdast.SourcePosition{
			StartLine:   firstContentLine,
			StartColumn: 1,
			EndLine:     firstContentLine,
			EndColumn:   1,
		}

		var msg string
		if requiredLevel == 1 {
			msg = "First line should be a top-level heading"
		} else {
			msg = fmt.Sprintf("First line should be an H%d heading", requiredLevel)
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Add an H%d heading at the beginning", requiredLevel)).
			Build()
		return []lint.Diagnostic{diag}, nil
	}

	// Check heading level.
	level := lint.HeadingLevel(firstBlock)
	if level != requiredLevel {
		pos := firstBlock.SourcePosition()
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("First heading should be H%d, found H%d", requiredLevel, level)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Change to an H%d heading", requiredLevel)).
			Build()
		return []lint.Diagnostic{diag}, nil
	}

	return nil, nil
}

// findFirstBlockAfterLine finds the first block-level node that starts at or after the given line.
func (r *FirstLineHeadingRule) findFirstBlockAfterLine(root *mdast.Node, lineNum int) *mdast.Node {
	if root == nil {
		return nil
	}

	for child := root.FirstChild; child != nil; child = child.Next {
		pos := child.SourcePosition()
		if pos.IsValid() && pos.StartLine >= lineNum {
			return child
		}
	}

	return nil
}

func (r *FirstLineHeadingRule) findFirstContentLine(file *mdast.FileSnapshot) int {
	if file == nil || len(file.Lines) == 0 {
		return 0
	}

	// Check for YAML front matter (---).
	firstLine := lint.LineContent(file, 1)
	if bytes.Equal(bytes.TrimSpace(firstLine), []byte("---")) {
		// Find closing ---.
		for lineNum := 2; lineNum <= len(file.Lines); lineNum++ {
			content := lint.LineContent(file, lineNum)
			if bytes.Equal(bytes.TrimSpace(content), []byte("---")) {
				// Return line after front matter.
				return lineNum + 1
			}
		}
	}

	// No front matter, first line is first content.
	// Skip leading blank lines.
	for lineNum := 1; lineNum <= len(file.Lines); lineNum++ {
		if !lint.IsBlankLine(file, lineNum) {
			return lineNum
		}
	}

	return 1
}

func (r *FirstLineHeadingRule) checkFrontMatterTitle(
	file *mdast.FileSnapshot,
	pattern string,
) (bool, error) {
	if file == nil || len(file.Lines) == 0 {
		return false, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid front matter title pattern: %w", err)
	}

	// Check for YAML front matter.
	firstLine := lint.LineContent(file, 1)
	if !bytes.Equal(bytes.TrimSpace(firstLine), []byte("---")) {
		return false, nil
	}

	// Search within front matter.
	for lineNum := 2; lineNum <= len(file.Lines); lineNum++ {
		content := lint.LineContent(file, lineNum)
		trimmed := bytes.TrimSpace(content)

		// End of front matter.
		if bytes.Equal(trimmed, []byte("---")) {
			break
		}

		// Check if line matches title pattern.
		if re.Match(content) {
			return true, nil
		}
	}

	return false, nil
}

// HeadingBlankLinesRule ensures headings are surrounded by blank lines.
type HeadingBlankLinesRule struct {
	lint.BaseRule
}

// NewHeadingBlankLinesRule creates a new heading blank lines rule.
func NewHeadingBlankLinesRule() *HeadingBlankLinesRule {
	return &HeadingBlankLinesRule{
		BaseRule: lint.NewBaseRule(
			"MD022",
			"heading-blank-lines",
			"Headings should be surrounded by blank lines",
			[]string{"headings", "whitespace"},
			true, // Auto-fixable.
		),
	}
}

// Apply checks that headings have blank lines around them.
func (r *HeadingBlankLinesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	linesAbove := ctx.OptionInt("lines_above", 1)
	linesBelow := ctx.OptionInt("lines_below", 1)

	headings := lint.Headings(ctx.Root)
	var diags []lint.Diagnostic

	for _, heading := range headings {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		pos := heading.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		// Check blank lines above (unless it's the first line or follows front matter).
		if pos.StartLine > 1 && linesAbove > 0 {
			blanksBefore := lint.CountBlankLinesBefore(ctx.File, pos.StartLine)
			if blanksBefore < linesAbove {
				// Check if previous line is also a heading (allow no blank between headings).
				prevIsHeading := r.isPreviousLineHeading(ctx.File, ctx.Root, pos.StartLine)
				if !prevIsHeading {
					diag := r.createBlankBeforeDiagnostic(ctx, heading, pos, blanksBefore, linesAbove)
					diags = append(diags, diag)
				}
			}
		}

		// Check blank lines below (unless it's the last line).
		if pos.EndLine < len(ctx.File.Lines) && linesBelow > 0 {
			blanksAfter := lint.CountBlankLinesAfter(ctx.File, pos.EndLine)
			if blanksAfter < linesBelow {
				// Check if next non-blank line is also a heading.
				nextIsHeading := r.isNextLineHeading(ctx.File, ctx.Root, pos.EndLine)
				if !nextIsHeading {
					diag := r.createBlankAfterDiagnostic(ctx, heading, pos, blanksAfter, linesBelow)
					diags = append(diags, diag)
				}
			}
		}
	}

	return diags, nil
}

func (r *HeadingBlankLinesRule) isPreviousLineHeading(
	file *mdast.FileSnapshot,
	root *mdast.Node,
	lineNum int,
) bool {
	if lineNum <= 1 {
		return false
	}

	// Find the previous non-blank line.
	for ln := lineNum - 1; ln >= 1; ln-- {
		if lint.IsBlankLine(file, ln) {
			continue
		}

		// Check if any heading ends on this line.
		headings := lint.Headings(root)
		for _, h := range headings {
			pos := h.SourcePosition()
			if pos.EndLine == ln {
				return true
			}
		}
		return false
	}

	return false
}

func (r *HeadingBlankLinesRule) isNextLineHeading(
	file *mdast.FileSnapshot,
	root *mdast.Node,
	lineNum int,
) bool {
	if lineNum >= len(file.Lines) {
		return false
	}

	// Find the next non-blank line.
	for ln := lineNum + 1; ln <= len(file.Lines); ln++ {
		if lint.IsBlankLine(file, ln) {
			continue
		}

		// Check if any heading starts on this line.
		headings := lint.Headings(root)
		for _, h := range headings {
			pos := h.SourcePosition()
			if pos.StartLine == ln {
				return true
			}
		}
		return false
	}

	return false
}

func (r *HeadingBlankLinesRule) createBlankBeforeDiagnostic(
	ctx *lint.RuleContext,
	heading *mdast.Node,
	pos mdast.SourcePosition,
	current, required int,
) lint.Diagnostic {
	msg := fmt.Sprintf("Heading needs %d blank line(s) above, found %d", required, current)

	// Build fix: insert blank lines before the heading.
	blanksNeeded := required - current
	insertion := strings.Repeat("\n", blanksNeeded)

	line := ctx.File.Lines[pos.StartLine-1]
	builder := ctx.Builder
	builder.Insert(line.StartOffset, insertion)

	return lint.NewDiagnostic(r.ID(), heading, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Add %d blank line(s) before the heading", blanksNeeded)).
		WithFix(builder).
		Build()
}

func (r *HeadingBlankLinesRule) createBlankAfterDiagnostic(
	ctx *lint.RuleContext,
	heading *mdast.Node,
	pos mdast.SourcePosition,
	current, required int,
) lint.Diagnostic {
	msg := fmt.Sprintf("Heading needs %d blank line(s) below, found %d", required, current)

	// Build fix: insert blank lines after the heading.
	blanksNeeded := required - current
	insertion := strings.Repeat("\n", blanksNeeded)

	line := ctx.File.Lines[pos.EndLine-1]
	builder := ctx.Builder
	builder.Insert(line.EndOffset, insertion)

	return lint.NewDiagnostic(r.ID(), heading, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Add %d blank line(s) after the heading", blanksNeeded)).
		WithFix(builder).
		Build()
}

// RequiredHeadingsRule checks that document follows required heading structure.
type RequiredHeadingsRule struct {
	lint.BaseRule
}

// NewRequiredHeadingsRule creates a new required headings rule.
func NewRequiredHeadingsRule() *RequiredHeadingsRule {
	return &RequiredHeadingsRule{
		BaseRule: lint.NewBaseRule(
			"MD043",
			"required-headings",
			"Required heading structure",
			[]string{"headings"},
			false, // Not auto-fixable.
		),
	}
}

// DefaultEnabled returns false - this rule requires configuration.
func (r *RequiredHeadingsRule) DefaultEnabled() bool {
	return false
}

// Apply checks document heading structure against required pattern.
func (r *RequiredHeadingsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	requiredHeadings := r.getRequiredHeadings(ctx)
	if len(requiredHeadings) == 0 {
		return nil, nil
	}

	matchCase := ctx.OptionBool("match_case", false)
	headings := lint.Headings(ctx.Root)
	actualHeadings := r.buildActualHeadings(headings)

	return r.matchHeadings(ctx, headings, actualHeadings, requiredHeadings, matchCase)
}

func (r *RequiredHeadingsRule) getRequiredHeadings(ctx *lint.RuleContext) []string {
	headingsOption := ctx.Option("headings", nil)
	if headingsOption == nil {
		return nil
	}

	switch h := headingsOption.(type) {
	case []string:
		return h
	case []interface{}:
		var result []string
		for _, item := range h {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func (r *RequiredHeadingsRule) buildActualHeadings(headings []*mdast.Node) []string {
	result := make([]string, 0, len(headings))
	for _, h := range headings {
		level := lint.HeadingLevel(h)
		text := lint.HeadingText(h)
		result = append(result, fmt.Sprintf("%s %s", strings.Repeat("#", level), text))
	}
	return result
}

func (r *RequiredHeadingsRule) matchHeadings(
	ctx *lint.RuleContext,
	headings []*mdast.Node,
	actualHeadings, requiredHeadings []string,
	matchCase bool,
) ([]lint.Diagnostic, error) {
	reqIdx, actIdx := 0, 0

	for reqIdx < len(requiredHeadings) && actIdx < len(actualHeadings) {
		required := requiredHeadings[reqIdx]

		switch required {
		case "*", "+":
			reqIdx, actIdx = r.handleWildcard(required, reqIdx, actIdx, actualHeadings, requiredHeadings, matchCase)
		case "?":
			actIdx++
			reqIdx++
		default:
			if r.headingMatches(actualHeadings[actIdx], required, matchCase) {
				actIdx++
				reqIdx++
				continue
			}
			return r.createMismatchDiagnostic(ctx, headings, actualHeadings, required, actIdx), nil
		}
	}

	return r.checkRemainingRequired(ctx, requiredHeadings, reqIdx)
}

func (r *RequiredHeadingsRule) handleWildcard(
	pattern string,
	reqIdx, actIdx int,
	actualHeadings, requiredHeadings []string,
	matchCase bool,
) (int, int) {
	if pattern == "+" {
		actIdx++ // Must match at least one
	}
	reqIdx++

	if reqIdx >= len(requiredHeadings) {
		return reqIdx, len(actualHeadings)
	}

	nextRequired := requiredHeadings[reqIdx]
	for actIdx < len(actualHeadings) {
		if r.headingMatches(actualHeadings[actIdx], nextRequired, matchCase) {
			break
		}
		actIdx++
	}
	return reqIdx, actIdx
}

func (r *RequiredHeadingsRule) createMismatchDiagnostic(
	ctx *lint.RuleContext,
	headings []*mdast.Node,
	actualHeadings []string,
	required string,
	actIdx int,
) []lint.Diagnostic {
	pos := r.getPositionForIndex(ctx, headings, actIdx)
	msg := r.getMismatchMessage(actualHeadings, required, actIdx)

	diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Update heading to match required structure").
		Build()
	return []lint.Diagnostic{diag}
}

func (r *RequiredHeadingsRule) getPositionForIndex(
	ctx *lint.RuleContext,
	headings []*mdast.Node,
	actIdx int,
) mdast.SourcePosition {
	if actIdx < len(headings) {
		return headings[actIdx].SourcePosition()
	}
	return mdast.SourcePosition{
		StartLine:   len(ctx.File.Lines),
		StartColumn: 1,
		EndLine:     len(ctx.File.Lines),
		EndColumn:   1,
	}
}

func (r *RequiredHeadingsRule) getMismatchMessage(actualHeadings []string, required string, actIdx int) string {
	if actIdx < len(actualHeadings) {
		return fmt.Sprintf("Expected heading %q, found %q", required, actualHeadings[actIdx])
	}
	return fmt.Sprintf("Missing required heading %q", required)
}

func (r *RequiredHeadingsRule) checkRemainingRequired(
	ctx *lint.RuleContext,
	requiredHeadings []string,
	reqIdx int,
) ([]lint.Diagnostic, error) {
	for reqIdx < len(requiredHeadings) {
		required := requiredHeadings[reqIdx]
		if required != "*" && required != "+" && required != "?" {
			pos := mdast.SourcePosition{
				StartLine:   len(ctx.File.Lines),
				StartColumn: 1,
				EndLine:     len(ctx.File.Lines),
				EndColumn:   1,
			}
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				fmt.Sprintf("Missing required heading %q", required)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Add required heading").
				Build()
			return []lint.Diagnostic{diag}, nil
		}
		reqIdx++
	}
	return nil, nil
}

func (r *RequiredHeadingsRule) headingMatches(actual, required string, matchCase bool) bool {
	if matchCase {
		return actual == required
	}
	return strings.EqualFold(actual, required)
}

// ProperNamesRule checks for correct capitalization of proper names.
type ProperNamesRule struct {
	lint.BaseRule
}

// NewProperNamesRule creates a new proper names rule.
func NewProperNamesRule() *ProperNamesRule {
	return &ProperNamesRule{
		BaseRule: lint.NewBaseRule(
			"MD044",
			"proper-names",
			"Proper names should have the correct capitalization",
			[]string{"spelling"},
			true, // Auto-fixable.
		),
	}
}

// DefaultEnabled returns false - this rule requires configuration.
func (r *ProperNamesRule) DefaultEnabled() bool {
	return false
}

// Apply checks for incorrect capitalization of proper names.
func (r *ProperNamesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Get proper names configuration
	namesOption := ctx.Option("names", nil)
	if namesOption == nil {
		return nil, nil // No names configured
	}

	var properNames []string
	switch n := namesOption.(type) {
	case []string:
		properNames = n
	case []interface{}:
		for _, item := range n {
			if s, ok := item.(string); ok {
				properNames = append(properNames, s)
			}
		}
	}

	if len(properNames) == 0 {
		return nil, nil
	}

	includeCodeBlocks := ctx.OptionBool("code_blocks", true)
	includeHTMLElements := ctx.OptionBool("html_elements", true)

	var diags []lint.Diagnostic

	// Build patterns for each proper name
	type namePattern struct {
		correct string
		pattern *regexp.Regexp
	}
	patterns := make([]namePattern, 0, len(properNames))

	for _, name := range properNames {
		// Create case-insensitive pattern that matches whole words
		escaped := regexp.QuoteMeta(name)
		pattern, err := regexp.Compile(`(?i)\b` + escaped + `\b`)
		if err != nil {
			continue
		}
		patterns = append(patterns, namePattern{correct: name, pattern: pattern})
	}

	// Check each line
	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		// Skip code blocks if configured
		if !includeCodeBlocks && lint.IsLineInCodeBlock(ctx.File, ctx.Root, lineNum) {
			continue
		}

		// Skip HTML if configured
		if !includeHTMLElements && r.isLineInHTML(ctx.File, ctx.Root, lineNum) {
			continue
		}

		lineContent := lint.LineContent(ctx.File, lineNum)

		for _, np := range patterns {
			matches := np.pattern.FindAllIndex(lineContent, -1)
			for _, match := range matches {
				found := string(lineContent[match[0]:match[1]])

				// Skip if already correct
				if found == np.correct {
					continue
				}

				pos := mdast.SourcePosition{
					StartLine:   lineNum,
					StartColumn: match[0] + 1,
					EndLine:     lineNum,
					EndColumn:   match[1] + 1,
				}

				line := ctx.File.Lines[lineNum-1]

				// Build fix
				builder := ctx.Builder
				builder.ReplaceRange(
					line.StartOffset+match[0],
					line.StartOffset+match[1],
					np.correct,
				)

				diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
					fmt.Sprintf("Proper name %q should be %q", found, np.correct)).
					WithSeverity(config.SeverityWarning).
					WithSuggestion(fmt.Sprintf("Use %q", np.correct)).
					WithFix(builder).
					Build()
				diags = append(diags, diag)
			}
		}
	}

	return diags, nil
}

func (r *ProperNamesRule) isLineInHTML(file *mdast.FileSnapshot, root *mdast.Node, lineNum int) bool {
	if file == nil || root == nil || lineNum < 1 {
		return false
	}

	htmlBlocks := lint.HTMLBlocks(root)
	for _, block := range htmlBlocks {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		if lineNum >= pos.StartLine && lineNum <= pos.EndLine {
			return true
		}
	}

	return false
}
