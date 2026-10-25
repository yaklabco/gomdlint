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
// a leading line break and fails on a tab before one. Pointers and
// interfaces stand for the values they hold, and a nil one for null.
func YAMLNode(value any, flow bool) (*yaml.Node, error) {
	null := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface {
		if reflected.IsNil() {
			return null, nil
		}
		reflected = reflected.Elem()
	}
	if !reflected.IsValid() {
		return null, nil // value is nil.
	}
	style := yaml.Style(0)
	if flow {
		style = yaml.FlowStyle
	}
	switch reflected.Kind() {
	case reflect.String:
		return stringNode(reflected.String()), nil
	case reflect.Slice, reflect.Array:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: style}
		for idx := range reflected.Len() {
			item, err := YAMLNode(reflected.Index(idx).Interface(), flow)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, item)
		}
		return node, nil
	case reflect.Map:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: style}
		keys := reflected.MapKeys()
		slices.SortFunc(keys, func(left, right reflect.Value) int {
			return cmp.Compare(fmt.Sprint(left.Interface()), fmt.Sprint(right.Interface()))
		})
		for _, key := range keys {
			keyNode, err := YAMLNode(key.Interface(), flow)
			if err != nil {
				return nil, err
			}
			item, err := YAMLNode(reflected.MapIndex(key).Interface(), flow)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, keyNode, item)
		}
		return node, nil
	default:
	}
	var node yaml.Node
	if err := node.Encode(reflected.Interface()); err != nil {
		return nil, fmt.Errorf("encode %v as YAML: %w", value, err)
	}
	return &node, nil
}

// stringNode returns the YAML node of text, a string (see YAMLNode).
func stringNode(text string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: text}
	// The encoder writes << as it is, which YAML reads as a merge key.
	if text == "<<" || strings.ContainsFunc(text, needsEscape) {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}

// needsEscape reports whether char cannot stand as it is on a line of YAML
// that a person reads: a character other than a space that does not print,
// such as a line break, a tab, a line or paragraph separator or a
// zero-width space. In YAML's double quotes, the encoder escapes the line
// breaks and tabs, and the others stand on the one line as they are.
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
		// JSON leaves NEL as it is, which YAML reads as a line break.
		return strings.ReplaceAll(string(encoded), "\u0085", `\u0085`)
	}
	return strconv.Quote(fmt.Sprint(value))
}
