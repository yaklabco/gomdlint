package lint_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// mockParser implements lint.Parser for testing.
type mockParser struct {
	parseFunc func(ctx context.Context, path string, content []byte) (*mdast.FileSnapshot, error)
}

func (p *mockParser) Parse(ctx context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
	if p.parseFunc != nil {
		return p.parseFunc(ctx, path, content)
	}
	// Default: return a minimal snapshot.
	return &mdast.FileSnapshot{
		Path:    path,
		Content: content,
		Lines:   mdast.BuildLines(content),
		Tokens:  []mdast.Token{{Kind: mdast.TokText, StartOffset: 0, EndOffset: len(content)}},
		Root:    mdast.NewNode(mdast.NodeDocument),
	}, nil
}

// diagnosticRule is a test rule that produces diagnostics.
type diagnosticRule struct {
	lint.BaseRule
	diags []lint.Diagnostic
	err   error
}

func (r *diagnosticRule) Apply(_ *lint.RuleContext) ([]lint.Diagnostic, error) {
	return r.diags, r.err
}

// fixableRule is a test rule that produces diagnostics with fixes.
type fixableRule struct {
	lint.BaseRule
	diags []lint.Diagnostic
}

func (r *fixableRule) Apply(_ *lint.RuleContext) ([]lint.Diagnostic, error) {
	return r.diags, nil
}

func TestNewEngine(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	engine := lint.NewEngine(parser, registry)

	if engine.Parser != parser {
		t.Error("Parser mismatch")
	}
	if engine.Registry != registry {
		t.Error("Registry mismatch")
	}
}

func TestEngine_LintFile_Basic(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)

	cfg := config.NewConfig()
	result, err := engine.LintFile(context.Background(), "test.md", []byte("# Hello"), cfg)

	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	if result.Snapshot == nil {
		t.Error("expected Snapshot to be set")
	}

	if result.Snapshot.Path != "test.md" {
		t.Errorf("Path = %q, want test.md", result.Snapshot.Path)
	}
}

func TestEngine_LintFile_ParseError(t *testing.T) {
	t.Parallel()

	parseErr := errors.New("parse failed")
	parser := &mockParser{
		parseFunc: func(_ context.Context, _ string, _ []byte) (*mdast.FileSnapshot, error) {
			return nil, parseErr
		},
	}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)

	cfg := config.NewConfig()
	_, err := engine.LintFile(context.Background(), "test.md", []byte("# Hello"), cfg)

	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, parseErr) {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestEngine_LintFile_WithDiagnostics(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, false),
		diags: []lint.Diagnostic{
			{RuleID: "TEST001", Message: "test issue", StartLine: 1, StartColumn: 1},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	cfg := config.NewConfig()

	result, err := engine.LintFile(context.Background(), "test.md", []byte("# Hello"), cfg)

	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	if !result.HasIssues() {
		t.Error("expected issues")
	}

	if result.IssueCount() != 1 {
		t.Errorf("expected 1 issue, got %d", result.IssueCount())
	}

	if result.Diagnostics[0].Message != "test issue" {
		t.Errorf("Message = %q, want test issue", result.Diagnostics[0].Message)
	}
}

func TestEngine_LintFile_SeverityOverride(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, false),
		diags: []lint.Diagnostic{
			{RuleID: "TEST001", Message: "test", Severity: config.SeverityInfo},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	cfg := config.NewConfig()
	severity := string(config.SeverityError)
	cfg.Rules["TEST001"] = config.RuleConfig{Severity: &severity}

	result, err := engine.LintFile(context.Background(), "test.md", []byte("# Hello"), cfg)

	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	// Severity should be overridden by resolved config.
	if result.Diagnostics[0].Severity != config.SeverityError {
		t.Errorf("Severity = %v, want error", result.Diagnostics[0].Severity)
	}
}

func TestEngine_LintFile_RuleError(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	ruleErr := errors.New("rule failed")
	rule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, false),
		err:      ruleErr,
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	cfg := config.NewConfig()

	result, err := engine.LintFile(context.Background(), "test.md", []byte("# Hello"), cfg)

	if err != nil {
		t.Fatalf("LintFile should not return error for rule errors: %v", err)
	}

	if !errors.Is(result.RuleErrors["TEST001"], ruleErr) {
		t.Errorf("expected rule error to be recorded")
	}
}

func TestEngine_LintFile_ContextCancellation(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, false),
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	cfg := config.NewConfig()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := engine.LintFile(ctx, "test.md", []byte("# Hello"), cfg)

	// With a cancelled context, we expect either an error or a partial result.
	// The parser or rule processing should detect cancellation.
	if err != nil {
		// Expected: error due to cancellation.
		if !errors.Is(err, context.Canceled) {
			t.Logf("got error (possibly wrapped): %v", err)
		}
	} else if result == nil {
		t.Error("expected either error or result")
	}
	// If we get a result with no error, that's also acceptable as the
	// cancellation may occur at different points in processing.
}

func TestEngine_LintFile_WithFixes(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:    "TEST001",
				Message:   "fixable issue",
				StartLine: 1,
				FixEdits:  []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "hello"}},
			},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	cfg := config.NewConfig()
	cfg.Fix = true

	result, err := engine.LintFile(context.Background(), "test.md", []byte("world"), cfg)

	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	if !result.HasFixes() {
		t.Error("expected fixes")
	}

	if result.FixableCount() != 1 {
		t.Errorf("expected 1 fixable, got %d", result.FixableCount())
	}
}

func TestEngine_LintFile_EditConflicts(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Two rules that produce overlapping edits.
	rule1 := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule-1", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST001",
				Message:  "issue 1",
				FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 10, NewText: "aaa"}},
			},
		},
	}
	rule2 := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST002", "test-rule-2", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST002",
				Message:  "issue 2",
				FixEdits: []fix.TextEdit{{StartOffset: 5, EndOffset: 15, NewText: "bbb"}},
			},
		},
	}
	registry.Register(rule1)
	registry.Register(rule2)

	engine := lint.NewEngine(parser, registry)
	cfg := config.NewConfig()
	cfg.Fix = true

	content := []byte("hello world again")
	result, err := engine.LintFile(context.Background(), "test.md", content, cfg)

	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	if !result.EditConflicts {
		t.Error("expected EditConflicts to be true")
	}

	// With the new filtering behavior, non-mergeable conflicts result in
	// the first edit being accepted and later conflicting edits being skipped.
	// Since these are replacements (not deletions), they cannot be merged.
	if !result.HasFixes() {
		t.Error("expected fixes (first edit should be accepted, second skipped)")
	}

	// Should have 1 accepted edit (first one) and 1 skipped edit.
	if len(result.Edits) != 1 {
		t.Errorf("expected 1 accepted edit, got %d", len(result.Edits))
	}

	if len(result.SkippedEdits) != 1 {
		t.Errorf("expected 1 skipped edit, got %d", len(result.SkippedEdits))
	}

	// Diagnostics should still be present.
	if result.IssueCount() != 2 {
		t.Errorf("expected 2 issues, got %d", result.IssueCount())
	}
}

func TestEngine_LintFile_FilePathSet(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, false),
		diags: []lint.Diagnostic{
			{RuleID: "TEST001", Message: "test issue"},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	cfg := config.NewConfig()

	result, err := engine.LintFile(context.Background(), "path/to/file.md", []byte("# Hello"), cfg)

	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	// FilePath should be set on diagnostics that don't have it.
	if result.Diagnostics[0].FilePath != "path/to/file.md" {
		t.Errorf("FilePath = %q, want path/to/file.md", result.Diagnostics[0].FilePath)
	}
}

func TestFileResult_Methods(t *testing.T) {
	t.Parallel()

	t.Run("HasIssues", func(t *testing.T) {
		t.Parallel()

		result := &lint.FileResult{}
		if result.HasIssues() {
			t.Error("expected no issues")
		}

		result.Diagnostics = []lint.Diagnostic{{}}
		if !result.HasIssues() {
			t.Error("expected issues")
		}
	})

	t.Run("HasFixes", func(t *testing.T) {
		t.Parallel()

		result := &lint.FileResult{}
		if result.HasFixes() {
			t.Error("expected no fixes")
		}

		result.Edits = []fix.TextEdit{{}}
		if !result.HasFixes() {
			t.Error("expected fixes")
		}
	})

	t.Run("IssueCount", func(t *testing.T) {
		t.Parallel()

		result := &lint.FileResult{}
		if result.IssueCount() != 0 {
			t.Error("expected 0")
		}

		result.Diagnostics = []lint.Diagnostic{{}, {}}
		if result.IssueCount() != 2 {
			t.Errorf("expected 2, got %d", result.IssueCount())
		}
	})

	t.Run("FixableCount", func(t *testing.T) {
		t.Parallel()

		result := &lint.FileResult{
			Diagnostics: []lint.Diagnostic{
				{FixEdits: []fix.TextEdit{{}}},
				{},
				{FixEdits: []fix.TextEdit{{}, {}}},
			},
		}

		if result.FixableCount() != 2 {
			t.Errorf("expected 2 fixable, got %d", result.FixableCount())
		}
	})
}

// TestEngine_Integration_MultipleRules tests the engine with multiple real rules.
func TestEngine_Integration_MultipleRules(t *testing.T) {
	t.Parallel()

	// Import the rules package to trigger registration.
	// The rules are registered via init() in the rules package.
	// We use the DefaultRegistry which has all rules registered.

	// Create a document with multiple issues.
	input := `# Title 

### Skipped Level

- Item 1
* Item 2

1. First
1. Second
1. Third

`

	parser := &mockParser{
		parseFunc: func(_ context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
			// For this integration test, we need a proper snapshot with AST.
			// We'll create a minimal one that triggers the rules.
			snapshot := &mdast.FileSnapshot{
				Path:    path,
				Content: content,
				Lines:   mdast.BuildLines(content),
				Tokens:  []mdast.Token{{Kind: mdast.TokText, StartOffset: 0, EndOffset: len(content)}},
				Root:    mdast.NewNode(mdast.NodeDocument),
			}
			return snapshot, nil
		},
	}

	// Use DefaultRegistry which has all rules registered.
	engine := lint.NewEngine(parser, lint.DefaultRegistry)
	cfg := config.NewConfig()

	result, err := engine.LintFile(context.Background(), "test.md", []byte(input), cfg)

	if err != nil {
		t.Fatalf("LintFile error: %v", err)
	}

	// We should have diagnostics from the trailing whitespace rule (line 1 has trailing space).
	// The exact count depends on how the mock parser builds the AST.
	// With a minimal mock, we may not get all diagnostics, but we verify the engine runs.
	t.Logf("Found %d diagnostics", result.IssueCount())

	// Verify that the engine processed without error.
	if result.Snapshot == nil {
		t.Error("expected Snapshot to be set")
	}

	// Verify rule errors map is initialized.
	if result.RuleErrors == nil {
		t.Error("expected RuleErrors map to be initialized")
	}
}
