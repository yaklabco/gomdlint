package configloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/lint"
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

	// Markdownlint is the file that holds the markdownlint configuration:
	// a markdownlint config file such as .markdownlint.json, or a
	// markdownlint-cli2 options file, whose config key holds it, in the
	// nearest directory that holds a config file or an options file that
	// sets the config key, or else the nearest options file (see
	// MarkdownlintCLI2).
	Markdownlint string

	// MarkdownlintCLI2 is the markdownlint-cli2 options file, such as
	// .markdownlint-cli2.jsonc, in the directory of Markdownlint, or the
	// empty string. markdownlint-cli2 reads both kinds of file in a
	// directory and lints with the markdownlint config file, so this is
	// Markdownlint itself only in a directory without one.
	MarkdownlintCLI2 string

	// MarkdownlintCLI2Nearer is the nearest markdownlint-cli2 options file
	// when it is nearer than Markdownlint's directory, which the search
	// passed because it does not set the config key, or the empty string.
	// Its other keys still configure markdownlint-cli2, so they get
	// warnings.
	MarkdownlintCLI2Nearer string

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

// markdownlintCLI2Files are the markdownlint-cli2 options files, whose
// config key holds a markdownlint configuration, in the order in which
// markdownlint-cli2 0.23.3 looks for them (optionsFiles in
// markdownlint-cli2.mjs): in a directory that holds several, the first one
// is used. markdownlint-cli2 reads both an options file and a markdownlint
// config file in a directory, and lints with the config file's
// configuration, so the options file holds the configuration only in a
// directory without one.
//
//nolint:gochecknoglobals // Read-only lookup table.
var markdownlintCLI2Files = []string{
	".markdownlint-cli2.jsonc",
	".markdownlint-cli2.yaml",
	".markdownlint-cli2.cjs",
	".markdownlint-cli2.mjs",
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
//   - Markdownlint config, by searching upward from workDir for a
//     markdownlint config file or a markdownlint-cli2 options file that
//     sets its config key (see markdownlintDir.optionsSetConfig), or
//     else the nearest options file
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

	// Find a markdownlint config, searching upward like the project config,
	// in the nearest directory that holds a markdownlint configuration (see
	// markdownlintDir.optionsSetConfig), or else the nearest options file.
	var markdownlint, nearestOptions markdownlintDir
	err = walkUpward(ctx, workDir, func(dir string) bool {
		found := readMarkdownlintDir(dir)
		passed := nearestOptions.options != "" // an options file in a nearer directory
		if found.options != "" && !passed {
			nearestOptions = found
		}
		setsConfig := found.optionsSetConfig()
		if found.config == "" && !setsConfig {
			return false
		}
		// Past an options file, markdownlint-cli2 merges this directory's
		// options file before it looks at its config file, so a config key
		// that the options file sets wins (markdownlint-cli2.mjs, the merge
		// by inheritance). In the directory it starts from, the config
		// file wins.
		found.preferOptions = passed && setsConfig
		markdownlint = found
		return true
	})
	if err != nil {
		return nil, err
	}
	switch {
	case markdownlint.source() == "":
		markdownlint = nearestOptions
	case nearestOptions.options != markdownlint.options:
		paths.MarkdownlintCLI2Nearer = nearestOptions.options
		paths.shadow(nearestOptions.options, nearestOptions.optionsShadowed)
	}
	paths.Markdownlint, paths.MarkdownlintCLI2 = markdownlint.source(), markdownlint.options
	if markdownlint.preferOptions {
		paths.shadow(markdownlint.options, slices.Concat(
			[]string{markdownlint.config}, markdownlint.configShadowed, markdownlint.optionsShadowed))
	} else {
		paths.shadow(markdownlint.config, markdownlint.configShadowed)
		paths.shadow(markdownlint.options, markdownlint.optionsShadowed)
	}

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
	var path string
	var shadowed []string
	err := walkUpward(ctx, startDir, func(dir string) bool {
		path, shadowed = firstFileIn(dir, names)
		return path != ""
	})
	return path, shadowed, err
}

// walkUpward calls found with startDir, an absolute form of it, and then
// with each of its parent directories in turn, until found reports true.
// It stops after a VCS root or the home directory, and at the filesystem
// root.
func walkUpward(ctx context.Context, startDir string, found func(dir string) bool) error {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
	}

	// Resolve to absolute path
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
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
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		// Check for config files in current directory
		if found(currentDir) {
			return nil
		}

		// Check if we've hit a VCS root (optimization: stop here)
		if isVCSRoot(currentDir) {
			return nil
		}

		// Check if we've reached home directory boundary
		if homeDir != "" && currentDir == homeDir {
			return nil
		}

		// Move to parent directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached filesystem root
			return nil
		}
		currentDir = parentDir
	}
}

// markdownlintDir is what a directory holds of the markdownlint
// configuration: its first markdownlint config file and its first
// markdownlint-cli2 options file, either of which may be empty, with the
// files of either kind that they shadow there.
type markdownlintDir struct {
	config, options                 string
	configShadowed, optionsShadowed []string

	// preferOptions reports that the options file's config key holds the
	// configuration in place of the config file, as it does in a parent
	// directory of an options file that sets none.
	preferOptions bool
}

// readMarkdownlintDir returns what dir holds of the markdownlint
// configuration.
func readMarkdownlintDir(dir string) markdownlintDir {
	var found markdownlintDir
	found.config, found.configShadowed = firstFileIn(dir, markdownlintConfigFiles)
	found.options, found.optionsShadowed = firstFileIn(dir, markdownlintCLI2Files)
	return found
}

// optionsSetConfig reports whether the directory's options file sets the
// config key (see lint.MarkdownlintCLI2Options.ConfigSet). An options file
// that gomdlint cannot read, such as a JavaScript one, may set it, and so
// counts as one, so that loading it warns or fails. markdownlint-cli2
// 0.23.3 lints a directory whose options files set none with the
// markdownlint config file of the nearest parent directory that has one.
func (d markdownlintDir) optionsSetConfig() bool {
	if d.options == "" {
		return false
	}
	options, read := readOptionsFile(d.options)
	return !read || options.ConfigSet
}

// readOptionsFile reads path, a markdownlint-cli2 options file, as
// markdownlint-cli2 reads it, and reports whether it could: not for a
// JavaScript options file, nor for one that cannot be opened or parsed.
func readOptionsFile(path string) (*lint.MarkdownlintCLI2Options, bool) {
	if IsJavaScriptConfig(path) {
		return nil, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	options, _, err := readMarkdownlintCLI2Options(path, content)
	return options, err == nil
}

// source returns the file that holds the directory's markdownlint
// configuration: its markdownlint config file, which markdownlint-cli2
// prefers to the config key of its options file unless preferOptions says
// otherwise, and otherwise its options file, or the empty string when it
// holds neither.
func (d markdownlintDir) source() string {
	if d.config != "" && !d.preferOptions {
		return d.config
	}
	return d.options
}

// findMarkdownlintConfig looks for a markdownlint config file in the given
// directory, or else a markdownlint-cli2 options file, whose config key
// holds a markdownlint configuration. Returns the path to the first found
// file, or empty string if none.
func findMarkdownlintConfig(dir string) string {
	return readMarkdownlintDir(dir).source()
}

// MigratedSourceAdvice returns the advice, in a sentence without its final
// period, for what to do with path, a markdownlint configuration whose
// settings are now in a gomdlint config file. A markdownlint config file
// can go. A markdownlint-cli2 options file holds keys that configure
// markdownlint-cli2 itself and that migration leaves out, such as globs,
// so only its config key can go, and when it has none, nothing can.
func MigratedSourceAdvice(path string) string {
	if !isMarkdownlintCLI2File(path) {
		return "you can now delete the old markdownlint configuration file"
	}
	if options, read := readOptionsFile(path); read && !options.ConfigSet {
		return "the markdownlint-cli2 options file sets no config key, " +
			"so the migrated configuration holds markdownlint's defaults"
	}
	return "you can now delete the config key of the markdownlint-cli2 options file; " +
		"its other keys still configure markdownlint-cli2"
}

// isMarkdownlintCLI2File reports whether path names a markdownlint-cli2
// options file, as markdownlint-cli2 tells one: by a name that ends in
// .markdownlint-cli2 and a supported extension.
func isMarkdownlintCLI2File(path string) bool {
	base := filepath.Base(path)
	for _, name := range markdownlintCLI2Files {
		if strings.HasSuffix(base, name) {
			return true
		}
	}
	return false
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
