package configloader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file cover the syntax of markdownlint config files,
// which gomdlint reads as markdownlint-cli2 0.23.3 reads them. Their
// expected values are what markdownlint-cli2 0.23.3, with markdownlint
// 0.41.1, does with each config: whether it loads it, and which rules it
// then runs.

// lineLengthOff reports whether result turns line-length (MD013) off with a
// rule setting of its own, as each config in these tests that loads does.
func lineLengthOff(t *testing.T, result *MigrationResult) bool {
	t.Helper()
	rule, ok := result.Config.Rules["MD013"]
	return ok && rule.Enabled != nil && !*rule.Enabled
}

// TestConvertMarkdownlintConfig_TrailingCommas covers JSON configs with a
// comma after the last member of an object or the last item of an array,
// which markdownlint-cli2 accepts in .markdownlint.jsonc and
// .markdownlint.json alike, with comments around them.
func TestConvertMarkdownlintConfig_TrailingCommas(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ file, content string }{
		{".markdownlint.jsonc", `{"MD013": false,}`},
		{".markdownlint.json", `{"MD013": false,}`},
		{".markdownlint.json", "{\n  \"MD013\": false,\n}\n"},
		{".markdownlint.json", `{"MD013": false, "MD009": [1, 2,],}`},
		{".markdownlint.json", `{"MD013":false,"MD009":{"enabled":true,},}`},
		{".markdownlint.json", "{\"MD013\": false /* a */ , /* b */ }\n"},
		{".markdownlint.jsonc", "{\n  // Long lines are fine.\n  \"MD013\": false, // here\n}\n"},
	} {
		t.Run(tt.file+" "+tt.content, func(t *testing.T) {
			t.Parallel()

			result := convertFile(t, tt.file, tt.content)

			assert.True(t, lineLengthOff(t, result))
			assert.Empty(t, result.Warnings)
		})
	}
}

// TestConvertMarkdownlintConfig_RejectsWhatMarkdownlintCLI2Rejects covers
// JSON configs that markdownlint-cli2 fails to load: a second comma, a
// comma that follows no value, a comma after the top-level object, a
// comment without its end, and a lone slash.
func TestConvertMarkdownlintConfig_RejectsWhatMarkdownlintCLI2Rejects(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ file, content string }{
		{".markdownlint.json", `{"MD013": false,,}`},
		{".markdownlint.json", `{,}`},
		{".markdownlint.json", `{"MD013": [1,,2]}`},
		{".markdownlint.json", `{"MD013": [,]}`},
		{".markdownlint.json", "{\"MD013\": false}\n,\n"},
		{".markdownlint.jsonc", "{\"MD013\": false} /* unterminated\n"},
		{".markdownlint.json", "{\"MD013\": false}\n/*/"},
		{".markdownlint.json", "{\"MD013\": false}/\n"},
	} {
		t.Run(tt.file+" "+tt.content, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), tt.file)
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))

			_, err := ConvertMarkdownlintConfig(path)

			require.Error(t, err)
		})
	}
}

// TestConvertMarkdownlintConfig_OptionWarningLinesWithTrailingCommas covers
// the lines of option warnings in a JSON config with comments and trailing
// commas, which keep their places.
func TestConvertMarkdownlintConfig_OptionWarningLinesWithTrailingCommas(t *testing.T) {
	t.Parallel()

	content := "{\n" + // 1
		"  /* Line length,\n" + // 2
		"     with a typo. */\n" + // 3
		"  \"MD013\": {\n" + // 4
		"    \"line_length\": 40,\n" + // 5
		"    \"stirct\": true, // here\n" + // 6
		"  },\n" + // 7
		"}\n" // 8

	result := convertFile(t, ".markdownlint.jsonc", content)

	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], `unknown option "stirct" for line-length`)
	assert.Equal(t, 6, result.WarningLine(0))
	assert.InDelta(t, 40, result.Config.Rules["MD013"].Options["max"], 0)
}

// TestConvertMarkdownlintConfig_ByteOrderMark covers markdownlint configs
// that start with a UTF-8 byte order mark, as some editors write them,
// which markdownlint-cli2 loads in every format. gomdlint skips the mark,
// so the option warnings keep their lines.
func TestConvertMarkdownlintConfig_ByteOrderMark(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ file, content string }{
		{".markdownlint.json", "\ufeff{\"MD013\": false}\n"},
		{".markdownlint.json", "\ufeff{\"MD013\": false,}\n"},
		{".markdownlint.jsonc", "\ufeff{\"MD013\": false}\n"},
		{".markdownlint.yaml", "\ufeffMD013: false\n"},
		{".markdownlint.yml", "\ufeffMD013: false\n"},
	} {
		t.Run(tt.file+" "+tt.content, func(t *testing.T) {
			t.Parallel()

			result := convertFile(t, tt.file, tt.content)

			assert.True(t, lineLengthOff(t, result))
			assert.Empty(t, result.Warnings)
		})
	}

	t.Run("option warning lines", func(t *testing.T) {
		t.Parallel()

		for _, file := range []string{".markdownlint.json", ".markdownlint.yaml"} {
			result := convertFile(t, file, "\ufeff{\n  \"MD013\": {\"stirct\": true}\n}\n")

			require.Len(t, result.Warnings, 1, file)
			assert.Equal(t, 2, result.WarningLine(0), file)
		}
	})
}

// TestConvertMarkdownlintConfig_ByteOrderMarkBeforeComments covers a JSON
// config with a byte order mark and comments. markdownlint-cli2's JSONC
// parser rejects the mark, so markdownlint-cli2 falls back to reading the
// file as YAML, which reads the comment and the key after it as one key
// that names nothing, and leaves line-length (MD013) on. gomdlint skips the
// mark and reads the file as JSON with comments, as written, so the
// setting applies. The README lists this difference.
func TestConvertMarkdownlintConfig_ByteOrderMarkBeforeComments(t *testing.T) {
	t.Parallel()

	result := convertFile(t, ".markdownlint.json", "\ufeff{\n  // Long lines are fine.\n  \"MD013\": false\n}\n")

	assert.True(t, lineLengthOff(t, result))
	assert.Empty(t, result.Warnings)
}

// TestConvertMarkdownlintConfig_JSONInAYAMLFile covers a .markdownlint.yaml
// or .yml file that holds JSON. markdownlint-cli2 reads every config that
// it finds with its JSONC parser first, so it reads such a file as JSON
// with comments and trailing commas: a key written twice keeps its first
// place and takes its last value, where YAML rejects a key written twice.
// The option warnings keep their lines.
func TestConvertMarkdownlintConfig_JSONInAYAMLFile(t *testing.T) {
	t.Parallel()

	t.Run("a key written twice", func(t *testing.T) {
		t.Parallel()

		for _, file := range []string{".markdownlint.yaml", ".markdownlint.yml"} {
			result := convertFile(t, file, `{"MD013": true, "MD009": false, "MD013": false}`)

			assert.True(t, lineLengthOff(t, result), file)
			assert.Empty(t, result.Warnings, file)
		}
	})

	t.Run("the first place holds", func(t *testing.T) {
		t.Parallel()

		// MD009 applies first, then the whitespace tag turns it back on,
		// though MD009 is written again after the tag.
		result := convertFile(t, ".markdownlint.yaml", `{"MD009": false, "whitespace": true, "MD009": false}`)

		rule := result.Config.Rules["MD009"]
		require.NotNil(t, rule.Enabled)
		assert.True(t, *rule.Enabled)
	})

	t.Run("comments, trailing commas and tabs", func(t *testing.T) {
		t.Parallel()

		for _, content := range []string{
			`{"MD013": true, "MD013": false,}`,
			"{\"MD013\": false} // Long lines are fine.\n",
			"{\n\t\"MD013\": false\n}\n",
		} {
			result := convertFile(t, ".markdownlint.yaml", content)

			assert.True(t, lineLengthOff(t, result), content)
			assert.Empty(t, result.Warnings, content)
		}
	})

	t.Run("option warning lines", func(t *testing.T) {
		t.Parallel()

		content := "{\n  \"MD013\": {\"stirct\": true},\n  \"MD013\": {\"line_length\": 40,\n    \"lin_length\": 30},\n}\n"

		result := convertFile(t, ".markdownlint.yaml", content)

		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0], `unknown option "lin_length" for line-length`)
		assert.Equal(t, 4, result.WarningLine(0), "the warning names the value that applies")
	})
}

// TestConvertMarkdownlintConfig_YAMLInAJSONFile covers a .markdownlint.json
// file that holds YAML rather than JSON, which markdownlint-cli2 reads as
// YAML once its JSONC parser rejects it, as gomdlint now does. A flow
// mapping with a key that is not quoted is YAML too, so { invalid json }
// holds the key "invalid json", which names no rule.
func TestConvertMarkdownlintConfig_YAMLInAJSONFile(t *testing.T) {
	t.Parallel()

	for _, content := range []string{"MD013: false\n", "{MD013: false}\n"} {
		result := convertFile(t, ".markdownlint.json", content)

		assert.True(t, lineLengthOff(t, result), content)
		assert.Empty(t, result.Warnings, content)
	}

	result := convertFile(t, ".markdownlint.json", "{ invalid json }\n")

	assert.Empty(t, result.Config.Rules)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], `unknown rule "invalid json"`)
}

// TestConvertMarkdownlintConfig_ValuesOtherThanAnObject covers markdownlint
// configs that hold a list, a string, a number or a boolean.
// markdownlint-cli2 loads each as an object without keys, so every rule
// keeps markdownlint's defaults. gomdlint does the same, with a warning.
func TestConvertMarkdownlintConfig_ValuesOtherThanAnObject(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ file, content, held string }{
		{".markdownlint.json", `["MD013"]`, "a list"},
		{".markdownlint.json", `"MD013"`, "a string"},
		{".markdownlint.jsonc", "// Off:\n42\n", "a number"},
		{".markdownlint.json", `false`, "a boolean"},
		{".markdownlint.yaml", "- MD013\n", "a list"},
		{".markdownlint.yml", "MD013\n", "a string"},
	} {
		t.Run(tt.file+" "+tt.content, func(t *testing.T) {
			t.Parallel()

			result := convertFile(t, tt.file, tt.content)

			assert.Empty(t, result.Config.Rules)
			assert.True(t, *result.Config.RuleDefaults["MD013"].Enabled)
			assert.Equal(t, []string{tt.held + " instead of an object of settings; it is ignored"}, result.Warnings)
			assert.Zero(t, result.WarningLine(0))
		})
	}
}

// TestConvertMarkdownlintConfig_NumbersBeyondFloat64 covers a number too
// large for a float64, which markdownlint-cli2's JSONC parser reads as
// Infinity, so the config loads. gomdlint used to fail it. Infinity turns a
// rule on, as any number but zero does, and is no integer for an option,
// which is ignored with a warning.
func TestConvertMarkdownlintConfig_NumbersBeyondFloat64(t *testing.T) {
	t.Parallel()

	result := convertFile(t, ".markdownlint.json", `{"MD009": 1e400, "MD013": {"line_length": 1e400}}`)

	for _, id := range []string{"MD009", "MD013"} {
		rule := result.Config.Rules[id]
		require.NotNil(t, rule.Enabled, id)
		assert.True(t, *rule.Enabled, id)
	}
	assert.EqualValues(t, 80, result.Config.Rules["MD013"].Options["max"], "markdownlint's default line length")
	assert.Equal(t, []string{`line-length option "line_length" must be an integer, not +Inf; it is ignored`}, result.Warnings)
}

// TestLoad_MarkdownlintJSONCComesFirst covers a directory that holds both
// .markdownlint.jsonc and .markdownlint.json. markdownlint-cli2 0.23.3 looks
// for its config files in the order .jsonc, .json, .yaml, .yml, .cjs and
// .mjs (configurationFiles in markdownlint-cli2.mjs), and uses the first it
// finds, so it reads the JSONC file and turns line-length (MD013) off.
// gomdlint used to read the JSON file first.
func TestLoad_MarkdownlintJSONCComesFirst(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := writeMarkdownlint(t, dir, ".markdownlint.jsonc", `{"MD013": false}`)
	writeMarkdownlint(t, dir, ".markdownlint.json", `{}`)

	result, err := Load(t.Context(), markdownlintOptions(dir, false, "", nil))
	require.NoError(t, err)

	assert.Equal(t, []string{first}, result.LoadedFrom)
	rule := result.Config.Rules["MD013"]
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	assert.Equal(t, first, FindMarkdownlintConfig(dir), "migrate looks in the same order")
}

// TestLoad_YAMLProjectConfigWithByteOrderMark covers gomdlint's own YAML
// config files, which load with a leading byte order mark, as JSON ones
// do (see TestLoad_JSONProjectConfig).
func TestLoad_YAMLProjectConfigWithByteOrderMark(t *testing.T) {
	t.Parallel()

	result, err := loadProject(t, "\ufeffflavor: commonmark\nrules:\n  MD009: false\n")
	require.NoError(t, err)

	assert.Equal(t, "commonmark", string(result.Config.Flavor))
	rule := result.Config.Rules["MD009"]
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	assert.Empty(t, result.Warnings)
}

// TestLoad_AppliesMarkdownlintConfigWithTrailingCommas covers a project
// whose only configuration is a JSONC markdownlint config with trailing
// commas: it applies to the run.
func TestLoad_AppliesMarkdownlintConfigWithTrailingCommas(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMarkdownlint(t, dir, ".markdownlint.jsonc", "{\n  \"MD013\": false,\n  \"MD009\": {\"br_spaces\": 3,},\n}\n")

	result, err := Load(t.Context(), markdownlintOptions(dir, false, "", nil))
	require.NoError(t, err)

	rule := result.Config.Rules["MD013"]
	require.NotNil(t, rule.Enabled)
	assert.False(t, *rule.Enabled)
	assert.InDelta(t, 3, result.Config.Rules["MD009"].Options["br_spaces"], 0)
}
