package runner

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
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
//   - Aggregates results into a single Result with statistics
//   - Respects context cancellation
func (r *Runner) Run(ctx context.Context, opts Options) (*Result, error) {
	// Discover files.
	files, err := Discover(ctx, opts)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Files: make([]FileOutcome, 0, len(files)),
		Stats: newStats(),
	}
	result.Stats.FilesDiscovered = len(files)

	if len(files) == 0 {
		return result, nil
	}

	// Determine job count.
	jobs := opts.Jobs
	if jobs <= 0 {
		jobs = runtime.NumCPU()
	}
	// Don't use more workers than files.
	if jobs > len(files) {
		jobs = len(files)
	}

	// Get pipeline options from config.
	pipelineOpts := lint.PipelineOptionsFromConfig(opts.Config)

	// Create channels.
	workCh := make(chan string)
	outCh := make(chan FileOutcome)

	var wg sync.WaitGroup

	// Start workers.
	for range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(ctx, workCh, outCh, opts.Config, pipelineOpts)
		}()
	}

	// Feed work in a separate goroutine.
	go func() {
		defer close(workCh)
		for _, path := range files {
			select {
			case <-ctx.Done():
				return
			case workCh <- path:
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

	// Build result in deterministic order.
	for _, path := range files {
		if outcome, ok := outcomes[path]; ok {
			result.accumulate(outcome)
		}
	}

	// Check for context error.
	if ctx.Err() != nil {
		return result, fmt.Errorf("run cancelled: %w", ctx.Err())
	}

	return result, nil
}

// worker processes files from workCh and sends outcomes to outCh.
func (r *Runner) worker(
	ctx context.Context,
	workCh <-chan string,
	outCh chan<- FileOutcome,
	cfg *config.Config,
	opts lint.PipelineOptions,
) {
	for path := range workCh {
		select {
		case <-ctx.Done():
			return
		default:
		}

		outcome := FileOutcome{Path: path}

		pr, err := r.Pipeline.ProcessFile(ctx, path, cfg, opts)
		if err != nil {
			outcome.Error = err
		} else {
			outcome.Result = pr
		}

		select {
		case <-ctx.Done():
			return
		case outCh <- outcome:
		}
	}
}
