package runner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/internal/ignore"
)

// Discover finds Markdown files matching opts under the given working directory.
// It returns a deterministically sorted list of absolute file paths.
//
// Directories that cannot be read are skipped; Runner.Run reports them as
// files that could not be linted.
func Discover(ctx context.Context, opts Options) ([]string, error) {
	found, err := discover(ctx, opts)
	if err != nil {
		return nil, err
	}
	return found.files, nil
}

// discovery is the outcome of file discovery.
type discovery struct {
	// files are the matching files, sorted.
	files []string

	// unreadable are directories whose contents could not be listed.
	unreadable []FileOutcome
}

// discover finds the files to lint and the directories it could not read.
func discover(ctx context.Context, opts Options) (*discovery, error) {
	// Resolve working directory.
	workDir, err := resolveWorkDir(opts.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}

	finder, err := newDiscoverer(workDir, opts)
	if err != nil {
		return nil, err
	}

	// Use a map for deduplication.
	seen := make(map[string]struct{})
	found := &discovery{}
	add := func(path string) {
		if _, ok := seen[path]; !ok {
			seen[path] = struct{}{}
			found.files = append(found.files, path)
		}
	}

	for _, inputPath := range opts.effectivePaths() {
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
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", inputPath, fs.ErrNotExist)
		}
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", inputPath, err)
		}

		if info.IsDir() {
			discovered, err := finder.walk(ctx, absPath)
			if err != nil {
				return nil, err
			}
			for _, path := range discovered {
				add(path)
			}
		} else if finder.matchesNamedFile(absPath) {
			add(absPath)
		}
	}

	// Sort for deterministic ordering.
	sort.Strings(found.files)
	found.files = uniqueFiles(found.files)
	found.unreadable = finder.unreadable

	return found, nil
}

// fileKey groups paths that might name the same file. Hard links and a
// symlink's target share size and modification time, so os.SameFile only
// compares paths within a group.
type fileKey struct {
	size    int64
	modTime int64
}

// uniqueFiles drops paths that name a file already listed, such as a hard
// link or a symlink to it, so two workers never process one file at once.
// Of the paths naming one file it keeps the first that is not a symlink.
// paths must be sorted; the result is too.
func uniqueFiles(paths []string) []string {
	type seenFile struct {
		info  fs.FileInfo
		index int // into kept
	}
	groups := make(map[fileKey][]seenFile)
	kept := make([]string, 0, len(paths))
	replaced := false

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			kept = append(kept, path) // Processing reports the error.
			continue
		}

		key := fileKey{size: info.Size(), modTime: info.ModTime().UnixNano()}
		duplicate := -1
		for _, seen := range groups[key] {
			if os.SameFile(info, seen.info) {
				duplicate = seen.index
				break
			}
		}
		if duplicate < 0 {
			groups[key] = append(groups[key], seenFile{info: info, index: len(kept)})
			kept = append(kept, path)
			continue
		}
		if isSymlink(kept[duplicate]) && !isSymlink(path) {
			kept[duplicate] = path
			replaced = true
		}
	}

	if replaced {
		sort.Strings(kept)
	}
	return kept
}

// isSymlink reports whether path itself is a symbolic link.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&fs.ModeSymlink != 0
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

// discoverer holds the matching rules of one discovery.
//
// Exclude patterns apply to every path, including those named on the
// command line. .gitignore files and skipped directory names only apply to
// entries found while walking, so a path the user names is always linted.
type discoverer struct {
	opts       Options
	extensions []string
	excludes   *ignore.Set
	includes   *ignore.Set

	// unreadable collects directories whose contents could not be listed.
	unreadable []FileOutcome
}

func newDiscoverer(workDir string, opts Options) (*discoverer, error) {
	base := workDir
	if opts.IgnoreBaseDir != "" {
		var err error
		if base, err = filepath.Abs(opts.IgnoreBaseDir); err != nil {
			return nil, fmt.Errorf("resolve ignore base directory: %w", err)
		}
	}

	excludes, err := ignore.NewSet(base, opts.ExcludeGlobs)
	if err != nil {
		return nil, fmt.Errorf("exclude patterns: %w", err)
	}
	includes, err := ignore.NewSet(workDir, opts.IncludeGlobs)
	if err != nil {
		return nil, fmt.Errorf("include patterns: %w", err)
	}

	return &discoverer{
		opts:       opts,
		extensions: opts.effectiveExtensions(),
		excludes:   excludes,
		includes:   includes,
	}, nil
}

// walk recursively walks root, a directory named by the caller, and
// returns the matching Markdown files under it.
func (d *discoverer) walk(ctx context.Context, root string) ([]string, error) {
	// Exclude patterns also cover a named directory and its parents.
	if d.excludes.MatchWithParents(root, true) {
		return nil, nil
	}

	// gitignores holds the .gitignore rules in effect inside each walked
	// directory; it stays empty when .gitignore files are not honoured.
	gitignores := make(map[string]*ignore.Gitignore)
	if d.opts.Gitignore {
		rules, err := ignore.GitignoreFor(root)
		if err != nil {
			return nil, fmt.Errorf("gitignore rules for %s: %w", root, err)
		}
		gitignores[root] = rules
	}

	var files []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		// Check for context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if walkErr != nil {
			// Report unreadable directories instead of failing the walk.
			if os.IsPermission(walkErr) {
				d.unreadable = append(d.unreadable, FileOutcome{
					Path:  path,
					Error: fmt.Errorf("read directory: %w", walkErr),
				})
				return nil
			}
			return walkErr
		}

		if path == root {
			return nil
		}

		rules := gitignores[filepath.Dir(path)]

		// Handle directories.
		if entry.IsDir() {
			if d.skipWalkedDir(path, entry.Name(), rules) {
				return filepath.SkipDir
			}
			if rules != nil {
				child, err := rules.Child(path)
				if err != nil {
					return fmt.Errorf("gitignore rules: %w", err)
				}
				gitignores[path] = child
			}
			return nil
		}

		// Skip hidden files.
		if strings.HasPrefix(entry.Name(), ".") {
			return nil
		}

		// Handle symlinks.
		if entry.Type()&fs.ModeSymlink != 0 {
			return d.walkSymlink(ctx, path, entry.Name(), rules, &files)
		}

		if rules != nil && rules.Ignored(path, false) {
			return nil
		}

		// Check if file matches criteria.
		if d.matchesWalkedFile(path) {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory %s: %w", root, err)
	}

	return files, nil
}

// skipWalkedDir reports whether a walk must not enter the directory path.
func (d *discoverer) skipWalkedDir(path, name string, rules *ignore.Gitignore) bool {
	return strings.HasPrefix(name, ".") ||
		slices.Contains(d.opts.SkipDirs, name) ||
		d.excludes.Match(path, true) ||
		(rules != nil && rules.Ignored(path, true))
}

// walkSymlink handles a symlink found while walking. A link to a file is
// checked like a file; a link to a directory is walked like a named
// directory when FollowSymlinks is set. Broken links are skipped.
func (d *discoverer) walkSymlink(ctx context.Context, path, name string, rules *ignore.Gitignore, files *[]string) error {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil //nolint:nilerr // Intentionally skip broken symlinks
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return nil //nolint:nilerr // Intentionally skip inaccessible symlink targets
	}

	if !info.IsDir() {
		if (rules == nil || !rules.Ignored(path, false)) && d.matchesWalkedFile(path) {
			*files = append(*files, path)
		}
		return nil
	}

	// Directory symlink: skip unless FollowSymlinks is set.
	if !d.opts.FollowSymlinks || d.skipWalkedDir(path, name, rules) {
		return nil
	}
	// Walk the symlink TARGET (realPath), not the symlink itself.
	// This avoids infinite recursion since WalkDir uses Lstat on root.
	subFiles, err := d.walk(ctx, realPath)
	if err != nil {
		return err
	}
	*files = append(*files, subFiles...)
	return nil
}

// matchesWalkedFile checks a file found while walking. Its directories have
// been checked on the way down.
func (d *discoverer) matchesWalkedFile(path string) bool {
	return hasMatchingExtension(path, d.extensions) &&
		!d.excludes.Match(path, false) &&
		d.included(path)
}

// matchesNamedFile checks a file named by the caller, whose directories no
// walk has checked.
func (d *discoverer) matchesNamedFile(path string) bool {
	return hasMatchingExtension(path, d.extensions) &&
		!d.excludes.MatchWithParents(path, false) &&
		d.included(path)
}

// included applies IncludeGlobs, which admit every file when empty.
func (d *discoverer) included(path string) bool {
	return len(d.opts.IncludeGlobs) == 0 || d.includes.Match(path, false)
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
