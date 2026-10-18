package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// MarkdownlintSetting is one key of a markdownlint configuration object, as
// written, with its value.
type MarkdownlintSetting struct {
	Key   string
	Value any
}

// MarkdownlintSettings holds the keys of a markdownlint configuration object
// with their values, in the order in which markdownlint 0.41.1 applies them:
// the order that the object's keys have in JavaScript. getEffectiveConfig
// (in lib/markdownlint.mjs) applies the keys in that order, each replacing
// what an earlier key set for the rules it names. As in a JavaScript object,
// keys compare exactly, and setting a key that the settings hold already
// replaces its value in its place. JSON.parse reads a key written twice so,
// and markdownlint spreads each markdownlint-configure-file comment over
// its configuration so. JavaScript also moves keys that are array indices,
// such as "1", before the others. No such key names a rule or a tag, so
// they keep their place here. The zero value holds no settings.
type MarkdownlintSettings struct {
	settings []MarkdownlintSetting

	// places maps each key to its index in settings.
	places map[string]int
}

// Set gives key value: in its place when the settings hold key already, and
// after the other keys otherwise.
func (s *MarkdownlintSettings) Set(key string, value any) {
	if place, ok := s.places[key]; ok {
		s.settings[place].Value = value
		return
	}
	if s.places == nil {
		s.places = make(map[string]int)
	}
	s.places[key] = len(s.settings)
	s.settings = append(s.settings, MarkdownlintSetting{Key: key, Value: value})
}

// Settings returns the settings in order.
func (s *MarkdownlintSettings) Settings() []MarkdownlintSetting {
	return slices.Clone(s.settings)
}

// Default returns the value of the default key of the settings, and whether
// they have one: the first key that IsMarkdownlintDefaultKey accepts.
// getEffectiveConfig takes the first such key and ignores any later one,
// and it gives every rule the default before it applies any other key,
// wherever the default key is written.
func (s *MarkdownlintSettings) Default() (any, bool) {
	for _, setting := range s.settings {
		if IsMarkdownlintDefaultKey(setting.Key) {
			return setting.Value, true
		}
	}
	return nil, false
}

// IsMarkdownlintDefaultKey reports whether key is the default key of a
// markdownlint configuration object, which gives every rule its baseline:
// whether markdownlint reads it as default (see markdownlintKey), as
// getEffectiveConfig does when it compares a key's JavaScript upper case
// with DEFAULT. So default, Default and DEFAULT all count.
func IsMarkdownlintDefaultKey(key string) bool {
	return markdownlintKey(key) == "default"
}

// MarkdownlintConfigRuleIDs returns the IDs of the registered rules that
// key, a key of a markdownlint configuration file such as
// .markdownlint.json, names, in ID order, and whether markdownlint knows
// the key. markdownlint 0.41.1 looks a key up whole by its JavaScript upper
// case, so a key names what a word of a markdownlint comment with the same
// key names (see markdownlintKey): a rule by its ID, its name or a
// markdownlint name of it, and the rules of a markdownlint tag. In a
// configuration file a tag means markdownlint's rules only, because
// markdownlint never runs gomdlint's own, though a key can still name one
// of those by its ID or name. The tags that rules report themselves, such
// as gomdlint's former tags, are not markdownlint's, so they name nothing
// here. markdownlint ignores a key that names nothing, which is not known,
// unless it is an identifier or tag of the catalog whose rules are not
// registered.
func (r *Registry) MarkdownlintConfigRuleIDs(key string) ([]string, bool) {
	word := r.lookups().words[markdownlintKey(key)]
	if word == nil || (word.rule == nil && !word.catalog) {
		return nil, false
	}
	var ids []string
	if word.rule != nil {
		ids = append(ids, word.rule.ID())
	}
	for _, rule := range word.tagged {
		if !IsGomdlintOnlyRule(rule.ID()) {
			ids = append(ids, rule.ID())
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), true
}

// ErrNotJSONObject reports JSON that holds a value other than an object
// where a markdownlint configuration object belongs.
var ErrNotJSONObject = errors.New("not a JSON object")

// DecodeMarkdownlintJSON reads data, one JSON value, as a markdownlint
// configuration object, into its settings in the order of its keys (see
// MarkdownlintSettings). Values decode as encoding/json decodes into an
// interface value, with numbers as float64, the float64 that JSON.parse
// reads for each: a number beyond float64's range is an infinity, where
// encoding/json fails. A syntax error reads as json.Unmarshal reports it.
// null holds no settings, as markdownlint reads it, and any other value
// that is not an object is an error that wraps ErrNotJSONObject.
func DecodeMarkdownlintJSON(data []byte) (*MarkdownlintSettings, error) {
	// Checking all of data first gives syntax errors json.Unmarshal's
	// wording, and leaves the decoder below only valid JSON to read.
	if err := json.Unmarshal(data, new(json.RawMessage)); err != nil {
		return nil, err //nolint:wrapcheck // The caller names the file or the comment.
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read JSON: %w", err)
	}
	settings := &MarkdownlintSettings{}
	if token == nil {
		return settings, nil
	}
	if token != json.Delim('{') {
		return nil, ErrNotJSONObject
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read JSON: %w", err)
		}
		key, isKey := token.(string)
		if !isKey { // Valid JSON, which the check above ensures, has string keys.
			return nil, fmt.Errorf("read JSON: object key %v is not a string", token)
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("read JSON: %w", err)
		}
		settings.Set(key, javaScriptNumbers(value))
	}
	return settings, nil
}

// javaScriptNumbers returns value, which a decoder decoded with json.Number
// for numbers, with each number, however deep, as the float64 that
// JSON.parse reads: the nearest one, an infinity beyond float64's range,
// and zero below it. It changes the maps and lists of value in place.
func javaScriptNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		number, err := strconv.ParseFloat(typed.String(), 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return value // A JSON number always parses; this is unreachable.
		}
		return number
	case map[string]any:
		for key, item := range typed {
			typed[key] = javaScriptNumbers(item)
		}
	case []any:
		for idx, item := range typed {
			typed[idx] = javaScriptNumbers(item)
		}
	}
	return value
}
