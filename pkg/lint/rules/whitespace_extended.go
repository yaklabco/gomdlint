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
// replaced by the spaces that reach the same tab stop. Other tabs, in text
// and code, are replaced by spaces_per_tab spaces, as markdownlint does.
func (r *HardTabsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	includeCodeBlocks := ctx.OptionBool("code_blocks", true)
	spacesPerTab := ctx.OptionInt("spaces_per_tab", 1)
	if spacesPerTab < 1 {
		spacesPerTab = 1
	}

	// Get ignore_code_languages option.
	ignoreCodeLanguages := make(map[string]bool)
	if langs := ctx.Option("ignore_code_languages", nil); langs != nil {
		if langSlice, ok := langs.([]any); ok {
			for _, l := range langSlice {
				if s, ok := l.(string); ok {
					ignoreCodeLanguages[strings.ToLower(s)] = true
				}
			}
		}
	}

	// Build map of code block lines and their languages.
	codeBlockInfo := buildCodeBlockInfo(ctx.Root)
	codeBlockByLine := make(map[int]*mdast.Node)
	for _, block := range ctx.CodeBlocks() {
		for lineNum := range lint.CodeContentLines([]*mdast.Node{block}) {
			codeBlockByLine[lineNum] = block
		}
	}

	// Like markdownlint, excluding code blocks also excludes code spans.
	var codeSpans []mdast.SourceRange
	if !includeCodeBlocks {
		for _, span := range ctx.CodeSpans() {
			codeSpans = append(codeSpans, span.SourceRange())
		}
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		lineContent := lint.LineContent(ctx.File, lineNum)

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

		line := ctx.File.Lines[lineNum-1]

		// Find all tabs in the line, outside excluded code spans.
		tabPositions := findTabPositions(lineContent, line.StartOffset, codeSpans)
		if len(tabPositions) == 0 {
			continue
		}

		// Build fix: replace all tabs with spaces.
		structureEnd := tabStructureEnd(ctx.File, lineNum, lineContent, codeBlockByLine[lineNum])
		builder := fix.NewEditBuilder()
		for _, tabPos := range tabPositions {
			offset := line.StartOffset + tabPos
			spaces := spacesPerTab
			if tabPos < structureEnd {
				spaces = tabWidth - visualColumn(lineContent[:tabPos])%tabWidth
			}
			builder.ReplaceRange(offset, offset+1, strings.Repeat(" ", spaces))
		}

		// Create a single diagnostic for the line at the first tab position.
		firstTabPos := tabPositions[0]
		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: firstTabPos + 1,
			EndLine:     lineNum,
			EndColumn:   firstTabPos + 2,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "Hard tab character found").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Replace tabs with spaces").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// tabStructureEnd returns the byte index where the part of a line that
// CommonMark measures in columns ends. On a code line it is the container
// prefix, followed for indented code by the four columns of code
// indentation. On other lines it is the leading run of whitespace,
// blockquote markers and list markers.
func tabStructureEnd(file *mdast.FileSnapshot, lineNum int, content []byte, codeBlock *mdast.Node) int {
	if codeBlock != nil {
		end := len(lint.ContainerPrefix(file, codeBlock, lineNum))
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

// findTabPositions returns the positions (0-indexed) of the tab characters in
// a line that starts at lineStart, skipping tabs inside the excluded ranges.
func findTabPositions(content []byte, lineStart int, excluded []mdast.SourceRange) []int {
	var positions []int
	for idx, ch := range content {
		if ch != '\t' {
			continue
		}
		offset := lineStart + idx
		inside := false
		for _, rng := range excluded {
			if rng.StartOffset <= offset && offset < rng.EndOffset {
				inside = true
				break
			}
		}
		if !inside {
			positions = append(positions, idx)
		}
	}
	return positions
}
