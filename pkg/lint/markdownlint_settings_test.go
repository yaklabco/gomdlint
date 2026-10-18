package lint_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
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

func TestIsMarkdownlintDefaultKey(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"default", "Default", "DEFAULT", "dEfAuLt"} {
		assert.True(t, lint.IsMarkdownlintDefaultKey(key), key)
	}
	for _, key := range []string{"", "defaults", " default", "MD009", "défault"} {
		assert.False(t, lint.IsMarkdownlintDefaultKey(key), key)
	}
}
