package refs

import (
	"bytes"
	"regexp"
)

// jsSpace is the character class body of a JavaScript regular expression's
// \s: tab, line feed, vertical tab, form feed, carriage return, space, the
// space separators, the line and paragraph separators and U+FEFF.
const jsSpace = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`

// htmlTagNamePattern matches the start of an HTML tag and its name as
// markdownlint's getHtmlTagInfo reads them: "<", then a name that does not
// start with "!" or ">" and runs up to "/", white space or ">". A closing
// tag's name starts with "/".
var htmlTagNamePattern = regexp.MustCompile(`^<([^!>][^/` + jsSpace + `>]*)`)

// htmlIDPattern and htmlNamePattern match an id or a name attribute as
// markdownlint's getHtmlAttributeRe builds the pattern (see
// htmlAttributeExpr).
var (
	htmlIDPattern   = regexp.MustCompile(htmlAttributeExpr("id"))
	htmlNamePattern = regexp.MustCompile(htmlAttributeExpr("name"))
)

// htmlAttributeExpr returns the expression that markdownlint's
// getHtmlAttributeRe builds for an attribute: white space, the attribute's
// name in any case, "=" with optional white space around it, an optional
// quote, and the value up to a quote, white space or ">". It matches
// anywhere in a tag's text, inside another attribute's quoted value too.
func htmlAttributeExpr(name string) string {
	return `(?i)[` + jsSpace + `]` + name + `[` + jsSpace + `]*=[` + jsSpace + `]*['"]?([^'"` + jsSpace + `>]*)`
}

// HTMLAnchor returns the anchor that an HTML tag names and where the
// anchor comes from, as markdownlint 0.41.1 reads it for MD051. tag is the
// tag's source, from its "<", over all of its lines. An opening tag names
// the value of its first id attribute; an "a" tag without one names the
// value of its first name attribute. Both are matched in the tag's text as
// markdownlint matches them, so the value may be quoted or not. A closing
// tag, a comment or a declaration names no anchor, and neither does an
// empty value, which still keeps an "a" tag's name from counting.
func HTMLAnchor(tag []byte) (string, AnchorSource, bool) {
	name := htmlTagNamePattern.FindSubmatch(tag)
	if name == nil || name[1][0] == '/' {
		return "", 0, false
	}
	match, source := htmlIDPattern.FindSubmatch(tag), AnchorFromHTMLID
	if match == nil && bytes.EqualFold(name[1], []byte("a")) {
		match, source = htmlNamePattern.FindSubmatch(tag), AnchorFromHTMLName
	}
	if match == nil || len(match[1]) == 0 {
		return "", 0, false
	}
	return string(match[1]), source, true
}
