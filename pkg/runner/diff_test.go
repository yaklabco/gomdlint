package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TestRunner_Run_PreviewFixesWithFixRecordsTheWrittenDiff covers
// PreviewFixes in a run with Fix: the fixes are written, and each fixed
// file carries the diff of what was written.
func TestRunner_Run_PreviewFixesWithFixRecordsTheWrittenDiff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	registry := lint.NewRegistry()
	registry.Register(newContentRule("TEST001", func(content []byte) ([]lint.Diagnostic, error) {
		if !strings.HasPrefix(string(content), "hello") {
			return nil, nil
		}
		return []lint.Diagnostic{{
			RuleID: "TEST001", Message: "fix needed", StartLine: 1, StartColumn: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}},
		}}, nil
	}))
	lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(&mockParser{}, registry)))

	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.NoBackups = true
	result, err := lintRunner.Run(context.Background(), runner.Options{
		Paths:        []string{"."},
		WorkingDir:   dir,
		Config:       cfg,
		PreviewFixes: true,
	})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "world", string(content), "the fix is written")

	require.Len(t, result.Files, 1)
	outcome := result.Files[0]
	require.NotNil(t, outcome.Result)
	assert.True(t, outcome.Result.Written)
	require.NotNil(t, outcome.Result.Diff, "a written file carries the diff of what was written")
	assert.Equal(t, fix.GenerateDiff(path, []byte("hello"), []byte("world")).String(), outcome.Result.Diff.String())
	assert.Zero(t, result.Stats.DiagnosticsTotal, "diagnostics describe the fixed file")
}

// replaceAllRule replaces every occurrence of from with to, one fix per
// occurrence.
func replaceAllRule(id, from, to string) *contentRule {
	return newContentRule(id, func(content []byte) ([]lint.Diagnostic, error) {
		var diags []lint.Diagnostic
		for offset := 0; ; {
			idx := strings.Index(string(content[offset:]), from)
			if idx < 0 {
				return diags, nil
			}
			start := offset + idx
			diags = append(diags, lint.Diagnostic{
				RuleID: id, Message: "fix needed", StartLine: 1, StartColumn: start + 1,
				FixEdits: []fix.TextEdit{{StartOffset: start, EndOffset: start + len(from), NewText: to}},
			})
			offset = start + len(from)
		}
	})
}

// TestRunner_Run_CountsAndDiffsFixesDeferredToALaterPass covers fixes that
// the engine defers because another rule fixes the same block in that
// pass: they count once a later pass applies them, and the diff covers
// every pass.
func TestRunner_Run_CountsAndDiffsFixesDeferredToALaterPass(t *testing.T) {
	t.Parallel()

	const original, fixed = "x y x\n", "X Y X\n"
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "written", true: "dry run"}[dryRun], func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "test.md")
			require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

			registry := lint.NewRegistry()
			registry.Register(replaceAllRule("TEST001", "x", "X"))
			registry.Register(replaceAllRule("TEST002", "y", "Y"))
			lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(&mockParser{}, registry)))

			cfg := config.NewConfig()
			cfg.Fix = true
			cfg.DryRun = dryRun
			cfg.NoBackups = true
			result, err := lintRunner.Run(context.Background(), runner.Options{
				Paths:        []string{"."},
				WorkingDir:   dir,
				Config:       cfg,
				PreviewFixes: true,
			})
			require.NoError(t, err)

			require.Len(t, result.Files, 1)
			outcome := result.Files[0]
			require.NotNil(t, outcome.Result)
			assert.Equal(t, 2, outcome.FixPasses, "TEST002's fix waits for the second pass")
			assert.Equal(t, 3, result.Stats.DiagnosticsFixed, "every fix applied in any pass counts once")
			require.NotNil(t, outcome.Result.Diff)
			assert.Equal(t, fix.GenerateDiff(path, []byte(original), []byte(fixed)).String(),
				outcome.Result.Diff.String(), "the diff covers the fixes of both passes")

			content, err := os.ReadFile(path)
			require.NoError(t, err)
			if dryRun {
				assert.Equal(t, original, string(content))
				assert.Equal(t, 1, result.Stats.FilesWithPendingFixes)
			} else {
				assert.Equal(t, fixed, string(content))
				assert.Equal(t, 1, result.Stats.FilesModified)
			}
		})
	}
}
