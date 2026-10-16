// Package runner provides multi-file linting orchestration.
package runner

import "github.com/yaklabco/gomdlint/pkg/config"

// Options controls multi-file linting behavior.
type Options struct {
	// Paths are the user-specified paths (files or directories) to process.
	// If empty, defaults to the current working directory.
	Paths []string

	// WorkingDir is the base directory used to resolve relative Paths.
	// If empty, the current process working directory is used.
	WorkingDir string

	// Extensions is the set of file extensions (lowercase, with leading dot)
	// considered Markdown. Defaults to [".md", ".markdown"] via DefaultExtensions().
	Extensions []string

	// IncludeGlobs are additional glob patterns to include, relative to WorkingDir.
	// Empty means "include everything that matches Extensions".
	IncludeGlobs []string

	// ExcludeGlobs are glob patterns used to skip files or directories.
	// These merge ignore rules from config and CLI (e.g. --ignore).
	// They apply to paths named in Paths as well as to walked entries.
	//
	// Patterns use forward slashes and match whole path segments: "*" stays
	// within a segment and a "**" segment matches any number of segments.
	// A pattern without a slash matches a name at any depth, and a trailing
	// slash matches directories only.
	ExcludeGlobs []string

	// IgnoreBaseDir is the directory relative ExcludeGlobs are resolved
	// against, such as the directory of the config file that set them.
	// Empty means WorkingDir.
	IgnoreBaseDir string

	// Gitignore skips entries that .gitignore files ignore while walking
	// directories. The .gitignore files of the enclosing Git working tree
	// apply from its root down. Paths named in Paths are always processed.
	Gitignore bool

	// SkipDirs names directories that walks never enter, such as
	// "node_modules". Directories named in Paths are still walked.
	SkipDirs []string

	// FollowSymlinks controls whether directory symlinks are traversed.
	FollowSymlinks bool

	// Jobs controls the maximum number of concurrent workers.
	// 0 or negative means "auto" (runtime.NumCPU()).
	Jobs int

	// Config is the resolved configuration for this run.
	Config *config.Config
}

// DefaultExtensions returns the default set of Markdown file extensions.
func DefaultExtensions() []string {
	return []string{".md", ".markdown"}
}

// DefaultSkipDirs returns the directories that hold third-party code, which
// walks skip by default.
func DefaultSkipDirs() []string {
	return []string{"node_modules", "vendor"}
}

// effectiveExtensions returns the extensions to use, defaulting if empty.
func (o Options) effectiveExtensions() []string {
	if len(o.Extensions) == 0 {
		return DefaultExtensions()
	}
	return o.Extensions
}

// effectivePaths returns the paths to process, defaulting to "." if empty.
func (o Options) effectivePaths() []string {
	if len(o.Paths) == 0 {
		return []string{"."}
	}
	return o.Paths
}
