package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

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
// as YAML, as js-yaml 5.4.1 reads it with its default core schema (see
// readMarkdownlintYAML), with the keys of a mapping in the order they are
// written. YAML rejects a key written twice. markdownlint-cli2 tries TOML
// between the two, which gomdlint does not read.
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
