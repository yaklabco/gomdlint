package rules

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// htmlConstruct is a CommonMark raw HTML construct (open tag, closing tag,
// comment, processing instruction, declaration, or CDATA section) located in
// source text.
type htmlConstruct struct {
	// Start and End are absolute byte offsets of the construct.
	Start, End int

	// Name is the element name in the form markdownlint reports it: the tag
	// name for opening and closing tags, "?target" for processing
	// instructions, and empty for comments, declarations, and CDATA sections.
	Name string

	// Closing is true for closing tags.
	Closing bool
}

// isOpeningElement reports whether the construct opens an element.
func (c htmlConstruct) isOpeningElement() bool {
	return c.Name != "" && !c.Closing
}

// text returns the construct's source bytes.
func (c htmlConstruct) text(content []byte) []byte {
	return content[c.Start:c.End]
}

// scanHTMLConstructs finds the raw HTML constructs in content[start:end].
// Text between constructs, including a '<' that does not begin a valid
// construct, is skipped. A comment, processing instruction, declaration, or
// CDATA section that begins at start may end beyond end: the parser reports
// the closing line of such HTML blocks separately from the block's lines.
func scanHTMLConstructs(content []byte, start, end int) []htmlConstruct {
	if start < 0 || end > len(content) || start >= end {
		return nil
	}
	var constructs []htmlConstruct
	for idx := start; idx < end; idx++ {
		if content[idx] != '<' {
			continue
		}
		limit := end
		if idx == start {
			limit = len(content)
		}
		construct, ok := matchHTMLConstruct(content[:limit], idx)
		if !ok {
			continue
		}
		constructs = append(constructs, construct)
		idx = construct.End - 1
	}
	return constructs
}

// htmlNodeConstruct returns the single raw HTML construct of an inline HTML node.
func htmlNodeConstruct(node *mdast.Node) (htmlConstruct, bool) {
	if node == nil || node.File == nil {
		return htmlConstruct{}, false
	}
	span := node.SourceRange()
	if span.StartOffset < 0 || span.EndOffset > len(node.File.Content) || span.IsEmpty() {
		return htmlConstruct{}, false
	}
	return matchHTMLConstruct(node.File.Content[:span.EndOffset], span.StartOffset)
}

// matchHTMLConstruct matches one raw HTML construct starting at src[pos],
// following the CommonMark raw HTML grammar.
func matchHTMLConstruct(src []byte, pos int) (htmlConstruct, bool) {
	if pos+1 >= len(src) || src[pos] != '<' {
		return htmlConstruct{}, false
	}
	rest := src[pos+1:]
	switch {
	case bytes.HasPrefix(rest, []byte("!--")):
		return matchHTMLComment(rest, pos)
	case bytes.HasPrefix(rest, []byte("![CDATA[")):
		return matchHTMLTerminated(rest, pos, len("![CDATA["), "]]>", "")
	case rest[0] == '!' && len(rest) > 1 && isASCIILetter(rest[1]):
		return matchHTMLTerminated(rest, pos, 1, ">", "")
	case rest[0] == '?':
		return matchHTMLTerminated(rest, pos, 1, "?>", processingInstructionName(rest))
	case rest[0] == '/':
		return matchHTMLClosingTag(rest, pos)
	default:
		return matchHTMLOpenTag(rest, pos)
	}
}

// matchHTMLComment matches "<!-->", "<!--->", or "<!--" ... "-->".
func matchHTMLComment(rest []byte, pos int) (htmlConstruct, bool) {
	body := rest[len("!--"):]
	switch {
	case bytes.HasPrefix(body, []byte(">")):
		return htmlConstruct{Start: pos, End: pos + len("<!-->")}, true
	case bytes.HasPrefix(body, []byte("->")):
		return htmlConstruct{Start: pos, End: pos + len("<!--->")}, true
	}
	idx := bytes.Index(body, []byte("-->"))
	if idx < 0 {
		return htmlConstruct{}, false
	}
	return htmlConstruct{Start: pos, End: pos + 1 + len("!--") + idx + len("-->")}, true
}

// matchHTMLTerminated matches a construct that runs from its opener to the
// first occurrence of terminator.
func matchHTMLTerminated(rest []byte, pos, openerLen int, terminator, name string) (htmlConstruct, bool) {
	idx := bytes.Index(rest[openerLen:], []byte(terminator))
	if idx < 0 {
		return htmlConstruct{}, false
	}
	return htmlConstruct{Start: pos, End: pos + 1 + openerLen + idx + len(terminator), Name: name}, true
}

// processingInstructionName returns the name markdownlint derives from a
// processing instruction: "?" followed by everything up to whitespace, '/', or '>'.
func processingInstructionName(rest []byte) string {
	end := 1
	for end < len(rest) && !isHTMLSpace(rest[end]) && rest[end] != '/' && rest[end] != '>' {
		end++
	}
	return string(rest[:end])
}

// matchHTMLClosingTag matches "</" tagname optional-whitespace ">".
func matchHTMLClosingTag(rest []byte, pos int) (htmlConstruct, bool) {
	idx := 1
	nameEnd, ok := scanHTMLTagName(rest, idx)
	if !ok {
		return htmlConstruct{}, false
	}
	idx = skipHTMLSpace(rest, nameEnd)
	if idx >= len(rest) || rest[idx] != '>' {
		return htmlConstruct{}, false
	}
	return htmlConstruct{Start: pos, End: pos + 1 + idx + 1, Name: string(rest[1:nameEnd]), Closing: true}, true
}

// matchHTMLOpenTag matches "<" tagname attribute* optional-whitespace "/"? ">".
func matchHTMLOpenTag(rest []byte, pos int) (htmlConstruct, bool) {
	nameEnd, ok := scanHTMLTagName(rest, 0)
	if !ok {
		return htmlConstruct{}, false
	}
	idx := nameEnd
	for {
		next, matched := scanHTMLAttribute(rest, idx)
		if !matched {
			break
		}
		idx = next
	}
	idx = skipHTMLSpace(rest, idx)
	if idx < len(rest) && rest[idx] == '/' {
		idx++
	}
	if idx >= len(rest) || rest[idx] != '>' {
		return htmlConstruct{}, false
	}
	return htmlConstruct{Start: pos, End: pos + 1 + idx + 1, Name: string(rest[:nameEnd])}, true
}

// scanHTMLTagName scans an ASCII letter followed by letters, digits, or hyphens.
func scanHTMLTagName(src []byte, idx int) (int, bool) {
	if idx >= len(src) || !isASCIILetter(src[idx]) {
		return idx, false
	}
	idx++
	for idx < len(src) && (isASCIILetter(src[idx]) || isASCIIDigit(src[idx]) || src[idx] == '-') {
		idx++
	}
	return idx, true
}

// scanHTMLAttribute scans whitespace, an attribute name, and an optional value specification.
func scanHTMLAttribute(src []byte, idx int) (int, bool) {
	nameStart := skipHTMLSpace(src, idx)
	if nameStart == idx || nameStart >= len(src) {
		return idx, false
	}
	first := src[nameStart]
	if !isASCIILetter(first) && first != '_' && first != ':' {
		return idx, false
	}
	nameEnd := nameStart + 1
	for nameEnd < len(src) && isHTMLAttributeNameChar(src[nameEnd]) {
		nameEnd++
	}
	equals := skipHTMLSpace(src, nameEnd)
	if equals >= len(src) || src[equals] != '=' {
		return nameEnd, true
	}
	valueStart := skipHTMLSpace(src, equals+1)
	valueEnd, ok := scanHTMLAttributeValue(src, valueStart)
	if !ok {
		return idx, false
	}
	return valueEnd, true
}

// scanHTMLAttributeValue scans an unquoted, single-quoted, or double-quoted value.
func scanHTMLAttributeValue(src []byte, idx int) (int, bool) {
	if idx >= len(src) {
		return idx, false
	}
	if quote := src[idx]; quote == '"' || quote == '\'' {
		closing := bytes.IndexByte(src[idx+1:], quote)
		if closing < 0 {
			return idx, false
		}
		return idx + 1 + closing + 1, true
	}
	end := idx
	for end < len(src) && !isHTMLSpace(src[end]) && !strings.ContainsRune("\"'=<>`", rune(src[end])) {
		end++
	}
	return end, end > idx
}

func isHTMLAttributeNameChar(ch byte) bool {
	return isASCIILetter(ch) || isASCIIDigit(ch) || ch == '_' || ch == '.' || ch == ':' || ch == '-'
}

// htmlAttributePattern returns markdownlint's getHtmlAttributeRe pattern for an attribute.
func htmlAttributePattern(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\s` + regexp.QuoteMeta(name) + `\s*=\s*['"]?([^'"\s>]*)`)
}
