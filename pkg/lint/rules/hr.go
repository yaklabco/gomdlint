package rules

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// styleConsistent is the configuration value for consistent style detection.
const styleConsistent = "consistent"

// minThematicBreakMarkers is the number of marker characters a thematic break needs.
const minThematicBreakMarkers = 3

// HRStyleRule checks for consistent horizontal rule style.
type HRStyleRule struct {
	lint.BaseRule
}

// NewHRStyleRule creates a new hr-style rule.
func NewHRStyleRule() *HRStyleRule {
	return &HRStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD035",
			"hr-style",
			"Horizontal rule style",
			[]string{"hr"},
			true,
		),
	}
}

// Apply checks for consistent horizontal rule style.
//
// Like markdownlint, it checks the parsed thematic breaks, including those in
// blockquotes and list items; setext underlines, front matter delimiters and
// lines in code or HTML blocks are not thematic breaks. A rule's style is its
// marker text without trailing whitespace.
//
// The fix rewrites only the marker characters, so the container prefix,
// indentation and trailing whitespace are kept. It is withheld when the
// expected style is not a thematic break, when it is a run of dashes directly
// under a non-blank line (which would make that line a setext heading), and
// when the line's container prefix holds the style's marker (a list item
// "- * * *" rewritten to "- ---" would become a rule and end the list).
func (r *HRStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := strings.TrimSpace(ctx.OptionString("style", styleConsistent))

	var diags []lint.Diagnostic
	checker := newFixChecker(ctx)
	var expectedStyle string

	if configStyle != styleConsistent {
		expectedStyle = configStyle
	}

	for _, node := range ctx.ThematicBreaks() {
		if ctx.Cancelled() {
			return checker.filter(diags), ctx.Ctx.Err()
		}

		rng := node.SourceRange()
		if rng.IsEmpty() || rng.EndOffset > len(ctx.File.Content) {
			continue
		}
		marker := bytes.TrimRight(ctx.File.Content[rng.StartOffset:rng.EndOffset], " \t")
		hrStyle := string(marker)

		// Set expected style from first HR if consistent mode.
		if expectedStyle == "" {
			expectedStyle = hrStyle
			continue
		}

		// Check for style mismatch.
		if hrStyle != expectedStyle {
			pos := rangePosition(ctx.File, mdast.SourceRange{
				StartOffset: rng.StartOffset,
				EndOffset:   rng.StartOffset + len(marker),
			})

			diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				fmt.Sprintf("Horizontal rule style %q does not match expected %q", hrStyle, expectedStyle)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %q for all horizontal rules", expectedStyle))

			if hrFixAllowed(ctx.File, node, pos.StartLine, expectedStyle) {
				builder := fix.NewEditBuilder()
				builder.ReplaceRange(rng.StartOffset, rng.StartOffset+len(marker), expectedStyle)
				diagBuilder = diagBuilder.WithFix(checker.offer(builder, fixIntent{}))
			}

			diags = append(diags, diagBuilder.Build())
		}
	}

	return checker.filter(diags), nil
}

// hrFixAllowed reports whether the thematic break node on line can be
// rewritten to style without changing the document structure.
func hrFixAllowed(file *mdast.FileSnapshot, node *mdast.Node, line int, style string) bool {
	if !isThematicBreakStyle(style) {
		return false
	}
	marker := strings.TrimLeft(style, " \t")[:1]
	if strings.Contains(string(lint.ContainerPrefix(file, node, line)), marker) {
		return false
	}
	// A run of dashes under a paragraph line is a setext heading underline.
	dashesOnly := strings.Trim(style, "-") == ""
	return !dashesOnly || line == 1 || lint.IsBlankLineInContainer(file, node, line-1)
}

// isThematicBreakStyle reports whether style is a valid thematic break: three
// or more of the same marker (-, * or _), optionally separated by spaces or tabs.
func isThematicBreakStyle(style string) bool {
	markers := strings.NewReplacer(" ", "", "\t", "").Replace(style)
	if len(markers) < minThematicBreakMarkers {
		return false
	}
	marker := markers[0]
	if marker != '-' && marker != '*' && marker != '_' {
		return false
	}
	return strings.Count(markers, string(marker)) == len(markers)
}
