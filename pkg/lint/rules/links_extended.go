package rules

import (
	"bytes"
	"regexp"
	"strings"

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
}

// Apply reports bare URLs and email addresses the way markdownlint does: the
// GFM autolink literals in inline text, with trailing punctuation and
// unbalanced parentheses left out. URLs in link text, code spans, autolinks,
// raw HTML, the content of inline HTML elements, HTML blocks, code blocks, and
// reference definitions are not bare. With the GFM flavor the parser has
// already turned these literals into autolinks; with CommonMark they are text.
// The fix wraps a URL or email address in angle brackets; a "www." address is
// reported without a fix, because without a scheme the brackets would not
// make an autolink and would show in the rendered text.
func (r *NoBareURLsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var urls []bareURL
	for _, container := range mdast.FindAll(ctx.Root, isTextContainer) {
		if ctx.Cancelled() {
			return nil, ctx.Ctx.Err()
		}
		scan := &bareURLScan{file: ctx.File, content: ctx.File.Content}
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
		if safe {
			builder := fix.NewEditBuilder()
			builder.ReplaceRange(url.start, url.end, "<"+string(literal)+">")
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}
	sortDiagnosticsByPosition(diags)
	return diags, nil
}

// emailAutolinkPattern is the CommonMark grammar for the address inside an
// email autolink, which is stricter than a GFM email literal: its domain
// labels hold only letters, digits, and inner hyphens.
var emailAutolinkPattern = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@" +
	`[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// angleBracketsKeepLiteral reports whether wrapping an autolink literal in
// angle brackets makes an autolink to the same address. A URI autolink
// cannot hold '<', '>', spaces, or ASCII control characters, and reads a
// backslash as part of the address where the literal's text read it as an
// escape. A "www." literal has no scheme, so brackets never make it an
// autolink.
func angleBracketsKeepLiteral(kind mdscan.LiteralKind, literal []byte) bool {
	switch kind {
	case mdscan.LiteralURL:
		return !bytes.ContainsAny(literal, "<>\\ ") && !bytes.ContainsFunc(literal, isASCIIControl)
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
	file     *mdast.FileSnapshot
	content  []byte
	brackets int
	found    []bareURL
}

func (s *bareURLScan) visit(parent *mdast.Node) {
	children := parent.Children()
	for idx := 0; idx < len(children); idx++ {
		child := children[idx]
		switch {
		case child.Kind == mdast.NodeText && child.Inline != nil:
			last := idx
			for last+1 < len(children) && isContiguousText(children[last], children[last+1]) {
				last++
			}
			s.scanText(children[idx].SourceRange().StartOffset, children[last].SourceRange().EndOffset)
			idx = last
		case child.Kind == mdast.NodeEmphasis || child.Kind == mdast.NodeStrong || child.Kind == mdast.NodeStrikethrough:
			s.visit(child)
		case child.Kind == mdast.NodeHTMLInline:
			idx = skipHTMLElementContent(s.file, children, idx)
		case isAutolinkNode(child):
			s.checkParsedLiteral(child)
		}
	}
}

// isContiguousText reports whether two siblings are text nodes that are
// adjacent in the source, so a URL may continue from one into the next.
func isContiguousText(prev, next *mdast.Node) bool {
	return next.Kind == mdast.NodeText && next.Inline != nil &&
		next.SourceRange().StartOffset == prev.SourceRange().EndOffset
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

// scanText searches content[start:end] for autolink literals, skipping
// backslash escapes and counting brackets.
func (s *bareURLScan) scanText(start, end int) {
	text := s.content[:end]
	for pos := start; pos < end; pos++ {
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
		if pos > 0 {
			prev = s.content[pos-1]
		}
		literalEnd, kind := mdscan.MatchAutolinkLiteral(text, pos, prev)
		if kind == mdscan.LiteralNone {
			continue
		}
		s.add(pos, literalEnd, kind)
		pos = literalEnd - 1
	}
}

// checkParsedLiteral reports an autolink the parser created from a GFM
// autolink literal: an http or https URL, a "www." address, or an email
// address. Autolinks written in angle brackets and other schemes are not
// reported.
func (s *bareURLScan) checkParsedLiteral(node *mdast.Node) {
	span := node.SourceRange()
	if s.brackets > 0 || span.IsEmpty() || s.content[span.StartOffset] == '<' {
		return
	}
	text := strings.ToLower(string(s.content[span.StartOffset:span.EndOffset]))
	switch {
	case strings.HasPrefix(text, "http://"), strings.HasPrefix(text, "https://"):
		s.add(span.StartOffset, span.EndOffset, mdscan.LiteralURL)
	case strings.HasPrefix(text, "www."):
		s.add(span.StartOffset, span.EndOffset, mdscan.LiteralWWW)
	case !strings.Contains(text, "://") && strings.Contains(text, "@"):
		s.add(span.StartOffset, span.EndOffset, mdscan.LiteralEmail)
	}
}

// add records a literal unless it sits between '<' and '>', which markdownlint
// ignores (micromark issue 164).
func (s *bareURLScan) add(start, end int, kind mdscan.LiteralKind) {
	if start > 0 && end < len(s.content) && s.content[start-1] == '<' && s.content[end] == '>' {
		return
	}
	s.found = append(s.found, bareURL{start: start, end: end, kind: kind})
}
