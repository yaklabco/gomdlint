// Package ignore decides which paths a lint run skips. It matches gomdlint's
// ignore patterns and the patterns of .gitignore files, both segment by
// segment, so "**" spans whole path segments and "*" never crosses a slash.
package ignore

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// doubleStar is the pattern segment that matches any number of segments.
const doubleStar = "**"

// pattern is a compiled glob pattern.
type pattern struct {
	segments []string
	dirOnly  bool
}

// matches reports whether the path segments match the pattern.
func (p pattern) matches(segments []string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}
	return matchSegments(p.segments, segments)
}

// matchSegments matches path segments against pattern segments. A "**"
// pattern segment matches zero or more path segments; any other pattern
// segment matches one path segment with path.Match.
func matchSegments(patternSegs, pathSegs []string) bool {
	for len(patternSegs) > 0 {
		if patternSegs[0] == doubleStar {
			rest := patternSegs[1:]
			for len(rest) > 0 && rest[0] == doubleStar {
				rest = rest[1:]
			}
			for skip := 0; skip <= len(pathSegs); skip++ {
				if matchSegments(rest, pathSegs[skip:]) {
					return true
				}
			}
			return false
		}

		if len(pathSegs) == 0 {
			return false
		}
		if ok, err := path.Match(patternSegs[0], pathSegs[0]); err != nil || !ok {
			return false
		}
		patternSegs, pathSegs = patternSegs[1:], pathSegs[1:]
	}
	return len(pathSegs) == 0
}

// validateSegments reports a malformed segment, such as an unclosed "[".
func validateSegments(segments []string) error {
	for _, segment := range segments {
		if _, err := path.Match(segment, ""); err != nil {
			return fmt.Errorf("segment %q: %w", segment, err)
		}
	}
	return nil
}

// relSegments returns the segments of target relative to base, and false
// when target lies outside base. The base directory itself has no segments.
func relSegments(base, target string) ([]string, bool) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return nil, false
	}
	rel = filepath.ToSlash(rel)
	switch {
	case rel == ".":
		return nil, true
	case rel == ".." || strings.HasPrefix(rel, "../"):
		return nil, false
	default:
		return strings.Split(rel, "/"), true
	}
}

// absSegments returns the segments of an absolute path.
func absSegments(abs string) []string {
	return strings.Split(strings.TrimPrefix(filepath.ToSlash(abs), "/"), "/")
}

// globPattern pairs a compiled pattern with how its paths are measured.
type globPattern struct {
	pattern
	absolute bool
}

// compileGlob compiles one of gomdlint's ignore patterns.
func compileGlob(raw string) (globPattern, error) {
	text := filepath.ToSlash(raw)
	absolute := filepath.IsAbs(raw) || strings.HasPrefix(text, "/")

	var compiled globPattern
	compiled.absolute = absolute
	if strings.HasSuffix(text, "/") {
		compiled.dirOnly = true
		text = strings.TrimRight(text, "/")
	}
	if absolute {
		text = strings.TrimPrefix(text, "/")
	} else {
		text = strings.TrimPrefix(text, "./")
	}
	if text == "" {
		return globPattern{}, fmt.Errorf("ignore pattern %q matches nothing", raw)
	}

	compiled.segments = strings.Split(text, "/")
	if !absolute && !strings.Contains(text, "/") {
		// Like .gitignore, a pattern without a slash matches at any depth.
		compiled.segments = append([]string{doubleStar}, compiled.segments...)
	}
	if err := validateSegments(compiled.segments); err != nil {
		return globPattern{}, fmt.Errorf("ignore pattern %q: %w", raw, err)
	}
	return compiled, nil
}

// ValidatePattern reports whether raw is a well-formed ignore pattern.
func ValidatePattern(raw string) error {
	_, err := compileGlob(raw)
	return err
}

// Set is a list of gomdlint ignore patterns resolved against a base
// directory.
//
// Patterns use forward slashes. "*", "?" and "[...]" match within one path
// segment, and a "**" segment matches any number of segments, including
// none, so "vendor/**" matches the vendor directory itself. A pattern
// without a slash matches a name at any depth, a trailing slash matches
// directories only, and a leading "./" stands for the base directory.
// Relative patterns match paths inside the base directory; absolute
// patterns match absolute paths.
type Set struct {
	base     string
	patterns []globPattern
}

// NewSet compiles patterns relative to base, an absolute directory.
func NewSet(base string, patterns []string) (*Set, error) {
	set := &Set{base: base, patterns: make([]globPattern, 0, len(patterns))}
	for _, raw := range patterns {
		compiled, err := compileGlob(raw)
		if err != nil {
			return nil, err
		}
		set.patterns = append(set.patterns, compiled)
	}
	return set, nil
}

// Match reports whether the absolute path matches a pattern by its own
// path. Walks use it for each entry, having pruned ignored directories.
func (s *Set) Match(target string, isDir bool) bool {
	if s == nil || len(s.patterns) == 0 {
		return false
	}
	rel, inside := relSegments(s.base, target)
	abs := absSegments(target)
	for _, compiled := range s.patterns {
		switch {
		case compiled.absolute:
			if compiled.matches(abs, isDir) {
				return true
			}
		case inside && len(rel) > 0:
			if compiled.matches(rel, isDir) {
				return true
			}
		}
	}
	return false
}

// MatchWithParents reports whether the absolute path, or a directory
// containing it, matches a pattern. Use it for paths named on the command
// line, whose parent directories no walk has checked.
func (s *Set) MatchWithParents(target string, isDir bool) bool {
	if s == nil || len(s.patterns) == 0 {
		return false
	}
	for dir := filepath.Dir(target); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if s.Match(dir, true) {
			return true
		}
	}
	return s.Match(target, isDir)
}
