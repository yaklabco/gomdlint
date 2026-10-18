package lint

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// markdownlint-cli2 0.23.3 reads YAML with js-yaml 5.4.1's load and its
// default schema, the core schema, which knows the tags str, seq, map,
// null, bool, int and float, and reads plain scalars as YAML 1.2's core
// schema does. Its mappings are JavaScript objects, so every key becomes a
// string, and a key whose string repeats another's is an error. This file
// reads markdownlint YAML into the same values: yaml.v3 parses the text into
// nodes, and the converter below gives each node js-yaml's value, without
// yaml.v3's own decoding, which reads scalars as YAML 1.1 does, applies
// merge keys, and compares every pair of keys of a mapping.
//
// js-yaml stops at a depth of 100, which it counts its own way, and the
// converter has no such limit, so a deeply nested config loads, as it did
// when gomdlint decoded YAML with yaml.v3. The converter recurses once for
// each level of nesting, as yaml.v3's parser does, whose scanner stops at
// 10,000 levels of flow collections and 10,000 of indentation, and Go's
// stacks grow as the recursion needs.

// jsNull is null as JavaScript writes it, which String(null) gives.
const jsNull = "null"

// errMoreDocuments reports YAML text that holds more than one document.
var errMoreDocuments = errors.New("expected one document, but found more")

// errDirectivesEnd reports directives that no "---" follows.
var errDirectivesEnd = errors.New("directives end mark is expected")

// The patterns of js-yaml 5.4.1's core schema (src/tag/scalar/*_core.ts).
var (
	// yamlImplicitInt is the int tag's pattern for a plain scalar.
	yamlImplicitInt = regexp.MustCompile(`^(?:0o[0-7]+|0x[0-9a-fA-F]+|[-+]?[0-9]+)$`)

	// yamlExplicitInt is its pattern for a scalar tagged !!int.
	yamlExplicitInt = regexp.MustCompile(`^(?:[-+]?0b[0-1]+|[-+]?0o[0-7]+|[-+]?0x[0-9a-fA-F]+|[-+]?[0-9]+)$`)

	// yamlFloat is the float tag's pattern, tagged or not.
	yamlFloat = regexp.MustCompile(`^(?:[-+]?[0-9]+(?:\.[0-9]*)?(?:[eE][-+]?[0-9]+)?|[-+]?\.[0-9]+(?:[eE][-+]?[0-9]+)?|` +
		`[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`)

	// yamlVersion is the form of the argument of a %YAML directive.
	yamlVersion = regexp.MustCompile(`^([0-9]+)\.[0-9]+$`)
)

// readMarkdownlintYAML reads text as a markdownlint configuration in YAML,
// as js-yaml 5.4.1's load reads it with its default core schema. The text
// must hold one document, or none, and nothing after it but comments and
// an end marker: yaml.v3 alone would ignore a second document, or text
// after a mapping in flow style, such as {"MD013": false} trailing. The
// whole document is read, so an error anywhere in it, such as a key
// written twice in a list, rejects the text, as in js-yaml, even when the
// document holds no object.
func readMarkdownlintYAML(text []byte) (*MarkdownlintSource, error) {
	text, err := jsYAMLPrologue(text)
	if err != nil {
		return nil, err
	}
	text = jsYAMLEpilogue(text)
	source := &MarkdownlintSource{Settings: &MarkdownlintSettings{}, Syntax: MarkdownlintYAML}
	decoder := yaml.NewDecoder(bytes.NewReader(text))
	var doc yaml.Node
	if err := decoder.Decode(&doc); errors.Is(err, io.EOF) {
		return source, nil // Empty text, or nothing but comments.
	} else if err != nil {
		return nil, err //nolint:wrapcheck // The caller says what it read.
	}
	if err := decoder.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err //nolint:wrapcheck // The caller says what it read.
		}
		return nil, errMoreDocuments
	}
	if len(doc.Content) == 0 {
		return source, nil
	}

	converter := newYAMLConverter(text)
	root := doc.Content[0]
	if root.Kind == yaml.MappingNode {
		settings, place, err := converter.root(root)
		if err != nil {
			return nil, err
		}
		source.Settings, source.Positions = settings, place
		return source, nil
	}
	value, err := converter.value(root, nil)
	if err != nil {
		return nil, err
	}
	switch value.value.(type) {
	case nil, map[string]any:
		// Null, or a scalar tagged !!map, which holds no keys.
	case []any:
		source.NotObject = kindList
	case string:
		source.NotObject = kindString
	case float64:
		source.NotObject = kindNumber
	case bool:
		source.NotObject = kindBoolean
	default:
		source.NotObject = kindOther
	}
	return source, nil
}

// jsYAMLPrologue returns text with the directives at its start that js-yaml
// accepts and yaml.v3 does not made acceptable to yaml.v3, keeping every
// byte's offset. js-yaml accepts a %YAML directive of any version 1.x and
// ignores directives other than %YAML and %TAG, while yaml.v3, a port of
// libyaml, accepts version 1.1 only and rejects other directives. So the
// version of a %YAML directive that js-yaml accepts becomes 1.1, and an
// unknown directive becomes a comment. js-yaml requires the "---" that ends
// the directives, which a directive turned comment no longer makes yaml.v3
// check, so its absence is an error here. Directives that js-yaml rejects
// stay as they are, and yaml.v3 rejects them too.
//
// js-yaml counts no indentation on the first line of the text, which no
// line break precedes, so a directive or a document marker there may
// follow spaces, as in the parameter of a configure-file comment, which
// starts after the space that ends the comment's kind. yaml.v3 requires
// them at the start of the line, so such a line's leading whitespace moves
// to its end.
func jsYAMLPrologue(text []byte) ([]byte, error) {
	start := 0
	if bytes.HasPrefix(text, []byte(byteOrderMark)) {
		start = len(byteOrderMark)
	}
	var rewritten []byte
	if first := text[start:lineEnd(text, start)]; len(first) > 0 && (first[0] == ' ' || first[0] == '\t') {
		content := bytes.TrimLeft(first, " \t")
		if bytes.HasPrefix(content, []byte("%")) || isDocumentStart(content) || isDocumentEnd(content) {
			rewritten = bytes.Clone(text)
			copy(rewritten[start:], content)
			copy(rewritten[start+len(content):start+len(first)], first[:len(first)-len(content)])
			text = rewritten
		}
	}
	directives := false
	for pos := start; pos < len(text); {
		end := lineEnd(text, pos)
		line := text[pos:end]
		next := end
		if next < len(text) && text[next] == '\r' && next+1 < len(text) && text[next+1] == '\n' {
			next++
		}
		if next < len(text) {
			next++
		}

		trimmed := bytes.TrimLeft(line, " \t")
		switch {
		case len(trimmed) == 0 || trimmed[0] == '#':
		case line[0] == '%':
			directives = true
			if replacement := rewriteDirective(line); replacement != nil {
				if rewritten == nil {
					rewritten = bytes.Clone(text)
				}
				copy(rewritten[pos:end], replacement)
			}
		default:
			if directives && !isDocumentStart(line) {
				return nil, errDirectivesEnd
			}
			if rewritten != nil {
				return rewritten, nil
			}
			return text, nil
		}
		pos = next
	}
	if directives {
		return nil, errDirectivesEnd
	}
	if rewritten != nil {
		return rewritten, nil
	}
	return text, nil
}

// rewriteDirective returns the line of a directive as yaml.v3 should read
// it, or nil when it should read the line as it is (see jsYAMLPrologue).
// The result has the line's length. The line is read as js-yaml's
// readDirective reads it: the name runs from the % to a space or a tab, so
// it may hold # or a no-break space, and the arguments are the words that
// spaces and tabs separate, up to a word that starts with #.
func rewriteDirective(line []byte) []byte {
	isSeparator := func(char byte) bool { return char == ' ' || char == '\t' }
	nameEnd := 1
	for nameEnd < len(line) && !isSeparator(line[nameEnd]) {
		nameEnd++
	}
	var args [][2]int // The start and end of each argument in line.
	for at := nameEnd; at < len(line); {
		for at < len(line) && isSeparator(line[at]) {
			at++
		}
		if at == len(line) || line[at] == '#' {
			break
		}
		start := at
		for at < len(line) && !isSeparator(line[at]) {
			at++
		}
		args = append(args, [2]int{start, at})
	}
	switch string(line[1:nameEnd]) {
	case "":
		return nil // js-yaml rejects a directive without a name, as yaml.v3 does.
	case "TAG":
		return nil
	case "YAML":
		if len(args) != 1 {
			return nil
		}
		start, end := args[0][0], args[0][1]
		match := yamlVersion.FindSubmatch(line[start:end])
		if match == nil {
			return nil
		}
		if major, err := strconv.Atoi(string(match[1])); err != nil || major != 1 {
			return nil
		}
		replacement := bytes.Clone(line)
		copy(replacement[start:end], "1.1"+strings.Repeat(" ", end-start-len("1.1")))
		return replacement
	default:
		replacement := bytes.Clone(line)
		replacement[0] = '#' // js-yaml ignores the directive.
		return replacement
	}
}

// byteOrderMark is the byte order mark in UTF-8.
const byteOrderMark = "\xef\xbb\xbf"

// jsYAMLEpilogue returns text with the byte order marks after its first
// document that js-yaml skips made acceptable to yaml.v3, keeping every
// byte's offset. js-yaml skips one byte order mark at the start of a line
// where a document may start (parseEvents in js-yaml 5.4.1): after the end
// marker "..." and the spaces, comments and line breaks that follow it, and
// again after each further "...". yaml.v3 reads such a mark as text, which
// starts a second document. So each such mark moves to the end of its line
// as three spaces, and what follows it on the line, such as a comment or
// another "...", moves to the start. What else follows the first document
// stays as it is, so yaml.v3 rejects a second document, as js-yaml does.
func jsYAMLEpilogue(text []byte) []byte {
	at := firstDocumentEnd(text)
	if at < 0 {
		return text
	}
	var rewritten []byte
	for {
		at = skipJSYAMLSeparation(text, at+len("..."), false)
		lineStart := at > 0 && (text[at-1] == '\n' || text[at-1] == '\r')
		if lineStart && bytes.HasPrefix(text[at:], []byte(byteOrderMark)) {
			if rewritten == nil {
				rewritten = bytes.Clone(text)
			}
			end := lineEnd(text, at)
			copy(rewritten[at:], text[at+len(byteOrderMark):end])
			copy(rewritten[end-len(byteOrderMark):end], "   ")
			// js-yaml starts the line after the mark that it skipped.
			afterMark := at + len(byteOrderMark)
			at = skipJSYAMLSeparation(text, afterMark, true)
			lineStart = at == afterMark || text[at-1] == '\n' || text[at-1] == '\r'
		}
		if !lineStart || !isDocumentEnd(text[at:lineEnd(text, at)]) {
			break
		}
	}
	if rewritten == nil {
		return text
	}
	return rewritten
}

// firstDocumentEnd returns the offset in text of the first document end
// marker "..." at the start of a line, or -1 when there is none.
func firstDocumentEnd(text []byte) int {
	for at := 0; at < len(text); {
		end := lineEnd(text, at)
		if isDocumentEnd(text[at:end]) {
			return at
		}
		at = nextLineStart(text, end)
	}
	return -1
}

// nextLineStart returns the offset in text of the start of the line after
// the line break at end, or the end of text.
func nextLineStart(text []byte, end int) int {
	if end < len(text) && text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
		end++
	}
	return min(end+1, len(text))
}

// skipJSYAMLSeparation returns the offset in text of the first character at
// or after at that js-yaml's skipSeparationSpace does not skip: spaces,
// tabs and line breaks, and comments, which start with # where a line
// starts or after a space, a tab or a line break. lineStart says whether
// at starts a line for js-yaml, as it does after a byte order mark that it
// skipped.
func skipJSYAMLSeparation(text []byte, at int, lineStart bool) int {
	separated := lineStart || at == 0 || strings.IndexByte(" \t\r\n", text[at-1]) >= 0
	for at < len(text) {
		switch char := text[at]; {
		case char == ' ' || char == '\t' || char == '\n' || char == '\r':
			at++
			separated = true
		case char == '#' && separated:
			at = lineEnd(text, at)
		default:
			return at
		}
	}
	return at
}

// lineEnd returns the offset of the line feed or carriage return that ends
// the line that starts at start, or the end of text.
func lineEnd(text []byte, start int) int {
	if end := bytes.IndexAny(text[start:], "\r\n"); end >= 0 {
		return start + end
	}
	return len(text)
}

// isDocumentStart reports whether line starts with the marker "---" that
// starts a document, followed by whitespace or nothing.
func isDocumentStart(line []byte) bool {
	return isMarker(line, "---")
}

// isDocumentEnd reports whether line starts with the marker "..." that ends
// a document, followed by whitespace or nothing.
func isDocumentEnd(line []byte) bool {
	return isMarker(line, "...")
}

// isMarker reports whether line starts with marker, followed by whitespace
// or nothing.
func isMarker(line []byte, marker string) bool {
	return bytes.HasPrefix(line, []byte(marker)) && (len(line) == len(marker) || line[3] == ' ' || line[3] == '\t')
}

// yamlConverter converts the nodes of one YAML document to the values that
// js-yaml 5.4.1 reads from them with its default core schema, and records
// where each key and item is written.
type yamlConverter struct {
	// text is the document's text, in which nodes have their positions.
	text []byte

	// positions finds the offset in text of a node's position.
	positions yamlPositions

	// done holds the values of the anchored nodes converted so far, with
	// the places of their entries. js-yaml reads an alias as the value of
	// its anchor itself, so values shared this way are converted once,
	// however many aliases repeat them. An alias inside the node it names
	// finds no value here yet (see alias).
	done map[*yaml.Node]yamlValue
}

// yamlValue is the value of a node, with the places of its keys or items
// when it is a mapping or a sequence.
type yamlValue struct {
	value any

	// place holds the places of the keys or items, and is nil for a
	// scalar.
	place *MarkdownlintPosition
}

// newYAMLConverter returns a converter for the nodes of a document of text.
func newYAMLConverter(text []byte) *yamlConverter {
	return &yamlConverter{text: text, positions: yamlPositions{text: text}, done: make(map[*yaml.Node]yamlValue)}
}

// root converts node, the mapping at the root of a document, into
// settings, keeping its keys in the order they are written, and returns
// the places of its keys.
func (c *yamlConverter) root(node *yaml.Node) (*MarkdownlintSettings, *MarkdownlintPosition, error) {
	pairs, place, err := c.mapping(node, nil)
	c.remember(node, yamlValue{value: pairsValue(pairs), place: place})
	if err != nil {
		return nil, nil, err
	}
	settings := &MarkdownlintSettings{}
	for _, pair := range pairs {
		settings.Set(pair.Key, pair.Value)
	}
	return settings, place, nil
}

// remember records value as the value of node, when aliases may name it.
func (c *yamlConverter) remember(node *yaml.Node, value yamlValue) {
	if node.Anchor != "" {
		c.done[node] = value
	}
}

// value converts node, which next, the node after it in the document,
// follows, or nothing when next is nil. A mapping becomes a
// map[string]any, a sequence a []any, and a scalar nil, a bool, a float64
// or a string.
func (c *yamlConverter) value(node, next *yaml.Node) (yamlValue, error) {
	if node.Kind == yaml.AliasNode {
		return c.alias(node), nil
	}
	var result yamlValue
	var err error
	switch node.Kind {
	case yaml.MappingNode:
		var pairs []MarkdownlintSetting
		pairs, result.place, err = c.mapping(node, next)
		result.value = pairsValue(pairs)
	case yaml.SequenceNode:
		result.value, result.place, err = c.sequence(node, next)
	default:
		result.value, err = c.scalar(node, next)
	}
	c.remember(node, result)
	return result, err
}

// alias returns the value of the node that node, an alias, names. js-yaml
// reads an alias inside the node it names as a value that holds itself;
// that reads as an empty collection of the node's kind here, so that no
// value holds a cycle.
func (c *yamlConverter) alias(node *yaml.Node) yamlValue {
	target := node.Alias
	if value, ok := c.done[target]; ok {
		return value
	}
	if target.Kind == yaml.MappingNode {
		return yamlValue{value: map[string]any{}}
	}
	return yamlValue{value: []any{}}
}

// pairsValue returns the pairs of a mapping as a map.
func pairsValue(pairs []MarkdownlintSetting) map[string]any {
	value := make(map[string]any, len(pairs))
	for _, pair := range pairs {
		value[pair.Key] = pair.Value
	}
	return value
}

// entryPlace returns the place of an entry written on line whose value's
// keys or items are at place, which is nil for a scalar.
func entryPlace(line int, place *MarkdownlintPosition) *MarkdownlintPosition {
	entry := &MarkdownlintPosition{Line: line}
	if place != nil {
		entry.Keys, entry.Items = place.Keys, place.Items
	}
	return entry
}

// mapping converts the pairs of node, a mapping that next follows (see
// value), in the order they are written, and returns the places of its
// keys. Each key becomes a string, as the key of a JavaScript object does
// (see jsString), and a key whose string repeats an earlier one's is an
// error, found in constant time per key.
func (c *yamlConverter) mapping(node, next *yaml.Node) ([]MarkdownlintSetting, *MarkdownlintPosition, error) {
	if node.Style&yaml.TaggedStyle != 0 && node.Tag != "!!map" {
		return nil, nil, fmt.Errorf("line %d: unknown mapping tag %s", node.Line, node.Tag)
	}
	pairs := make([]MarkdownlintSetting, 0, len(node.Content)/2)
	place := &MarkdownlintPosition{Line: node.Line, Keys: make(map[string]*MarkdownlintPosition, len(node.Content)/2)}
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		keyNode, valueNode := node.Content[idx], node.Content[idx+1]
		valueNext := next
		if idx+2 < len(node.Content) {
			valueNext = node.Content[idx+2]
		}
		key, err := c.value(keyNode, valueNode)
		if err != nil {
			return nil, nil, err
		}
		name, ok := jsString(key.value)
		if !ok {
			return nil, nil, fmt.Errorf("line %d: a mapping or a sequence cannot be a key", keyNode.Line)
		}
		if _, repeated := place.Keys[name]; repeated {
			return nil, nil, fmt.Errorf("line %d: duplicated mapping key %q", keyNode.Line, name)
		}
		value, err := c.value(valueNode, valueNext)
		if err != nil {
			return nil, nil, err
		}
		pairs = append(pairs, MarkdownlintSetting{Key: name, Value: value.value})
		place.Keys[name] = entryPlace(keyNode.Line, value.place)
	}
	return pairs, place, nil
}

// sequence converts the items of node, a sequence that next follows (see
// value), and returns their places.
func (c *yamlConverter) sequence(node, next *yaml.Node) ([]any, *MarkdownlintPosition, error) {
	if node.Style&yaml.TaggedStyle != 0 && node.Tag != "!!seq" {
		return nil, nil, fmt.Errorf("line %d: unknown sequence tag %s", node.Line, node.Tag)
	}
	items := make([]any, len(node.Content))
	place := &MarkdownlintPosition{Line: node.Line, Items: make([]*MarkdownlintPosition, len(node.Content))}
	for idx, item := range node.Content {
		itemNext := next
		if idx+1 < len(node.Content) {
			itemNext = node.Content[idx+1]
		}
		value, err := c.value(item, itemNext)
		if err != nil {
			return nil, nil, err
		}
		items[idx] = value.value
		place.Items[idx] = entryPlace(item.Line, value.place)
	}
	return items, place, nil
}

// scalar converts node, a scalar that next follows (see value), as js-yaml's
// core schema reads it. A quoted scalar, a block scalar and a scalar with
// the non-specific tag ! are strings. A plain scalar resolves to null, a
// bool, an int or a float when its text has one of their forms (see
// implicitScalar), and is a string otherwise. An explicit tag must be one
// that the schema knows.
func (c *yamlConverter) scalar(node, next *yaml.Node) (any, error) {
	if node.Style&yaml.TaggedStyle != 0 {
		return explicitScalar(node)
	}
	if node.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return node.Value, nil
	}
	value := implicitScalar(node.Value)
	if _, isString := value.(string); !isString && c.nonSpecific(node, next) {
		return node.Value, nil
	}
	return value, nil
}

// nonSpecific reports whether node, a plain scalar that next follows (see
// value), has the non-specific tag !. yaml.v3 reads a scalar tagged ! as
// one without a tag, so its node records nothing of it, and the tag is
// found in the text: yaml.v3 places a node at its first property, so the
// tag is there, or after the anchor and the spaces, line breaks and
// comments that may follow it. A ! at or after the place of next is next's
// own, as is the tag of a key after an empty value.
func (c *yamlConverter) nonSpecific(node, next *yaml.Node) bool {
	at, ok := c.positions.offset(node.Line, node.Column)
	if !ok {
		return false
	}
	end := len(c.text)
	if next != nil {
		if limit, found := c.positions.offset(next.Line, next.Column); found && limit < end {
			end = limit
		}
	}
	if node.Anchor != "" && at < end && c.text[at] == '&' {
		at = skipYAMLSeparation(c.text, at+1+len(node.Anchor), end)
	}
	if at >= end || c.text[at] != '!' {
		return false
	}
	after, _ := utf8.DecodeRune(c.text[at+1:])
	return at+1 == len(c.text) || isYAMLSpace(after)
}

// skipYAMLSeparation returns the offset in text of the first character at
// or after at, and before end, that is neither a space, a tab or a line
// break, nor in a comment, or end.
func skipYAMLSeparation(text []byte, at, end int) int {
	for at < end {
		char, size := utf8.DecodeRune(text[at:])
		switch {
		case isYAMLSpace(char):
			at += size
		case char == '#' && at > 0:
			// A comment starts after a space or a line break.
			if before, _ := utf8.DecodeLastRune(text[:at]); !isYAMLSpace(before) {
				return at
			}
			at = min(lineEnd(text, at), end)
		default:
			return at
		}
	}
	return end
}

// isYAMLSpace reports whether char is a space, a tab or a character that
// ends a line in yaml.v3 (see yamlLineStarts).
func isYAMLSpace(char rune) bool {
	switch char {
	case ' ', '\t', '\r', '\n', '\u0085', '\u2028', '\u2029':
		return true
	}
	return false
}

// yamlPositions finds the offsets in text of positions as yaml.v3 gives
// them, by line and column, both counted from 1: lines end at a line feed,
// a carriage return or both, or at the characters U+0085, U+2028 and
// U+2029, and columns count characters. Each line is looked at once, the
// first time a position on it is wanted, so finding every node of a
// document takes time in proportion to its length.
type yamlPositions struct {
	text []byte

	// lineStarts holds the offset of the start of each line of text.
	lineStarts []int

	// looked records which lines have been looked at.
	looked []bool

	// columns holds, for each line that has been looked at and holds a
	// character beyond ASCII, the offset of each of its characters and of
	// its end. A line of ASCII text needs none, since each of its columns
	// is one byte.
	columns [][]int
}

// offset returns the offset in text of the character at line and column,
// and whether the text has that position.
func (p *yamlPositions) offset(line, column int) (int, bool) {
	if p.lineStarts == nil {
		p.lineStarts = yamlLineStarts(p.text)
		p.looked = make([]bool, len(p.lineStarts))
		p.columns = make([][]int, len(p.lineStarts))
	}
	if line < 1 || line > len(p.lineStarts) || column < 1 {
		return 0, false
	}
	start, end := p.lineStarts[line-1], len(p.text)
	if line < len(p.lineStarts) {
		end = p.lineStarts[line]
	}
	if !p.looked[line-1] {
		p.looked[line-1] = true
		if !asciiOnly(p.text[start:end]) {
			offsets := make([]int, 0, end-start+1)
			for at := start; at < end; {
				offsets = append(offsets, at)
				_, size := utf8.DecodeRune(p.text[at:end])
				at += size
			}
			p.columns[line-1] = append(offsets, end)
		}
	}
	if offsets := p.columns[line-1]; offsets != nil {
		if column > len(offsets) {
			return 0, false
		}
		return offsets[column-1], true
	}
	if at := start + column - 1; at <= end {
		return at, true
	}
	return 0, false
}

// asciiOnly reports whether text holds ASCII characters only.
func asciiOnly(text []byte) bool {
	for _, char := range text {
		if char >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// yamlLineStarts returns the offset of the start of each line of text, with
// lines broken as yaml.v3 breaks them (see yamlPositions).
func yamlLineStarts(text []byte) []int {
	starts := []int{0}
	if bytes.HasPrefix(text, []byte(byteOrderMark)) {
		starts[0] = len(byteOrderMark)
	}
	for at := starts[0]; at < len(text); {
		char, size := utf8.DecodeRune(text[at:])
		at += size
		switch char {
		case '\r':
			if at < len(text) && text[at] == '\n' {
				at++
			}
		case '\n', '\u0085', '\u2028', '\u2029':
		default:
			continue
		}
		starts = append(starts, at)
	}
	return starts
}

// explicitScalar converts node, a scalar with an explicit tag, as js-yaml's
// core schema reads it: str keeps the text, null, bool, int and float
// require text of their forms, and map and seq an empty text, which gives
// an empty collection. Every other tag is unknown.
func explicitScalar(node *yaml.Node) (any, error) {
	value := node.Value
	switch node.Tag {
	case "!!str":
		return value, nil
	case "!!null":
		if isYAMLNull(value) {
			return nil, nil //nolint:nilnil // nil is the null that the tag asks for.
		}
	case "!!bool":
		if boolean, ok := yamlBool(value); ok {
			return boolean, nil
		}
	case "!!int":
		if yamlExplicitInt.MatchString(value) {
			if number, ok := yamlInt(value); ok {
				return number, nil
			}
		}
	case "!!float":
		if number, ok := yamlFloatValue(value); ok {
			return number, nil
		}
	case "!!map":
		if value == "" {
			return map[string]any{}, nil
		}
	case "!!seq":
		if value == "" {
			return []any{}, nil
		}
	default:
		return nil, fmt.Errorf("line %d: unknown scalar tag %s", node.Line, node.Tag)
	}
	return nil, fmt.Errorf("line %d: cannot read %q with the explicit tag %s", node.Line, value, node.Tag)
}

// implicitScalar converts the text of a plain scalar as js-yaml's core
// schema resolves it: null, a bool, an int or a float when the text has
// one of their forms, each tried only for the first characters it can start
// with, and a string otherwise.
func implicitScalar(text string) any {
	if text == "" {
		return nil
	}
	switch first := text[0]; {
	case first == '~' || first == 'n' || first == 'N':
		if isYAMLNull(text) {
			return nil
		}
	case first == 't' || first == 'T' || first == 'f' || first == 'F':
		if boolean, ok := yamlBool(text); ok {
			return boolean
		}
	case first == '-' || first == '+' || '0' <= first && first <= '9':
		if yamlImplicitInt.MatchString(text) {
			if number, ok := yamlInt(text); ok {
				return number
			}
		}
		if number, ok := yamlFloatValue(text); ok {
			return number
		}
	case first == '.':
		if number, ok := yamlFloatValue(text); ok {
			return number
		}
	}
	return text
}

// isYAMLNull reports whether text is a null of js-yaml's core schema.
func isYAMLNull(text string) bool {
	switch text {
	case "", "~", jsNull, "Null", "NULL":
		return true
	}
	return false
}

// yamlBool returns the bool of js-yaml's core schema that text spells, and
// whether it spells one.
func yamlBool(text string) (bool, bool) {
	switch text {
	case "true", "True", "TRUE":
		return true, true
	case "false", "False", "FALSE":
		return false, true
	}
	return false, false
}

// yamlInt returns the number that text, which matches one of the int tag's
// patterns, holds, as js-yaml's parseYamlInteger reads it: a sign, then a
// binary, octal or hexadecimal number after 0b, 0o or 0x, or a decimal one,
// rounded to the nearest float64. A number too large for a float64 is no
// int, as in js-yaml, which leaves it to the next tag.
func yamlInt(text string) (float64, bool) {
	sign := 1.0
	switch text[0] {
	case '-':
		sign, text = -1, text[1:]
	case '+':
		text = text[1:]
	}
	const decimal = 10
	base := decimal
	for _, prefix := range []struct {
		text string
		base int
	}{{"0b", 2}, {"0o", 8}, {"0x", 16}} {
		if strings.HasPrefix(text, prefix.text) {
			text, base = text[len(prefix.text):], prefix.base
			break
		}
	}
	var number float64
	if base == decimal {
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return 0, false
		}
		number = parsed
	} else {
		integer, ok := new(big.Int).SetString(text, base)
		if !ok {
			return 0, false
		}
		number, _ = new(big.Float).SetInt(integer).Float64()
	}
	if math.IsInf(number, 0) {
		return 0, false
	}
	return sign * number, true
}

// yamlFloatValue returns the number that text holds as a float of js-yaml's
// core schema, and whether it holds one: a decimal number, .inf or .nan,
// in the cases that the pattern allows. A number too large for a float64 is
// no float, as in js-yaml, so the text stays a string.
func yamlFloatValue(text string) (float64, bool) {
	if !yamlFloat.MatchString(text) {
		return 0, false
	}
	lower := strings.ToLower(text)
	sign := 1.0
	switch lower[0] {
	case '-':
		sign, lower = -1, lower[1:]
	case '+':
		lower = lower[1:]
	}
	switch lower {
	case ".inf":
		return math.Inf(int(sign)), true
	case ".nan":
		return math.NaN(), true
	}
	number, err := strconv.ParseFloat(lower, 64)
	if (err != nil && !errors.Is(err, strconv.ErrRange)) || math.IsInf(number, 0) {
		return 0, false
	}
	return sign * number, true
}

// jsString returns the string that JavaScript's String gives value, a
// scalar's value, as the key of an object, and false for a mapping or a
// sequence, which js-yaml's objects do not take as keys.
func jsString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case nil:
		return jsNull, true
	case bool:
		return strconv.FormatBool(typed), true
	case float64:
		return jsNumberString(typed), true
	}
	return "", false
}

// jsNumberString returns the string that JavaScript's Number.prototype
// .toString gives number: the shortest digits that read back as it, in
// plain notation from 1e-7 up to 1e21 and in exponential notation outside,
// with NaN, Infinity and -Infinity, and 0 for -0.
func jsNumberString(number float64) string {
	switch {
	case math.IsNaN(number):
		return "NaN"
	case math.IsInf(number, 1):
		return "Infinity"
	case math.IsInf(number, -1):
		return "-Infinity"
	case number == 0:
		return "0"
	case number < 0:
		return "-" + jsNumberString(-number)
	}
	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(number, 'e', -1, 64), "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	power, _ := strconv.Atoi(exponent) //nolint:errcheck // FormatFloat writes a valid exponent.
	// The number is 0.digits times 10 to the power point.
	point, count := power+1, len(digits)
	switch {
	case count <= point && point <= 21:
		return digits + strings.Repeat("0", point-count)
	case 0 < point && point <= 21:
		return digits[:point] + "." + digits[point:]
	case -6 < point && point <= 0:
		return "0." + strings.Repeat("0", -point) + digits
	}
	sign := "+"
	if point-1 < 0 {
		sign = "-"
	}
	shown := digits[:1]
	if count > 1 {
		shown += "." + digits[1:]
	}
	return shown + "e" + sign + strconv.Itoa(int(math.Abs(float64(point-1))))
}
