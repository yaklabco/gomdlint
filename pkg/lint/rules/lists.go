package rules

import (
	"fmt"
	"strconv"
	"strings"

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

// orderedListStyles maps the MD029 styles to markdownlint's example text.
var orderedListStyles = map[string]string{
	"one":     "1/1/1",
	"ordered": "1/2/3",
	"zero":    "0/0/0",
}

// orderedItem is an ordered list item and its located marker.
type orderedItem struct {
	item   *mdast.Node
	marker listItemMarker
	found  bool
}

// numberMismatch is an ordered list item whose number differs from the
// expected one, with the text that would replace its number.
type numberMismatch struct {
	orderedItem
	expected int
	text     string
}

// Apply checks ordered list item prefixes.
//
// The default style is markdownlint's one_or_ordered: a list is valid when all
// its items are numbered 1, or when it counts up from 0 or 1. The styles one,
// ordered and zero force one pattern. Fixes replace only the digits, keep the
// delimiter, and keep right-aligned or zero-padded numbers aligned.
func (r *OrderedListIncrementRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	style := ctx.OptionString("style", "one_or_ordered")
	allowRenumbering := ctx.OptionBool("allow_renumbering", true)

	var mismatches []numberMismatch
	for _, list := range ctx.Lists() {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		if !lint.IsOrderedList(list) {
			continue
		}
		mismatches = append(mismatches, orderedListMismatches(ctx.File, list, style)...)
	}

	plan := newListShiftPlan(ctx.File)
	if allowRenumbering {
		for _, mismatch := range mismatches {
			if shift, moves := numberShift(plan, mismatch); moves {
				plan.propose(mismatch.item, shift)
			}
		}
	}
	safeShifts := plan.resolve()

	diags := make([]lint.Diagnostic, 0, len(mismatches))
	for _, mismatch := range mismatches {
		fixable := allowRenumbering
		if shift, moves := numberShift(plan, mismatch); moves {
			accepted, ok := safeShifts[mismatch.item]
			fixable = fixable && ok && accepted == shift
		}
		diags = append(diags, r.numberDiagnostic(ctx, mismatch, fixable))
	}
	return diags, nil
}

// orderedListMismatches applies markdownlint's numbering policy to one list.
func orderedListMismatches(file *mdast.FileSnapshot, list *mdast.Node, style string) []numberMismatch {
	listItems := lint.ListItems(list)
	items := make([]orderedItem, 0, len(listItems))
	for _, item := range listItems {
		marker, found := findListItemMarker(file, item)
		items = append(items, orderedItem{item: item, marker: marker, found: found && marker.Ordered})
	}
	if len(items) == 0 {
		return nil
	}

	expected := 1
	incrementing := false
	zeroPrefixed := false
	if len(items) > 1 {
		first, second := items[0], items[1]
		if !first.found || !second.found {
			// The numbering pattern cannot be determined.
			return nil
		}
		if second.marker.Number != 1 || first.marker.Number == 0 {
			incrementing = true
			if first.marker.Number == 0 {
				expected = 0
			}
		}
		zeroPrefixed = strings.HasPrefix(first.marker.NumberText, "0")
	}
	rightAligned := orderedPrefixesAligned(items)

	listStyle := style
	if _, known := orderedListStyles[listStyle]; !known {
		listStyle = "one"
		if incrementing {
			listStyle = "ordered"
		}
	}
	switch listStyle {
	case "zero":
		expected = 0
	case "one":
		expected = 1
	}

	var mismatches []numberMismatch
	for _, item := range items {
		if item.found && item.marker.Number != expected {
			mismatches = append(mismatches, numberMismatch{
				orderedItem: item,
				expected:    expected,
				text:        orderedNumberText(expected, item.marker.NumberText, rightAligned, zeroPrefixed),
			})
		}
		if listStyle == "ordered" {
			expected++
		}
	}
	return mismatches
}

// orderedNumberText formats the replacement number. In a right-aligned list
// it is padded to the width of the number it replaces, with zeros when the
// list is zero-padded.
func orderedNumberText(expected int, actual string, rightAligned, zeroPrefixed bool) string {
	text := strconv.Itoa(expected)
	if !rightAligned || len(text) >= len(actual) {
		return text
	}
	padding := " "
	if zeroPrefixed {
		padding = "0"
	}
	return strings.Repeat(padding, len(actual)-len(text)) + text
}

// orderedPrefixesAligned reports whether every item prefix (the marker and the
// whitespace after it) ends in the same column, the way markdownlint detects
// right-aligned numbers.
func orderedPrefixesAligned(items []orderedItem) bool {
	end := -1
	for _, item := range items {
		if !item.found {
			continue
		}
		spacing := 0
		if item.marker.HasContent() {
			spacing = item.marker.Spacing()
			if spacing > maxListItemSpacing {
				spacing = 1
			}
		}
		itemEnd := item.marker.Column() + item.marker.Width() + spacing
		if end >= 0 && itemEnd != end {
			return false
		}
		end = itemEnd
	}
	return true
}

// numberShift returns the column change a renumbering fix makes: a wider or
// narrower number moves the content, and padding moves the marker start.
func numberShift(plan *listShiftPlan, mismatch numberMismatch) (listShift, bool) {
	padding := len(mismatch.text) - len(strings.TrimLeft(mismatch.text, " "))
	widthChange := len(mismatch.text) - len(mismatch.marker.NumberText)
	if padding == 0 && widthChange == 0 {
		return listShift{}, false
	}
	layout, ok := plan.layout(mismatch.item)
	if !ok {
		// Without a layout the shift cannot be checked; propose an impossible
		// one so that the fix is withheld.
		return listShift{markerCol: -1, contentCol: -1}, true
	}
	return listShift{
		markerCol:  layout.markerCol + padding,
		contentCol: layout.contentCol + widthChange,
	}, true
}

func (r *OrderedListIncrementRule) numberDiagnostic(ctx *lint.RuleContext, mismatch numberMismatch, fixable bool) lint.Diagnostic {
	marker := mismatch.marker
	pos := mdast.SourcePosition{
		StartLine:   marker.Line,
		StartColumn: marker.Column() + 1,
		EndLine:     marker.Line,
		EndColumn:   marker.Column() + 1 + marker.Width(),
	}
	diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
		fmt.Sprintf("Ordered list item numbered %d should be %d", marker.Number, mismatch.expected)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use %d%c instead", mismatch.expected, marker.Delimiter))
	if fixable {
		builder := fix.NewEditBuilder()
		builder.ReplaceRange(marker.Start, marker.Start+len(marker.NumberText), mismatch.text)
		diagBuilder = diagBuilder.WithFix(builder)
	}
	return diagBuilder.Build()
}
