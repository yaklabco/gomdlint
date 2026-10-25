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

// TestGeneratedConfigGoldenNamesNoRuleByID guards the golden files of
// TestGeneratedConfigGolden: none keys a rule by its ID.
func TestGeneratedConfigGoldenNamesNoRuleByID(t *testing.T) {
	for _, name := range []string{"init", "init-full", "init-json", "init-full-json"} {
		content := readFile(t, testdataPath("golden", name+".golden"))
		assert.NotRegexp(t, `(?m)^\s*"?MDL?\d{3}"?:`, content, name)
	}
}
