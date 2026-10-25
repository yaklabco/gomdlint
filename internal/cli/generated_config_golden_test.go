package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// generatedConfigCase is a command line that writes a configuration file,
// in a project that holds files.
type generatedConfigCase struct {
	name  string
	files map[string]string
	args  []string

	// output is the file that the command writes.
	output string
}

// namedMarkdownlintConfig is a markdownlint config that names rules by ID,
// by markdownlint's names and by tags, one of which covers a single rule,
// with a later key that replaces what a tag set for one rule, and a typo
// of a rule name.
const namedMarkdownlintConfig = `{
  // Every rule runs, as markdownlint runs them without a default key.
  "default": true,
  "MD013": {"line_length": 120, "tables": false},
  "no-hard-tabs": false,
  "whitespace": false,
  "no-trailing-spaces": {"br_spaces": 3},
  "single-title": {"level": 1},
  "ul-style": "warning",
  "hr": {"style": "---"},
  "no-trailling-spaces": true,
}
`

// cli2MarkdownlintConfig is a markdownlint-cli2 options file whose config
// names rules by markdownlint's other names and turns a tag off.
const cli2MarkdownlintConfig = `globs:
  - "**/*.md"
config:
  blanks-around-headings:
    lines_above: 2
  first-line-h1: false
  emphasis: false
  MD033:
    allowed_elements: [br, details]
`

// TestGeneratedConfigGolden runs init and migrate and compares the
// transcript of each, followed by the file it writes, with
// testdata/golden/<name>.golden. Generated files key rules by name, with
// the ID in a comment where the format has comments, and write options
// under options. Each written file loads without a warning.
func TestGeneratedConfigGolden(t *testing.T) {
	skipUnlessSlashPaths(t)

	tests := []generatedConfigCase{
		{name: "init", args: []string{"init"}, output: ".gomdlint.yml"},
		{name: "init-full", args: []string{"init", "--full"}, output: ".gomdlint.yml"},
		{name: "init-json", args: []string{"init", "--format", "json"}, output: ".gomdlint.json"},
		{name: "init-full-json", args: []string{"init", "--full", "--format", "json"}, output: ".gomdlint.json"},
		{
			name:   "migrate-named",
			files:  map[string]string{".markdownlint.jsonc": namedMarkdownlintConfig},
			args:   []string{"migrate", ".markdownlint.jsonc"},
			output: ".gomdlint.yml",
		},
		{
			name:   "migrate-named-json",
			files:  map[string]string{".markdownlint.jsonc": namedMarkdownlintConfig},
			args:   []string{"migrate", ".markdownlint.jsonc", "--output", ".gomdlint.json"},
			output: ".gomdlint.json",
		},
		{
			// A markdownlint-cli2 options file in YAML, without a default
			// key, whose config names rules by markdownlint's other names.
			name:   "migrate-cli2-yaml",
			files:  map[string]string{".markdownlint-cli2.yaml": cli2MarkdownlintConfig},
			args:   []string{"migrate", ".markdownlint-cli2.yaml"},
			output: ".gomdlint.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := inProject(t, tt.files)

			got := execute(t, tt.args...)

			require.Equal(t, cli.ExitSuccess, got.code, got.stderr)
			path := filepath.Join(dir, tt.output)
			content := readFile(t, path)
			assertGolden(t, testdataPath("golden", tt.name+".golden"),
				transcript(t, got, dir)+"-- "+tt.output+" --\n"+content)
			loadTemplate(t, path)
		})
	}
}

// migrationDoc breaks many rules, so that linting it shows the rules that a
// configuration turns on and how their options set them.
const migrationDoc = "Preamble before the heading.\n\n# Title\n\n### Skipped level #\n\n" +
	"Trailing spaces   \nand a hard\ttab, on a line that is longer than eighty characters, by quite a long way.\n\n\n" +
	"* one\n+ two\n- three\n\n___\n\n" +
	"<br>Some <details>HTML</details>.\n\n_emphasis_ and *emphasis*, __strong__ and **strong**.\n\n" +
	"Visit https://example.com or [here](https://example.com).\n\n" +
	"| a | b |\n|-|-|\n| 1 | 2 |\n"

// TestMigratedConfigLintsAsItsSource covers the configs of the migrate
// cases of TestGeneratedConfigGolden: a document lints the same under the
// file that migrate writes, in YAML and in JSON, as under the markdownlint
// config that it came from, which lint translates for the run.
func TestMigratedConfigLintsAsItsSource(t *testing.T) {
	for _, source := range []struct{ name, content string }{
		{name: ".markdownlint.jsonc", content: namedMarkdownlintConfig},
		{name: ".markdownlint-cli2.yaml", content: cli2MarkdownlintConfig},
	} {
		t.Run(source.name, func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": migrationDoc, source.name: source.content})
			want := execute(t, "lint", "--format", "json", "doc.md")
			require.Equal(t, cli.ExitLintErrors, want.code, want.stderr)

			for _, output := range []string{"migrated.yml", "migrated.json"} {
				migrated := execute(t, "migrate", source.name, "--output", output)
				require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)

				got := execute(t, "lint", "--config", output, "--format", "json", "doc.md")

				assert.Equal(t, want.code, got.code, got.stderr)
				assert.JSONEq(t, want.stdout, got.stdout, output)
			}
		})
	}
}

// TestGeneratedConfigGoldenNamesNoRuleByID guards the golden files of
// TestGeneratedConfigGolden: none keys a rule by its ID.
func TestGeneratedConfigGoldenNamesNoRuleByID(t *testing.T) {
	for _, name := range []string{
		"init", "init-full", "init-json", "init-full-json", "migrate-named", "migrate-named-json", "migrate-cli2-yaml",
	} {
		content := readFile(t, testdataPath("golden", name+".golden"))
		assert.NotRegexp(t, `(?m)^\s*"?MDL?\d{3}"?:`, content, name)
	}
}
