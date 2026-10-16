package display

import (
	"path/filepath"
	"strings"
)

// Path returns path in the form reporters show it: relative to workDir when
// path lies inside workDir, and unchanged otherwise. Relative paths and paths
// reported with an empty workDir are returned unchanged.
//
// When path is not lexically inside workDir, Path resolves symbolic links in
// workDir and in the directory holding path and compares again, so two
// spellings of the same directory (for example a macOS /Volumes mount and its
// home-directory alias) still produce a relative path. The file name itself
// is never resolved, so a symlinked file keeps the name it was linted by.
func Path(path, workDir string) string {
	if workDir == "" || !filepath.IsAbs(path) {
		return path
	}

	base, err := filepath.Abs(workDir)
	if err != nil {
		return path
	}

	if rel, ok := relativeInside(base, path); ok {
		return rel
	}

	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return path
	}

	resolvedDir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return path
	}

	if rel, ok := relativeInside(resolvedBase, filepath.Join(resolvedDir, filepath.Base(path))); ok {
		return rel
	}

	return path
}

// relativeInside returns target relative to base when target lies strictly
// inside base.
func relativeInside(base, target string) (string, bool) {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return rel, true
}
