package rules

import (
	"fmt"
	"strings"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
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

// Apply checks for hard tab characters on each line.
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

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
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

		// Find all tabs in the line.
		tabPositions := findTabPositions(lineContent)
		if len(tabPositions) == 0 {
			continue
		}

		line := ctx.File.Lines[lineNum-1]

		// Build fix: replace all tabs with spaces.
		builder := fix.NewEditBuilder()
		for _, tabPos := range tabPositions {
			offset := line.StartOffset + tabPos
			builder.ReplaceRange(offset, offset+1, strings.Repeat(" ", spacesPerTab))
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
			WithSuggestion(fmt.Sprintf("Replace tab with %d space(s)", spacesPerTab)).
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// codeBlockLineInfo stores information about a line inside a code block.
type codeBlockLineInfo struct {
	language string
}

// buildCodeBlockInfo returns a map of line numbers that are inside code blocks,
// along with the language of each code block.
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

		for lineNum := pos.StartLine; lineNum <= pos.EndLine; lineNum++ {
			info[lineNum] = codeBlockLineInfo{language: langName}
		}
	}

	return info
}

// findTabPositions returns the positions (0-indexed) of all tab characters in the content.
func findTabPositions(content []byte) []int {
	var positions []int
	for i, ch := range content {
		if ch == '\t' {
			positions = append(positions, i)
		}
	}
	return positions
}
