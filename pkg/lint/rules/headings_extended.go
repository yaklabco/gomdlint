package rules

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	gmhtml "github.com/yuin/goldmark/renderer/html"

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

// keycapHash is the keycap number sign emoji (#, U+FE0F, U+20E3), which
// starts with a hash but is not a heading.
const keycapHash = "#\uFE0F\u20E3"

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
	checker := newFixChecker(ctx)
	htmlLines := htmlBlockLines(ctx)

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}

		if ctx.IsLineInCodeBlock(lineNum) || htmlLines[lineNum] {
			continue
		}

		lineContent, bom := lint.LineText(ctx.File, lineNum), lint.LineTextOffset(ctx.File, lineNum)
		match := atxHeadingNoSpacePattern.FindSubmatch(lineContent)
		if match == nil || closingHashPattern.Match(lineContent) || bytes.HasPrefix(lineContent, []byte(keycapHash)) {
			continue
		}

		hashes := match[1]
		line := ctx.File.Lines[lineNum-1]

		// Build fix: insert space after hashes.
		hashStart := line.StartOffset + bom
		hashRange := mdast.SourceRange{StartOffset: hashStart, EndOffset: hashStart + len(hashes)}
		var offered *fix.EditBuilder
		if intent, ok := lineHeadingIntent(ctx, line, hashStart, headingAttrs(len(hashes), StyleATX), false, hashRange); ok {
			builder := fix.NewEditBuilder()
			builder.Insert(hashRange.EndOffset, " ")
			offered = checker.offer(builder, intent.withLocal(true))
		}

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: bom + 1,
			EndLine:     lineNum,
			EndColumn:   bom + len(hashes) + 2,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			"No space after hash on ATX style heading").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add a space after the hash characters").
			WithFix(offered).
			Build()
		diags = append(diags, diag)
	}

	return checker.filter(diags), nil
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
	checker := newFixChecker(ctx)

	for _, heading := range ctx.Headings() {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}
		parts, ok := atxHeadingParts(ctx.File, heading)
		if !ok || !parts.close.IsEmpty() || parts.openSpace.Len() < 2 {
			continue
		}
		spaces := parts.openSpace

		// Build fix: replace multiple spaces with single space.
		builder := fix.NewEditBuilder()
		builder.ReplaceRange(spaces.StartOffset, spaces.EndOffset, " ")

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, rangePosition(ctx.File, spaces),
			fmt.Sprintf("Multiple spaces (%d) after hash on ATX style heading", spaces.Len())).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a single space after the hash characters").
			WithFix(checker.offer(builder, fixIntent{local: true})).
			Build()
		diags = append(diags, diag)
	}

	return checker.filter(diags), nil
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
	checker := newFixChecker(ctx)
	htmlLines := htmlBlockLines(ctx)

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}

		if ctx.IsLineInCodeBlock(lineNum) || htmlLines[lineNum] {
			continue
		}

		lineContent, bom := lint.LineText(ctx.File, lineNum), lint.LineTextOffset(ctx.File, lineNum)
		loc := closedATXPattern.FindSubmatchIndex(lineContent)
		if loc == nil {
			continue
		}
		match := make([][]byte, len(loc)/2)
		for group := range match {
			match[group] = lineContent[loc[2*group]:loc[2*group+1]]
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
		textStart := line.StartOffset + bom
		openRange := mdast.SourceRange{
			StartOffset: textStart + loc[2*closedATXOpen], EndOffset: textStart + loc[2*closedATXOpen+1],
		}
		closeRange := mdast.SourceRange{
			StartOffset: textStart + loc[2*closedATXClose], EndOffset: textStart + loc[2*closedATXClose+1],
		}
		var offered *fix.EditBuilder
		if intent, ok := lineHeadingIntent(ctx, line, textStart,
			headingAttrs(len(openHashes), StyleATXClosed), true, openRange, closeRange); ok {
			builder := fix.NewEditBuilder()
			builder.ReplaceRange(textStart, line.NewlineStart, replacement)
			offered = checker.offer(builder, intent.withLocal(true))
		}

		closeEnd := bom + len(lineContent) - len(match[closedATXTrail])
		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: bom + 1,
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
			WithFix(offered).
			Build()
		diags = append(diags, diag)
	}

	return checker.filter(diags), nil
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
	checker := newFixChecker(ctx)

	for _, heading := range ctx.Headings() {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}
		parts, ok := atxHeadingParts(ctx.File, heading)
		if !ok || parts.close.IsEmpty() {
			continue
		}
		openSpaces, closeSpaces := parts.openSpace, parts.closeSpace

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

		pos := rangePosition(ctx.File, mdast.SourceRange{StartOffset: parts.open.StartOffset, EndOffset: parts.close.EndOffset})

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
			WithFix(checker.offer(builder, fixIntent{local: true})).
			Build()
		diags = append(diags, diag)
	}

	return checker.filter(diags), nil
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

// Apply checks that headings start at the beginning of the line.
//
// Like markdownlint, it checks parsed headings (ATX and setext) and measures
// indentation from the content column of the heading's containers: the
// blockquote markers and list item indentation do not count, so a heading
// at a list item's content column is not reported. The fix deletes only the
// extra indentation and keeps the container prefix.
func (r *HeadingStartLeftRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || ctx.Root == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	checker := newFixChecker(ctx)

	for _, heading := range ctx.Headings() {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}

		start := heading.SourceRange().StartOffset
		line := lineOf(ctx.File, start)
		if line == nil {
			continue
		}
		lineNum, _ := ctx.File.LineAt(start)
		// The container prefix includes a byte order mark that starts the file.
		prefixEnd := line.StartOffset + len(lint.ContainerPrefix(ctx.File, heading, lineNum))
		if prefixEnd >= start {
			continue
		}
		indent := ctx.File.Content[prefixEnd:start]
		if len(bytes.Trim(indent, " \t")) > 0 {
			continue
		}

		// Build fix: remove the indentation after the container prefix. A tab
		// that the containers claim only in part is not in the prefix; the
		// columns they claim of it are kept as spaces.
		claimed := len(lint.ContinuationPrefix(ctx.File, heading)) -
			columnsOf(ctx.File.Content[line.StartOffset+lint.LineTextOffset(ctx.File, lineNum):prefixEnd])
		builder := fix.NewEditBuilder()
		builder.ReplaceRange(prefixEnd, start, strings.Repeat(" ", max(claimed, 0)))

		pos := rangePosition(ctx.File, mdast.SourceRange{StartOffset: prefixEnd, EndOffset: start})
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Heading is indented by %d character(s)", len(indent))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove leading whitespace from the heading").
			WithFix(checker.offer(builder, fixIntent{local: true})).
			Build()
		diags = append(diags, diag)
	}

	return checker.filter(diags), nil
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

		text := markdownlintHeadingText(ctx.File, heading)
		if first, ok := seen[text]; ok {
			firstPos := first.SourcePosition()
			diag := lint.NewDiagnostic(r.ID(), heading,
				fmt.Sprintf("Duplicate heading text %q (first occurrence on line %d)", strings.TrimSpace(text), firstPos.StartLine)).
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

// checkSiblings compares each heading with the earlier headings of its level
// since the last heading of a lower level, as markdownlint does: going down
// a level starts an empty list for the deeper level, and going back up
// forgets the lists below.
func (r *NoDuplicateHeadingRule) checkSiblings(ctx *lint.RuleContext, headings []*mdast.Node) []lint.Diagnostic {
	const maxLevel = 6
	var diags []lint.Diagnostic
	seen := make([]map[string]*mdast.Node, maxLevel+1)
	seen[1] = make(map[string]*mdast.Node)
	lastLevel := 1

	for _, heading := range headings {
		if ctx.Cancelled() {
			break
		}

		level := min(max(lint.HeadingLevel(heading), 1), maxLevel)
		for lastLevel < level {
			lastLevel++
			seen[lastLevel] = make(map[string]*mdast.Node)
		}
		for lastLevel > level {
			seen[lastLevel] = nil
			lastLevel--
		}

		text := markdownlintHeadingText(ctx.File, heading)
		if first, ok := seen[level][text]; ok {
			firstPos := first.SourcePosition()
			diag := lint.NewDiagnostic(r.ID(), heading,
				fmt.Sprintf("Duplicate sibling heading text %q (first occurrence on line %d)", strings.TrimSpace(text), firstPos.StartLine)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use unique heading text among siblings").
				Build()
			diags = append(diags, diag)
		} else {
			seen[level][text] = heading
		}
	}

	return diags
}

// markdownlintHeadingText returns a heading's text the way markdownlint's
// getHeadingText reads it, for MD024 to compare: the text as written,
// without the raw HTML directly in it, with each line ending read as a
// space. An ATX heading leaves out the whitespace around its text. A setext
// heading leaves out the indentation of its first line but keeps the prefix
// of each later line, such as "> " or a list item's indentation, and the
// whitespace at the end of its last line.
func markdownlintHeadingText(file *mdast.FileSnapshot, heading *mdast.Node) string {
	content := heading.ContentRange()
	if file == nil || content.StartOffset < 0 || content.EndOffset < content.StartOffset ||
		content.EndOffset > len(file.Content) {
		return lint.HeadingText(heading)
	}
	start, end := content.StartOffset, content.EndOffset
	if isSetextHeading(file, heading) {
		for end < len(file.Content) && (file.Content[end] == ' ' || file.Content[end] == '\t') {
			end++
		}
	} else {
		for end > start && (file.Content[end-1] == ' ' || file.Content[end-1] == '\t') {
			end--
		}
	}

	var text strings.Builder
	pos := start
	for child := heading.FirstChild; child != nil; child = child.Next {
		span := child.SourceRange()
		if child.Kind != mdast.NodeHTMLInline || span.StartOffset < pos || span.EndOffset > end {
			continue
		}
		text.Write(file.Content[pos:span.StartOffset])
		pos = span.EndOffset
	}
	text.Write(file.Content[pos:end])
	return markdownlintLineEndingsAsSpaces.Replace(text.String())
}

// markdownlintLineEndingsAsSpaces replaces each line ending with a space, as
// markdownlint's newlineRe does.
//
//nolint:gochecknoglobals // A Replacer is safe for concurrent use and costly to build.
var markdownlintLineEndingsAsSpaces = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ")

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
const defaultPunctuation = ".,;:!\u3002\uFF0C\uFF1B\uFF1A\uFF01"

// htmlEntityPattern matches an HTML entity reference at the end of text
// (markdownlint's endOfLineHtmlEntityRe).
var htmlEntityPattern = regexp.MustCompile(
	`&(?:#\d+|#[xX][\da-fA-F]+|[a-zA-Z]{2,31}|blk\d{2}|emsp1[34]|frac\d{2}|sup\d|there4);$`)

// characterReferencePattern matches a named, decimal or hexadecimal
// character reference such as "&copy;", "&#169;" or "&#xA9;".
var characterReferencePattern = regexp.MustCompile(
	`&(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{1,31});`)

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
	checker := newFixChecker(ctx)

	for _, heading := range headings {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
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
		// The semicolon that ends a character reference such as "&copy;"
		// belongs to the reference, not to the trailing punctuation.
		for _, ref := range characterReferencePattern.FindAllIndex(text, -1) {
			if ref[1] > punctStart {
				punctStart = ref[1]
			}
		}
		if punctStart >= len(text) {
			continue
		}
		run := text[punctStart:]
		start := content.StartOffset + punctStart

		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, rangePosition(ctx.File,
			mdast.SourceRange{StartOffset: start, EndOffset: content.EndOffset}),
			fmt.Sprintf("Heading ends with trailing punctuation %q", run)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove trailing punctuation from the heading")
		if builder := trailingPunctuationFix(ctx, checker, heading, content, start); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return checker.filter(diags), nil
}

// trailingPunctuationFix deletes the punctuation starting at start, with the
// backslash that escapes it and the whitespace before them on the same line.
// It returns nil unless the fix changes nothing but the heading's text, and
// that only by the deleted characters: removing the punctuation must not
// leave the heading showing nothing (see showsContent), leave a
// setext line blank or turn it into a thematic break, underline or other
// block, expose a closing hash sequence, leave a backslash showing, or
// change an anchor that a fragment link uses.
func trailingPunctuationFix(
	ctx *lint.RuleContext,
	checker *fixChecker,
	heading *mdast.Node,
	content mdast.SourceRange,
	start int,
) *fix.EditBuilder {
	file := ctx.File
	line := lineOf(file, start)
	if line == nil {
		return nil
	}
	lineStart := max(line.StartOffset, content.StartOffset)
	deleteStart := start
	if isASCIIPunctuation(file.Content[start]) && escapedAt(file.Content, lineStart, start) {
		deleteStart--
	}
	for deleteStart > lineStart && (file.Content[deleteStart-1] == ' ' || file.Content[deleteStart-1] == '\t') {
		deleteStart--
	}
	if !showsContent(file, heading, deleteStart) {
		return nil
	}
	deleted := mdast.SourceRange{StartOffset: deleteStart, EndOffset: content.EndOffset}
	builder := fix.NewEditBuilder()
	builder.Delete(deleted.StartOffset, deleted.EndOffset)
	return checker.offer(builder, changeText(heading, textSub{rng: deleted}).withLocal(true))
}

// showsContent reports whether the inline content of node that starts before
// offset shows something: a visible character once escapes and character
// references are resolved, a code span, or an image. Raw HTML, a task
// checkbox and a link without text show nothing that counts. MD026 asks it
// about a heading, which never holds a checkbox: the parser reads one only
// at the start of a paragraph, as GFM does, so "[ ]" in a heading is text or
// the text of a link. A heading left showing nothing renders differently and
// gets another anchor.
func showsContent(file *mdast.FileSnapshot, node *mdast.Node, offset int) bool {
	for child := node.FirstChild; child != nil; child = child.Next {
		span := child.SourceRange()
		if span.StartOffset >= offset || span.EndOffset > len(file.Content) {
			break
		}
		switch child.Kind {
		case mdast.NodeCodeSpan, mdast.NodeImage:
			return true
		case mdast.NodeText:
			if showsText(file.Content[span.StartOffset:min(span.EndOffset, offset)]) {
				return true
			}
		case mdast.NodeHTMLInline, mdast.NodeTaskCheckbox:
		default:
			if showsContent(file, child, offset) {
				return true
			}
		}
	}
	return false
}

// showsText reports whether source text renders a visible character: one
// that is graphic, not a space and not default ignorable, so neither
// whitespace nor a character that renders nothing, such as U+200B or the
// variation selector U+FE0F, counts.
func showsText(source []byte) bool {
	var rendered bytes.Buffer
	writer := bufio.NewWriter(&rendered)
	gmhtml.DefaultWriter.Write(writer, source)
	_ = writer.Flush() // A bytes.Buffer never fails to write.
	return bytes.ContainsFunc(rendered.Bytes(), func(r rune) bool {
		return unicode.IsGraphic(r) && !unicode.IsSpace(r) && !defaultIgnorable(r)
	})
}

// defaultIgnorable reports whether char has Unicode's Default_Ignorable_Code_Point
// property, which marks characters that render nothing unless a renderer
// supports them. Go's unicode package lacks the derived property, so this
// derives it as DerivedCoreProperties.txt does: Other_Default_Ignorable_Code_Point,
// format characters and variation selectors, without white space, the
// interlinear annotation characters U+FFF9 to U+FFFB, the Egyptian
// hieroglyph format characters U+13430 to U+1343F, and the prepended
// concatenation marks.
func defaultIgnorable(char rune) bool {
	switch {
	case unicode.Is(unicode.White_Space, char),
		char >= 0xFFF9 && char <= 0xFFFB,
		char >= 0x13430 && char <= 0x1343F,
		unicode.Is(unicode.Prepended_Concatenation_Mark, char):
		return false
	}
	return unicode.Is(unicode.Other_Default_Ignorable_Code_Point, char) ||
		unicode.Is(unicode.Cf, char) ||
		unicode.Is(unicode.Variation_Selector, char)
}

// escapedAt reports whether the byte at offset follows an odd number of
// backslashes, counting back no further than from, so that a backslash
// escapes it.
func escapedAt(content []byte, from, offset int) bool {
	count := 0
	for pos := offset - 1; pos >= from && content[pos] == '\\'; pos-- {
		count++
	}
	return count%2 == 1
}

// columnsOf returns the number of columns that text takes from the start of
// a line, with tab stops of four.
func columnsOf(text []byte) int {
	const tabWidth = 4
	columns := 0
	for _, b := range text {
		if b == '\t' {
			columns += tabWidth - columns%tabWidth
		} else {
			columns++
		}
	}
	return columns
}

// lineHeadingIntent returns the intent of a fix that turns a line, whose
// text starts at textStart, into an ATX heading with attrs whose text is the
// line's without the hash sequences in markers. The line may be a paragraph
// or part of one, which then splits around the heading, or, when
// atxHeading is true, an ATX heading already. ok is false for other blocks.
func lineHeadingIntent(
	ctx *lint.RuleContext,
	line mdast.LineInfo,
	textStart int,
	attrs string,
	atxHeading bool,
	markers ...mdast.SourceRange,
) (fixIntent, bool) {
	target := leafBlockAt(ctx.Root, textStart)
	if target == nil {
		return fixIntent{}, false
	}
	subs := make([]textSub, 0, len(markers))
	for _, marker := range markers {
		subs = append(subs, textSub{rng: marker})
	}
	// The pieces cover whole lines of the target, so that the intents of
	// fixes to several of its lines can be merged.
	rng := target.SourceRange()
	spanStart, spanEnd := lineStartOf(ctx.File, rng.StartOffset), lineEndOf(ctx.File, rng.EndOffset)
	heading := blockWant{
		kind:  mdast.NodeHeading,
		attrs: attrs,
		from:  mdast.SourceRange{StartOffset: line.StartOffset, EndOffset: line.EndOffset},
		subs:  subs,
	}
	switch {
	case target.Kind == mdast.NodeHeading && atxHeading && !isSetextHeading(ctx.File, target):
		heading.from = mdast.SourceRange{StartOffset: spanStart, EndOffset: spanEnd}
		return fixIntent{target: target, into: []blockWant{heading}}, true
	case target.Kind != mdast.NodeParagraph:
		return fixIntent{}, false
	}
	var into []blockWant
	if spanStart < line.StartOffset {
		into = append(into, blockWant{
			kind: mdast.NodeParagraph,
			from: mdast.SourceRange{StartOffset: spanStart, EndOffset: line.StartOffset},
		})
	}
	into = append(into, heading)
	if line.EndOffset < spanEnd {
		into = append(into, blockWant{
			kind: mdast.NodeParagraph,
			from: mdast.SourceRange{StartOffset: line.EndOffset, EndOffset: spanEnd},
		})
	}
	return fixIntent{target: target, into: into}, true
}
