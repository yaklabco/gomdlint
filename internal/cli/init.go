package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/logging"
	"github.com/yaklabco/gomdlint/pkg/config"
)

// configFilePermissions is the file mode for configuration files (world-readable).
const configFilePermissions = 0644

// initFlags holds the flags for the init command.
type initFlags struct {
	force  bool
	full   bool
	format string
	output string
}

func newInitCommand() *cobra.Command {
	flags := &initFlags{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new gomdlint configuration file",
		Long: `Create a new .gomdlint.yml configuration file in the current directory
with sensible defaults. The file can be customized to enable/disable rules,
change severities, and configure other options.

Examples:
  gomdlint init                   Create minimal .gomdlint.yml
  gomdlint init --full            Create full config with all rules documented
  gomdlint init --format json     Create .gomdlint.json instead
  gomdlint init --output custom.yml  Write to a custom file path`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInit(flags)
		},
	}

	cmd.Flags().BoolVarP(&flags.force, "force", "f", false, "Overwrite existing configuration file")
	cmd.Flags().BoolVar(&flags.full, "full", false, "Generate full template with all rules documented")
	cmd.Flags().StringVar(&flags.format, "format", "yaml", "Output format: yaml or json")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Output file path (default: .gomdlint.yml or .gomdlint.json)")

	return cmd
}

func runInit(flags *initFlags) error {
	logger := logging.NewInteractive()

	// Validate format
	if flags.format != "yaml" && flags.format != "json" {
		return fmt.Errorf("invalid format %q: must be yaml or json", flags.format)
	}

	// Determine output path
	outputPath := flags.output
	if outputPath == "" {
		if flags.format == "json" {
			outputPath = ".gomdlint.json"
		} else {
			outputPath = ".gomdlint.yml"
		}
	}

	// Make path absolute
	absPath, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err == nil {
		if !flags.force {
			return fmt.Errorf("file %q already exists; use --force to overwrite", outputPath)
		}
		logger.Warn("overwriting existing file", logging.FieldPath, outputPath)
	}

	// Generate template
	opts := config.TemplateOptions{
		Full:   flags.full,
		Format: flags.format,
	}

	content, err := config.GenerateTemplate(opts)
	if err != nil {
		return fmt.Errorf("generate template: %w", err)
	}

	// Write file
	if err := os.WriteFile(absPath, content, configFilePermissions); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	logger.Info("created configuration file", logging.FieldPath, outputPath)

	if flags.full {
		logger.Info("full template includes all rules with documentation")
	}

	logger.Info("customize your configuration by editing the file")
	logger.Info("run 'gomdlint rules' to see all available rules")

	return nil
}
