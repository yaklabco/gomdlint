package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/configloader"
	"github.com/yaklabco/gomdlint/internal/display"
	"github.com/yaklabco/gomdlint/internal/ignore"
	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/internal/ui/pretty"
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

func newLintCommand(info BuildInfo, registry *lint.Registry) *cobra.Command {
	var cfg config.Config
	flags := &lintFlags{toolVersion: info.Version}

	cmd := &cobra.Command{
		Use:   "lint [paths...]",
		Short: "Lint Markdown files",
		Long:  lintLongDescription,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd, args, &cfg, flags, registry)
		},
	}

	addLintFlags(cmd, &cfg, flags)
	registerRuleCompletions(cmd, registry)

	return cmd
}

const lintLongDescription = `Lint Markdown files for style and syntax issues.

By default, lints all .md and .markdown files in the current directory
and subdirectories. Specify paths to lint specific files or directories.

Examples:
  gomdlint lint                        # Lint current directory
  gomdlint lint docs/                  # Lint docs directory
  gomdlint lint README.md              # Lint single file
  gomdlint lint --fix                  # Lint and auto-fix issues
  gomdlint lint --fix --dry-run        # Show fixes without applying
  gomdlint lint --format diff          # Preview fixes as a diff
  gomdlint lint --fix --format diff    # Apply fixes and show them as a diff
  gomdlint lint --format json          # Output as JSON for CI
  gomdlint lint --strict               # Treat warnings as errors
  gomdlint lint --disable line-length  # Skip a rule, by name or ID
  gomdlint lint --disable whitespace   # Skip every rule a tag covers

--enable, --disable and --fix-rules take rule names and IDs, as
'gomdlint rules' lists them, and markdownlint's names for the rules. They
also take markdownlint's tags, such as whitespace or headings, which stand
for every rule they cover. Case does not matter.

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

// colorFlag returns the value of the persistent --color flag, or auto when
// the flag cannot be read.
func colorFlag(cmd *cobra.Command) string {
	colorMode, err := cmd.Flags().GetString("color")
	if err != nil {
		return colorModeAuto
	}
	return colorMode
}

func runLint(cmd *cobra.Command, args []string, cfg *config.Config, flags *lintFlags, registry *lint.Registry) error {
	logger := logging.FromContext(cmd.Context())

	if err := validateLintFlags(flags, cfg.Jobs); err != nil {
		return err
	}
	ruleLists, err := resolveRuleListFlags(flags, registry)
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
	if cmd.Flags().Changed("rule-format") {
		cfg.RuleFormat = config.RuleFormat(flags.ruleFormat)
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

	// Create the lint engine.
	engine := lint.NewEngine(parser, registry)

	// Create the safety pipeline.
	pipeline := lint.NewPipeline(engine)

	// Create the runner.
	lintRunner := runner.New(pipeline)

	// The output format comes from the flag, GOMDLINT_FORMAT or the
	// default, in that order; loading has validated it.
	format, err := reporter.ParseFormat(string(finalCfg.Format))
	if err != nil {
		return withExitCode(ExitConfigError, err)
	}

	runOpts := runnerOptions(args, workDir, loadResult)
	// The diff format shows the changes --fix makes: those it writes, or,
	// without --fix or with --dry-run, those it would write.
	runOpts.PreviewFixes = format == reporter.FormatDiff

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

	// Create reporter.
	colorMode := colorFlag(cmd)
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
		RuleFormat:   finalCfg.RuleFormat, // The flag, GOMDLINT_RULE_FORMAT or the default.
		SummaryOrder: config.SummaryOrder(flags.summaryOrder),
		WorkingDir:   workDir,
		ToolVersion:  flags.toolVersion,
		Registry:     registry,
		RuleHelp:     ruledocs.RuleHelp, // SARIF rule help: the pages that explain prints.
		BuiltInRule:  ruledocs.IsBuiltInRule,
	})
	if err != nil {
		return withExitCode(ExitInternalError, fmt.Errorf("create reporter: %w", err))
	}

	// Report results.
	shown, err := rep.Report(ctx, result)
	if err != nil {
		return withExitCode(ExitIOError, fmt.Errorf("report results: %w", err))
	}
	if err := writeExplainHint(cmd, colorMode, format, shown); err != nil {
		return err
	}
	for _, warning := range noticeWarnings(result, workDir) {
		logger.Warn(warning)
	}
	for _, warning := range skipWarnings(result) {
		logger.Warn(warning)
	}
	for _, ruleFailure := range ruleFailures(result, workDir) {
		logger.Error(ruleFailure)
	}

	// Write memory profile if requested.
	if flags.memprofile != "" {
		if err := writeMemProfile(flags.memprofile); err != nil {
			return withExitCode(ExitIOError, err)
		}
	}

	return resultError(result, flags.strict)
}

// writeExplainHint ends a report in format that lists shown findings with a
// line that names the command that explains them, for text output only.
func writeExplainHint(cmd *cobra.Command, colorMode string, format reporter.Format, shown int) error {
	if format != reporter.FormatText || shown == 0 {
		return nil
	}
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintln(out, pretty.NewStylesFor(colorMode, out).Dim.Render(explainHint)); err != nil {
		return withExitCode(ExitIOError, fmt.Errorf("report results: %w", err))
	}
	return nil
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

// noticeWarnings describes the notices of each file, such as a markdownlint
// comment that names no rule, at the file and line of the comment. They go
// to stderr, so every output format shows them. Notices change neither the
// findings nor the exit code.
func noticeWarnings(result *runner.Result, workDir string) []string {
	var warnings []string
	for idx := range result.Files {
		file := &result.Files[idx]
		if len(file.Notices) == 0 {
			continue
		}
		path := display.Path(file.Path, workDir)
		for _, notice := range file.Notices {
			warnings = append(warnings, fmt.Sprintf("%s:%d: %s", path, notice.Line, notice.Message))
		}
	}
	return warnings
}

// skipWarnings describes the files whose fixes the pipeline did not write,
// with its reasons. They go to stderr, so every output format shows them.
func skipWarnings(result *runner.Result) []string {
	var warnings []string
	for idx := range result.Files {
		file := &result.Files[idx]
		if file.Result != nil && file.Result.Skipped {
			warnings = append(warnings, fmt.Sprintf("%s: fixes not written: %s", file.Path, file.Result.SkipReason))
		}
	}
	return warnings
}

// ruleFailures describes each rule that failed on a file, with its cause,
// naming the rule by name, then ID. They go to stderr, so every output
// format shows them. The stack of a rule that panicked is in the debug log.
func ruleFailures(result *runner.Result, workDir string) []string {
	var failures []string
	for idx := range result.Files {
		file := &result.Files[idx]
		for _, ruleErr := range file.RuleErrors {
			failures = append(failures, fmt.Sprintf("rule %s failed on %s: %v",
				lint.RuleLabel(ruleErr.RuleID, ruleErr.RuleName), display.Path(file.Path, workDir), ruleErr.Err))
		}
	}
	return failures
}

// resultError turns the outcome of a completed run into the command error
// that carries its exit code.
func resultError(result *runner.Result, strict bool) error {
	code := ExitCodeFromResult(result, strict)
	switch code {
	case ExitSuccess:
		return nil
	case ExitIOError:
		return &ExitError{Code: code, Err: fmt.Errorf("%d %s could not be linted",
			result.Stats.FilesErrored, plural(result.Stats.FilesErrored, "file", "files"))}
	case ExitInternalError:
		return &ExitError{Code: code, Err: errors.New(internalErrorSummary(result.Stats))}
	default:
		return &ExitError{Code: code, Err: ErrLintIssuesFound}
	}
}

// internalErrorSummary describes the internal errors of a run: files whose
// processing failed because of a bug, and rules that failed on files.
func internalErrorSummary(stats runner.Stats) string {
	var parts []string
	if files := stats.FilesWithInternalErrors; files > 0 {
		parts = append(parts, fmt.Sprintf("%d %s could not be linted because of an internal error",
			files, plural(files, "file", "files")))
	}
	if stats.RuleErrors > 0 {
		parts = append(parts, fmt.Sprintf("%d rule %s on %d %s",
			stats.RuleErrors, plural(stats.RuleErrors, "failure", "failures"),
			stats.FilesWithRuleErrors, plural(stats.FilesWithRuleErrors, "file", "files")))
	}
	return strings.Join(parts, "; ")
}

// plural returns one for a count of one and otherwise many.
func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
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
	for _, pattern := range flags.ignore {
		if err := ignore.ValidatePattern(pattern); err != nil {
			return withExitCode(ExitInvalidUsage, fmt.Errorf("--ignore: %w", err))
		}
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

// listRulesHint ends the error about rule flag values that name no rule.
const listRulesHint = "Run 'gomdlint rules' to list rules."

// explainHint ends text output that lists findings.
const explainHint = "Run 'gomdlint explain <rule>' to see what a rule checks and how to configure it."

// resolveRuleListFlags resolves the values of --enable, --disable and
// --fix-rules through registry. A value that names no rule or tag is a
// usage error. The error describes every such value, one per line, with
// the rules and tags closest to it.
func resolveRuleListFlags(flags *lintFlags, registry *lint.Registry) (ruleLists, error) {
	var lists ruleLists
	var problems []string
	lists.enable, problems = resolveRuleList(registry, "--enable", flags.enable, problems)
	lists.disable, problems = resolveRuleList(registry, "--disable", flags.disable, problems)
	lists.fix, problems = resolveRuleList(registry, "--fix-rules", flags.fixRules, problems)
	if len(problems) > 0 {
		return ruleLists{}, &ExitError{
			Code: ExitInvalidUsage,
			Err:  errors.New(strings.Join(append(problems, listRulesHint), "\n")),
		}
	}
	return lists, nil
}

// resolveRuleList resolves the values of one rule list flag through
// registry: rule IDs, names and aliases, and markdownlint tags, which stand
// for every rule they cover. Matching ignores case. It returns the IDs of
// the rules, each once, in the order given, and appends a description of
// each value that names no rule or tag to problems.
func resolveRuleList(registry *lint.Registry, flag string, keys, problems []string) ([]string, []string) {
	if keys == nil {
		return nil, problems
	}

	ids := make([]string, 0, len(keys))
	listed := make(map[string]bool, len(keys))
	add := func(rule lint.Rule) {
		if !listed[rule.ID()] {
			listed[rule.ID()] = true
			ids = append(ids, rule.ID())
		}
	}
	for _, key := range keys {
		rule, isRule := registry.Lookup(key)
		tagged := registry.RulesByTag(key)
		if !isRule && len(tagged) == 0 {
			problems = append(problems, flag+": "+lint.UnknownRuleText(key, registry.Closest(key)))
			continue
		}
		if isRule {
			add(rule)
		}
		for _, rule := range tagged {
			add(rule)
		}
	}
	return ids, problems
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
	cmd.Flags().StringVar(&flags.format, "format", "text",
		"output format: text, table, json, sarif, diff (the changes --fix makes, previewed without --fix), summary")
	cmd.Flags().IntVar(&cfg.Jobs, "jobs", 0, "number of parallel workers (0 = auto)")
	cmd.Flags().StringSliceVar(&flags.ignore, "ignore", nil,
		"glob patterns to ignore, relative to the current directory (replace config ignore patterns)")
	cmd.Flags().BoolVar(&flags.noGitignore, "no-gitignore", false,
		"lint files that .gitignore files ignore")
	cmd.Flags().BoolVar(&flags.noDefaultIgnores, "no-default-ignores", false,
		"lint files in node_modules and vendor directories")
	cmd.Flags().StringSliceVar(&flags.enable, "enable", nil,
		"rules to enable, by name, ID or tag (overrides config files)")
	cmd.Flags().StringSliceVar(&flags.disable, "disable", nil,
		"rules to disable, by name, ID or tag (overrides config files and --enable)")
	cmd.Flags().StringSliceVar(&flags.fixRules, "fix-rules", nil, "limit auto-fix to these rules, by name, ID or tag")
	cmd.Flags().BoolVar(&cfg.NoBackups, "no-backups", false, "disable backup creation when fixing")
	cmd.Flags().StringVar(&flags.flavor, "flavor", string(config.FlavorGFM), "Markdown flavor: commonmark, gfm")
	cmd.Flags().BoolVar(&flags.strict, "strict", false, "treat warnings as errors for exit code")
	cmd.Flags().BoolVar(&flags.noContext, "no-context", false, "hide source line context in output")
	cmd.Flags().BoolVar(&flags.compact, "compact", false, "use compact output format")
	cmd.Flags().BoolVar(&flags.perFile, "per-file", false, "output separate report for each file (table format)")
	cmd.Flags().StringVar(&flags.ruleFormat, "rule-format", "name",
		"how text, table and summary output name rules: name, id, or combined")
	cmd.Flags().StringVar(&flags.summaryOrder, "summary-order", "rules",
		"order of tables in summary output: rules, files")

	// Profiling flags.
	cmd.Flags().StringVar(&flags.cpuprofile, "cpuprofile", "", "write CPU profile to file")
	cmd.Flags().StringVar(&flags.memprofile, "memprofile", "", "write memory profile to file")
	cmd.Flags().StringVar(&flags.trace, "trace", "", "write execution trace to file")
}
