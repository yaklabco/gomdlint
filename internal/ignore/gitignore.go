package ignore

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// gitignoreFile is the name of the files whose patterns Gitignore applies.
const gitignoreFile = ".gitignore"

// utf8BOM is the byte order mark some editors write at the start of a file.
// Git skips it before reading the first pattern.
const utf8BOM = "\xef\xbb\xbf"

// gitRule is one pattern of a .gitignore file.
type gitRule struct {
	pattern

	// base is the directory holding the .gitignore file. Patterns apply to
	// paths below it.
	base string

	// negate re-includes paths that earlier patterns ignored.
	negate bool
}

// Gitignore holds the .gitignore patterns in effect in one directory, in
// precedence order: patterns from deeper directories and later lines win.
//
// It follows the gitignore format: blank lines and "#" comments are skipped,
// "!" negates, a trailing "/" matches directories only, a pattern with a
// slash other than a trailing one is anchored to its file's directory,
// "**" matches across directories, and backslash escapes a special
// character. Patterns match with Git's wildmatch rules, as described on
// Set. As in Git, a path inside an ignored directory cannot be re-included,
// because walks never enter ignored directories.
//
// One quirk of Git's matcher is not copied. When the first wildcard of an
// anchored pattern is a run of stars after literal text, as in "ab**/c",
// Git lets the run cross directories and even match "abc". Here such a run
// matches like "*", as the gitignore documentation describes.
//
// The global excludes file and .git/info/exclude are not read.
type Gitignore struct {
	rules []gitRule
}

// GitignoreFor returns the .gitignore patterns in effect inside dir: those
// of the .gitignore files from the root of the enclosing Git working tree
// down to dir itself. Outside a working tree only dir's own file applies.
func GitignoreFor(dir string) (*Gitignore, error) {
	dirs := []string{dir}
	if root, ok := workTreeRoot(dir); ok {
		dirs = dirs[:0]
		for current := dir; ; current = filepath.Dir(current) {
			dirs = append(dirs, current)
			if current == root || current == filepath.Dir(current) {
				break
			}
		}
	}

	rules := &Gitignore{}
	for idx := len(dirs) - 1; idx >= 0; idx-- {
		next, err := rules.Child(dirs[idx])
		if err != nil {
			return nil, err
		}
		rules = next
	}
	return rules, nil
}

// workTreeRoot returns the nearest directory at or above dir that holds a
// .git entry. Worktrees and submodules have a .git file, not a directory.
func workTreeRoot(dir string) (string, bool) {
	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current, true
		}
		if current == filepath.Dir(current) {
			return "", false
		}
	}
}

// Child returns the patterns in effect inside dir, a subdirectory of the
// directory g describes: g's patterns followed by those of dir's .gitignore.
// It returns g itself when dir has no .gitignore.
func (g *Gitignore) Child(dir string) (*Gitignore, error) {
	content, err := os.ReadFile(filepath.Join(dir, gitignoreFile))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return g, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Join(dir, gitignoreFile), err)
	}

	added := parseGitignore(dir, content)
	if len(added) == 0 {
		return g, nil
	}
	rules := make([]gitRule, 0, len(g.rules)+len(added))
	rules = append(rules, g.rules...)
	rules = append(rules, added...)
	return &Gitignore{rules: rules}, nil
}

// Ignored reports whether the absolute path is ignored by its own path.
// Walks call it for each entry after pruning ignored directories.
func (g *Gitignore) Ignored(target string, isDir bool) bool {
	ignored := false
	for _, rule := range g.rules {
		rel, inside := relSegments(rule.base, target)
		if !inside || len(rel) == 0 {
			continue
		}
		if rule.matches(rel, isDir) {
			ignored = !rule.negate
		}
	}
	return ignored
}

// parseGitignore parses the content of the .gitignore file in dir.
// Malformed patterns are skipped, as Git skips them.
func parseGitignore(dir string, content []byte) []gitRule {
	var rules []gitRule
	content = bytes.TrimPrefix(content, []byte(utf8BOM))
	for _, line := range bytes.Split(content, []byte("\n")) {
		rule, ok := parseGitignoreLine(dir, strings.TrimSuffix(string(line), "\r"))
		if ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

// parseGitignoreLine parses one .gitignore line.
func parseGitignoreLine(dir, line string) (gitRule, bool) {
	line = trimTrailingSpaces(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return gitRule{}, false
	}

	rule := gitRule{base: dir}
	if strings.HasPrefix(line, "!") {
		rule.negate = true
		line = line[1:]
	}
	// Only one trailing slash marks a directory pattern: Git keeps any
	// other, which anchors the pattern and leaves an empty last segment.
	if strings.HasSuffix(line, "/") {
		rule.dirOnly = true
		line = line[:len(line)-1]
	}
	if line == "" {
		return gitRule{}, false
	}

	// A slash at the start or in the middle anchors the pattern to dir.
	anchored := strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")
	segments := splitPattern(line)
	if !anchored {
		segments = append([]string{doubleStar}, segments...)
	}
	// A trailing "/**" matches everything inside a directory but not the
	// directory itself, so a later pattern can re-include its contents.
	if len(segments) > 1 && isDoubleStar(segments[len(segments)-1]) {
		segments = append(segments[:len(segments)-1], "*", doubleStar)
	}
	if validateSegments(segments) != nil {
		return gitRule{}, false
	}

	rule.segments = segments
	return rule, true
}

// trimTrailingSpaces removes trailing spaces unless a backslash escapes
// them. Like Git, it reads escapes from the left, so in "a\\ " the
// backslashes escape each other and the space goes.
func trimTrailingSpaces(line string) string {
	spaces := -1 // where the run of trailing spaces starts
	for idx := 0; idx < len(line); idx++ {
		switch line[idx] {
		case ' ':
			if spaces < 0 {
				spaces = idx
			}
		case '\\':
			idx++
			spaces = -1
		default:
			spaces = -1
		}
	}
	if spaces < 0 {
		return line
	}
	return line[:spaces]
}
