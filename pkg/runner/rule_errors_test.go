package runner_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// contentRule runs a function of the file content.
type contentRule struct {
	lint.BaseRule

	apply func(content []byte) ([]lint.Diagnostic, error)
}

func (r *contentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return r.apply(ctx.File.Content)
}

func newContentRule(id string, apply func(content []byte) ([]lint.Diagnostic, error)) *contentRule {
	return &contentRule{BaseRule: lint.NewBaseRule(id, "rule-"+id, "", nil, true), apply: apply}
}

func runFiles(t *testing.T, cfg *config.Config, files map[string]string, rules ...lint.Rule) *runner.Result {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
	}
	lintRunner := runner.New(lint.NewPipeline(lint.NewEngine(&mockParser{}, registry)))

	result, err := lintRunner.Run(context.Background(), runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Jobs:       2,
		Config:     cfg,
	})
	require.NoError(t, err)
	return result
}

func outcomeFor(t *testing.T, result *runner.Result, name string) runner.FileOutcome {
	t.Helper()
	for _, outcome := range result.Files {
		if filepath.Base(outcome.Path) == name {
			return outcome
		}
	}
	t.Fatalf("no outcome for %s", name)
	return runner.FileOutcome{}
}

func TestRunner_Run_ReportsRuleErrorsPerFile(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	failing := newContentRule("TEST001", func(content []byte) ([]lint.Diagnostic, error) {
		if bytes.Contains(content, []byte("boom")) {
			return nil, errBoom
		}
		return nil, nil
	})
	panicking := newContentRule("TEST002", func(content []byte) ([]lint.Diagnostic, error) {
		if bytes.Contains(content, []byte("panic")) {
			panic("rule bug")
		}
		return nil, nil
	})

	result := runFiles(t, config.NewConfig(), map[string]string{
		"a.md": "boom\n",
		"b.md": "fine\n",
		"c.md": "panic\n",
	}, failing, panicking)

	failed := outcomeFor(t, result, "a.md")
	require.NoError(t, failed.Error, "a rule error does not stop the file from being processed")
	require.Len(t, failed.RuleErrors, 1)
	assert.Equal(t, "TEST001", failed.RuleErrors[0].RuleID)
	require.ErrorIs(t, failed.RuleErrors[0], errBoom)

	assert.Empty(t, outcomeFor(t, result, "b.md").RuleErrors)

	panicked := outcomeFor(t, result, "c.md")
	require.Len(t, panicked.RuleErrors, 1)
	require.ErrorIs(t, panicked.RuleErrors[0], lint.ErrRulePanic)

	assert.Equal(t, 2, result.Stats.FilesWithRuleErrors)
	assert.Equal(t, 2, result.Stats.RuleErrors)
	assert.Equal(t, 3, result.Stats.FilesProcessed)
	assert.Zero(t, result.Stats.FilesErrored, "FilesErrored still counts files that could not be processed at all")
	assert.True(t, result.HasRuleErrors())
}

func TestRunner_Run_ReportsSkippedEditsAndOscillation(t *testing.T) {
	t.Parallel()

	// Both rules always propose their fix, and the fixes overlap: the first
	// is applied, then reproposed as a no-op, so the loop stops with the
	// second fix still skipped.
	first := newContentRule("TEST001", func([]byte) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{{RuleID: "TEST001", Message: "one", StartLine: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "HELLO"}}}}, nil
	})
	second := newContentRule("TEST002", func([]byte) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{{RuleID: "TEST002", Message: "two", StartLine: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 3, EndOffset: 8, NewText: "xx"}}}}, nil
	})

	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.NoBackups = true
	result := runFiles(t, cfg, map[string]string{"a.md": "hello world\n"}, first, second)

	outcome := outcomeFor(t, result, "a.md")
	require.NoError(t, outcome.Error)
	assert.True(t, outcome.Oscillated)
	assert.Equal(t, []lint.SkippedEdit{{
		RuleID: "TEST002",
		Edit:   fix.TextEdit{StartOffset: 3, EndOffset: 8, NewText: "xx"},
	}}, outcome.SkippedEdits)
	assert.Equal(t, 1, result.Stats.EditsSkipped)
	assert.False(t, result.HasRuleErrors())
}

func TestResult_HasRuleErrors(t *testing.T) {
	t.Parallel()

	var nilResult *runner.Result
	assert.False(t, nilResult.HasRuleErrors())
	assert.False(t, (&runner.Result{}).HasRuleErrors())
	assert.True(t, (&runner.Result{Stats: runner.Stats{FilesWithRuleErrors: 1}}).HasRuleErrors())
}
