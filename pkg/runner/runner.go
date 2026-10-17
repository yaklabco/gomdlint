package runner

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"sync"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// Runner orchestrates multi-file linting using a lint.Pipeline.
type Runner struct {
	// Pipeline handles per-file processing with safety guarantees.
	Pipeline *lint.Pipeline
}

// New creates a new Runner with the given pipeline.
func New(pipeline *lint.Pipeline) *Runner {
	return &Runner{Pipeline: pipeline}
}

// Run discovers files under opts.Paths and processes them concurrently.
// It returns a deterministic collection of FileOutcome values and aggregate stats.
//
// The runner:
//   - Discovers files matching the options criteria
//   - Processes files concurrently using a worker pool
//   - Processes the paths of one file, such as hard links, in turn
//   - Aggregates results into a single Result with statistics
//   - Respects context cancellation
func (r *Runner) Run(ctx context.Context, opts Options) (*Result, error) {
	// Discover files.
	found, err := discover(ctx, opts)
	if err != nil {
		return nil, err
	}
	files, groups := found.files, found.groups

	result := &Result{
		Files: make([]FileOutcome, 0, len(files)+len(found.unreadable)),
		Stats: newStats(),
	}
	result.Stats.FilesDiscovered = len(files)

	if len(files) == 0 {
		for _, outcome := range found.unreadable {
			result.accumulate(outcome)
		}
		return result, nil
	}

	// Determine job count.
	jobs := opts.Jobs
	if jobs <= 0 {
		jobs = runtime.NumCPU()
	}
	// Don't use more workers than there is work.
	if jobs > len(groups) {
		jobs = len(groups)
	}

	// Get pipeline options from config. Fixes only follow a symbolic link to
	// a file inside the paths being linted.
	pipelineOpts := lint.PipelineOptionsFromConfig(opts.Config)
	pipelineOpts.WriteRoots = lint.WriteRoots(opts.WorkingDir, opts.effectivePaths())

	// Create channels. Each unit of work is the paths naming one file.
	workCh := make(chan []string)
	outCh := make(chan FileOutcome)

	var wg sync.WaitGroup

	// Start workers.
	for range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx, workCh, outCh, opts.Config, pipelineOpts, opts.PreviewFixes)
		}()
	}

	// Feed work in a separate goroutine.
	go func() {
		defer close(workCh)
		for _, group := range groups {
			select {
			case <-ctx.Done():
				return
			case workCh <- group:
			}
		}
	}()

	// Close outCh when all workers are done.
	go func() {
		wg.Wait()
		close(outCh)
	}()

	// Collect results.
	// Use a map to maintain order since workers may complete out of order.
	outcomes := make(map[string]FileOutcome, len(files))

	for outcome := range outCh {
		outcomes[outcome.Path] = outcome
	}

	// Directories that could not be read fail the run like unreadable files.
	for _, outcome := range found.unreadable {
		outcomes[outcome.Path] = outcome
	}

	// Build result in deterministic order.
	paths := make([]string, 0, len(outcomes))
	for path := range outcomes {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		result.accumulate(outcomes[path])
	}

	// Check for context error.
	if ctx.Err() != nil {
		return result, fmt.Errorf("run cancelled: %w", ctx.Err())
	}

	return result, nil
}

// worker processes groups of paths from workCh, the paths of each group in
// turn, and sends outcomes to outCh.
func (r *Runner) worker(
	ctx context.Context,
	workCh <-chan []string,
	outCh chan<- FileOutcome,
	cfg *config.Config,
	opts lint.PipelineOptions,
	preview bool,
) {
	for group := range workCh {
		for _, path := range group {
			select {
			case <-ctx.Done():
				return
			default:
			}

			outcome := r.processFile(ctx, path, cfg, opts, preview)

			select {
			case <-ctx.Done():
				return
			case outCh <- outcome:
			}
		}
	}
}

// processFile lints one file. A panic while processing it fails that file
// instead of crashing the run, which would exit with Go's status 2.
func (r *Runner) processFile(
	ctx context.Context,
	path string,
	cfg *config.Config,
	opts lint.PipelineOptions,
	preview bool,
) FileOutcome {
	outcome := FileOutcome{Path: path}

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				outcome = FileOutcome{Path: path, Error: fmt.Errorf("internal error: %v", recovered)}
			}
		}()

		pr, err := r.Pipeline.ProcessFile(ctx, path, cfg, opts)
		if err == nil && preview && pr.FileResult != nil && pr.FixableCount() > 0 {
			pr.Diff, err = r.previewDiff(ctx, path, cfg, opts)
		}
		if err != nil {
			outcome.Error = err
			return
		}
		outcome.Result = pr
		outcome.Exhausted = pr.Exhausted
		outcome.RemainingEdits = pr.RemainingEdits
		outcome.FixPasses = pr.FixPasses
		outcome.Oscillated = pr.Oscillated
		if pr.FileResult != nil {
			outcome.RuleErrors = pr.RuleErrorList()
			outcome.SkippedEdits = pr.SkippedEdits
		}
	}()

	return outcome
}

// previewDiff returns the diff that fixing path would produce, computed in
// dry-run mode so nothing is written.
func (r *Runner) previewDiff(
	ctx context.Context,
	path string,
	cfg *config.Config,
	opts lint.PipelineOptions,
) (*fix.Diff, error) {
	fixCfg := config.NewConfig()
	if cfg != nil {
		fixCfg = new(config.Config)
		*fixCfg = *cfg
	}
	fixCfg.Fix = true
	opts.Fix = true
	opts.DryRun = true

	preview, err := r.Pipeline.ProcessFile(ctx, path, fixCfg, opts)
	if err != nil {
		return nil, fmt.Errorf("preview fixes: %w", err)
	}
	return preview.Diff, nil
}
