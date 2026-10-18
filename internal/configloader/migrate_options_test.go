package configloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
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
	assert.Equal(t, map[string]any{"ignore_code_blocks": false, "br_spaces": 2}, result.Config.Rules["MD009"].Options)
	assert.Equal(t, map[string]any{
		"max": float64(100), "heading_max": float64(90), "code_block_max": float64(120),
		"ignore_tables": true, "ignore_headings": false,
		"ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true,
	}, result.Config.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"max_consecutive": float64(2)}, result.Config.Rules["MD012"].Options)
	assert.Equal(t, map[string]any{"patterns": []any{"details"}, "markdownlint_compatible": true},
		result.Config.Rules["MD059"].Options)
}

// TestConvertMarkdownlintConfig_OptionsWrittenOut checks that, as #42's
// migration did, every setting of some rules writes out markdownlint's
// defaults for some of their options, in rules and in rule_defaults, so
// that the setting replaces them in a lower configuration layer, and that
// a setting of any other rule writes no option that the source leaves out.
func TestConvertMarkdownlintConfig_OptionsWrittenOut(t *testing.T) {
	t.Parallel()

	result := convertSettings(t, `{"MD013": true, "MD009": {"enabled": true}, "MD003": true}`)

	md013 := map[string]any{"max": 80, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true}
	md009 := map[string]any{"ignore_code_blocks": true, "br_spaces": 2}
	assert.Equal(t, md013, result.Config.Rules["MD013"].Options)
	assert.Equal(t, md009, result.Config.Rules["MD009"].Options)
	assert.Nil(t, result.Config.Rules["MD003"].Options)
	assert.Equal(t, md013, result.Config.RuleDefaults["MD013"].Options)
	assert.Equal(t, md009, result.Config.RuleDefaults["MD009"].Options)
	assert.Nil(t, result.Config.RuleDefaults["MD003"].Options)
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
	assert.Equal(t, map[string]any{
		"max": 80, "ignore_code_blocks": false, "ignore_urls": false, "markdownlint_compatible": true,
	}, result.Config.Rules["MD013"].Options, "the options that every setting of the rule writes out")
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
	assert.Equal(t, map[string]any{"br_spaces": float64(3), "ignore_code_blocks": true}, result.Config.Rules["MD009"].Options)
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
			// js-yaml 5, which markdownlint-cli2 reads YAML with, reads a
			// merge key as an ordinary key, here an option that line-length
			// does not read, so the merged line_length is never read.
			name: "YAML merge key", file: ".markdownlint.yaml",
			content: "base: &length\n  line_length: oops\nMD013:\n  <<: *length\n  tables: false\n",
			warnings: []string{
				`unknown rule "base"; it is ignored`,
				`unknown option "<<" for line-length; it is ignored`,
			},
			lines: []int{0, 4},
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
		{
			name: "JSON option written twice", file: ".markdownlint.json",
			content:  "{\n  \"MD013\": {\n    \"tables\": 1,\n    \"tables\": \"no\"\n  }\n}\n",
			warnings: []string{`line-length option "tables" must be true or false, not "no"; it is ignored`},
			lines:    []int{4},
		},
		{
			name: "YAML option beside a merge key", file: ".markdownlint.yaml",
			content: "base: &length\n  line_length: 1\nMD013:\n  <<: *length\n  line_length: oops\n",
			warnings: []string{
				`unknown rule "base"; it is ignored`,
				`unknown option "<<" for line-length; it is ignored`,
				`line-length option "line_length" must be an integer, not "oops"; it is ignored`,
			},
			lines: []int{0, 4, 5},
		},
		{
			name: "YAML merge key with two mappings", file: ".markdownlint.yaml",
			content: "earlier: &earlier\n  line_length: oops\nlater: &later\n  line_length: 1\n" +
				"MD013:\n  <<: [*earlier, *later]\n",
			warnings: []string{
				`unknown rule "earlier"; it is ignored`,
				`unknown rule "later"; it is ignored`,
				`unknown option "<<" for line-length; it is ignored`,
			},
			lines: []int{0, 0, 6},
		},
		{
			name: "YAML merge key with a merged mapping", file: ".markdownlint.yaml",
			content: "inner: &inner\n  line_length: oops\nouter: &outer\n  <<: *inner\n  tables: false\n" +
				"MD013:\n  <<: [*outer, {line_length: 1}]\n",
			warnings: []string{
				`unknown rule "inner"; it is ignored`,
				`unknown rule "outer"; it is ignored`,
				`unknown option "<<" for line-length; it is ignored`,
			},
			lines: []int{0, 0, 7},
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

// TestSourceLocatorReadsEachEntryOnce guards the cost of placing option
// warnings: the reading that loads the config records where each key is
// written, in one pass, and each warning looks its option up in constant
// time, so placing a warning for every option of a rule takes time linear
// in the size of the config, not quadratic.
func TestSourceLocatorReadsEachEntryOnce(t *testing.T) {
	t.Parallel()

	const options = 300
	var content strings.Builder
	content.WriteString("MD013:\n")
	for idx := range options {
		fmt.Fprintf(&content, "  option_%03d: %d\n", idx, idx)
	}

	read, err := lint.ReadMarkdownlintConfig([]byte(content.String()))
	require.NoError(t, err)
	locate := newSourceLocator(read, []byte(content.String()))
	for idx := range options {
		require.Equal(t, idx+2, locate.optionLine("MD013", fmt.Sprintf("option_%03d", idx), 0))
	}
	assert.Zero(t, locate.optionLine("MD013", "absent", 0))
	assert.Zero(t, locate.optionLine("MD999", "option_000", 0))

	result := convertFile(t, ".markdownlint.yaml", content.String())
	require.Len(t, result.Warnings, options)
	for idx := range result.Warnings {
		assert.Equal(t, idx+2, result.WarningLine(idx), result.Warnings[idx])
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
