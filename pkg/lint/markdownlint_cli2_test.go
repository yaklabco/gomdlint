package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestReadMarkdownlintCLI2Options covers markdownlint-cli2 options files,
// which markdownlint-cli2 0.23.3 reads with the one parser that the name
// selects: JSON with comments and trailing commas for
// .markdownlint-cli2.jsonc, and YAML for .markdownlint-cli2.yaml. The
// markdownlint configuration under the config key keeps the order of its
// keys, which markdownlint applies in order, and a value other than an
// object there holds no settings, as markdownlint-cli2 then lints with
// markdownlint's defaults. A key written twice in JSON takes its last
// value.
func TestReadMarkdownlintCLI2Options(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		syntax    lint.MarkdownlintSyntax
		keys      []string
		config    []string // the keys of the config key's settings, nil without one
		notObject string   // what the config key holds in place of an object
		file      string   // what the file holds in place of an object
	}{
		{
			name: "JSON with comments", syntax: lint.MarkdownlintJSONC,
			text: `{"globs": ["*.md"], /* c */ "config": {"MD013": false, "whitespace": true,},}`,
			keys: []string{"globs", "config"}, config: []string{"MD013", "whitespace"},
		},
		{
			name: "JSON key written twice", syntax: lint.MarkdownlintJSONC,
			text: `{"config": {"MD013": false}, "fix": true, "config": {"MD009": false, "MD010": false}}`,
			keys: []string{"config", "fix"}, config: []string{"MD009", "MD010"},
		},
		{
			name: "JSON config list", syntax: lint.MarkdownlintJSONC, text: `{"config": [1]}`,
			keys: []string{"config"}, config: []string{}, notObject: "a list",
		},
		{name: "JSON config null", syntax: lint.MarkdownlintJSONC, text: `{"config": null}`, keys: []string{"config"}, config: []string{}},
		{name: "JSON without config", syntax: lint.MarkdownlintJSONC, text: `{"ignores": []}`, keys: []string{"ignores"}},
		{name: "JSON list", syntax: lint.MarkdownlintJSONC, text: `[1]`, file: "a list"},
		{name: "JSON null", syntax: lint.MarkdownlintJSONC, text: `null`},
		{
			name: "YAML", syntax: lint.MarkdownlintYAML,
			text: "config:\n  MD009: true\n  whitespace: false\nignores:\n  - x.md\n",
			keys: []string{"config", "ignores"}, config: []string{"MD009", "whitespace"},
		},
		{
			name: "YAML alias", syntax: lint.MarkdownlintYAML,
			text: "base: &b\n  MD013: false\n  MD009: true\nconfig: *b\n",
			keys: []string{"base", "config"}, config: []string{"MD013", "MD009"},
		},
		{
			name: "YAML holding JSON", syntax: lint.MarkdownlintYAML, text: `{"config": {"b": 1, "a": 2}}`,
			keys: []string{"config"}, config: []string{"b", "a"},
		},
		{
			name: "YAML after a byte order mark", syntax: lint.MarkdownlintYAML, text: "\ufeffconfig:\n  MD013: false\n",
			keys: []string{"config"}, config: []string{"MD013"},
		},
		{
			name: "YAML config number", syntax: lint.MarkdownlintYAML, text: "config: 5\n",
			keys: []string{"config"}, config: []string{}, notObject: "a number",
		},
		{name: "YAML string", syntax: lint.MarkdownlintYAML, text: "text\n", file: "a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			options, err := lint.ReadMarkdownlintCLI2Options([]byte(tt.text), tt.syntax)
			require.NoError(t, err)

			assert.Equal(t, tt.syntax, options.Syntax)
			assert.Equal(t, tt.keys, options.Keys)
			assert.Equal(t, tt.file, options.NotObject)
			if tt.config == nil {
				assert.Nil(t, options.Config)
				return
			}
			require.NotNil(t, options.Config)
			assert.Equal(t, tt.notObject, options.Config.NotObject)
			if len(tt.config) == 0 {
				assert.Empty(t, settingKeys(options.Config.Settings))
			} else {
				assert.Equal(t, tt.config, settingKeys(options.Config.Settings))
			}
		})
	}
}

// TestReadMarkdownlintCLI2OptionsPositions covers the lines of the keys of
// a YAML options file, and of the keys under its config key, which warnings
// about them name, through an alias too.
func TestReadMarkdownlintCLI2OptionsPositions(t *testing.T) {
	t.Parallel()

	options, err := lint.ReadMarkdownlintCLI2Options(
		[]byte("ignores:\n  - x.md\nconfig:\n  MD009: true\n  MD013:\n    line_length: 1\n"), lint.MarkdownlintYAML)
	require.NoError(t, err)
	assert.Equal(t, 1, options.Positions.Key("ignores").Line)
	assert.Equal(t, 3, options.Positions.Key("config").Line)
	assert.Equal(t, 4, options.Config.Positions.Key("MD009").Line)
	assert.Equal(t, 6, options.Config.Positions.Key("MD013").Key("line_length").Line)

	options, err = lint.ReadMarkdownlintCLI2Options(
		[]byte("base: &b\n  MD013: false\nconfig: *b\n"), lint.MarkdownlintYAML)
	require.NoError(t, err)
	assert.Equal(t, 2, options.Config.Positions.Key("MD013").Line, "the line where the alias's anchor writes the key")
}

// TestReadMarkdownlintCLI2OptionsRejectsWhatMarkdownlintCLI2Rejects covers
// options files that markdownlint-cli2 0.23.3 fails on: text that the one
// parser of the file's name cannot read, such as YAML in a .jsonc file or a
// byte order mark before JSON, and a file that holds no document at all,
// which a markdownlint configuration file may.
func TestReadMarkdownlintCLI2OptionsRejectsWhatMarkdownlintCLI2Rejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		syntax lint.MarkdownlintSyntax
	}{
		{name: "YAML in JSON", text: "config:\n  MD013: false\n", syntax: lint.MarkdownlintJSONC},
		{name: "byte order mark before JSON", text: "\ufeff{}", syntax: lint.MarkdownlintJSONC},
		{name: "empty JSON", text: "", syntax: lint.MarkdownlintJSONC},
		{name: "JSON comment alone", text: "// nothing\n", syntax: lint.MarkdownlintJSONC},
		{name: "unfinished JSON", text: `{"config": `, syntax: lint.MarkdownlintJSONC},
		{name: "empty YAML", text: "", syntax: lint.MarkdownlintYAML},
		{name: "YAML comment alone", text: "# nothing\n", syntax: lint.MarkdownlintYAML},
		{name: "YAML key written twice", text: "config: {}\nconfig: {}\n", syntax: lint.MarkdownlintYAML},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := lint.ReadMarkdownlintCLI2Options([]byte(tt.text), tt.syntax)
			assert.Error(t, err)
		})
	}
}

// TestReadMarkdownlintCLI2OptionsConfigSet covers whether an options file
// sets a markdownlint configuration, which markdownlint-cli2 0.23.3 decides
// by JavaScript's truth of the config key's value: a directory whose
// options file sets none takes the markdownlint config file of the nearest
// parent directory that has one, even when the file has a config key that
// holds null, false, 0 or the empty string.
func TestReadMarkdownlintCLI2OptionsConfigSet(t *testing.T) {
	t.Parallel()

	values := map[string]bool{
		"{}": true, `{"MD013": false}`: true, "[]": true, `"x"`: true, "1": true, "true": true,
		"1e400": true, "-1e400": true, `{"MD013": {"line_length": 1e400}}`: true,
		"null": false, "false": false, "0": false, `""`: false, "-0": false, "0.0": false, "0e5": false,
	}
	for value, set := range values {
		t.Run("JSON "+value, func(t *testing.T) {
			t.Parallel()

			options, err := lint.ReadMarkdownlintCLI2Options([]byte(`{"config": `+value+`}`), lint.MarkdownlintJSONC)
			require.NoError(t, err)
			assert.Equal(t, set, options.ConfigSet)
		})
		t.Run("YAML "+value, func(t *testing.T) {
			t.Parallel()

			options, err := lint.ReadMarkdownlintCLI2Options([]byte("config: "+value+"\n"), lint.MarkdownlintYAML)
			require.NoError(t, err)
			assert.Equal(t, set, options.ConfigSet)
		})
	}

	for _, text := range []string{"{}", `{"globs": ["*.md"]}`, "[]", "null"} {
		t.Run("no config key "+text, func(t *testing.T) {
			t.Parallel()

			options, err := lint.ReadMarkdownlintCLI2Options([]byte(text), lint.MarkdownlintJSONC)
			require.NoError(t, err)
			assert.False(t, options.ConfigSet)
		})
	}
	yamlTexts := map[string]bool{
		"config:\n": false, "config: ~\n": false, "config: .nan\n": false, "config: 0x0\n": false,
		"config: {MD013: false}\n": true, "config: &c []\nx: *c\n": true, "x: &c 1\nconfig: *c\n": true,
	}
	for text, set := range yamlTexts {
		t.Run("YAML "+text, func(t *testing.T) {
			t.Parallel()

			options, err := lint.ReadMarkdownlintCLI2Options([]byte(text), lint.MarkdownlintYAML)
			require.NoError(t, err)
			assert.Equal(t, set, options.ConfigSet)
		})
	}
}
