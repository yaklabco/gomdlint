package cli_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// transcript records the outcome of a command line for a golden file: the
// exit code, stdout and stderr. The project directory dir, which differs
// between runs, appears as $PROJECT.
func transcript(t *testing.T, got execution, dir string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	anonymize := strings.NewReplacer(resolved, "$PROJECT", dir, "$PROJECT").Replace

	return fmt.Sprintf("exit code %d\n-- stdout --\n%s-- stderr --\n%s",
		got.code, anonymize(got.stdout), anonymize(got.stderr))
}

// ruleNameDoc reports trailing punctuation in its heading (a headings
// rule), a hard tab and a trailing space (whitespace rules).
const ruleNameDoc = "# Title.\n\nText\twith a tab and a trailing space \n"

// unknownMarkdownlintKeys is a markdownlint config with a typo of a rule
// name, a typo of a tag and a key of two rule IDs.
const unknownMarkdownlintKeys = `{"no-trailling-spaces": false, "tables": false, "MD009 MD010": false}` + "\n"

// TestRuleNameMessagesGolden runs command lines that name rules that do
// not exist, or one rule twice, and compares the transcript of each with
// testdata/golden/<name>.golden. Messages name rules by name and offer the
// rules and tags closest to a typo.
func TestRuleNameMessagesGolden(t *testing.T) {
	skipUnlessSlashPaths(t)

	tests := []struct {
		name  string
		files map[string]string
		args  []string
	}{
		{
			name: "lint-disable-typo",
			args: []string{"lint", "--disable", "no-trailling-spaces", "doc.md"},
		},
		{
			// A typo of a tag, an ID as close to many rules as to any, and
			// a name like no rule, in all three rule flags.
			name: "lint-rule-flags-unknown",
			args: []string{
				"lint", "--enable", "MD002", "--disable", "MD009,tables",
				"--fix-rules", "frobnicate", "doc.md",
			},
		},
		{
			// A markdownlint tag stands for every rule it covers.
			name: "lint-disable-tag",
			args: []string{"lint", "--color", "never", "--no-context", "--disable", "whitespace", "doc.md"},
		},
		{
			name:  "lint-config-unknown-rule",
			files: map[string]string{".gomdlint.yml": "rules:\n  no-trailling-spaces: false\n  hard_tab: false\n"},
			args:  []string{"lint", "--color", "never", "--no-context", "doc.md"},
		},
		{
			name:  "lint-config-duplicate-rule",
			files: map[string]string{".gomdlint.yml": "rules:\n  MD013:\n    enabled: false\n  line-length:\n    severity: error\n"},
			args:  []string{"lint", "doc.md"},
		},
		{
			name:  "lint-markdownlint-unknown-rule",
			files: map[string]string{".markdownlint.json": unknownMarkdownlintKeys},
			args:  []string{"lint", "--color", "never", "--no-context", "doc.md"},
		},
		{
			name:  "migrate-unknown-rule",
			files: map[string]string{".markdownlint.json": unknownMarkdownlintKeys},
			args:  []string{"migrate", ".markdownlint.json", "--output", ".gomdlint.yml"},
		},
		{
			// markdownlint reads a key of two rule IDs as one name, which
			// names nothing, so it configures neither rule.
			name: "lint-configure-file-two-rules",
			files: map[string]string{
				"configured.md": "<!-- markdownlint-configure-file {\"MD009 MD010\": false} -->\n\n" + ruleNameDoc,
			},
			args: []string{"lint", "--color", "never", "--no-context", "configured.md"},
		},
		{
			// A typo in a markdownlint comment silences nothing, and a
			// former gomdlint tag still works. Both get a warning.
			name:  "lint-directive-notices",
			files: map[string]string{"notices.md": noticeDoc},
			args:  []string{"lint", "--color", "never", "--no-context", "notices.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"doc.md": ruleNameDoc}
			for name, content := range tt.files {
				files[name] = content
			}
			dir := inProject(t, files)

			got := execute(t, tt.args...)

			assertGolden(t, testdataPath("golden", tt.name+".golden"), transcript(t, got, dir))
		})
	}
}

// TestRuleCompletionGolden covers the shell completion of the rule flags,
// which cobra's completion scripts request with the hidden __complete
// command, and compares its output with testdata/golden/<name>.golden.
// Each line is a value and its description, separated by a tab, and the
// last line is cobra's completion directive.
func TestRuleCompletionGolden(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		// Each rule by name, and the tags.
		{name: "complete-disable", args: []string{"__complete", "lint", "--disable", ""}},
		// IDs, once the word starts one.
		{name: "complete-enable-id", args: []string{"__complete", "lint", "--enable", "MD00"}},
		// A tag, a name and an alias that start alike.
		{name: "complete-fix-rules-ul", args: []string{"__complete", "lint", "--fix-rules", "ul"}},
		// The last item of a list.
		{name: "complete-disable-list", args: []string{"__complete", "lint", "--disable", "MD009,no-t"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inProject(t, nil)

			got := execute(t, tt.args...)

			assert.Equal(t, cli.ExitSuccess, got.code, got.stderr)
			assert.Contains(t, got.stderr, "ShellCompDirectiveNoFileComp", "rule flags never complete file names")
			assertGolden(t, testdataPath("golden", tt.name+".golden"), got.stdout)
		})
	}
}
