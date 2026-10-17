package rules

import (
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// HardTabsRule checks for hard tab characters in the document.
type HardTabsRule struct {
	lint.BaseRule
}

// NewHardTabsRule creates a new hard tabs rule.
func NewHardTabsRule() *HardTabsRule {
	return &HardTabsRule{
		BaseRule: lint.NewBaseRule(
			"MD010",
			"no-hard-tabs",
			"Hard tabs should not be used",
			[]string{"hard_tab", "whitespace"},
			true,
		),
	}
}

// tabWidth is the CommonMark tab stop width.
const tabWidth = 4

// Apply checks for hard tab characters on each line.
//
// The fix keeps the document structure: a tab in the part of a line that
// CommonMark measures in columns (indentation, blockquote markers, list
// markers and the space after them, and the indentation of indented code) is
// replaced by the spaces that reach the same tab stop. Other tabs in text are
// replaced by spaces_per_tab spaces, as markdownlint does. Tabs in code, the
// text of code blocks and code spans, are reported like markdownlint reports
// them but never fixed, because they can be significant, as in Makefiles, Go
// or tab-separated data. The ignore_code_languages option names fenced code
// languages, matched on the first word of the info string in any case, whose
// code is not checked at all.
func (r *HardTabsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	includeCodeBlocks := ctx.OptionBool("code_blocks", true)
	spacesPerTab := ctx.OptionInt("spaces_per_tab", 1)
	if spacesPerTab < 1 {
		spacesPerTab = 1
	}

	ignoreCodeLanguages := make(map[string]bool)
	for _, language := range ctx.OptionStringSlice("ignore_code_languages", nil) {
		ignoreCodeLanguages[strings.ToLower(language)] = true
	}

	// Build map of code block lines and their languages.
	codeBlockInfo := buildCodeBlockInfo(ctx.Root)
	codeBlockByLine := make(map[int]*mdast.Node)
	for _, block := range ctx.CodeBlocks() {
		for lineNum := range lint.CodeContentLines([]*mdast.Node{block}) {
			codeBlockByLine[lineNum] = block
		}
	}

	var codeSpans []mdast.SourceRange
	for _, span := range ctx.CodeSpans() {
		codeSpans = append(codeSpans, span.SourceRange())
	}
	// Like markdownlint, excluding code blocks also excludes code spans.
	var excludedSpans []mdast.SourceRange
	if !includeCodeBlocks {
		excludedSpans = codeSpans
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		lineContent := lint.LineText(ctx.File, lineNum)
		textOffset := lint.LineTextOffset(ctx.File, lineNum)

		// Check if line is in a code block.
		if info, inCodeBlock := codeBlockInfo[lineNum]; inCodeBlock {
			// Skip if not including code blocks.
			if !includeCodeBlocks {
				continue
			}
			// Skip if the language is in the ignore list.
			if info.language != "" && ignoreCodeLanguages[strings.ToLower(info.language)] {
				continue
			}
		}

		textStart := ctx.File.Lines[lineNum-1].StartOffset + textOffset

		// Find all tabs in the line, outside excluded code spans.
		tabPositions := findTabPositions(lineContent, textStart, excludedSpans)
		if len(tabPositions) == 0 {
			continue
		}

		// Build the fix from the tabs outside code text.
		codeLine := codeBlockByLine[lineNum] != nil
		structureEnd := tabStructureEnd(ctx.File, lineNum, lineContent, codeBlockByLine[lineNum])
		builder := fix.NewEditBuilder()
		for _, tabPos := range tabPositions {
			offset := textStart + tabPos
			structural := tabPos < structureEnd
			if (codeLine && !structural) || inRanges(offset, codeSpans) {
				continue
			}
			spaces := spacesPerTab
			if structural {
				spaces = tabWidth - visualColumn(lineContent[:tabPos])%tabWidth
			}
			builder.ReplaceRange(offset, offset+1, strings.Repeat(" ", spaces))
		}

		// Create a single diagnostic for the line at the first tab position.
		firstTabPos := tabPositions[0]
		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: textOffset + firstTabPos + 1,
			EndLine:     lineNum,
			EndColumn:   textOffset + firstTabPos + 2,
		}

		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "Hard tab character found").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Replace tabs with spaces")
		if len(builder.Edits) > 0 {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return keepSafeFixes(ctx, diags), nil
}

// tabStructureEnd returns the byte index in the text of a line, after a
// byte order mark, where the part that CommonMark measures in columns ends.
// On a code line it is the container prefix, followed for indented code by
// the four columns of code indentation. On other lines it is the leading run
// of whitespace, blockquote markers and list markers.
func tabStructureEnd(file *mdast.FileSnapshot, lineNum int, content []byte, codeBlock *mdast.Node) int {
	if codeBlock != nil {
		end := len(lint.ContainerPrefix(file, codeBlock, lineNum)) - lint.LineTextOffset(file, lineNum)
		if !lint.IsIndentedCodeBlock(codeBlock) {
			return end
		}
		base := visualColumn(content[:end])
		for end < len(content) && (content[end] == ' ' || content[end] == '\t') && visualColumn(content[:end])-base < tabWidth {
			end++
		}
		return end
	}
	pos := 0
	for pos < len(content) {
		switch {
		case content[pos] == ' ' || content[pos] == '\t' || content[pos] == '>':
			pos++
		case isLeadingListMarker(content, pos):
			for pos < len(content) && content[pos] != ' ' && content[pos] != '\t' {
				pos++
			}
		default:
			return pos
		}
	}
	return pos
}

// isLeadingListMarker reports whether a list marker followed by whitespace
// starts at pos.
func isLeadingListMarker(content []byte, pos int) bool {
	end := pos
	switch {
	case content[pos] == '-' || content[pos] == '+' || content[pos] == '*':
		end++
	case isASCIIDigit(content[pos]):
		for end < len(content) && isASCIIDigit(content[end]) {
			end++
		}
		if end == len(content) || (content[end] != '.' && content[end] != ')') {
			return false
		}
		end++
	default:
		return false
	}
	return end < len(content) && (content[end] == ' ' || content[end] == '\t')
}

// visualColumn returns the column after text, expanding tabs to tab stops.
func visualColumn(text []byte) int {
	col := 0
	for _, char := range text {
		if char == '\t' {
			col += tabWidth - col%tabWidth
			continue
		}
		col++
	}
	return col
}

// codeBlockLineInfo stores information about a line inside a code block.
type codeBlockLineInfo struct {
	language string
}

// buildCodeBlockInfo returns a map of the code lines of code blocks, along
// with the language of each code block. The fence lines of fenced code blocks
// are not code, so they are not included.
func buildCodeBlockInfo(root *mdast.Node) map[int]codeBlockLineInfo {
	info := make(map[int]codeBlockLineInfo)
	if root == nil {
		return info
	}

	codeBlocks := lint.CodeBlocks(root)
	for _, codeBlock := range codeBlocks {
		pos := codeBlock.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		lang := lint.CodeBlockInfo(codeBlock)
		// Extract just the language part (first word).
		langParts := strings.Fields(lang)
		langName := ""
		if len(langParts) > 0 {
			langName = langParts[0]
		}

		for lineNum := range lint.CodeContentLines([]*mdast.Node{codeBlock}) {
			info[lineNum] = codeBlockLineInfo{language: langName}
		}
	}

	return info
}

// inRanges reports whether an offset lies in one of the ranges.
func inRanges(offset int, ranges []mdast.SourceRange) bool {
	for _, rng := range ranges {
		if rng.StartOffset <= offset && offset < rng.EndOffset {
			return true
		}
	}
	return false
}

// findTabPositions returns the positions (0-indexed) of the tab characters in
// a line that starts at lineStart, skipping tabs inside the excluded ranges.
func findTabPositions(content []byte, lineStart int, excluded []mdast.SourceRange) []int {
	var positions []int
	for idx, ch := range content {
		if ch == '\t' && !inRanges(lineStart+idx, excluded) {
			positions = append(positions, idx)
		}
	}
	return positions
}
