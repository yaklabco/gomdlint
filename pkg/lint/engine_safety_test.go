package lint_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// funcRule is a test rule whose behaviour is a function of the rule context.
type funcRule struct {
	lint.BaseRule

	apply func(ctx *lint.RuleContext) ([]lint.Diagnostic, error)
}

func (r *funcRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return r.apply(ctx)
}

func newFuncRule(id string, apply func(ctx *lint.RuleContext) ([]lint.Diagnostic, error)) *funcRule {
	return &funcRule{
		BaseRule: lint.NewBaseRule(id, "rule-"+id, "", nil, true),
		apply:    apply,
	}
}

// fixDiag returns a diagnostic on line 1 with the given fix edits.
func fixDiag(id string, edits ...fix.TextEdit) lint.Diagnostic {
	return lint.Diagnostic{RuleID: id, Message: id + " issue", StartLine: 1, StartColumn: 1, FixEdits: edits}
}

func lintWithRules(t *testing.T, content string, fixMode bool, rules ...lint.Rule) *lint.FileResult {
	t.Helper()

	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
	}
	engine := lint.NewEngine(&mockParser{}, registry)
	cfg := config.NewConfig()
	cfg.Fix = fixMode

	result, err := engine.LintFile(context.Background(), "test.md", []byte(content), cfg)
	require.NoError(t, err)
	return result
}

// requireRuleError checks that ruleID failed with an error matching target.
func requireRuleError(t *testing.T, result *lint.FileResult, ruleID string, target error) {
	t.Helper()

	err, ok := result.RuleErrors[ruleID]
	require.True(t, ok, "expected a rule error for %s, got %v", ruleID, result.RuleErrors)
	require.ErrorIs(t, err, target)
	ruleErr := ruleErrorOf(t, result, ruleID)
	assert.Equal(t, ruleID, ruleErr.RuleID)
	assert.Equal(t, "rule-"+ruleID, ruleErr.RuleName)
}

// ruleErrorOf returns the *lint.RuleError recorded for ruleID.
func ruleErrorOf(t *testing.T, result *lint.FileResult, ruleID string) *lint.RuleError {
	t.Helper()

	var ruleErr *lint.RuleError
	require.ErrorAs(t, result.RuleErrors[ruleID], &ruleErr)
	return ruleErr
}

func TestEngine_InvalidFixDropsOnlyThatFix(t *testing.T) {
	t.Parallel()

	good := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 0, EndOffset: 5, NewText: "world"})}, nil
	})
	bad := newFuncRule("TEST002", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST002", fix.TextEdit{StartOffset: 6, EndOffset: 100, NewText: ""})}, nil
	})

	result := lintWithRules(t, "hello there", true, good, bad)

	assert.Equal(t, []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}}, result.Edits,
		"the valid fix must still be applied")
	requireRuleError(t, result, "TEST002", lint.ErrInvalidFix)
	assert.NotContains(t, result.RuleErrors, "TEST001")

	require.Len(t, result.Diagnostics, 2, "the finding of the rule with the bad fix is still reported")
	assert.Equal(t, 1, result.FixableCount(), "a diagnostic whose fix was dropped is not fixable")
}

func TestEngine_InvalidFixIsReportedWithoutFixMode(t *testing.T) {
	t.Parallel()

	bad := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 3, EndOffset: 2})}, nil
	})

	result := lintWithRules(t, "hello", false, bad)

	requireRuleError(t, result, "TEST001", lint.ErrInvalidFix)
	require.Len(t, result.Diagnostics, 1)
	assert.Empty(t, result.Diagnostics[0].FixEdits)
	assert.Empty(t, result.Edits)
}

func TestEngine_SkippedEditsNameTheirRule(t *testing.T) {
	t.Parallel()

	first := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 0, EndOffset: 10, NewText: "aaa"})}, nil
	})
	second := newFuncRule("TEST002", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST002", fix.TextEdit{StartOffset: 5, EndOffset: 15, NewText: "bbb"})}, nil
	})

	result := lintWithRules(t, "hello world again", true, first, second)

	assert.True(t, result.EditConflicts)
	assert.Equal(t, []lint.SkippedEdit{{
		RuleID: "TEST002",
		Edit:   fix.TextEdit{StartOffset: 5, EndOffset: 15, NewText: "bbb"},
	}}, result.SkippedEdits)
	assert.Empty(t, result.RuleErrors, "a conflict is not a rule error")
}

func TestEngine_MultiEditFixIsAppliedWholeOrNotAtAll(t *testing.T) {
	t.Parallel()

	// TEST001 rewrites a pair of markers; TEST002 overlaps the first one.
	pair := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001",
			fix.TextEdit{StartOffset: 2, EndOffset: 3, NewText: "_"},
			fix.TextEdit{StartOffset: 8, EndOffset: 9, NewText: "_"},
		)}, nil
	})
	overlap := newFuncRule("TEST002", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST002", fix.TextEdit{StartOffset: 1, EndOffset: 4, NewText: "xyz"})}, nil
	})

	result := lintWithRules(t, "a *bc de* f", true, pair, overlap)

	assert.Equal(t, []fix.TextEdit{{StartOffset: 1, EndOffset: 4, NewText: "xyz"}}, result.Edits,
		"the closing marker must not be rewritten without the opening one")
	assert.Equal(t, []lint.SkippedEdit{
		{RuleID: "TEST001", Edit: fix.TextEdit{StartOffset: 2, EndOffset: 3, NewText: "_"}},
		{RuleID: "TEST001", Edit: fix.TextEdit{StartOffset: 8, EndOffset: 9, NewText: "_"}},
	}, result.SkippedEdits)
}

func TestEngine_RecoversFromRulePanic(t *testing.T) {
	t.Parallel()

	panicking := newFuncRule("TEST001", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		nodes := make([]int, 0, 1)
		_ = nodes[len(ctx.File.Content)] // index out of range
		return nil, nil
	})
	healthy := newFuncRule("TEST002", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST002")}, nil
	})

	result := lintWithRules(t, "hello", false, panicking, healthy)

	requireRuleError(t, result, "TEST001", lint.ErrRulePanic)
	ruleErr := ruleErrorOf(t, result, "TEST001")
	assert.Contains(t, ruleErr.Error(), "index out of range")
	assert.NotEmpty(t, ruleErr.Stack, "the stack is kept for debugging")
	require.Len(t, result.Diagnostics, 1, "other rules still run")
	assert.Equal(t, "TEST002", result.Diagnostics[0].RuleID)
}

func TestEngine_LogsRulePanicStackAtDebugLevel(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := log.New(&logs)
	logger.SetLevel(log.DebugLevel)
	ctx := logging.WithLogger(context.Background(), logger)

	registry := lint.NewRegistry()
	registry.Register(newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		panic("rule bug")
	}))
	engine := lint.NewEngine(&mockParser{}, registry)

	_, err := engine.LintFile(ctx, "doc.md", []byte("hello"), config.NewConfig())
	require.NoError(t, err)

	out := logs.String()
	assert.Contains(t, out, "rule panicked")
	assert.Contains(t, out, "TEST001")
	assert.Contains(t, out, "doc.md")
	assert.Contains(t, out, "goroutine", "the debug log must carry the stack trace")
}

func TestEngine_RulePanicWithErrorValueIsUnwrappable(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	panicking := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		panic(sentinel)
	})

	result := lintWithRules(t, "hello", false, panicking)

	requireRuleError(t, result, "TEST001", lint.ErrRulePanic)
	require.ErrorIs(t, result.RuleErrors["TEST001"], sentinel)
}

func TestEngine_ReturnedRuleErrorIsWrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("rule failed")
	failing := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return nil, sentinel
	})

	result := lintWithRules(t, "hello", false, failing)

	requireRuleError(t, result, "TEST001", sentinel)
	assert.Equal(t, []*lint.RuleError{ruleErrorOf(t, result, "TEST001")}, result.RuleErrorList())
	assert.True(t, result.HasRuleErrors())
}

func TestEngine_ConvertsFixNewlinesToTheFileLineEnding(t *testing.T) {
	t.Parallel()

	// The rule inserts a blank line after the heading using "\n", as rules do.
	insertBlank := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 9, EndOffset: 9, NewText: "\n"})}, nil
	})

	crlf := lintWithRules(t, "# Title\r\nPara\r\n", true, insertBlank)
	require.Len(t, crlf.Edits, 1)
	assert.Equal(t, "\r\n", crlf.Edits[0].NewText)
	assert.Equal(t, "\r\n", crlf.Diagnostics[0].FixEdits[0].NewText, "reported fixes use the file's line ending too")

	lf := lintWithRules(t, "# Title\nPara\n", true, newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 8, EndOffset: 8, NewText: "\n"})}, nil
	}))
	require.Len(t, lf.Edits, 1)
	assert.Equal(t, "\n", lf.Edits[0].NewText)
}

func TestEngine_MergesBlankLineInsertionsInCRLFFiles(t *testing.T) {
	t.Parallel()

	insert := func(id string) *funcRule {
		return newFuncRule(id, func(*lint.RuleContext) ([]lint.Diagnostic, error) {
			return []lint.Diagnostic{fixDiag(id, fix.TextEdit{StartOffset: 9, EndOffset: 9, NewText: "\n"})}, nil
		})
	}

	result := lintWithRules(t, "# Title\r\n- item\r\n", true, insert("TEST001"), insert("TEST002"))

	assert.Equal(t, []fix.TextEdit{{StartOffset: 9, EndOffset: 9, NewText: "\r\n"}}, result.Edits,
		"two blank-line requirements at one boundary insert one blank line")
}

// insertRule returns a rule that inserts text at offset, the way a
// blank-line rule fixes a missing blank line.
func insertRule(id string, offset int, text string) *funcRule {
	return newFuncRule(id, func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{fixDiag(id, fix.TextEdit{StartOffset: offset, EndOffset: offset, NewText: text})}, nil
	})
}

func TestEngine_MergesContainerBlankLineInsertions(t *testing.T) {
	t.Parallel()

	// Offset 6 is the boundary between the two quoted headings.
	const quoted = "> # A\n> ## B\n"

	tests := []struct {
		name    string
		content string
		offset  int
		texts   []string
		want    string
	}{
		{
			name:    "the same quoted blank line from two rules",
			content: quoted, offset: 6,
			texts: []string{">\n", ">\n"},
			want:  ">\n",
		},
		{
			name:    "the longest requirement wins",
			content: quoted, offset: 6,
			texts: []string{">\n", ">\n>\n"},
			want:  ">\n>\n",
		},
		{
			name:    "a container-aware blank line beats a bare one",
			content: quoted, offset: 6,
			texts: []string{"\n", ">\n"},
			want:  ">\n",
		},
		{
			name:    "more blank lines win over a longer prefix",
			content: quoted, offset: 6,
			texts: []string{">\n", "\n\n"},
			want:  "\n\n",
		},
		{
			name:    "more blank lines win whatever the rule order",
			content: quoted, offset: 6,
			texts: []string{"  > >\n", "\n\n\n"},
			want:  "\n\n\n",
		},
		{
			name:    "nested containers with indentation",
			content: "- item\n\n  > > # A\n  > > ## B\n", offset: 18,
			texts: []string{"  > >\n", "  > >\n"},
			want:  "  > >\n",
		},
		{
			name:    "converted to CRLF and still merged",
			content: "> # A\r\n> ## B\r\n", offset: 7,
			texts: []string{">\n", ">\n"},
			want:  ">\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var rules []lint.Rule
			for idx, text := range tt.texts {
				rules = append(rules, insertRule(fmt.Sprintf("TEST%03d", idx+1), tt.offset, text))
			}

			result := lintWithRules(t, tt.content, true, rules...)

			assert.Equal(t, []fix.TextEdit{{StartOffset: tt.offset, EndOffset: tt.offset, NewText: tt.want}}, result.Edits,
				"blank-line insertions at one boundary must insert one blank line")
		})
	}
}

func TestEngine_DoesNotMergeInsertionsWithContent(t *testing.T) {
	t.Parallel()

	result := lintWithRules(t, "> # A\n> ## B\n", true,
		insertRule("TEST001", 6, ">\n"),
		insertRule("TEST002", 6, "> text\n"),
	)

	assert.Equal(t, []fix.TextEdit{
		{StartOffset: 6, EndOffset: 6, NewText: ">\n"},
		{StartOffset: 6, EndOffset: 6, NewText: "> text\n"},
	}, result.Edits)
}

func TestEngine_RejectsFixThatSplitsCRLF(t *testing.T) {
	t.Parallel()

	splitting := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		// Offset 8 is between the "\r" and the "\n" after the heading.
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: 8, EndOffset: 8, NewText: "\n"})}, nil
	})

	result := lintWithRules(t, "# Title\r\nPara\r\n", true, splitting)

	requireRuleError(t, result, "TEST001", lint.ErrInvalidFix)
	assert.Empty(t, result.Edits)
}

func TestRuleContext_LineEnding(t *testing.T) {
	t.Parallel()

	var seen []string
	recorder := newFuncRule("TEST001", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		seen = append(seen, ctx.LineEnding())
		return nil, nil
	})

	lintWithRules(t, "a\r\nb\r\n", false, recorder)
	lintWithRules(t, "a\nb\n", false, recorder)
	assert.Equal(t, []string{"\r\n", "\n"}, seen)

	direct := lint.NewRuleContext(context.Background(), nil, nil, nil)
	assert.Equal(t, "\n", direct.LineEnding(), "a context without a file defaults to LF")
}
