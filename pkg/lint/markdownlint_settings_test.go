package lint_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// TestDecodeMarkdownlintJSON covers the order of a configuration object's
// keys, which markdownlint applies in turn: the order they are written, as
// JSON.parse keeps them, with a key written twice in its first place and
// with its last value.
func TestDecodeMarkdownlintJSON(t *testing.T) {
	t.Parallel()

	settings, err := lint.DecodeMarkdownlintJSON([]byte(
		`{"whitespace": false, "MD013": {"line_length": 100, "tables": false}, "whitespace": true, "": null}`))
	require.NoError(t, err)

	assert.Equal(t, []lint.MarkdownlintSetting{
		{Key: "whitespace", Value: true},
		{Key: "MD013", Value: map[string]any{"line_length": float64(100), "tables": false}},
		{Key: "", Value: nil},
	}, settings.Settings())
}

// TestDecodeMarkdownlintJSONValues covers JSON that holds no object: null
// holds no settings, as markdownlint reads it, while any other value is an
// error, and a syntax error reads as json.Unmarshal reports it.
func TestDecodeMarkdownlintJSONValues(t *testing.T) {
	t.Parallel()

	for _, data := range []string{`null`, " \n{}\t", `{}`} {
		settings, err := lint.DecodeMarkdownlintJSON([]byte(data))
		require.NoError(t, err, data)
		assert.Empty(t, settings.Settings(), data)
	}

	for _, data := range []string{`[]`, `"default"`, `1`, `true`, `[{"MD009": false}]`} {
		_, err := lint.DecodeMarkdownlintJSON([]byte(data))
		require.ErrorIs(t, err, lint.ErrNotJSONObject, data)
	}

	for _, data := range []string{``, `{"MD009":`, `{"MD009": false} {}`, `{"MD009": false,}`, "{'MD009': false}"} {
		_, err := lint.DecodeMarkdownlintJSON([]byte(data))
		want := json.Unmarshal([]byte(data), new(json.RawMessage))
		require.Error(t, want, data)
		require.EqualError(t, err, want.Error(), data)
	}
}

// TestDecodeMarkdownlintJSONNumbers covers numbers beyond the range of a
// float64, which JSON.parse and markdownlint-cli2's JSONC parser read as
// Infinity, or as 0 when too small, so that markdownlint-cli2 loads a
// config that holds one, in a rule's value or deep in its options.
// encoding/json rejects them.
func TestDecodeMarkdownlintJSONNumbers(t *testing.T) {
	t.Parallel()

	settings, err := lint.DecodeMarkdownlintJSON([]byte(
		`{"MD013": 1e400, "MD009": {"br_spaces": -1e400, "list": [1e-400, 2.5]}, "MD010": 12345678901234567890}`))
	require.NoError(t, err)

	values := settings.Settings()
	require.Len(t, values, 3)
	infinity, ok := values[0].Value.(float64)
	require.True(t, ok)
	assert.True(t, math.IsInf(infinity, 1))
	options, ok := values[1].Value.(map[string]any)
	require.True(t, ok)
	negative, ok := options["br_spaces"].(float64)
	require.True(t, ok)
	assert.True(t, math.IsInf(negative, -1))
	assert.Equal(t, []any{float64(0), 2.5}, options["list"])
	assert.InDelta(t, 12345678901234567890.0, values[2].Value, 0)
}

// TestMarkdownlintSettingsSet covers setting keys as JavaScript sets the
// properties of an object: a new key goes last, and a key that the
// settings hold already, compared exactly, keeps its place.
func TestMarkdownlintSettingsSet(t *testing.T) {
	t.Parallel()

	var settings lint.MarkdownlintSettings
	assert.Empty(t, settings.Settings(), "the zero value holds no settings")

	settings.Set("whitespace", false)
	settings.Set("MD009", true)
	settings.Set("WHITESPACE", true)
	settings.Set("whitespace", nil)

	assert.Equal(t, []lint.MarkdownlintSetting{
		{Key: "whitespace", Value: nil},
		{Key: "MD009", Value: true},
		{Key: "WHITESPACE", Value: true},
	}, settings.Settings())
}

// TestMarkdownlintSettingsDefault covers the default key, which
// markdownlint 0.41.1 matches in any case, taking the first one written.
func TestMarkdownlintSettingsDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		data  string
		want  any
		found bool
	}{
		{data: `{"MD009": false}`},
		{data: `{"default": false}`, want: false, found: true},
		{data: `{"MD009": true, "Default": "warning"}`, want: "warning", found: true},
		{data: `{"DEFAULT": false, "default": true}`, want: false, found: true},
		{data: `{"defaults": false, " default": false, "default ": false}`},
	}
	for _, tt := range tests {
		settings, err := lint.DecodeMarkdownlintJSON([]byte(tt.data))
		require.NoError(t, err, tt.data)

		value, found := settings.Default()

		assert.Equal(t, tt.found, found, tt.data)
		assert.Equal(t, tt.want, value, tt.data)
	}
}

// TestMarkdownlintConfigRuleIDs covers the keys of a markdownlint
// configuration file, which markdownlint 0.41.1 looks up whole by their
// JavaScript upper case: a rule's ID, name or a markdownlint name of it,
// or a markdownlint tag, which names markdownlint's rules only, since
// markdownlint never runs gomdlint's own. Former gomdlint tags, keys with
// whitespace and empty keys name nothing.
func TestMarkdownlintConfigRuleIDs(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)

	tests := []struct {
		key   string
		ids   []string
		known bool
	}{
		{key: "MD009", ids: []string{"MD009"}, known: true},
		{key: "No-Trailing-Spaces", ids: []string{"MD009"}, known: true},
		{key: "ul-style", ids: []string{"MD004"}, known: true},
		{key: "unordered-list-style", ids: []string{"MD004"}, known: true},
		{key: "no-trailing-ſpaces", ids: []string{"MD009"}, known: true},
		{key: "MDL001", ids: []string{"MDL001"}, known: true},
		{key: "links", ids: []string{"MD011", "MD034", "MD039", "MD042", "MD051", "MD052", "MD053", "MD054", "MD059"}, known: true},
		{key: "TABLE", ids: []string{"MD055", "MD056", "MD058", "MD060"}, known: true},
		{key: "NO-TRAİLING-SPACES"},
		{key: "style"},
		{key: " MD009"},
		{key: "MD009 MD010"},
		{key: ""},
		{key: "default"},
	}
	for _, tt := range tests {
		ids, known := registry.MarkdownlintConfigRuleIDs(tt.key)

		assert.Equal(t, tt.known, known, tt.key)
		assert.Equal(t, tt.ids, ids, tt.key)
	}
}

func TestIsMarkdownlintDefaultKey(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"default", "Default", "DEFAULT", "dEfAuLt"} {
		assert.True(t, lint.IsMarkdownlintDefaultKey(key), key)
	}
	for _, key := range []string{"", "defaults", " default", "MD009", "défault"} {
		assert.False(t, lint.IsMarkdownlintDefaultKey(key), key)
	}
}
