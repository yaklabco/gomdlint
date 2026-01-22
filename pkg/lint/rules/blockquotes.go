package rules

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// NoMultipleSpaceBlockquoteRule checks for multiple spaces after blockquote symbol.
type NoMultipleSpaceBlockquoteRule struct {
	lint.BaseRule
}

// NewNoMultipleSpaceBlockquoteRule creates a new no-multiple-space-blockquote rule.
func NewNoMultipleSpaceBlockquoteRule() *NoMultipleSpaceBlockquoteRule {
	return &NoMultipleSpaceBlockquoteRule{
		BaseRule: lint.NewBaseRule(
			"MD027",
			"no-multiple-space-blockquote",
			"Multiple spaces after blockquote symbol",
			[]string{"blockquote", "indentation", "whitespace"},
			true,
		),
	}
}

// blockquoteMultiSpacePattern matches blockquote lines with multiple spaces after >.
var blockquoteMultiSpacePattern = regexp.MustCompile(`^(>+)([ ]{2,})(\S)`)

// blockquoteListPattern matches list items in blockquotes.
var blockquoteListPattern = regexp.MustCompile(`^(>+)\s*([-*+]|\d+[.)]) `)

// Apply checks for multiple spaces after blockquote symbol.
func (r *NoMultipleSpaceBlockquoteRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	includeListItems := ctx.OptionBool("list_items", true)

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		lineContent := lint.LineContent(ctx.File, lineNum)

		// Check for multiple spaces after >
		match := blockquoteMultiSpacePattern.FindSubmatch(lineContent)
		if match == nil {
			continue
		}

		// Skip list items if configured.
		if !includeListItems && blockquoteListPattern.Match(lineContent) {
			continue
		}

		prefix := match[1]
		spaces := match[2]
		line := ctx.File.Lines[lineNum-1]

		// Build fix: replace multiple spaces with single space.
		builder := fix.NewEditBuilder()
		spaceStart := line.StartOffset + len(prefix)
		spaceEnd := spaceStart + len(spaces)
		builder.ReplaceRange(spaceStart, spaceEnd, " ")

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: len(prefix) + 1,
			EndLine:     lineNum,
			EndColumn:   len(prefix) + len(spaces) + 1,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Multiple spaces (%d) after blockquote symbol", len(spaces))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Use a single space after the blockquote symbol").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// NoBlanksBlockquoteRule checks for blank lines inside blockquotes.
type NoBlanksBlockquoteRule struct {
	lint.BaseRule
}

// NewNoBlanksBlockquoteRule creates a new no-blanks-blockquote rule.
func NewNoBlanksBlockquoteRule() *NoBlanksBlockquoteRule {
	return &NoBlanksBlockquoteRule{
		BaseRule: lint.NewBaseRule(
			"MD028",
			"no-blanks-blockquote",
			"Blank line inside blockquote",
			[]string{"blockquote", "whitespace"},
			false, // Not auto-fixable - requires human decision.
		),
	}
}

// Apply checks for blank lines separating blockquotes.
func (r *NoBlanksBlockquoteRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || len(ctx.File.Lines) < 3 {
		return nil, nil
	}

	var diags []lint.Diagnostic
	var inBlockquote bool
	var lastBlockquoteLine int

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		isBlockquoteLine := len(lineContent) > 0 && lineContent[0] == '>'
		isBlankLine := len(bytes.TrimSpace(lineContent)) == 0

		if isBlockquoteLine {
			// Check if we're returning to a blockquote after a gap.
			if inBlockquote && lastBlockquoteLine > 0 && lastBlockquoteLine < lineNum-1 {
				// There was a gap - check if it was just blank lines.
				allBlank := true
				for checkLine := lastBlockquoteLine + 1; checkLine < lineNum; checkLine++ {
					checkContent := lint.LineContent(ctx.File, checkLine)
					if len(bytes.TrimSpace(checkContent)) > 0 {
						allBlank = false
						break
					}
				}

				if allBlank {
					pos := mdast.SourcePosition{
						StartLine:   lastBlockquoteLine + 1,
						StartColumn: 1,
						EndLine:     lineNum - 1,
						EndColumn:   1,
					}

					diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
						"Blank line inside blockquote separates it into multiple blockquotes").
						WithSeverity(config.SeverityWarning).
						WithSuggestion("Add text between blockquotes or use '>' on blank lines").
						Build()
					diags = append(diags, diag)
				}
			}

			inBlockquote = true
			lastBlockquoteLine = lineNum
		} else if !isBlankLine {
			// Non-blank, non-blockquote line resets the tracking.
			inBlockquote = false
			lastBlockquoteLine = 0
		}
		// Blank lines don't reset tracking - we want to find gaps.
	}

	return diags, nil
}
