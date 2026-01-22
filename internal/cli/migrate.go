package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/configloader"
	"github.com/yaklabco/gomdlint/internal/logging"
)

// migrateFlags holds the flags for the migrate command.
type migrateFlags struct {
	force  bool
	output string
	input  string
}

func newMigrateCommand() *cobra.Command {
	flags := &migrateFlags{}

	cmd := &cobra.Command{
		Use:   "migrate [input]",
		Short: "Convert a markdownlint configuration to gomdlint format",
		Long: `Convert an existing markdownlint configuration file (.markdownlint.json,
.markdownlint.yaml, etc.) to gomdlint format (.gomdlint.yml).

If no input file is specified, the command will search for markdownlint
configuration files in the current directory.

JavaScript configuration files (.markdownlint.cjs, .markdownlint.mjs) cannot
be converted automatically and require manual migration.

Examples:
  gomdlint migrate                       Auto-detect and convert markdownlint config
  gomdlint migrate .markdownlint.json    Convert specific file
  gomdlint migrate --output config.yml   Write to custom output path`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 {
				flags.input = args[0]
			}
			return runMigrate(flags)
		},
	}

	cmd.Flags().BoolVarP(&flags.force, "force", "f", false, "Overwrite existing output file")
	cmd.Flags().StringVarP(&flags.output, "output", "o", ".gomdlint.yml", "Output file path")

	return cmd
}

func runMigrate(flags *migrateFlags) error {
	logger := logging.NewInteractive()

	// Find input file
	inputPath := flags.input
	if inputPath == "" {
		// Auto-detect markdownlint config
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}

		inputPath = configloader.FindMarkdownlintConfig(cwd)
		if inputPath == "" {
			return errors.New("no markdownlint configuration file found in current directory")
		}

		logger.Info("found markdownlint config", logging.FieldPath, inputPath)
	}

	// Check input exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file does not exist: %s", inputPath)
	}

	// Check if we can migrate
	if !configloader.CanMigrate(inputPath) {
		return fmt.Errorf("migration not supported: %s", configloader.GetMigrationWarning(inputPath))
	}

	// Make output path absolute
	absOutput, err := filepath.Abs(flags.output)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}

	// Check output exists
	if _, err := os.Stat(absOutput); err == nil {
		if !flags.force {
			return fmt.Errorf("output file %q already exists; use --force to overwrite", flags.output)
		}
		logger.Warn("overwriting existing file", logging.FieldPath, flags.output)
	}

	// Perform migration
	result, err := configloader.ConvertMarkdownlintConfig(inputPath)
	if err != nil {
		return fmt.Errorf("convert configuration: %w", err)
	}

	// Report warnings
	for _, warning := range result.Warnings {
		logger.Warn(warning)
	}

	// Serialize to YAML
	header := configloader.GenerateMigrationHeader(inputPath)
	content, err := result.Config.ToYAMLWithHeader(header)
	if err != nil {
		return fmt.Errorf("serialize configuration: %w", err)
	}

	// Write output
	if err := os.WriteFile(absOutput, content, configFilePermissions); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}

	logger.Info("migration complete", logging.FieldInput, inputPath, logging.FieldOutput, flags.output)

	if len(result.Warnings) > 0 {
		logger.Warn("review warnings above and verify the migrated configuration")
	}

	logger.Info("you can now delete the old markdownlint configuration file")

	return nil
}
