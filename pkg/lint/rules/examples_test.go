package rules

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// examplesPending lists the categories whose rules do not all have examples
// yet. The rule reference gains examples a few categories at a time, so
// that each set gets a real review. Remove a category once every rule in
// it has its examples: TestEveryRuleHasExamples fails while a category
// here has them all.
var examplesPending = []lint.Category{
	lint.CategoryLists,
	lint.CategoryLayout,
	lint.CategoryCode,
	lint.CategoryLinks,
	lint.CategoryTables,
	lint.CategoryEmphasis,
}

// TestEveryRuleHasExamples checks that every rule outside the pending
// categories has an example document that breaks it, with the findings
// that the rule reports there. A rule that fixes what it finds shows the
// fixed document. A rule that cannot shows a document that follows it
// instead.
func TestEveryRuleHasExamples(t *testing.T) {
	missing := make(map[lint.Category]int)
	for _, meta := range Catalog() {
		example, found, err := RuleExample(meta.ID)
		require.NoError(t, err, meta.ID)
		if !found {
			missing[meta.Category]++
			assert.Contains(t, examplesPending, meta.Category,
				"%s (%s) has no example: write testdata/%s/example.input.md", meta.ID, meta.Name, meta.ID)
			continue
		}

		assert.NotEmpty(t, example.Input, "%s: the example breaks the rule", meta.ID)
		assert.NotEmpty(t, example.Findings, "%s: the rule reports the example", meta.ID)
		if meta.Fixable {
			assert.NotEqual(t, example.Input, example.Fixed, "%s: the example shows the fix", meta.ID)
		} else {
			assert.Equal(t, example.Input, example.Fixed, "%s: the rule fixes nothing", meta.ID)
			assert.NotEmpty(t, example.Good,
				"%s cannot fix what it finds, so it needs testdata/%s/example_good.input.md", meta.ID, meta.ID)
		}
	}

	for _, category := range examplesPending {
		assert.Positive(t, missing[category],
			"every %s rule has examples now: remove the category from examplesPending", category)
	}
}

// TestRuleExampleOfAnUnknownRule covers the IDs that have no example: an
// unknown one, and words that are not an ID.
func TestRuleExampleOfAnUnknownRule(t *testing.T) {
	for _, id := range []string{"MD999", "", "..", "MD001/../MD003", "testdata"} {
		_, found, err := RuleExample(id)
		require.NoError(t, err, "%q", id)
		assert.False(t, found, "%q", id)
	}
}

// TestExamplesShowOnlyTheirRule lints each example with every rule on, as
// a user of gomdlint would, under the configuration that the example's
// sidecar sets: the document that breaks the rule breaks only that rule,
// with the findings that the example lists, and the fixed and good
// documents break none.
func TestExamplesShowOnlyTheirRule(t *testing.T) {
	baseDir := testdataDir(t)
	for _, meta := range Catalog() {
		example, found, err := RuleExample(meta.ID)
		require.NoError(t, err, meta.ID)
		if !found {
			continue
		}
		t.Run(meta.ID, func(t *testing.T) {
			dir := filepath.Join(baseDir, meta.ID)
			cfg, err := loadGoldenConfig(filepath.Join(dir, "example"+goldenConfigSuffix))
			require.NoError(t, err)

			got := lintWithEveryRule(t, cfg, example.Input)
			want := make([]ExampleFinding, 0, len(got))
			for _, diag := range got {
				assert.Equal(t, meta.ID, diag.RuleID, "only %s reports the example: %s", meta.ID, diag.Message)
				want = append(want, ExampleFinding{
					Line: diag.StartLine, Column: diag.StartColumn, Message: diag.Message, Fixable: diag.HasFix(),
				})
			}
			assert.Equal(t, want, example.Findings, "the example lists what the rule reports")

			if meta.Fixable {
				assert.Empty(t, lintWithEveryRule(t, cfg, example.Fixed), "the fixed example is clean")
			}

			if example.Good == "" {
				return
			}
			goodConfig := filepath.Join(dir, "example_good"+goldenConfigSuffix)
			goodCfg, err := loadGoldenConfig(goodConfig)
			require.NoError(t, err)
			assert.Equal(t, readOptional(t, filepath.Join(dir, "example"+goldenConfigSuffix)), readOptional(t, goodConfig),
				"the good example runs under the example's configuration")
			assert.Empty(t, lintWithEveryRule(t, goodCfg, example.Good), "the good example is clean")
		})
	}
}

// lintWithEveryRule lints content with every built-in rule that runs by
// default, which is every rule, under cfg, and returns its findings in
// order of position.
func lintWithEveryRule(t *testing.T, cfg *config.Config, content string) []lint.Diagnostic {
	t.Helper()

	registry := lint.NewRegistry()
	RegisterAll(registry)
	engine := lint.NewEngine(goldmark.New(string(cfg.Flavor)), registry)
	result, err := engine.LintFile(context.Background(), "example.md", []byte(content), cfg)
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)

	diags := slices.Clone(result.Diagnostics)
	sortDiagnostics(diags)
	return diags
}

// readOptional returns the content of the file at path, or "" when there
// is none.
func readOptional(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}
