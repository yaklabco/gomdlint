package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// formerlyOptInDoc breaks the rules that used to be off by default and
// report with their default options: no-inline-html (MD033),
// first-line-heading (MD041), descriptive-link-text (MD059),
// table-column-style (MD060) and link-destination-style (MDL001), which
// reports the file: URL after a relative link.
const formerlyOptInDoc = "Introduction with <b>bold</b> text.\n\n" +
	"# Title\n\n" +
	"Read the guide [here](https://example.com/guide) or [the docs](docs/guide.md).\n\n" +
	"Open [the local copy](file:///tmp/guide.md) offline.\n\n" +
	"| Name | Description |\n|---|---|\n| item | a longer description |\n"

// formerlyOptIn holds the IDs and names of the rules formerlyOptInDoc breaks.
var formerlyOptIn = map[string]string{
	"MD033":  "no-inline-html",
	"MD041":  "first-line-heading",
	"MD059":  "descriptive-link-text",
	"MD060":  "table-column-style",
	"MDL001": "link-destination-style",
}

// TestLint_FormerlyOptInRulesRunByDefault covers a run without a config
// file: the rules that used to be off by default report, as markdownlint's
// do, and a config or --disable turns them off by name or by ID.
func TestLint_FormerlyOptInRulesRunByDefault(t *testing.T) {
	t.Run("without a config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": formerlyOptInDoc})

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		reported := reportedRules(t, got.stdout)
		for id := range formerlyOptIn {
			assert.True(t, reported[id], "%s reports without a config", id)
		}
	})

	t.Run("turned off by name in a config", func(t *testing.T) {
		var config strings.Builder
		config.WriteString("rules:\n")
		for _, name := range formerlyOptIn {
			config.WriteString("  " + name + ": false\n")
		}
		inProject(t, map[string]string{"doc.md": formerlyOptInDoc, ".gomdlint.yml": config.String()})

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		reported := reportedRules(t, got.stdout)
		for id := range formerlyOptIn {
			assert.False(t, reported[id], "%s is off", id)
		}
	})

	t.Run("turned off by ID with --disable", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": formerlyOptInDoc})

		args := []string{"lint", "--format", "json"}
		for id := range formerlyOptIn {
			args = append(args, "--disable", id)
		}
		got := execute(t, append(args, "doc.md")...)

		require.Empty(t, got.stderr)
		reported := reportedRules(t, got.stdout)
		for id := range formerlyOptIn {
			assert.False(t, reported[id], "%s is off", id)
		}
	})
}

// configuredRulesDoc breaks the heading outline and the name that
// configuredRulesOptions gives required-headings (MD043) and proper-names
// (MD044), on lines 5 and 3.
const configuredRulesDoc = "# Title\n\nUse github here.\n\n## Other\n"

// configuredRulesOptions is a gomdlint config that gives MD043 and MD044
// their options, without "enabled: true".
const configuredRulesOptions = "rules:\n" +
	"  required-headings:\n    options:\n      headings: [\"# Title\", \"## Usage\"]\n" +
	"  proper-names:\n    options:\n      names: [GitHub]\n"

// TestLint_ConfiguredRulesRunByDefault covers required-headings (MD043) and
// proper-names (MD044), which check nothing until they are given headings
// or names. Like every other rule they run by default, so they report
// nothing without a config and check the document once a config gives them
// options, without "enabled: true", as in markdownlint.
func TestLint_ConfiguredRulesRunByDefault(t *testing.T) {
	t.Run("without a config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": configuredRulesDoc})

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		reported := reportedRules(t, got.stdout)
		assert.False(t, reported["MD043"])
		assert.False(t, reported["MD044"])
	})

	t.Run("options alone in a config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": configuredRulesDoc, ".gomdlint.yml": configuredRulesOptions})

		got := execute(t, "lint", "--format", "json", "doc.md")

		require.Empty(t, got.stderr)
		reported := reportedRules(t, got.stdout)
		assert.True(t, reported["MD043"])
		assert.True(t, reported["MD044"])
	})
}

// TestLint_ConfiguredRulesFollowMarkdownlintConfigs covers MD043 and MD044 in
// markdownlint settings, applied for the run, migrated, or in a
// markdownlint-configure-file comment. As in markdownlint, a rule given
// options runs and default false turns the others off.
func TestLint_ConfiguredRulesFollowMarkdownlintConfigs(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     map[string]bool
	}{
		{name: "defaults", settings: `{}`, want: map[string]bool{"MD043": false, "MD044": false}},
		{
			name:     "options for both",
			settings: `{"MD044": {"names": ["GitHub"]}, "required-headings": {"headings": ["# Title", "## Usage"]}}`,
			want:     map[string]bool{"MD043": true, "MD044": true},
		},
		{
			name:     "only one with default false",
			settings: `{"default": false, "proper-names": {"names": ["GitHub"]}}`,
			want:     map[string]bool{"MD043": false, "MD044": true},
		},
	}

	reports := func(t *testing.T, stdout string) map[string]bool {
		t.Helper()
		reported := reportedRules(t, stdout)
		return map[string]bool{"MD043": reported["MD043"], "MD044": reported["MD044"]}
	}

	for _, tt := range tests {
		t.Run(tt.name+" applied for the run", func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": configuredRulesDoc, ".markdownlint.json": tt.settings})

			got := execute(t, "lint", "--format", "json", "doc.md")

			assert.Equal(t, tt.want, reports(t, got.stdout))
		})

		t.Run(tt.name+" migrated", func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": configuredRulesDoc, ".markdownlint.json": tt.settings})

			migrated := execute(t, "migrate", ".markdownlint.json", "--output", "migrated.yml")
			require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
			got := execute(t, "lint", "--config", "migrated.yml", "--format", "json", "doc.md")

			assert.Equal(t, tt.want, reports(t, got.stdout))
		})

		t.Run(tt.name+" in markdownlint-configure-file", func(t *testing.T) {
			doc := configuredRulesDoc + "\n<!-- markdownlint-configure-file " + tt.settings + " -->\n"
			inProject(t, map[string]string{"doc.md": doc})

			got := execute(t, "lint", "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, tt.want, reports(t, got.stdout))
		})
	}
}

// TestLint_MarkdownlintConfigLeavesGomdlintRulesOff covers markdownlint
// configs, which know nothing of gomdlint's own rules. Applied for the run
// or migrated, a config that keeps markdownlint's defaults runs the
// markdownlint rules and leaves link-destination-style (MDL001) off, while a
// run without any config runs it.
func TestLint_MarkdownlintConfigLeavesGomdlintRulesOff(t *testing.T) {
	files := map[string]string{"doc.md": formerlyOptInDoc, ".markdownlint.json": "{}\n"}
	markdownlintRules := []string{"MD033", "MD041", "MD059", "MD060"}

	t.Run("without a config", func(t *testing.T) {
		inProject(t, map[string]string{"doc.md": formerlyOptInDoc})

		got := execute(t, "lint", "--format", "json", "doc.md")

		assert.True(t, reportedRules(t, got.stdout)["MDL001"])
	})

	t.Run("markdownlint config applied for the run", func(t *testing.T) {
		inProject(t, files)

		got := execute(t, "lint", "--format", "json", "doc.md")

		reported := reportedRules(t, got.stdout)
		assert.False(t, reported["MDL001"])
		for _, id := range markdownlintRules {
			assert.True(t, reported[id], id)
		}
	})

	t.Run("migrated config", func(t *testing.T) {
		inProject(t, files)

		migrated := execute(t, "migrate", ".markdownlint.json", "--output", "migrated.yml")
		require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
		got := execute(t, "lint", "--config", "migrated.yml", "--format", "json", "doc.md")

		reported := reportedRules(t, got.stdout)
		assert.False(t, reported["MDL001"])
		for _, id := range markdownlintRules {
			assert.True(t, reported[id], id)
		}
	})
}
