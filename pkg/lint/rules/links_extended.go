package rules

import (
	"regexp"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// NoBareURLsRule checks for bare URLs without angle brackets.
type NoBareURLsRule struct {
	lint.BaseRule
}

// NewNoBareURLsRule creates a new no-bare-urls rule.
func NewNoBareURLsRule() *NoBareURLsRule {
	return &NoBareURLsRule{
		BaseRule: lint.NewBaseRule(
			"MD034",
			"no-bare-urls",
			"Bare URL used",
			[]string{"links", "url"},
			true,
		),
	}
}

// bareURLPattern matches bare URLs and emails without consuming boundary characters.
// Boundary validation (angle brackets, parens, brackets) is done in code after matching.
var bareURLPattern = regexp.MustCompile(`https?://[^\s<>\[\]()]+|[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

var emailCheckPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// Apply checks for bare URLs without angle brackets.
func (r *NoBareURLsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip lines in code blocks.
		if ctx.IsLineInCodeBlock(lineNum) {
			continue
		}

		lineContent := lint.LineContent(ctx.File, lineNum)

		// Skip lines that are already autolinks.
		if isAutolinkLine(lineContent) {
			continue
		}

		matches := bareURLPattern.FindAllIndex(lineContent, -1)

		for _, match := range matches {
			urlStart, urlEnd := match[0], match[1]
			url := string(lineContent[urlStart:urlEnd])

			// Skip if the URL is inside a code span.
			if r.isInsideCodeSpan(lineContent, urlStart) {
				continue
			}

			// Skip if preceded by <, (, or [ (autolink, markdown link, or reference).
			if urlStart > 0 {
				prev := lineContent[urlStart-1]
				if prev == '<' || prev == '(' || prev == '[' {
					continue
				}
			}

			// Skip if followed by >, ), or ] (closing autolink, link, or reference).
			if urlEnd < len(lineContent) {
				next := lineContent[urlEnd]
				if next == '>' || next == ')' || next == ']' {
					continue
				}
			}

			line := ctx.File.Lines[lineNum-1]

			// Build fix: wrap in angle brackets.
			builder := fix.NewEditBuilder()
			builder.ReplaceRange(line.StartOffset+urlStart, line.StartOffset+urlEnd, "<"+url+">")

			diagPos := mdast.SourcePosition{
				StartLine:   lineNum,
				StartColumn: urlStart + 1,
				EndLine:     lineNum,
				EndColumn:   urlEnd + 1,
			}

			var msg string
			if isEmail(url) {
				msg = "Bare email address used"
			} else {
				msg = "Bare URL used"
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Wrap the URL/email in angle brackets").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

func (r *NoBareURLsRule) isInsideCodeSpan(line []byte, pos int) bool {
	// Count backticks before the position.
	backticks := 0
	for i := 0; i < pos && i < len(line); i++ {
		if line[i] == '`' {
			backticks++
		}
	}
	// Odd number of backticks means we're inside a code span.
	return backticks%2 == 1
}

func isAutolinkLine(line []byte) bool {
	// Simple check for <url> pattern.
	return len(line) >= 2 && line[0] == '<' && line[len(line)-1] == '>'
}

func isEmail(s string) bool {
	return emailCheckPattern.MatchString(s)
}
