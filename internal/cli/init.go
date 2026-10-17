package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/configloader"
	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
)

// configFilePermissions is the file mode for configuration files (world-readable).
const configFilePermissions = 0644

// The formats init writes config files in, the values of its --format flag.
const (
	templateYAML = "yaml"
	templateJSON = "json"
)

// initFlags holds the flags for the init command.
type initFlags struct {
	force  bool
	full   bool
	format string
	output string

	// formatSet is true when --format was given, so the output name does
	// not choose the format.
	formatSet bool
}

func newInitCommand() *cobra.Command {
	flags := &initFlags{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new gomdlint configuration file",
		Long: `Create a new .gomdlint.yml configuration file in the current directory
with sensible defaults. The file can be customized to enable/disable rules,
change severities, and configure other options.

A file whose name ends in .json holds JSON, so --output with such a name
writes JSON unless --format says otherwise.

Examples:
  gomdlint init                   Create minimal .gomdlint.yml
  gomdlint init --full            Create full config with all rules documented
  gomdlint init --format json     Create .gomdlint.json instead
  gomdlint init --output custom.yml  Write to a custom file path`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags.formatSet = cmd.Flags().Changed("format")
			return runInit(flags)
		},
	}

	cmd.Flags().BoolVarP(&flags.force, "force", "f", false, "Overwrite existing configuration file")
	cmd.Flags().BoolVar(&flags.full, "full", false, "Generate full template with all rules documented")
	cmd.Flags().StringVar(&flags.format, "format", templateYAML,
		"Output format: yaml or json (json by default for an --output name ending in .json)")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Output file path (default: .gomdlint.yml or .gomdlint.json)")

	return cmd
}

func runInit(flags *initFlags) error {
	logger := logging.NewInteractive()

	// Validate format
	format := flags.format
	if format != templateYAML && format != templateJSON {
		return usageError("invalid format %q: must be yaml or json", format)
	}

	// Determine output path. lint reads a .json file as JSON, so such a
	// name selects JSON, and YAML cannot go there.
	outputPath := flags.output
	switch {
	case outputPath == "" && format == templateJSON:
		outputPath = ".gomdlint.json"
	case outputPath == "":
		outputPath = ".gomdlint.yml"
	case configloader.HoldsJSON(outputPath) && !flags.formatSet:
		format = templateJSON
	case configloader.HoldsJSON(outputPath) && format == templateYAML:
		return usageError("--format yaml cannot write %q: a .json file must hold JSON; use --format json or a .yml name", outputPath)
	}

	// Make path absolute
	absPath, err := filepath.Abs(outputPath)
	if err != nil {
		return withExitCode(ExitInternalError, fmt.Errorf("resolve path: %w", err))
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err == nil {
		if !flags.force {
			return usageError("file %q already exists; use --force to overwrite", outputPath)
		}
		logger.Warn("overwriting existing file", logging.FieldPath, outputPath)
	}

	// Generate template
	opts := config.TemplateOptions{
		Full:   flags.full,
		Format: format,
	}

	content, err := config.GenerateTemplate(opts)
	if err != nil {
		return withExitCode(ExitInternalError, fmt.Errorf("generate template: %w", err))
	}

	// Write file
	if err := os.WriteFile(absPath, content, configFilePermissions); err != nil {
		return withExitCode(ExitIOError, fmt.Errorf("write file: %w", err))
	}

	logger.Info("created configuration file", logging.FieldPath, outputPath)

	if flags.full {
		logger.Info("full template includes all rules with documentation")
	}

	logger.Info("customize your configuration by editing the file")
	logger.Info("run 'gomdlint rules' to see all available rules")

	return nil
}
