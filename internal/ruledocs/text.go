package ruledocs

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/config"
)

const (
	// columnGap separates the columns of a text listing.
	columnGap = "  "

	// rowIndent indents the rows of a text listing under their category's
	// title.
	rowIndent = "  "

	// detailsHint ends a text listing.
	detailsHint = "Run 'gomdlint explain <rule>' for details."
)

// ListOptions controls a text listing of rules.
type ListOptions struct {
	// RuleFormat names the rules: by name, then ID, by ID, then name, or by
	// both in one column. The rules of a category are in order of the
	// identifier that comes first.
	RuleFormat config.RuleFormat

	// Total is the number of rules that the listed ones were chosen from,
	// for the count that ends the listing, which gives it when it is more
	// than the rules listed. It may be zero.
	Total int
}

// textColumn is a column of a text listing, before the description.
type textColumn struct {
	header string
	cell   func(*Entry) string
}

// textColumns returns the columns of a text listing in ruleFormat.
func textColumns(ruleFormat config.RuleFormat) []textColumn {
	name := textColumn{header: "NAME", cell: func(entry *Entry) string { return entry.Name }}
	id := textColumn{header: "ID", cell: func(entry *Entry) string { return entry.ID }}
	fix := textColumn{header: "FIX", cell: func(entry *Entry) string { return yesNo(entry.Fixable) }}
	status := textColumn{header: "STATUS", cell: (*Entry).statusText}
	switch ruleFormat {
	case config.RuleFormatID:
		return []textColumn{id, name, fix, status}
	case config.RuleFormatCombined:
		combined := textColumn{header: "RULE", cell: func(entry *Entry) string {
			return config.FormatRuleID(config.RuleFormatCombined, entry.ID, entry.Name)
		}}
		return []textColumn{combined, fix, status}
	}
	return []textColumn{name, id, fix, status}
}

// WriteText writes entries as a text table, as the rules command prints
// it: a header, then the rules of each category under its title, each
// with its identifiers, whether it fixes what it finds, its status and its
// description, and a count of the rules listed, on and fixable.
func WriteText(writer io.Writer, entries []Entry, opts ListOptions) error {
	if len(entries) == 0 {
		return write(writer, "No rules match.\n")
	}

	columns := textColumns(opts.RuleFormat)
	widths := make([]int, len(columns))
	for idx, column := range columns {
		widths[idx] = utf8.RuneCountInString(column.header)
		for _, entry := range entries {
			width := utf8.RuneCountInString(column.cell(&entry))
			if idx == 0 {
				width += len(rowIndent)
			}
			widths[idx] = max(widths[idx], width)
		}
	}

	var out strings.Builder
	headers := make([]string, len(columns))
	for idx, column := range columns {
		headers[idx] = column.header
	}
	out.WriteString(textRow(widths, headers, "DESCRIPTION"))

	enabled, fixable := 0, 0
	for _, group := range groups(entries, opts.RuleFormat) {
		out.WriteString("\n" + group.title() + "\n")
		for _, entry := range group.entries {
			cells := make([]string, len(columns))
			for idx, column := range columns {
				cells[idx] = column.cell(&entry)
			}
			cells[0] = rowIndent + cells[0]
			out.WriteString(textRow(widths, cells, entry.Description))
			if entry.State.Enabled {
				enabled++
			}
			if entry.Fixable {
				fixable++
			}
		}
	}

	listed := fmt.Sprintf("%d %s", len(entries), plural(len(entries), "rule", "rules"))
	if opts.Total > len(entries) {
		listed = fmt.Sprintf("%d of %d rules", len(entries), opts.Total)
	}
	fmt.Fprintf(&out, "\n%s, %d on, %d fixable. %s\n", listed, enabled, fixable, detailsHint)
	return write(writer, out.String())
}

// textRow writes one row of a text listing: cells padded to widths, then
// the description, without trailing spaces.
func textRow(widths []int, cells []string, description string) string {
	var row strings.Builder
	for idx, cell := range cells {
		row.WriteString(cell)
		row.WriteString(strings.Repeat(" ", widths[idx]-utf8.RuneCountInString(cell)))
		row.WriteString(columnGap)
	}
	row.WriteString(description)
	return strings.TrimRight(row.String(), " ") + "\n"
}

// plural returns one for a count of one and otherwise many.
func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

// textPage collects the sections of a text page, which a blank line
// separates.
type textPage struct {
	sections [][]string
}

// add appends a section of lines, unless it has none.
func (p *textPage) add(lines ...string) {
	if len(lines) > 0 {
		p.sections = append(p.sections, lines)
	}
}

// prose appends text wrapped at lineWidth after label, unless text is
// empty.
func (p *textPage) prose(label, text string) {
	if text != "" {
		p.add(wrap(label+text, lineWidth, "", "")...)
	}
}

// String returns the page, with the spaces of its literals restored (see
// unbrokenSpace).
func (p *textPage) String() string {
	var out strings.Builder
	for idx, lines := range p.sections {
		if idx > 0 {
			out.WriteString("\n")
		}
		for _, line := range lines {
			out.WriteString(line + "\n")
		}
	}
	return strings.ReplaceAll(out.String(), unbrokenSpace, " ")
}

// WriteTextPage writes the page of entry's rule, as explain prints it: its
// name, ID and category, its state by default and under the configuration,
// what it checks and why, its tags and options, what its fix does, how to
// configure it and silence it, and where markdownlint documents it. asked
// is the word that named the rule: when it is one of the rule's aliases,
// the page says that it is markdownlint's name for the rule.
func WriteTextPage(writer io.Writer, entry Entry, asked string) error {
	var page textPage

	header := []string{entry.Name + " (" + entry.ID + ")"}
	if entry.Category != "" {
		header[0] += ", " + entry.Category.Title()
	}
	header = append(header, defaultStateText(entry))
	if !entry.inMarkdownlint() {
		header = append(header, "markdownlint does not have this rule.")
	}
	if configured := configuredStateText(entry); configured != "" {
		header = append(header, configured)
	}
	header = append(header, aliasTexts(entry, asked, plainWords)...)
	page.add(header...)

	page.prose("", entry.Description)
	page.prose("Why: ", entry.Rationale)
	if len(entry.Tags) > 0 {
		page.add("Tags: " + strings.Join(entry.Tags, ", "))
	}
	page.add(textOptions(entry)...)
	page.prose("Fix: ", entry.Fix)
	parts, err := exampleParts(entry)
	if err != nil {
		return err
	}
	for _, section := range textExample(parts) {
		page.add(section...)
	}

	if example := exampleConfig(entry); example != nil {
		lines := make([]string, 0, len(example)+1)
		lines = append(lines, "Configure:")
		for _, line := range example {
			lines = append(lines, "  "+line)
		}
		page.add(lines...)
	}
	page.prose("", switchText(entry, plainWords))
	page.add("Silence one line:", "  "+directive(entry))
	if url := entry.markdownlintURL(); url != "" {
		page.add("markdownlint: " + url)
	}

	return write(writer, page.String())
}

// textOptions returns the lines of a text page that describe the rule's
// options, or none for a rule that does not declare them.
func textOptions(entry Entry) []string {
	if !entry.OptionsDeclared {
		return nil
	}
	if len(entry.Options) == 0 {
		return []string{"This rule has no options."}
	}

	lines := []string{"Options, under rules." + yamlKey(entry.configKey()) + ".options:"}
	for _, spec := range entry.Options {
		lines = append(lines, "  "+yamlKey(spec.Name)+" ("+optionFacts(entry, spec, plainWords)+")")
		if details := optionDetails(spec, plainWords); details != "" {
			lines = append(lines, wrap(details, lineWidth, "    ", "    ")...)
		}
	}
	return lines
}
