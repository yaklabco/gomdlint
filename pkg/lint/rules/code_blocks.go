package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/langdetect"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// CodeBlockLanguageRule checks that fenced code blocks have a language specified.
type CodeBlockLanguageRule struct {
	lint.BaseRule
}

// NewCodeBlockLanguageRule creates a new code block language rule.
func NewCodeBlockLanguageRule() *CodeBlockLanguageRule {
	return &CodeBlockLanguageRule{
		BaseRule: lint.NewBaseRule(
			"MD040",
			"fenced-code-language",
			"Fenced code blocks should have a language specified",
			[]string{"code"},
			true, // Auto-fixable via language detection.
		),
	}
}

// Apply checks that fenced code blocks have an info string. Like
// markdownlint, it reads the info string as GFM does (see fenceInfo),
// compares the language, its first word (see infoWords), with each name in
// allowed_languages exactly, so case matters, and checks that list and
// language_only independently.
func (r *CodeBlockLanguageRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	allowed := allowedLanguages(ctx)
	codeBlocks := ctx.CodeBlocks()
	var diags []lint.Diagnostic

	for _, cb := range codeBlocks {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip indented code blocks.
		if lint.IsIndentedCodeBlock(cb) {
			continue
		}

		words := infoWords(fenceInfo(ctx.File, cb))
		if ctx.OptionBool("language_only", false) && len(words) > 1 {
			diags = append(diags, lint.NewDiagnostic(r.ID(), cb, "Code fence info must contain only a language identifier").
				WithSeverity(config.SeverityWarning).Build())
		}

		if len(words) == 0 {
			diagBuilder := lint.NewDiagnostic(r.ID(), cb,
				"Fenced code block has no language specified").
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Add a language identifier after the opening fence")

			// Add autofix if file is available.
			if ctx.File != nil {
				if fixer := r.buildLanguageFix(ctx.File, cb, allowed); fixer != nil {
					diagBuilder = diagBuilder.WithFix(fixer)
				}
			}

			diags = append(diags, diagBuilder.Build())
			continue
		}

		// Check against allowed languages if configured.
		if language := words[0]; allowed != nil && !allowed[language] {
			diag := lint.NewDiagnostic(r.ID(), cb,
				fmt.Sprintf("Language '%s' is not in the allowed list", language)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use one of the allowed language identifiers").
				Build()
			diags = append(diags, diag)
		}
	}

	return verifyFixes(ctx, diags, fixAllowance{codeInfo: true}), nil
}

// infoWords splits a fence's info string into words at spaces and tabs, as
// GFM and markdownlint do. The first word is the language. Other space
// characters, such as U+00A0, belong to the words they stand in.
func infoWords(info string) []string {
	return strings.FieldsFunc(info, func(r rune) bool { return r == ' ' || r == '\t' })
}

// fenceInfo returns the info string of a fenced code block as GFM reads it:
// the text after the opening fence characters to the end of the line, where
// a carriage return ends a line as well as a line feed, without the spaces
// and tabs around it. goldmark ends lines only at line feeds, so its info
// string can take in the text after a bare carriage return, and it drops a
// carriage return that comes before the info string.
func fenceInfo(file *mdast.FileSnapshot, cb *mdast.Node) string {
	if after, ok := afterOpeningFence(file, cb); ok {
		if rest, ok := restOfLine(file, after); ok {
			return infoLine(rest)
		}
	}
	return infoLine(lint.CodeBlockInfo(cb))
}

// infoLine returns text up to its first carriage return or line feed,
// without the spaces and tabs around it.
func infoLine(text string) string {
	if end := strings.IndexAny(text, "\r\n"); end >= 0 {
		text = text[:end]
	}
	return strings.Trim(text, " \t")
}

// afterOpeningFence returns the offset just past the fence characters of a
// fenced code block's opening fence, when the source holds them where the
// block starts.
func afterOpeningFence(file *mdast.FileSnapshot, cb *mdast.Node) (int, bool) {
	if file == nil {
		return 0, false
	}
	start, length, char := cb.SourceRange().StartOffset, lint.CodeFenceLength(cb), lint.CodeFenceChar(cb)
	if length == 0 || start < 0 || start+length > len(file.Content) {
		return 0, false
	}
	for _, ch := range file.Content[start : start+length] {
		if ch != char {
			return 0, false
		}
	}
	return start + length, true
}

// restOfLine returns the source from offset to the end of its line, without
// the line ending. Lines end only at line feeds here, as in goldmark, so the
// text can hold a bare carriage return.
func restOfLine(file *mdast.FileSnapshot, offset int) (string, bool) {
	line, _ := file.LineAt(offset)
	if line < 1 || line > len(file.Lines) || offset > file.Lines[line-1].NewlineStart {
		return "", false
	}
	return string(file.Content[offset:file.Lines[line-1].NewlineStart]), true
}

// allowedLanguages returns the names that allowed_languages permits, or nil
// when it sets no list and every language is allowed. As in markdownlint, an
// empty list allows every language. A list with an item that is no string
// has the wrong type, so the registry drops it before the rule runs, and
// every language is allowed; given such a list directly, the rule allows
// only its strings.
func allowedLanguages(ctx *lint.RuleContext) map[string]bool {
	names, ok := ctx.Option("allowed_languages", nil).([]any)
	if !ok || len(names) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		if language, ok := name.(string); ok {
			allowed[language] = true
		}
	}
	return allowed
}

// buildLanguageFix detects the language and creates a fix to insert it. It
// returns nil when no language is detected, when allowed, which is nil when
// every language is allowed, leaves out the language detected, or when the
// fixed fence would name another language than the one inserted: the rule
// would report such a language on the next run, and the fix check reads no
// info string. It also returns nil when the fence line holds a bare carriage
// return: GFM ends the line there and reads what follows as code, which
// goldmark reads into the fence, so detection would not see the block's code.
func (r *CodeBlockLanguageRule) buildLanguageFix(
	file *mdast.FileSnapshot,
	cb *mdast.Node,
	allowed map[string]bool,
) *fix.EditBuilder {
	// The language goes right after the opening fence characters.
	insertAt, ok := afterOpeningFence(file, cb)
	if !ok {
		return nil
	}
	if rest, ok := restOfLine(file, insertAt); !ok || strings.ContainsRune(rest, '\r') {
		return nil
	}

	// Get code block content for detection.
	content := r.getCodeBlockContent(file, cb)
	if len(content) == 0 {
		return nil
	}

	// Detect language.
	detectedLang := langdetect.Detect(content)
	if detectedLang == "text" {
		return nil // Don't insert "text" as language.
	}
	if allowed != nil && !allowed[detectedLang] {
		return nil
	}
	if fenceLanguageAfterInsert(file, insertAt, detectedLang) != detectedLang {
		return nil
	}
	builder := fix.NewEditBuilder()
	builder.Insert(insertAt, detectedLang)
	return builder
}

// fenceLanguageAfterInsert returns the language that the line holding offset,
// an opening fence that ends at offset, would name with text inserted at
// offset: the first word of the info string that would follow the fence.
func fenceLanguageAfterInsert(file *mdast.FileSnapshot, offset int, text string) string {
	rest, ok := restOfLine(file, offset)
	if !ok {
		return ""
	}
	words := infoWords(infoLine(text + rest))
	if len(words) == 0 {
		return ""
	}
	return words[0]
}

// getCodeBlockContent extracts the content lines of a code block (excluding fences).
func (r *CodeBlockLanguageRule) getCodeBlockContent(
	file *mdast.FileSnapshot,
	cb *mdast.Node,
) []byte {
	if cb.ContentRange().IsEmpty() {
		return nil
	}
	pos := cb.ContentPosition()
	if !pos.IsValid() {
		return nil
	}

	startLine := pos.StartLine
	endLine := pos.EndLine

	if startLine > endLine || startLine < 1 || endLine > len(file.Lines) {
		return nil
	}

	startOffset := file.Lines[startLine-1].StartOffset
	endOffset := file.Lines[endLine-1].NewlineStart

	if endOffset > len(file.Content) {
		endOffset = len(file.Content)
	}

	return file.Content[startOffset:endOffset]
}

// CodeBlockStyleRule enforces consistent code block style (fenced vs indented).
type CodeBlockStyleRule struct {
	lint.BaseRule
}

// NewCodeBlockStyleRule creates a new code block style rule.
func NewCodeBlockStyleRule() *CodeBlockStyleRule {
	return &CodeBlockStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD046",
			"code-block-style",
			"Code block style should be consistent",
			[]string{"code", "style"},
			false, // Not auto-fixable (complex transformation).
		),
	}
}

// CodeBlockStyle represents the style of code blocks.
type CodeBlockStyle string

const (
	// CodeBlockFenced uses fenced code blocks (```).
	CodeBlockFenced CodeBlockStyle = "fenced"
	// CodeBlockIndented uses indented code blocks.
	CodeBlockIndented CodeBlockStyle = "indented"
	// CodeBlockConsistent uses whatever style is first encountered.
	CodeBlockConsistent CodeBlockStyle = "consistent"
)

// Apply checks that code blocks use a consistent style.
func (r *CodeBlockStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	configStyle := CodeBlockStyle(ctx.OptionString("style", string(CodeBlockConsistent)))
	effectiveStyle := configStyle
	if configStyle == CodeBlockConsistent {
		effectiveStyle = "" // Will be set from first code block.
	}

	codeBlocks := ctx.CodeBlocks()
	var diags []lint.Diagnostic

	for _, cb := range codeBlocks {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		var detectedStyle CodeBlockStyle
		if lint.IsFencedCodeBlock(cb) {
			detectedStyle = CodeBlockFenced
		} else {
			detectedStyle = CodeBlockIndented
		}

		// Set consistent style from first code block.
		if effectiveStyle == "" {
			effectiveStyle = detectedStyle
			continue
		}

		// Check for style mismatch.
		if detectedStyle != effectiveStyle {
			msg := fmt.Sprintf("Code block style '%s' does not match expected '%s'",
				detectedStyle, effectiveStyle)

			diag := lint.NewDiagnostic(r.ID(), cb, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %s code blocks", effectiveStyle)).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// CodeFenceStyleRule enforces consistent code fence style (backtick vs tilde).
type CodeFenceStyleRule struct {
	lint.BaseRule
}

// NewCodeFenceStyleRule creates a new code fence style rule.
func NewCodeFenceStyleRule() *CodeFenceStyleRule {
	return &CodeFenceStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD048",
			"code-fence-style",
			"Code fence style should be consistent",
			[]string{"code", "style"},
			true, // Auto-fixable.
		),
	}
}

// FenceStyle represents the style of code fences.
type FenceStyle string

const (
	// FenceBacktick uses backticks (```).
	FenceBacktick FenceStyle = "backtick"
	// FenceTilde uses tildes (~~~).
	FenceTilde FenceStyle = "tilde"
	// FenceConsistent uses whatever style is first encountered.
	FenceConsistent FenceStyle = "consistent"
)

// Apply checks that fenced code blocks use a consistent fence style.
func (r *CodeFenceStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := FenceStyle(ctx.OptionString("style", string(FenceConsistent)))
	effectiveStyle := configStyle
	effectiveChar := byte('`')

	switch configStyle {
	case FenceConsistent:
		effectiveStyle = "" // Will be set from first fence.
		effectiveChar = 0
	case FenceTilde:
		effectiveChar = '~'
	case FenceBacktick:
		// Default values already set.
	}

	codeBlocks := ctx.CodeBlocks()
	var diags []lint.Diagnostic

	for _, cb := range codeBlocks {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip indented code blocks.
		if lint.IsIndentedCodeBlock(cb) {
			continue
		}

		fenceChar := lint.CodeFenceChar(cb)
		if fenceChar == 0 {
			continue
		}

		var detectedStyle FenceStyle
		if fenceChar == '`' {
			detectedStyle = FenceBacktick
		} else {
			detectedStyle = FenceTilde
		}

		// Set consistent style from first fence.
		if effectiveStyle == "" {
			effectiveStyle = detectedStyle
			effectiveChar = fenceChar
			continue
		}

		// Check for style mismatch.
		if fenceChar != effectiveChar {
			msg := fmt.Sprintf("Code fence style '%s' does not match expected '%s'",
				detectedStyle, effectiveStyle)

			builder := r.buildFenceFix(ctx.File, cb, effectiveChar)

			diagBuilder := lint.NewDiagnostic(r.ID(), cb, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %s for code fences", effectiveStyle))

			if builder != nil {
				diagBuilder = diagBuilder.WithFix(builder)
			}

			diags = append(diags, diagBuilder.Build())
		}
	}

	return verifyFixes(ctx, diags, fixAllowance{}), nil
}

// CommandsShowOutputRule checks for unnecessary dollar signs in shell code blocks.
type CommandsShowOutputRule struct {
	lint.BaseRule
}

// NewCommandsShowOutputRule creates a new commands-show-output rule.
func NewCommandsShowOutputRule() *CommandsShowOutputRule {
	return &CommandsShowOutputRule{
		BaseRule: lint.NewBaseRule(
			"MD014",
			"commands-show-output",
			"Dollar signs used before commands without showing output",
			[]string{"code"},
			true, // Auto-fixable
		),
	}
}

// dollarCommandPattern is markdownlint's dollarCommandRe: optional
// indentation, then a dollar sign followed by whitespace.
var dollarCommandPattern = regexp.MustCompile(`^(\s*)(\$\s+)`)

// dollarCommandIndices is the number of submatch indices dollarCommandPattern produces.
const dollarCommandIndices = 6

// codeTabWidth is the tab stop CommonMark uses to measure indentation.
const codeTabWidth = 4

// Apply reports dollar-sign prompts in code blocks that show no output. Like
// markdownlint, every fenced and indented code block is checked whatever its
// info string, a block is reported only when every non-blank line starts with
// "$" and whitespace, and each of those lines gets its own diagnostic and fix.
// A line is blank when nothing is left once the block's own indentation is
// removed; whitespace beyond it is code, which counts as output.
func (r *CommandsShowOutputRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	for _, cb := range ctx.CodeBlocks() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		diags = append(diags, r.checkCodeBlock(ctx, cb)...)
	}

	return verifyFixes(ctx, diags, fixAllowance{prompts: true}), nil
}

func (r *CommandsShowOutputRule) checkCodeBlock(ctx *lint.RuleContext, cb *mdast.Node) []lint.Diagnostic {
	type dollarLine struct {
		line    codeLineInfo
		indices []int
	}
	var commands []dollarLine
	indent := codeIndent(ctx.File, cb)
	for _, line := range codeBlockContentLines(ctx.File, cb) {
		if isBlankCodeLine(line.content, indent) {
			continue
		}
		indices := dollarCommandPattern.FindStringSubmatchIndex(line.content)
		if len(indices) < dollarCommandIndices {
			return nil // A line without a prompt is output, so the prompts are justified.
		}
		commands = append(commands, dollarLine{line: line, indices: indices})
	}

	diags := make([]lint.Diagnostic, 0, len(commands))
	for _, command := range commands {
		promptStart := command.line.startOffset + command.indices[4]
		promptEnd := command.line.startOffset + command.indices[5]
		startLine, startColumn := ctx.File.LineAt(promptStart)
		endLine, endColumn := ctx.File.LineAt(promptEnd)
		pos := mdast.SourcePosition{StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn}
		builder := fix.NewEditBuilder()
		builder.Delete(promptStart, promptEnd)
		diags = append(diags, lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			"Dollar signs used before commands without showing output").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove the dollar sign prompt from command-only code blocks").
			WithFix(builder).
			Build())
	}
	return diags
}

// codeBlockIndent is the indentation that makes an indented code block.
const codeBlockIndent = 4

// codeIndent returns how many columns of indentation belong to a code block
// rather than to its code: four for an indented block, or the indentation of
// a fenced block's opening fence after its container prefix.
func codeIndent(file *mdast.FileSnapshot, cb *mdast.Node) int {
	if lint.IsIndentedCodeBlock(cb) {
		return codeBlockIndent
	}
	pos := cb.SourcePosition()
	line := lint.RawLineContent(file, pos.StartLine)
	if !pos.IsValid() || line == nil {
		return 0
	}
	indent := 0
	for _, ch := range line[min(len(lint.ContainerPrefix(file, cb, pos.StartLine)), len(line)):] {
		if ch != ' ' {
			break
		}
		indent++
	}
	return indent
}

// isBlankCodeLine reports whether a code line, read after its container
// prefix, holds nothing beyond indent columns of spaces and tabs.
func isBlankCodeLine(content string, indent int) bool {
	column := 0
	for idx := range len(content) {
		switch content[idx] {
		case ' ':
			column++
		case '\t':
			column += codeTabWidth - column%codeTabWidth
		default:
			return false
		}
		if column > indent {
			return false
		}
	}
	return true
}

type codeLineInfo struct {
	content     string
	startOffset int
	lineNum     int
}

// codeBlockContentLines returns the lines of a code block's content span,
// excluding fence lines. Each line starts after the prefix of the enclosing
// blockquotes and list items, so it begins with the code's own indentation.
func codeBlockContentLines(file *mdast.FileSnapshot, cb *mdast.Node) []codeLineInfo {
	if file == nil || cb.ContentRange().IsEmpty() {
		return nil
	}
	pos := cb.ContentPosition()
	if !pos.IsValid() {
		return nil
	}

	var lines []codeLineInfo
	for lineNum := pos.StartLine; lineNum <= pos.EndLine && lineNum <= len(file.Lines); lineNum++ {
		lineInfo := file.Lines[lineNum-1]
		start := lineInfo.StartOffset + len(lint.ContainerPrefix(file, cb, lineNum))
		lines = append(lines, codeLineInfo{
			content:     string(file.Content[start:lineInfo.NewlineStart]),
			startOffset: start,
			lineNum:     lineNum,
		})
	}
	return lines
}

func (r *CodeFenceStyleRule) buildFenceFix(
	file *mdast.FileSnapshot,
	cb *mdast.Node,
	expectedChar byte,
) *fix.EditBuilder {
	if file == nil || cb == nil {
		return nil
	}

	fenceLength := lint.CodeFenceLength(cb)
	span := cb.SourceRange()
	if fenceLength < 3 || span.StartOffset+fenceLength > len(file.Content) || !lint.IsFencedCodeBlock(cb) {
		return nil
	}
	pos := cb.SourcePosition()
	if !pos.IsValid() || pos.EndLine > len(file.Lines) {
		return nil
	}
	closed := cb.Block.CodeBlock.Closed

	// A backtick fence cannot have a backtick in its info string.
	if expectedChar == '`' && strings.Contains(lint.CodeBlockInfo(cb), "`") {
		return nil
	}

	// The new fence must not be closed early by a content line.
	lastContentLine := pos.EndLine
	if closed {
		lastContentLine--
	}
	for line := pos.StartLine + 1; line <= lastContentLine; line++ {
		if closesFence(file, cb, line, expectedChar, fenceLength) {
			return nil
		}
	}

	builder := fix.NewEditBuilder()
	builder.ReplaceRange(span.StartOffset, span.StartOffset+fenceLength, strings.Repeat(string(expectedChar), fenceLength))

	if closed && pos.EndLine > pos.StartLine {
		start, end, ok := fenceRun(file, cb, pos.EndLine)
		if !ok {
			return nil
		}
		builder.ReplaceRange(start, end, strings.Repeat(string(expectedChar), end-start))
	}

	return builder
}

// fenceRun returns the offsets of the run of fence characters that starts a
// line after its container prefix and up to three spaces of indentation.
func fenceRun(file *mdast.FileSnapshot, cb *mdast.Node, lineNum int) (int, int, bool) {
	line := file.Lines[lineNum-1]
	start := line.StartOffset + len(lint.ContainerPrefix(file, cb, lineNum))
	for indent := 0; indent < 3 && start < line.NewlineStart && file.Content[start] == ' '; indent++ {
		start++
	}
	if start >= line.NewlineStart || (file.Content[start] != '`' && file.Content[start] != '~') {
		return 0, 0, false
	}
	end := start
	for end < line.NewlineStart && file.Content[end] == file.Content[start] {
		end++
	}
	return start, end, true
}

// closesFence reports whether a content line would close a fence of
// fenceChar repeated fenceLength times: a run at least that long followed by
// nothing but spaces or tabs.
func closesFence(file *mdast.FileSnapshot, cb *mdast.Node, lineNum int, fenceChar byte, fenceLength int) bool {
	start, end, ok := fenceRun(file, cb, lineNum)
	if !ok || file.Content[start] != fenceChar || end-start < fenceLength {
		return false
	}
	rest := file.Content[end:file.Lines[lineNum-1].NewlineStart]
	return len(bytes.Trim(rest, " \t")) == 0
}
