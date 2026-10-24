package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// optionDoc has one line longer than 40 characters and none longer than 80.
const optionDoc = "# Title\n\nThis line is longer than forty characters, by some margin.\n"

// optionMarkdownlintConfig is a markdownlint config with markdownlint's
// spellings of line-length options, a typo of an option on line 4, and an
// option on line 7, under a tag that none of the tag's rules reads.
const optionMarkdownlintConfig = `{
  "MD013": {
    "line_length": 40, "tables": false,
    "stirct": true
  },
  "whitespace": {
    "br_spcaes": 3
  }
}
`

// TestRuleOptionMessagesGolden runs command lines whose configuration sets
// rule options in markdownlint's spellings, with mistakes, and compares the
// transcript of each with testdata/golden/<name>.golden. markdownlint's
// spellings configure the rules, unknown options and values of the wrong
// type get warnings that name the file and line, and two spellings of one
// option are a configuration error.
func TestRuleOptionMessagesGolden(t *testing.T) {
	skipUnlessSlashPaths(t)

	tests := []struct {
		name  string
		files map[string]string
		args  []string
	}{
		{
			name: "lint-config-option-warnings",
			files: map[string]string{".gomdlint.yml": `rules:
  line-length:
    options:
      line_length: 40
      tables: false
      stirct: true
      headings: "no"
  unordered-list-style:
    options:
      style: dashes
`},
			args: []string{"lint", "--color", "never", "--no-context", "doc.md"},
		},
		{
			name:  "lint-config-option-conflict",
			files: map[string]string{".gomdlint.yml": "rules:\n  line-length:\n    options:\n      line_length: 40\n      max: 50\n"},
			args:  []string{"lint", "doc.md"},
		},
		{
			name:  "lint-markdownlint-option-warnings",
			files: map[string]string{".markdownlint.json": optionMarkdownlintConfig},
			args:  []string{"lint", "--color", "never", "--no-context", "doc.md"},
		},
		{
			name:  "migrate-option-warnings",
			files: map[string]string{".markdownlint.json": optionMarkdownlintConfig},
			args:  []string{"migrate", ".markdownlint.json", "--output", ".gomdlint.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"doc.md": optionDoc}
			for name, content := range tt.files {
				files[name] = content
			}
			dir := inProject(t, files)

			got := execute(t, tt.args...)

			assertGolden(t, testdataPath("golden", tt.name+".golden"), transcript(t, got, dir))
		})
	}
}

// TestMigrateWritesCanonicalOptionNames checks the config that migrate
// writes: options under gomdlint's canonical names, an inverted spelling
// with the opposite value, reset_options for each rule that a rule key
// turns on, which then takes markdownlint's default for every option it
// does not set, markdownlint's defaults under rule_defaults for the
// options that the default key writes out, as #42's migration did, and no
// option that no rule reads.
func TestMigrateWritesCanonicalOptionNames(t *testing.T) {
	inProject(t, map[string]string{".markdownlint.json": optionMarkdownlintConfig})

	got := execute(t, "migrate", ".markdownlint.json", "--output", ".gomdlint.yml")
	require.Zero(t, got.code, got.stderr)

	written, err := os.ReadFile(".gomdlint.yml")
	require.NoError(t, err)
	content := string(written)
	assert.Contains(t, content, "max: 40")
	assert.Contains(t, content, "ignore_tables: true")
	assert.Contains(t, content, "reset_options: true")
	rules, ruleDefaults, found := strings.Cut(content, "rule_defaults:")
	require.True(t, found, content)
	assert.NotContains(t, rules, "ignore_urls", "a rule key that resets the options writes none of the defaults")
	assert.Contains(t, ruleDefaults, "ignore_urls: false")
	assert.Contains(t, ruleDefaults, "markdownlint_compatible: true")
	assert.NotContains(t, ruleDefaults, "reset_options", "the default key resets nothing")
	for _, absent := range []string{"line_length", "tables: false", "stirct", "br_spcaes"} {
		assert.NotContains(t, content, absent)
	}
}
