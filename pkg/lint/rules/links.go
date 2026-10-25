package rules

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"

	gmutil "github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/internal/jstext"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/lint/refs"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// ReversedLinkRule detects reversed link syntax: (text)[url] instead of [text](url).
type ReversedLinkRule struct {
	lint.BaseRule
}

// NewReversedLinkRule creates a new reversed link rule.
func NewReversedLinkRule() *ReversedLinkRule {
	return &ReversedLinkRule{
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD011")),
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
			end, text, dest, ok := reversedLinkAt(lineContent, pos)
			if !ok {
				continue
			}
			start := pos
			pos = end - 1 // Like a global regexp, the next match starts after this one.
			if strings.HasSuffix(text, "\\") || strings.HasSuffix(dest, "\\") {
				continue
			}
			startOffset, endOffset := lineStart+start, lineStart+end
			if overlapsCodeSpan(ctx, startOffset, endOffset) {
				continue
			}

			// Build fix: convert (text)[url] to [text](url)
			builder := fix.NewEditBuilder()
			newText := fmt.Sprintf("[%s](%s)", text, dest)
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
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD039")),
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
	micromark := &micromarkLinks{ctx: ctx}
	diags := make([]lint.Diagnostic, 0, len(links))

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if isAutolinkNode(link) || !micromark.reads(link) {
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

// micromarkLinks tells which links and images of a document micromark, and
// so markdownlint, reads as links and images.
type micromarkLinks struct {
	ctx *lint.RuleContext
	// footnotes holds the footnote definitions micromark is certain to
	// read (see certainFootnoteLabels), built on first use.
	footnotes map[string]bool
}

// reads reports whether micromark reads link, a link or image, as one.
// Three references are links to goldmark alone: one whose label micromark
// reads as no label, such as "[ ]" or "[x][ ]" (see refs.MicromarkLabel); a
// shortcut reference right before a "[", which goldmark reads when the "["
// opens no label it can close, as in "[x][y" at the end of a line, and
// micromark never does; and one whose label calls a GFM footnote, such as
// "[x][^n]" with "[^n]: note", which goldmark reads as a link reference
// definition. Only a plain footnote label and a footnote definition that
// micromark is certain to read count (see plainFootnoteLabel).
func (m *micromarkLinks) reads(link *mdast.Node) bool {
	if link.Inline == nil || link.Inline.Link == nil {
		return true
	}
	label := link.Inline.Link.ReferenceLabel
	switch link.Inline.Link.ReferenceStyle {
	case mdast.RefStyleFull, mdast.RefStyleCollapsed:
	case mdast.RefStyleShortcut:
		if file := link.File; file != nil {
			if end := link.SourceRange().EndOffset; end >= 0 && end < len(file.Content) && file.Content[end] == '[' {
				return false
			}
		}
	case mdast.RefStyleInline, mdast.RefStyleAutolink:
		return true
	}
	if !refs.MicromarkLabel(label) {
		return false
	}
	if footnote, ok := strings.CutPrefix(label, "^"); ok && plainFootnoteLabel(footnote) {
		if m.footnotes == nil {
			m.footnotes = certainFootnoteLabels(m.ctx)
		}
		return !m.footnotes[labelKey(label)]
	}
	return true
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
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD042")),
	}
}

// Apply checks for links whose destination is empty or an empty fragment
// ("#"), as markdownlint does: an inline link is empty when its destination
// is missing or "#", and a reference link when its definition's destination
// is "#". Links with empty text but a destination are not empty.
//
// markdownlint finds a reference link's definition by its label as written,
// trimmed, among the definitions keyed by refs.MarkdownlintKey, and so does
// MD042: "[X]" with "[x]: #" is not reported, though the link resolves.
func (r *EmptyLinkRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	links := ctx.Links()
	micromark := &micromarkLinks{ctx: ctx}
	diags := make([]lint.Diagnostic, 0, len(links))
	var definitions map[string]*refs.ReferenceDefinition

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		style := mdast.RefStyleInline
		if link.Inline != nil && link.Inline.Link != nil {
			style = link.Inline.Link.ReferenceStyle
		}
		if style == mdast.RefStyleAutolink || !micromark.reads(link) {
			continue
		}

		var msg string
		switch style {
		case mdast.RefStyleFull, mdast.RefStyleCollapsed, mdast.RefStyleShortcut:
			if definitions == nil {
				definitions = refs.MarkdownlintDefinitions(ctx.RefContext())
			}
			def := definitions[jstext.TrimSpace(link.Inline.Link.ReferenceLabel)]
			if def == nil || refs.MarkdownlintDestination(def) != "#" {
				continue
			}
			msg = "Link has empty fragment"
		default:
			switch destination := strings.TrimSpace(lint.LinkDestination(link)); destination {
			case "#":
				msg = "Link has empty fragment"
			case "":
				msg = "Link has empty destination"
			default:
				continue
			}
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
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD045")),
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

// Apply checks that Markdown images have alt text and that HTML img
// elements have an alt attribute or are hidden with aria-hidden="true".
// Like markdownlint, a Markdown image lacks alt text only when nothing at
// all is written between its brackets, so "![ ](a.png)" passes.
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

		if hasAltText(img) {
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
		construct, tag := image.construct, image.tag
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

// hasAltText reports whether something is written between the brackets of a
// Markdown image, which markdownlint counts as alt text even when it is only
// whitespace or raw HTML. Without the source range of the brackets it falls
// back to the image's text.
func hasAltText(img *mdast.Node) bool {
	label, full := img.ContentRange(), img.SourceRange()
	if label.StartOffset > full.StartOffset && label.EndOffset >= label.StartOffset && label.EndOffset < full.EndOffset {
		return label.EndOffset > label.StartOffset
	}
	return len(bytes.TrimSpace([]byte(lint.ImageAlt(img)))) > 0
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
// The sources come from htmlSources, so the file is not copied for each
// element.
func (r *ImageAltTextRule) htmlImages(ctx *lint.RuleContext) []htmlImage {
	var images []htmlImage
	sources := htmlSources{file: ctx.File}
	keep := func(source []byte, construct mdscan.HTMLConstruct, _ bool) {
		if construct.IsOpeningElement() && strings.EqualFold(construct.Name, "img") {
			// The source is valid only while it is read, so the image keeps
			// its own copy of the tag.
			images = append(images, htmlImage{construct: construct, tag: bytes.Clone(construct.Text(source))})
		}
	}
	for _, block := range ctx.HTMLBlocks() {
		eachHTMLBlockConstruct(&sources, block, keep)
	}
	for _, inline := range ctx.HTMLInlines() {
		sources.with(inline, func(source []byte) {
			if construct, ok := htmlNodeConstruct(source, inline); ok {
				keep(source, construct, false)
			}
		})
	}
	return images
}

// htmlImage is an img element and its tag, read from the source that
// htmlSources gives it, with the attributes that decide whether the image
// has alt text.
type htmlImage struct {
	construct mdscan.HTMLConstruct
	tag       []byte
}

// LinkDestinationStyleRule enforces link destination style (relative vs absolute).
type LinkDestinationStyleRule struct {
	lint.BaseRule
}

// NewLinkDestinationStyleRule creates a new link destination style rule.
func NewLinkDestinationStyleRule() *LinkDestinationStyleRule {
	return &LinkDestinationStyleRule{
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MDL001")),
	}
}

// LinkDestStyle represents the style of link destinations.
type LinkDestStyle string

const (
	// LinkDestRelative requires relative URLs.
	LinkDestRelative LinkDestStyle = "relative"
	// LinkDestAbsolute requires absolute URLs.
	LinkDestAbsolute LinkDestStyle = "absolute"
	// LinkDestConsistent uses the style of the first link that could be
	// written either way.
	LinkDestConsistent LinkDestStyle = "consistent"
)

// Apply checks the links that could be written either way, as a relative
// link or as an absolute URL: each must use the style that the style option
// names or, with "consistent", the style of the first of them. A link that
// can take only one form is never reported and sets no style (see
// destinationKind). An autolink is one: it shows its URI as its text, so it
// cannot hold a relative link. That covers email autolinks, which the parser
// gives without their mailto: scheme, and GFM autolink literals.
func (r *LinkDestinationStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil {
		return nil, nil
	}

	configStyle := LinkDestStyle(ctx.OptionString("style", string(LinkDestConsistent)))
	effectiveStyle := configStyle
	if configStyle == LinkDestConsistent {
		effectiveStyle = "" // Set from the first link that counts.
	}

	links := ctx.Links()
	var diags []lint.Diagnostic

	for _, link := range links {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if isAutolinkNode(link) {
			continue
		}
		detectedStyle, counts := destinationKind(resolvedDestination(link))
		if !counts {
			continue
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

// isAbsoluteDestination reports whether MDL001 reads a link destination as
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

// destinationKind returns the kind of the link destination dest, as the
// renderer reads it, and whether MDL001 compares the link: whether the
// destination could be written in the other kind. The kind is empty for a
// link that does not count.
//
// A relative link names a place on the document's own site, which an
// absolute URL could name too, so it counts, as an absolute path such as
// /docs/a.md does. An absolute URL could be relative only when it names a
// place on the same site. gomdlint cannot tell a URL of the document's own
// site from one of another host, so it takes every host but the local
// machine for another one, and a link to another host has no relative form.
// The only absolute URLs that count are file: URLs that name no other host
// (see isLocalFileURL): they name local files, which a relative link can
// name too. A URI with another scheme and no host, such as mailto: or urn:,
// names nothing that a relative link could. An empty destination and a link
// to a fragment of the document never count.
func destinationKind(dest string) (LinkDestStyle, bool) {
	switch {
	case dest == "" || strings.HasPrefix(dest, "#"):
		return "", false
	case !isAbsoluteDestination(dest):
		return LinkDestRelative, true
	case isLocalFileURL(dest):
		return LinkDestAbsolute, true
	default:
		return "", false
	}
}

// fileScheme is the scheme of file URLs, which matches in any case.
const fileScheme = "file:"

// isLocalFileURL reports whether dest is a file URL that names no other
// host: one without an authority, as in file:/home/a.md, or whose host is
// empty, as in file:///home/a.md, or localhost, which RFC 8089 reads as the
// local machine. Any other authority counts as another host: the server of
// file://server/share/a.md, or the drive letter of file://C:/a.md, which
// RFC 3986 reads as a host. So does an authority with a port or userinfo,
// which RFC 8089 does not allow, and an empty host followed by a second
// "//", as in file:////server/share/a.md, which RFC 8089 (Appendix E.3.2)
// reads as a UNC path that names the server after it. The host is compared
// after percent-decoding, since percent-encoded unreserved characters name
// the same host (RFC 3986, section 2.3), so file://%6Cocalhost/a.md is
// local. An encoded port or userinfo decodes to more than localhost, and a
// malformed escape fails to decode, so both still count as another host.
func isLocalFileURL(dest string) bool {
	if len(dest) < len(fileScheme) || !strings.EqualFold(dest[:len(fileScheme)], fileScheme) {
		return false
	}
	rest, hasAuthority := strings.CutPrefix(dest[len(fileScheme):], "//")
	if !hasAuthority {
		return true
	}
	authority, path := rest, ""
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		authority, path = rest[:end], rest[end:]
	}
	if authority == "" {
		return !strings.HasPrefix(path, "//")
	}
	host, err := url.PathUnescape(authority)
	return err == nil && strings.EqualFold(host, "localhost")
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
