package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/jsonc"
)

// MarkdownlintCLI2ConfigKey is the key of a markdownlint-cli2 options file
// that holds a markdownlint configuration.
const MarkdownlintCLI2ConfigKey = "config"

// ErrNoDocument reports an options file that holds no document, which
// markdownlint-cli2 rejects: empty text, or nothing but comments.
var ErrNoDocument = errors.New("expected a document, but the input is empty")

// MarkdownlintCLI2Options is a markdownlint-cli2 options file, such as
// .markdownlint-cli2.jsonc, that ReadMarkdownlintCLI2Options read.
type MarkdownlintCLI2Options struct {
	// Keys are the top-level keys of the file, config among them, in the
	// order in which they are written. A key written twice keeps its first
	// place. They are empty when the file holds no object.
	Keys []string

	// Config is the markdownlint configuration under the config key, with
	// its keys in order, as ReadMarkdownlintConfig reads one: its
	// settings, or none, with NotObject set, when the key holds a value
	// other than an object, with which markdownlint-cli2 lints as without
	// a configuration. For YAML, its Positions are those of the keys under
	// the config key. It is nil when the file has no config key.
	Config *MarkdownlintSource

	// NotObject describes the value that the file holds in place of an
	// object, as MarkdownlintSource.NotObject does.
	NotObject string

	// Syntax is the syntax in which the file was read.
	Syntax MarkdownlintSyntax

	// Positions records where YAML text writes the keys of the file and
	// their values, as MarkdownlintSource.Positions does. It is nil for
	// JSON.
	Positions *MarkdownlintPosition
}

// ReadMarkdownlintCLI2Options reads text, the text of a markdownlint-cli2
// options file, as markdownlint-cli2 0.23.3 reads it: with the one parser
// that the file's name selects, syntax, which is MarkdownlintJSONC for
// .markdownlint-cli2.jsonc and MarkdownlintYAML for .markdownlint-cli2.yaml.
// Each reads as ReadMarkdownlintConfig reads that syntax, so YAML may hold
// JSON, but JSON may not hold YAML. Unlike a markdownlint configuration
// file, an options file must hold a document, and JSON may not start with
// a byte order mark, which markdownlint-cli2's JSONC parser rejects. The
// markdownlint configuration under the config key reads as a markdownlint
// configuration does, with its keys in order.
func ReadMarkdownlintCLI2Options(text []byte, syntax MarkdownlintSyntax) (*MarkdownlintCLI2Options, error) {
	switch syntax {
	case MarkdownlintJSONC:
		return readMarkdownlintCLI2JSON(jsonc.Strip(text))
	case MarkdownlintYAML:
		return readMarkdownlintCLI2YAML(text)
	default:
		return nil, fmt.Errorf("unknown syntax %d", syntax)
	}
}

// readMarkdownlintCLI2JSON reads text, JSON in which jsonc.Strip has
// blanked comments and trailing commas, as an options file.
func readMarkdownlintCLI2JSON(text []byte) (*MarkdownlintCLI2Options, error) {
	// Checking all of text first gives syntax errors json.Unmarshal's
	// wording, and leaves the decoder below only valid JSON to read.
	if err := json.Unmarshal(text, new(json.RawMessage)); err != nil {
		return nil, err //nolint:wrapcheck // The caller names the file.
	}
	options := &MarkdownlintCLI2Options{Syntax: MarkdownlintJSONC}
	decoder := json.NewDecoder(bytes.NewReader(text))
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read JSON: %w", err)
	}
	switch {
	case token == nil:
		return options, nil
	case token != json.Delim('{'):
		options.NotObject = jsonValueKind(text)
		return options, nil
	}

	var config json.RawMessage
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read JSON: %w", err)
		}
		key, isKey := token.(string)
		if !isKey { // Valid JSON, which the check above ensures, has string keys.
			return nil, fmt.Errorf("read JSON: object key %v is not a string", token)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("read JSON: %w", err)
		}
		if !seen[key] {
			seen[key] = true
			options.Keys = append(options.Keys, key)
		}
		if key == MarkdownlintCLI2ConfigKey {
			config = value // A key written twice takes its last value, as in JSON.parse.
		}
	}
	if config != nil {
		if options.Config, err = readMarkdownlintJSON(config); err != nil {
			return nil, err
		}
	}
	return options, nil
}

// readMarkdownlintCLI2YAML reads text as an options file in YAML, as js-yaml
// 5.4.1 reads it for markdownlint-cli2 (see readMarkdownlintYAML).
func readMarkdownlintCLI2YAML(text []byte) (*MarkdownlintCLI2Options, error) {
	doc, converter, err := parseMarkdownlintYAML(text)
	switch {
	case err != nil:
		return nil, err
	case doc == nil:
		return nil, ErrNoDocument
	}
	options := &MarkdownlintCLI2Options{Syntax: MarkdownlintYAML}
	if len(doc.Content) == 0 {
		return options, nil
	}
	root := doc.Content[0]
	converter.keepMappings = true
	settings, place, notObject, err := converter.object(root)
	if err != nil {
		return nil, err
	}
	options.NotObject, options.Positions = notObject, place
	if notObject != "" || root.Kind != yaml.MappingNode {
		return options, nil
	}
	for _, setting := range settings.settings {
		options.Keys = append(options.Keys, setting.Key)
	}

	for idx := 0; idx+1 < len(root.Content); idx += 2 {
		key, err := converter.value(root.Content[idx], root.Content[idx+1])
		if err != nil {
			return nil, err
		}
		if name, ok := jsString(key.value); !ok || name != MarkdownlintCLI2ConfigKey {
			continue
		}
		valueNode := root.Content[idx+1]
		config := &MarkdownlintSource{Settings: &MarkdownlintSettings{}, Syntax: MarkdownlintYAML}
		if converted, ok := converter.done[yamlAliasTarget(valueNode)]; ok && converted.pairs != nil {
			config.Settings, config.Positions = pairsSettings(converted.pairs), converted.place
		} else if value, found := settings.Value(MarkdownlintCLI2ConfigKey); found {
			config.NotObject = notObjectKind(value)
		}
		options.Config = config
		break // YAML rejects a key written twice.
	}
	return options, nil
}
