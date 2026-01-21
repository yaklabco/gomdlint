package lint_test

import (
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

const testRuleIDDiag = "MD001"

func TestNewDiagnostic(t *testing.T) {
	t.Parallel()

	// Create a file snapshot with tokens for position calculation.
	file := &mdast.FileSnapshot{
		Path:    "test.md",
		Content: []byte("# Hello\n"),
		Lines:   mdast.BuildLines([]byte("# Hello\n")),
		Tokens: []mdast.Token{
			{Kind: mdast.TokHeadingMarker, StartOffset: 0, EndOffset: 1},
			{Kind: mdast.TokWhitespace, StartOffset: 1, EndOffset: 2},
			{Kind: mdast.TokText, StartOffset: 2, EndOffset: 7},
			{Kind: mdast.TokNewline, StartOffset: 7, EndOffset: 8},
		},
	}

	node := mdast.NewNode(mdast.NodeHeading)
	node.FirstToken = 0
	node.LastToken = 3
	node.File = file

	diag := lint.NewDiagnostic(testRuleIDDiag, node, "test message").Build()

	if diag.RuleID != testRuleIDDiag {
		t.Errorf("RuleID = %q, want MD001", diag.RuleID)
	}
	if diag.Message != "test message" {
		t.Errorf("Message = %q, want test message", diag.Message)
	}
	if diag.FilePath != "test.md" {
		t.Errorf("FilePath = %q, want test.md", diag.FilePath)
	}
	if diag.StartLine != 1 {
		t.Errorf("StartLine = %d, want 1", diag.StartLine)
	}
}

func TestNewDiagnostic_NilNode(t *testing.T) {
	t.Parallel()

	diag := lint.NewDiagnostic(testRuleIDDiag, nil, "test message").Build()

	if diag.RuleID != testRuleIDDiag {
		t.Errorf("RuleID = %q, want MD001", diag.RuleID)
	}
	if diag.FilePath != "" {
		t.Errorf("FilePath = %q, want empty", diag.FilePath)
	}
	if diag.StartLine != 0 {
		t.Errorf("StartLine = %d, want 0", diag.StartLine)
	}
}

func TestNewDiagnosticAt(t *testing.T) {
	t.Parallel()

	pos := mdast.SourcePosition{
		StartLine:   5,
		StartColumn: 10,
		EndLine:     5,
		EndColumn:   20,
	}

	diag := lint.NewDiagnosticAt("MD002", "file.md", pos, "custom position").Build()

	if diag.RuleID != "MD002" {
		t.Errorf("RuleID = %q, want MD002", diag.RuleID)
	}
	if diag.FilePath != "file.md" {
		t.Errorf("FilePath = %q, want file.md", diag.FilePath)
	}
	if diag.StartLine != 5 {
		t.Errorf("StartLine = %d, want 5", diag.StartLine)
	}
	if diag.StartColumn != 10 {
		t.Errorf("StartColumn = %d, want 10", diag.StartColumn)
	}
	if diag.EndLine != 5 {
		t.Errorf("EndLine = %d, want 5", diag.EndLine)
	}
	if diag.EndColumn != 20 {
		t.Errorf("EndColumn = %d, want 20", diag.EndColumn)
	}
}

func TestDiagnosticBuilder_WithSeverity(t *testing.T) {
	t.Parallel()

	diag := lint.NewDiagnostic(testRuleIDDiag, nil, "test").
		WithSeverity(config.SeverityError).
		Build()

	if diag.Severity != config.SeverityError {
		t.Errorf("Severity = %v, want error", diag.Severity)
	}
}

func TestDiagnosticBuilder_WithSuggestion(t *testing.T) {
	t.Parallel()

	diag := lint.NewDiagnostic(testRuleIDDiag, nil, "test").
		WithSuggestion("fix it this way").
		Build()

	if diag.Suggestion != "fix it this way" {
		t.Errorf("Suggestion = %q, want fix it this way", diag.Suggestion)
	}
}

func TestDiagnosticBuilder_WithFix(t *testing.T) {
	t.Parallel()

	builder := fix.NewEditBuilder()
	builder.ReplaceRange(0, 5, "hello")
	builder.ReplaceRange(10, 15, "world")

	diag := lint.NewDiagnostic(testRuleIDDiag, nil, "test").
		WithFix(builder).
		Build()

	if len(diag.FixEdits) != 2 {
		t.Fatalf("FixEdits length = %d, want 2", len(diag.FixEdits))
	}

	if diag.FixEdits[0].StartOffset != 0 {
		t.Errorf("FixEdits[0].StartOffset = %d, want 0", diag.FixEdits[0].StartOffset)
	}
}

func TestDiagnosticBuilder_WithFix_Nil(t *testing.T) {
	t.Parallel()

	diag := lint.NewDiagnostic(testRuleIDDiag, nil, "test").
		WithFix(nil).
		Build()

	if len(diag.FixEdits) != 0 {
		t.Errorf("FixEdits length = %d, want 0", len(diag.FixEdits))
	}
}

func TestDiagnosticBuilder_WithEdit(t *testing.T) {
	t.Parallel()

	edit := fix.TextEdit{StartOffset: 0, EndOffset: 5, NewText: "hello"}

	diag := lint.NewDiagnostic(testRuleIDDiag, nil, "test").
		WithEdit(edit).
		Build()

	if len(diag.FixEdits) != 1 {
		t.Fatalf("FixEdits length = %d, want 1", len(diag.FixEdits))
	}

	if diag.FixEdits[0] != edit {
		t.Error("FixEdits[0] does not match input edit")
	}
}

func TestDiagnosticBuilder_Chaining(t *testing.T) {
	t.Parallel()

	edit := fix.TextEdit{StartOffset: 0, EndOffset: 5, NewText: "hello"}

	diag := lint.NewDiagnostic(testRuleIDDiag, nil, "test message").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("try this").
		WithEdit(edit).
		Build()

	if diag.RuleID != testRuleIDDiag {
		t.Errorf("RuleID = %q, want MD001", diag.RuleID)
	}
	if diag.Message != "test message" {
		t.Errorf("Message = %q, want test message", diag.Message)
	}
	if diag.Severity != config.SeverityWarning {
		t.Errorf("Severity = %v, want warning", diag.Severity)
	}
	if diag.Suggestion != "try this" {
		t.Errorf("Suggestion = %q, want try this", diag.Suggestion)
	}
	if len(diag.FixEdits) != 1 {
		t.Errorf("FixEdits length = %d, want 1", len(diag.FixEdits))
	}
}

func TestDiagnostic_HasFix(t *testing.T) {
	t.Parallel()

	t.Run("has fix", func(t *testing.T) {
		t.Parallel()

		diag := lint.Diagnostic{
			FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1, NewText: "x"}},
		}

		if !diag.HasFix() {
			t.Error("expected HasFix to return true")
		}
	})

	t.Run("no fix", func(t *testing.T) {
		t.Parallel()

		diag := lint.Diagnostic{}

		if diag.HasFix() {
			t.Error("expected HasFix to return false")
		}
	})
}

func TestDiagnostic_SourcePosition(t *testing.T) {
	t.Parallel()

	diag := lint.Diagnostic{
		StartLine:   1,
		StartColumn: 5,
		EndLine:     2,
		EndColumn:   10,
	}

	pos := diag.SourcePosition()

	if pos.StartLine != 1 {
		t.Errorf("StartLine = %d, want 1", pos.StartLine)
	}
	if pos.StartColumn != 5 {
		t.Errorf("StartColumn = %d, want 5", pos.StartColumn)
	}
	if pos.EndLine != 2 {
		t.Errorf("EndLine = %d, want 2", pos.EndLine)
	}
	if pos.EndColumn != 10 {
		t.Errorf("EndColumn = %d, want 10", pos.EndColumn)
	}
}

func TestNewDiagnosticAtWithRegistry_IncludesRuleName(t *testing.T) {
	t.Parallel()

	// Register a rule so we can look up its name
	reg := lint.NewRegistry()
	reg.Register(&testMockRule{id: "MD009", name: "no-trailing-spaces"})

	pos := mdast.SourcePosition{StartLine: 1, StartColumn: 1}
	diag := lint.NewDiagnosticAtWithRegistry("MD009", "test.md", pos, "test message", reg).Build()

	if diag.RuleID != "MD009" {
		t.Errorf("RuleID = %q, want MD009", diag.RuleID)
	}
	if diag.RuleName != "no-trailing-spaces" {
		t.Errorf("RuleName = %q, want no-trailing-spaces", diag.RuleName)
	}
}

func TestNewDiagnosticAtWithRegistry_NilRegistry(t *testing.T) {
	t.Parallel()

	pos := mdast.SourcePosition{StartLine: 1, StartColumn: 1}
	diag := lint.NewDiagnosticAtWithRegistry("MD009", "test.md", pos, "test message", nil).Build()

	if diag.RuleID != "MD009" {
		t.Errorf("RuleID = %q, want MD009", diag.RuleID)
	}
	if diag.RuleName != "" {
		t.Errorf("RuleName = %q, want empty string", diag.RuleName)
	}
}

func TestNewDiagnosticAtWithRegistry_UnknownRule(t *testing.T) {
	t.Parallel()

	reg := lint.NewRegistry()
	// Don't register the rule

	pos := mdast.SourcePosition{StartLine: 1, StartColumn: 1}
	diag := lint.NewDiagnosticAtWithRegistry("MD999", "test.md", pos, "test message", reg).Build()

	if diag.RuleID != "MD999" {
		t.Errorf("RuleID = %q, want MD999", diag.RuleID)
	}
	if diag.RuleName != "" {
		t.Errorf("RuleName = %q, want empty string", diag.RuleName)
	}
}

// testMockRule is a mock rule for testing diagnostic builder with registry.
type testMockRule struct {
	id   string
	name string
}

func (m *testMockRule) ID() string                                         { return m.id }
func (m *testMockRule) Name() string                                       { return m.name }
func (m *testMockRule) Description() string                                { return "mock" }
func (m *testMockRule) DefaultEnabled() bool                               { return true }
func (m *testMockRule) DefaultSeverity() config.Severity                   { return config.SeverityWarning }
func (m *testMockRule) Tags() []string                                     { return nil }
func (m *testMockRule) CanFix() bool                                       { return false }
func (m *testMockRule) Apply(*lint.RuleContext) ([]lint.Diagnostic, error) { return nil, nil }
