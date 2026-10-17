package configloader

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// utf8BOM is the byte order mark some editors write at the start of a file.
var utf8BOM = []byte("\xef\xbb\xbf") //nolint:gochecknoglobals // Read-only constant bytes.

// errTrailingData reports a JSON config file with more after its value.
var errTrailingData = errors.New("unexpected data after the top-level value")

// HoldsJSON reports whether the gomdlint config file at path must hold
// JSON: whether its name ends in .json, in any letter case. gomdlint reads
// such a file as JSON and every other config file as YAML, so commands
// that write a config file write JSON for such a name.
func HoldsJSON(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json")
}

// jsonDocument parses content, a JSON config file, into a YAML document
// node, so that it decodes, and its keys are checked, exactly as a YAML
// file's are. The content is parsed as JSON, because the YAML decoder reads
// some JSON differently or not at all: it rejects an escaped slash (\/) and
// characters escaped as UTF-16 surrogate pairs, and it reads a missing
// value as null. Every node records its line, for warnings and errors. A
// leading byte order mark is skipped.
func jsonDocument(content []byte) (*yaml.Node, error) {
	text := bytes.TrimPrefix(content, utf8BOM)
	if err := checkJSON(text); err != nil {
		if looksLikeYAML(text) {
			return nil, fmt.Errorf("%w; it looks like YAML; rename it to .yml, or rewrite it as JSON", err)
		}
		return nil, err
	}

	reader := &jsonReader{decoder: json.NewDecoder(bytes.NewReader(text)), content: text, line: 1}
	reader.decoder.UseNumber()
	root, err := reader.value()
	if err != nil {
		return nil, err
	}
	if _, err := reader.decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errTrailingData // checkJSON has ruled this out.
	}
	return &yaml.Node{Kind: yaml.DocumentNode, Line: root.Line, Content: []*yaml.Node{root}}, nil
}

// looksLikeYAML reports whether content, which is not valid JSON, is a YAML
// mapping that does not start like JSON does: a YAML config in a .json
// file, as init and migrate wrote for such a name before they wrote JSON.
// Malformed JSON starts with a brace or bracket, so it does not qualify.
func looksLikeYAML(content []byte) bool {
	trimmed := bytes.TrimLeft(content, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] == '{' || trimmed[0] == '[' {
		return false
	}
	var doc yaml.Node
	return yaml.Unmarshal(content, &doc) == nil && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode
}

// checkJSON returns the first syntax error in content, with its line and
// column, or nil if content is one valid JSON value. Decoding into a raw
// message checks the syntax of all of content and converts nothing, so no
// value, such as a number too large for a float64, fails it.
func checkJSON(content []byte) error {
	var value json.RawMessage
	err := json.Unmarshal(content, &value)
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		return err //nolint:wrapcheck // Nil: a raw message fails only on syntax errors.
	}
	// The error occurred after reading Offset bytes, at the last of them.
	line, column := textPosition(content, int(syntaxErr.Offset)-1)
	return fmt.Errorf("line %d, column %d: %w", line, column, err)
}

// textPosition returns the 1-based line and character column of the byte at
// offset in content. Offsets before the start count as the first byte.
func textPosition(content []byte, offset int) (int, int) {
	before := content[:min(max(offset, 0), len(content))]
	lineStart := bytes.LastIndexByte(before, '\n') + 1
	return bytes.Count(before, []byte{'\n'}) + 1, utf8.RuneCount(before[lineStart:]) + 1
}

// jsonReader builds YAML nodes from the tokens of a decoder over valid JSON.
type jsonReader struct {
	decoder *json.Decoder
	content []byte

	// line is the line of the byte at offset. Tokens come in order, so
	// lines are counted from the last token on.
	offset int
	line   int
}

// token returns the next token and its line. JSON tokens never span lines.
func (r *jsonReader) token() (json.Token, int, error) {
	token, err := r.decoder.Token()
	if err != nil {
		return nil, 0, fmt.Errorf("read JSON: %w", err)
	}
	// The decoder's offset is just past the token, so its last byte is
	// the one before.
	end := int(r.decoder.InputOffset()) - 1
	if end > r.offset {
		r.line += bytes.Count(r.content[r.offset:end], []byte{'\n'})
		r.offset = end
	}
	return token, r.line, nil
}

// value reads the next JSON value as a YAML node. Strings become string
// scalars, whatever they hold. Numbers, booleans and null become plain
// scalars of their JSON text, which YAML resolves to the same values.
func (r *jsonReader) value() (*yaml.Node, error) {
	token, line, err := r.token()
	if err != nil {
		return nil, err
	}

	switch token := token.(type) {
	case json.Delim:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Line: line}
		if token == '{' {
			node.Kind, node.Tag = yaml.MappingNode, "!!map"
		}
		// An object alternates keys, which are strings, and values.
		for r.decoder.More() {
			child, err := r.value()
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		if _, _, err := r.token(); err != nil { // The closing delimiter.
			return nil, err
		}
		return node, nil
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: token, Line: line}, nil
	case json.Number:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: token.String(), Line: line}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(token), Line: line}, nil
	default: // nil, for null.
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null", Line: line}, nil
	}
}
