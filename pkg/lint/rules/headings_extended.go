package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
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

// atxHeadingNoSpacePattern matches ATX headings without space after hashes.
// Matches: #Heading, ##Heading, etc. (no space after #).
var atxHeadingNoSpacePattern = regexp.MustCompile(`^(#{1,6})([^#\s])`)

// Apply checks for missing space after hash on ATX headings.
func (r *NoMissingSpaceATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		trimmed := bytes.TrimLeft(lineContent, " \t")

		match := atxHeadingNoSpacePattern.FindSubmatch(trimmed)
		if match == nil {
			continue
		}

		hashes := string(match[1])
		line := ctx.File.Lines[lineNum-1]

		// Find the position of the hashes in the original line.
		hashStart := bytes.Index(lineContent, match[1])
		if hashStart < 0 {
			continue
		}

		// Build fix: insert space after hashes.
		builder := fix.NewEditBuilder()
		insertPos := line.StartOffset + hashStart + len(hashes)
		builder.Insert(insertPos, " ")

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: hashStart + 1,
			EndLine:     lineNum,
			EndColumn:   hashStart + len(hashes) + 2,
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

// atxHeadingMultiSpacePattern matches ATX headings with multiple spaces after hashes.
var atxHeadingMultiSpacePattern = regexp.MustCompile(`^(#{1,6})([ \t]{2,})(\S)`)

// Apply checks for multiple spaces after hash on ATX headings.
func (r *NoMultipleSpaceATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		trimmed := bytes.TrimLeft(lineContent, " \t")

		match := atxHeadingMultiSpacePattern.FindSubmatch(trimmed)
		if match == nil {
			continue
		}

		hashes := string(match[1])
		spaces := string(match[2])
		line := ctx.File.Lines[lineNum-1]

		// Find the position of the hashes in the original line.
		hashStart := bytes.Index(lineContent, match[1])
		if hashStart < 0 {
			continue
		}

		// Build fix: replace multiple spaces with single space.
		builder := fix.NewEditBuilder()
		spaceStart := line.StartOffset + hashStart + len(hashes)
		spaceEnd := spaceStart + len(spaces)
		builder.ReplaceRange(spaceStart, spaceEnd, " ")

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: hashStart + len(hashes) + 1,
			EndLine:     lineNum,
			EndColumn:   hashStart + len(hashes) + len(spaces) + 1,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Multiple spaces (%d) after hash on ATX style heading", len(spaces))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a single space after the hash characters").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
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

// closedATXPattern matches closed ATX headings.
var closedATXPattern = regexp.MustCompile(`^(#{1,6})(.+?)(#{1,6})\s*$`)

// Apply checks for missing space inside hashes on closed ATX headings.
func (r *NoMissingSpaceClosedATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		trimmed := bytes.TrimLeft(lineContent, " \t")

		match := closedATXPattern.FindSubmatch(trimmed)
		if match == nil {
			continue
		}

		openHashes := string(match[1])
		content := match[2]
		closeHashes := string(match[3])

		// Check if there's no space at the beginning of content.
		missingOpenSpace := len(content) > 0 && content[0] != ' ' && content[0] != '\t'
		// Check if there's no space at the end of content.
		missingCloseSpace := len(content) > 0 && content[len(content)-1] != ' ' && content[len(content)-1] != '\t'

		if !missingOpenSpace && !missingCloseSpace {
			continue
		}

		line := ctx.File.Lines[lineNum-1]
		hashStart := bytes.Index(lineContent, match[1])
		if hashStart < 0 {
			continue
		}

		// Build fix.
		builder := fix.NewEditBuilder()
		contentStr := strings.TrimSpace(string(content))

		// Calculate positions and build replacement.
		contentStart := line.StartOffset + hashStart + len(openHashes)
		contentEnd := contentStart + len(content)

		var newContent string
		switch {
		case missingOpenSpace && missingCloseSpace:
			newContent = " " + contentStr + " "
		case missingOpenSpace:
			newContent = " " + string(content)
		default:
			newContent = string(content) + " "
		}

		builder.ReplaceRange(contentStart, contentEnd, newContent)

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: hashStart + 1,
			EndLine:     lineNum,
			EndColumn:   hashStart + len(openHashes) + len(content) + len(closeHashes) + 1,
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
func (r *NoMultipleSpaceClosedATXRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		trimmed := bytes.TrimLeft(lineContent, " \t")

		match := closedATXPattern.FindSubmatch(trimmed)
		if match == nil {
			continue
		}

		openHashes := string(match[1])
		content := match[2]
		closeHashes := string(match[3])

		// Check for multiple spaces at the beginning.
		multipleOpenSpaces := len(content) >= 2 &&
			(content[0] == ' ' || content[0] == '\t') &&
			(content[1] == ' ' || content[1] == '\t')

		// Check for multiple spaces at the end.
		multipleCloseSpaces := len(content) >= 2 &&
			(content[len(content)-1] == ' ' || content[len(content)-1] == '\t') &&
			(content[len(content)-2] == ' ' || content[len(content)-2] == '\t')

		if !multipleOpenSpaces && !multipleCloseSpaces {
			continue
		}

		line := ctx.File.Lines[lineNum-1]
		hashStart := bytes.Index(lineContent, match[1])
		if hashStart < 0 {
			continue
		}

		// Build fix: normalize to single space on each side.
		builder := fix.NewEditBuilder()
		contentStr := strings.TrimSpace(string(content))

		contentStart := line.StartOffset + hashStart + len(openHashes)
		contentEnd := contentStart + len(content)

		newContent := " " + contentStr + " "
		builder.ReplaceRange(contentStart, contentEnd, newContent)

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: hashStart + 1,
			EndLine:     lineNum,
			EndColumn:   hashStart + len(openHashes) + len(content) + len(closeHashes) + 1,
		}

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
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
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
		if lint.IsLineInCodeBlock(ctx.File, ctx.Root, lineNum) {
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

	headings := lint.Headings(ctx.Root)
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

	var diags []lint.Diagnostic
	var parentStack []parentInfo

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

// defaultPunctuation is the default set of trailing punctuation characters.
const defaultPunctuation = ".,;:!"

// htmlEntityPattern matches HTML entity references at the end of text.
var htmlEntityPattern = regexp.MustCompile(`&[a-zA-Z]+;$|&#[0-9]+;$|&#x[0-9a-fA-F]+;$`)

// Apply checks for trailing punctuation in headings.
func (r *NoTrailingPunctuationRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	punctuation := ctx.OptionString("punctuation", defaultPunctuation)
	if punctuation == "" {
		return nil, nil // Empty string disables the rule.
	}

	headings := lint.Headings(ctx.Root)
	var diags []lint.Diagnostic

	for _, heading := range headings {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		text := lint.HeadingText(heading)
		if text == "" {
			continue
		}

		// Check for HTML entity at the end - ignore if present.
		if htmlEntityPattern.MatchString(text) {
			continue
		}

		// Get the last rune.
		lastRune, _ := utf8.DecodeLastRuneInString(text)
		if lastRune == utf8.RuneError {
			continue
		}

		// Check if it's in the punctuation set.
		if !strings.ContainsRune(punctuation, lastRune) {
			continue
		}

		pos := heading.SourcePosition()
		line := ctx.File.Lines[pos.StartLine-1]
		lineContent := lint.LineContent(ctx.File, pos.StartLine)

		// Find the position of the trailing punctuation.
		// We need to find where the heading text ends in the line.
		trimmedLine := bytes.TrimRight(lineContent, " \t#\n\r")
		if len(trimmedLine) == 0 {
			continue
		}

		punctPos := len(trimmedLine) - utf8.RuneLen(lastRune)

		// Build fix: remove trailing punctuation.
		builder := fix.NewEditBuilder()
		builder.Delete(line.StartOffset+punctPos, line.StartOffset+punctPos+utf8.RuneLen(lastRune))

		diagPos := mdast.SourcePosition{
			StartLine:   pos.StartLine,
			StartColumn: punctPos + 1,
			EndLine:     pos.StartLine,
			EndColumn:   punctPos + 2,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos,
			fmt.Sprintf("Heading ends with trailing punctuation %q", string(lastRune))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove trailing punctuation from the heading").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}
