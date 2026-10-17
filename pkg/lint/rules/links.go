package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	gmutil "github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// ReversedLinkRule detects reversed link syntax: (text)[url] instead of [text](url).
type ReversedLinkRule struct {
	lint.BaseRule
}

// NewReversedLinkRule creates a new reversed link rule.
func NewReversedLinkRule() *ReversedLinkRule {
	return &ReversedLinkRule{
		BaseRule: lint.NewBaseRule(
			"MD011",
			"no-reversed-links",
			"Reversed link syntax (text)[url] should be [text](url)",
			[]string{"links"},
			true,
		),
	}
}

// reversedLinkPattern is markdownlint's reversedLinkRe, anchored at a "("
// that no backslash escapes: "(text)" without parentheses inside, then
// "[destination]" that does not start like a footnote label. Its lookahead,
// which rejects a match followed by "(", is checked by reversedLinkAt.
var reversedLinkPattern = regexp.MustCompile(`^\(([^()]+)\)\[([^\]^][^\]]*)\]`)

// reversedLinkMatchIndices is the number of submatch indices
// reversedLinkPattern produces: the match, the text, and the destination.
const reversedLinkMatchIndices = 6

// Apply checks for reversed link syntax in the file content, as markdownlint
// does: lines in code blocks and matches that overlap a code span are
// skipped, and so are matches followed by "(" or whose text or destination
// ends with a backslash.
func (r *ReversedLinkRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
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

		lineContent := lint.LineText(ctx.File, lineNum)
		lineStart := ctx.File.Lines[lineNum-1].StartOffset + lint.LineTextOffset(ctx.File, lineNum)
		for pos := 0; pos < len(lineContent); pos++ {
			end, text, url, ok := reversedLinkAt(lineContent, pos)
			if !ok {
				continue
			}
			start := pos
			pos = end - 1 // Like a global regexp, the next match starts after this one.
			if strings.HasSuffix(text, "\\") || strings.HasSuffix(url, "\\") {
				continue
			}
			startOffset, endOffset := lineStart+start, lineStart+end
			if overlapsCodeSpan(ctx, startOffset, endOffset) {
				continue
			}

			// Build fix: convert (text)[url] to [text](url)
			builder := fix.NewEditBuilder()
			newText := fmt.Sprintf("[%s](%s)", text, url)
			builder.ReplaceRange(startOffset, endOffset, newText)

			startLine, startColumn := ctx.File.LineAt(startOffset)
			endLine, endColumn := ctx.File.LineAt(endOffset)
			pos := mdast.SourcePosition{
				StartLine:   startLine,
				StartColumn: startColumn,
				EndLine:     endLine,
				EndColumn:   endColumn,
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				"Reversed link syntax detected").
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use [text](url) instead of (text)[url]").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return verifyFixes(ctx, diags, fixAllowance{reversedLinks: true}), nil
}

// reversedLinkAt matches a reversed link starting at line[pos]: a "(" that
// no backslash escapes, matching reversedLinkPattern and not followed by
// "(". It returns the offset after the match, the text, and the destination.
func reversedLinkAt(line []byte, pos int) (int, string, string, bool) {
	if line[pos] != '(' || (pos > 0 && line[pos-1] == '\\') {
		return 0, "", "", false
	}
	match := reversedLinkPattern.FindSubmatchIndex(line[pos:])
	if len(match) < reversedLinkMatchIndices {
		return 0, "", "", false
	}
	end := pos + match[1]
	if end < len(line) && line[end] == '(' {
		return 0, "", "", false
	}
	return end, string(line[pos+match[2] : pos+match[3]]), string(line[pos+match[4] : pos+match[5]]), true
}

// overlapsCodeSpan reports whether content[start:end] overlaps a code span.
func overlapsCodeSpan(ctx *lint.RuleContext, start, end int) bool {
	for _, span := range ctx.CodeSpans() {
		if source := span.SourceRange(); source.StartOffset < end && start < source.EndOffset {
			return true
		}
	}
	return false
}

// LinkSpacesRule detects spaces inside link text: [ text ] instead of [text].
type LinkSpacesRule struct {
	lint.BaseRule
}

// NewLinkSpacesRule creates a new link spaces rule.
func NewLinkSpacesRule() *LinkSpacesRule {
	return &LinkSpacesRule{
		BaseRule: lint.NewBaseRule(
			"MD039",
			"no-space-in-links",
			"Link text should not have leading or trailing spaces",
			[]string{"links", "whitespace"},
			true,
		),
	}
}

// Apply checks the raw link text between the brackets for leading or trailing
// whitespace. Like markdownlint, the fix removes the spaces and tabs beside
// each bracket but never a line ending.
func (r *LinkSpacesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	links := ctx.Links()
	diags := make([]lint.Diagnostic, 0, len(links))

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if isAutolinkNode(link) {
			continue
		}

		start, end, ok := linkTextSpan(ctx.File.Content, link)
		if !ok {
			continue
		}
		text := string(ctx.File.Content[start:end])
		allLeading := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
		allTrailing := len(text) - len(strings.TrimRightFunc(text, unicode.IsSpace))
		leading := len(text) - len(strings.TrimLeftFunc(text, isSameLineSpace))
		trailing := len(text) - len(strings.TrimRightFunc(text, isSameLineSpace))

		// Each side is reported where its spaces are. Without spaces on the
		// bracket's own line (the text starts or ends with a line ending),
		// the side is reported where the whitespace ends, without a fix.
		if allLeading > 0 {
			at, deleteTo := start+allLeading, start
			if leading > 0 {
				at, deleteTo = start, start+leading
			}
			diags = append(diags, r.spaceDiagnostic(ctx, at, start, deleteTo, "Link text has leading spaces"))
		}
		if allTrailing > 0 {
			at, deleteFrom := end-allTrailing, end
			if trailing > 0 && leading < len(text) {
				at, deleteFrom = end-trailing, end-trailing
			}
			// A backslash left before the closing bracket would escape it.
			if escapesNext(ctx.File.Content, start, deleteFrom) {
				deleteFrom = end
			}
			diags = append(diags, r.spaceDiagnostic(ctx, at, deleteFrom, end, "Link text has trailing spaces"))
		}
	}

	return verifyFixes(ctx, diags, fixAllowance{linkTextSpace: true}), nil
}

// spaceDiagnostic reports spaces at offset at, with a fix that deletes
// content[from:to] when that range is not empty.
func (r *LinkSpacesRule) spaceDiagnostic(ctx *lint.RuleContext, at, from, to int, msg string) lint.Diagnostic {
	line, column := ctx.File.LineAt(at)
	pos := mdast.SourcePosition{StartLine: line, StartColumn: column, EndLine: line, EndColumn: column + 1}
	diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Remove spaces from link text")
	if from < to {
		builder := fix.NewEditBuilder()
		builder.Delete(from, to)
		diagBuilder = diagBuilder.WithFix(builder)
	}
	return diagBuilder.Build()
}

// escapesNext reports whether content[from:at] ends with an odd number of
// backslashes, so that the last one escapes the character placed at at.
func escapesNext(content []byte, from, at int) bool {
	count := 0
	for idx := at - 1; idx >= from && content[idx] == '\\'; idx-- {
		count++
	}
	return count%2 == 1
}

// isSameLineSpace reports whether r is whitespace other than a line ending.
func isSameLineSpace(r rune) bool {
	return r != '\n' && r != '\r' && unicode.IsSpace(r)
}

// isAutolinkNode reports whether a link node is an autolink, which has no link text brackets.
func isAutolinkNode(node *mdast.Node) bool {
	return node.Inline != nil && node.Inline.Link != nil && node.Inline.Link.ReferenceStyle == mdast.RefStyleAutolink
}

// linkTextSpan returns the byte range of the text between the brackets of a
// link or image: its content span. It reports false when the span is not
// enclosed by brackets.
func linkTextSpan(content []byte, node *mdast.Node) (int, int, bool) {
	text := node.ContentRange()
	if text.StartOffset < 1 || text.EndOffset >= len(content) ||
		content[text.StartOffset-1] != '[' || content[text.EndOffset] != ']' {
		return 0, 0, false
	}
	return text.StartOffset, text.EndOffset, true
}

// EmptyLinkRule detects links with empty destination or text.
type EmptyLinkRule struct {
	lint.BaseRule
}

// NewEmptyLinkRule creates a new empty link rule.
func NewEmptyLinkRule() *EmptyLinkRule {
	return &EmptyLinkRule{
		BaseRule: lint.NewBaseRule(
			"MD042",
			"no-empty-links",
			"Links should have both text and destination",
			[]string{"links"},
			false, // Not auto-fixable.
		),
	}
}

// Apply checks for links whose destination is empty or an empty fragment
// ("#"), as markdownlint does: an inline link is empty when its destination
// is missing or "#", and a reference link when its definition's destination
// is "#". Links with empty text but a destination are not empty.
func (r *EmptyLinkRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	links := ctx.Links()
	diags := make([]lint.Diagnostic, 0, len(links))

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		style := mdast.RefStyleInline
		if link.Inline != nil && link.Inline.Link != nil {
			style = link.Inline.Link.ReferenceStyle
		}
		if style == mdast.RefStyleAutolink {
			continue
		}

		var msg string
		switch destination := strings.TrimSpace(lint.LinkDestination(link)); {
		case destination == "#":
			msg = "Link has empty fragment"
		case destination == "" && style == mdast.RefStyleInline:
			msg = "Link has empty destination"
		default:
			continue
		}

		diag := lint.NewDiagnostic(r.ID(), link, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Provide a link destination").
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

// ImageAltTextRule checks that images have alt text.
type ImageAltTextRule struct {
	lint.BaseRule
}

// NewImageAltTextRule creates a new image alt text rule.
func NewImageAltTextRule() *ImageAltTextRule {
	return &ImageAltTextRule{
		BaseRule: lint.NewBaseRule(
			"MD045",
			"no-alt-text",
			"Images should have alt text",
			[]string{"links", "images", "accessibility"},
			false, // Not auto-fixable.
		),
	}
}

// imgAltAttributePattern and imgAriaHiddenAttributePattern are markdownlint's
// getHtmlAttributeRe patterns for the attributes that decide whether an HTML
// image has alt text: whitespace, the name, optional whitespace, '=', and a
// value captured up to a quote, whitespace, or '>'.
var (
	imgAltAttributePattern        = regexp.MustCompile(`(?i)\salt\s*=\s*['"]?([^'"\s>]*)`)
	imgAriaHiddenAttributePattern = regexp.MustCompile(`(?i)\saria-hidden\s*=\s*['"]?([^'"\s>]*)`)
)

// Apply checks that Markdown images have non-empty alt text and that HTML img
// elements have an alt attribute or are hidden with aria-hidden="true".
func (r *ImageAltTextRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	images := ctx.Images()
	diags := make([]lint.Diagnostic, 0, len(images))

	for _, img := range images {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		alt := lint.ImageAlt(img)
		if len(bytes.TrimSpace([]byte(alt))) > 0 {
			continue
		}

		diag := lint.NewDiagnostic(r.ID(), img, "Image is missing alt text").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add descriptive alt text to the image").
			Build()
		diags = append(diags, diag)
	}

	if ctx.File == nil {
		return diags, nil
	}
	for _, image := range r.htmlImages(ctx) {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		construct, tag := image.construct, image.construct.Text(image.source)
		if imgAltAttributePattern.Match(tag) {
			continue
		}
		if hidden := imgAriaHiddenAttributePattern.FindSubmatch(tag); hidden != nil &&
			strings.EqualFold(string(hidden[1]), "true") {
			continue
		}
		startLine, startColumn := ctx.File.LineAt(construct.Start)
		endLine, endColumn := ctx.File.LineAt(construct.End)
		pos := mdast.SourcePosition{StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn}
		diags = append(diags, lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "HTML image is missing alt text").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add an alt attribute to the img element").
			Build())
	}

	sortDiagnosticsByPosition(diags)
	return diags, nil
}

// sortDiagnosticsByPosition orders diagnostics by start line, then start column.
func sortDiagnosticsByPosition(diags []lint.Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].StartLine != diags[j].StartLine {
			return diags[i].StartLine < diags[j].StartLine
		}
		return diags[i].StartColumn < diags[j].StartColumn
	})
}

// htmlImages returns the opening img tags in HTML blocks and inline HTML.
func (r *ImageAltTextRule) htmlImages(ctx *lint.RuleContext) []htmlImage {
	var images []htmlImage
	keep := func(source []byte, construct mdscan.HTMLConstruct) {
		if construct.IsOpeningElement() && strings.EqualFold(construct.Name, "img") {
			images = append(images, htmlImage{construct: construct, source: source})
		}
	}
	for _, block := range ctx.HTMLBlocks() {
		span := block.SourceRange()
		source := htmlSource(ctx.File, block)
		for _, construct := range mdscan.ScanHTML(source, span.StartOffset, span.EndOffset) {
			keep(source, construct)
		}
	}
	for _, inline := range ctx.HTMLInlines() {
		source := htmlSource(ctx.File, inline)
		if construct, ok := htmlNodeConstruct(source, inline); ok {
			keep(source, construct)
		}
	}
	return images
}

// htmlImage is an img element and the source its attributes are read from.
type htmlImage struct {
	construct mdscan.HTMLConstruct
	source    []byte
}

// LinkDestinationStyleRule enforces link destination style (relative vs absolute).
type LinkDestinationStyleRule struct {
	lint.BaseRule
}

// NewLinkDestinationStyleRule creates a new link destination style rule.
func NewLinkDestinationStyleRule() *LinkDestinationStyleRule {
	return &LinkDestinationStyleRule{
		BaseRule: lint.NewBaseRule(
			"MDL001",
			"link-destination-style",
			"Link destination style should be consistent",
			[]string{"links", "style"},
			false, // Not auto-fixable.
		),
	}
}

// LinkDestStyle represents the style of link destinations.
type LinkDestStyle string

const (
	// LinkDestRelative requires relative URLs.
	LinkDestRelative LinkDestStyle = "relative"
	// LinkDestAbsolute requires absolute URLs.
	LinkDestAbsolute LinkDestStyle = "absolute"
	// LinkDestConsistent uses whatever style is first encountered.
	LinkDestConsistent LinkDestStyle = "consistent"
)

// DefaultEnabled returns false for this optional rule.
func (r *LinkDestinationStyleRule) DefaultEnabled() bool {
	return false
}

// Apply checks link destination style consistency.
func (r *LinkDestinationStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	configStyle := LinkDestStyle(ctx.OptionString("style", string(LinkDestConsistent)))
	effectiveStyle := configStyle
	if configStyle == LinkDestConsistent {
		effectiveStyle = "" // Will be set from first link.
	}

	links := ctx.Links()
	var diags []lint.Diagnostic

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		dest := resolvedDestination(link)
		if dest == "" {
			continue
		}

		// Skip fragment-only links (#anchor).
		if strings.HasPrefix(dest, "#") {
			continue
		}

		// An autolink is always absolute: CommonMark autolinks need a scheme,
		// and email autolinks, which the parser gives without one, render
		// as mailto: links. Other links are classified by their destination,
		// so an autolink and an inline link to the same URI agree.
		isAbsolute := isAutolinkNode(link) || isAbsoluteDestination(dest)
		detectedStyle := LinkDestRelative
		if isAbsolute {
			detectedStyle = LinkDestAbsolute
		}

		// Set consistent style from first link.
		if effectiveStyle == "" {
			effectiveStyle = detectedStyle
			continue
		}

		// Check for style mismatch.
		if detectedStyle != effectiveStyle {
			var msg string
			if effectiveStyle == LinkDestAbsolute {
				msg = "Link uses relative URL, but absolute URLs are expected"
			} else {
				msg = "Link uses absolute URL, but relative URLs are expected"
			}

			diag := lint.NewDiagnostic(r.ID(), link, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %s URLs consistently", effectiveStyle)).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// uriSchemePattern matches a URI scheme and its colon at the start of a link
// destination, as CommonMark defines a scheme for autolinks: an ASCII letter
// followed by 1 to 31 letters, digits, "+", "." or "-".
var uriSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]{1,31}:`)

// isAbsoluteDestination reports whether MDL001 counts a link destination as
// absolute. A destination is absolute when it starts with a URI scheme, such
// as https:, mailto:, sms: or urn:, or when it starts with "//", a
// network-path reference that names its own host and keeps the page's
// scheme. A scheme has at least two characters, so a Windows drive letter,
// as in C:\docs or C:/docs, starts a relative path. A destination such as
// localhost:8080 starts with the scheme "localhost", as browsers and
// CommonMark autolinks read it, so it is absolute; a local server's address
// written with http:// reads the same way.
func isAbsoluteDestination(dest string) bool {
	return strings.HasPrefix(dest, "//") || uriSchemePattern.MatchString(dest)
}

// resolvedDestination returns a link's destination as the renderer reads
// it. The parser keeps a destination as written, so backslash escapes and
// entity and numeric character references are resolved here with
// goldmark's own functions, in the order its HTML renderer applies them.
// Percent-encoding stays as written, so https%3A//x starts no scheme.
func resolvedDestination(link *mdast.Node) string {
	dest := []byte(lint.LinkDestination(link))
	dest = gmutil.UnescapePunctuations(dest)
	dest = gmutil.ResolveNumericReferences(dest)
	return string(gmutil.ResolveEntityNames(dest))
}
