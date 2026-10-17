package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/configloader"
	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	_ "github.com/yaklabco/gomdlint/pkg/lint/rules" // Register built-in rules
	goldmarkparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

type lintFlags struct {
	format           string
	flavor           string
	ignore           []string
	noGitignore      bool
	noDefaultIgnores bool
	enable           []string
	disable          []string
	fixRules         []string
	strict           bool
	noContext        bool
	compact          bool
	perFile          bool
	ruleFormat       string
	summaryOrder     string
	cpuprofile       string
	memprofile       string
	trace            string

	// toolVersion is not a flag: it is the build version that machine-readable
	// reports record.
	toolVersion string
}

func newLintCommand(info BuildInfo) *cobra.Command {
	var cfg config.Config
	flags := &lintFlags{toolVersion: info.Version}

	cmd := &cobra.Command{
		Use:   "lint [paths...]",
		Short: "Lint Markdown files",
		Long:  lintLongDescription,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd, args, &cfg, flags)
		},
	}

	addLintFlags(cmd, &cfg, flags)

	return cmd
}

const lintLongDescription = `Lint Markdown files for style and syntax issues.

By default, lints all .md and .markdown files in the current directory
and subdirectories. Specify paths to lint specific files or directories.

Examples:
  mdlint lint                    # Lint current directory
  mdlint lint docs/              # Lint docs directory
  mdlint lint README.md          # Lint single file
  mdlint lint --fix              # Lint and auto-fix issues
  mdlint lint --fix --dry-run    # Show fixes without applying
  mdlint lint --format json      # Output as JSON for CI
  mdlint lint --strict           # Treat warnings as errors

` + exitCodeHelp

// profileCleanup holds cleanup functions for profiling.
type profileCleanup struct {
	funcs []func()
}

func (p *profileCleanup) add(fn func()) {
	p.funcs = append(p.funcs, fn)
}

func (p *profileCleanup) run() {
	for i := len(p.funcs) - 1; i >= 0; i-- {
		p.funcs[i]()
	}
}

func setupProfiling(flags *lintFlags) (*profileCleanup, error) {
	cleanup := &profileCleanup{}

	// Start CPU profiling if requested.
	if flags.cpuprofile != "" {
		cpuFile, err := os.Create(flags.cpuprofile)
		if err != nil {
			return nil, fmt.Errorf("create CPU profile: %w", err)
		}
		cleanup.add(func() { _ = cpuFile.Close() })
		if err := pprof.StartCPUProfile(cpuFile); err != nil {
			cleanup.run()
			return nil, fmt.Errorf("start CPU profile: %w", err)
		}
		cleanup.add(pprof.StopCPUProfile)
	}

	// Start execution tracing if requested.
	if flags.trace != "" {
		traceFile, err := os.Create(flags.trace)
		if err != nil {
			cleanup.run()
			return nil, fmt.Errorf("create trace file: %w", err)
		}
		cleanup.add(func() { _ = traceFile.Close() })
		if err := trace.Start(traceFile); err != nil {
			cleanup.run()
			return nil, fmt.Errorf("start trace: %w", err)
		}
		cleanup.add(trace.Stop)
	}

	return cleanup, nil
}

func runLint(cmd *cobra.Command, args []string, cfg *config.Config, flags *lintFlags) error {
	logger := logging.FromContext(cmd.Context())

	if err := validateLintFlags(flags, cfg.Jobs); err != nil {
		return err
	}
	ruleLists, err := resolveRuleListFlags(flags)
	if err != nil {
		return err
	}

	// Setup profiling.
	profCleanup, err := setupProfiling(flags)
	if err != nil {
		return withExitCode(ExitIOError, err)
	}
	defer profCleanup.run()

	// Map string flags to typed config values.
	// Only set values that were explicitly provided via CLI flags.
	if cmd.Flags().Changed("format") {
		cfg.Format = config.OutputFormat(flags.format)
	}
	if cmd.Flags().Changed("flavor") {
		cfg.Flavor = config.Flavor(flags.flavor)
	}
	cfg.Ignore = flags.ignore
	cfg.EnableRules = ruleLists.enable
	cfg.DisableRules = ruleLists.disable
	cfg.FixRules = ruleLists.fix

	// Load and merge configuration. Cobra always provides a context.
	ctx := cmd.Context()

	// Get the explicit config path from the root command's persistent flag.
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return withExitCode(ExitInternalError, fmt.Errorf("get config flag: %w", err))
	}

	// Get working directory for config discovery.
	workDir, err := os.Getwd()
	if err != nil {
		return withExitCode(ExitInternalError, fmt.Errorf("get working directory: %w", err))
	}

	// Build load options.
	loadOpts := configloader.LoadOptions{
		WorkingDir:   workDir,
		ExplicitPath: configPath,
		CLIConfig:    cfg,
		Stdin:        cmd.InOrStdin(),
		Stderr:       cmd.ErrOrStderr(),
	}

	loadResult, err := configloader.Load(ctx, loadOpts)
	if err != nil {
		return failure(ctx, ExitConfigError, fmt.Errorf("load configuration: %w", err))
	}

	finalCfg := loadResult.Config
	applyExplicitFlags(cmd, flags, cfg, finalCfg)

	// Log notices and warnings from config loading.
	for _, notice := range loadResult.Notices {
		logger.Info(notice)
	}
	for _, warning := range loadResult.Warnings {
		logger.Warn(warning)
	}

	// Log loaded configuration files.
	if len(loadResult.LoadedFrom) > 0 {
		logger.Debug("loaded configuration from", "files", loadResult.LoadedFrom)
	}

	logger.Debug("configuration loaded",
		"flavor", finalCfg.Flavor,
		"fix", finalCfg.Fix,
		"dry_run", finalCfg.DryRun,
		"jobs", finalCfg.Jobs,
	)

	// Create the parser based on flavor.
	parser := goldmarkparser.New(string(finalCfg.Flavor))

	// Use the default registry which has all built-in rules registered.
	registry := lint.DefaultRegistry

	// Create the lint engine.
	engine := lint.NewEngine(parser, registry)

	// Create the safety pipeline.
	pipeline := lint.NewPipeline(engine)

	// Create the runner.
	lintRunner := runner.New(pipeline)

	runOpts := runnerOptions(args, workDir, loadResult)

	logger.Debug("starting lint run",
		"paths", runOpts.Paths,
		"working_dir", runOpts.WorkingDir,
		"jobs", runOpts.Jobs,
	)

	// Run linting.
	result, err := lintRunner.Run(ctx, runOpts)
	if err != nil {
		return failure(ctx, ExitIOError, err)
	}

	// Get color mode from persistent flag.
	colorMode, err := cmd.Flags().GetString("color")
	if err != nil {
		colorMode = "auto" // Default to auto if flag retrieval fails
	}

	// The output format comes from the flag, GOMDLINT_FORMAT or the
	// default, in that order; loading has validated it.
	format, err := reporter.ParseFormat(string(finalCfg.Format))
	if err != nil {
		return withExitCode(ExitConfigError, err)
	}

	// Create reporter.
	rep, err := reporter.New(reporter.Options{
		Writer:       cmd.OutOrStdout(),
		ErrorWriter:  cmd.ErrOrStderr(),
		Format:       format,
		Color:        colorMode,
		ShowContext:  !flags.noContext,
		ShowSummary:  true,
		GroupByFile:  true,
		Compact:      flags.compact,
		PerFile:      flags.perFile,
		RuleFormat:   config.RuleFormat(flags.ruleFormat),
		SummaryOrder: config.SummaryOrder(flags.summaryOrder),
		WorkingDir:   workDir,
		ToolVersion:  flags.toolVersion,
		Registry:     registry,
	})
	if err != nil {
		return withExitCode(ExitInternalError, fmt.Errorf("create reporter: %w", err))
	}

	// Report results.
	if _, err := rep.Report(ctx, result); err != nil {
		return withExitCode(ExitIOError, fmt.Errorf("report results: %w", err))
	}

	// Write memory profile if requested.
	if flags.memprofile != "" {
		if err := writeMemProfile(flags.memprofile); err != nil {
			return withExitCode(ExitIOError, err)
		}
	}

	return resultError(result, flags.strict)
}

// runnerOptions builds the file discovery and processing options of a run.
func runnerOptions(args []string, workDir string, loadResult *configloader.LoadResult) runner.Options {
	cfg := loadResult.Config
	opts := runner.Options{
		Paths:         args,
		WorkingDir:    workDir,
		Extensions:    runner.DefaultExtensions(),
		ExcludeGlobs:  cfg.Ignore,
		IgnoreBaseDir: loadResult.IgnoreBaseDir,
		Gitignore:     cfg.GitignoreEnabled(),
		Jobs:          cfg.Jobs,
		Config:        cfg,
	}
	if cfg.DefaultIgnoresEnabled() {
		opts.SkipDirs = runner.DefaultSkipDirs()
	}
	return opts
}

// applyExplicitFlags gives flags the user set explicitly the last word over
// the loaded configuration. The configuration merge only sees non-zero
// values, so without this --fix=false or --jobs 0 could not override the
// environment.
func applyExplicitFlags(cmd *cobra.Command, flags *lintFlags, flagCfg, finalCfg *config.Config) {
	changed := cmd.Flags().Changed
	if changed("fix") {
		finalCfg.Fix = flagCfg.Fix
	}
	if changed("dry-run") {
		finalCfg.DryRun = flagCfg.DryRun
	}
	if changed("no-backups") {
		finalCfg.NoBackups = flagCfg.NoBackups
	}
	if changed("jobs") {
		finalCfg.Jobs = flagCfg.Jobs
	}
	if changed("no-gitignore") {
		finalCfg.Gitignore = config.Bool(!flags.noGitignore)
	}
	if changed("no-default-ignores") {
		finalCfg.DefaultIgnores = config.Bool(!flags.noDefaultIgnores)
	}
}

// resultError turns the outcome of a completed run into the command error
// that carries its exit code.
func resultError(result *runner.Result, strict bool) error {
	code := ExitCodeFromResult(result, strict)
	switch code {
	case ExitSuccess:
		return nil
	case ExitIOError:
		noun := "files"
		if result.Stats.FilesErrored == 1 {
			noun = "file"
		}
		return &ExitError{Code: code, Err: fmt.Errorf("%d %s could not be linted", result.Stats.FilesErrored, noun)}
	default:
		return &ExitError{Code: code, Err: ErrLintIssuesFound}
	}
}

// failure classifies the error of a stage that also fails when the run is
// interrupted. An interrupted run is an internal error, not a failure of the
// stage's inputs.
func failure(ctx context.Context, code int, err error) error {
	if ctx.Err() != nil {
		return withExitCode(ExitInternalError, err)
	}
	return withExitCode(code, err)
}

// validateLintFlags rejects invalid flag values before any work starts.
func validateLintFlags(flags *lintFlags, jobs int) error {
	if _, err := reporter.ParseFormat(flags.format); err != nil {
		return withExitCode(ExitInvalidUsage, err)
	}
	if !configloader.IsValidFlavor(config.Flavor(flags.flavor)) {
		return usageError("invalid flavor %q; valid flavors: commonmark, gfm", flags.flavor)
	}
	if !config.RuleFormat(flags.ruleFormat).IsValid() {
		return usageError("invalid rule format %q; valid formats: name, id, combined", flags.ruleFormat)
	}
	if !config.SummaryOrder(flags.summaryOrder).IsValid() {
		return usageError("invalid summary order %q; valid orders: rules, files", flags.summaryOrder)
	}
	if jobs < 0 {
		return usageError("invalid jobs value %d; use 0 for one worker per CPU", jobs)
	}
	return nil
}

// ruleLists holds the canonical rule IDs from --enable, --disable and
// --fix-rules. A nil list means the flag was not given.
type ruleLists struct {
	enable  []string
	disable []string
	fix     []string
}

// resolveRuleListFlags resolves the rule IDs, names and aliases given to
// --enable, --disable and --fix-rules, rejecting rules that do not exist.
func resolveRuleListFlags(flags *lintFlags) (ruleLists, error) {
	var lists ruleLists
	var err error
	if lists.enable, err = canonicalRuleIDs("--enable", flags.enable); err != nil {
		return lists, err
	}
	if lists.disable, err = canonicalRuleIDs("--disable", flags.disable); err != nil {
		return lists, err
	}
	if lists.fix, err = canonicalRuleIDs("--fix-rules", flags.fixRules); err != nil {
		return lists, err
	}
	return lists, nil
}

// canonicalRuleIDs resolves keys given to flag through the rule registry.
func canonicalRuleIDs(flag string, keys []string) ([]string, error) {
	if keys == nil {
		return nil, nil
	}

	ids := make([]string, 0, len(keys))
	var unknown []string
	for _, key := range keys {
		id, ok := lint.LookupRuleID(lint.DefaultRegistry, key)
		if !ok {
			unknown = append(unknown, strconv.Quote(key))
			continue
		}
		ids = append(ids, id)
	}

	switch len(unknown) {
	case 0:
		return ids, nil
	case 1:
		return nil, usageError("%s: unknown rule %s; run 'gomdlint rules' to list rules", flag, unknown[0])
	default:
		return nil, usageError("%s: unknown rules %s; run 'gomdlint rules' to list rules", flag, strings.Join(unknown, ", "))
	}
}

// writeMemProfile writes a heap profile to path.
func writeMemProfile(path string) error {
	memFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create memory profile: %w", err)
	}
	defer memFile.Close()
	runtime.GC() //nolint:revive // Intentional GC before heap profile for accurate stats.
	if err := pprof.WriteHeapProfile(memFile); err != nil {
		return fmt.Errorf("write memory profile: %w", err)
	}
	return nil
}

func addLintFlags(cmd *cobra.Command, cfg *config.Config, flags *lintFlags) {
	cmd.Flags().BoolVar(&cfg.Fix, "fix", false, "automatically fix issues")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "show fixes without applying them")
	cmd.Flags().StringVar(&flags.format, "format", "text", "output format: text, table, json, sarif, diff, summary")
	cmd.Flags().IntVar(&cfg.Jobs, "jobs", 0, "number of parallel workers (0 = auto)")
	cmd.Flags().StringSliceVar(&flags.ignore, "ignore", nil,
		"glob patterns to ignore, relative to the current directory (replace config ignore patterns)")
	cmd.Flags().BoolVar(&flags.noGitignore, "no-gitignore", false,
		"lint files that .gitignore files ignore")
	cmd.Flags().BoolVar(&flags.noDefaultIgnores, "no-default-ignores", false,
		"lint files in node_modules and vendor directories")
	cmd.Flags().StringSliceVar(&flags.enable, "enable", nil,
		"rules to enable, by ID or name (overrides config files)")
	cmd.Flags().StringSliceVar(&flags.disable, "disable", nil,
		"rules to disable, by ID or name (overrides config files and --enable)")
	cmd.Flags().StringSliceVar(&flags.fixRules, "fix-rules", nil, "limit auto-fix to these rules, by ID or name")
	cmd.Flags().BoolVar(&cfg.NoBackups, "no-backups", false, "disable backup creation when fixing")
	cmd.Flags().StringVar(&flags.flavor, "flavor", string(config.FlavorGFM), "Markdown flavor: commonmark, gfm")
	cmd.Flags().BoolVar(&flags.strict, "strict", false, "treat warnings as errors for exit code")
	cmd.Flags().BoolVar(&flags.noContext, "no-context", false, "hide source line context in output")
	cmd.Flags().BoolVar(&flags.compact, "compact", false, "use compact output format")
	cmd.Flags().BoolVar(&flags.perFile, "per-file", false, "output separate report for each file (table format)")
	cmd.Flags().StringVar(&flags.ruleFormat, "rule-format", "name",
		"rule identifier format in output: name, id, or combined")
	cmd.Flags().StringVar(&flags.summaryOrder, "summary-order", "rules",
		"order of tables in summary output: rules, files")

	// Profiling flags.
	cmd.Flags().StringVar(&flags.cpuprofile, "cpuprofile", "", "write CPU profile to file")
	cmd.Flags().StringVar(&flags.memprofile, "memprofile", "", "write memory profile to file")
	cmd.Flags().StringVar(&flags.trace, "trace", "", "write execution trace to file")
}
