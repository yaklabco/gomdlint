package lint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/fsutil"
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

	// ErrInternal indicates a bug: processing the file panicked. The panic
	// was recovered so other files are still processed.
	ErrInternal = errors.New("internal error")
)

// PipelineResult contains the result of processing a single file through the safety pipeline.
type PipelineResult struct {
	// FileResult contains lint diagnostics and edits for the final content:
	// after fixing, the fixed content is linted once more, so diagnostics
	// describe what is left. RuleErrors also includes failures from earlier
	// fix passes.
	*FileResult

	// Path is the file path that was processed.
	Path string

	// OriginalInfo is the file state before processing.
	OriginalInfo *fsutil.FileInfo

	// Modified is true if fixing changed the file's bytes.
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

	// FixPasses is the number of fix passes that changed the content.
	FixPasses int

	// TotalEditsApplied is the total number of edits applied across all passes.
	TotalEditsApplied int

	// Exhausted is true if the fix loop hit MaxFixPasses while the fixed
	// content still had fixes to apply.
	Exhausted bool

	// Oscillated is true if the fix loop stopped because applying the
	// remaining fixes would only recreate content from an earlier pass: the
	// fixes undo each other, or change nothing.
	Oscillated bool

	// RemainingEdits is the number of edits still proposed for the final
	// content when the loop stopped early (Exhausted or Oscillated).
	RemainingEdits int
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

	// ReParseAfterFix is kept for compatibility. Fixed content is always
	// re-linted, and so re-parsed, after every fix pass, and a fix whose
	// result cannot be parsed is never written, whatever this is set to.
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
//  2. Lint the file and, in fix mode, apply fixes pass by pass, re-linting
//     the fixed content after each pass, until no fix remains, the fixes
//     start repeating earlier content, or MaxFixPasses is reached.
//  3. Generate diff (if dry-run mode).
//  4. Check for concurrent modifications.
//  5. Create backup (if enabled).
//  6. Write the modified content (see fsutil.WriteAtomic).
//
// A panic while processing the file is recovered and returned as an error
// wrapping ErrInternal.
func (p *Pipeline) ProcessFile(
	ctx context.Context,
	path string,
	cfg *config.Config,
	opts PipelineOptions,
) (*PipelineResult, error) {
	var (
		result *PipelineResult
		err    error
	)
	if panicErr := recoverPanic(ctx, path, func() {
		result, err = p.processFile(ctx, path, cfg, opts)
	}); panicErr != nil {
		return nil, panicErr
	}
	return result, err
}

func (p *Pipeline) processFile(
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

	// Step 2: Lint, and fix pass by pass.
	content, err := p.lintAndFix(ctx, path, originalContent, cfg, opts, result)
	if err != nil {
		return nil, err
	}
	if !result.Modified {
		return result, nil
	}
	result.ModifiedContent = content

	// Step 3: Handle dry-run mode.
	if opts.DryRun {
		result.Diff = fix.GenerateDiff(path, originalContent, content)
		return result, nil
	}

	// Step 4: Check for concurrent modifications before writing.
	modified, err := p.checkModified(ctx, info, opts.StrictRaceDetection)
	if err != nil {
		return nil, fmt.Errorf("check modified: %w", err)
	}
	if modified {
		result.Skipped = true
		result.SkipReason = "file modified during processing"
		return result, nil
	}

	// Step 5: Create backup if enabled.
	if opts.Backup.Enabled {
		created, err := fsutil.CreateBackup(ctx, path, opts.Backup)
		if err != nil {
			return nil, fmt.Errorf("create backup: %w", err)
		}
		result.BackupCreated = created
	}

	// Step 6: Write the modified content.
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
	var (
		result *PipelineResult
		err    error
	)
	if panicErr := recoverPanic(ctx, path, func() {
		result, err = p.processContent(ctx, path, originalContent, cfg, opts)
	}); panicErr != nil {
		return nil, panicErr
	}
	return result, err
}

func (p *Pipeline) processContent(
	ctx context.Context,
	path string,
	originalContent []byte,
	cfg *config.Config,
	opts PipelineOptions,
) (*PipelineResult, error) {
	result := &PipelineResult{
		Path: path,
	}

	content, err := p.lintAndFix(ctx, path, originalContent, cfg, opts, result)
	if err != nil {
		return nil, err
	}
	if !result.Modified {
		return result, nil
	}
	result.ModifiedContent = content

	// Generate diff for review.
	if opts.DryRun {
		result.Diff = fix.GenerateDiff(path, originalContent, content)
	}

	return result, nil
}

// lintAndFix lints original and, in fix mode, applies the accepted fixes
// pass by pass. After every pass the fixed content is linted again, so the
// final FileResult always describes the returned content. The loop stops
// when no fix remains, when applying the remaining fixes would recreate
// content seen before (Oscillated), or after MaxFixPasses passes
// (Exhausted). Rule errors from every pass are kept.
//
// If the fixed content cannot be linted (a fix produced content the parser
// rejects), the file is marked skipped and the original content is kept.
func (p *Pipeline) lintAndFix(
	ctx context.Context,
	path string,
	original []byte,
	cfg *config.Config,
	opts PipelineOptions,
	result *PipelineResult,
) ([]byte, error) {
	maxPasses := opts.MaxFixPasses
	if maxPasses <= 0 {
		maxPasses = DefaultMaxFixPasses
	}

	// Use the original file's line ending for every pass.
	lineEnding := fix.DetectLineEnding(original)
	ruleErrors := make(map[string]error)
	lintContent := func(content []byte) (*FileResult, error) {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("processing cancelled: %w", ctx.Err())
		default:
		}
		fileResult, err := p.Engine.lintFile(ctx, path, content, cfg, lineEnding)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrParseFailure, err)
		}
		for id, ruleErr := range fileResult.RuleErrors {
			if _, seen := ruleErrors[id]; !seen {
				ruleErrors[id] = ruleErr
			}
		}
		return fileResult, nil
	}

	content := original
	fileResult, err := lintContent(content)
	if err != nil {
		return nil, err
	}

	seen := map[[sha256.Size]byte]struct{}{sha256.Sum256(content): {}}
	for opts.Fix && len(fileResult.Edits) > 0 {
		if result.FixPasses >= maxPasses {
			result.Exhausted = true
			break
		}

		next := fix.ApplyEdits(content, fileResult.Edits)
		digest := sha256.Sum256(next)
		if _, repeated := seen[digest]; repeated {
			result.Oscillated = true
			break
		}
		seen[digest] = struct{}{}

		nextResult, err := lintContent(next)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			// Never write content that cannot be linted.
			result.Skipped = true
			result.SkipReason = fmt.Sprintf("re-parse failed: %v", err)
			break
		}

		content = next
		result.FixPasses++
		result.TotalEditsApplied += len(fileResult.Edits)
		fileResult = nextResult
	}

	if result.Exhausted || result.Oscillated {
		result.RemainingEdits = len(fileResult.Edits)
	}

	// Report rule failures from every pass, preferring the final one.
	for id, ruleErr := range ruleErrors {
		if _, ok := fileResult.RuleErrors[id]; !ok {
			fileResult.RuleErrors[id] = ruleErr
		}
	}

	result.FileResult = fileResult
	if result.Skipped {
		return original, nil
	}
	result.Modified = !bytes.Equal(content, original)
	return content, nil
}

// recoverPanic runs process and turns a panic into an error wrapping
// ErrInternal, writing the stack to the debug log.
func recoverPanic(ctx context.Context, path string, process func()) error {
	var panicErr error
	func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			logging.FromContext(ctx).Debug("panic while processing file",
				"path", path, "panic", recovered, "stack", string(debug.Stack()))
			panicErr = fmt.Errorf("%w: panic while processing %s: %v", ErrInternal, path, recovered)
		}()
		process()
	}()
	return panicErr
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
		return categorized(ErrFileNotFound, err)
	}

	// Check for permission errors.
	if errors.Is(err, fsutil.ErrPermissionDenied) || errors.Is(err, os.ErrPermission) {
		return categorized(ErrPermissionDenied, err)
	}

	return err
}

// categorized wraps err with a pipeline error category. When err's message
// already starts with the category, as fsutil errors do, the message is
// kept as is rather than repeating it.
func categorized(category, err error) error {
	if strings.HasPrefix(err.Error(), category.Error()+":") {
		return &categoryError{category: category, err: err}
	}
	return fmt.Errorf("%w: %w", category, err)
}

// categoryError matches its category and the wrapped error but reads as the
// wrapped error alone.
type categoryError struct {
	category error
	err      error
}

func (e *categoryError) Error() string { return e.err.Error() }

func (e *categoryError) Unwrap() []error { return []error{e.category, e.err} }

// IsPipelineError checks if an error is a known pipeline error type.
func IsPipelineError(err error) bool {
	return errors.Is(err, ErrFileNotFound) ||
		errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrParseFailure) ||
		errors.Is(err, ErrWriteFailure) ||
		errors.Is(err, ErrInternal)
}

// BackupConfigFromConfig creates an fsutil.BackupConfig from config.Config.
func BackupConfigFromConfig(cfg *config.Config) fsutil.BackupConfig {
	if cfg == nil {
		return fsutil.DefaultBackupConfig()
	}
	return fsutil.BackupConfig{
		Enabled: cfg.Backups.IsEnabled() && !cfg.NoBackups,
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
