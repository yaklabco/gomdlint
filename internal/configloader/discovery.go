package configloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ConfigPaths represents discovered configuration file paths.
type ConfigPaths struct {
	// System is the system-wide config path (e.g., /etc/gomdlint/config.yaml).
	System string

	// User is the user-level config path (e.g., ~/.config/gomdlint/config.yaml).
	User string

	// Project is the project-level config path (e.g., ./.gomdlint.yml).
	Project string

	// Explicit is a config path provided via --config flag.
	Explicit string

	// Markdownlint is a detected markdownlint config file path.
	Markdownlint string
}

// gomdlintConfigFiles are the config file names we search for, in order of preference.
//
//nolint:gochecknoglobals // Read-only lookup table.
var gomdlintConfigFiles = []string{
	".gomdlint.yml",
	".gomdlint.yaml",
	"gomdlint.yml",
	"gomdlint.yaml",
}

// markdownlintConfigFiles are the markdownlint config files we detect for migration.
//
//nolint:gochecknoglobals // Read-only lookup table.
var markdownlintConfigFiles = []string{
	".markdownlint.json",
	".markdownlint.jsonc",
	".markdownlint.yaml",
	".markdownlint.yml",
	".markdownlint.cjs",
	".markdownlint.mjs",
}

// vcsRootMarkers are directories that indicate a VCS root.
//
//nolint:gochecknoglobals // Read-only lookup table.
var vcsRootMarkers = []string{".git", ".hg", ".svn"}

// DiscoverPaths finds configuration files in standard locations.
// It searches for:
//   - System config at /etc/gomdlint/config.{yaml,yml}
//   - User config at $XDG_CONFIG_HOME/gomdlint/config.{yaml,yml}
//   - Project config by searching upward from workDir for .gomdlint.{yaml,yml}
//   - Markdownlint config for migration purposes
//
// Missing files are represented as empty strings (not errors).
func DiscoverPaths(ctx context.Context, workDir string) (*ConfigPaths, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	paths := &ConfigPaths{}

	// Find system config
	paths.System = findSystemConfig()

	// Find user config
	paths.User = findUserConfig()

	// Find project config (searches upward)
	projectConfig, err := FindProjectConfig(ctx, workDir)
	if err != nil {
		return nil, err
	}
	paths.Project = projectConfig

	// Find markdownlint config (for migration)
	paths.Markdownlint = findMarkdownlintConfig(workDir)

	return paths, nil
}

// findSystemConfig returns the path to the system-wide config file, if it exists.
func findSystemConfig() string {
	if runtime.GOOS == "windows" {
		// On Windows, use ProgramData
		programData := os.Getenv("ProgramData")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return findConfigInDir(filepath.Join(programData, "gomdlint"))
	}

	// On Unix-like systems, use /etc
	return findConfigInDir("/etc/gomdlint")
}

// findUserConfig returns the path to the user-level config file, if it exists.
func findUserConfig() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}

	return findConfigInDir(filepath.Join(configHome, "gomdlint"))
}

// findConfigInDir looks for config files in the given directory.
// Returns the path to the first found file, or empty string if none.
func findConfigInDir(dir string) string {
	for _, name := range []string{"config.yaml", "config.yml"} {
		path := filepath.Join(dir, name)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// FindProjectConfig searches upward from startDir for a project config file.
// Returns the path to the first config file found, or empty string if none.
// Stops at filesystem boundaries, VCS roots, or when reaching root.
func FindProjectConfig(ctx context.Context, startDir string) (string, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
	}

	// Resolve to absolute path
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		// If we can't get home dir, we'll just skip the home boundary check.
		homeDir = ""
	}

	currentDir := absDir
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		// Check for gomdlint config files in current directory
		for _, name := range gomdlintConfigFiles {
			path := filepath.Join(currentDir, name)
			if fileExists(path) {
				return path, nil
			}
		}

		// Check if we've hit a VCS root (optimization: stop here)
		if isVCSRoot(currentDir) {
			return "", nil
		}

		// Check if we've reached home directory boundary
		if homeDir != "" && currentDir == homeDir {
			return "", nil
		}

		// Move to parent directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached filesystem root
			return "", nil
		}
		currentDir = parentDir
	}
}

// findMarkdownlintConfig looks for a markdownlint config file in the given directory.
// Returns the path to the first found file, or empty string if none.
func findMarkdownlintConfig(dir string) string {
	for _, name := range markdownlintConfigFiles {
		path := filepath.Join(dir, name)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// FindMarkdownlintConfig is the exported version that searches for markdownlint config.
func FindMarkdownlintConfig(dir string) string {
	return findMarkdownlintConfig(dir)
}

// isVCSRoot returns true if the directory contains a VCS root marker.
func isVCSRoot(dir string) bool {
	for _, marker := range vcsRootMarkers {
		path := filepath.Join(dir, marker)
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// fileExists returns true if the path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// IsJavaScriptConfig returns true if the path is a JavaScript config file.
// These cannot be converted and require user action.
func IsJavaScriptConfig(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".cjs" || ext == ".mjs"
}

// IsJSONConfig returns true if the path is a JSON config file.
func IsJSONConfig(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".json" || ext == ".jsonc"
}

// IsYAMLConfig returns true if the path is a YAML config file.
func IsYAMLConfig(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yaml" || ext == ".yml"
}
