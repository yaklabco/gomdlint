package runner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover finds Markdown files matching opts under the given working directory.
// It returns a deterministically sorted list of absolute file paths.
func Discover(ctx context.Context, opts Options) ([]string, error) {
	// Resolve working directory.
	workDir, err := resolveWorkDir(opts.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}

	extensions := opts.effectiveExtensions()
	paths := opts.effectivePaths()

	// Use a map for deduplication.
	seen := make(map[string]struct{})
	var files []string

	for _, inputPath := range paths {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("discovery cancelled: %w", ctx.Err())
		default:
		}

		// Resolve to absolute path.
		absPath := inputPath
		if !filepath.IsAbs(inputPath) {
			absPath = filepath.Join(workDir, inputPath)
		}
		absPath = filepath.Clean(absPath)

		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", inputPath, err)
		}

		if info.IsDir() {
			// Walk directory.
			discovered, err := walkDirectory(ctx, absPath, workDir, extensions, opts)
			if err != nil {
				return nil, err
			}
			for _, f := range discovered {
				if _, ok := seen[f]; !ok {
					seen[f] = struct{}{}
					files = append(files, f)
				}
			}
		} else if matchesFile(absPath, workDir, extensions, opts) {
			// Single file: check if it matches criteria.
			if _, ok := seen[absPath]; !ok {
				seen[absPath] = struct{}{}
				files = append(files, absPath)
			}
		}
	}

	// Sort for deterministic ordering.
	sort.Strings(files)

	return files, nil
}

// resolveWorkDir resolves the working directory, defaulting to os.Getwd().
func resolveWorkDir(workDir string) (string, error) {
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		return wd, nil
	}
	absPath, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	return absPath, nil
}

// walkDirectory recursively walks a directory and returns matching Markdown files.
func walkDirectory(
	ctx context.Context,
	root string,
	workDir string,
	extensions []string,
	opts Options,
) ([]string, error) {
	var files []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		// Check for context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if walkErr != nil {
			// Handle permission errors gracefully.
			if os.IsPermission(walkErr) {
				return nil
			}
			return walkErr
		}

		// Get relative path for pattern matching.
		relPath, relErr := filepath.Rel(workDir, path)
		if relErr != nil {
			relPath = path
		}

		// Handle directories.
		if entry.IsDir() {
			// Skip hidden directories (except root).
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}

			// Check if directory should be excluded.
			if matchesExcludePattern(relPath, opts.ExcludeGlobs) {
				return filepath.SkipDir
			}

			return nil
		}

		// Handle symlinks.
		if entry.Type()&fs.ModeSymlink != 0 {
			// Resolve symlink to check if it points to a file or directory.
			realPath, evalErr := filepath.EvalSymlinks(path)
			if evalErr != nil {
				// Broken symlink, skip silently.
				return nil //nolint:nilerr // Intentionally skip broken symlinks
			}
			info, statErr := os.Stat(realPath)
			if statErr != nil {
				// Cannot stat target, skip silently.
				return nil //nolint:nilerr // Intentionally skip inaccessible symlink targets
			}
			if info.IsDir() {
				// Directory symlink: skip unless FollowSymlinks is set.
				if !opts.FollowSymlinks {
					return nil
				}
				// Walk the symlink TARGET (realPath), not the symlink itself.
				// This avoids infinite recursion since WalkDir uses Lstat on root.
				subFiles, err := walkDirectory(ctx, realPath, workDir, extensions, opts)
				if err != nil {
					return err
				}
				files = append(files, subFiles...)
				return nil
			}
			// File symlink: continue to check as regular file.
		}

		// Skip hidden files.
		if strings.HasPrefix(entry.Name(), ".") {
			return nil
		}

		// Check if file matches criteria.
		if matchesFile(path, workDir, extensions, opts) {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory %s: %w", root, err)
	}

	return files, nil
}

// matchesFile checks if a file path matches the inclusion criteria.
func matchesFile(path, workDir string, extensions []string, opts Options) bool {
	// Get relative path for pattern matching.
	relPath, err := filepath.Rel(workDir, path)
	if err != nil {
		relPath = path
	}

	// Check extension.
	if !hasMatchingExtension(path, extensions) {
		return false
	}

	// Check exclude patterns.
	if matchesExcludePattern(relPath, opts.ExcludeGlobs) {
		return false
	}

	// Check include patterns (if specified).
	if len(opts.IncludeGlobs) > 0 {
		if !matchesIncludePattern(relPath, opts.IncludeGlobs) {
			return false
		}
	}

	return true
}

// hasMatchingExtension checks if the file has a matching extension.
func hasMatchingExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range extensions {
		if strings.ToLower(e) == ext {
			return true
		}
	}
	return false
}

// matchesExcludePattern checks if the path matches any exclude pattern.
func matchesExcludePattern(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(relPath, pattern) {
			return true
		}
	}
	return false
}

// matchesIncludePattern checks if the path matches any include pattern.
func matchesIncludePattern(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(relPath, pattern) {
			return true
		}
	}
	return false
}

// matchGlob matches a path against a glob pattern.
// It supports patterns like "*.md", "docs/**", "vendor/**", etc.
func matchGlob(path, pattern string) bool {
	// Normalize path separators for matching.
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	// Handle ** patterns for recursive matching.
	if strings.Contains(pattern, "**") {
		return matchDoubleStarPattern(path, pattern)
	}

	// Standard filepath.Match for simple patterns.
	matched, matchErr := filepath.Match(pattern, path)
	if matchErr != nil {
		return false
	}
	if matched {
		return true
	}

	// Also try matching against just the filename.
	matched, matchErr = filepath.Match(pattern, filepath.Base(path))
	if matchErr != nil {
		return false
	}
	return matched
}

// matchDoubleStarPattern handles ** glob patterns.
func matchDoubleStarPattern(path, pattern string) bool {
	// Split pattern by **
	parts := strings.Split(pattern, "**")

	if len(parts) == 1 {
		// No ** found, shouldn't happen but handle gracefully.
		matched, matchErr := filepath.Match(pattern, path)
		if matchErr != nil {
			return false
		}
		return matched
	}

	// Handle common patterns:
	// "**/foo" - matches foo anywhere
	// "foo/**" - matches anything under foo
	// "**/foo/**" - matches foo directory anywhere

	if parts[0] == "" && len(parts) == 2 {
		// Pattern starts with **/, e.g., "**/vendor"
		suffix := strings.TrimPrefix(parts[1], "/")
		if suffix == "" {
			// Just "**" matches everything.
			return true
		}

		// Check if path ends with the suffix or contains it as a path component.
		if strings.HasSuffix(path, suffix) {
			return true
		}

		// Check if any path component matches.
		pathParts := strings.Split(path, "/")
		for _, part := range pathParts {
			matched, matchErr := filepath.Match(suffix, part)
			if matchErr == nil && matched {
				return true
			}
		}

		// Check if suffix matches a subpath.
		if strings.Contains(path, suffix) {
			return true
		}

		return false
	}

	if parts[1] == "" || parts[1] == "/" {
		// Pattern ends with /**, e.g., "vendor/**"
		prefix := strings.TrimSuffix(parts[0], "/")
		if prefix == "" {
			return true
		}
		return strings.HasPrefix(path, prefix+"/") || path == prefix
	}

	// Complex pattern with ** in the middle.
	// Simplified: check if prefix matches start and suffix matches end.
	prefix := strings.TrimSuffix(parts[0], "/")
	suffix := strings.TrimPrefix(parts[1], "/")

	if prefix != "" && !strings.HasPrefix(path, prefix) {
		return false
	}

	if suffix != "" && !strings.HasSuffix(path, suffix) {
		// Also check if suffix pattern matches.
		matched, matchErr := filepath.Match(suffix, filepath.Base(path))
		if matchErr != nil || !matched {
			return false
		}
	}

	return true
}
