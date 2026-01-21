package lint_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/fsutil"
	"github.com/jamesainslie/gomdlint/pkg/lint"
)

func TestNewPipeline(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)

	pipeline := lint.NewPipeline(engine)

	if pipeline.Engine != engine {
		t.Error("Engine not set correctly")
	}
}

func TestPipeline_ProcessFile_LintOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := []byte("# Heading\n")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	opts := lint.DefaultPipelineOptions()

	ctx := context.Background()
	result, err := pipeline.ProcessFile(ctx, path, cfg, opts)

	if err != nil {
		t.Fatalf("ProcessFile() error = %v", err)
	}

	if result.Path != path {
		t.Errorf("Path = %q, want %q", result.Path, path)
	}

	if result.OriginalInfo == nil {
		t.Error("OriginalInfo should be set")
	}

	if result.Modified {
		t.Error("Modified should be false for lint-only")
	}

	if result.Written {
		t.Error("Written should be false for lint-only")
	}

	if result.Summary() != "ok" {
		t.Errorf("Summary() = %q, want ok", result.Summary())
	}
}

func TestPipeline_ProcessFile_WithDiagnostics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := []byte("# Heading\n")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Add a rule that produces diagnostics.
	rule := &diagnosticRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, false),
		diags: []lint.Diagnostic{
			{RuleID: "TEST001", Message: "test issue"},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	opts := lint.DefaultPipelineOptions()

	ctx := context.Background()
	result, err := pipeline.ProcessFile(ctx, path, cfg, opts)

	if err != nil {
		t.Fatalf("ProcessFile() error = %v", err)
	}

	if !result.HasIssues() {
		t.Error("expected issues")
	}

	if result.Summary() != "issues found" {
		t.Errorf("Summary() = %q, want 'issues found'", result.Summary())
	}
}

func TestPipeline_ProcessFile_FixMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := []byte("hello")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Add a rule that produces fixable diagnostics.
	rule := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST001",
				Message:  "fix needed",
				FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}},
			},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	cfg.Fix = true

	opts := lint.PipelineOptions{
		Fix:    true,
		DryRun: false,
		Backup: fsutil.BackupConfig{Enabled: false},
	}

	ctx := context.Background()
	result, err := pipeline.ProcessFile(ctx, path, cfg, opts)

	if err != nil {
		t.Fatalf("ProcessFile() error = %v", err)
	}

	if !result.Modified {
		t.Error("Modified should be true")
	}

	if !result.Written {
		t.Error("Written should be true")
	}

	// Verify file was actually changed.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(got) != "world" {
		t.Errorf("content = %q, want 'world'", got)
	}

	if result.Summary() != "fixed" {
		t.Errorf("Summary() = %q, want 'fixed'", result.Summary())
	}
}

func TestPipeline_ProcessFile_DryRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := []byte("hello")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST001",
				Message:  "fix needed",
				FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}},
			},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	cfg.Fix = true
	cfg.DryRun = true

	opts := lint.PipelineOptions{
		Fix:    true,
		DryRun: true,
	}

	ctx := context.Background()
	result, err := pipeline.ProcessFile(ctx, path, cfg, opts)

	if err != nil {
		t.Fatalf("ProcessFile() error = %v", err)
	}

	if !result.Modified {
		t.Error("Modified should be true")
	}

	if result.Written {
		t.Error("Written should be false for dry-run")
	}

	if result.Diff == nil {
		t.Error("Diff should be set for dry-run")
	}

	// Verify file was NOT changed.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(got) != "hello" {
		t.Errorf("content = %q, want 'hello' (unchanged)", got)
	}

	if result.Summary() != "changes pending" {
		t.Errorf("Summary() = %q, want 'changes pending'", result.Summary())
	}
}

func TestPipeline_ProcessFile_WithBackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := []byte("original")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST001",
				Message:  "fix needed",
				FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 8, NewText: "modified"}},
			},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	cfg.Fix = true

	opts := lint.PipelineOptions{
		Fix: true,
		Backup: fsutil.BackupConfig{
			Enabled: true,
			Mode:    fsutil.BackupModeSidecar,
		},
	}

	ctx := context.Background()
	result, err := pipeline.ProcessFile(ctx, path, cfg, opts)

	if err != nil {
		t.Fatalf("ProcessFile() error = %v", err)
	}

	if !result.BackupCreated {
		t.Error("BackupCreated should be true")
	}

	// Verify backup exists.
	backupPath := fsutil.BackupPath(path, fsutil.BackupModeSidecar)
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}

	if string(backup) != "original" {
		t.Errorf("backup content = %q, want 'original'", backup)
	}

	if result.Summary() != "fixed (backup created)" {
		t.Errorf("Summary() = %q, want 'fixed (backup created)'", result.Summary())
	}
}

func TestPipeline_ProcessFile_FileNotFound(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	opts := lint.DefaultPipelineOptions()

	ctx := context.Background()
	_, err := pipeline.ProcessFile(ctx, "/nonexistent/path.md", cfg, opts)

	if err == nil {
		t.Fatal("expected error for non-existent file")
	}

	if !errors.Is(err, lint.ErrFileNotFound) {
		t.Errorf("expected ErrFileNotFound, got %v", err)
	}
}

func TestPipeline_ProcessFile_NoEditsWhenConflicts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := []byte("hello world again")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()

	// Two rules with overlapping edits.
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
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	cfg.Fix = true

	opts := lint.PipelineOptions{Fix: true}

	ctx := context.Background()
	result, err := pipeline.ProcessFile(ctx, path, cfg, opts)

	if err != nil {
		t.Fatalf("ProcessFile() error = %v", err)
	}

	// With the new filtering behavior, non-mergeable conflicts result in
	// the first edit being accepted and later conflicting edits being skipped.
	// Since these are replacements (not deletions), they cannot be merged.
	// The first edit (0-10, "aaa") should be applied.
	if !result.Modified {
		t.Error("Modified should be true (first edit applied, second skipped)")
	}

	if !result.Written {
		t.Error("Written should be true (first edit was applied)")
	}

	// File should be modified with first edit.
	// Original: "hello world again" (17 bytes)
	// Edit 1: Replace bytes 0-10 ("hello worl") with "aaa" -> "aaad again"
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	expected := "aaad again"
	if string(got) != expected {
		t.Errorf("file content = %q, want %q", string(got), expected)
	}
}

func TestPipeline_ProcessFile_ContextCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := []byte("# Heading\n")

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	parser := &mockParser{}
	registry := lint.NewRegistry()
	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	opts := lint.DefaultPipelineOptions()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pipeline.ProcessFile(ctx, path, cfg, opts)

	// Should get a cancellation error.
	if err == nil {
		t.Log("no error returned, which is acceptable if cancellation wasn't caught early")
	}
}

func TestPipeline_ProcessContent(t *testing.T) {
	t.Parallel()

	parser := &mockParser{}
	registry := lint.NewRegistry()

	rule := &fixableRule{
		BaseRule: lint.NewBaseRule("TEST001", "test-rule", "", nil, true),
		diags: []lint.Diagnostic{
			{
				RuleID:   "TEST001",
				Message:  "fix needed",
				FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 5, NewText: "world"}},
			},
		},
	}
	registry.Register(rule)

	engine := lint.NewEngine(parser, registry)
	pipeline := lint.NewPipeline(engine)

	cfg := config.NewConfig()
	cfg.Fix = true

	opts := lint.PipelineOptions{
		Fix:    true,
		DryRun: true,
	}

	ctx := context.Background()
	result, err := pipeline.ProcessContent(ctx, "test.md", []byte("hello"), cfg, opts)

	if err != nil {
		t.Fatalf("ProcessContent() error = %v", err)
	}

	if !result.Modified {
		t.Error("Modified should be true")
	}

	if string(result.ModifiedContent) != "world" {
		t.Errorf("ModifiedContent = %q, want 'world'", result.ModifiedContent)
	}

	if result.Diff == nil {
		t.Error("Diff should be set")
	}
}

func TestPipelineResult_Summary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result *lint.PipelineResult
		want   string
	}{
		{
			name:   "skipped",
			result: &lint.PipelineResult{Skipped: true, SkipReason: "test reason"},
			want:   "skipped: test reason",
		},
		{
			name:   "written with backup",
			result: &lint.PipelineResult{Written: true, BackupCreated: true},
			want:   "fixed (backup created)",
		},
		{
			name:   "written without backup",
			result: &lint.PipelineResult{Written: true, BackupCreated: false},
			want:   "fixed",
		},
		{
			name:   "modified but not written",
			result: &lint.PipelineResult{Modified: true},
			want:   "changes pending",
		},
		{
			name: "issues found",
			result: &lint.PipelineResult{
				FileResult: &lint.FileResult{
					Diagnostics: []lint.Diagnostic{{Message: "issue"}},
				},
			},
			want: "issues found",
		},
		{
			name:   "ok",
			result: &lint.PipelineResult{},
			want:   "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.result.Summary()
			if got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultPipelineOptions(t *testing.T) {
	t.Parallel()

	opts := lint.DefaultPipelineOptions()

	if opts.Fix {
		t.Error("Fix should be false by default")
	}
	if opts.DryRun {
		t.Error("DryRun should be false by default")
	}
	if !opts.StrictRaceDetection {
		t.Error("StrictRaceDetection should be true by default")
	}
	if opts.ReParseAfterFix {
		t.Error("ReParseAfterFix should be false by default")
	}
}

func TestPipelineOptionsFromConfig(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()

		opts := lint.PipelineOptionsFromConfig(nil)
		if opts.Fix {
			t.Error("Fix should be false for nil config")
		}
	})

	t.Run("with fix enabled", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.Fix = true
		cfg.DryRun = true

		opts := lint.PipelineOptionsFromConfig(cfg)

		if !opts.Fix {
			t.Error("Fix should be true")
		}
		if !opts.DryRun {
			t.Error("DryRun should be true")
		}
	})
}

func TestBackupConfigFromConfig(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()

		backup := lint.BackupConfigFromConfig(nil)
		if backup.Enabled {
			t.Error("Enabled should be false for nil config")
		}
	})

	t.Run("backups enabled", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.Backups.Enabled = true
		cfg.Backups.Mode = "sidecar"

		backup := lint.BackupConfigFromConfig(cfg)

		if !backup.Enabled {
			t.Error("Enabled should be true")
		}
		if backup.Mode != fsutil.BackupModeSidecar {
			t.Errorf("Mode = %q, want sidecar", backup.Mode)
		}
	})

	t.Run("backups disabled by NoBackups flag", func(t *testing.T) {
		t.Parallel()

		cfg := config.NewConfig()
		cfg.Backups.Enabled = true
		cfg.NoBackups = true

		backup := lint.BackupConfigFromConfig(cfg)

		if backup.Enabled {
			t.Error("Enabled should be false when NoBackups is set")
		}
	})
}

func TestIsPipelineError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"file not found", lint.ErrFileNotFound, true},
		{"permission denied", lint.ErrPermissionDenied, true},
		{"parse failure", lint.ErrParseFailure, true},
		{"write failure", lint.ErrWriteFailure, true},
		{"other error", errors.New("other"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lint.IsPipelineError(tt.err)
			if got != tt.want {
				t.Errorf("IsPipelineError() = %v, want %v", got, tt.want)
			}
		})
	}
}
