package configloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad_OptionSpellings checks that a config file may write options in
// markdownlint's spellings, which load as gomdlint's canonical names, an
// inverted spelling with the opposite value.
func TestLoad_OptionSpellings(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, `rules:
  line-length:
    options:
      line_length: 100
      tables: false
  MD012:
    options:
      maximum: 2
rule_defaults:
  no-trailing-spaces:
    options:
      code_blocks: true
`)
	require.NoError(t, err)

	assert.Empty(t, result.Warnings)
	assert.Equal(t, map[string]any{"max": 100, "ignore_tables": true}, result.Config.Rules["MD013"].Options)
	assert.Equal(t, map[string]any{"max_consecutive": 2}, result.Config.Rules["MD012"].Options)
	assert.Equal(t, map[string]any{"ignore_code_blocks": false}, result.Config.RuleDefaults["MD009"].Options)
}

// TestLoad_OptionSpellingsMergeByOption checks that configuration layers
// merge option by option whatever spelling each writes: a project config
// that sets markdownlint's line_length overrides a user config's max.
func TestLoad_OptionSpellingsMergeByOption(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	user := filepath.Join(dir, "user.yml")
	project := filepath.Join(dir, "project.yml")
	require.NoError(t, os.WriteFile(user, []byte("rules:\n  MD013:\n    options:\n      max: 90\n      strict: true\n"), 0o600))
	require.NoError(t, os.WriteFile(project, []byte("rules:\n  line-length:\n    options:\n      line_length: 100\n"), 0o600))

	userCfg, _, err := loadConfigFile(user)
	require.NoError(t, err)
	projectCfg, _, err := loadConfigFile(project)
	require.NoError(t, err)

	merged := merge(userCfg, projectCfg)
	assert.Equal(t, map[string]any{"max": 100, "strict": true}, merged.Rules["MD013"].Options)
}

// TestLoad_WarnsAboutOptions checks the warnings about rule options that
// a rule cannot use: an unknown option, with the closest spelling, a value
// of the wrong type, which is dropped so the rule uses its default, and a
// value that the option does not allow. Each names the file and line.
func TestLoad_WarnsAboutOptions(t *testing.T) {
	t.Parallel()

	content := `rules:
  line-length:
    options:
      lin_length: 100
      max: 100px
      strict: true
  unordered-list-style:
    options:
      style: dashes
  list-indent:
    options:
      indent: 2
rule_defaults:
  MD009:
    options:
      br_space: 3
  frobnicate:
    options:
      anything: 1
`
	result, err := loadProject(t, content)
	require.NoError(t, err, "option problems do not fail loading")

	assert.Equal(t, []string{
		`.gomdlint.yml:4: unknown option "lin_length" for line-length (did you mean "line_length"?); it is ignored`,
		`.gomdlint.yml:5: line-length option "max" must be an integer, not "100px"; it is ignored`,
		`.gomdlint.yml:9: unordered-list-style option "style" must be one of "consistent", "asterisk", "plus", "dash" or ` +
			`"sublist", not "dashes" (did you mean "dash"?)`,
		`.gomdlint.yml:12: unknown option "indent" for list-indent (it has no options); it is ignored`,
		`.gomdlint.yml:16: unknown option "br_space" for no-trailing-spaces (did you mean "br_spaces"?); it is ignored`,
		`.gomdlint.yml:17: unknown rule "frobnicate"; it is ignored`,
	}, relativeWarnings(result.Warnings, ".gomdlint.yml"))

	assert.Equal(t, map[string]any{"lin_length": 100, "strict": true}, result.Config.Rules["MD013"].Options,
		"the value of the wrong type is dropped")
	assert.Equal(t, map[string]any{"style": "dashes"}, result.Config.Rules["MD004"].Options,
		"a value outside the allowed ones is kept for the rule")
}

// TestLoad_WarnsAboutOptionsInJSON checks the same warnings for a JSON
// config file, with the lines of its keys.
func TestLoad_WarnsAboutOptionsInJSON(t *testing.T) {
	t.Parallel()

	content := `{
  "rules": {
    "MD013": {
      "options": {"tables": "no", "heading_length": 60}
    }
  }
}
`
	result, err := loadProjectFile(t, ".gomdlint.json", content)
	require.NoError(t, err)

	assert.Equal(t, []string{
		`.gomdlint.json:4: unknown option "heading_length" for line-length; it is ignored`,
		`.gomdlint.json:4: line-length option "tables" must be true or false, not "no"; it is ignored`,
	}, relativeWarnings(result.Warnings, ".gomdlint.json"))
}

// TestLoad_RejectsTwoSpellingsOfAnOption checks that setting one option
// under two spellings fails loading, as setting one rule under two keys
// does: either value could win.
func TestLoad_RejectsTwoSpellingsOfAnOption(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, content, want string
	}{
		{
			name:    "rules",
			content: "rules:\n  line-length:\n    options:\n      max: 90\n      line_length: 100\n",
			want:    `rules: "line_length" and "max" both set line-length option "max"; keep one of them`,
		},
		{
			name:    "inverted",
			content: "rule_defaults:\n  MD009:\n    options:\n      code_blocks: true\n      ignore_code_blocks: false\n",
			want:    `rule_defaults: "code_blocks" and "ignore_code_blocks" both set no-trailing-spaces option "ignore_code_blocks"; keep one of them`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadProject(t, tt.content)
			require.Error(t, err)
			assert.True(t, strings.HasSuffix(err.Error(), ".gomdlint.yml: "+tt.want), err.Error())
		})
	}
}
