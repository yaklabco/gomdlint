package lint

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/fsutil"
)

// DefaultMaxFixPasses is the maximum number of fix passes to prevent infinite loops.
// This should be sufficient for most files - if more passes are needed, there may
// be rules that create issues for each other.
const DefaultMaxFixPasses = 10

// Pipeline error types for categorization.
var (
	// ErrFileNotFound indicates the file does not exist.
	ErrFileNotFound = errors.New("file not found")

	// ErrPermissionDenied indicates a permission error.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrParseFailure indicates a parsing error.
	ErrParseFailure = errors.New("parse failure")

	// ErrWriteFailure indicates a write error.
	ErrWriteFailure = errors.New("write failure")
)

// PipelineResult contains the result of processing a single file through the safety pipeline.
type PipelineResult struct {
	// FileResult contains lint diagnostics and edits from the FINAL pass.
	// For multi-pass fixing, this reflects the state after all passes.
	*FileResult

	// Path is the file path that was processed.
	Path string

	// OriginalInfo is the file state before processing.
	OriginalInfo *fsutil.FileInfo

	// Modified is true if the file content was changed.
	Modified bool

	// ModifiedContent is the new content after applying edits (nil if not modified).
	ModifiedContent []byte

	// Diff is the unified diff for dry-run mode (nil if not in dry-run).
	Diff *fix.Diff

	// Skipped is true if the file was skipped (e.g., due to concurrent modification).
	Skipped bool

	// SkipReason explains why the file was skipped.
	SkipReason string

	// BackupCreated is true if a backup was created for this file.
	BackupCreated bool

	// Written is true if the file was written to disk.
	Written bool

	// FixPasses is the number of fix passes performed (for multi-pass fixing).
	FixPasses int

	// TotalEditsApplied is the total number of edits applied across all passes.
	TotalEditsApplied int
}

// Summary returns a human-readable summary of the pipeline result.
func (pr *PipelineResult) Summary() string {
	if pr.Skipped {
		return "skipped: " + pr.SkipReason
	}
	if pr.Written {
		if pr.BackupCreated {
			return "fixed (backup created)"
		}
		return "fixed"
	}
	if pr.Modified {
		return "changes pending"
	}
	if pr.FileResult != nil && pr.HasIssues() {
		return "issues found"
	}
	return "ok"
}

// PipelineOptions controls safety pipeline behavior.
type PipelineOptions struct {
	// Fix enables auto-fix mode.
	Fix bool

	// DryRun generates diffs without writing files.
	DryRun bool

	// Backup configures backup behavior.
	Backup fsutil.BackupConfig

	// StrictRaceDetection uses hash comparison for modification detection.
	// When false, only mod time and size are checked.
	StrictRaceDetection bool

	// ReParseAfterFix re-parses the modified content to validate fixes.
	ReParseAfterFix bool

	// MaxFixPasses limits the number of fix iterations to prevent infinite loops.
	// When conflicting edits are skipped, a subsequent pass may be able to fix them.
	// Set to 0 to use DefaultMaxFixPasses.
	MaxFixPasses int
}

// DefaultPipelineOptions returns sensible defaults.
func DefaultPipelineOptions() PipelineOptions {
	return PipelineOptions{
		Fix:                 false,
		DryRun:              false,
		Backup:              fsutil.DefaultBackupConfig(),
		StrictRaceDetection: true,
		ReParseAfterFix:     false,
	}
}

// Pipeline orchestrates the safe processing of a single file.
type Pipeline struct {
	// Engine is the lint engine used for parsing and rule execution.
	Engine *Engine
}

// NewPipeline creates a new safety pipeline with the given engine.
func NewPipeline(engine *Engine) *Pipeline {
	return &Pipeline{Engine: engine}
}

// ProcessFile runs the full safety pipeline for a single file.
//
// The pipeline performs the following steps:
//  1. Read and hash the original file.
//  2. Multi-pass fix loop (if fix mode enabled):
//     a. Run the lint engine.
//     b. If no edits, exit loop.
//     c. Apply edits in memory.
//     d. Repeat with modified content until stable or max passes.
//  3. Optionally re-parse to validate fixes.
//  4. Generate diff (if dry-run mode).
//  5. Check for concurrent modifications.
//  6. Create backup (if enabled).
//  7. Write the modified content atomically.
func (p *Pipeline) ProcessFile(
	ctx context.Context,
	path string,
	cfg *config.Config,
	opts PipelineOptions,
) (*PipelineResult, error) {
	result := &PipelineResult{
		Path: path,
	}

	// Step 1: Read and hash the original file.
	originalContent, info, err := fsutil.ReadFile(ctx, path)
	if err != nil {
		return nil, categorizeError(err)
	}
	result.OriginalInfo = info

	// Determine max passes (use default if not set).
	maxPasses := opts.MaxFixPasses
	if maxPasses <= 0 {
		maxPasses = DefaultMaxFixPasses
	}

	// Working content starts as original.
	content := originalContent
	var fileResult *FileResult

	// Step 2: Multi-pass fix loop.
	for range maxPasses {
		// Check for cancellation.
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("processing cancelled: %w", ctx.Err())
		default:
		}

		// Run the lint engine on current content.
		var lintErr error
		fileResult, lintErr = p.Engine.LintFile(ctx, path, content, cfg)
		if lintErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrParseFailure, lintErr)
		}

		// If not in fix mode or no edits available, we're done.
		if !opts.Fix || len(fileResult.Edits) == 0 {
			break
		}

		// Apply edits in memory.
		content = fix.ApplyEdits(content, fileResult.Edits)
		result.FixPasses++
		result.TotalEditsApplied += len(fileResult.Edits)
		result.Modified = true
	}

	// Store the final lint result.
	result.FileResult = fileResult
	result.ModifiedContent = content

	// If no modifications were made, clear ModifiedContent.
	if !result.Modified {
		result.ModifiedContent = nil
		return result, nil
	}

	// Step 3: Optional re-parse to validate fixes.
	if opts.ReParseAfterFix {
		_, err := p.Engine.Parser.Parse(ctx, path, content)
		if err != nil {
			// Re-parse failed; abort fix.
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("re-parse failed: %v", err)
			result.Modified = false
			result.ModifiedContent = nil
			return result, nil
		}
	}

	// Step 4: Handle dry-run mode.
	if opts.DryRun {
		result.Diff = fix.GenerateDiff(path, originalContent, content)
		return result, nil
	}

	// Step 5: Check for concurrent modifications before writing.
	modified, err := p.checkModified(ctx, info, opts.StrictRaceDetection)
	if err != nil {
		return nil, fmt.Errorf("check modified: %w", err)
	}
	if modified {
		result.Skipped = true
		result.SkipReason = "file modified during processing"
		return result, nil
	}

	// Step 6: Create backup if enabled.
	if opts.Backup.Enabled {
		created, err := fsutil.CreateBackup(ctx, path, opts.Backup)
		if err != nil {
			return nil, fmt.Errorf("create backup: %w", err)
		}
		result.BackupCreated = created
	}

	// Step 7: Write the modified content atomically.
	if err := fsutil.WriteAtomic(ctx, path, content, info.Mode); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWriteFailure, err)
	}
	result.Written = true

	return result, nil
}

// ProcessContent processes in-memory content without file I/O.
// This is useful for testing or when content is already loaded.
// It supports multi-pass fixing just like ProcessFile.
func (p *Pipeline) ProcessContent(
	ctx context.Context,
	path string,
	originalContent []byte,
	cfg *config.Config,
	opts PipelineOptions,
) (*PipelineResult, error) {
	result := &PipelineResult{
		Path: path,
	}

	// Determine max passes (use default if not set).
	maxPasses := opts.MaxFixPasses
	if maxPasses <= 0 {
		maxPasses = DefaultMaxFixPasses
	}

	// Working content starts as original.
	content := originalContent
	var fileResult *FileResult

	// Multi-pass fix loop.
	for range maxPasses {
		// Check for cancellation.
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("processing cancelled: %w", ctx.Err())
		default:
		}

		// Run the lint engine on current content.
		var lintErr error
		fileResult, lintErr = p.Engine.LintFile(ctx, path, content, cfg)
		if lintErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrParseFailure, lintErr)
		}

		// If not in fix mode or no edits available, we're done.
		if !opts.Fix || len(fileResult.Edits) == 0 {
			break
		}

		// Apply edits in memory.
		content = fix.ApplyEdits(content, fileResult.Edits)
		result.FixPasses++
		result.TotalEditsApplied += len(fileResult.Edits)
		result.Modified = true
	}

	// Store the final lint result.
	result.FileResult = fileResult
	result.ModifiedContent = content

	// If no modifications were made, clear ModifiedContent.
	if !result.Modified {
		result.ModifiedContent = nil
		return result, nil
	}

	// Optional re-parse to validate fixes.
	if opts.ReParseAfterFix {
		_, err := p.Engine.Parser.Parse(ctx, path, content)
		if err != nil {
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("re-parse failed: %v", err)
			result.Modified = false
			result.ModifiedContent = nil
			return result, nil
		}
	}

	// Generate diff for review.
	if opts.DryRun {
		result.Diff = fix.GenerateDiff(path, originalContent, content)
	}

	return result, nil
}

// checkModified checks if a file has been modified since it was read.
func (p *Pipeline) checkModified(ctx context.Context, info *fsutil.FileInfo, strict bool) (bool, error) {
	var modified bool
	var err error

	if strict {
		modified, err = fsutil.CheckModified(ctx, info)
	} else {
		modified, err = fsutil.CheckModifiedQuick(ctx, info)
	}

	if err != nil {
		return false, fmt.Errorf("check modified: %w", err)
	}
	return modified, nil
}

// categorizeError wraps an error with the appropriate pipeline error type.
// It uses errors.Is for robust error detection rather than string matching.
func categorizeError(err error) error {
	if err == nil {
		return nil
	}

	// Check for file not found errors.
	if errors.Is(err, fsutil.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %w", ErrFileNotFound, err)
	}

	// Check for permission errors.
	if errors.Is(err, fsutil.ErrPermissionDenied) || errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w: %w", ErrPermissionDenied, err)
	}

	return err
}

// IsPipelineError checks if an error is a known pipeline error type.
func IsPipelineError(err error) bool {
	return errors.Is(err, ErrFileNotFound) ||
		errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrParseFailure) ||
		errors.Is(err, ErrWriteFailure)
}

// BackupConfigFromConfig creates an fsutil.BackupConfig from config.Config.
func BackupConfigFromConfig(cfg *config.Config) fsutil.BackupConfig {
	if cfg == nil {
		return fsutil.DefaultBackupConfig()
	}
	return fsutil.BackupConfig{
		Enabled: cfg.Backups.Enabled && !cfg.NoBackups,
		Mode:    fsutil.BackupMode(cfg.Backups.Mode),
	}
}

// PipelineOptionsFromConfig creates PipelineOptions from config.Config.
func PipelineOptionsFromConfig(cfg *config.Config) PipelineOptions {
	if cfg == nil {
		return DefaultPipelineOptions()
	}
	return PipelineOptions{
		Fix:                 cfg.Fix,
		DryRun:              cfg.DryRun,
		Backup:              BackupConfigFromConfig(cfg),
		StrictRaceDetection: true,
		ReParseAfterFix:     false,
	}
}
