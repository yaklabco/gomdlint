package refs

import (
	"strings"

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

// MarkdownlintDefinitions returns the link reference definitions of ctx as
// markdownlint records them: keyed by MarkdownlintKey, the first definition
// of each key.
func MarkdownlintDefinitions(ctx *Context) map[string]*ReferenceDefinition {
	definitions := make(map[string]*ReferenceDefinition, len(ctx.AllDefinitions))
	for _, def := range ctx.AllDefinitions {
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
