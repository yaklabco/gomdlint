package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// NoMissingSpaceATXRule checks for missing space after hash on ATX headings.
type NoMissingSpaceATXRule struct {
	lint.BaseRule
}

// NewNoMissingSpaceATXRule creates a new no-missing-space-atx rule.
func NewNoMissingSpaceATXRule() *NoMissingSpaceATXRule {
	return &NoMissingSpaceATXRule{
		BaseRule: lint.NewBaseRule(
			"MD018",
			"no-missing-space-atx",
			"No space after hash on ATX style heading",
			[]string{"atx", "headings", "spaces"},
			true,
		),
	}
}

// atxHeadingNoSpacePattern matches a line that starts with an ATX opening
// sequence followed directly by text: #Heading, ##Heading, etc.
var atxHeadingNoSpacePattern = regexp.MustCompile(`^(#{1,6})[^#\s]`)

// closingHashPattern matches a line that ends with a hash and optional
// whitespace; such lines look like closed headings, which MD020 handles.
var closingHashPattern = regexp.MustCompile(`#\s*$`)

// keycapHash is the "#️⃣" emoji, which starts with a hash but is not a heading.
const keycapHash = "#️⃣"

// Apply checks for missing space after hash on ATX headings.
//
// Like markdownlint, the check is line based: the hash must start the line,
// lines in code and HTML blocks are skipped, and lines ending in a hash are
// left to MD020 (no-missing-space-closed-atx).
func (r *NoMissingSpaceATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	htmlLines := htmlBlockLines(ctx)

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		if ctx.IsLineInCodeBlock(lineNum) || htmlLines[lineNum] {
			continue
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		match := atxHeadingNoSpacePattern.FindSubmatch(lineContent)
		if match == nil || closingHashPattern.Match(lineContent) || bytes.HasPrefix(lineContent, []byte(keycapHash)) {
			continue
		}

		hashes := match[1]
		line := ctx.File.Lines[lineNum-1]

		// Build fix: insert space after hashes.
		builder := fix.NewEditBuilder()
		builder.Insert(line.StartOffset+len(hashes), " ")

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: 1,
			EndLine:     lineNum,
			EndColumn:   len(hashes) + 2,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			"No space after hash on ATX style heading").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add a space after the hash characters").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// htmlBlockLines returns the set of lines occupied by HTML blocks.
func htmlBlockLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, block := range ctx.HTMLBlocks() {
		rng := block.SourceRange()
		if rng.StartOffset < 0 || rng.EndOffset <= rng.StartOffset {
			continue
		}
		// The block ends on the line holding its last byte, not on the line
		// its exclusive end offset may touch.
		first, _ := ctx.File.LineAt(rng.StartOffset)
		last, _ := ctx.File.LineAt(rng.EndOffset - 1)
		for line := first; first > 0 && line <= last; line++ {
			lines[line] = true
		}
	}
	return lines
}

// NoMultipleSpaceATXRule checks for multiple spaces after hash on ATX headings.
type NoMultipleSpaceATXRule struct {
	lint.BaseRule
}

// NewNoMultipleSpaceATXRule creates a new no-multiple-space-atx rule.
func NewNoMultipleSpaceATXRule() *NoMultipleSpaceATXRule {
	return &NoMultipleSpaceATXRule{
		BaseRule: lint.NewBaseRule(
			"MD019",
			"no-multiple-space-atx",
			"Multiple spaces after hash on ATX style heading",
			[]string{"atx", "headings", "spaces"},
			true,
		),
	}
}

// Apply checks for multiple spaces after hash on ATX headings.
//
// Like markdownlint, it checks parsed open ATX headings, including headings
// in blockquotes and lists; closed headings are left to MD021.
func (r *NoMultipleSpaceATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || ctx.Root == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for _, heading := range ctx.Headings() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if detectHeadingStyle(ctx.File, heading) != StyleATX {
			continue
		}
		content, ok := headingContentRange(ctx.File, heading)
		if !ok {
			continue
		}
		spaces, _ := atxOpeningWhitespace(ctx.File, content)
		if spaces.Len() < 2 {
			continue
		}

		// Build fix: replace multiple spaces with single space.
		builder := fix.NewEditBuilder()
		builder.ReplaceRange(spaces.StartOffset, spaces.EndOffset, " ")

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, rangePosition(ctx.File, spaces),
			fmt.Sprintf("Multiple spaces (%d) after hash on ATX style heading", spaces.Len())).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a single space after the hash characters").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// rangePosition converts a byte range to a line and column position.
func rangePosition(file *mdast.FileSnapshot, rng mdast.SourceRange) mdast.SourcePosition {
	startLine, startColumn := file.LineAt(rng.StartOffset)
	endLine, endColumn := file.LineAt(rng.EndOffset)
	return mdast.SourcePosition{
		StartLine:   startLine,
		StartColumn: startColumn,
		EndLine:     endLine,
		EndColumn:   endColumn,
	}
}

// NoMissingSpaceClosedATXRule checks for missing space inside hashes on closed ATX headings.
type NoMissingSpaceClosedATXRule struct {
	lint.BaseRule
}

// NewNoMissingSpaceClosedATXRule creates a new no-missing-space-closed-atx rule.
func NewNoMissingSpaceClosedATXRule() *NoMissingSpaceClosedATXRule {
	return &NoMissingSpaceClosedATXRule{
		BaseRule: lint.NewBaseRule(
			"MD020",
			"no-missing-space-closed-atx",
			"No space inside hashes on closed ATX style heading",
			[]string{"atx_closed", "headings", "spaces"},
			true,
		),
	}
}

// closedATXPattern is markdownlint's MD020 grammar for a closed ATX heading
// line: opening hashes, optional space, content that neither starts nor ends
// with a hash, space or backslash, optional space, an optional escaped hash
// that belongs to the content, closing hashes and trailing whitespace.
var closedATXPattern = regexp.MustCompile(
	`^(#+)([ \t]*)([^# \t\\]|[^# \t][^#]*?[^# \t\\])([ \t]*)((?:\\#)?)(#+)(\s*)$`)

// Submatch indexes for closedATXPattern.
const (
	closedATXOpen = iota + 1
	closedATXOpenSpace
	closedATXContent
	closedATXCloseSpace
	closedATXEscape
	closedATXClose
	closedATXTrail
)

// Apply checks for missing space inside hashes on closed ATX headings.
//
// Like markdownlint, the check is line based: the hash must start the line
// and lines in code and HTML blocks are skipped. An escaped hash (\#) is
// content, so "## Learn C\#" is not a closed heading.
func (r *NoMissingSpaceClosedATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	htmlLines := htmlBlockLines(ctx)

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		if ctx.IsLineInCodeBlock(lineNum) || htmlLines[lineNum] {
			continue
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		match := closedATXPattern.FindSubmatch(lineContent)
		if match == nil {
			continue
		}

		openHashes := match[closedATXOpen]
		content := match[closedATXContent]
		escape := match[closedATXEscape]
		closeHashes := match[closedATXClose]

		missingOpenSpace := len(match[closedATXOpenSpace]) == 0
		missingCloseSpace := len(match[closedATXCloseSpace]) == 0 || len(escape) > 0
		if !missingOpenSpace && !missingCloseSpace {
			continue
		}

		// Build fix. An escaped hash stays attached to the content (markdownlint
		// moves it after the inserted space, which changes the heading text).
		line := ctx.File.Lines[lineNum-1]
		replacement := string(openHashes) + " " + string(content)
		if len(escape) > 0 {
			replacement += string(match[closedATXCloseSpace]) + string(escape)
		}
		replacement += " " + string(closeHashes)
		builder := fix.NewEditBuilder()
		builder.ReplaceRange(line.StartOffset, line.NewlineStart, replacement)

		closeEnd := len(lineContent) - len(match[closedATXTrail])
		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: 1,
			EndLine:     lineNum,
			EndColumn:   closeEnd + 1,
		}

		var msg string
		switch {
		case missingOpenSpace && missingCloseSpace:
			msg = "No space inside hashes on closed ATX style heading (both sides)"
		case missingOpenSpace:
			msg = "No space after opening hashes on closed ATX style heading"
		default:
			msg = "No space before closing hashes on closed ATX style heading"
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add spaces inside the hash characters").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// NoMultipleSpaceClosedATXRule checks for multiple spaces inside hashes on closed ATX headings.
type NoMultipleSpaceClosedATXRule struct {
	lint.BaseRule
}

// NewNoMultipleSpaceClosedATXRule creates a new no-multiple-space-closed-atx rule.
func NewNoMultipleSpaceClosedATXRule() *NoMultipleSpaceClosedATXRule {
	return &NoMultipleSpaceClosedATXRule{
		BaseRule: lint.NewBaseRule(
			"MD021",
			"no-multiple-space-closed-atx",
			"Multiple spaces inside hashes on closed ATX style heading",
			[]string{"atx_closed", "headings", "spaces"},
			true,
		),
	}
}

// Apply checks for multiple spaces inside hashes on closed ATX headings.
//
// Like markdownlint, it checks parsed closed ATX headings, including headings
// in blockquotes and lists.
func (r *NoMultipleSpaceClosedATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || ctx.Root == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for _, heading := range ctx.Headings() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		content, ok := headingContentRange(ctx.File, heading)
		if !ok || detectHeadingStyle(ctx.File, heading) != StyleATXClosed {
			continue
		}
		closing, closed := atxClosingSequence(ctx.File, content)
		if !closed {
			continue
		}
		openSpaces, openStart := atxOpeningWhitespace(ctx.File, content)
		closeSpaces := mdast.SourceRange{StartOffset: content.EndOffset, EndOffset: closing.StartOffset}

		multipleOpenSpaces := openSpaces.Len() > 1
		multipleCloseSpaces := closeSpaces.Len() > 1
		if !multipleOpenSpaces && !multipleCloseSpaces {
			continue
		}

		// Build fix: normalize to single space on each side.
		builder := fix.NewEditBuilder()
		if multipleOpenSpaces {
			builder.ReplaceRange(openSpaces.StartOffset, openSpaces.EndOffset, " ")
		}
		if multipleCloseSpaces {
			builder.ReplaceRange(closeSpaces.StartOffset, closeSpaces.EndOffset, " ")
		}

		pos := rangePosition(ctx.File, mdast.SourceRange{StartOffset: openStart, EndOffset: closing.EndOffset})

		var msg string
		switch {
		case multipleOpenSpaces && multipleCloseSpaces:
			msg = "Multiple spaces inside hashes on closed ATX style heading (both sides)"
		case multipleOpenSpaces:
			msg = "Multiple spaces after opening hashes on closed ATX style heading"
		default:
			msg = "Multiple spaces before closing hashes on closed ATX style heading"
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a single space inside the hash characters").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// HeadingStartLeftRule checks that headings start at the beginning of the line.
type HeadingStartLeftRule struct {
	lint.BaseRule
}

// NewHeadingStartLeftRule creates a new heading-start-left rule.
func NewHeadingStartLeftRule() *HeadingStartLeftRule {
	return &HeadingStartLeftRule{
		BaseRule: lint.NewBaseRule(
			"MD023",
			"heading-start-left",
			"Headings must start at the beginning of the line",
			[]string{"headings", "spaces"},
			true,
		),
	}
}

// indentedHeadingPattern matches headings that have any leading whitespace.
var indentedHeadingPattern = regexp.MustCompile(`^([ \t]+)(#{1,6})(\s|$)`)

// codeBlockIndent is the minimum spaces that indicate an indented code block.
const codeBlockIndent = 4

// Apply checks that headings start at the beginning of the line.
func (r *HeadingStartLeftRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		match := indentedHeadingPattern.FindSubmatch(lineContent)
		if match == nil {
			continue
		}

		indent := match[1]

		// Skip if 4+ spaces (would be an indented code block, not a heading).
		spaceCount := 0
		for _, ch := range indent {
			if ch == ' ' {
				spaceCount++
			} else if ch == '\t' {
				// Tab is typically rendered as 4 spaces, so treat it as a potential code block.
				// However, a tab followed by # is usually an indented heading, not code.
				// We'll allow tabs to be flagged since they're usually a mistake.
				break
			}
		}
		if spaceCount >= codeBlockIndent {
			continue
		}

		// Also skip lines inside actual code blocks.
		if ctx.IsLineInCodeBlock(lineNum) {
			continue
		}

		line := ctx.File.Lines[lineNum-1]

		// Build fix: remove leading whitespace.
		builder := fix.NewEditBuilder()
		builder.Delete(line.StartOffset, line.StartOffset+len(indent))

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: 1,
			EndLine:     lineNum,
			EndColumn:   len(indent) + 1,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Heading is indented by %d character(s)", len(indent))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove leading whitespace from the heading").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// NoDuplicateHeadingRule checks for multiple headings with the same content.
type NoDuplicateHeadingRule struct {
	lint.BaseRule
}

// NewNoDuplicateHeadingRule creates a new no-duplicate-heading rule.
func NewNoDuplicateHeadingRule() *NoDuplicateHeadingRule {
	return &NoDuplicateHeadingRule{
		BaseRule: lint.NewBaseRule(
			"MD024",
			"no-duplicate-heading",
			"Multiple headings with the same content",
			[]string{"headings"},
			false, // Not auto-fixable.
		),
	}
}

// Apply checks for duplicate heading content.
func (r *NoDuplicateHeadingRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	siblingsOnly := ctx.OptionBool("siblings_only", false)

	headings := ctx.Headings()
	var diags []lint.Diagnostic

	if siblingsOnly {
		diags = r.checkSiblings(ctx, headings)
	} else {
		diags = r.checkAll(ctx, headings)
	}

	return diags, nil
}

func (r *NoDuplicateHeadingRule) checkAll(ctx *lint.RuleContext, headings []*mdast.Node) []lint.Diagnostic {
	seen := make(map[string]*mdast.Node)
	var diags []lint.Diagnostic

	for _, heading := range headings {
		if ctx.Cancelled() {
			break
		}

		text := lint.HeadingText(heading)
		if text == "" {
			continue
		}

		if first, ok := seen[text]; ok {
			firstPos := first.SourcePosition()
			diag := lint.NewDiagnostic(r.ID(), heading,
				fmt.Sprintf("Duplicate heading text %q (first occurrence on line %d)", text, firstPos.StartLine)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use unique heading text").
				Build()
			diags = append(diags, diag)
		} else {
			seen[text] = heading
		}
	}

	return diags
}

func (r *NoDuplicateHeadingRule) checkSiblings(ctx *lint.RuleContext, headings []*mdast.Node) []lint.Diagnostic {
	// For siblings_only mode, track headings by their parent context.
	// Headings at the same level under the same parent are considered siblings.
	// A parent is any heading with a lower level number.

	type parentInfo struct {
		level int
		text  string
	}

	diags := make([]lint.Diagnostic, 0, len(headings))
	parentStack := make([]parentInfo, 0, len(headings))

	// Map from (level, parent_path) -> (text -> first_heading)
	seen := make(map[string]map[string]*mdast.Node)

	for _, heading := range headings {
		if ctx.Cancelled() {
			break
		}

		level := lint.HeadingLevel(heading)
		text := lint.HeadingText(heading)
		if text == "" {
			continue
		}

		// Pop parent stack until we find a parent with lower level.
		for len(parentStack) > 0 && parentStack[len(parentStack)-1].level >= level {
			parentStack = parentStack[:len(parentStack)-1]
		}

		// Build a unique key for the parent context.
		// Include both level and text of each parent to distinguish different sections.
		var parentKeyBuilder strings.Builder
		for _, p := range parentStack {
			fmt.Fprintf(&parentKeyBuilder, "%d:%s/", p.level, p.text)
		}
		parentKey := parentKeyBuilder.String()

		// Create a key combining level and parent path.
		contextKey := fmt.Sprintf("%d@%s", level, parentKey)

		if seen[contextKey] == nil {
			seen[contextKey] = make(map[string]*mdast.Node)
		}

		if first, ok := seen[contextKey][text]; ok {
			firstPos := first.SourcePosition()
			diag := lint.NewDiagnostic(r.ID(), heading,
				fmt.Sprintf("Duplicate sibling heading text %q (first occurrence on line %d)", text, firstPos.StartLine)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use unique heading text among siblings").
				Build()
			diags = append(diags, diag)
		} else {
			seen[contextKey][text] = heading
		}

		// Push this heading onto the parent stack for potential children.
		parentStack = append(parentStack, parentInfo{level: level, text: text})
	}

	return diags
}

// NoTrailingPunctuationRule checks for trailing punctuation in headings.
type NoTrailingPunctuationRule struct {
	lint.BaseRule
}

// NewNoTrailingPunctuationRule creates a new no-trailing-punctuation rule.
func NewNoTrailingPunctuationRule() *NoTrailingPunctuationRule {
	return &NoTrailingPunctuationRule{
		BaseRule: lint.NewBaseRule(
			"MD026",
			"no-trailing-punctuation",
			"Trailing punctuation in heading",
			[]string{"headings"},
			true,
		),
	}
}

// defaultPunctuation is markdownlint's default set of trailing punctuation
// characters: normal and full-width, without the question mark.
const defaultPunctuation = ".,;:!。，；：！"

// htmlEntityPattern matches an HTML entity reference at the end of text
// (markdownlint's endOfLineHtmlEntityRe).
var htmlEntityPattern = regexp.MustCompile(
	`&(?:#\d+|#[xX][\da-fA-F]+|[a-zA-Z]{2,31}|blk\d{2}|emsp1[34]|frac\d{2}|sup\d|there4);$`)

// gemojiCodePattern matches a GitHub emoji code such as :smile: at the end of
// text (markdownlint's endOfLineGemojiCodeRe).
var gemojiCodePattern = regexp.MustCompile(
	`:(?:[abmovx]|[-+]1|100|1234|(?:1st|2nd|3rd)_place_medal|8ball|clock\d{1,4}|e-mail|non-potable_water|o2|` +
		`t-rex|u5272|u5408|u55b6|u6307|u6708|u6709|u6e80|u7121|u7533|u7981|u7a7a|[a-z]{2,15}2?|` +
		`[a-z]{1,14}(?:_[a-z\d]{1,16})+):$`)

// Apply checks for trailing punctuation in headings.
//
// Like markdownlint, it checks the raw source of the heading content, so a
// heading that ends with a code span, link or emphasis is not reported. The
// fix deletes the trailing punctuation run and any whitespace before it.
func (r *NoTrailingPunctuationRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	punctuation := ctx.OptionString("punctuation", defaultPunctuation)
	if punctuation == "" {
		return nil, nil // Empty string disables the rule.
	}

	headings := ctx.Headings()
	diags := make([]lint.Diagnostic, 0, len(headings))

	for _, heading := range headings {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		content, ok := headingContentRange(ctx.File, heading)
		if !ok {
			continue
		}
		text := ctx.File.Content[content.StartOffset:content.EndOffset]
		if htmlEntityPattern.Match(text) || gemojiCodePattern.Match(text) {
			continue
		}

		punctStart := len(text)
		for punctStart > 0 {
			lastRune, size := utf8.DecodeLastRune(text[:punctStart])
			if lastRune == utf8.RuneError || !strings.ContainsRune(punctuation, lastRune) {
				break
			}
			punctStart -= size
		}
		if punctStart == len(text) {
			continue
		}
		run := text[punctStart:]
		start := content.StartOffset + punctStart

		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, rangePosition(ctx.File,
			mdast.SourceRange{StartOffset: start, EndOffset: content.EndOffset}),
			fmt.Sprintf("Heading ends with trailing punctuation %q", run)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove trailing punctuation from the heading")
		if builder := trailingPunctuationFix(ctx.File, heading, content, start); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
}

// trailingPunctuationFix deletes the punctuation starting at start, together
// with the whitespace before it on the same line. It returns nil when the
// deletion would empty a line of a setext heading, which would turn the
// heading into a paragraph or split it.
func trailingPunctuationFix(
	file *mdast.FileSnapshot,
	heading *mdast.Node,
	content mdast.SourceRange,
	start int,
) *fix.EditBuilder {
	line := lineOf(file, start)
	if line == nil {
		return nil
	}
	lineStart := max(line.StartOffset, content.StartOffset)
	deleteStart := start
	for deleteStart > lineStart && (file.Content[deleteStart-1] == ' ' || file.Content[deleteStart-1] == '\t') {
		deleteStart--
	}
	if detectHeadingStyle(file, heading) == StyleSetext {
		// Only indentation and blockquote markers may precede the content
		// of a setext continuation line.
		if len(bytes.TrimLeft(file.Content[line.StartOffset:deleteStart], " \t>")) == 0 {
			return nil
		}
	}
	builder := fix.NewEditBuilder()
	builder.Delete(deleteStart, content.EndOffset)
	return builder
}
