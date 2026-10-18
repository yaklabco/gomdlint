package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	_ "github.com/yaklabco/gomdlint/pkg/lint/rules" // Register the built-in rules.
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

func TestRunner_Run_ReportsNoticesPerFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		"noticed.md": "# Noticed\n\n<!-- markdownlint-disable frobnicate -->\n",
		"quiet.md":   "# Quiet\n\n<!-- markdownlint-disable no-trailing-spaces -->\n",
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry)))

	result, err := lintRunner.Run(context.Background(), runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Jobs:       2,
		Config:     config.NewConfig(),
	})
	require.NoError(t, err)

	noticed := outcomeFor(t, result, "noticed.md")
	assert.Equal(t, []lint.Notice{{
		Kind: lint.NoticeUnknownRule, Line: 3, Directive: "markdownlint-disable", Name: "frobnicate",
		Message: `markdownlint-disable: unknown rule "frobnicate"; it is ignored`,
	}}, noticed.Notices)
	assert.False(t, noticed.Failed(), "a notice does not fail the file")

	assert.Empty(t, outcomeFor(t, result, "quiet.md").Notices)

	assert.Zero(t, result.Stats.DiagnosticsTotal, "notices are not findings")
	assert.Zero(t, result.Stats.FilesWithIssues)
	assert.False(t, result.HasIssues())
}
