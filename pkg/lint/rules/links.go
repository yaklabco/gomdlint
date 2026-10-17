package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

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

// reversedLinkPattern matches (text)[url] patterns.
var reversedLinkPattern = regexp.MustCompile(`\(([^)]*)\)\[([^\]]*)\]`)

// reversedLinkMatchIndices is the minimum number of submatch indices required.
// Pattern has 2 capture groups: full (0:1), text (2:3), url (4:5).
const reversedLinkMatchIndices = 6

// Apply checks for reversed link syntax in the file content.
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

		lineContent := lint.LineContent(ctx.File, lineNum)
		matches := reversedLinkPattern.FindAllSubmatchIndex(lineContent, -1)

		for _, match := range matches {
			if len(match) < reversedLinkMatchIndices {
				continue
			}

			// match[0]:match[1] = full match
			// match[2]:match[3] = text (inside parens)
			// match[4]:match[5] = url (inside brackets)
			text := string(lineContent[match[2]:match[3]])
			url := string(lineContent[match[4]:match[5]])

			line := ctx.File.Lines[lineNum-1]
			startOffset := line.StartOffset + match[0]
			endOffset := line.StartOffset + match[1]
			inCode := false
			for _, span := range ctx.CodeSpans() {
				if span.SourceRange().Contains(startOffset) {
					inCode = true
					break
				}
			}
			if inCode {
				continue
			}

			// Build fix: convert (text)[url] to [text](url)
			builder := fix.NewEditBuilder()
			newText := fmt.Sprintf("[%s](%s)", text, url)
			builder.ReplaceRange(startOffset, endOffset, newText)

			pos := mdast.SourcePosition{
				StartLine:   lineNum,
				StartColumn: match[0] + 1,
				EndLine:     lineNum,
				EndColumn:   match[1],
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

	return diags, nil
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

	return diags, nil
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

// Apply checks for links whose destination is empty or an empty fragment ("#").
// Like markdownlint, the destination of a reference link is the destination of
// its definition, and links with empty text but a destination are not empty.
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

		if link.Inline != nil && link.Inline.Link != nil && link.Inline.Link.ReferenceStyle == mdast.RefStyleAutolink {
			continue
		}

		var msg string
		switch strings.TrimSpace(lint.LinkDestination(link)) {
		case "":
			msg = "Link has empty destination"
		case "#":
			msg = "Link has empty fragment"
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

		dest := lint.LinkDestination(link)
		if dest == "" {
			continue
		}

		// Skip fragment-only links (#anchor).
		if strings.HasPrefix(dest, "#") {
			continue
		}

		isAbsolute := isAbsoluteURL(dest)
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

// isAbsoluteURL returns true if the URL is absolute (has a scheme).
func isAbsoluteURL(url string) bool {
	// Check for common schemes.
	return strings.HasPrefix(url, "http://") ||
		strings.HasPrefix(url, "https://") ||
		strings.HasPrefix(url, "ftp://") ||
		strings.HasPrefix(url, "mailto:") ||
		strings.HasPrefix(url, "tel:") ||
		strings.HasPrefix(url, "file://") ||
		strings.Contains(url, "://")
}
