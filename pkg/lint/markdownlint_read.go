package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/jsonc"
)

// MarkdownlintSyntax is the syntax in which ReadMarkdownlintConfig read the
// text of a markdownlint configuration.
type MarkdownlintSyntax int

const (
	// MarkdownlintJSONC is JSON with comments and trailing commas, as
	// markdownlint-cli2's JSONC parser reads it (see jsonc.Strip).
	MarkdownlintJSONC MarkdownlintSyntax = iota + 1

	// MarkdownlintYAML is YAML.
	MarkdownlintYAML
)

// MarkdownlintSource is a markdownlint configuration that
// ReadMarkdownlintConfig read from its text.
type MarkdownlintSource struct {
	// Settings are the keys of the configuration object with their
	// values, in order (see MarkdownlintSettings). They are empty when the
	// text holds no object.
	Settings *MarkdownlintSettings

	// Syntax is the syntax in which the text was read.
	Syntax MarkdownlintSyntax

	// NotObject describes the value that the text holds in place of an
	// object, such as "a list", which markdownlint reads as an object
	// without keys (see NotObjectText). It is empty when the text holds an
	// object, null or nothing.
	NotObject string
}

// MarkdownlintSyntaxError reports text that ReadMarkdownlintConfig reads
// neither as JSON with comments nor as YAML.
type MarkdownlintSyntaxError struct {
	// JSON is what reading the text as JSON with comments reported.
	JSON error

	// YAML is what reading the text as YAML reported.
	YAML error
}

// Error describes both errors.
func (e *MarkdownlintSyntaxError) Error() string {
	return fmt.Sprintf("neither JSON (%v) nor YAML (%v)", e.JSON, e.YAML)
}

// Unwrap returns both errors.
func (e *MarkdownlintSyntaxError) Unwrap() []error {
	return []error{e.JSON, e.YAML}
}

// The kinds of values that MarkdownlintSource.NotObject describes.
const (
	kindList    = "a list"
	kindString  = "a string"
	kindNumber  = "a number"
	kindBoolean = "a boolean"
	kindOther   = "a value"
)

// NotObjectText describes notObject, the value that a markdownlint
// configuration holds in place of an object (MarkdownlintSource.NotObject),
// for a warning, as in "a list instead of an object of settings; it is
// ignored".
func NotObjectText(notObject string) string {
	return notObject + " instead of an object of settings; it is ignored"
}

// ReadMarkdownlintConfig reads text, the text of a markdownlint
// configuration, as markdownlint-cli2 0.23.3 reads a configuration file
// that it finds, such as .markdownlint.json or .markdownlint.yaml, whatever
// its name says, and as markdownlint 0.41.1, which markdownlint-cli2 runs,
// reads the parameter of a markdownlint-configure-file comment: with
// markdownlint-cli2's parsers in turn, taking what the first that succeeds
// reads (parseConfiguration in markdownlint's lib/parse-configuration.mjs).
//
// The first parser reads JSON with comments and trailing commas (see
// jsonc.Strip), which DecodeMarkdownlintJSON decodes in the order of its
// keys, so a key written twice keeps its first place and takes its last
// value. Text that holds nothing but whitespace and comments holds no
// settings, as markdownlint-cli2 reads it. Failing that, the text is read
// as YAML, with the keys of a mapping in the order of yamlKeyOrder. YAML
// rejects a key written twice. markdownlint-cli2 tries TOML between the
// two, which gomdlint does not read.
//
// A value other than an object, such as a list, a string or a number,
// holds no settings, since markdownlint reads it as an object without
// keys, and NotObject describes it. Null and empty text hold no settings
// either. Text that neither parser reads is a *MarkdownlintSyntaxError. A
// byte order mark is the caller's to skip.
func ReadMarkdownlintConfig(text []byte) (*MarkdownlintSource, error) {
	stripped := jsonc.Strip(text)
	if len(bytes.TrimLeft(stripped, " \t\r\n")) == 0 {
		return &MarkdownlintSource{Settings: &MarkdownlintSettings{}, Syntax: MarkdownlintJSONC}, nil
	}
	jsonErr := json.Unmarshal(stripped, new(json.RawMessage))
	if jsonErr == nil {
		return readMarkdownlintJSON(stripped)
	}
	source, yamlErr := readMarkdownlintYAML(text)
	if yamlErr != nil {
		return nil, &MarkdownlintSyntaxError{JSON: jsonErr, YAML: yamlErr}
	}
	return source, nil
}

// readMarkdownlintJSON reads text, one valid JSON value, as a markdownlint
// configuration.
func readMarkdownlintJSON(text []byte) (*MarkdownlintSource, error) {
	source := &MarkdownlintSource{Settings: &MarkdownlintSettings{}, Syntax: MarkdownlintJSONC}
	settings, err := DecodeMarkdownlintJSON(text)
	switch {
	case errors.Is(err, ErrNotJSONObject):
		source.NotObject = jsonValueKind(text)
	case err != nil:
		return nil, err
	default:
		source.Settings = settings
	}
	return source, nil
}

// jsonValueKind describes the kind of the value that text, one valid JSON
// value that is neither an object nor null, holds.
func jsonValueKind(text []byte) string {
	switch bytes.TrimLeft(text, " \t\r\n")[0] {
	case '[':
		return kindList
	case '"':
		return kindString
	case 't', 'f':
		return kindBoolean
	default:
		return kindNumber
	}
}

// errMoreDocuments reports YAML text that holds more than one document.
var errMoreDocuments = errors.New("expected one document, but found more")

// readMarkdownlintYAML reads text as a markdownlint configuration in YAML.
// As js-yaml's load, which markdownlint-cli2 calls, the text must hold one
// document, or none, and nothing after it but comments and an end marker:
// yaml.Unmarshal would ignore a second document, or text after a mapping
// in flow style, such as {"MD013": false} trailing.
func readMarkdownlintYAML(text []byte) (*MarkdownlintSource, error) {
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
	root := doc.Content[0]
	switch {
	case root.Kind == yaml.MappingNode:
		settings, err := markdownlintYAMLSettings(root)
		if err != nil {
			return nil, err
		}
		source.Settings = settings
	case root.Kind == yaml.SequenceNode:
		source.NotObject = kindList
	case root.ShortTag() != "!!null":
		source.NotObject = yamlScalarKind(root)
	}
	return source, nil
}

// yamlScalarKind describes the kind of the value of node, a YAML scalar
// that is not null.
func yamlScalarKind(node *yaml.Node) string {
	switch node.ShortTag() {
	case "!!str":
		return kindString
	case "!!int", "!!float":
		return kindNumber
	case "!!bool":
		return kindBoolean
	default:
		return kindOther
	}
}

// markdownlintYAMLSettings reads root, the mapping of a markdownlint config
// in YAML, into its settings: the keys of the mapping in the order of
// yamlKeyOrder, with the values that decoding gives them. Decoding rejects
// a key written twice.
func markdownlintYAMLSettings(root *yaml.Node) (*MarkdownlintSettings, error) {
	var values map[string]any
	if err := root.Decode(&values); err != nil {
		return nil, err //nolint:wrapcheck // The caller says what it read.
	}
	settings := &MarkdownlintSettings{}
	for _, key := range yamlKeyOrder(root) {
		if value, ok := values[key]; ok {
			settings.Set(key, value)
			delete(values, key)
		}
	}
	// The order holds every key that decoding reads. Should one be missing,
	// it still applies, after the others.
	for _, key := range slices.Sorted(maps.Keys(values)) {
		settings.Set(key, values[key])
	}
	return settings, nil
}

// yamlKeyOrder returns the keys of node, a YAML mapping or an alias of one,
// in the order in which js-yaml 4 adds them to an object: each key written
// in the mapping in its place, and the keys of the mappings that a merge key
// (<<) brings in, in their own order, in the place of the merge key. A key
// comes once, in its first place, which keeps it when a later entry
// replaces its value. Decoding gives a key written in the mapping the value
// written with it, and a merged key the value of the first mapping that
// brings it in, as js-yaml 4 does too.
//
// Each mapping is walked once. Its keys, and those of the mappings it
// merges, have their places once it has been walked, so a later merge of it
// adds nothing, and a mapping that merges itself ends the walk there.
func yamlKeyOrder(node *yaml.Node) []string {
	var keys []string
	seen := make(map[string]bool)
	walked := make(map[*yaml.Node]bool)
	var walk func(node *yaml.Node)
	walk = func(node *yaml.Node) {
		for node != nil && node.Kind == yaml.AliasNode {
			node = node.Alias
		}
		if node == nil || node.Kind != yaml.MappingNode || walked[node] {
			return
		}
		walked[node] = true

		for idx := 0; idx+1 < len(node.Content); idx += 2 {
			key, value := node.Content[idx], node.Content[idx+1]
			if isYAMLMergeKey(key) {
				sources := []*yaml.Node{value}
				if value.Kind == yaml.SequenceNode {
					sources = value.Content
				}
				for _, source := range sources {
					walk(source)
				}
				continue
			}
			var name string
			if key.Decode(&name) == nil && !seen[name] {
				seen[name] = true
				keys = append(keys, name)
			}
		}
	}
	walk(node)
	return keys
}

// isYAMLMergeKey reports whether key is a merge key, as YAML decoding
// tells one: the plain scalar <<, or << tagged !!merge. A quoted "<<" is an
// ordinary key.
func isYAMLMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.Value == "<<" && key.ShortTag() == "!!merge"
}
