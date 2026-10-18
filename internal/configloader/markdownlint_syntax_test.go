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
