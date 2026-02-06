package rules

import (
	"fmt"
	"strconv"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// BulletStyle represents the style of unordered list bullets.
type BulletStyle string

const (
	// BulletDash uses "-" as the bullet marker.
	BulletDash BulletStyle = "dash"
	// BulletPlus uses "+" as the bullet marker.
	BulletPlus BulletStyle = "plus"
	// BulletAsterisk uses "*" as the bullet marker.
	BulletAsterisk BulletStyle = "asterisk"
	// BulletConsistent uses whatever style is first encountered.
	BulletConsistent BulletStyle = "consistent"
)

// getBulletMarker returns the character representation for a bullet style.
func getBulletMarker(style BulletStyle) string {
	switch style {
	case BulletDash:
		return "-"
	case BulletPlus:
		return "+"
	case BulletAsterisk:
		return "*"
	default:
		return ""
	}
}

// getBulletStyle returns the bullet style for a marker character.
func getBulletStyle(marker string) (BulletStyle, bool) {
	switch marker {
	case "-":
		return BulletDash, true
	case "+":
		return BulletPlus, true
	case "*":
		return BulletAsterisk, true
	default:
		return "", false
	}
}

// UnorderedListStyleRule enforces consistent bullet markers in unordered lists.
type UnorderedListStyleRule struct {
	lint.BaseRule
}

// NewUnorderedListStyleRule creates a new unordered list style rule.
func NewUnorderedListStyleRule() *UnorderedListStyleRule {
	return &UnorderedListStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD004",
			"unordered-list-style",
			"Unordered list style should be consistent",
			[]string{"lists", "style"},
			true,
		),
	}
}

// Apply checks that all unordered lists use consistent bullet markers.
func (r *UnorderedListStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := BulletStyle(ctx.OptionString("style", string(BulletDash)))

	// Determine effective style.
	effectiveStyle := configStyle
	effectiveMarker := getBulletMarker(effectiveStyle)

	if configStyle == BulletConsistent {
		effectiveStyle = "" // Will be set from first list.
		effectiveMarker = ""
	}

	lists := ctx.Lists()
	var diags []lint.Diagnostic

	for _, list := range lists {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip ordered lists.
		if lint.IsOrderedList(list) {
			continue
		}

		marker := lint.ListBulletMarker(list)
		if marker == "" {
			continue
		}

		// Set consistent style from first list.
		if effectiveStyle == "" {
			if style, ok := getBulletStyle(marker); ok {
				effectiveStyle = style
				effectiveMarker = marker
			}
			continue
		}

		// Check for style mismatch.
		if marker != effectiveMarker {
			items := lint.ListItems(list)
			for _, item := range items {
				diag := r.createBulletDiagnostic(ctx, item, marker, effectiveMarker)
				diags = append(diags, diag)
			}
		}
	}

	return diags, nil
}

func (r *UnorderedListStyleRule) createBulletDiagnostic(
	ctx *lint.RuleContext,
	item *mdast.Node,
	actual, expected string,
) lint.Diagnostic {
	msg := fmt.Sprintf("Unordered list bullet '%s' does not match expected '%s'", actual, expected)

	// Find the bullet marker position and create a fix.
	builder := r.buildBulletFix(ctx.File, item, expected)

	diagBuilder := lint.NewDiagnostic(r.ID(), item, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use '%s' as the bullet marker", expected))

	if builder != nil {
		diagBuilder = diagBuilder.WithFix(builder)
	}

	return diagBuilder.Build()
}

func (r *UnorderedListStyleRule) buildBulletFix(
	file *mdast.FileSnapshot,
	item *mdast.Node,
	expectedMarker string,
) *fix.EditBuilder {
	if file == nil || item == nil {
		return nil
	}

	pos := item.SourcePosition()
	if pos.StartLine < 1 || pos.StartLine > len(file.Lines) {
		return nil
	}

	// Get the line content and find the bullet marker.
	lineContent := lint.LineContent(file, pos.StartLine)
	line := file.Lines[pos.StartLine-1]

	// Find the first bullet character (-, +, or *).
	for i, ch := range lineContent {
		if ch == '-' || ch == '+' || ch == '*' {
			builder := fix.NewEditBuilder()
			offset := line.StartOffset + i
			builder.ReplaceRange(offset, offset+1, expectedMarker)
			return builder
		}
	}

	return nil
}

// OrderedListIncrementRule enforces sequential numbering in ordered lists.
type OrderedListIncrementRule struct {
	lint.BaseRule
}

// NewOrderedListIncrementRule creates a new ordered list increment rule.
func NewOrderedListIncrementRule() *OrderedListIncrementRule {
	return &OrderedListIncrementRule{
		BaseRule: lint.NewBaseRule(
			"MD029",
			"ol-prefix",
			"Ordered list item prefix",
			[]string{"ol"},
			true,
		),
	}
}

// Apply checks that ordered lists have sequential numbering.
func (r *OrderedListIncrementRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	allowRenumbering := ctx.OptionBool("allow_renumbering", true)

	lists := ctx.Lists()
	var diags []lint.Diagnostic

	for _, list := range lists {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip unordered lists.
		if !lint.IsOrderedList(list) {
			continue
		}

		startNumber := lint.ListStartNumber(list)
		if startNumber == 0 {
			startNumber = 1
		}
		delimiter := lint.ListDelimiter(list)
		if delimiter == "" {
			delimiter = "."
		}

		items := lint.ListItems(list)
		expectedNum := startNumber

		for _, item := range items {
			actualNum := extractListItemNumber(ctx.File, item)

			if actualNum != expectedNum {
				diag := r.createNumberDiagnostic(ctx, item, actualNum, expectedNum, delimiter, allowRenumbering)
				diags = append(diags, diag)
			}

			expectedNum++
		}
	}

	return diags, nil
}

func (r *OrderedListIncrementRule) createNumberDiagnostic(
	ctx *lint.RuleContext,
	item *mdast.Node,
	actual, expected int,
	delimiter string,
	allowRenumbering bool,
) lint.Diagnostic {
	msg := fmt.Sprintf("Ordered list item numbered %d should be %d", actual, expected)

	diagBuilder := lint.NewDiagnostic(r.ID(), item, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use %d%s instead", expected, delimiter))

	if allowRenumbering {
		builder := r.buildNumberFix(ctx.File, item, expected, delimiter)
		if builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
	}

	return diagBuilder.Build()
}

func (r *OrderedListIncrementRule) buildNumberFix(
	file *mdast.FileSnapshot,
	item *mdast.Node,
	expectedNum int,
	delimiter string,
) *fix.EditBuilder {
	if file == nil || item == nil {
		return nil
	}

	pos := item.SourcePosition()
	if pos.StartLine < 1 || pos.StartLine > len(file.Lines) {
		return nil
	}

	lineContent := lint.LineContent(file, pos.StartLine)
	line := file.Lines[pos.StartLine-1]

	// Find the number and delimiter in the line.
	numStart := -1
	numEnd := -1
	delimEnd := -1

	for idx, ch := range lineContent {
		// Skip leading whitespace.
		if ch == ' ' || ch == '\t' {
			continue
		}

		// Look for digits.
		switch {
		case ch >= '0' && ch <= '9':
			if numStart < 0 {
				numStart = idx
			}
			numEnd = idx + 1
		case numEnd > 0:
			// After digits, expect delimiter.
			if ch == '.' || ch == ')' {
				delimEnd = idx + 1
			}
		}

		// Determine if we should stop parsing.
		foundDelimiter := delimEnd > 0
		isWhitespace := ch == ' ' || ch == '\t'
		isDigit := ch >= '0' && ch <= '9'
		isDelimiterChar := ch == '.' || ch == ')'
		hitNonDigitBeforeNumber := numEnd == 0 && !isWhitespace
		hitInvalidCharAfterNumber := numEnd > 0 && !isDelimiterChar && !isDigit

		if foundDelimiter || hitNonDigitBeforeNumber || hitInvalidCharAfterNumber {
			break
		}
	}

	if numStart < 0 || delimEnd < 0 {
		return nil
	}

	builder := fix.NewEditBuilder()
	offset := line.StartOffset + numStart
	endOffset := line.StartOffset + delimEnd
	newText := fmt.Sprintf("%d%s", expectedNum, delimiter)
	builder.ReplaceRange(offset, endOffset, newText)

	return builder
}

// extractListItemNumber extracts the number from an ordered list item.
func extractListItemNumber(file *mdast.FileSnapshot, item *mdast.Node) int {
	if file == nil || item == nil {
		return 0
	}

	pos := item.SourcePosition()
	if pos.StartLine < 1 || pos.StartLine > len(file.Lines) {
		return 0
	}

	lineContent := lint.LineContent(file, pos.StartLine)

	// Parse the number from the beginning of the line (after whitespace).
	foundDigit := false
	const typicalListNumberLen = 8
	numBuilder := make([]byte, 0, typicalListNumberLen)

	for _, ch := range lineContent {
		if ch == ' ' || ch == '\t' {
			if foundDigit {
				break
			}
			continue
		}

		if ch < '0' || ch > '9' {
			break
		}

		numBuilder = append(numBuilder, ch)
		foundDigit = true
	}
	numStr := string(numBuilder)

	if numStr == "" {
		return 0
	}

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}

	return num
}
