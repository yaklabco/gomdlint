package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
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

	// Skip front matter to find first content.
	firstContentLine := r.findFirstContentLine(ctx.File)
	if firstContentLine < 1 {
		return nil, nil
	}

	// A front matter title serves as the document title.
	if hasFrontMatterTitle(ctx) {
		return nil, nil
	}

	// Find the first block at or after the first content line.
	// This skips any front matter that goldmark may parse as blocks.
	firstBlock := r.findFirstBlockAfterLine(ctx.Root, firstContentLine)
	if ctx.OptionBool("allow_preamble", false) {
		firstBlock = lint.FirstHeading(ctx.Root)
	}
	if firstBlock == nil {
		return nil, nil
	}
	firstContentLine = firstBlock.SourcePosition().StartLine

	// Like markdownlint, an HTML heading (<h1> to <h6>) may start the document.
	if level, ok := htmlHeadingLevel(firstBlock); ok {
		if level == requiredLevel {
			return nil, nil
		}
		pos := firstBlock.SourcePosition()
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("First heading should be H%d, found H%d", requiredLevel, level)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Change to an H%d heading", requiredLevel)).
			Build()
		return []lint.Diagnostic{diag}, nil
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

// htmlHeadingLevel returns the level of an HTML block whose first tag is an
// HTML heading (<h1> to <h6>).
func htmlHeadingLevel(block *mdast.Node) (int, bool) {
	if block.Kind != mdast.NodeHTMLBlock {
		return 0, false
	}
	tag := lint.ExtractHTMLTagName(block.Text())
	if len(tag) != len("h1") || tag[0] != 'h' || tag[1] < '1' || tag[1] > '6' {
		return 0, false
	}
	return int(tag[1] - '0'), true
}

// findFirstBlockAfterLine finds the first block-level node that starts at or after the given line.
func (r *FirstLineHeadingRule) findFirstBlockAfterLine(root *mdast.Node, lineNum int) *mdast.Node {
	if root == nil {
		return nil
	}

	for child := root.FirstChild; child != nil; child = child.Next {
		if child.Kind == mdast.NodeHTMLBlock && lint.IsHTMLComment(child) {
			continue
		}
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

	for lineNum := file.FrontMatterLines + 1; lineNum <= len(file.Lines); lineNum++ {
		if !lint.IsBlankLine(file, lineNum) {
			return lineNum
		}
	}

	return 1
}

// frontMatterTitleRegexp returns the rule's front_matter_title pattern compiled
// the way markdownlint uses it: the default pattern applies when the option is
// unset, "" or false disables front matter titles, and matching ignores case.
// It returns nil when front matter titles are disabled or the pattern is invalid.
func frontMatterTitleRegexp(ctx *lint.RuleContext) *regexp.Regexp {
	pattern := config.MarkdownlintFrontMatterTitlePattern
	switch value := ctx.Option("front_matter_title", nil).(type) {
	case string:
		pattern = value
	case bool:
		if !value {
			return nil
		}
	}
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}
	return re
}

// hasFrontMatterTitle reports whether the file's front matter contains a title
// matching the rule's front_matter_title pattern.
func hasFrontMatterTitle(ctx *lint.RuleContext) bool {
	re := frontMatterTitleRegexp(ctx)
	if re == nil || ctx.File == nil {
		return false
	}
	// Front matter lines include the opening and closing delimiters.
	for lineNum := 2; lineNum < ctx.File.FrontMatterLines; lineNum++ {
		if re.Match(lint.RawLineContent(ctx.File, lineNum)) {
			return true
		}
	}
	return false
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
			[]string{"headings", "whitespace", "blank_lines"},
			true, // Auto-fixable.
		),
	}
}

// Apply checks that headings have blank lines around them.
//
// The gomdlint-only markdownlint_compatible option defaults to true, which
// matches markdownlint: adjacent headings need blank lines between them.
// Setting it to false allows headings to touch other headings.
//
// Blank lines are counted inside the heading's containers, so a line holding
// only ">" is blank within a blockquote, and the fix inserts the container
// prefix (">" in a blockquote) so it never splits the container. A heading
// followed only by blank lines up to the end of the file needs none below.
//
// Each diagnostic's fix inserts the blank lines its own heading needs. Where
// two adjacent headings share a gap, both insert at the same offset and the
// engine keeps the larger insertion after applying suppressions.
func (r *HeadingBlankLinesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	compatible := ctx.OptionBool("markdownlint_compatible", true)
	includeMetadata := ctx.OptionBool("include_front_matter", false)
	lastLine := lastSourceLine(ctx.File)
	headings := ctx.Headings()
	var diags []lint.Diagnostic

	for _, heading := range headings {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		pos := heading.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		linesAbove := headingBlankLineOption(ctx, "lines_above", lint.HeadingLevel(heading))
		linesBelow := headingBlankLineOption(ctx, "lines_below", lint.HeadingLevel(heading))

		// Check blank lines above (unless it's the first line or follows front matter).
		if pos.StartLine > 1 && linesAbove > 0 && (!headingIsFirstContent(ctx, heading) || includeMetadata) {
			blanksBefore := 0
			for line := pos.StartLine - 1; line >= 1; line-- {
				if includeMetadata && line <= ctx.File.FrontMatterLines {
					break
				}
				if !lint.IsBlankLineInContainer(ctx.File, heading, line) {
					break
				}
				blanksBefore++
			}
			if blanksBefore < linesAbove {
				// Check if previous line is also a heading (allow no blank between headings).
				if compatible || !r.isPreviousLineHeading(ctx, heading, pos.StartLine) {
					diag := r.createBlankBeforeDiagnostic(ctx, heading, pos, blanksBefore, linesAbove)
					diags = append(diags, diag)
				}
			}
		}

		// Check blank lines below (unless only blank lines follow).
		if pos.EndLine < lastLine && linesBelow > 0 {
			blanksAfter := lint.CountBlankLinesAfterInContainer(ctx.File, heading, pos.EndLine)
			if blanksAfter < linesBelow && pos.EndLine+blanksAfter < lastLine {
				// Check if next non-blank line is also a heading.
				if compatible || !r.isNextLineHeading(ctx, heading, pos.EndLine) {
					diag := r.createBlankAfterDiagnostic(ctx, heading, pos, blanksAfter, linesBelow)
					diags = append(diags, diag)
				}
			}
		}
	}

	return diags, nil
}

// lastSourceLine returns the number of the last line, not counting the empty
// line that follows a final newline.
func lastSourceLine(file *mdast.FileSnapshot) int {
	count := len(file.Lines)
	if count > 0 && file.Lines[count-1].StartOffset == len(file.Content) {
		count--
	}
	return count
}

func headingBlankLineOption(ctx *lint.RuleContext, key string, level int) int {
	values, ok := ctx.Option(key, nil).([]any)
	if !ok || level < 1 || level > len(values) {
		return ctx.OptionInt(key, 1)
	}
	switch value := values[level-1].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 1
	}
}

func headingIsFirstContent(ctx *lint.RuleContext, heading *mdast.Node) bool {
	for node := ctx.Root.FirstChild; node != nil; node = node.Next {
		if node == heading {
			return true
		}
		if node.Kind != mdast.NodeHTMLBlock || !lint.IsHTMLComment(node) {
			return false
		}
	}
	return false
}

// isPreviousLineHeading reports whether the last non-blank line (within the
// heading's containers) before lineNum ends a heading.
func (r *HeadingBlankLinesRule) isPreviousLineHeading(ctx *lint.RuleContext, heading *mdast.Node, lineNum int) bool {
	for ln := lineNum - 1; ln >= 1; ln-- {
		if lint.IsBlankLineInContainer(ctx.File, heading, ln) {
			continue
		}
		for _, h := range ctx.Headings() {
			if h.SourcePosition().EndLine == ln {
				return true
			}
		}
		return false
	}
	return false
}

// isNextLineHeading reports whether the next non-blank line (within the
// heading's containers) after lineNum starts a heading.
func (r *HeadingBlankLinesRule) isNextLineHeading(ctx *lint.RuleContext, heading *mdast.Node, lineNum int) bool {
	for ln := lineNum + 1; ln <= len(ctx.File.Lines); ln++ {
		if lint.IsBlankLineInContainer(ctx.File, heading, ln) {
			continue
		}
		for _, h := range ctx.Headings() {
			if h.SourcePosition().StartLine == ln {
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

	// Build fix: insert blank lines, with the container prefix, before the heading.
	blanksNeeded := required - current
	insertion := strings.Repeat(lint.BlankLineText(ctx.File, heading), blanksNeeded)

	line := ctx.File.Lines[pos.StartLine-1]
	builder := fix.NewEditBuilder()
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

	// Build fix: insert blank lines, with the container prefix, after the heading.
	blanksNeeded := required - current
	insertion := strings.Repeat(lint.BlankLineText(ctx.File, heading), blanksNeeded)

	line := ctx.File.Lines[pos.EndLine-1]
	builder := fix.NewEditBuilder()
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
	headings := ctx.Headings()
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
			return diags, ctx.Ctx.Err()
		}

		// Skip code blocks if configured
		if !includeCodeBlocks && ctx.IsLineInCodeBlock(lineNum) {
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
				builder := fix.NewEditBuilder()
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
