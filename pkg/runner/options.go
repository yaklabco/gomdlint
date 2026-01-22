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
	ExcludeGlobs []string

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
