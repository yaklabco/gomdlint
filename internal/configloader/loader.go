// Package configloader provides configuration loading and resolution.
// It implements XDG-compliant configuration discovery, hierarchical merging,
// environment variable support, validation, and markdownlint migration.
package configloader

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// configFilePermissions is the file mode for configuration files (world-readable).
const configFilePermissions = 0644

// LoadOptions controls configuration loading behavior.
type LoadOptions struct {
	// WorkingDir is the directory to search from for project config.
	// Defaults to current working directory if empty.
	WorkingDir string

	// ExplicitPath is an explicit config file path (from --config flag).
	// If set, project config discovery is skipped.
	ExplicitPath string

	// IgnoreSystemConfig skips loading system-level configuration.
	IgnoreSystemConfig bool

	// IgnoreUserConfig skips loading user-level configuration.
	IgnoreUserConfig bool

	// IgnoreProjectConfig skips loading project-level configuration.
	IgnoreProjectConfig bool

	// IgnoreEnv skips loading environment variables.
	IgnoreEnv bool

	// IgnoreMarkdownlint skips markdownlint config detection and migration.
	IgnoreMarkdownlint bool

	// Verbose enables logging of configuration resolution steps.
	Verbose bool

	// NonInteractive disables interactive prompts (e.g., in CI).
	NonInteractive bool

	// CLIConfig contains configuration from CLI flags.
	// These take highest precedence.
	CLIConfig *config.Config
}

// LoadResult contains the resolved configuration and metadata.
type LoadResult struct {
	// Config is the final merged configuration.
	Config *config.Config

	// Paths contains the discovered configuration file paths.
	Paths *ConfigPaths

	// LoadedFrom lists the files that were actually loaded (in order).
	LoadedFrom []string

	// Warnings contains non-fatal issues encountered during loading.
	Warnings []string

	// MigrationPerformed is true if a markdownlint config was converted.
	MigrationPerformed bool
}

// Load resolves the final configuration by merging all sources.
// Precedence (highest to lowest):
//  1. CLI flags (opts.CLIConfig)
//  2. Environment variables (GOMDLINT_*)
//  3. Explicit config file (opts.ExplicitPath)
//  4. Project config (.gomdlint.yml upward search)
//  5. User config ($XDG_CONFIG_HOME/gomdlint/config.yaml)
//  6. System config (/etc/gomdlint/config.yaml)
//  7. Defaults
func Load(ctx context.Context, opts LoadOptions) (*LoadResult, error) {
	result := &LoadResult{
		Paths: &ConfigPaths{},
	}

	// Resolve working directory
	workDir := opts.WorkingDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	// Start with defaults
	cfg := config.NewConfig()

	// Discover config paths
	paths, err := DiscoverPaths(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("discover paths: %w", err)
	}
	result.Paths = paths

	// Handle explicit config path
	if opts.ExplicitPath != "" {
		result.Paths.Explicit = opts.ExplicitPath
	}

	// Check for markdownlint config migration
	if !opts.IgnoreMarkdownlint {
		migrated, err := handleMarkdownlintMigration(ctx, paths, result, opts, workDir)
		if err != nil {
			return nil, err
		}
		if migrated {
			// Re-discover paths after migration
			paths, err = DiscoverPaths(ctx, workDir)
			if err != nil {
				return nil, fmt.Errorf("discover paths after migration: %w", err)
			}
			result.Paths = paths
		}
	}

	// Load and merge in order (lowest to highest precedence)

	// 1. System config
	if !opts.IgnoreSystemConfig && paths.System != "" {
		systemCfg, err := loadConfigFile(paths.System)
		if err != nil {
			return nil, fmt.Errorf("load system config: %w", err)
		}
		cfg = merge(cfg, systemCfg)
		result.LoadedFrom = append(result.LoadedFrom, paths.System)
	}

	// 2. User config
	if !opts.IgnoreUserConfig && paths.User != "" {
		userCfg, err := loadConfigFile(paths.User)
		if err != nil {
			return nil, fmt.Errorf("load user config: %w", err)
		}
		cfg = merge(cfg, userCfg)
		result.LoadedFrom = append(result.LoadedFrom, paths.User)
	}

	// 3. Project config
	if !opts.IgnoreProjectConfig && paths.Project != "" {
		projectCfg, err := loadConfigFile(paths.Project)
		if err != nil {
			return nil, fmt.Errorf("load project config: %w", err)
		}
		cfg = merge(cfg, projectCfg)
		result.LoadedFrom = append(result.LoadedFrom, paths.Project)
	}

	// 4. Explicit config (--config flag)
	if opts.ExplicitPath != "" {
		explicitCfg, err := loadConfigFile(opts.ExplicitPath)
		if err != nil {
			return nil, fmt.Errorf("load explicit config: %w", err)
		}
		cfg = merge(cfg, explicitCfg)
		result.LoadedFrom = append(result.LoadedFrom, opts.ExplicitPath)
	}

	// 5. Environment variables
	if !opts.IgnoreEnv {
		if err := LoadFromEnv(cfg); err != nil {
			return nil, fmt.Errorf("load environment: %w", err)
		}
	}

	// 6. CLI config (highest precedence)
	if opts.CLIConfig != nil {
		cfg = merge(cfg, opts.CLIConfig)
	}

	// Normalize rule keys to canonical IDs
	// This allows users to use rule names like "no-trailing-spaces" in config
	normalizeRuleKeys(cfg, lint.DefaultRegistry, result)

	// Validate final configuration
	validation := Validate(cfg)
	if !validation.Valid() {
		// Return first error
		return nil, &validation.Errors[0]
	}

	// Add validation warnings to result
	for _, w := range validation.Warnings {
		result.Warnings = append(result.Warnings, w.Message)
	}

	result.Config = cfg
	return result, nil
}

// loadConfigFile loads a configuration from a YAML file.
func loadConfigFile(path string) (*config.Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	cfg := &config.Config{}
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	// Ensure Rules map is initialized
	if cfg.Rules == nil {
		cfg.Rules = make(map[string]config.RuleConfig)
	}

	return cfg, nil
}

// handleMarkdownlintMigration checks for markdownlint config and offers migration.
func handleMarkdownlintMigration(
	_ context.Context,
	paths *ConfigPaths,
	result *LoadResult,
	opts LoadOptions,
	_ string,
) (bool, error) {
	// If we already have a gomdlint config, ignore markdownlint config
	if paths.Project != "" {
		if paths.Markdownlint != "" {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("both .gomdlint.yml and %s exist; using .gomdlint.yml", paths.Markdownlint))
		}
		return false, nil
	}

	// No markdownlint config found
	if paths.Markdownlint == "" {
		return false, nil
	}

	// Check if we can migrate
	if !CanMigrate(paths.Markdownlint) {
		result.Warnings = append(result.Warnings, GetMigrationWarning(paths.Markdownlint))
		return false, nil
	}

	// In non-interactive mode, don't prompt
	if opts.NonInteractive || !isInteractive() {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("found %s but no .gomdlint.yml; run 'gomdlint migrate' to convert", paths.Markdownlint))
		return false, nil
	}

	// Prompt user for migration
	shouldMigrate, err := promptMigration(paths.Markdownlint)
	if err != nil {
		return false, err
	}

	if !shouldMigrate {
		return false, nil
	}

	// Perform migration
	migrationResult, err := ConvertMarkdownlintConfig(paths.Markdownlint)
	if err != nil {
		return false, fmt.Errorf("convert markdownlint config: %w", err)
	}

	// Add migration warnings
	result.Warnings = append(result.Warnings, migrationResult.Warnings...)

	// Write the new config
	outputPath := ".gomdlint.yml"
	if err := writeConfig(migrationResult.Config, outputPath); err != nil {
		return false, fmt.Errorf("write migrated config: %w", err)
	}

	result.MigrationPerformed = true
	result.Warnings = append(result.Warnings,
		fmt.Sprintf("migrated %s to %s; you can now delete the old file", paths.Markdownlint, outputPath))

	return true, nil
}

// promptMigration asks the user if they want to migrate.
func promptMigration(markdownlintPath string) (bool, error) {
	// Write prompt to stdout
	if _, err := os.Stdout.WriteString("Found " + markdownlintPath + " but no .gomdlint.yml\n"); err != nil {
		return false, fmt.Errorf("write prompt: %w", err)
	}
	if _, err := os.Stdout.WriteString("Convert to gomdlint format? [Y/n] "); err != nil {
		return false, fmt.Errorf("write prompt: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "" || response == "y" || response == "yes", nil
}

// isInteractive returns true if stdin is a terminal.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// writeConfig writes a configuration to a YAML file.
func writeConfig(cfg *config.Config, path string) error {
	content, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Add header comment
	header := `# gomdlint configuration
# See: https://github.com/yaklabco/gomdlint

`
	fullContent := header + string(content)

	if err := os.WriteFile(path, []byte(fullContent), configFilePermissions); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// normalizeRuleKeys converts rule names/aliases to canonical IDs in the config.
// This allows users to use human-readable names like "no-trailing-spaces" in config files.
// If a rule is specified by both ID and name, warns and uses the last value encountered.
func normalizeRuleKeys(cfg *config.Config, registry *lint.Registry, result *LoadResult) {
	if len(cfg.Rules) == 0 {
		return
	}

	// Build a new map with normalized keys
	normalized := make(map[string]config.RuleConfig, len(cfg.Rules))

	// Track which canonical IDs we've seen to detect duplicates
	seenIDs := make(map[string]string) // canonical ID -> original key

	for key, ruleCfg := range cfg.Rules {
		// Try to resolve the key to a canonical ID
		canonicalID, _, found := registry.Resolve(key)
		if !found {
			// Unknown rule - keep it as-is, validation will warn about it later
			normalized[key] = ruleCfg
			continue
		}

		// Check for duplicates (same rule specified multiple times with different keys)
		if originalKey, exists := seenIDs[canonicalID]; exists {
			// Duplicate detected - warn and use last value (overwrite)
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("duplicate rule configuration: %q and %q both refer to %s; using last value",
					originalKey, key, canonicalID))
		}

		// Store with canonical ID
		seenIDs[canonicalID] = key
		normalized[canonicalID] = ruleCfg
	}

	// Replace the rules map with normalized version
	cfg.Rules = normalized
}
