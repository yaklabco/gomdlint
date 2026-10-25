package ruledocs

import (
	"cmp"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// YAMLNode returns the YAML node of value, a value that a configuration
// holds, such as a rule's option, for the YAML encoder to write: strings as
// strings, which the encoder quotes where YAML would read them as another
// value, such as null, 123 or a: b, and double-quotes, with escapes, where
// they hold a character that a line of YAML cannot hold as it is, such as a
// line break; lists and mappings with their items, in flow style, on one
// line, when flow is set, and in block style otherwise; and mappings with
// their keys in sorted order. Other values, such as booleans and numbers,
// are encoded as they are.
//
// The node is built from value itself, so that strings reach the encoder as
// they are: yaml.Node.Encode reads back what the encoder writes, which drops
// a leading line break and fails on a tab before one.
func YAMLNode(value any, flow bool) (*yaml.Node, error) {
	if text, isString := value.(string); isString {
		return stringNode(text), nil
	}
	reflected := reflect.ValueOf(value)
	style := yaml.Style(0)
	if flow {
		style = yaml.FlowStyle
	}
	switch {
	case value == nil:
	case reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: style}
		for idx := range reflected.Len() {
			item, err := YAMLNode(reflected.Index(idx).Interface(), flow)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, item)
		}
		return node, nil
	case reflected.Kind() == reflect.Map && reflected.Type().Key().Kind() == reflect.String:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: style}
		keys := reflected.MapKeys()
		slices.SortFunc(keys, func(left, right reflect.Value) int { return cmp.Compare(left.String(), right.String()) })
		for _, key := range keys {
			item, err := YAMLNode(reflected.MapIndex(key).Interface(), flow)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, stringNode(key.String()), item)
		}
		return node, nil
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, fmt.Errorf("encode %v as YAML: %w", value, err)
	}
	return &node, nil
}

// stringNode returns the YAML node of text, a string (see YAMLNode).
func stringNode(text string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: text}
	if strings.ContainsFunc(text, needsEscape) {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

// needsEscape reports whether char cannot stand as it is on a line of YAML
// that a person reads: a character other than a space that does not print,
// such as a line break, a tab, a line or paragraph separator or a
// zero-width space. YAML's double quotes escape it.
func needsEscape(char rune) bool {
	return char != ' ' && !unicode.IsPrint(char)
}

// yamlValue writes value as a configuration file holds it, in YAML, on one
// line, as the YAML encoder writes its node (see YAMLNode), such as
// "null", [x] or "first\nsecond", so that YAML reads the line back as
// value, after a key or after a comment marker. A value that the encoder
// cannot write, or that would take more than one line, is written as JSON,
// which YAML reads too, or failing that as a quoted string.
func yamlValue(value any) string {
	if node, err := YAMLNode(value, true); err == nil {
		if encoded, err := yaml.Marshal(node); err == nil {
			if line := strings.TrimSuffix(string(encoded), "\n"); !strings.ContainsAny(line, "\r\n\u0085\u2028\u2029") {
				return line
			}
		}
	}
	if encoded, err := json.Marshal(value); err == nil && utf8.Valid(encoded) {
		return string(encoded)
	}
	return strconv.Quote(fmt.Sprint(value))
}
