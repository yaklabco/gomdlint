// Package rules provides reference link/image tracking rules.
// These rules require document-wide analysis via the refs package.
package rules

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/refs"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// LinkFragmentsRule validates that link fragments reference valid anchors (MD051).
type LinkFragmentsRule struct {
	lint.BaseRule
}

// NewLinkFragmentsRule creates a new link fragments rule.
func NewLinkFragmentsRule() *LinkFragmentsRule {
	return &LinkFragmentsRule{
		BaseRule: lint.NewBaseRule(
			"MD051",
			"link-fragments",
			"Link fragments should be valid",
			[]string{"links"},
			false, // Not auto-fixable
		),
	}
}

// Apply checks that link fragments reference valid document anchors.
//
// Headings, custom heading anchors and HTML tags name anchors. Like
// markdownlint, MD051 reads the tags of an HTML block from its lines parsed
// again as Markdown, as MD033 does (see htmlBlockInlineHTML), so a tag on
// any line of the block counts and a tag in a code span does not. A tag
// names the value of its first id attribute, or an "a" tag without one the
// value of its first name attribute (see refs.HTMLAnchor).
func (r *LinkFragmentsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil || !hasFragmentLink(ctx) {
		return nil, nil
	}

	refCtx := refs.CollectWithOptions(ctx.Root, ctx.File, refs.Options{
		Prefix: func(node *mdast.Node, line int) int { return len(lint.ContainerPrefix(ctx.File, node, line)) },
		HTMLBlockTags: func(block *mdast.Node) []mdast.SourceRange {
			tags := htmlBlockInlineHTML(ctx.File, block)
			ranges := make([]mdast.SourceRange, len(tags))
			for idx, tag := range tags {
				ranges[idx] = tag.span
			}
			return ranges
		},
	})
	ignoreCase := ctx.OptionBool("ignore_case", false)
	ignoredPatternStr := ctx.OptionString("ignored_pattern", "")

	var ignoredPattern *regexp.Regexp
	if ignoredPatternStr != "" {
		var err error
		ignoredPattern, err = regexp.Compile(ignoredPatternStr)
		if err != nil {
			// Invalid pattern - ignore it
			ignoredPattern = nil
		}
	}

	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip if no fragment
		if usage.Fragment == "" {
			continue
		}

		// Only validate same-file fragments (e.g., "#heading").
		// External URLs have fragments pointing to external sites.
		// Local file paths (./file.md#anchor) would need cross-file validation.
		if !usage.IsSameFileFragment() {
			continue
		}

		fragment := strings.TrimPrefix(usage.Fragment, "#")

		// Skip if matches ignored pattern
		if ignoredPattern != nil && ignoredPattern.MatchString(fragment) {
			continue
		}

		// Validate fragment
		valid := refCtx.ValidateFragment(usage.Fragment)
		if !valid && ignoreCase {
			valid = refCtx.ValidateFragmentIgnoreCase(usage.Fragment)
		}

		if !valid {
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position,
				fmt.Sprintf("Link fragment '#%s' does not match any heading", fragment)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use a valid heading anchor").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// hasFragmentLink reports whether a link or image in the document has a
// destination that starts with "#", the only kind MD051 checks, so that a
// document without one is not read for anchors.
func hasFragmentLink(ctx *lint.RuleContext) bool {
	for _, nodes := range [][]*mdast.Node{ctx.Links(), ctx.Images()} {
		for _, node := range nodes {
			if strings.HasPrefix(lint.LinkDestination(node), "#") {
				return true
			}
		}
	}
	return false
}

// ReferenceLinkImagesRule validates that reference labels are defined (MD052).
type ReferenceLinkImagesRule struct {
	lint.BaseRule
}

// NewReferenceLinkImagesRule creates a new reference links/images rule.
func NewReferenceLinkImagesRule() *ReferenceLinkImagesRule {
	return &ReferenceLinkImagesRule{
		BaseRule: lint.NewBaseRule(
			"MD052",
			"reference-links-images",
			"Reference links and images should use defined labels",
			[]string{"links", "images"},
			false, // Not auto-fixable
		),
	}
}

// Apply reports full and collapsed references, and shortcut references when
// shortcut_syntax is set, whose label matches no definition. The parser turns
// every reference with a defined label into a link or image, so only the
// reference syntax it left as text can be undefined. A reference whose label
// is nothing but white space is never reported (see refs.BlankLabel).
//
// Like markdownlint, MD052 reads GFM footnotes, which goldmark does not:
// "[^label]" calls a footnote that the document defines with a line that
// starts "[^label]:", so it is no undefined reference. Only a link, never
// an image, whose label is "^" and then ASCII letters, digits and hyphens
// can call a footnote (see plainFootnoteCall), only the definitions that
// micromark is certain to read count (see certainFootnoteLabels), and a
// call matches a definition only by labelKey. Every other reference is
// read as goldmark reads it.
func (r *ReferenceLinkImagesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	includeShortcut := ctx.OptionBool("shortcut_syntax", false)
	ignoredLabels := ctx.OptionStringSlice("ignored_labels", []string{"x"})

	// Build ignored set
	ignoredSet := make(map[string]bool)
	for _, label := range ignoredLabels {
		ignoredSet[refs.NormalizeLabel(label)] = true
	}

	var footnotes map[string]bool
	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if refs.BlankLabel(usage.Label) {
			continue
		}

		// A footnote definition is no link reference definition, so a
		// plain footnote label calls the footnote even where goldmark
		// resolved it as a link to a definition such as "[^n]: /url".
		if plainFootnoteCall(usage) {
			if footnotes == nil {
				footnotes = certainFootnoteLabels(ctx)
			}
			if footnotes[labelKey(usage.Label)] {
				if footnoteShortcut(ctx.File, usage, includeShortcut, ignoredSet, ignoredLabels) {
					diags = append(diags, r.undefinedLabel(ctx, usage))
				}
				continue
			}
		}

		// A defined label can still stay text, for example when the link
		// text contains another link, so only labels with no definition are
		// reported.
		if !usage.Unresolved || usage.ResolvedDefinition != nil || ignoredSet[usage.NormalizedLabel] ||
			(usage.Style == refs.StyleShortcut && !includeShortcut) {
			continue
		}
		diags = append(diags, r.undefinedLabel(ctx, usage))
	}

	return diags, nil
}

// undefinedLabel builds the finding for usage, a reference whose label
// matches no definition.
func (r *ReferenceLinkImagesRule) undefinedLabel(ctx *lint.RuleContext, usage *refs.ReferenceUsage) lint.Diagnostic {
	kind := "link"
	if usage.IsImage {
		kind = "image"
	}
	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position,
		fmt.Sprintf("Reference %s uses undefined label [%s]", kind, displayLabel(usage.Label))).
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Define the reference or use inline syntax").
		Build()
}

// footnoteShortcut reports whether MD052 reports usage, which uses a
// footnote that micromark reads. A footnote call is no reference. In
// "[text][^label]" the second pair calls the footnote, so the first is a
// shortcut reference, which markdownlint reports with shortcut_syntax unless
// it finds "text" defined or ignored. Neither goldmark's definitions nor
// its link text tell for certain what micromark reads there, so MD052
// reports the full reference with shortcut_syntax exactly where main does,
// except where markdownlint certainly ignores "text" (see ignoresShortcut).
func footnoteShortcut(
	file *mdast.FileSnapshot, usage *refs.ReferenceUsage, includeShortcut bool, ignoredSet map[string]bool, ignoredLabels []string,
) bool {
	if usage.Style != refs.StyleFull || !includeShortcut {
		return false
	}
	if !usage.Unresolved || usage.ResolvedDefinition != nil || ignoredSet[usage.NormalizedLabel] {
		return false
	}
	text, ok := shortcutSource(file, usage)
	return !ok || !ignoresShortcut(text, ignoredLabels)
}

// plainFootnoteCall reports whether MD052 may read usage as a GFM footnote
// call: a link, never an image, whose label is "^" and then nothing but
// ASCII letters, digits and hyphens. No escape, entity, emphasis, code span
// or HTML can change such a label, so micromark and markdownlint read it as
// written. Every other label keeps main's handling, and so does an image:
// markdownlint records an image's reference with the label it reads from
// the image's own brackets, even where micromark goes on to read a "!" and
// a footnote call there, and MD052 does not model which of them it reports.
func plainFootnoteCall(usage *refs.ReferenceUsage) bool {
	label, ok := strings.CutPrefix(usage.Label, "^")
	return ok && !usage.IsImage && plainFootnoteLabel(label)
}

// plainFootnoteLabel reports whether a footnote label, after its "^", is
// not empty and holds nothing but ASCII letters, digits and hyphens, which
// micromark, markdownlint and goldmark all read as written.
func plainFootnoteLabel(label string) bool {
	if label == "" {
		return false
	}
	for idx := range len(label) {
		if char := label[idx]; char != '-' && !isASCIILetter(char) && !isASCIIDigit(char) {
			return false
		}
	}
	return true
}

// shortcutSource returns the source between the brackets of text in a full
// reference "[text][label]", and false when the reference's span does not
// hold it that way.
func shortcutSource(file *mdast.FileSnapshot, usage *refs.ReferenceUsage) (string, bool) {
	start, okStart := file.Offset(usage.Position.StartLine, usage.Position.StartColumn)
	end, okEnd := file.Offset(usage.Position.EndLine, usage.Position.EndColumn)
	suffix := "][" + usage.Label + "]"
	if !okStart || !okEnd || end > len(file.Content) || start+1+len(suffix) > end || file.Content[start] != '[' ||
		!bytes.HasSuffix(file.Content[:end], []byte(suffix)) {
		return "", false
	}
	return string(file.Content[start+1 : end-len(suffix)]), true
}

// ignoresShortcut reports whether markdownlint certainly ignores a shortcut
// reference whose source between its brackets is text. It records no
// reference for blank text (see refs.BlankLabel), and ignores a label that,
// lowercased with its white space collapsed by normalizeReference, equals
// one of ignored_labels as written. That is certain only for text of
// nothing but ASCII letters, digits, spaces and tabs, which markdownlint
// reads as written, with no markup, and which Go and JavaScript lowercase
// alike.
func ignoresShortcut(text string, ignoredLabels []string) bool {
	if refs.BlankLabel(text) {
		return true
	}
	for idx := range len(text) {
		if char := text[idx]; char != ' ' && char != '\t' && !isASCIILetter(char) && !isASCIIDigit(char) {
			return false
		}
	}
	key := labelKey(text)
	return key == "" || slices.Contains(ignoredLabels, key)
}

// labelKey returns the key under which MD052 matches a footnote call with a
// footnote definition: the label with each run of white space, as a
// JavaScript regular expression's \s reads it, made one space and trimmed
// at both ends, as markdownlint's normalizeReference does, and lowercased
// only when it is all ASCII. markdownlint lowercases other labels, goldmark
// folds their case, and the two disagree on some ("ſ" and "s", "ß" and
// "ss"), so two labels that hold a character outside ASCII match only when
// their bytes are the same: MD052 then never matches labels that
// markdownlint keeps apart.
func labelKey(label string) string {
	var key strings.Builder
	space := false
	for idx := 0; idx < len(label); {
		char, width := utf8.DecodeRuneInString(label[idx:])
		if isJSSpace(char) {
			space = key.Len() > 0
		} else {
			if space {
				key.WriteByte(' ')
				space = false
			}
			key.WriteString(label[idx : idx+width])
		}
		idx += width
	}
	if isASCII(key.String()) {
		return strings.ToLower(key.String())
	}
	return key.String()
}

// isASCII reports whether text holds nothing but ASCII characters.
func isASCII(text string) bool {
	for idx := range len(text) {
		if text[idx] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// certainFootnoteLabels returns the labels, each with its "^" and as
// labelKey gives them, of the GFM footnote definitions that micromark is
// certain to read (see certainFootnoteDefinitions).
func certainFootnoteLabels(ctx *lint.RuleContext) map[string]bool {
	labels := make(map[string]bool)
	for _, def := range certainFootnoteDefinitions(ctx) {
		labels[def.key()] = true
	}
	return labels
}

// footnoteDefinition is a GFM footnote definition that micromark is
// certain to read (see certainFootnoteDefinitions).
type footnoteDefinition struct {
	// line is the 1-based line that the definition starts at column 0.
	line int
	// label is the label as written, without its "^".
	label string
	// caret is the offset of the "^" after the definition's "[".
	caret int
}

// key returns the key under which a call matches the definition (see
// labelKey).
func (d footnoteDefinition) key() string {
	return labelKey("^" + d.label)
}

// certainFootnoteDefinitions returns, in document order, the GFM footnote
// definitions that micromark is certain to read, which goldmark does not
// parse. A definition counts on a line that starts with "[^label]:" at
// column 0 at the start of the
// document, right after a blank line or right after another definition that
// counts, since micromark reads two definitions in a row as two. goldmark
// must read the line at the top level of the document: outside its front
// matter, code blocks and HTML
// blocks, and in no blockquote or list item.
//
// So that micromark reads the lines before the definition as goldmark does,
// no line before it may start, after its indentation and container markers,
// with "$$" or ":::", where micromark may open math or a directive, and
// every line before it that may open a block holding blank lines must open
// a fenced code block that both parsers read alike (see sharedBlocks.vet).
//
// No definition counts where markdownlint may not take the parser's front
// matter (see markdownlintFrontMatterLines), or where micromark drops a byte
// order mark that the parser reads as text (see
// micromarkDropsByteOrderMark). A definition that does not count leaves its
// calls read as goldmark reads them, as before MD052 read footnotes, and
// MD053 does not report it.
func certainFootnoteDefinitions(ctx *lint.RuleContext) []footnoteDefinition {
	file := ctx.File
	var defs []footnoteDefinition
	if frontMatter, ok := markdownlintFrontMatterLines(file); !ok || frontMatter != file.FrontMatterLines ||
		micromarkDropsByteOrderMark(file) {
		return nil
	}
	blocks := sharedBlocks{ctx: ctx}
	var topLevel []bool
	afterBlank, afterDefinition := true, false
	// readThrough is the last line of a fenced code block that both parsers
	// read alike, whose lines need no reading.
	readThrough := 0
	for line := 1; line <= len(file.Lines); line++ {
		text := lint.RawLineContent(file, line)
		if line == 1 {
			text = bytes.TrimPrefix(text, []byte(mdast.ByteOrderMark))
		}
		rest := withoutContainerPrefix(text)
		if bytes.HasPrefix(rest, []byte("$$")) || bytes.HasPrefix(rest, []byte(":::")) {
			break
		}
		if line <= file.FrontMatterLines {
			continue
		}
		if line > readThrough {
			end, ok := blocks.vet(line, len(rest) == len(text) && afterBlank, rest)
			if !ok {
				break
			}
			readThrough = end
		}
		label, counts := footnoteDefinitionLabel(text)
		counts = counts && (afterBlank || afterDefinition)
		if counts {
			if topLevel == nil {
				topLevel = topLevelLines(ctx)
			}
			counts = topLevel[line]
		}
		if counts {
			caret := file.Lines[line-1].StartOffset + lint.LineTextOffset(file, line) + len("[")
			defs = append(defs, footnoteDefinition{line: line, label: label, caret: caret})
		}
		afterBlank, afterDefinition = len(bytes.Trim(text, " \t")) == 0, counts
	}
	return defs
}

// sharedBlocks finds the fenced code blocks that micromark reads as
// goldmark does, whose lines need no reading. goldmark's top-level fenced
// code blocks are read once, when a line first needs them.
type sharedBlocks struct {
	ctx    *lint.RuleContext
	fences map[int]int
}

// vet reads a line, rest after its indentation and container markers,
// that may open a block holding blank lines. It returns the last line of a
// fenced code block that both parsers read alike, 0 when the line opens no
// such block, and false when micromark may read the lines after it
// otherwise than goldmark. Both read alike a fenced code block that
// goldmark reads at the top level from a line that starts at column 0 and
// is fresh: the first line of the document or a line after a blank one,
// where no paragraph, container or HTML block of kind 6 or 7 goes on in
// either parser. Its end is then the same line in both, since both end a
// fence by the same rules, which read ASCII only. An HTML block of kinds 1
// to 5 that goes on past its first line is never alike: the parsers may
// end it on different lines, as goldmark reads "</ſcript>" as an end tag
// and micromark does not.
func (b *sharedBlocks) vet(line int, fresh bool, rest []byte) (int, bool) {
	if opensRawHTMLBlock(rest) {
		return 0, false
	}
	if !bytes.HasPrefix(rest, []byte("```")) && !bytes.HasPrefix(rest, []byte("~~~")) {
		return 0, true
	}
	if b.fences == nil {
		b.fences = topLevelFences(b.ctx)
	}
	end, ok := b.fences[line]
	return end, fresh && ok && end > line
}

// topLevelFences maps the first line of each fenced code block that is a
// child of the document to its last line.
func topLevelFences(ctx *lint.RuleContext) map[int]int {
	file := ctx.File
	fences := make(map[int]int)
	for _, block := range ctx.CodeBlocks() {
		span := block.SourceRange()
		if block.Parent != ctx.Root || !lint.IsFencedCodeBlock(block) || span.StartOffset < 0 || span.EndOffset <= span.StartOffset {
			continue
		}
		first, _ := file.LineAt(span.StartOffset)
		last, _ := file.LineAt(span.EndOffset - 1)
		if first > 0 {
			fences[first] = last
		}
	}
	return fences
}

// cdataOpening starts a CDATA section, an HTML block of kind 5.
const cdataOpening = "<![CDATA["

// opensRawHTMLBlock reports whether text starts an HTML block of
// CommonMark's kinds 1 to 5, as micromark reads them, that does not end on
// the line: one that starts with a script, pre, style or textarea tag, a
// comment, a processing instruction, a declaration or a CDATA section.
// Such a block ends only at its end marker, so it may hold blank lines.
func opensRawHTMLBlock(text []byte) bool {
	switch {
	case bytes.HasPrefix(text, []byte("<!--")):
		return !bytes.Contains(text[len("<!"):], []byte("-->"))
	case bytes.HasPrefix(text, []byte("<?")):
		return !bytes.Contains(text[len("<"):], []byte("?>"))
	case bytes.HasPrefix(text, []byte(cdataOpening)):
		return !closesCDATA(text[len(cdataOpening):])
	case len(text) > len("<!") && text[0] == '<' && text[1] == '!' && isASCIILetter(text[2]):
		return bytes.IndexByte(text[len("<!a"):], '>') < 0
	}
	if len(text) == 0 || text[0] != '<' {
		return false
	}
	name := rawHTMLTagName(text[len("<"):])
	if name == 0 {
		return false
	}
	if after := len("<") + name; len(text) > after && text[after] != ' ' && text[after] != '\t' && text[after] != '>' {
		return false
	}
	return !holdsRawHTMLEndTag(text[len("<")+name:])
}

// rawHTMLTagName returns the length of script, pre, style or textarea, the
// names that start an HTML block of kind 1, when text starts with one of
// them in any case, and 0 otherwise. The caller checks the byte after it.
func rawHTMLTagName(text []byte) int {
	for _, name := range [...]string{"script", "pre", "style", "textarea"} {
		if len(text) >= len(name) && bytes.EqualFold(text[:len(name)], []byte(name)) {
			return len(name)
		}
	}
	return 0
}

// holdsRawHTMLEndTag reports whether text holds an end tag of script, pre,
// style or textarea, in any case, which ends an HTML block of kind 1. As in
// micromark, the name must be followed directly by ">", so "</pre >" does
// not count.
func holdsRawHTMLEndTag(text []byte) bool {
	rest := text
	for {
		idx := bytes.Index(rest, []byte("</"))
		if idx < 0 {
			return false
		}
		rest = rest[idx+len("</"):]
		if name := rawHTMLTagName(rest); name > 0 && len(rest) > name && rest[name] == '>' {
			return true
		}
	}
}

// closesCDATA reports whether text, the rest of a line after "<![CDATA[",
// ends the section for micromark, which reads its end only at a ">" right
// after a run of "]" of even length, at least two.
func closesCDATA(text []byte) bool {
	run := 0
	for _, char := range text {
		switch {
		case char == ']':
			run++
		case char == '>' && run >= len("]]") && run%2 == 0:
			return true
		default:
			run = 0
		}
	}
	return false
}

// topLevelLines marks, for each line from 1, whether goldmark reads it at
// the top level of the document, outside its code blocks, HTML blocks,
// blockquotes and list items.
func topLevelLines(ctx *lint.RuleContext) []bool {
	lines := len(ctx.File.Lines)
	// opened counts, for each line, the blockquotes and list items that
	// start on it less those that end on the line before it.
	opened := make([]int, lines+2)
	for _, containers := range [][]*mdast.Node{ctx.Blockquotes(), ctx.ListItems()} {
		for _, container := range containers {
			pos := container.SourcePosition()
			first, last := max(pos.StartLine, 1), min(pos.EndLine, lines)
			if pos.IsValid() && first <= last {
				opened[first]++
				opened[last+1]--
			}
		}
	}
	html := htmlBlockLines(ctx)
	topLevel := make([]bool, lines+1)
	open := 0
	for line := 1; line <= lines; line++ {
		open += opened[line]
		topLevel[line] = open == 0 && !html[line] && !ctx.IsLineInCodeBlock(line)
	}
	return topLevel
}

// micromarkDropsByteOrderMark reports whether the text that markdownlint
// gives micromark may start with a byte order mark, which micromark drops
// and the parser reads as text, so that a fence, a comment or "$$" right
// after it opens for micromark alone. markdownlint drops one byte order
// mark from the start of the file, then the front matter together with the
// lines after it that hold only white space to a JavaScript regular
// expression, which frontMatterRe's "\s*" takes as well. The front matter
// must be the parser's (see markdownlintFrontMatterLines). To stay on the
// safe side, any line from there up to the first line that holds more than
// white space leaves it unsure when it starts with a byte order mark or
// holds a line separator, which the pattern reads as a line ending.
func micromarkDropsByteOrderMark(file *mdast.FileSnapshot) bool {
	for line := file.FrontMatterLines + 1; line <= len(file.Lines); line++ {
		text := lint.RawLineContent(file, line)
		if line == 1 {
			text = bytes.TrimPrefix(text, []byte(mdast.ByteOrderMark))
		}
		switch {
		case bytes.HasPrefix(text, []byte(mdast.ByteOrderMark)):
			return true
		case file.FrontMatterLines == 0:
			return false
		case hasLineSeparator(text):
			return true
		case !onlyJSWhiteSpace(text):
			return false
		}
	}
	return false
}

// markdownlintFrontMatterLines returns the number of lines up to the
// closing line of the front matter that markdownlint's frontMatterRe finds
// at the start of the file, and whether it can tell. The pattern opens
// front matter with a first line of "---", "+++" or "{" and white space,
// after one byte order mark if there is one, and closes it with the first
// later line of "---", of "+++" or "...", or of "}", and white space. It
// reads U+2028 and U+2029 as line endings, which the parser does not, so a
// line that holds either up to the closing line leaves it unsure.
func markdownlintFrontMatterLines(file *mdast.FileSnapshot) (int, bool) {
	if len(file.Lines) == 0 {
		return 0, true
	}
	first := bytes.TrimPrefix(lint.RawLineContent(file, 1), []byte(mdast.ByteOrderMark))
	var opening string
	var closings []string
	switch {
	case bytes.HasPrefix(first, []byte(yamlFrontMatterFence)):
		opening, closings = yamlFrontMatterFence, []string{yamlFrontMatterFence}
	case bytes.HasPrefix(first, []byte(tomlFrontMatterFence)):
		opening, closings = tomlFrontMatterFence, []string{tomlFrontMatterFence, "..."}
	case bytes.HasPrefix(first, []byte("{")):
		opening, closings = "{", []string{"}"}
	default:
		return 0, true
	}
	if hasLineSeparator(first) {
		return 0, false
	}
	if !onlyJSWhiteSpace(first[len(opening):]) {
		return 0, true
	}
	for line := 2; line <= len(file.Lines); line++ {
		text := lint.RawLineContent(file, line)
		if hasLineSeparator(text) {
			return 0, false
		}
		for _, closing := range closings {
			if bytes.HasPrefix(text, []byte(closing)) && onlyJSWhiteSpace(text[len(closing):]) {
				return line, true
			}
		}
	}
	return 0, true
}

// hasLineSeparator reports whether text holds U+2028 or U+2029, which a
// JavaScript regular expression reads as a line ending.
func hasLineSeparator(text []byte) bool {
	return bytes.Contains(text, []byte("\u2028")) || bytes.Contains(text, []byte("\u2029"))
}

// onlyJSWhiteSpace reports whether text holds nothing but the white space,
// other than line endings, that a JavaScript regular expression's \s
// matches (see isJSWhiteSpace).
func onlyJSWhiteSpace(text []byte) bool {
	for _, char := range string(text) {
		if !isJSWhiteSpace(char) {
			return false
		}
	}
	return true
}

// isJSWhiteSpace reports whether char is white space, other than a line
// ending, to a JavaScript regular expression's \s: tab, vertical tab, form
// feed, U+FEFF or a space separator.
func isJSWhiteSpace(char rune) bool {
	return char == '\t' || char == '\v' || char == '\f' || char == byteOrderMarkRune || unicode.Is(unicode.Zs, char)
}

// isJSSpace reports whether a JavaScript regular expression's \s matches
// char: white space (see isJSWhiteSpace) or a line ending, which is a line
// feed, a carriage return, U+2028 or U+2029.
func isJSSpace(char rune) bool {
	return char == '\n' || char == '\r' || char == lineSeparatorRune || char == paragraphSeparatorRune || isJSWhiteSpace(char)
}

// The runes of the byte order mark and of the two separators that a
// JavaScript regular expression reads as line endings.
const (
	byteOrderMarkRune      = 0xFEFF
	lineSeparatorRune      = 0x2028
	paragraphSeparatorRune = 0x2029
)

// withoutContainerPrefix returns text after the blockquote markers, list
// item markers and indentation that start it.
func withoutContainerPrefix(text []byte) []byte {
	for len(text) > 0 {
		switch ch := text[0]; {
		case ch == ' ' || ch == '\t' || ch == '>':
			text = text[1:]
		case (ch == '-' || ch == '+' || ch == '*') && len(text) > 1 && (text[1] == ' ' || text[1] == '\t'):
			text = text[2:]
		case ch >= '0' && ch <= '9':
			digits := 1
			for digits < len(text) && digits < 9 && text[digits] >= '0' && text[digits] <= '9' {
				digits++
			}
			if digits+1 >= len(text) || (text[digits] != '.' && text[digits] != ')') ||
				(text[digits+1] != ' ' && text[digits+1] != '\t') {
				return text
			}
			text = text[digits+2:]
		default:
			return text
		}
	}
	return text
}

// maxFootnoteLabel is the most UTF-16 code units that micromark allows in
// a footnote label.
const maxFootnoteLabel = 999

// footnoteDefinitionLabel returns the label of a GFM footnote definition
// that starts text, "[^label]:", as written. As in micromark, the label is
// not empty, holds no space, tab, line ending or "[", a backslash escapes
// a bracket or backslash after it, and it is at most maxFootnoteLabel
// UTF-16 code units long, escapes included.
func footnoteDefinitionLabel(text []byte) (string, bool) {
	if !bytes.HasPrefix(text, []byte("[^")) {
		return "", false
	}
	size := 0
	for idx := len("[^"); idx < len(text); {
		char, width := utf8.DecodeRune(text[idx:])
		switch {
		case size > maxFootnoteLabel:
			return "", false
		case char == ']':
			if size == 0 || idx+1 >= len(text) || text[idx+1] != ':' {
				return "", false
			}
			return string(text[len("[^"):idx]), true
		case char == '[' || char == ' ' || char == '\t' || char == '\r' || char == '\n':
			return "", false
		case char == '\\' && idx+1 < len(text) && (text[idx+1] == '[' || text[idx+1] == '\\' || text[idx+1] == ']'):
			idx += 2
			size += 2
			continue
		}
		idx += width
		size += utf16.RuneLen(char)
	}
	return "", false
}

// displayLabel returns a label for a message, with each run of whitespace,
// line breaks included, shown as one space.
func displayLabel(label string) string {
	return strings.Join(strings.Fields(label), " ")
}

// LinkImageRefDefsRule detects unused/duplicate reference definitions (MD053).
type LinkImageRefDefsRule struct {
	lint.BaseRule
}

// NewLinkImageRefDefsRule creates a new link/image reference definitions rule.
func NewLinkImageRefDefsRule() *LinkImageRefDefsRule {
	return &LinkImageRefDefsRule{
		BaseRule: lint.NewBaseRule(
			"MD053",
			"link-image-reference-definitions",
			"Link and image reference definitions should be needed",
			[]string{"links", "images"},
			true, // Auto-fixable (can remove unused/duplicate definitions)
		),
	}
}

// Apply checks for unused and duplicate reference definitions.
func (r *LinkImageRefDefsRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	ignoredDefs := ctx.OptionStringSlice("ignored_definitions", []string{"//"})

	// Build ignored set
	ignoredSet := make(map[string]bool)
	for _, def := range ignoredDefs {
		ignoredSet[refs.NormalizeLabel(def)] = true
	}

	var diags []lint.Diagnostic
	removal := newDefinitionRemoval(ctx, refCtx.AllDefinitions)
	footnoteDefs, footnotes := footnoteStates(ctx)
	// read marks the lines of the footnote definitions that goldmark reads
	// as link reference definitions too, which the loop below reports.
	read := make(map[int]bool)

	for _, def := range refCtx.AllDefinitions {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip ignored definitions
		if ignoredSet[def.NormalizedLabel] {
			continue
		}

		state, footnote := footnotes[def.Lines[0]]
		read[def.Lines[0]] = footnote
		var msg, suggestion string
		switch {
		case def.IsDuplicate || state == footnoteDuplicate:
			msg, suggestion = duplicateDefinition, "Remove duplicate definition"
		case def.UsageCount == 0 || state == footnoteUnused:
			msg, suggestion = unusedDefinition, "Remove unused definition or add a reference"
		default:
			continue
		}
		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, definitionPosition(ctx.File, def.Lines[0]), fmt.Sprintf(msg, displayLabel(def.Label))).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(suggestion)
		// A definition whose label starts with "^" may be a footnote
		// definition to GitHub and micromark, whose note goes on in the
		// indented lines after a blank line, which deleting its first line
		// would show.
		if builder := removal.fix(def); builder != nil && !strings.HasPrefix(def.Label, "^") {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}
	diags = append(diags, r.unreadFootnotes(ctx, footnoteDefs, footnotes, read, ignoredSet)...)
	sortDiagnosticsByPosition(diags)

	return verifyFixes(ctx, diags, fixAllowance{}), nil
}

// definitionPosition returns where MD053 reports a definition that starts on a
// 1-based line: the whole line, container prefix included, as markdownlint
// 0.41.1 reports it. A byte order mark before the line's text is left out.
func definitionPosition(file *mdast.FileSnapshot, line int) mdast.SourcePosition {
	return mdast.SourcePosition{
		StartLine:   line,
		StartColumn: lint.LineTextOffset(file, line) + 1,
		EndLine:     line,
		EndColumn:   lint.LineLength(file, line) + 1,
	}
}

// The messages of MD053's findings, which take the label.
const (
	duplicateDefinition = "Duplicate reference definition [%s]"
	unusedDefinition    = "Unused reference definition [%s]"
)

// unreadFootnotes reports the footnote definitions among defs that are
// duplicates or unused by footnotes, their states, and that goldmark does
// not read as link reference definitions, which read holds. They get no
// fix: a footnote's note can go on past its first line.
func (r *LinkImageRefDefsRule) unreadFootnotes(
	ctx *lint.RuleContext, defs []footnoteDefinition, footnotes map[int]footnoteState, read map[int]bool, ignoredSet map[string]bool,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for _, def := range defs {
		state, ok := footnotes[def.line]
		if !ok || read[def.line] || state == footnoteCalled || ignoredSet[refs.NormalizeLabel("^"+def.label)] {
			continue
		}
		msg, suggestion := unusedDefinition, "Remove unused definition or add a reference"
		if state == footnoteDuplicate {
			msg, suggestion = duplicateDefinition, "Remove duplicate definition"
		}
		diags = append(diags, lint.NewDiagnosticAt(r.ID(), ctx.File.Path, definitionPosition(ctx.File, def.line), fmt.Sprintf(msg, "^"+def.label)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(suggestion).
			Build())
	}
	return diags
}

// footnoteState is what MD053 makes of a GFM footnote definition.
type footnoteState int

const (
	// footnoteCalled is a definition that the file may call: MD053 does not
	// report it.
	footnoteCalled footnoteState = iota
	// footnoteUnused is a definition that nothing calls.
	footnoteUnused
	// footnoteDuplicate is a definition of a label defined before.
	footnoteDuplicate
)

// footnoteStates reads the GFM footnote definitions that MD053 can report
// as markdownlint 0.41.1 does, keyed by the line each starts on: those that
// micromark is certain to read (see certainFootnoteDefinitions), whose
// label holds nothing but ASCII letters, digits and hyphens (see
// plainFootnoteLabel), so that every parser reads it as written. A
// definition repeats a label when an earlier certain definition has the
// same key. Otherwise it is unused when "^" and its label appear nowhere
// else in the file (see footnoteMentions): in a call, in a reference, in
// code or in a definition MD053 cannot be certain of, all of which count,
// so that MD053 never reports a definition that markdownlint finds called.
// It returns the certain definitions too.
func footnoteStates(ctx *lint.RuleContext) ([]footnoteDefinition, map[int]footnoteState) {
	if !bytes.Contains(ctx.File.Content, []byte("[^")) {
		return nil, nil
	}
	defs := certainFootnoteDefinitions(ctx)
	if len(defs) == 0 {
		return nil, nil
	}
	own := make(map[int]bool, len(defs))
	for _, def := range defs {
		own[def.caret] = true
	}
	mentions := footnoteMentions(ctx.File.Content, own)
	states := make(map[int]footnoteState, len(defs))
	seen := make(map[string]bool, len(defs))
	for _, def := range defs {
		key := def.key()
		repeated := seen[key]
		seen[key] = true
		switch {
		case !plainFootnoteLabel(def.label):
		case repeated:
			states[def.line] = footnoteDuplicate
		case mentions[strings.ToLower(def.label)] == 0:
			states[def.line] = footnoteUnused
		default:
			states[def.line] = footnoteCalled
		}
	}
	return defs, states
}

// kelvinSign is U+212A, which JavaScript lowercases to "k", so
// markdownlint reads it as that letter in a footnote label.
const kelvinSign = "\u212a"

// footnoteMentions counts, by label in lowercase, the places in content
// where "^" is followed by a run of ASCII letters, digits, hyphens and
// Kelvin signs, read as "k", other than at the offsets in skip. Each may
// call the footnote with that label, as in "[^label]" or "[text][^label]",
// written in any case. The run takes no other character, so a mention in
// code, or one that goes on with another character, counts too, which errs
// towards a call.
func footnoteMentions(content []byte, skip map[int]bool) map[string]int {
	mentions := make(map[string]int)
	var run []byte
	for idx := bytes.IndexByte(content, '^'); idx >= 0; {
		pos := idx + 1
		if !skip[idx] {
			run = run[:0]
			for pos < len(content) {
				char := content[pos]
				if char == '-' || isASCIILetter(char) || isASCIIDigit(char) {
					if 'A' <= char && char <= 'Z' {
						char += 'a' - 'A'
					}
					run = append(run, char)
					pos++
					continue
				}
				if !bytes.HasPrefix(content[pos:], []byte(kelvinSign)) {
					break
				}
				run = append(run, 'k')
				pos += len(kelvinSign)
			}
			if len(run) > 0 {
				mentions[string(run)]++
			}
		}
		next := bytes.IndexByte(content[pos:], '^')
		if next < 0 {
			break
		}
		idx = pos + next
	}
	return mentions
}

// definitionRemoval builds the MD053 fix, which deletes a definition's lines.
type definitionRemoval struct {
	file *mdast.FileSnapshot
	// columns maps the first line of each definition to the column it starts at.
	columns map[int]int
	// absorbers are the lists and indented code blocks, which can take in
	// what follows them once a definition no longer separates them.
	absorbers []*mdast.Node
}

func newDefinitionRemoval(ctx *lint.RuleContext, defs []*refs.ReferenceDefinition) *definitionRemoval {
	columns := make(map[int]int, len(defs))
	for _, def := range defs {
		columns[def.Lines[0]] = definitionColumn(ctx.File, def)
	}
	return &definitionRemoval{
		file:    ctx.File,
		columns: columns,
		absorbers: mdast.FindAll(ctx.Root, func(node *mdast.Node) bool {
			return node.Kind == mdast.NodeList || lint.IsIndentedCodeBlock(node)
		}),
	}
}

// fix deletes the lines of a definition, line endings included, when nothing
// else on them would be lost and the blocks around them parse the same
// without them. A byte order mark before the first line stays.
func (d *definitionRemoval) fix(def *refs.ReferenceDefinition) *fix.EditBuilder {
	first, last := def.Lines[0], def.Lines[1]
	if !def.Removable || first < 1 || last < first || last > len(d.file.Lines) || !d.keepsStructure(def) {
		return nil
	}
	builder := fix.NewEditBuilder()
	builder.Delete(d.file.Lines[first-1].StartOffset+lint.LineTextOffset(d.file, first), d.file.Lines[last-1].EndOffset)
	return builder
}

// keepsStructure reports whether the blocks around a definition parse the
// same once its lines are deleted. The line after it continues the
// definition's paragraph, so it must be blank or start another definition at
// the same column: anything else could start a different block once it
// follows a blank line (indented code, an ordered list not starting at 1)
// or leave its blockquote as a lazy line. After a list or an indented code
// block, the next non-blank line must also start such a definition, or the
// document must end, because the list could take in a list item or
// indented text and two indented code blocks would merge into one.
func (d *definitionRemoval) keepsStructure(def *refs.ReferenceDefinition) bool {
	first, last := def.Lines[0], def.Lines[1]
	startsSibling := func(line int) bool {
		column, ok := d.columns[line]
		return ok && column == definitionColumn(d.file, def)
	}
	if next := last + 1; next <= len(d.file.Lines) && !d.isBlank(next) && !startsSibling(next) {
		return false
	}
	prev := first - 1
	for prev >= 1 && d.isBlank(prev) {
		prev--
	}
	if prev < 1 || !d.absorbs(prev, first) {
		return true
	}
	next := last + 1
	for next <= len(d.file.Lines) && d.isBlank(next) {
		next++
	}
	return next > len(d.file.Lines) || startsSibling(next)
}

// absorbs reports whether line prev belongs to a list or an indented code
// block that does not also hold line first.
func (d *definitionRemoval) absorbs(prev, first int) bool {
	for _, node := range d.absorbers {
		pos := node.SourcePosition()
		if !pos.IsValid() || prev < pos.StartLine || prev > pos.EndLine {
			continue
		}
		if first < pos.StartLine || first > pos.EndLine {
			return true
		}
	}
	return false
}

// definitionColumn returns the column a definition starts at, counted from
// the start of its line's text, so a byte order mark does not count.
func definitionColumn(file *mdast.FileSnapshot, def *refs.ReferenceDefinition) int {
	return def.Position.StartColumn - lint.LineTextOffset(file, def.Lines[0])
}

// isBlank reports whether a line holds nothing but spaces, tabs, and
// blockquote markers, so it is blank inside its blockquotes.
func (d *definitionRemoval) isBlank(line int) bool {
	return len(bytes.Trim(lint.LineContent(d.file, line), " \t\r>")) == 0
}

// LinkImageStyleRule enforces consistent link/image syntax styles (MD054).
type LinkImageStyleRule struct {
	lint.BaseRule
}

// NewLinkImageStyleRule creates a new link/image style rule.
func NewLinkImageStyleRule() *LinkImageStyleRule {
	return &LinkImageStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD054",
			"link-image-style",
			"Link and image style should be consistent",
			[]string{"links", "images"},
			false, // Not auto-fixable
		),
	}
}

// Apply checks link/image style consistency. Every style is allowed by
// default, as in markdownlint, so the rule reports nothing until an option
// disallows a style, and like markdownlint it then returns at once.
func (r *LinkImageStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	// Get allowed styles from config
	allowAutolink := ctx.OptionBool("autolink", true)
	allowInline := ctx.OptionBool("inline", true)
	allowFull := ctx.OptionBool("full", true)
	allowCollapsed := ctx.OptionBool("collapsed", true)
	allowShortcut := ctx.OptionBool("shortcut", true)
	allowURLInline := ctx.OptionBool("url_inline", true)
	if allowAutolink && allowInline && allowFull && allowCollapsed && allowShortcut && allowURLInline {
		return nil, nil
	}

	refCtx := ctx.RefContext()
	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Reference syntax the parser left as text is not a link or image,
		// and markdownlint reads no reference whose label is nothing but
		// white space (see refs.BlankLabel).
		if usage.Unresolved || (isReferenceStyle(usage.Style) && refs.BlankLabel(usage.Label)) {
			continue
		}

		allowed := true
		var msg string

		switch usage.Style {
		case refs.StyleAutolink:
			if !allowAutolink {
				allowed = false
				msg = "Autolinks are not allowed"
			}
		case refs.StyleInline:
			if !allowInline {
				allowed = false
				msg = "Inline links/images are not allowed"
			} else if !allowURLInline && allowAutolink && isURLAsText(ctx.File.Content, usage) {
				allowed = false
				msg = "URL as inline link text is not allowed; use autolink syntax"
			}
		case refs.StyleFull:
			if !allowFull {
				allowed = false
				msg = "Full reference syntax is not allowed"
			}
		case refs.StyleCollapsed:
			if !allowCollapsed {
				allowed = false
				msg = "Collapsed reference syntax is not allowed"
			}
		case refs.StyleShortcut:
			if !allowShortcut {
				allowed = false
				msg = "Shortcut reference syntax is not allowed"
			}
		}

		if !allowed {
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, usage.Position, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use an allowed link/image style").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// isReferenceStyle reports whether style is that of a reference: full,
// collapsed or shortcut.
func isReferenceStyle(style refs.ReferenceStyle) bool {
	return style == refs.StyleFull || style == refs.StyleCollapsed || style == refs.StyleShortcut
}

// isURLAsText reports whether an inline link could be written as an
// autolink, as markdownlint's url_inline decides it: a link, not an image,
// with no title, whose label is its destination, and whose destination an
// autolink can hold. The caller also requires autolinks to be allowed. Like
// markdownlint, it compares the label as written in content, between the
// brackets, with the destination as written, so a label with formatting or
// an escape that the destination lacks, which an autolink would lose, does
// not match.
func isURLAsText(content []byte, usage *refs.ReferenceUsage) bool {
	if usage.Style != refs.StyleInline || usage.IsImage {
		return false
	}
	node := usage.Node
	if node == nil || node.Inline == nil || node.Inline.Link == nil || node.Inline.Link.Title != "" {
		return false
	}
	start, end, ok := linkTextSpan(content, node)
	return ok && string(content[start:end]) == usage.Destination && isAutolinkDestination(usage.Destination)
}

// isAutolinkDestination reports whether a destination could be written as a
// CommonMark autolink: it starts with a scheme, as uriSchemePattern matches
// one in any case, and holds no spaces, ASCII control characters, "<" or
// ">". A protocol-relative //host/path has no scheme, so it cannot.
// markdownlint asks instead whether a browser parses the destination as a
// URL, which also accepts a one-letter scheme, such as c:x, that starts no
// autolink, and rejects http:// without a host, which an autolink can hold.
func isAutolinkDestination(dest string) bool {
	return uriSchemePattern.MatchString(dest) && !strings.ContainsFunc(dest, func(char rune) bool {
		return char == ' ' || char == '<' || char == '>' || char < 0x20 || char == 0x7f
	})
}

// DescriptiveLinkTextRule checks for generic link text (MD059).
type DescriptiveLinkTextRule struct {
	lint.BaseRule
}

// NewDescriptiveLinkTextRule creates a new descriptive link text rule.
func NewDescriptiveLinkTextRule() *DescriptiveLinkTextRule {
	return &DescriptiveLinkTextRule{
		BaseRule: lint.NewBaseRule(
			"MD059",
			"descriptive-link-text",
			"Link text should be descriptive",
			[]string{"links", "accessibility"},
			false, // Not auto-fixable
		),
	}
}

// getDefaultGenericPatterns returns markdownlint's default prohibited_texts.
func getDefaultGenericPatterns() []string {
	return []string{
		"click here",
		"here",
		"link",
		"more",
	}
}

// Apply checks for generic/non-descriptive link text. The prohibited texts
// come from patterns, which markdownlint calls prohibited_texts: the
// registry rewrites that spelling to patterns before the rule runs. With
// the default markdownlint_compatible, link text is compared like
// markdownlint: as written between the brackets, with punctuation and other
// characters that are not ASCII letters or digits ignored, and a link whose
// text holds a code span or raw HTML directly, not inside emphasis, is
// skipped. Autolinks are never checked. A finding points at the link text,
// one column after its opening bracket, as markdownlint's does.
func (r *DescriptiveLinkTextRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	refCtx := ctx.RefContext()

	// Get patterns from config, or use defaults
	patterns := ctx.OptionStringSlice("patterns", getDefaultGenericPatterns())
	compatible := ctx.OptionBool("markdownlint_compatible", true)

	// Build pattern set (lowercase for case-insensitive matching)
	patternSet := make(map[string]bool)
	for _, p := range patterns {
		if compatible {
			p = normalizeMarkdownlintLinkText(p)
		}
		patternSet[strings.ToLower(strings.TrimSpace(p))] = true
	}

	var diags []lint.Diagnostic

	for _, usage := range refCtx.Usages {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip images - they have alt text, not link text - autolinks, whose
		// text is their address, and reference syntax the parser left as
		// text, which is not a link.
		if usage.IsImage || usage.Unresolved || usage.Style == refs.StyleAutolink {
			continue
		}
		label, hasLabel := linkLabel(ctx.File, usage.Node)
		if compatible && holdsMarkdownlintChild(usage.Node, mdast.NodeCodeSpan, mdast.NodeHTMLInline) {
			continue
		}

		// Check if text is generic
		text := strings.ToLower(strings.TrimSpace(usage.Text))
		if compatible {
			source := usage.Text
			if hasLabel {
				source = string(ctx.File.Content[label.StartOffset:label.EndOffset])
			}
			text = normalizeMarkdownlintLinkText(source)
		}
		if text == "" {
			continue
		}

		if patternSet[text] {
			pos := usage.Position
			if hasLabel {
				pos = rangePosition(ctx.File, label)
			}
			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				fmt.Sprintf("Link text '%s' is not descriptive", visibleLinkText(usage.Node))).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Use descriptive text that explains the link destination").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// linkLabel returns the source range of a link's text, between its brackets.
func linkLabel(file *mdast.FileSnapshot, link *mdast.Node) (mdast.SourceRange, bool) {
	label := link.ContentRange()
	full := link.SourceRange()
	if file == nil || label.EndOffset <= label.StartOffset || label.StartOffset <= full.StartOffset ||
		label.EndOffset > len(file.Content) {
		return mdast.SourceRange{}, false
	}
	return label, true
}

// visibleLinkText returns the text a link shows, including the content of
// its code spans, for messages. A line break counts as a space.
func visibleLinkText(link *mdast.Node) string {
	var text strings.Builder
	var add func(parent *mdast.Node)
	add = func(parent *mdast.Node) {
		for child := parent.FirstChild; child != nil; child = child.Next {
			switch {
			case (child.Kind == mdast.NodeText || child.Kind == mdast.NodeCodeSpan) && child.Inline != nil:
				text.Write(child.Inline.Text)
			case child.Kind == mdast.NodeSoftBreak || child.Kind == mdast.NodeHardBreak:
				text.WriteByte(' ')
			default:
				add(child)
			}
		}
	}
	add(link)
	return text.String()
}

// markdownlintChildren returns the children of node that markdownlint's
// micromark tree gives it, in order: the node's children, with the children
// of a strikethrough in its place, because markdownlint parses no
// strikethrough.
func markdownlintChildren(node *mdast.Node) []*mdast.Node {
	var children []*mdast.Node
	for child := node.FirstChild; child != nil; child = child.Next {
		if child.Kind == mdast.NodeStrikethrough {
			children = append(children, markdownlintChildren(child)...)
			continue
		}
		children = append(children, child)
	}
	return children
}

// holdsMarkdownlintChild reports whether markdownlint's micromark tree gives
// node a child of one of kinds (see markdownlintChildren).
func holdsMarkdownlintChild(node *mdast.Node, kinds ...mdast.NodeKind) bool {
	for _, child := range markdownlintChildren(node) {
		if slices.Contains(kinds, child.Kind) {
			return true
		}
	}
	return false
}

func normalizeMarkdownlintLinkText(text string) string {
	text = strings.Map(func(char rune) rune {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			return char
		}
		return ' '
	}, text)
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
