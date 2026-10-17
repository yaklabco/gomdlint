package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// FirstLineHeadingRule checks that files begin with a top-level heading.
type FirstLineHeadingRule struct {
	lint.BaseRule
}

// NewFirstLineHeadingRule creates a new first line heading rule.
func NewFirstLineHeadingRule() *FirstLineHeadingRule {
	return &FirstLineHeadingRule{
		BaseRule: lint.NewBaseRule(
			"MD041",
			"first-line-heading",
			"First line in a file should be a top-level heading",
			[]string{"headings", "metadata"},
			false, // Not auto-fixable.
		),
	}
}

// DefaultEnabled returns false - this rule is opt-in.
func (r *FirstLineHeadingRule) DefaultEnabled() bool {
	return false
}

// Apply checks that the first content in the file is a top-level heading.
func (r *FirstLineHeadingRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil || len(ctx.File.Content) == 0 {
		return nil, nil
	}

	requiredLevel := ctx.OptionInt("level", 1)

	// Skip front matter to find first content.
	firstContentLine := r.findFirstContentLine(ctx.File)
	if firstContentLine < 1 {
		return nil, nil
	}

	// A front matter title serves as the document title.
	if hasFrontMatterTitle(ctx) {
		return nil, nil
	}

	// Find the first block at or after the first content line.
	// This skips any front matter that goldmark may parse as blocks.
	firstBlock := r.findFirstBlockAfterLine(ctx.Root, firstContentLine)
	if ctx.OptionBool("allow_preamble", false) {
		firstBlock = lint.FirstHeading(ctx.Root)
	}
	if firstBlock == nil {
		return nil, nil
	}
	firstContentLine = firstBlock.SourcePosition().StartLine

	// Like markdownlint, an HTML heading (<h1> to <h6>) may start the document.
	if level, ok := htmlHeadingLevel(firstBlock); ok {
		if level == requiredLevel {
			return nil, nil
		}
		pos := firstBlock.SourcePosition()
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("First heading should be H%d, found H%d", requiredLevel, level)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Change to an H%d heading", requiredLevel)).
			Build()
		return []lint.Diagnostic{diag}, nil
	}

	// If first block is not a heading.
	if firstBlock.Kind != mdast.NodeHeading {
		pos := mdast.SourcePosition{
			StartLine:   firstContentLine,
			StartColumn: 1,
			EndLine:     firstContentLine,
			EndColumn:   1,
		}

		var msg string
		if requiredLevel == 1 {
			msg = "First line should be a top-level heading"
		} else {
			msg = fmt.Sprintf("First line should be an H%d heading", requiredLevel)
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Add an H%d heading at the beginning", requiredLevel)).
			Build()
		return []lint.Diagnostic{diag}, nil
	}

	// Check heading level.
	level := lint.HeadingLevel(firstBlock)
	if level != requiredLevel {
		pos := firstBlock.SourcePosition()
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("First heading should be H%d, found H%d", requiredLevel, level)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Change to an H%d heading", requiredLevel)).
			Build()
		return []lint.Diagnostic{diag}, nil
	}

	return nil, nil
}

// htmlHeadingLevel returns the level of an HTML block whose first tag is an
// HTML heading (<h1> to <h6>).
func htmlHeadingLevel(block *mdast.Node) (int, bool) {
	if block.Kind != mdast.NodeHTMLBlock {
		return 0, false
	}
	tag := lint.ExtractHTMLTagName(block.Text())
	if len(tag) != len("h1") || tag[0] != 'h' || tag[1] < '1' || tag[1] > '6' {
		return 0, false
	}
	return int(tag[1] - '0'), true
}

// findFirstBlockAfterLine finds the first block-level node that starts at or after the given line.
func (r *FirstLineHeadingRule) findFirstBlockAfterLine(root *mdast.Node, lineNum int) *mdast.Node {
	if root == nil {
		return nil
	}

	for child := root.FirstChild; child != nil; child = child.Next {
		if child.Kind == mdast.NodeHTMLBlock && lint.IsHTMLComment(child) {
			continue
		}
		pos := child.SourcePosition()
		if pos.IsValid() && pos.StartLine >= lineNum {
			return child
		}
	}

	return nil
}

func (r *FirstLineHeadingRule) findFirstContentLine(file *mdast.FileSnapshot) int {
	if file == nil || len(file.Lines) == 0 {
		return 0
	}

	for lineNum := file.FrontMatterLines + 1; lineNum <= len(file.Lines); lineNum++ {
		if !lint.IsBlankLine(file, lineNum) {
			return lineNum
		}
	}

	return 1
}

// frontMatterTitleRegexp returns the rule's front_matter_title pattern compiled
// the way markdownlint uses it: the default pattern applies when the option is
// unset, "" or false disables front matter titles, and matching ignores case.
// It returns nil when front matter titles are disabled or the pattern is invalid.
func frontMatterTitleRegexp(ctx *lint.RuleContext) *regexp.Regexp {
	pattern := config.MarkdownlintFrontMatterTitlePattern
	switch value := ctx.Option("front_matter_title", nil).(type) {
	case string:
		pattern = value
	case bool:
		if !value {
			return nil
		}
	}
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil
	}
	return re
}

// hasFrontMatterTitle reports whether the file's front matter contains a title
// matching the rule's front_matter_title pattern.
func hasFrontMatterTitle(ctx *lint.RuleContext) bool {
	re := frontMatterTitleRegexp(ctx)
	if re == nil || ctx.File == nil {
		return false
	}
	// Front matter lines include the opening and closing delimiters.
	for lineNum := 2; lineNum < ctx.File.FrontMatterLines; lineNum++ {
		if re.Match(lint.RawLineContent(ctx.File, lineNum)) {
			return true
		}
	}
	return false
}

// HeadingBlankLinesRule ensures headings are surrounded by blank lines.
type HeadingBlankLinesRule struct {
	lint.BaseRule
}

// NewHeadingBlankLinesRule creates a new heading blank lines rule.
func NewHeadingBlankLinesRule() *HeadingBlankLinesRule {
	return &HeadingBlankLinesRule{
		BaseRule: lint.NewBaseRule(
			"MD022",
			"heading-blank-lines",
			"Headings should be surrounded by blank lines",
			[]string{"headings", "whitespace", "blank_lines"},
			true, // Auto-fixable.
		),
	}
}

// Apply checks that headings have blank lines around them.
//
// The gomdlint-only markdownlint_compatible option defaults to true, which
// matches markdownlint: adjacent headings need blank lines between them.
// Setting it to false allows headings to touch other headings.
//
// Blank lines are counted inside the heading's containers, so a line holding
// only ">" is blank within a blockquote, and the fix inserts the container
// prefix (">" in a blockquote) so it never splits the container. A heading
// followed only by blank lines up to the end of the file needs none below.
//
// Each diagnostic's fix inserts the blank lines its own heading needs. Where
// two adjacent headings share a gap, both insert at the same offset and the
// engine keeps the larger insertion after applying suppressions.
func (r *HeadingBlankLinesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	compatible := ctx.OptionBool("markdownlint_compatible", true)
	includeMetadata := ctx.OptionBool("include_front_matter", false)
	lastLine := lastSourceLine(ctx.File)
	headings := ctx.Headings()
	var diags []lint.Diagnostic

	for _, heading := range headings {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		pos := heading.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		linesAbove := headingBlankLineOption(ctx, "lines_above", lint.HeadingLevel(heading))
		linesBelow := headingBlankLineOption(ctx, "lines_below", lint.HeadingLevel(heading))

		// Check blank lines above (unless it's the first line or follows front matter).
		if pos.StartLine > 1 && linesAbove > 0 && (!headingIsFirstContent(ctx, heading) || includeMetadata) {
			blanksBefore := 0
			for line := pos.StartLine - 1; line >= 1; line-- {
				if includeMetadata && line <= ctx.File.FrontMatterLines {
					break
				}
				if !lint.IsBlankLineInContainer(ctx.File, heading, line) {
					break
				}
				blanksBefore++
			}
			if blanksBefore < linesAbove {
				// Check if previous line is also a heading (allow no blank between headings).
				if compatible || !r.isPreviousLineHeading(ctx, heading, pos.StartLine) {
					diag := r.createBlankBeforeDiagnostic(ctx, heading, pos, blanksBefore, linesAbove)
					diags = append(diags, diag)
				}
			}
		}

		// Check blank lines below (unless only blank lines follow).
		if pos.EndLine < lastLine && linesBelow > 0 {
			blanksAfter := lint.CountBlankLinesAfterInContainer(ctx.File, heading, pos.EndLine)
			if blanksAfter < linesBelow && pos.EndLine+blanksAfter < lastLine {
				// Check if next non-blank line is also a heading.
				if compatible || !r.isNextLineHeading(ctx, heading, pos.EndLine) {
					diag := r.createBlankAfterDiagnostic(ctx, heading, pos, blanksAfter, linesBelow)
					diags = append(diags, diag)
				}
			}
		}
	}

	return diags, nil
}

// lastSourceLine returns the number of the last line, not counting the empty
// line that follows a final newline.
func lastSourceLine(file *mdast.FileSnapshot) int {
	count := len(file.Lines)
	if count > 0 && file.Lines[count-1].StartOffset == len(file.Content) {
		count--
	}
	return count
}

func headingBlankLineOption(ctx *lint.RuleContext, key string, level int) int {
	values, ok := ctx.Option(key, nil).([]any)
	if !ok || level < 1 || level > len(values) {
		return ctx.OptionInt(key, 1)
	}
	switch value := values[level-1].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 1
	}
}

func headingIsFirstContent(ctx *lint.RuleContext, heading *mdast.Node) bool {
	for node := ctx.Root.FirstChild; node != nil; node = node.Next {
		if node == heading {
			return true
		}
		if node.Kind != mdast.NodeHTMLBlock || !lint.IsHTMLComment(node) {
			return false
		}
	}
	return false
}

// isPreviousLineHeading reports whether the last non-blank line (within the
// heading's containers) before lineNum ends a heading.
func (r *HeadingBlankLinesRule) isPreviousLineHeading(ctx *lint.RuleContext, heading *mdast.Node, lineNum int) bool {
	for ln := lineNum - 1; ln >= 1; ln-- {
		if lint.IsBlankLineInContainer(ctx.File, heading, ln) {
			continue
		}
		for _, h := range ctx.Headings() {
			if h.SourcePosition().EndLine == ln {
				return true
			}
		}
		return false
	}
	return false
}

// isNextLineHeading reports whether the next non-blank line (within the
// heading's containers) after lineNum starts a heading.
func (r *HeadingBlankLinesRule) isNextLineHeading(ctx *lint.RuleContext, heading *mdast.Node, lineNum int) bool {
	for ln := lineNum + 1; ln <= len(ctx.File.Lines); ln++ {
		if lint.IsBlankLineInContainer(ctx.File, heading, ln) {
			continue
		}
		for _, h := range ctx.Headings() {
			if h.SourcePosition().StartLine == ln {
				return true
			}
		}
		return false
	}
	return false
}

func (r *HeadingBlankLinesRule) createBlankBeforeDiagnostic(
	ctx *lint.RuleContext,
	heading *mdast.Node,
	pos mdast.SourcePosition,
	current, required int,
) lint.Diagnostic {
	msg := fmt.Sprintf("Heading needs %d blank line(s) above, found %d", required, current)

	// Build fix: insert blank lines, with the container prefix, before the heading.
	blanksNeeded := required - current
	insertion := strings.Repeat(lint.BlankLineText(ctx.File, heading), blanksNeeded)

	line := ctx.File.Lines[pos.StartLine-1]
	diag := lint.NewDiagnostic(r.ID(), heading, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Add %d blank line(s) before the heading", blanksNeeded))
	if !loosensList(heading, line.StartOffset) {
		builder := fix.NewEditBuilder()
		builder.Insert(line.StartOffset, insertion)
		diag = diag.WithFix(builder)
	}
	return diag.Build()
}

func (r *HeadingBlankLinesRule) createBlankAfterDiagnostic(
	ctx *lint.RuleContext,
	heading *mdast.Node,
	pos mdast.SourcePosition,
	current, required int,
) lint.Diagnostic {
	msg := fmt.Sprintf("Heading needs %d blank line(s) below, found %d", required, current)

	// Build fix: insert blank lines, with the container prefix, after the heading.
	blanksNeeded := required - current
	insertion := strings.Repeat(lint.BlankLineText(ctx.File, heading), blanksNeeded)

	line := ctx.File.Lines[pos.EndLine-1]
	diag := lint.NewDiagnostic(r.ID(), heading, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Add %d blank line(s) after the heading", blanksNeeded))
	if !loosensList(heading, line.EndOffset) {
		builder := fix.NewEditBuilder()
		builder.Insert(line.EndOffset, insertion)
		diag = diag.WithFix(builder)
	}
	return diag.Build()
}

// loosensList reports whether a blank line inserted at offset would make a
// tight list around node loose, which changes how the list renders: the
// offset falls between two items of the list, or between two blocks of one
// of its items.
func loosensList(node *mdast.Node, offset int) bool {
	for item := node.Parent; item != nil; item = item.Parent {
		if item.Kind != mdast.NodeListItem || item.Parent == nil || !lint.IsTightList(item.Parent) {
			continue
		}
		if separatesChildren(item.Parent, offset) || separatesChildren(item, offset) {
			return true
		}
	}
	return false
}

// separatesChildren reports whether offset lies between two consecutive
// children of parent.
func separatesChildren(parent *mdast.Node, offset int) bool {
	for child := parent.FirstChild; child != nil && child.Next != nil; child = child.Next {
		if child.SourceRange().EndOffset <= offset && offset <= child.Next.SourceRange().StartOffset {
			return true
		}
	}
	return false
}

// RequiredHeadingsRule checks that document follows required heading structure.
type RequiredHeadingsRule struct {
	lint.BaseRule
}

// NewRequiredHeadingsRule creates a new required headings rule.
func NewRequiredHeadingsRule() *RequiredHeadingsRule {
	return &RequiredHeadingsRule{
		BaseRule: lint.NewBaseRule(
			"MD043",
			"required-headings",
			"Required heading structure",
			[]string{"headings"},
			false, // Not auto-fixable.
		),
	}
}

// DefaultEnabled returns false - this rule requires configuration.
func (r *RequiredHeadingsRule) DefaultEnabled() bool {
	return false
}

// noMoreHeadings is markdownlint's placeholder for "past the end of the list".
const noMoreHeadings = "[None]"

// Apply checks document heading structure against required pattern.
//
// It follows markdownlint: each heading is written as "## Text" (any heading
// style) with its text as it appears in the source, and is compared, in order
// and case-insensitively unless match_case is set, with the headings list.
// "*" matches zero or more headings, "+" one or more and "?" exactly one. A
// heading after the end of the list is reported, as is a list that is not
// used up. Only the first mismatch is reported. Unlike markdownlint, an
// empty list checks nothing.
func (r *RequiredHeadingsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	required := ctx.OptionStringSlice("headings", nil)
	if len(required) == 0 {
		return nil, nil
	}
	matchCase := ctx.OptionBool("match_case", false)
	fold := func(text string) string {
		if matchCase {
			return text
		}
		return strings.ToLower(text)
	}

	idx := 0
	nextExpected := func() string {
		idx++
		if idx-1 < len(required) {
			return required[idx-1]
		}
		return noMoreHeadings
	}
	matchAny, anyHeadings := false, false

	for _, heading := range ctx.Headings() {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		anyHeadings = true
		actual := strings.Repeat("#", lint.HeadingLevel(heading)) + " " + headingSourceText(ctx.File, heading)
		expected := nextExpected()
		switch {
		case expected == "*":
			if fold(nextExpected()) != fold(actual) {
				matchAny = true
				idx--
			}
		case expected == "+":
			matchAny = true
		case expected == "?":
			// Any single heading matches.
		case fold(expected) == fold(actual):
			matchAny = false
		case matchAny:
			idx--
		default:
			msg := fmt.Sprintf("Expected heading %q, found %q", expected, actual)
			if expected == noMoreHeadings {
				msg = fmt.Sprintf("Expected no more headings, found %q", actual)
			}
			diag := lint.NewDiagnostic(r.ID(), heading, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Update heading to match required structure").
				Build()
			return []lint.Diagnostic{diag}, nil
		}
	}

	remaining := len(required) - idx
	if (remaining > 1 || (remaining == 1 && required[idx] != "*")) && (anyHeadings || !allWildcards(required)) {
		lastLine := len(ctx.File.Lines)
		pos := mdast.SourcePosition{StartLine: lastLine, StartColumn: 1, EndLine: lastLine, EndColumn: 1}
		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Missing required heading %q", required[idx])).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add required heading").
			Build()
		return []lint.Diagnostic{diag}, nil
	}
	return nil, nil
}

// allWildcards reports whether every required heading is "*".
func allWildcards(required []string) bool {
	for _, heading := range required {
		if heading != "*" {
			return false
		}
	}
	return true
}

// headingSourceText returns a heading's text as written, like markdownlint's
// heading text: the raw content without its inline HTML, with line breaks
// read as spaces.
func headingSourceText(file *mdast.FileSnapshot, heading *mdast.Node) string {
	content, ok := headingContentRange(file, heading)
	if !ok {
		return ""
	}
	var text strings.Builder
	pos := content.StartOffset
	for child := heading.FirstChild; child != nil; child = child.Next {
		if child.Kind != mdast.NodeHTMLInline {
			continue
		}
		rng := child.SourceRange()
		if rng.StartOffset < pos || rng.EndOffset > content.EndOffset {
			continue
		}
		text.Write(file.Content[pos:rng.StartOffset])
		pos = rng.EndOffset
	}
	text.Write(file.Content[pos:content.EndOffset])
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(text.String())
}

// ProperNamesRule checks for correct capitalization of proper names.
type ProperNamesRule struct {
	lint.BaseRule
}

// NewProperNamesRule creates a new proper names rule.
func NewProperNamesRule() *ProperNamesRule {
	return &ProperNamesRule{
		BaseRule: lint.NewBaseRule(
			"MD044",
			"proper-names",
			"Proper names should have the correct capitalization",
			[]string{"spelling"},
			true, // Auto-fixable.
		),
	}
}

// DefaultEnabled returns false - this rule requires configuration.
func (r *ProperNamesRule) DefaultEnabled() bool {
	return false
}

// literalLinkPattern matches the addresses GFM turns into links (http(s)
// URLs, www. addresses and e-mail addresses); names inside them are not
// checked, as in markdownlint.
var literalLinkPattern = regexp.MustCompile(
	`(?i)(?:https?://|www\.)[^\s<>]*|[\w.+-]+@[\w-]+(?:\.[\w-]+)+`)

// autolinkPattern matches a CommonMark URI or e-mail autolink.
var autolinkPattern = regexp.MustCompile("^<(?:[A-Za-z][A-Za-z0-9+.-]{1,31}:[^\\x00-\\x20\\x7f<>]*|" +
	"[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?" +
	"(?:\\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*)>")

// properNameRegion is a range of source text that MD044 checks.
type properNameRegion struct {
	rng mdast.SourceRange
	// links is true when addresses in the region are links whose names are
	// not checked (prose and code, but not raw HTML).
	links bool
}

// Apply checks for incorrect capitalization of proper names.
//
// Like markdownlint, it checks the text of the document: prose, code spans
// and code block content (code_blocks, default true) and HTML (html_elements,
// default true). Link destinations, reference definitions, fence info
// strings, URLs and e-mail addresses are never checked. Text between the tags
// of an HTML block is prose, so it is checked even without html_elements.
// Longer names are matched first, and text matching one configured name is
// not checked against the names it contains, so "github.com" stays as written
// when both "GitHub" and "github.com" are listed.
func (r *ProperNamesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	properNames := ctx.OptionStringSlice("names", nil)
	if len(properNames) == 0 {
		return nil, nil
	}
	names := slices.Clone(properNames)
	sort.SliceStable(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})

	regions := r.regions(ctx, ctx.OptionBool("code_blocks", true), ctx.OptionBool("html_elements", true))
	content := ctx.File.Content

	// Addresses are excluded like matches of earlier (longer) names.
	var exclusions []mdast.SourceRange
	for _, region := range regions {
		if !region.links {
			continue
		}
		for _, match := range literalLinkPattern.FindAllIndex(content[region.rng.StartOffset:region.rng.EndOffset], -1) {
			exclusions = append(exclusions, mdast.SourceRange{
				StartOffset: region.rng.StartOffset + match[0],
				EndOffset:   region.rng.StartOffset + match[1],
			})
		}
	}

	var diags []lint.Diagnostic
	for _, name := range names {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		pattern := properNamePattern(name)
		if pattern == nil {
			continue
		}
		for _, region := range regions {
			text := content[region.rng.StartOffset:region.rng.EndOffset]
			for _, match := range pattern.FindAllSubmatchIndex(text, -1) {
				found := mdast.SourceRange{
					StartOffset: region.rng.StartOffset + match[2],
					EndOffset:   region.rng.StartOffset + match[3],
				}
				foundText := string(content[found.StartOffset:found.EndOffset])
				if !slices.Contains(properNames, foundText) && !overlapsAny(exclusions, found) {
					diags = append(diags, r.nameDiagnostic(ctx, found, foundText, name))
				}
				exclusions = append(exclusions, found)
			}
		}
	}

	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].StartLine != diags[j].StartLine {
			return diags[i].StartLine < diags[j].StartLine
		}
		return diags[i].StartColumn < diags[j].StartColumn
	})
	return diags, nil
}

// properNamePattern matches name case-insensitively as a whole word, allowing
// surrounding underscores, like markdownlint. The name is the first submatch.
func properNamePattern(name string) *regexp.Regexp {
	if name == "" {
		return nil
	}
	start, end := `\b_*`, `_*\b`
	if !isASCIIWordChar(name[0]) {
		start = ""
	}
	if !isASCIIWordChar(name[len(name)-1]) {
		end = ""
	}
	pattern, err := regexp.Compile(`(?i)` + start + `(` + regexp.QuoteMeta(name) + `)` + end)
	if err != nil {
		return nil
	}
	return pattern
}

// overlapsAny reports whether rng overlaps any of ranges.
func overlapsAny(ranges []mdast.SourceRange, rng mdast.SourceRange) bool {
	for _, other := range ranges {
		if rng.StartOffset < other.EndOffset && other.StartOffset < rng.EndOffset {
			return true
		}
	}
	return false
}

func (r *ProperNamesRule) nameDiagnostic(
	ctx *lint.RuleContext,
	found mdast.SourceRange,
	foundText, name string,
) lint.Diagnostic {
	builder := fix.NewEditBuilder()
	builder.ReplaceRange(found.StartOffset, found.EndOffset, name)
	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, rangePosition(ctx.File, found),
		fmt.Sprintf("Proper name %q should be %q", foundText, name)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Use %q", name)).
		WithFix(builder).
		Build()
}

// properNameScan collects the source text MD044 checks.
type properNameScan struct {
	ctx         *lint.RuleContext
	includeCode bool
	includeHTML bool
	regions     []properNameRegion
}

// regions returns the source text MD044 checks.
func (r *ProperNamesRule) regions(ctx *lint.RuleContext, includeCode, includeHTML bool) []properNameRegion {
	scan := &properNameScan{ctx: ctx, includeCode: includeCode, includeHTML: includeHTML}
	scan.collect(ctx.File, 0, nil, false)
	return scan.regions
}

// addRegion records the document range from start to end.
func (s *properNameScan) addRegion(start, end int, links bool) {
	if start < end {
		s.regions = append(s.regions, properNameRegion{
			rng:   mdast.SourceRange{StartOffset: start, EndOffset: end},
			links: links,
		})
	}
}

// collect adds the regions of file. For the content of an HTML block parsed
// again, nested is true, base is the block's offset in the document and tags
// holds the masked tags, sorted, which are cut out of the prose around them.
func (s *properNameScan) collect(file *mdast.FileSnapshot, base int, tags []mdast.SourceRange, nested bool) {
	add := func(rng mdast.SourceRange, links bool, cuts []mdast.SourceRange) {
		if rng.StartOffset < 0 || rng.StartOffset >= rng.EndOffset || rng.EndOffset > len(file.Content) {
			return
		}
		start := rng.StartOffset
		for _, cut := range cuts {
			if cut.EndOffset <= start || cut.StartOffset >= rng.EndOffset {
				continue
			}
			s.addRegion(base+start, base+cut.StartOffset, links)
			start = cut.EndOffset
		}
		s.addRegion(base+start, base+rng.EndOffset, links)
	}
	//nolint:errcheck // Walk visitor never returns error.
	_ = mdast.Walk(file.Root, func(node *mdast.Node) error {
		switch node.Kind {
		case mdast.NodeText, mdast.NodeSoftBreak, mdast.NodeHardBreak:
			if insideAutolink(node) {
				return nil
			}
			if start, ok := textPrefixStart(file, node); ok {
				add(mdast.SourceRange{StartOffset: start, EndOffset: start + len(node.Inline.Text)}, true, tags)
			}
		case mdast.NodeCodeSpan, mdast.NodeCodeBlock:
			if s.includeCode {
				add(node.ContentRange(), true, nil)
			}
		case mdast.NodeHTMLInline:
			if s.includeHTML {
				add(node.SourceRange(), false, nil)
			}
		case mdast.NodeHTMLBlock:
			// Content parsed again has no HTML blocks, as maskHTML masks
			// every '<' that could start one.
			if !nested {
				s.collectHTMLBlock(file, node)
			}
		}
		return nil
	})
}

// collectHTMLBlock adds the regions of an HTML block. A comment block is
// HTML throughout. Any other block is read as Markdown without HTML blocks or
// indented code, as markdownlint does: its tags are HTML, checked only with
// html_elements, and the rest is parsed again, so that its prose is checked,
// its code, tags included, follows code_blocks, and fence info strings and
// link destinations are skipped.
func (s *properNameScan) collectHTMLBlock(file *mdast.FileSnapshot, block *mdast.Node) {
	rng := block.SourceRange()
	if rng.IsEmpty() || rng.EndOffset > len(file.Content) {
		return
	}
	if lint.IsHTMLComment(block) {
		if s.includeHTML {
			s.addRegion(rng.StartOffset, rng.EndOffset, false)
		}
		return
	}

	masked, tags := maskHTML(file.Content[rng.StartOffset:rng.EndOffset])
	maskCodeIndentation(masked)
	snapshot, ok := reparse(s.ctx, masked)
	if !ok {
		return
	}
	if s.includeHTML {
		code := codeRanges(snapshot)
		for _, tag := range tags {
			if !overlapsAny(code, tag) {
				s.addRegion(rng.StartOffset+tag.StartOffset, rng.StartOffset+tag.EndOffset, false)
			}
		}
	}
	s.collect(snapshot, rng.StartOffset, tags, true)
}

// maskHTML returns a copy of text, the content of an HTML block, in which
// every tag, comment, processing instruction, declaration and CDATA section
// is replaced with commas, keeping line breaks, so that offsets hold and the
// tags neither form HTML blocks nor join the words around them. Any other '<'
// outside an autolink is replaced as well, as it might still start an HTML
// block. It also returns the ranges of the replaced constructs, in order.
func maskHTML(text []byte) ([]byte, []mdast.SourceRange) {
	masked := slices.Clone(text)
	var tags []mdast.SourceRange
	for idx := 0; idx < len(text); idx++ {
		if text[idx] != '<' {
			continue
		}
		end := htmlConstructEnd(text, idx)
		if end < 0 {
			if loc := autolinkPattern.FindIndex(text[idx:]); loc != nil {
				idx += loc[1] - 1
			} else {
				masked[idx] = ','
			}
			continue
		}
		tags = append(tags, mdast.SourceRange{StartOffset: idx, EndOffset: end})
		for pos := idx; pos < end; pos++ {
			if masked[pos] != '\n' && masked[pos] != '\r' {
				masked[pos] = ','
			}
		}
		idx = end - 1
	}
	return masked, tags
}

// codeRanges returns the ranges of the code spans and code blocks of file,
// fences included.
func codeRanges(file *mdast.FileSnapshot) []mdast.SourceRange {
	var ranges []mdast.SourceRange
	//nolint:errcheck // Walk visitor never returns error.
	_ = mdast.Walk(file.Root, func(node *mdast.Node) error {
		if node.Kind == mdast.NodeCodeSpan || node.Kind == mdast.NodeCodeBlock {
			ranges = append(ranges, node.SourceRange())
		}
		return nil
	})
	return ranges
}

// maskCodeIndentation replaces the indentation of lines indented by four or
// more columns with commas: markdownlint reads HTML block content without
// indented code blocks, so such lines are prose.
func maskCodeIndentation(text []byte) {
	const codeIndent = 4
	for lineStart := 0; lineStart < len(text); {
		col, pos := 0, lineStart
		for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t') {
			if text[pos] == '\t' {
				col += codeIndent - col%codeIndent
			} else {
				col++
			}
			pos++
		}
		if col >= codeIndent {
			for idx := lineStart; idx < pos; idx++ {
				text[idx] = ','
			}
		}
		next := bytes.IndexByte(text[lineStart:], '\n')
		if next < 0 {
			break
		}
		lineStart += next + 1
	}
}

// htmlConstructEnd returns the end of the HTML tag, comment, processing
// instruction, declaration or CDATA section that starts with the '<' at
// text[start], following CommonMark's raw HTML grammar, or -1 when none does.
// Quoted attribute values may hold '>'.
func htmlConstructEnd(text []byte, start int) int {
	rest := text[start:]
	switch {
	case bytes.HasPrefix(rest, []byte("<!-->")):
		return start + len("<!-->")
	case bytes.HasPrefix(rest, []byte("<!--->")):
		return start + len("<!--->")
	case bytes.HasPrefix(rest, []byte("<!--")):
		return indexAfter(text, start+len("<!--"), "-->")
	case bytes.HasPrefix(rest, []byte("<?")):
		return indexAfter(text, start+len("<?"), "?>")
	case bytes.HasPrefix(rest, []byte("<![CDATA[")):
		return indexAfter(text, start+len("<![CDATA["), "]]>")
	case len(rest) > 2 && rest[1] == '!' && isASCIILetter(rest[2]):
		return indexAfter(text, start+2, ">")
	case len(rest) > 2 && rest[1] == '/' && isASCIILetter(rest[2]):
		pos := skipTagName(text, start+2)
		pos = skipHTMLSpace(text, pos)
		if pos < len(text) && text[pos] == '>' {
			return pos + 1
		}
		return -1
	case len(rest) > 1 && isASCIILetter(rest[1]):
		return openTagEnd(text, skipTagName(text, start+1))
	default:
		return -1
	}
}

// openTagEnd scans the attributes of an open tag from pos, just after the tag
// name, and returns the end of the tag or -1.
func openTagEnd(text []byte, pos int) int {
	for {
		spaceEnd := skipHTMLSpace(text, pos)
		switch {
		case spaceEnd >= len(text):
			return -1
		case text[spaceEnd] == '>':
			return spaceEnd + 1
		case text[spaceEnd] == '/':
			if spaceEnd+1 < len(text) && text[spaceEnd+1] == '>' {
				return spaceEnd + 2
			}
			return -1
		case spaceEnd == pos || !isAttributeNameStart(text[spaceEnd]):
			// Attributes must be separated by whitespace.
			return -1
		}
		pos = spaceEnd + 1
		for pos < len(text) && isAttributeNameChar(text[pos]) {
			pos++
		}
		valueStart := skipHTMLSpace(text, pos)
		if valueStart >= len(text) || text[valueStart] != '=' {
			continue
		}
		pos = attributeValueEnd(text, skipHTMLSpace(text, valueStart+1))
		if pos < 0 {
			return -1
		}
	}
}

// attributeValueEnd returns the end of the attribute value at pos: quoted
// with ' or ", or unquoted. It returns -1 when there is no valid value.
func attributeValueEnd(text []byte, pos int) int {
	if pos >= len(text) {
		return -1
	}
	if quote := text[pos]; quote == '"' || quote == '\'' {
		end := bytes.IndexByte(text[pos+1:], quote)
		if end < 0 {
			return -1
		}
		return pos + 1 + end + 1
	}
	end := pos
	for end < len(text) && strings.IndexByte(" \t\r\n\"'=<>`", text[end]) < 0 {
		end++
	}
	if end == pos {
		return -1
	}
	return end
}

// indexAfter returns the offset just past the first marker at or after pos,
// or -1.
func indexAfter(text []byte, pos int, marker string) int {
	idx := bytes.Index(text[pos:], []byte(marker))
	if idx < 0 {
		return -1
	}
	return pos + idx + len(marker)
}

// skipTagName skips the letters, digits and hyphens of a tag name.
func skipTagName(text []byte, pos int) int {
	for pos < len(text) && (isASCIILetter(text[pos]) || (text[pos] >= '0' && text[pos] <= '9') || text[pos] == '-') {
		pos++
	}
	return pos
}

// skipHTMLSpace skips spaces, tabs and line endings.
func skipHTMLSpace(text []byte, pos int) int {
	for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t' || text[pos] == '\n' || text[pos] == '\r') {
		pos++
	}
	return pos
}

// isASCIILetter reports whether b is an ASCII letter.
func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isAttributeNameStart reports whether b can start an HTML attribute name.
func isAttributeNameStart(b byte) bool {
	return isASCIILetter(b) || b == '_' || b == ':'
}

// isAttributeNameChar reports whether b can continue an HTML attribute name.
func isAttributeNameChar(b byte) bool {
	return isAttributeNameStart(b) || (b >= '0' && b <= '9') || b == '.' || b == '-'
}

// insideAutolink reports whether node is inside an autolink.
func insideAutolink(node *mdast.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeLink && parent.Inline != nil && parent.Inline.Link != nil &&
			parent.Inline.Link.ReferenceStyle == mdast.RefStyleAutolink {
			return true
		}
	}
	return false
}

// textPrefixStart returns where a node's raw inline text starts in the
// source: text nodes span exactly their text, and line breaks start with the
// text that ends the line.
func textPrefixStart(file *mdast.FileSnapshot, node *mdast.Node) (int, bool) {
	if node.Inline == nil || len(node.Inline.Text) == 0 {
		return 0, false
	}
	rng := node.SourceRange()
	end := rng.StartOffset + len(node.Inline.Text)
	if rng.StartOffset < 0 || end > rng.EndOffset || end > len(file.Content) ||
		!bytes.Equal(file.Content[rng.StartOffset:end], node.Inline.Text) {
		return 0, false
	}
	return rng.StartOffset, true
}
