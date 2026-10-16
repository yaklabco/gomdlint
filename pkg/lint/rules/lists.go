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
	// BulletSublist requires each nesting level to use one style that differs
	// from the style of its parent level.
	BulletSublist BulletStyle = "sublist"
)

// differentBulletStyle returns the style markdownlint picks for a sublist whose
// first marker repeats its parent's style.
func differentBulletStyle(style BulletStyle) BulletStyle {
	switch style {
	case BulletDash:
		return BulletPlus
	case BulletPlus:
		return BulletAsterisk
	default:
		return BulletDash
	}
}

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

// bulletMismatch is an unordered list item whose bullet differs from the
// expected one.
type bulletMismatch struct {
	list     *mdast.Node
	item     *mdast.Node
	marker   listItemMarker
	found    bool
	actual   string
	expected string
}

// Apply checks that all unordered lists use consistent bullet markers.
//
// The default style is consistent: every list uses the bullet of the first
// list in the document. Like markdownlint, an unknown style falls back to dash
// and the sublist style gives each nesting level one bullet that differs from
// its parent level's.
func (r *UnorderedListStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	style := BulletStyle(ctx.OptionString("style", string(BulletConsistent)))
	switch style {
	case BulletDash, BulletPlus, BulletAsterisk, BulletConsistent, BulletSublist:
	default:
		style = BulletDash
	}
	expected := style
	var nestingStyles []BulletStyle

	var mismatches []bulletMismatch
	for _, list := range ctx.Lists() {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		if lint.IsOrderedList(list) {
			continue
		}
		listStyle, ok := getBulletStyle(lint.ListBulletMarker(list))
		if !ok {
			continue
		}
		nesting := listNesting(list)
		for _, item := range lint.ListItems(list) {
			marker, found := findListItemMarker(ctx.File, item)
			itemStyle := listStyle
			if found {
				if markerStyle, known := getBulletStyle(string(marker.Bullet)); known {
					itemStyle = markerStyle
				}
			}
			if expected == BulletConsistent {
				expected = itemStyle
			}
			want := expected
			if style == BulletSublist {
				want = sublistStyle(&nestingStyles, nesting, itemStyle)
			}
			if itemStyle == want {
				continue
			}
			mismatches = append(mismatches, bulletMismatch{
				list:     list,
				item:     item,
				marker:   marker,
				found:    found,
				actual:   getBulletMarker(itemStyle),
				expected: getBulletMarker(want),
			})
		}
	}

	return r.bulletDiagnostics(ctx, mismatches), nil
}

// sublistStyle returns the style expected at a nesting level, fixing the
// level's style from its first item the first time the level is seen.
func sublistStyle(nestingStyles *[]BulletStyle, nesting int, itemStyle BulletStyle) BulletStyle {
	for len(*nestingStyles) <= nesting {
		*nestingStyles = append(*nestingStyles, "")
	}
	styles := *nestingStyles
	if styles[nesting] == "" {
		styles[nesting] = itemStyle
		if nesting > 0 && styles[nesting-1] == itemStyle {
			styles[nesting] = differentBulletStyle(itemStyle)
		}
	}
	return styles[nesting]
}

// adjacentToBulletList reports whether a list directly follows or precedes
// another bullet list, with nothing but blank lines between them.
func adjacentToBulletList(list *mdast.Node) bool {
	for _, sibling := range []*mdast.Node{list.Prev, list.Next} {
		if sibling != nil && sibling.Kind == mdast.NodeList && !lint.IsOrderedList(sibling) {
			return true
		}
	}
	return false
}

// listNesting counts the lists, ordered or unordered, that contain a list.
func listNesting(list *mdast.Node) int {
	nesting := 0
	for parent := list.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeList {
			nesting++
		}
	}
	return nesting
}

// bulletDiagnostics reports each mismatch. A mismatch is fixed by replacing
// its bullet, unless that would change the document structure: when the list
// sits next to another bullet list, the differing bullet is what keeps the two
// lists apart, and when the replacements would turn the line into a thematic
// break, the list items would disappear.
func (r *UnorderedListStyleRule) bulletDiagnostics(ctx *lint.RuleContext, mismatches []bulletMismatch) []lint.Diagnostic {
	replacements := make(map[int]map[int]byte)
	for _, mismatch := range mismatches {
		if !mismatch.found || adjacentToBulletList(mismatch.list) {
			continue
		}
		if replacements[mismatch.marker.Line] == nil {
			replacements[mismatch.marker.Line] = make(map[int]byte)
		}
		replacements[mismatch.marker.Line][mismatch.marker.Start] = mismatch.expected[0]
	}
	unsafeLines := make(map[int]bool)
	for lineNum, lineReplacements := range replacements {
		unsafeLines[lineNum] = replacementMakesThematicBreak(ctx.File, lineNum, lineReplacements)
	}

	diags := make([]lint.Diagnostic, 0, len(mismatches))
	for _, mismatch := range mismatches {
		msg := fmt.Sprintf("Unordered list bullet '%s' does not match expected '%s'", mismatch.actual, mismatch.expected)
		var diagBuilder *lint.DiagnosticBuilder
		switch {
		case mismatch.found:
			column := mismatch.marker.Column() + 1
			pos := mdast.SourcePosition{
				StartLine: mismatch.marker.Line, StartColumn: column,
				EndLine: mismatch.marker.Line, EndColumn: column + 1,
			}
			diagBuilder = lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg)
			if !unsafeLines[mismatch.marker.Line] && !adjacentToBulletList(mismatch.list) {
				builder := fix.NewEditBuilder()
				builder.ReplaceRange(mismatch.marker.Start, mismatch.marker.End, mismatch.expected)
				diagBuilder = diagBuilder.WithFix(builder)
			}
		case mismatch.item.SourcePosition().IsValid():
			diagBuilder = lint.NewDiagnostic(r.ID(), mismatch.item, msg)
		default:
			continue
		}
		diags = append(diags, diagBuilder.
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Use '%s' as the bullet marker", mismatch.expected)).
			Build())
	}
	return diags
}

// replacementMakesThematicBreak reports whether replacing bullets on a line
// would leave a run of three or more identical bullets and nothing else after
// the first list marker, which CommonMark parses as a thematic break.
func replacementMakesThematicBreak(file *mdast.FileSnapshot, lineNum int, replacements map[int]byte) bool {
	line := file.Lines[lineNum-1]
	first := line.NewlineStart
	for offset := range replacements {
		first = min(first, offset)
	}
	text := make([]byte, 0, line.NewlineStart-first)
	for offset := first; offset < line.NewlineStart; offset++ {
		char := file.Content[offset]
		if replacement, ok := replacements[offset]; ok {
			char = replacement
		}
		text = append(text, char)
	}
	// Bullets before the first replaced one on the same line belong to the
	// same run of markers, so include them.
	for offset := first - 1; offset >= line.StartOffset; offset-- {
		char := file.Content[offset]
		if char != ' ' && char != '\t' && char != '-' && char != '*' && char != '+' {
			break
		}
		text = append([]byte{char}, text...)
	}
	return isThematicBreakText(text)
}

// isThematicBreakText reports whether text holds three or more of the same
// thematic break character and otherwise only spaces and tabs.
func isThematicBreakText(text []byte) bool {
	var breakChar byte
	count := 0
	for _, char := range text {
		switch char {
		case ' ', '\t':
		case '-', '*', '_':
			if breakChar != 0 && char != breakChar {
				return false
			}
			breakChar = char
			count++
		default:
			return false
		}
	}
	return count >= 3
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
		expectedNum, incrementing := orderedListPolicy(ctx, items, startNumber)

		for _, item := range items {
			if ctx.OptionString("style", "") != "" && item.Parent != list {
				continue
			}
			actualNum := extractListItemNumber(ctx.File, item)

			if actualNum != expectedNum {
				diag := r.createNumberDiagnostic(ctx, item, actualNum, expectedNum, delimiter, allowRenumbering)
				diags = append(diags, diag)
			}

			if incrementing {
				expectedNum++
			}
		}
	}

	return diags, nil
}

func orderedListPolicy(ctx *lint.RuleContext, items []*mdast.Node, startNumber int) (int, bool) {
	style := ctx.OptionString("style", "")
	switch style {
	case "":
		return startNumber, true
	case "one":
		return 1, false
	case "zero":
		return 0, false
	}
	start := 1
	if len(items) > 1 && extractListItemNumber(ctx.File, items[0]) == 0 {
		start = 0
	}
	if style == "ordered" {
		return start, true
	}
	incrementing := len(items) > 1 && (start == 0 || extractListItemNumber(ctx.File, items[1]) != 1)
	return start, incrementing
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

	lineContent := lint.LineText(file, pos.StartLine)

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
