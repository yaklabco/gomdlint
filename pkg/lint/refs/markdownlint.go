package refs

import (
	"strings"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/internal/jstext"
)

// MarkdownlintKey returns the key under which markdownlint 0.41.1 records a
// link reference definition (normalizeReference in its helpers): the label
// lowercased as JavaScript lowercases it, trimmed as JavaScript trims it, and
// with each run of white space made one space. It folds no case, unlike
// NormalizeLabel, so "Straße" and "strasse" keep different keys.
//
// markdownlint's MD042 and MD054 look a reference up in these keys with the
// label as written, not normalized, so a label they find must already be in
// this form.
func MarkdownlintKey(label string) string {
	fields := strings.FieldsFunc(jsToLower(label), jstext.IsSpace)
	return strings.Join(fields, " ")
}

// MicromarkLabel reports whether micromark, which markdownlint parses with,
// reads label as a link label: it holds a character other than a space, a
// tab or a line ending. goldmark resolves a reference whose label holds
// nothing else, such as "[ ]" or "[x][ ]", when a definition's label is
// nothing but white space with form feeds or vertical tabs, as in
// "[\f]: /url", and micromark makes no link of it.
func MicromarkLabel(label string) bool {
	return strings.Trim(label, " \t\r\n") != ""
}

// FootnoteText returns the definitions of ctx that micromark, which
// markdownlint parses with, reads as the text of a GFM footnote: each one
// after a footnote definition whose text starts a paragraph (see
// startsParagraph), such as "[^1]: note", in the paragraph that
// goldmark reads the two from. The footnote holds a paragraph, which takes
// the lines below it until a blank line or the start of another block, and
// a link reference definition cannot interrupt a paragraph. goldmark, which
// reads no footnotes, reads them as definitions. Only a footnote whose label
// is plain counts (see PlainFootnoteLabel), so that a label micromark may
// not read as a footnote keeps goldmark's reading.
func FootnoteText(ctx *Context) map[*ReferenceDefinition]bool {
	var text map[*ReferenceDefinition]bool
	paragraph, inFootnote := 0, false
	for _, def := range ctx.AllDefinitions {
		if def.paragraph != paragraph || def.paragraph == 0 {
			paragraph, inFootnote = def.paragraph, false
		}
		if footnote, ok := strings.CutPrefix(def.Label, "^"); ok && PlainFootnoteLabel(footnote) {
			inFootnote = startsParagraph(def)
			continue
		}
		if inFootnote {
			if text == nil {
				text = make(map[*ReferenceDefinition]bool)
			}
			text[def] = true
		}
	}
	return text
}

// startsParagraph reports whether the text of def, a footnote definition
// that goldmark reads as a link reference definition, starts a paragraph,
// which takes the lines below it, rather than another block: an ATX
// heading such as "[^1]: #", a thematic break, a list item, a fence, a
// blockquote, HTML, math or a directive (see mayBeDirective). goldmark
// reads the text as a destination and an optional title, so the
// destination is the text's first word. A directive has nothing after it
// on its line, so a title on the same line makes the word none; a title on
// a later line may stand under a container directive. Text that may start
// another block, such as a "<" or "[" at its start, counts as none, so that
// the definitions below keep goldmark's reading.
func startsParagraph(def *ReferenceDefinition) bool {
	word := def.Destination
	if def.AngleBrackets || word == "" {
		return false
	}
	switch {
	case strings.Trim(word, "#") == "" && len(word) <= maxATXLevel,
		def.Title == "" && len(word) >= minThematicBreak && strings.Count(word, word[:1]) == len(word) &&
			strings.ContainsAny(word[:1], "*-_"),
		word == "-" || word == "+" || word == "*",
		orderedListMarker(word),
		strings.HasPrefix(word, "```") || strings.HasPrefix(word, "~~~"),
		strings.HasPrefix(word, "$$"),
		(def.Title == "" || def.Lines[0] != def.Lines[1]) && mayBeDirective(word),
		strings.ContainsAny(word[:1], "><["):
		return false
	}
	return true
}

// mayBeDirective reports whether word, a whole line, may be a leaf or
// container directive to micromark's directive extension: two or more
// colons, a name, and an optional label in brackets and attributes in
// braces. A name starts with a character other than ASCII punctuation, goes
// on with letters, digits, "-" and "_", and does not end with "-" or "_".
// Where micromark's rules need more, this errs toward a directive, so that
// a line micromark might read as one keeps goldmark's reading: a character
// outside ASCII counts as a letter, and what follows the name counts when
// it starts a label or attributes that the word closes, without their
// content told apart.
func mayBeDirective(word string) bool {
	rest := strings.TrimLeft(word, ":")
	if len(word)-len(rest) < len("::") {
		return false
	}
	name := 0
	for name < len(rest) {
		char := rest[name]
		if !isASCIIDigit(char) && (char|0x20 < 'a' || char|0x20 > 'z') && char < utf8.RuneSelf &&
			(name == 0 || (char != '-' && char != '_')) {
			break
		}
		name++
	}
	if name == 0 || rest[name-1] == '-' || rest[name-1] == '_' {
		return false
	}
	switch rest = rest[name:]; {
	case rest == "":
		return true
	case rest[0] == '[':
		return strings.Contains(rest, "]")
	case rest[0] == '{':
		return strings.Contains(rest, "}")
	}
	return false
}

// Limits of the blocks startsParagraph tells apart: the deepest ATX
// heading and the shortest thematic break.
const (
	maxATXLevel      = 6
	minThematicBreak = 3
)

// maxOrderedMarkerDigits is CommonMark's limit on the digits of an ordered
// list item's number.
const maxOrderedMarkerDigits = 9

// orderedListMarker reports whether word is an ordered list item marker:
// one to nine digits and "." or ")".
func orderedListMarker(word string) bool {
	digits := word[:len(word)-1]
	if last := word[len(word)-1]; (last != '.' && last != ')') || digits == "" || len(digits) > maxOrderedMarkerDigits {
		return false
	}
	for idx := range len(digits) {
		if !isASCIIDigit(digits[idx]) {
			return false
		}
	}
	return true
}

// PlainFootnoteLabel reports whether a footnote label, after its "^", is
// not empty and holds nothing but ASCII letters, digits and hyphens, which
// micromark, markdownlint and goldmark all read as written.
func PlainFootnoteLabel(label string) bool {
	if label == "" {
		return false
	}
	for idx := range len(label) {
		char := label[idx]
		if char != '-' && !isASCIIDigit(char) && (char|0x20 < 'a' || char|0x20 > 'z') {
			return false
		}
	}
	return true
}

// isASCIIDigit reports whether ch is an ASCII digit.
func isASCIIDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// MarkdownlintDefinitions returns the link reference definitions of ctx as
// markdownlint records them: keyed by MarkdownlintKey, the first definition
// of each key, leaving out those micromark reads as footnote text (see
// FootnoteText).
func MarkdownlintDefinitions(ctx *Context) map[string]*ReferenceDefinition {
	definitions := make(map[string]*ReferenceDefinition, len(ctx.AllDefinitions))
	footnoteText := FootnoteText(ctx)
	for _, def := range ctx.AllDefinitions {
		if footnoteText[def] {
			continue
		}
		key := MarkdownlintKey(def.Label)
		if _, found := definitions[key]; !found {
			definitions[key] = def
		}
	}
	return definitions
}

// MarkdownlintDestination returns the destination of def as markdownlint
// reads it: as written, escapes included, and empty when it is written in
// angle brackets, since markdownlint reads only a destination without them.
func MarkdownlintDestination(def *ReferenceDefinition) string {
	if def.AngleBrackets {
		return ""
	}
	return def.Destination
}
