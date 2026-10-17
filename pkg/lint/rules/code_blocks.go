package rules

import (
	"bytes"
	"fmt"
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

// Apply checks that fenced code blocks have an info string.
func (r *CodeBlockLanguageRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	allowedLanguages := ctx.Option("allowed_languages", nil)
	var allowedSet map[string]bool
	if langs, ok := allowedLanguages.([]any); ok && len(langs) > 0 {
		allowedSet = make(map[string]bool)
		for _, l := range langs {
			if s, ok := l.(string); ok {
				allowedSet[strings.ToLower(s)] = true
			}
		}
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

		info := lint.CodeBlockInfo(cb)
		if ctx.OptionBool("language_only", false) && len(strings.Fields(info)) > 1 {
			diags = append(diags, lint.NewDiagnostic(r.ID(), cb, "Code fence info must contain only a language identifier").
				WithSeverity(config.SeverityWarning).Build())
			continue
		}
		// Extract just the language part (first word).
		lang := strings.Fields(info)
		langName := ""
		if len(lang) > 0 {
			langName = strings.ToLower(lang[0])
		}

		if langName == "" {
			diagBuilder := lint.NewDiagnostic(r.ID(), cb,
				"Fenced code block has no language specified").
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Add a language identifier after the opening fence")

			// Add autofix if file is available.
			if ctx.File != nil {
				if fixer := r.buildLanguageFix(ctx.File, cb); fixer != nil {
					diagBuilder = diagBuilder.WithFix(fixer)
				}
			}

			diags = append(diags, diagBuilder.Build())
			continue
		}

		// Check against allowed languages if configured.
		if allowedSet != nil && !allowedSet[langName] {
			diag := lint.NewDiagnostic(r.ID(), cb,
				fmt.Sprintf("Language '%s' is not in the allowed list", langName)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use one of the allowed language identifiers").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// buildLanguageFix detects the language and creates a fix to insert it.
func (r *CodeBlockLanguageRule) buildLanguageFix(
	file *mdast.FileSnapshot,
	cb *mdast.Node,
) *fix.EditBuilder {
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

	// The full span of a fenced code block starts at its opening fence.
	fence := cb.SourceRange()
	fenceLength := lint.CodeFenceLength(cb)
	if fenceLength == 0 || fence.StartOffset+fenceLength > len(file.Content) {
		return nil
	}

	// Insert language right after fence.
	builder := fix.NewEditBuilder()
	builder.Insert(fence.StartOffset+fenceLength, detectedLang)
	return builder
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

	configStyle := CodeBlockStyle(ctx.OptionString("style", string(CodeBlockFenced)))
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

	configStyle := FenceStyle(ctx.OptionString("style", string(FenceBacktick)))
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

	return diags, nil
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

// Apply checks for unnecessary dollar signs in code blocks.
func (r *CommandsShowOutputRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	codeBlocks := ctx.CodeBlocks()
	var diags []lint.Diagnostic

	for _, cb := range codeBlocks {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		if diag := r.checkCodeBlock(ctx, cb); diag != nil {
			diags = append(diags, *diag)
		}
	}

	return diags, nil
}

func (r *CommandsShowOutputRule) checkCodeBlock(ctx *lint.RuleContext, cb *mdast.Node) *lint.Diagnostic {
	pos := cb.SourcePosition()
	if !pos.IsValid() {
		return nil
	}

	if !r.isShellCodeBlock(cb) {
		return nil
	}

	contentLines := r.getCodeBlockContentLines(ctx.File, pos)
	if len(contentLines) == 0 {
		return nil
	}

	if !r.hasOnlyDollarCommands(contentLines) {
		return nil
	}

	builder := r.buildDollarRemovalFix(contentLines)
	// Report at the first command line: the content span of the code block.
	diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, cb.ContentPosition(),
		"Dollar signs used before commands without showing output").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Remove dollar signs from command-only code blocks").
		WithFix(builder).
		Build()
	return &diag
}

func (r *CommandsShowOutputRule) isShellCodeBlock(cb *mdast.Node) bool {
	info := strings.ToLower(lint.CodeBlockInfo(cb))
	return info == "" || info == "sh" || info == "shell" || info == "bash" ||
		info == "zsh" || info == "console" || info == "terminal"
}

func (r *CommandsShowOutputRule) startsWithDollar(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "$\t") || trimmed == "$"
}

func (r *CommandsShowOutputRule) hasOnlyDollarCommands(lines []codeLineInfo) bool {
	hasAnyCommand := false

	for lineIdx, line := range lines {
		trimmed := strings.TrimSpace(line.content)
		if trimmed == "" {
			continue
		}

		if !r.startsWithDollar(trimmed) {
			return false
		}
		hasAnyCommand = true

		// Check if there's output after this command
		if r.hasOutputAfter(lines, lineIdx) {
			return false
		}
	}

	return hasAnyCommand
}

func (r *CommandsShowOutputRule) hasOutputAfter(lines []codeLineInfo, startIdx int) bool {
	for j := startIdx + 1; j < len(lines); j++ {
		nextTrimmed := strings.TrimSpace(lines[j].content)
		if nextTrimmed == "" {
			continue
		}
		// If next non-empty line doesn't start with $, it's output
		return !r.startsWithDollar(nextTrimmed)
	}
	return false
}

func (r *CommandsShowOutputRule) buildDollarRemovalFix(lines []codeLineInfo) *fix.EditBuilder {
	builder := fix.NewEditBuilder()
	for _, line := range lines {
		trimmed := strings.TrimSpace(line.content)
		if trimmed == "" {
			continue
		}

		dollarIdx := strings.Index(line.content, "$")
		if dollarIdx < 0 {
			continue
		}

		removeEnd := dollarIdx + 1
		if removeEnd < len(line.content) && (line.content[removeEnd] == ' ' || line.content[removeEnd] == '\t') {
			removeEnd++
		}
		builder.Delete(line.startOffset+dollarIdx, line.startOffset+removeEnd)
	}
	return builder
}

type codeLineInfo struct {
	content     string
	startOffset int
	lineNum     int
}

func (r *CommandsShowOutputRule) getCodeBlockContentLines(file *mdast.FileSnapshot, pos mdast.SourcePosition) []codeLineInfo {
	var lines []codeLineInfo

	// For fenced code blocks, skip the fence lines
	startLine := pos.StartLine
	endLine := pos.EndLine

	// Check if first line is a fence
	if startLine >= 1 && startLine <= len(file.Lines) {
		firstLineContent := string(lint.LineContent(file, startLine))
		if strings.HasPrefix(strings.TrimSpace(firstLineContent), "```") ||
			strings.HasPrefix(strings.TrimSpace(firstLineContent), "~~~") {
			startLine++
		}
	}

	// Check if last line is a fence
	if endLine >= 1 && endLine <= len(file.Lines) {
		lastLineContent := string(lint.LineContent(file, endLine))
		trimmedLast := strings.TrimSpace(lastLineContent)
		if trimmedLast == "```" || trimmedLast == "~~~" ||
			strings.HasPrefix(trimmedLast, "```") || strings.HasPrefix(trimmedLast, "~~~") {
			endLine--
		}
	}

	for lineNum := startLine; lineNum <= endLine && lineNum <= len(file.Lines); lineNum++ {
		lineInfo := file.Lines[lineNum-1]
		content := string(file.Content[lineInfo.StartOffset:lineInfo.NewlineStart])
		lines = append(lines, codeLineInfo{
			content:     content,
			startOffset: lineInfo.StartOffset,
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

	pos := cb.SourcePosition()
	if !pos.IsValid() || pos.EndLine > len(file.Lines) || cb.Block == nil || cb.Block.CodeBlock == nil {
		return nil
	}
	attrs := cb.Block.CodeBlock

	// The full span of a fenced code block starts at its opening fence.
	opening := cb.SourceRange().StartOffset
	if attrs.FenceLength < 3 || opening+attrs.FenceLength > len(file.Content) {
		return nil
	}
	// The converted block must parse the same: a backtick fence cannot have
	// a backtick in its info string, and a line of code that starts with a
	// run of the new fence character could close the block early.
	if expectedChar == '`' && strings.ContainsRune(attrs.Info, '`') {
		return nil
	}
	if codeHoldsFenceRun(file, cb, expectedChar) {
		return nil
	}

	builder := fix.NewEditBuilder()
	builder.ReplaceRange(opening, opening+attrs.FenceLength, strings.Repeat(string(expectedChar), attrs.FenceLength))

	// The closing fence may be longer than the opening one: replace its
	// whole run.
	if attrs.Closed {
		start, end, ok := closingFenceRun(file, cb, pos.EndLine, attrs.FenceChar)
		if !ok {
			return nil
		}
		builder.ReplaceRange(start, end, strings.Repeat(string(expectedChar), end-start))
	}

	return builder
}

// closingFenceRun returns the byte range of the run of fence characters on
// the closing line of a fenced code block, after its container prefix and
// indentation.
func closingFenceRun(file *mdast.FileSnapshot, cb *mdast.Node, lineNum int, fenceChar byte) (int, int, bool) {
	line := file.Lines[lineNum-1]
	start := line.StartOffset + len(lint.ContainerPrefix(file, cb, lineNum))
	for start < line.NewlineStart && (file.Content[start] == ' ' || file.Content[start] == '\t') {
		start++
	}
	end := start
	for end < line.NewlineStart && file.Content[end] == fenceChar {
		end++
	}
	return start, end, end-start >= 3
}

// codeHoldsFenceRun reports whether a line of the code block's content
// starts, after its container prefix and indentation, with three or more of
// fenceChar.
func codeHoldsFenceRun(file *mdast.FileSnapshot, cb *mdast.Node, fenceChar byte) bool {
	pos := cb.SourcePosition()
	last := pos.EndLine
	if cb.Block.CodeBlock.Closed {
		last--
	}
	fence := []byte{fenceChar, fenceChar, fenceChar}
	for lineNum := pos.StartLine + 1; lineNum <= last; lineNum++ {
		content := lint.RawLineContent(file, lineNum)
		rest := bytes.TrimLeft(content[len(lint.ContainerPrefix(file, cb, lineNum)):], " \t")
		if bytes.HasPrefix(rest, fence) {
			return true
		}
	}
	return false
}
