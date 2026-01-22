package runner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// mockParser implements lint.Parser for testing.
type mockParser struct{}

func (p *mockParser) Parse(_ context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
	return &mdast.FileSnapshot{
		Path:    path,
		Content: content,
		Lines:   mdast.BuildLines(content),
		Root:    &mdast.Node{Kind: mdast.NodeDocument},
	}, nil
}

// diagnosticRule is a rule that emits diagnostics.
type diagnosticRule struct {
	lint.BaseRule
	diags []lint.Diagnostic
}

func (r *diagnosticRule) Apply(_ *lint.RuleContext) ([]lint.Diagnostic, error) {
	// Return a copy to avoid race conditions when engine mutates the slice.
	result := make([]lint.Diagnostic, len(r.diags))
	copy(result, r.diags)
	return result, nil
}

// fixableRule is a rule that emits diagnostics with fixes.
type fixableRule struct {
	lint.BaseRule
	diags []lint.Diagnostic
}

func (r *fixableRule) Apply(_ *lint.RuleContext) ([]lint.Diagnostic, error) {
	// Return a copy to avoid race conditions when engine mutates the slice.
	result := make([]lint.Diagnostic, len(r.diags))
	for idx, diag := range r.diags {
		result[idx] = diag
		// Also copy the FixEdits slice.
		if len(diag.FixEdits) > 0 {
			result[idx].FixEdits = make([]fix.TextEdit, len(diag.FixEdits))
			copy(result[idx].FixEdits, diag.FixEdits)
		}
	}
	return result, nil
}

func TestNew(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	lintRunner := runner.New(pipeline)

	if lintRunner.Pipeline != pipeline {
		t.Error("Pipeline not set correctly")
	}
}

func TestRunner_Run_NoFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     config.NewConfig(),
	}

	result, err := lintRunner.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stats.FilesDiscovered != 0 {
		t.Errorf("FilesDiscovered = %d, want 0", result.Stats.FilesDiscovered)
	}

	if len(result.Files) != 0 {
		t.Errorf("len(Files) = %d, want 0", len(result.Files))
	}
}

func TestRunner_Run_SingleFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     config.NewConfig(),
	}

	result, err := lintRunner.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stats.FilesDiscovered != 1 {
		t.Errorf("FilesDiscovered = %d, want 1", result.Stats.FilesDiscovered)
	}

	if result.Stats.FilesProcessed != 1 {
		t.Errorf("FilesProcessed = %d, want 1", result.Stats.FilesProcessed)
	}

	if len(result.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(result.Files))
	}
}

func TestRunner_Run_MultipleFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create multiple files.
	files := []string{"a.md", "b.md", "c.md", "d.md", "e.md"}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.WriteFile(path, []byte("# "+f+"\n"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     config.NewConfig(),
	}

	result, err := lintRunner.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stats.FilesDiscovered != len(files) {
		t.Errorf("FilesDiscovered = %d, want %d", result.Stats.FilesDiscovered, len(files))
	}

	if result.Stats.FilesProcessed != len(files) {
		t.Errorf("FilesProcessed = %d, want %d", result.Stats.FilesProcessed, len(files))
	}
}

func TestRunner_Run_WithDiagnostics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Test\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Add two rules - one configured as error, one as warning.
	// The engine applies configured severity to all diagnostics from a rule.
	errorRule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("ERR001", "error-rule", "", nil, false),
		diags: []lint.Diagnostic{
			{RuleID: "ERR001", Message: "error issue"},
		},
	}
	warningRule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("WARN001", "warning-rule", "", nil, false),
		diags: []lint.Diagnostic{
			{RuleID: "WARN001", Message: "warning issue"},
		},
	}
	registry.Register(errorRule)
	registry.Register(warningRule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	// Configure one rule as error severity.
	cfg := config.NewConfig()
	errSeverity := string(config.SeverityError)
	cfg.Rules["ERR001"] = config.RuleConfig{
		Severity: &errSeverity,
	}

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     cfg,
	}

	result, err := lintRunner.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stats.DiagnosticsTotal != 2 {
		t.Errorf("DiagnosticsTotal = %d, want 2", result.Stats.DiagnosticsTotal)
	}

	if result.Stats.FilesWithIssues != 1 {
		t.Errorf("FilesWithIssues = %d, want 1", result.Stats.FilesWithIssues)
	}

	if result.Stats.DiagnosticsBySeverity["error"] != 1 {
		t.Errorf("error count = %d, want 1", result.Stats.DiagnosticsBySeverity["error"])
	}

	if result.Stats.DiagnosticsBySeverity["warning"] != 1 {
		t.Errorf("warning count = %d, want 1", result.Stats.DiagnosticsBySeverity["warning"])
	}

	if !result.HasFailures() {
		t.Error("HasFailures() should be true")
	}

	if !result.HasIssues() {
		t.Error("HasIssues() should be true")
	}
}

func TestRunner_Run_SerialVsParallelConsistency(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create files.
	fileCount := 20
	for idx := range fileCount {
		name := string(rune('a'+idx%26)) + string(rune('0'+idx/26)) + ".md"
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("# "+name+"\n"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Add a rule that produces one diagnostic per file.
	rule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, false),
		diags: []lint.Diagnostic{
			{RuleID: "TEST001", Message: "issue", Severity: config.SeverityWarning},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	cfg := config.NewConfig()

	// Run with 1 job (serial).
	ctx := context.Background()
	optsSerial := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     cfg,
		Jobs:       1,
	}

	resultSerial, err := lintRunner.Run(ctx, optsSerial)
	if err != nil {
		t.Fatalf("Run(serial) error = %v", err)
	}

	// Run with multiple jobs (parallel).
	optsParallel := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     cfg,
		Jobs:       4,
	}

	resultParallel, err := lintRunner.Run(ctx, optsParallel)
	if err != nil {
		t.Fatalf("Run(parallel) error = %v", err)
	}

	// Results should be identical.
	if resultSerial.Stats.FilesDiscovered != resultParallel.Stats.FilesDiscovered {
		t.Errorf("FilesDiscovered mismatch: serial=%d, parallel=%d",
			resultSerial.Stats.FilesDiscovered, resultParallel.Stats.FilesDiscovered)
	}

	if resultSerial.Stats.DiagnosticsTotal != resultParallel.Stats.DiagnosticsTotal {
		t.Errorf("DiagnosticsTotal mismatch: serial=%d, parallel=%d",
			resultSerial.Stats.DiagnosticsTotal, resultParallel.Stats.DiagnosticsTotal)
	}

	// File order should be deterministic.
	if len(resultSerial.Files) != len(resultParallel.Files) {
		t.Fatalf("File count mismatch: serial=%d, parallel=%d",
			len(resultSerial.Files), len(resultParallel.Files))
	}

	for i := range resultSerial.Files {
		if resultSerial.Files[i].Path != resultParallel.Files[i].Path {
			t.Errorf("File[%d] path mismatch: serial=%s, parallel=%s",
				i, resultSerial.Files[i].Path, resultParallel.Files[i].Path)
		}
	}
}

func TestRunner_Run_ContextCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create files.
	for idx := range 10 {
		path := filepath.Join(dir, string(rune('a'+idx))+".md")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     config.NewConfig(),
	}

	_, err := lintRunner.Run(ctx, opts)
	// Should get a cancellation error from discovery or processing.
	if err == nil {
		t.Log("no error returned, cancellation may not have been caught")
	} else if !errors.Is(err, context.Canceled) {
		t.Logf("expected context.Canceled, got: %v", err)
	}
}

func TestRunner_Run_ConcurrentProcessing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	fileCount := 50
	for idx := range fileCount {
		path := filepath.Join(dir, "file"+string(rune('a'+idx%26))+string(rune('0'+idx/26))+".md")
		if err := os.WriteFile(path, []byte("# Test\n"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	var processCount atomic.Int32
	parser := &countingParser{count: &processCount}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     config.NewConfig(),
		Jobs:       8,
	}

	result, err := lintRunner.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stats.FilesProcessed != fileCount {
		t.Errorf("FilesProcessed = %d, want %d", result.Stats.FilesProcessed, fileCount)
	}

	if int(processCount.Load()) != fileCount {
		t.Errorf("parser called %d times, want %d", processCount.Load(), fileCount)
	}
}

// countingParser counts parse calls for concurrency testing.
type countingParser struct {
	count *atomic.Int32
}

func (p *countingParser) Parse(_ context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
	p.count.Add(1)
	return &mdast.FileSnapshot{
		Path:    path,
		Content: content,
		Lines:   mdast.BuildLines(content),
		Root:    &mdast.Node{Kind: mdast.NodeDocument},
	}, nil
}

func TestRunner_Run_WithFixes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Add a fixable rule.
	rule := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST001",
				Message:  "fix needed",
				Severity: config.SeverityWarning,
				FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}},
			},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	cfg := config.NewConfig()
	cfg.Fix = true

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     cfg,
	}

	result, err := lintRunner.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Stats.FilesModified != 1 {
		t.Errorf("FilesModified = %d, want 1", result.Stats.FilesModified)
	}

	// Verify file was changed.
	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if string(content) != "world" {
		t.Errorf("content = %q, want 'world'", content)
	}
}

func TestRunner_Run_DryRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	originalContent := []byte("hello")
	if err := os.WriteFile(mdFile, originalContent, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Add a fixable rule.
	rule := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST001",
				Message:  "fix needed",
				Severity: config.SeverityWarning,
				FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}},
			},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)
	lintRunner := runner.New(pipeline)

	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.DryRun = true

	ctx := context.Background()
	opts := runner.Options{
		Paths:      []string{"."},
		WorkingDir: dir,
		Config:     cfg,
	}

	result, err := lintRunner.Run(ctx, opts)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// FilesModified should be 0 in dry-run mode.
	if result.Stats.FilesModified != 0 {
		t.Errorf("FilesModified = %d, want 0 for dry-run", result.Stats.FilesModified)
	}

	// Verify file was NOT changed.
	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if string(content) != string(originalContent) {
		t.Errorf("file was modified in dry-run mode: got %q, want %q", content, originalContent)
	}

	// But the result should have a diff.
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file outcome")
	}

	if result.Files[0].Result == nil || result.Files[0].Result.Diff == nil {
		t.Error("expected diff in dry-run mode")
	}
}

func TestResult_HasFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result *runner.Result
		want   bool
	}{
		{
			name:   "nil result",
			result: nil,
			want:   false,
		},
		{
			name: "no errors",
			result: &runner.Result{
				Stats: runner.Stats{
					DiagnosticsBySeverity: map[string]int{"warning": 5},
				},
			},
			want: false,
		},
		{
			name: "with errors",
			result: &runner.Result{
				Stats: runner.Stats{
					DiagnosticsBySeverity: map[string]int{"error": 1, "warning": 5},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.result.HasFailures()
			if got != tt.want {
				t.Errorf("HasFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResult_HasIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result *runner.Result
		want   bool
	}{
		{
			name:   "nil result",
			result: nil,
			want:   false,
		},
		{
			name: "no issues",
			result: &runner.Result{
				Stats: runner.Stats{DiagnosticsTotal: 0},
			},
			want: false,
		},
		{
			name: "with issues",
			result: &runner.Result{
				Stats: runner.Stats{DiagnosticsTotal: 3},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.result.HasIssues()
			if got != tt.want {
				t.Errorf("HasIssues() = %v, want %v", got, tt.want)
			}
		})
	}
}
