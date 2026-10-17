package lint_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// markerPair returns rule TEST001, whose one fix rewrites the "*" pair
// around a word, two edits for one finding, while the content still has
// a "*".
func markerPair() *funcRule {
	const id = "TEST001"
	return newFuncRule(id, func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		content := string(ctx.File.Content)
		open := strings.IndexByte(content, '*')
		if open < 0 {
			return nil, nil
		}
		closing := open + 1 + strings.IndexByte(content[open+1:], '*')
		return []lint.Diagnostic{fixDiag(id,
			fix.TextEdit{StartOffset: open, EndOffset: open + 1, NewText: "_"},
			fix.TextEdit{StartOffset: closing, EndOffset: closing + 1, NewText: "_"},
		)}, nil
	})
}

// TestEngine_AcceptedFixesCountsFindings covers FileResult.AcceptedFixes,
// the findings whose fixes Edits holds, which differs from the number of
// edits: a fix may take several edits, and blank-line fixes at one
// boundary share one.
func TestEngine_AcceptedFixesCountsFindings(t *testing.T) {
	t.Parallel()

	t.Run("a fix with two edits is one fix", func(t *testing.T) {
		t.Parallel()

		result := lintWithRules(t, "a *bc* d", true, markerPair())

		assert.Len(t, result.Edits, 2)
		assert.Equal(t, 1, result.AcceptedFixes)
	})

	t.Run("blank-line fixes sharing an edit are two fixes", func(t *testing.T) {
		t.Parallel()

		result := lintWithRules(t, "# Title\n- item\n", true, insertRule("TEST001", 8, "\n"), insertRule("TEST002", 8, "\n"))

		assert.Len(t, result.Edits, 1)
		assert.Equal(t, 2, result.AcceptedFixes)
	})

	t.Run("a skipped fix is not accepted", func(t *testing.T) {
		t.Parallel()

		overlap := newFuncRule("TEST002", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
			return []lint.Diagnostic{fixDiag("TEST002", fix.TextEdit{StartOffset: 1, EndOffset: 4, NewText: "xyz"})}, nil
		})

		result := lintWithRules(t, "a *bc* d", true, markerPair(), overlap)

		assert.NotEmpty(t, result.SkippedEdits, "the fixes conflict, so one of them is skipped")
		assert.Equal(t, 1, result.AcceptedFixes)
	})

	t.Run("a conflicting blank-line fix is not accepted", func(t *testing.T) {
		t.Parallel()

		result := lintWithRules(t, "> # A\n> ## B\n", true, insertRule("TEST001", 6, ">\n"), insertRule("TEST002", 6, "> >\n"))

		assert.Len(t, result.Edits, 1)
		assert.Equal(t, 1, result.AcceptedFixes)
	})

	t.Run("nothing is accepted without fix mode", func(t *testing.T) {
		t.Parallel()

		result := lintWithRules(t, "a *bc* d", false, markerPair())

		assert.Zero(t, result.AcceptedFixes)
	})
}

// TestPipeline_TotalFixesAppliedCountsFindings covers the fixes of a
// multi-pass run: TEST001 rewrites one marker pair per pass, two edits for
// each of its two findings, so four edits fix two findings.
func TestPipeline_TotalFixesAppliedCountsFindings(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(markerPair())
	pipeline := lint.NewPipeline(lint.NewEngine(&mockParser{}, registry))
	cfg := config.NewConfig()
	cfg.Fix = true

	result, err := pipeline.ProcessContent(context.Background(), "test.md", []byte("*a* and *b*"), cfg,
		lint.PipelineOptions{Fix: true, DryRun: true})

	require.NoError(t, err)
	assert.Equal(t, "_a_ and _b_", string(result.ModifiedContent))
	assert.Equal(t, 2, result.FixPasses)
	assert.Equal(t, 4, result.TotalEditsApplied)
	assert.Equal(t, 2, result.TotalFixesApplied)
}
