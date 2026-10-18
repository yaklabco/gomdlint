package configloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertMarkdownlintConfig_OptionSpellings checks that markdownlint's
// spellings of options become gomdlint's canonical names in a migrated
// config, an inverted spelling with the opposite value.
func TestConvertMarkdownlintConfig_OptionSpellings(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{
		"MD009": {"code_blocks": true},
		"MD013": {"line_length": 100, "heading_line_length": 90, "code_block_line_length": 120, "tables": false, "headings": true},
		"no-multiple-blanks": {"maximum": 2},
		"MD059": {"prohibited_texts": ["details"]}
	}`)

	assert.Empty(t, result.Warnings)
	assert.Equal(t, map[string]any{"ignore_code_blocks": false}, result.Config.Rules["MD009"].Options)
	assert.Equal(t, map[string]any{
		"max": float64(100), "heading_max": float64(90), "code_block_max": float64(120),
		"ignore_tables": true, "ignore_headings": false,
	}, result.Config.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"max_consecutive": float64(2)}, result.Config.Rules["MD012"].Options)
	assert.Equal(t, map[string]any{"patterns": []any{"details"}}, result.Config.Rules["MD059"].Options)
}

// TestConvertMarkdownlintConfig_OptionsKeepDefaults checks that migration
// writes only the options that the source sets: every rule's defaults are
// markdownlint's, so options left out need no value.
func TestConvertMarkdownlintConfig_OptionsKeepDefaults(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{"MD013": true, "MD009": {"enabled": true}}`)

	assert.Nil(t, result.Config.Rules["MD013"].Options)
	assert.Nil(t, result.Config.Rules["MD009"].Options)
	for id, baseline := range result.Config.RuleDefaults {
		assert.Nil(t, baseline.Options, "rule_defaults.%s", id)
	}
}

// TestConvertMarkdownlintConfig_OptionWarnings checks the warnings about
// options that a rule cannot use. markdownlint ignores them, so migration
// leaves them out, but a typo gets the closest spelling. A value that the
// option does not allow is kept, as rules treat it as markdownlint does.
func TestConvertMarkdownlintConfig_OptionWarnings(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{
		"MD013": {"lin_length": 100, "strict": "yes"},
		"MD004": {"style": "dashes"},
		"MD005": {"indent": 2}
	}`)

	assert.Equal(t, []string{
		`unknown option "lin_length" for line-length (did you mean "line_length"?); it is ignored`,
		`line-length option "strict" must be true or false, not "yes"; it is ignored`,
		`unordered-list-style option "style" must be one of "consistent", "asterisk", "plus", "dash" or "sublist", ` +
			`not "dashes" (did you mean "dash"?)`,
		`unknown option "indent" for list-indent (it has no options); it is ignored`,
	}, result.Warnings, "in the order the keys are written")
	assert.Nil(t, result.Config.Rules["MD013"].Options)
	assert.Nil(t, result.Config.Rules["MD005"].Options)
	assert.Equal(t, map[string]any{"style": "dashes"}, result.Config.Rules["MD004"].Options)
}

// TestConvertMarkdownlintConfig_TagOptions checks options under a tag,
// which markdownlint gives every rule the tag covers. Each rule keeps the
// options it reads, and an option gets a warning only when none of the
// rules reads it.
func TestConvertMarkdownlintConfig_TagOptions(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{"whitespace": {"br_spaces": 3, "br_spcaes": 4}}`)

	assert.Equal(t, []string{
		`unknown option "br_spcaes" for the rules of the tag "whitespace"; it is ignored`,
	}, result.Warnings)
	assert.Equal(t, map[string]any{"br_spaces": float64(3)}, result.Config.Rules["MD009"].Options)
	assert.Nil(t, result.Config.Rules["MD010"].Options, "an option of another rule configures nothing")
}

// TestConvertMarkdownlintConfig_OptionWarningLines checks that a warning
// about an option of a markdownlint config knows the line of the source
// file that it is about: the line of the option's key, or, for an item of
// a list, the item's line, in YAML and in JSON with comments.
func TestConvertMarkdownlintConfig_OptionWarningLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, file, content string
		warnings            []string
		lines               []int
	}{
		{
			name: "YAML", file: ".markdownlint.yaml", content: "MD013:\n  line_length: oops\n",
			warnings: []string{`line-length option "line_length" must be an integer, not "oops"; it is ignored`},
			lines:    []int{2},
		},
		{
			name: "YAML list item", file: ".markdownlint.yml", content: "MD022:\n  lines_above:\n    - 1\n    - 2.5\n",
			warnings: []string{
				`heading-blank-lines option "lines_above" item 2 must be an integer, not 2.5; the option is ignored`,
			},
			lines: []int{4},
		},
		{
			name: "YAML merged option", file: ".markdownlint.yaml",
			content: "base: &length\n  line_length: oops\nMD013:\n  <<: *length\n  tables: false\n",
			warnings: []string{
				`unknown rule "base"; it is ignored`,
				`line-length option "line_length" must be an integer, not "oops"; it is ignored`,
			},
			lines: []int{0, 2},
		},
		{
			name: "JSON with comments", file: ".markdownlint.jsonc",
			content: "{\n  /* A comment\n     over two lines */\n  \"MD013\": {\n    \"stirct\": true,\n" +
				"    \"line_length\": 80\n  },\n  // A tag.\n  \"whitespace\": {\"br_spcaes\": 3}\n}\n",
			warnings: []string{
				`unknown option "stirct" for line-length (did you mean "strict"?); it is ignored`,
				`unknown option "br_spcaes" for the rules of the tag "whitespace"; it is ignored`,
			},
			lines: []int{5, 9},
		},
		{
			name: "JSON key written twice", file: ".markdownlint.json",
			content:  "{\n  \"MD013\": {\"tables\": 1},\n  \"MD013\": {\n    \"tables\": \"no\"\n  }\n}\n",
			warnings: []string{`line-length option "tables" must be true or false, not "no"; it is ignored`},
			lines:    []int{4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := convertFile(t, tt.file, tt.content)

			assert.Equal(t, tt.warnings, result.Warnings)
			lines := make([]int, len(result.Warnings))
			for idx := range result.Warnings {
				lines[idx] = result.WarningLine(idx)
			}
			assert.Equal(t, tt.lines, lines)
		})
	}
}

// TestConvertMarkdownlintConfig_OptionConflict checks that two spellings
// of one option fail the migration, as in a gomdlint config file.
func TestConvertMarkdownlintConfig_OptionConflict(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".markdownlint.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"MD013": {"line_length": 100, "max": 90}}`), 0o600))

	_, err := ConvertMarkdownlintConfig(path)

	require.EqualError(t, err, `MD013: "line_length" and "max" both set line-length option "max"; keep one of them`)
}
