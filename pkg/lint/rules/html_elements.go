package rules

import (
	"regexp"
	"strings"
)

// htmlTag is a start or end tag found in raw HTML.
type htmlTag struct {
	name        string
	closing     bool
	selfClosing bool
	// styled is true when the tag has a style, class or id attribute, which
	// lets CSS change how the element treats whitespace.
	styled bool
}

// htmlTagPattern matches, at the start of the text, the raw HTML that
// CommonMark recognizes: a comment, processing instruction, declaration,
// CDATA section, end tag (group 1 is its name) or start tag (group 2 is its
// name, group 3 its attributes and group 4 the slash of a self-closing tag).
var htmlTagPattern = regexp.MustCompile(`^(?:<!--[\s\S]*?-->|<\?[\s\S]*?\?>|<![A-Za-z][^>]*>|<!\[CDATA\[[\s\S]*?\]\]>|` +
	`</([A-Za-z][A-Za-z0-9-]*)\s*>|` +
	`<([A-Za-z][A-Za-z0-9-]*)((?:\s+[A-Za-z_:][A-Za-z0-9_.:-]*(?:\s*=\s*(?:[^\s"'=<>` + "`" + `]+|'[^']*'|"[^"]*"))?)*)\s*(/?)>)`)

// htmlAttributePattern matches one attribute of a start tag; group 1 is its
// name.
var htmlAttributePattern = regexp.MustCompile(`([A-Za-z_:][A-Za-z0-9_.:-]*)(?:\s*=\s*(?:[^\s"'=<>` + "`" + `]+|'[^']*'|"[^"]*"))?`)

// scanHTMLTags returns the start and end tags in raw HTML, in order. The
// content of a raw text element such as a script is text, so the tags in it
// are skipped.
func scanHTMLTags(raw string) []htmlTag {
	var tags []htmlTag
	for pos := 0; pos < len(raw); {
		next := strings.IndexByte(raw[pos:], '<')
		if next < 0 {
			break
		}
		pos += next
		match := htmlTagPattern.FindStringSubmatchIndex(raw[pos:])
		if match == nil {
			pos++
			continue
		}
		end := pos + match[1]
		switch {
		case match[2] >= 0:
			tags = append(tags, htmlTag{name: strings.ToLower(raw[pos+match[2] : pos+match[3]]), closing: true})
		case match[4] >= 0:
			tag := htmlTag{name: strings.ToLower(raw[pos+match[4] : pos+match[5]]), selfClosing: match[9] > match[8]}
			for _, attribute := range htmlAttributePattern.FindAllStringSubmatch(raw[pos+match[6]:pos+match[7]], -1) {
				switch strings.ToLower(attribute[1]) {
				case "style", "class", "id":
					tag.styled = true
				}
			}
			tags = append(tags, tag)
			if isRawTextElement(tag.name) && !tag.selfClosing {
				closing := strings.Index(strings.ToLower(raw[end:]), "</"+tag.name)
				if closing < 0 {
					return tags
				}
				end += closing
			}
		default:
		}
		pos = end
	}
	return tags
}

// openElement is an HTML element that raw HTML opened and did not close yet.
type openElement struct {
	name string
	// exact is true when the element may keep the whitespace of its text.
	exact bool
	// inline is true when inline raw HTML opened the element.
	inline bool
	// rawText is true for an element such as a textarea, whose content runs
	// to its end tag whatever comes between.
	rawText bool
}

// htmlElements tracks the HTML elements that raw HTML opened, in document
// order.
type htmlElements struct {
	open []openElement
}

// apply updates the open elements with a tag. An element that inline raw
// HTML opens keeps its text exact, because CSS can make any element keep
// whitespace. One that an HTML block opens around Markdown keeps its text
// exact unless it is a known element that collapses whitespace and has no
// style, class or id. An end tag closes the innermost element of its name
// and the elements inside it.
func (e *htmlElements) apply(tag htmlTag, inline bool) {
	if tag.closing {
		for idx := len(e.open) - 1; idx >= 0; idx-- {
			if e.open[idx].name == tag.name {
				e.open = e.open[:idx]
				return
			}
		}
		return
	}
	if tag.selfClosing || isVoidElement(tag.name) {
		return
	}
	rawText := isRawTextElement(tag.name)
	e.open = append(e.open, openElement{
		name:    tag.name,
		exact:   inline || rawText || tag.styled || !isCollapsingElement(tag.name),
		inline:  inline,
		rawText: rawText,
	})
}

// exact reports whether text at this point may keep its whitespace when it
// renders.
func (e *htmlElements) exact() bool {
	for _, element := range e.open {
		if element.exact {
			return true
		}
	}
	return false
}

// closeInline closes the elements that inline HTML opened after the first
// depth elements, as the end of a paragraph does, except raw text elements,
// which take all the HTML that follows them.
func (e *htmlElements) closeInline(depth int) {
	kept := e.open[:depth]
	for _, element := range e.open[depth:] {
		if element.rawText || !element.inline {
			kept = append(kept, element)
		}
	}
	e.open = kept
}

// isVoidElement reports whether an element has no content or end tag.
func isVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "keygen", "link", "meta", "param", "source",
		"track", "wbr":
		return true
	default:
		return false
	}
}

// isRawTextElement reports whether an element holds raw text up to its end
// tag.
func isRawTextElement(name string) bool {
	switch name {
	case "script", "style", "textarea", "title", "xmp", "plaintext", "iframe", "noembed", "noframes", "noscript":
		return true
	default:
		return false
	}
}

// isCollapsingElement reports whether browsers collapse the whitespace of an
// element's text by default. Preformatted and code elements do not, and the
// handling of unknown elements cannot be established.
func isCollapsingElement(name string) bool {
	switch name {
	case "a", "abbr", "acronym", "address", "article", "aside", "b", "bdi", "bdo", "big", "blockquote", "caption",
		"center", "cite", "data", "dd", "del", "details", "dfn", "div", "dl", "dt", "em", "fieldset", "figcaption",
		"figure", "font", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "i", "ins", "label",
		"legend", "li", "main", "mark", "nav", "ol", "p", "picture", "q", "rp", "rt", "ruby", "s", "section", "small",
		"span", "strike", "strong", "sub", "summary", "sup", "table", "tbody", "td", "tfoot", "th", "thead", "time",
		"tr", "u", "ul":
		return true
	default:
		return false
	}
}
