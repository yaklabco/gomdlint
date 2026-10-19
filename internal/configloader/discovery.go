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

	// Project is the project-level config path (e.g., ./.gomdlint.yml or
	// ./.gomdlint.json).
	Project string

	// Explicit is a config path provided via --config flag.
	Explicit string

	// Markdownlint is a detected markdownlint config file path.
	Markdownlint string

	// Shadowed maps each config file found to the other config files in
	// its directory, in their order of precedence, which it shadows:
	// gomdlint reads the file and ignores them, as it reads one config
	// file of each kind per directory.
	Shadowed map[string][]string
}

// shadow records that path shadows ignored, unless ignored is empty.
func (p *ConfigPaths) shadow(path string, ignored []string) {
	if len(ignored) == 0 {
		return
	}
	if p.Shadowed == nil {
		p.Shadowed = make(map[string][]string)
	}
	p.Shadowed[path] = ignored
}

// gomdlintConfigFiles are the project config file names, in order of
// precedence: in a directory that holds several, the first one is used and
// the others are ignored. "gomdlint init" writes the first, or the fifth
// with --format json. YAML names come before JSON ones, so a project with a
// YAML config keeps using it when a JSON one appears beside it.
//
//nolint:gochecknoglobals // Read-only lookup table.
var gomdlintConfigFiles = []string{
	".gomdlint.yml",
	".gomdlint.yaml",
	"gomdlint.yml",
	"gomdlint.yaml",
	".gomdlint.json",
	"gomdlint.json",
}

// markdownlintConfigFiles are the markdownlint config files we detect for
// migration, in the order in which markdownlint-cli2 0.23.3 looks for them
// (configurationFiles in markdownlint-cli2.mjs): in a directory that holds
// several, the first one is used.
//
//nolint:gochecknoglobals // Read-only lookup table.
var markdownlintConfigFiles = []string{
	".markdownlint.jsonc",
	".markdownlint.json",
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
//   - System config at /etc/gomdlint/config.{yaml,yml,json}
//   - User config at $XDG_CONFIG_HOME/gomdlint/config.{yaml,yml,json}
//   - Project config by searching upward from workDir for one of
//     gomdlintConfigFiles, such as .gomdlint.yml or .gomdlint.json
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
	var shadowed []string

	// Find system config
	paths.System, shadowed = findSystemConfig()
	paths.shadow(paths.System, shadowed)

	// Find user config
	paths.User, shadowed = findUserConfig()
	paths.shadow(paths.User, shadowed)

	// Find project config (searches upward)
	var err error
	paths.Project, shadowed, err = findUpwardShadowing(ctx, workDir, gomdlintConfigFiles)
	if err != nil {
		return nil, err
	}
	paths.shadow(paths.Project, shadowed)

	// Find a markdownlint config, searching upward like the project config.
	paths.Markdownlint, shadowed, err = findUpwardShadowing(ctx, workDir, markdownlintConfigFiles)
	if err != nil {
		return nil, err
	}
	paths.shadow(paths.Markdownlint, shadowed)

	return paths, nil
}

// goosWindows is runtime.GOOS on Windows.
const goosWindows = "windows"

// findSystemConfig returns the path to the system-wide config file, if it
// exists, and the config files in its directory that it shadows (see
// findConfigInDir).
func findSystemConfig() (string, []string) {
	if runtime.GOOS == goosWindows {
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

// findUserConfig returns the path to the user-level config file, if it
// exists, and the config files in its directory that it shadows (see
// findConfigInDir).
func findUserConfig() (string, []string) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil
		}
		configHome = filepath.Join(home, ".config")
	}

	return findConfigInDir(filepath.Join(configHome, "gomdlint"))
}

// findConfigInDir looks for config files in the given directory.
// Returns the path to the first found file, or empty string if none, and
// the paths of the other config files found there, which it shadows.
// YAML names come first, as they do for project configs.
func findConfigInDir(dir string) (string, []string) {
	return firstFileIn(dir, []string{"config.yaml", "config.yml", "config.json"})
}

// firstFileIn returns the path of the file in dir with the first of names
// that one has, or the empty string when none has, and the paths of the
// files in dir with later names, which the first shadows, in the order of
// names.
func firstFileIn(dir string, names []string) (string, []string) {
	var found string
	var shadowed []string
	for _, name := range names {
		path := filepath.Join(dir, name)
		switch {
		case !fileExists(path):
		case found == "":
			found = path
		default:
			shadowed = append(shadowed, path)
		}
	}
	return found, shadowed
}

// FindProjectConfig searches upward from startDir for a project config file.
// Returns the path to the first config file found, or empty string if none.
// The nearest directory with a config file wins, and within it the name
// that comes first in gomdlintConfigFiles. Stops at filesystem boundaries,
// VCS roots, or when reaching root.
func FindProjectConfig(ctx context.Context, startDir string) (string, error) {
	path, _, err := findUpwardShadowing(ctx, startDir, gomdlintConfigFiles)
	return path, err
}

// findUpwardShadowing searches upward from startDir for a file with one of
// names, preferring earlier names within a directory, and returns its path
// and the paths of the files with later names in its directory, which it
// shadows. It stops after a VCS root or the home directory, and at the
// filesystem root.
func findUpwardShadowing(ctx context.Context, startDir string, names []string) (string, []string, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	// Resolve to absolute path
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve absolute path: %w", err)
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
			return "", nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		// Check for config files in current directory
		if path, shadowed := firstFileIn(currentDir, names); path != "" {
			return path, shadowed, nil
		}

		// Check if we've hit a VCS root (optimization: stop here)
		if isVCSRoot(currentDir) {
			return "", nil, nil
		}

		// Check if we've reached home directory boundary
		if homeDir != "" && currentDir == homeDir {
			return "", nil, nil
		}

		// Move to parent directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached filesystem root
			return "", nil, nil
		}
		currentDir = parentDir
	}
}

// findMarkdownlintConfig looks for a markdownlint config file in the given directory.
// Returns the path to the first found file, or empty string if none.
func findMarkdownlintConfig(dir string) string {
	path, _ := firstFileIn(dir, markdownlintConfigFiles)
	return path
}

// FindMarkdownlintConfig is the exported version that searches for markdownlint config.
func FindMarkdownlintConfig(dir string) string {
	return findMarkdownlintConfig(dir)
}

// isVCSRoot returns true if the directory contains a VCS root marker. Git
// worktrees and submodules mark their root with a .git file instead of a
// directory, so any entry counts.
func isVCSRoot(dir string) bool {
	for _, marker := range vcsRootMarkers {
		if _, err := os.Lstat(filepath.Join(dir, marker)); err == nil {
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
