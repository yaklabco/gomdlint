package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// settingKeys returns the keys of settings in order.
func settingKeys(settings *lint.MarkdownlintSettings) []string {
	var keys []string
	for _, setting := range settings.Settings() {
		keys = append(keys, setting.Key)
	}
	return keys
}

// TestReadMarkdownlintConfig covers the text of markdownlint configurations
// as markdownlint-cli2 0.23.3 reads them, whatever the file's name: as JSON
// with comments and trailing commas first, which keeps a key written twice
// in its first place with its last value, and then as YAML.
func TestReadMarkdownlintConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		syntax lint.MarkdownlintSyntax
		keys   []string
		values []any
	}{
		{
			name: "JSON", text: `{"MD013": false, "whitespace": true}`,
			syntax: lint.MarkdownlintJSONC, keys: []string{"MD013", "whitespace"}, values: []any{false, true},
		},
		{
			name: "JSON with comments and trailing commas", text: "{\n  // c\n  \"MD013\": {\"line_length\": 100,},\n}\n",
			syntax: lint.MarkdownlintJSONC, keys: []string{"MD013"}, values: []any{map[string]any{"line_length": float64(100)}},
		},
		{
			name: "a key written twice", text: `{"MD009": false, "whitespace": true, "MD009": 0}`,
			syntax: lint.MarkdownlintJSONC, keys: []string{"MD009", "whitespace"}, values: []any{float64(0), true},
		},
		{
			name: "YAML", text: "MD013: false\nwhitespace: true\n",
			syntax: lint.MarkdownlintYAML, keys: []string{"MD013", "whitespace"}, values: []any{false, true},
		},
		{
			name: "YAML flow mapping", text: "{MD013: false}",
			syntax: lint.MarkdownlintYAML, keys: []string{"MD013"}, values: []any{false},
		},
		{
			name: "YAML document markers", text: "---\nMD013: false\n...\n# The end.\n",
			syntax: lint.MarkdownlintYAML, keys: []string{"MD013"}, values: []any{false},
		},
		{
			name: "YAML merge key", text: "x: &ws\n  whitespace: false\n<<: *ws\nMD009: true\n",
			syntax: lint.MarkdownlintYAML, keys: []string{"x", "whitespace", "MD009"},
			values: []any{map[string]any{"whitespace": false}, false, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, err := lint.ReadMarkdownlintConfig([]byte(tt.text))
			require.NoError(t, err)

			assert.Equal(t, tt.syntax, source.Syntax)
			assert.Empty(t, source.NotObject)
			assert.Equal(t, tt.keys, settingKeys(source.Settings))
			var values []any
			for _, setting := range source.Settings.Settings() {
				values = append(values, setting.Value)
			}
			assert.Equal(t, tt.values, values)
		})
	}
}

// TestReadMarkdownlintConfigWithoutAnObject covers text that holds no
// configuration object, which markdownlint reads as an object without
// keys: a value of another type, which NotObject describes, and null or
// nothing at all, which need no description.
func TestReadMarkdownlintConfigWithoutAnObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text, notObject string
	}{
		{text: `["MD013"]`, notObject: "a list"},
		{text: `"MD013"`, notObject: "a string"},
		{text: `42`, notObject: "a number"},
		{text: `true`, notObject: "a boolean"},
		{text: "- MD013\n", notObject: "a list"},
		{text: "MD013\n", notObject: "a string"},
		{text: "MD013 = false\n", notObject: "a string"},
		{text: "4.5\n", notObject: "a number"},
		{text: "yes\n", notObject: "a string"},
		{text: "false\n", notObject: "a boolean"},
		{text: "null"},
		{text: ""},
		{text: " \n"},
		{text: "// Nothing yet.\n"},
		{text: "/* Nothing yet. */"},
		{text: "# Nothing yet.\n"},
		{text: "~\n"},
	}

	for _, tt := range tests {
		source, err := lint.ReadMarkdownlintConfig([]byte(tt.text))
		require.NoError(t, err, "%q", tt.text)

		assert.Empty(t, source.Settings.Settings(), "%q", tt.text)
		assert.Equal(t, tt.notObject, source.NotObject, "%q", tt.text)
	}
}

// TestReadMarkdownlintConfigRejectsWhatNeitherParserReads covers text that
// is neither JSON with comments nor YAML: the error carries what each
// parser reported.
func TestReadMarkdownlintConfigRejectsWhatNeitherParserReads(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		`{"MD013": false,,}`,
		`{,}`,
		"{\"MD013\": false}\n,\n",
		"MD013: true\nMD013: false\n",
		"{ invalid json",
		// js-yaml, which markdownlint-cli2 reads YAML with, takes one
		// document and nothing after it but comments.
		"{\"MD013\": false} trailing\n",
		"{\"MD013\": false}\nx\n",
		"MD013: false\n---\nMD009: false\n",
		"MD013: false\n---\n",
	} {
		_, err := lint.ReadMarkdownlintConfig([]byte(text))

		var syntaxErr *lint.MarkdownlintSyntaxError
		require.ErrorAs(t, err, &syntaxErr, "%q", text)
		require.Error(t, syntaxErr.JSON, "%q", text)
		require.Error(t, syntaxErr.YAML, "%q", text)
		assert.Contains(t, err.Error(), syntaxErr.JSON.Error(), "%q", text)
		assert.Contains(t, err.Error(), syntaxErr.YAML.Error(), "%q", text)
	}
}
