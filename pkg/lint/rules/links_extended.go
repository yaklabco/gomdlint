package rules

import (
	"bytes"
	"regexp"
	"strings"

	gmutil "github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
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

// bareURL is an autolink literal found in inline text.
type bareURL struct {
	start, end int
	kind       mdscan.LiteralKind
	// fixable reports whether the parser read the literal the way GFM does
	// (as text or as one autolink with the same span) and no scheme before
	// it joins the address to a longer link, so the fix check can see what
	// angle brackets do to it.
	fixable bool
}

// Apply reports bare URLs and email addresses the way markdownlint does: the
// GFM autolink literals that a port of micromark's grammar finds in inline
// text, with trailing punctuation and unbalanced parentheses left out. URLs in
// link text, code spans, autolinks, raw HTML, the content of inline HTML
// elements, HTML blocks, code blocks, and reference definitions are not bare.
// The fix wraps a URL or email address in angle brackets. The fix check sees
// only what the parser sees, so the fix is attached only where the parser read
// the literal the same way: as text, which it is with CommonMark, or as one
// autolink with the same span, which it usually is with GFM. goldmark ends
// some literals elsewhere, before a backslash or after an unbalanced ")"
// that punctuation follows, and such literals are reported without a fix. So
// are "www." addresses, which without a scheme would not become autolinks and
// would show the brackets, and addresses after mailto: or xmpp:, which GitHub
// links together with the scheme.
func (r *NoBareURLsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var urls []bareURL
	for _, container := range mdast.FindAll(ctx.Root, isTextContainer) {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		scan := newBareURLScan(ctx.File, container)
		scan.visit(container)
		urls = append(urls, scan.found...)
	}

	diags := make([]lint.Diagnostic, 0, len(urls))
	for _, url := range urls {
		startLine, startColumn := ctx.File.LineAt(url.start)
		endLine, endColumn := ctx.File.LineAt(url.end)
		pos := mdast.SourcePosition{StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn}

		literal := ctx.File.Content[url.start:url.end]
		msg, suggestion := "Bare URL used", "Wrap the URL/email in angle brackets"
		safe := angleBracketsKeepLiteral(url.kind, literal)
		switch {
		case url.kind == mdscan.LiteralWWW:
			// Angle brackets only make an autolink with a scheme, so there is no safe fix.
			suggestion = "Write the address with a scheme in angle brackets, such as <https://www.example.com>"
		case url.kind == mdscan.LiteralEmail && !safe:
			msg, suggestion = "Bare email address used", "Write the address as a link, such as [text](mailto:...)"
		case url.kind == mdscan.LiteralEmail:
			msg = "Bare email address used"
		case !safe:
			suggestion = "Write the URL as a link with its <, >, and backslash characters percent-encoded"
		}
		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(suggestion)
		if safe && url.fixable {
			builder := fix.NewEditBuilder()
			builder.ReplaceRange(url.start, url.end, "<"+string(literal)+">")
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}
	sortDiagnosticsByPosition(diags)
	return verifyFixes(ctx, diags, fixAllowance{autolinks: true}), nil
}

// emailAutolinkPattern is the CommonMark grammar for the address inside an
// email autolink, which is stricter than a GFM email literal: its domain
// labels hold only letters, digits, and inner hyphens.
var emailAutolinkPattern = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@" +
	`[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// angleBracketsKeepLiteral reports whether wrapping an autolink literal in
// angle brackets makes an autolink to the same address. A URI autolink
// cannot hold '<', '>', spaces, or ASCII control characters, and reads a
// backslash or an entity reference as written where the literal's text read
// it as an escape or a character (GitHub decodes "&copy;" in a GFM literal
// too). A "www." literal has no scheme, so brackets never make it an
// autolink.
func angleBracketsKeepLiteral(kind mdscan.LiteralKind, literal []byte) bool {
	switch kind {
	case mdscan.LiteralURL:
		return !bytes.ContainsAny(literal, "<>\\ ") && !bytes.ContainsFunc(literal, isASCIIControl) &&
			bytes.Equal(renderedText(literal), gmutil.EscapeHTML(literal))
	case mdscan.LiteralEmail:
		return emailAutolinkPattern.Match(literal)
	case mdscan.LiteralNone, mdscan.LiteralWWW:
	}
	return false
}

// isASCIIControl reports whether r is an ASCII control character.
func isASCIIControl(r rune) bool {
	return r < ' ' || r == 0x7f
}

// isTextContainer reports whether a block's children are inline content: a
// paragraph, heading, or table cell. Link and image text is never searched.
func isTextContainer(node *mdast.Node) bool {
	if node.IsInline() {
		return false
	}
	for child := node.FirstChild; child != nil; child = child.Next {
		if child.IsInline() {
			return true
		}
	}
	return false
}

// bareURLScan walks one container's inline nodes in order, tracking the
// unclosed '[' that suppress autolink literals, like micromark.
type bareURLScan struct {
	file    *mdast.FileSnapshot
	content []byte
	// textStart is where the document's text starts: after a byte order
	// mark, which the parser skips, so nothing comes before a literal there.
	textStart int
	// limit is where the container's inline content ends. A literal can run
	// on past the text it starts in, over markup the parser read there.
	limit int
	// consumed is where the last literal ended; no literal starts before it.
	consumed int
	// parsed holds the spans of the autolinks the parser made from literals.
	parsed   []mdast.SourceRange
	brackets int
	found    []bareURL
}

// newBareURLScan prepares a scan of a container's inline content.
func newBareURLScan(file *mdast.FileSnapshot, container *mdast.Node) *bareURLScan {
	scan := &bareURLScan{file: file, content: file.Content, textStart: lint.LineTextOffset(file, 1)}
	if last := container.LastChild; last != nil {
		scan.limit = last.SourceRange().EndOffset
	}
	_ = mdast.Walk(container, func(node *mdast.Node) error { //nolint:errcheck // visitor never returns error
		if isLinkifiedAutolink(file.Content, node) {
			scan.parsed = append(scan.parsed, node.SourceRange())
		}
		return nil
	})
	return scan
}

func (s *bareURLScan) visit(parent *mdast.Node) {
	children := parent.Children()
	for idx := 0; idx < len(children); idx++ {
		child := children[idx]
		switch {
		case s.isLiteralText(child):
			last := idx
			for last+1 < len(children) && s.isLiteralText(children[last+1]) &&
				children[last+1].SourceRange().StartOffset == children[last].SourceRange().EndOffset {
				last++
			}
			s.scanText(children[idx].SourceRange().StartOffset, children[last].SourceRange().EndOffset)
			idx = last
		case child.Kind == mdast.NodeEmphasis || child.Kind == mdast.NodeStrong || child.Kind == mdast.NodeStrikethrough:
			s.visit(child)
		case child.Kind == mdast.NodeHTMLInline:
			idx = skipHTMLElementContent(s.file, children, idx)
		}
	}
}

// isLiteralText reports whether a node's source is text in which the GFM
// grammar looks for literals: a text node, or an autolink the parser made
// from a literal. Adjacent ones are searched together, so a literal the
// parser ended early or late is still read whole.
func (s *bareURLScan) isLiteralText(node *mdast.Node) bool {
	return (node.Kind == mdast.NodeText && node.Inline != nil) || isLinkifiedAutolink(s.content, node)
}

// isLinkifiedAutolink reports whether a node is an autolink the parser made
// from a GFM literal rather than one written in angle brackets.
func isLinkifiedAutolink(content []byte, node *mdast.Node) bool {
	span := node.SourceRange()
	return isAutolinkNode(node) && !span.IsEmpty() && content[span.StartOffset] != '<'
}

// skipHTMLElementContent returns the index of the closing tag that matches the
// opening tag at children[idx], or idx when there is none, so the element's
// content is skipped as markdownlint does.
func skipHTMLElementContent(file *mdast.FileSnapshot, children []*mdast.Node, idx int) int {
	open, ok := htmlNodeConstruct(htmlSource(file, children[idx]), children[idx])
	if !ok || !open.IsOpeningElement() {
		return idx
	}
	depth := 1
	for next := idx + 1; next < len(children); next++ {
		if children[next].Kind != mdast.NodeHTMLInline {
			continue
		}
		tag, ok := htmlNodeConstruct(htmlSource(file, children[next]), children[next])
		if !ok || !strings.EqualFold(tag.Name, open.Name) {
			continue
		}
		if !tag.Closing {
			depth++
			continue
		}
		depth--
		if depth == 0 {
			return next
		}
	}
	return idx
}

// scanText searches the text content[start:end] for autolink literals,
// skipping backslash escapes and counting brackets. A literal may run on past
// the text, over markup the parser read there; the parser then read it
// differently, so it gets no fix.
func (s *bareURLScan) scanText(start, end int) {
	text := s.content[:max(s.limit, end)]
	for pos := max(start, s.consumed); pos < end; pos++ {
		switch ch := text[pos]; {
		case ch == '\\' && pos+1 < end && mdscan.IsASCIIPunct(text[pos+1]):
			pos++
			continue
		case ch == '[':
			s.brackets++
			continue
		case ch == ']':
			if s.brackets > 0 {
				s.brackets--
			}
			continue
		}
		if s.brackets > 0 {
			continue
		}
		var prev byte
		if pos > s.textStart {
			prev = s.content[pos-1]
		}
		literalEnd, kind := mdscan.MatchAutolinkLiteral(text, pos, prev)
		if kind == mdscan.LiteralNone {
			continue
		}
		fixable := literalEnd <= end && s.parsedAgrees(pos, literalEnd) &&
			(kind != mdscan.LiteralEmail || !followsLinkScheme(s.content[:pos]))
		s.add(pos, literalEnd, kind, fixable)
		s.consumed = literalEnd
		pos = literalEnd - 1
	}
}

// parsedAgrees reports whether the parser read content[start:end] the way the
// GFM grammar did: no autolink the parser made overlaps it, or one has
// exactly its span.
func (s *bareURLScan) parsedAgrees(start, end int) bool {
	for _, span := range s.parsed {
		overlaps := span.StartOffset < end && start < span.EndOffset
		if overlaps && (span.StartOffset != start || span.EndOffset != end) {
			return false
		}
	}
	return true
}

// followsLinkScheme reports whether text ends with "mailto:" or "xmpp:",
// which GFM as GitHub renders it links together with the address after it.
func followsLinkScheme(text []byte) bool {
	tail := strings.ToLower(string(text[max(0, len(text)-len("mailto:")):]))
	return strings.HasSuffix(tail, "mailto:") || strings.HasSuffix(tail, "xmpp:")
}

// add records a literal unless it sits between '<' and '>', which markdownlint
// ignores (micromark issue 164).
func (s *bareURLScan) add(start, end int, kind mdscan.LiteralKind, fixable bool) {
	if start > 0 && end < len(s.content) && s.content[start-1] == '<' && s.content[end] == '>' {
		return
	}
	s.found = append(s.found, bareURL{start: start, end: end, kind: kind, fixable: fixable})
}
