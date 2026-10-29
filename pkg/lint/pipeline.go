package lint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/fsutil"
)

// DefaultMaxFixPasses is the number of fix passes allowed for fixes that
// reveal new findings, to prevent infinite loops. The default pass limit
// adds a pass for each rule that can fix (see PipelineOptions.MaxFixPasses).
// This should be sufficient for most files. If more passes are needed, there
// may be rules that create issues for each other.
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
	// was recovered so other files are still processed. The error is an
	// *InternalError.
	ErrInternal = errors.New("internal error")
)

// InternalError records a panic recovered while processing one file. It
// matches ErrInternal with errors.Is, and also the panic value when that is
// an error.
type InternalError struct {
	// Path is the file being processed.
	Path string

	// Value is the value the code panicked with.
	Value any

	// Stack is the stack trace captured when the panic was recovered.
	Stack []byte
}

// Error implements error.
func (e *InternalError) Error() string {
	return fmt.Sprintf("%s: panic while processing %s: %v", ErrInternal.Error(), e.Path, e.Value)
}

// Unwrap returns ErrInternal and, when the panic value is an error, that
// error.
func (e *InternalError) Unwrap() []error {
	if err, ok := e.Value.(error); ok {
		return []error{ErrInternal, err}
	}
	return []error{ErrInternal}
}

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

	// Diff is the unified diff of the fixes, from the content that was read
	// to the fixed content. It is computed in dry-run mode and, when
	// PipelineOptions.Diff is set, for a file that was written or content
	// fixed by ProcessContent. It is nil otherwise.
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

	// TotalFixesApplied is the total number of fixes applied across all
	// passes, one for each diagnostic whose fix was applied (see
	// FileResult.AcceptedFixes).
	TotalFixesApplied int

	// Exhausted is true if the fix loop hit its pass limit (see
	// PipelineOptions.MaxFixPasses) while the fixed content still had fixes
	// to apply.
	Exhausted bool

	// Oscillated is true if the fix loop stopped because applying the
	// remaining fixes would only recreate content from an earlier pass: the
	// fixes undo each other, or change nothing.
	Oscillated bool

	// RemainingEdits is the number of edits still proposed for the final
	// content when the loop stopped early (Exhausted or Oscillated),
	// including the edits of fixes deferred to a later pass.
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

	// Diff records in PipelineResult.Diff the diff of the fixes that are
	// written (or, for ProcessContent, applied in memory), from the content
	// that was read to the fixed content. Dry runs always record the diff
	// of the fixes they compute.
	Diff bool

	// Backup configures backup behavior.
	Backup fsutil.BackupConfig

	// StrictRaceDetection uses hash comparison for modification detection.
	// When false, only mod time and size are checked.
	StrictRaceDetection bool

	// ReParseAfterFix has no effect. Fixed content is always re-linted, and so
	// re-parsed, after every fix pass, and a fix whose result cannot be parsed
	// is never written.
	//
	// Deprecated: The pipeline always re-parses fixed content.
	ReParseAfterFix bool

	// MaxFixPasses limits the number of fix iterations to prevent infinite loops.
	// When conflicting edits are skipped, a subsequent pass may be able to fix them.
	// Set to 0 to allow DefaultMaxFixPasses passes plus one for each rule
	// that can fix: each pass applies the fixes of one rule to each
	// top-level block, so a block with findings of several rules takes a
	// pass per rule.
	MaxFixPasses int

	// WriteRoots limits where fixes are written: a file is fixed only if its
	// fully resolved path (every symbolic link along it followed, whether
	// the file itself or a directory above it) lies inside one of these
	// paths, directories or files (see WriteRoots). Otherwise the file is
	// linted but not fixed and is marked Skipped, so linting a tree never
	// changes a file outside it through a link. Empty means no restriction.
	// Hard links are not affected: they are the same file under another name.
	// A pipeline resolves the roots once and reuses them, for as long as it
	// lives, for every file that passes the same roots, so a link among them
	// keeps the target it first had. Do not change the slice while files are
	// processed with it.
	WriteRoots []string
}

// WriteRoots returns the paths that PipelineOptions.WriteRoots should hold
// for a run over paths: each path made absolute (relative paths start from
// workingDir, or the current directory when it is empty) with symbolic links
// resolved, so a link named on the command line allows its target.
func WriteRoots(workingDir string, paths []string) []string {
	if workingDir == "" {
		if wd, err := os.Getwd(); err == nil {
			workingDir = wd
		}
	}

	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		roots = append(roots, resolvePath(path))
	}
	return roots
}

// resolvePath returns path as an absolute path with symbolic links resolved,
// or as clean an approximation as possible when it cannot be resolved.
func resolvePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// resolveFile resolves every symbolic link in path and returns the absolute
// path of the file that path names. A missing or unreadable path fails with
// the fsutil error that reading it would give, so it is categorized alike.
func resolveFile(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("%w: %s: %w", fsutil.ErrNotFound, path, err)
	case errors.Is(err, fs.ErrPermission):
		return "", fmt.Errorf("%w: %s: %w", fsutil.ErrPermissionDenied, path, err)
	case err != nil:
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	target, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	return target, nil
}

// writeRootSet holds write roots resolved once, and finds the root of a
// file by looking up the file and each of its directories, so the cost of a
// file does not grow with the number of roots.
type writeRootSet struct {
	// passed is the slice the roots came in and given a copy of it, to
	// recognize the roots of a later file of the same run.
	passed []string
	given  []string
	roots  []writeRootEntry
	// first maps the key of each resolved root to its first position.
	first map[string]int
	// foldCase compares paths without regard to case, as filepath.Rel does
	// on Windows.
	foldCase bool
}

type writeRootEntry struct {
	path string
	dir  bool
}

func newWriteRootSet(roots []string, foldCase bool) *writeRootSet {
	set := &writeRootSet{
		passed:   roots,
		given:    slices.Clone(roots),
		roots:    make([]writeRootEntry, 0, len(roots)),
		first:    make(map[string]int, len(roots)),
		foldCase: foldCase,
	}
	for pos, root := range roots {
		resolved := resolvePath(root)
		info, err := os.Stat(resolved)
		set.roots = append(set.roots, writeRootEntry{path: resolved, dir: err == nil && info.IsDir()})
		// Key the root as filepath.Dir writes the directories of a file
		// inside it: a UNC share root such as \\host\share gains the
		// separator that Dir keeps.
		key := rootKey(filepath.Dir(filepath.Join(resolved, "x")), foldCase)
		if _, seen := set.first[key]; !seen {
			set.first[key] = pos
		}
	}
	return set
}

// rootKey returns path in the form that tells whether two clean paths name
// the same place, as filepath.Rel compares them: as written, or with each
// letter replaced by the smallest letter that strings.EqualFold matches it
// to when foldCase is set.
func rootKey(path string, foldCase bool) string {
	if !foldCase {
		return path
	}
	return strings.Map(func(r rune) rune {
		smallest := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			smallest = min(smallest, f)
		}
		return smallest
	}, path)
}

// find picks the directory that every operation on target is confined to,
// and target's path relative to it: the first write root that contains
// target, or target's own directory when that root is the file itself or
// no roots are set. It reports false when roots are set and none contains
// target. Target must be absolute and clean.
func (s *writeRootSet) find(target string) (string, string, bool) {
	dir, name := filepath.Dir(target), filepath.Base(target)
	if len(s.roots) == 0 {
		return dir, name, true
	}
	// A root contains target when it is target or one of its directories.
	best := -1
	for path := target; ; {
		if pos, ok := s.first[rootKey(path, s.foldCase)]; ok && (best < 0 || pos < best) {
			best = pos
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	if best < 0 {
		return dir, name, false
	}
	if root := s.roots[best]; root.dir {
		if rel, err := filepath.Rel(root.path, target); err == nil {
			return root.path, rel, true
		}
	}
	return dir, name, true
}

// sameSlice reports whether a and b are one slice: the same elements in
// the same memory.
func sameSlice(a, b []string) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// resolvedWriteRoots returns roots resolved, reusing the set of the
// previous file when its roots are the same slice, as the runner passes
// for every file of a run, or an equal one.
func (p *Pipeline) resolvedWriteRoots(roots []string) *writeRootSet {
	if set := p.writeRoots.Load(); set != nil && (sameSlice(set.passed, roots) || slices.Equal(set.given, roots)) {
		return set
	}
	set := newWriteRootSet(roots, runtime.GOOS == "windows")
	p.writeRoots.Store(set)
	return set
}

// DefaultPipelineOptions returns sensible defaults.
func DefaultPipelineOptions() PipelineOptions {
	return PipelineOptions{
		Fix:                 false,
		DryRun:              false,
		Backup:              fsutil.DefaultBackupConfig(),
		StrictRaceDetection: true,
	}
}

// Pipeline orchestrates the safe processing of a single file.
type Pipeline struct {
	// Engine is the lint engine used for parsing and rule execution.
	Engine *Engine

	// beforeWrite, when set, runs after every check and right before the
	// backup and write. Tests use it to change the file system at that point.
	beforeWrite func()

	// writeRoots holds the resolved write roots of the last file processed.
	// It makes a Pipeline unsafe to copy.
	writeRoots atomic.Pointer[writeRootSet]
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
//     start repeating earlier content, or the pass limit is reached (see
//     PipelineOptions.MaxFixPasses).
//  3. Generate diff (if dry-run mode).
//  4. Check for concurrent modifications.
//  5. Create backup (if enabled).
//  6. Write the modified content (see fsutil.WriteAtomic), and record its
//     diff when PipelineOptions.Diff is set.
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

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("processing cancelled: %w", ctx.Err())
	default:
	}

	// Step 1: Resolve the file and pin it inside the write root that
	// contains it. The pinned file holds the root directory and the file it
	// read: every later check, backup and write goes through that directory
	// handle and verifies that the name still refers to the same file, so
	// neither a moved link nor a replaced directory or file can redirect
	// the write.
	target, err := resolveFile(path)
	if err != nil {
		return nil, categorizeError(err)
	}
	rootDir, name, inside := p.resolvedWriteRoots(opts.WriteRoots).find(target)
	pinned, err := fsutil.Pin(rootDir, name)
	if err != nil {
		return nil, categorizeError(err)
	}
	defer pinned.Close()

	originalContent := pinned.Content()
	result.OriginalInfo = pinned.Info()

	// A fix may only write inside the linted paths. A file outside them is
	// linted without fixing (in dry runs too, so a preview never shows a
	// change that a real run would refuse).
	outside := opts.Fix && !inside
	lintOpts := opts
	if outside {
		lintOpts.Fix = false
	}

	// Step 3: Lint, and fix pass by pass.
	content, original, err := p.lintAndFix(ctx, path, originalContent, cfg, lintOpts, result)
	if err != nil {
		return nil, err
	}
	if outside {
		if len(original.Edits) > 0 {
			refuseWrite(result, original,
				fmt.Sprintf("not fixed: %s resolves to %s, outside the linted paths", path, target))
		}
		return result, nil
	}
	if !result.Modified {
		return result, nil
	}
	result.ModifiedContent = content

	// Step 2: Handle dry-run mode.
	if opts.DryRun {
		result.Diff = fix.GenerateDiff(path, originalContent, content)
		return result, nil
	}

	// Step 3: Make sure the path given still leads to the pinned file (a
	// link retargeted meanwhile makes it unclear which file was meant), that
	// the name still refers to the file that was read, and that nobody
	// changed it meanwhile.
	if current, err := resolveFile(path); err != nil || current != target {
		refuseWrite(result, original, fmt.Sprintf("not fixed: %s no longer leads to %s", path, target))
		return result, nil
	}
	if err := pinned.Check(opts.StrictRaceDetection); err != nil {
		if errors.Is(err, fsutil.ErrFileChanged) {
			refuseWrite(result, original, "not fixed: "+err.Error())
			return result, nil
		}
		return nil, fmt.Errorf("check modified: %w", err)
	}

	if p.beforeWrite != nil {
		p.beforeWrite()
	}

	// Step 4: Back up the content that was read, if enabled. The backup
	// checks again that the file is unchanged and stays in its directory.
	if opts.Backup.Enabled {
		created, err := pinned.Backup(opts.Backup.Mode)
		if err != nil {
			if errors.Is(err, fsutil.ErrFileChanged) {
				refuseWrite(result, original, "not fixed: "+err.Error())
				return result, nil
			}
			return nil, fmt.Errorf("create backup: %w", err)
		}
		result.BackupCreated = created
	}

	// Step 5: Write the fixed content. The write checks again, at commit
	// time, that the name still refers to the pinned file.
	if err := pinned.Write(content); err != nil {
		if errors.Is(err, fsutil.ErrFileChanged) {
			refuseWrite(result, original, "not fixed: "+err.Error())
			return result, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrWriteFailure, err)
	}
	result.Written = true
	if opts.Diff {
		result.Diff = fix.GenerateDiff(path, originalContent, content)
	}

	return result, nil
}

// refuseWrite marks a file as skipped without writing it. The result then
// describes the file as it is: the lint result of the original content,
// with no pending edits and no fix passes counted, and with the rule
// failures of every pass.
func refuseWrite(result *PipelineResult, original *FileResult, reason string) {
	if result.FileResult != nil && result.FileResult != original {
		for id, ruleErr := range result.RuleErrors {
			if _, ok := original.RuleErrors[id]; !ok {
				original.RuleErrors[id] = ruleErr
			}
		}
	}
	original.Edits = nil
	original.AcceptedFixes = 0
	original.SkippedEdits = nil
	original.EditConflicts = false
	original.DeferredEdits = nil

	result.FileResult = original
	result.Skipped = true
	result.SkipReason = reason
	result.Modified = false
	result.ModifiedContent = nil
	result.Diff = nil
	result.FixPasses = 0
	result.TotalEditsApplied = 0
	result.TotalFixesApplied = 0
	result.Exhausted = false
	result.Oscillated = false
	result.RemainingEdits = 0
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

	content, _, err := p.lintAndFix(ctx, path, originalContent, cfg, opts, result)
	if err != nil {
		return nil, err
	}
	if !result.Modified {
		return result, nil
	}
	result.ModifiedContent = content

	// Generate diff for review.
	if opts.DryRun || opts.Diff {
		result.Diff = fix.GenerateDiff(path, originalContent, content)
	}

	return result, nil
}

// lintAndFix lints original and, in fix mode, applies the accepted fixes
// pass by pass. After every pass the fixed content is linted again, so the
// final FileResult always describes the returned content, and the fixes the
// engine deferred are proposed again for that content. The loop stops
// when no fix remains, when applying the remaining fixes would recreate
// content seen before (Oscillated), or at the pass limit (Exhausted; see
// PipelineOptions.MaxFixPasses). Rule errors from every pass are kept. It
// also returns the lint result of the original content.
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
) ([]byte, *FileResult, error) {
	maxPasses := opts.MaxFixPasses
	if maxPasses <= 0 {
		maxPasses = p.defaultMaxFixPasses()
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
		return nil, nil, err
	}
	originalResult := fileResult

	var reparseErr error
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
				return nil, nil, err
			}
			reparseErr = err
			break
		}

		content = next
		result.FixPasses++
		result.TotalEditsApplied += len(fileResult.Edits)
		result.TotalFixesApplied += fileResult.AcceptedFixes
		fileResult = nextResult
	}

	if result.Exhausted || result.Oscillated {
		result.RemainingEdits = len(fileResult.Edits) + len(fileResult.DeferredEdits)
	}

	// Report rule failures from every pass, preferring the final one.
	for id, ruleErr := range ruleErrors {
		if _, ok := fileResult.RuleErrors[id]; !ok {
			fileResult.RuleErrors[id] = ruleErr
		}
	}
	result.FileResult = fileResult

	if reparseErr != nil {
		// Never write content that cannot be linted: keep the file as it is
		// and report on it.
		refuseWrite(result, originalResult, fmt.Sprintf("re-parse failed: %v", reparseErr))
		return original, originalResult, nil
	}
	result.Modified = !bytes.Equal(content, original)
	return content, originalResult, nil
}

// defaultMaxFixPasses returns the pass limit used when
// PipelineOptions.MaxFixPasses is not set: DefaultMaxFixPasses, plus a pass
// for each registered rule that can fix. In each pass the engine applies
// the fixes of one rule to each top-level block and defers the others (see
// FileResult.DeferredEdits), so a block with findings of several rules
// takes a pass per rule before any fix reveals a new finding.
func (p *Pipeline) defaultMaxFixPasses() int {
	passes := DefaultMaxFixPasses
	if p.Engine == nil || p.Engine.Registry == nil {
		return passes
	}
	for _, rule := range p.Engine.Registry.Rules() {
		if rule.CanFix() {
			passes++
		}
	}
	return passes
}

// recoverPanic runs process and turns a panic into an error wrapping
// ErrInternal, writing the stack to the debug log.
func recoverPanic(ctx context.Context, path string, process func()) error {
	var panicErr *InternalError
	func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			stack := debug.Stack()
			logging.FromContext(ctx).Debug("panic while processing file",
				"path", path, "panic", recovered, "stack", string(stack))
			panicErr = &InternalError{Path: path, Value: recovered, Stack: stack}
		}()
		process()
	}()
	if panicErr == nil {
		return nil
	}
	return panicErr
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
	}
}
