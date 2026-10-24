package ruledocs

import (
	"io"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
)

const (
	// markdownPunctuation lists the characters that can start or end
	// Markdown syntax inside a line of text, which markdownText escapes:
	// backslash escapes, code spans, emphasis, links and images, raw HTML
	// and autolinks, character references, table cells, strikethrough,
	// math and headings.
	markdownPunctuation = "\\`*_[]<>&|~$#"

	// maxListNumberDigits is the most digits that the number of an ordered
	// list item may have.
	maxListNumberDigits = 9
)

// markdownText writes text, plain text such as a catalog description, as
// Markdown that shows exactly that text, in a paragraph, a list item or a
// table cell, wrapped at any space. It escapes the punctuation of inline
// syntax, words that would start a list, a heading underline or a
// thematic break at the start of a line, and the URLs and www. addresses
// that GitHub links without brackets. It does not keep email addresses
// from becoming links.
func markdownText(text string) string {
	split := strings.Split(text, " ")
	for idx, word := range split {
		split[idx] = markdownWord(word)
	}
	return strings.Join(split, " ")
}

// markdownWord escapes one word of text for markdownText.
func markdownWord(word string) string {
	var escaped strings.Builder
	for _, char := range word {
		if strings.ContainsRune(markdownPunctuation, char) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(char)
	}
	out := escaped.String()

	switch {
	case word != "" && strings.Trim(word, "-+=") == "":
		// A list marker, a setext heading underline or a thematic break.
		return `\` + out
	case isOrderedMarker(word):
		last := len(out) - 1
		return out[:last] + `\` + out[last:]
	}

	out = strings.ReplaceAll(out, "://", `\://`)
	return escapeWWW(out)
}

// escapeWWW escapes the dot of each www. in word, in any case, which
// GitHub would link: after a space, a parenthesis or emphasis, and, in
// micromark, which markdownlint uses and gomdlint's no-bare-urls follows,
// even with no domain after it.
func escapeWWW(word string) string {
	const prefix = "www."
	var out strings.Builder
	start := 0
	for idx := 0; idx+len(prefix) <= len(word); idx++ {
		if strings.EqualFold(word[idx:idx+len(prefix)], prefix) {
			dot := idx + len(prefix) - 1
			out.WriteString(word[start:dot] + `\`)
			start = dot
		}
	}
	out.WriteString(word[start:])
	return out.String()
}

// isOrderedMarker reports whether word would start an ordered list item at
// the start of a line: a number, then . or ), such as 1. or 2).
func isOrderedMarker(word string) bool {
	last := len(word) - 1
	if last < 1 || last > maxListNumberDigits || (word[last] != '.' && word[last] != ')') {
		return false
	}
	return strings.Trim(word[:last], "0123456789") == ""
}

// markdownCode writes word as a Markdown code span.
func markdownCode(word string) string {
	if strings.Contains(word, "`") {
		return "`` " + word + " ``"
	}
	return "`" + word + "`"
}

// markdownProse writes text as a Markdown paragraph, wrapped at lineWidth.
func markdownProse(text string) string {
	return strings.Join(wrap(markdownText(text), lineWidth, "", ""), "\n")
}

// markdownItem writes item, Markdown text, as an item of a bullet list,
// wrapped at lineWidth.
func markdownItem(item string) string {
	return strings.Join(wrap(item, lineWidth, "- ", "  "), "\n")
}

// markdownBlock writes lines as a fenced code block in language.
func markdownBlock(language string, lines ...string) string {
	return markdownFence(language, strings.Join(lines, "\n"))
}

// markdownDocument collects the blocks of a Markdown document, which a
// blank line separates.
type markdownDocument struct {
	blocks []string
}

// add appends blocks to the document.
func (d *markdownDocument) add(blocks ...string) {
	d.blocks = append(d.blocks, blocks...)
}

// heading appends a heading of level with text.
func (d *markdownDocument) heading(level int, text string) {
	d.add(strings.Repeat("#", level) + " " + markdownText(text))
}

// String returns the document, ending in a newline.
func (d *markdownDocument) String() string {
	return strings.Join(d.blocks, "\n\n") + "\n"
}

// WriteMarkdown writes entries as Markdown, as rules --format markdown
// prints them: a heading with title, and for each category a heading and a
// table of its rules, by name, with their IDs, whether they fix what they
// find, and their descriptions. The table does not depend on the rules'
// states.
func WriteMarkdown(writer io.Writer, title string, entries []Entry) error {
	var doc markdownDocument
	doc.heading(1, title)

	listed := groups(entries, config.RuleFormatName)
	if len(listed) == 0 {
		doc.add("No rules match.")
	}
	for _, group := range listed {
		doc.heading(2, group.title())
		rows := make([]string, 0, len(group.entries)+2) // The header and delimiter rows, then a row per rule.
		rows = append(rows, "| Name | ID | Fix | Description |", "| --- | --- | --- | --- |")
		for _, entry := range group.entries {
			rows = append(rows, "| "+strings.Join([]string{
				markdownCode(entry.Name), markdownText(entry.ID), yesNo(entry.Fixable), markdownText(entry.Description),
			}, " | ")+" |")
		}
		doc.add(strings.Join(rows, "\n"))
	}

	return write(writer, doc.String())
}

// WriteMarkdownPage writes the page of entry's rule as Markdown, as explain
// --format markdown prints it: its ID, category, built-in state and tags,
// what it checks and why, its options, what its fix does, and how to
// configure it and silence it. The page does not depend on the rule's
// state under a configuration.
func WriteMarkdownPage(writer io.Writer, entry Entry) error {
	var doc markdownDocument
	doc.heading(1, entry.Name)
	doc.add(markdownFacts(entry))
	if entry.Description != "" {
		doc.add(markdownProse(entry.Description))
	}

	if entry.Rationale != "" {
		doc.heading(2, "Why")
		doc.add(markdownProse(entry.Rationale))
	}
	if entry.OptionsDeclared {
		doc.heading(2, "Options")
		doc.add(markdownOptions(entry)...)
	}
	if entry.Fix != "" {
		doc.heading(2, "Fix")
		doc.add(markdownProse(entry.Fix))
	}
	parts, err := exampleParts(entry)
	if err != nil {
		return err
	}
	if len(parts) > 0 {
		doc.heading(2, "Example")
		doc.add(markdownExample(parts)...)
	}

	doc.heading(2, "Configure")
	if example := exampleConfig(entry); example != nil {
		doc.add(markdownBlock("yaml", example...))
	}
	doc.add(strings.Join(wrap(switchText(entry, markdownWords), lineWidth, "", ""), "\n"))

	doc.heading(2, "Silence one line")
	doc.add(markdownBlock("markdown", directive(entry)))

	doc.heading(2, "markdownlint")
	if url := entry.markdownlintURL(); url != "" {
		doc.add("[" + entry.ID + " in markdownlint's documentation](" + url + ")")
	} else {
		doc.add("markdownlint does not have this rule.")
	}

	return write(writer, doc.String())
}

// markdownFacts lists the rule's ID, category, built-in state, fix flag,
// tags and other names.
func markdownFacts(entry Entry) string {
	state := "on"
	if !entry.Default.Enabled {
		state = "off"
	}
	items := []string{"ID: " + markdownCode(entry.ID)}
	if entry.Category != "" {
		items = append(items, "Category: "+markdownText(entry.Category.Title()))
	}
	items = append(items,
		"Default: "+state+", severity "+string(entry.Default.Severity),
		"Fixable: "+yesNo(entry.Fixable))
	if len(entry.Tags) > 0 {
		items = append(items, "Tags: "+codeList(entry.Tags))
	}
	if len(entry.Aliases) > 0 {
		items = append(items, "Also named: "+codeList(entry.Aliases)+markdownlintNames(entry))
	}

	lines := make([]string, len(items))
	for idx, item := range items {
		lines[idx] = markdownItem(item)
	}
	return strings.Join(lines, "\n")
}

// markdownOptions returns the blocks that describe the rule's options.
func markdownOptions(entry Entry) []string {
	if len(entry.Options) == 0 {
		return []string{"This rule has no options."}
	}

	items := make([]string, len(entry.Options))
	for idx, spec := range entry.Options {
		item := markdownCode(spec.Name) + " (" + optionFacts(entry, spec, markdownWords) + ")"
		if details := optionDetails(spec, markdownWords); details != "" {
			item += ": " + details
		}
		items[idx] = markdownItem(item)
	}
	return []string{
		"Set them under " + markdownCode("rules."+entry.Name+".options") + ".",
		strings.Join(items, "\n"),
	}
}

// codeList writes words as code spans, separated by commas.
func codeList(words []string) string {
	spans := make([]string, len(words))
	for idx, word := range words {
		spans[idx] = markdownCode(word)
	}
	return strings.Join(spans, ", ")
}
