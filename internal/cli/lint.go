package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jamesainslie/gomdlint/internal/configloader"
	"github.com/jamesainslie/gomdlint/internal/logging"
	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	_ "github.com/jamesainslie/gomdlint/pkg/lint/rules" // Register built-in rules
	goldmarkparser "github.com/jamesainslie/gomdlint/pkg/parser/goldmark"
	"github.com/jamesainslie/gomdlint/pkg/reporter"
	"github.com/jamesainslie/gomdlint/pkg/runner"
)

// ErrLintIssuesFound is returned when lint issues are found.
var ErrLintIssuesFound = errors.New("lint issues found")

type lintFlags struct {
	format       string
	flavor       string
	ignore       []string
	enable       []string
	disable      []string
	fixRules     []string
	strict       bool
	noContext    bool
	compact      bool
	perFile      bool
	ruleFormat   string
	summaryOrder string
	cpuprofile   string
	memprofile   string
	trace        string
}

func newLintCommand() *cobra.Command {
	var cfg config.Config
	flags := &lintFlags{}

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
  mdlint lint --strict           # Treat warnings as errors`

func runLint(cmd *cobra.Command, args []string, cfg *config.Config, flags *lintFlags) error {
	logger := logging.Default()

	// Map string flags to typed config values.
	// Only set values that were explicitly provided via CLI flags.
	cfg.Format = config.OutputFormat(flags.format)
	if cmd.Flags().Changed("flavor") {
		cfg.Flavor = config.Flavor(flags.flavor)
	}
	cfg.Ignore = flags.ignore
	cfg.EnableRules = flags.enable
	cfg.DisableRules = flags.disable
	cfg.FixRules = flags.fixRules

	// Load and merge configuration.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Get the explicit config path from the root command's persistent flag.
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("get config flag: %w", err)
	}

	// Get working directory for config discovery.
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Build load options.
	loadOpts := configloader.LoadOptions{
		WorkingDir:   workDir,
		ExplicitPath: configPath,
		CLIConfig:    cfg,
	}

	loadResult, err := configloader.Load(ctx, loadOpts)
	if err != nil {
		return errors.Join(errors.New("failed to load configuration"), err)
	}

	finalCfg := loadResult.Config

	// Log warnings from config loading.
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

	// Build runner options.
	runOpts := runner.Options{
		Paths:        args,
		WorkingDir:   workDir,
		Extensions:   runner.DefaultExtensions(),
		ExcludeGlobs: finalCfg.Ignore,
		Jobs:         finalCfg.Jobs,
		Config:       finalCfg,
	}

	logger.Debug("starting lint run",
		"paths", runOpts.Paths,
		"working_dir", runOpts.WorkingDir,
		"jobs", runOpts.Jobs,
	)

	// Run linting.
	result, err := lintRunner.Run(ctx, runOpts)
	if err != nil {
		return errors.Join(errors.New("lint run failed"), err)
	}

	// Get color mode from persistent flag.
	colorMode, err := cmd.Flags().GetString("color")
	if err != nil {
		colorMode = "auto" // Default to auto if flag retrieval fails
	}

	// Parse output format.
	format, err := reporter.ParseFormat(flags.format)
	if err != nil {
		return fmt.Errorf("invalid format: %w", err)
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
	})
	if err != nil {
		return fmt.Errorf("create reporter: %w", err)
	}

	// Report results.
	if _, err := rep.Report(ctx, result); err != nil {
		logger.Error("report failed", "error", err)
		return fmt.Errorf("report results: %w", err)
	}

	// Determine exit code based on result.
	exitCode := ExitCodeFromResult(result, flags.strict)
	if exitCode != ExitSuccess {
		return ErrLintIssuesFound
	}

	return nil
}

func addLintFlags(cmd *cobra.Command, cfg *config.Config, flags *lintFlags) {
	cmd.Flags().BoolVar(&cfg.Fix, "fix", false, "automatically fix issues")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "show fixes without applying them")
	cmd.Flags().StringVar(&flags.format, "format", "text", "output format: text, table, json, sarif, diff, summary")
	cmd.Flags().IntVar(&cfg.Jobs, "jobs", 0, "number of parallel workers (0 = auto)")
	cmd.Flags().StringSliceVar(&flags.ignore, "ignore", nil, "glob patterns to ignore")
	cmd.Flags().StringSliceVar(&flags.enable, "enable", nil, "rule IDs to enable")
	cmd.Flags().StringSliceVar(&flags.disable, "disable", nil, "rule IDs to disable")
	cmd.Flags().StringSliceVar(&flags.fixRules, "fix-rules", nil, "limit auto-fix to specific rule IDs")
	cmd.Flags().BoolVar(&cfg.NoBackups, "no-backups", false, "disable backup creation when fixing")
	cmd.Flags().StringVar(&flags.flavor, "flavor", "commonmark", "Markdown flavor: commonmark, gfm")
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
