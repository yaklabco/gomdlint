package lint

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/fsutil"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// stubParser returns a minimal snapshot of the content.
type stubParser struct{}

func (stubParser) Parse(_ context.Context, path string, content []byte) (*mdast.FileSnapshot, error) {
	return &mdast.FileSnapshot{
		Path:    path,
		Content: content,
		Lines:   mdast.BuildLines(content),
		Tokens:  []mdast.Token{{Kind: mdast.TokText, StartOffset: 0, EndOffset: len(content)}},
		Root:    mdast.NewNode(mdast.NodeDocument),
	}, nil
}

// xToYRule fixes the first "x" to "y".
type xToYRule struct {
	BaseRule
}

func (r *xToYRule) Apply(ctx *RuleContext) ([]Diagnostic, error) {
	idx := bytes.IndexByte(ctx.File.Content, 'x')
	if idx < 0 {
		return nil, nil
	}
	return []Diagnostic{{
		RuleID: "TEST001", Message: "x", StartLine: 1, StartColumn: idx + 1,
		FixEdits: []fix.TextEdit{{StartOffset: idx, EndOffset: idx + 1, NewText: "y"}},
	}}, nil
}

// processWithHook runs a fixing pipeline on path, with beforeWrite run after
// every check and right before the write.
func processWithHook(t *testing.T, path string, roots []string, beforeWrite func()) *PipelineResult {
	t.Helper()
	return processWithOptions(t, path, PipelineOptions{Fix: true, WriteRoots: NewWriteRoots("", roots)}, beforeWrite)
}

func processWithOptions(t *testing.T, path string, opts PipelineOptions, beforeWrite func()) *PipelineResult {
	t.Helper()

	registry := NewRegistry()
	registry.Register(&xToYRule{BaseRule: NewBaseRule("TEST001", "x-to-y", "", nil, true)})
	pipeline := NewPipeline(NewEngine(stubParser{}, registry))
	pipeline.beforeWrite = beforeWrite

	cfg := config.NewConfig()
	cfg.Fix = true
	result, err := pipeline.ProcessFile(context.Background(), path, cfg, opts)
	require.NoError(t, err)
	return result
}

func withBackups(roots ...string) PipelineOptions {
	return PipelineOptions{
		Fix:        true,
		WriteRoots: NewWriteRoots("", roots),
		Backup:     fsutil.BackupConfig{Enabled: true, Mode: fsutil.BackupModeSidecar},
	}
}

func TestPipeline_KeepsContentSavedIntoTheFileBeforeTheWrite(t *testing.T) {
	t.Parallel()

	for _, linked := range []bool{false, true} {
		name := "ordinary file"
		if linked {
			name = "hard-linked file"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, "doc.md")
			require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))
			if linked {
				require.NoError(t, os.Link(path, filepath.Join(root, "twin.md")))
			}

			// Another program saves into the same file after the checks.
			result := processWithOptions(t, path, withBackups(root), func() {
				require.NoError(t, os.WriteFile(path, []byte("concurrent x\n"), 0o644))
			})

			requireRefused(t, result)
			assert.Equal(t, "concurrent x\n", readString(t, path), "a concurrent save must not be lost")
			assert.NoFileExists(t, path+fsutil.BackupSuffix, "no backup of content that will not be replaced")
		})
	}
}

func TestPipeline_NeverBacksUpIntoAReplacedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, dir := range []string{"docs", "other"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "doc.md"), []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other", "doc.md"), []byte("other x\n"), 0o644))

	result := processWithOptions(t, filepath.Join(root, "docs", "doc.md"), withBackups(root), func() {
		require.NoError(t, os.Rename(filepath.Join(root, "docs"), filepath.Join(root, "docs.aside")))
		if err := os.Symlink("other", filepath.Join(root, "docs")); err != nil {
			t.Skipf("symlinks not supported here: %v", err)
		}
	})

	requireRefused(t, result)
	assert.NoFileExists(t, filepath.Join(root, "other", "doc.md"+fsutil.BackupSuffix),
		"a backup must never land next to another file")
	assert.Equal(t, "other x\n", readString(t, filepath.Join(root, "other", "doc.md")))
}

func readString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// requireRefused checks that a write was refused and that the result
// describes the unfixed file.
func requireRefused(t *testing.T, result *PipelineResult) {
	t.Helper()
	assert.True(t, result.Skipped)
	assert.False(t, result.Written)
	assert.Zero(t, result.TotalEditsApplied)
	assert.Zero(t, result.TotalFixesApplied)
	require.Len(t, result.Diagnostics, 1, "the unfixed finding is reported")
}

func TestPipeline_RefusesAParentDirectoryReplacedBeforeTheWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "doc.md"), []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "doc.md"), []byte("outside x\n"), 0o644))

	result := processWithHook(t, filepath.Join(root, "docs", "doc.md"), []string{root}, func() {
		require.NoError(t, os.Rename(filepath.Join(root, "docs"), filepath.Join(root, "docs.aside")))
		if err := os.Symlink(outside, filepath.Join(root, "docs")); err != nil {
			t.Skipf("symlinks not supported here: %v", err)
		}
	})

	requireRefused(t, result)
	assert.Equal(t, "outside x\n", readString(t, filepath.Join(outside, "doc.md")),
		"a replaced parent directory must not redirect the write")
	assert.Equal(t, "x\n", readString(t, filepath.Join(root, "docs.aside", "doc.md")))
}

func TestPipeline_RefusesAFileReplacedByAHardLinkBeforeTheWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	victim := filepath.Join(t.TempDir(), "victim.md")
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(victim, []byte("victim x\n"), 0o644))

	result := processWithHook(t, path, []string{root}, func() {
		require.NoError(t, os.Remove(path))
		if err := os.Link(victim, path); err != nil {
			t.Skipf("hard links not supported here: %v", err)
		}
	})

	requireRefused(t, result)
	assert.Equal(t, "victim x\n", readString(t, victim), "a file linked in after the checks must not be rewritten")
}

func TestPipeline_KeepsAFileWrittenConcurrentlyBeforeTheWrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))

	result := processWithHook(t, path, []string{root}, func() {
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.WriteFile(path, []byte("concurrent x\n"), 0o644))
	})

	requireRefused(t, result)
	assert.Equal(t, "concurrent x\n", readString(t, path), "another writer's file must not be replaced")
}

func TestPipeline_WritesThePinnedFileWhenALinkMovesBeforeTheWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outside := t.TempDir()
	inside := filepath.Join(dir, "inside.md")
	victim := filepath.Join(outside, "victim.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(inside, []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(victim, []byte("unrelated x\n"), 0o644))
	if err := os.Symlink(inside, link); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}

	registry := NewRegistry()
	registry.Register(&xToYRule{BaseRule: NewBaseRule("TEST001", "x-to-y", "", nil, true)})
	pipeline := NewPipeline(NewEngine(stubParser{}, registry))
	// Point the link outside after every check, right before the write.
	pipeline.beforeWrite = func() {
		require.NoError(t, os.Remove(link))
		require.NoError(t, os.Symlink(victim, link))
	}

	cfg := config.NewConfig()
	cfg.Fix = true
	result, err := pipeline.ProcessFile(context.Background(), link, cfg,
		PipelineOptions{Fix: true, WriteRoots: NewWriteRoots("", []string{dir})})
	require.NoError(t, err)

	got, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "unrelated x\n", string(got), "the write must not follow the moved link")
	got, err = os.ReadFile(inside)
	require.NoError(t, err)
	assert.Equal(t, "y\n", string(got), "the file that was read and checked is the one fixed")
	assert.True(t, result.Written)
}

func TestNewWriteRoots_KeepsTheRootsItWasGiven(t *testing.T) {
	t.Parallel()

	// A link in linked points at a file in target. The roots of linked never
	// let a fix through the link, even after the caller reuses the slice the
	// roots of target came from.
	target := resolvePath(t.TempDir())
	linked := resolvePath(t.TempDir())
	outside := filepath.Join(target, "outside.md")

	paths := []string{target}
	roots := NewWriteRoots("", paths)
	paths[0] = linked
	_, _, inside := roots.find(outside)
	assert.True(t, inside, "the roots keep the paths they were made from")

	_, _, inside = NewWriteRoots("", paths).find(outside)
	assert.False(t, inside, "the roots of another directory do not hold the file")
}

func TestWriteRoots_Find(t *testing.T) {
	t.Parallel()

	dir := resolvePath(t.TempDir())
	docs := filepath.Join(dir, "docs")
	guide := filepath.Join(docs, "guide")
	require.NoError(t, os.MkdirAll(guide, 0o755))
	file := filepath.Join(guide, "a.md")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "docs2"), 0o755))
	link := filepath.Join(dir, "link")
	if err := os.Symlink(docs, link); err != nil {
		t.Skipf("symlinks not supported here: %v", err)
	}
	top := filepath.VolumeName(dir) + string(filepath.Separator)
	relFromTop, err := filepath.Rel(top, file)
	require.NoError(t, err)

	tests := []struct {
		name       string
		roots      []string
		wantDir    string
		wantName   string
		wantInside bool
	}{
		{name: "no roots", roots: nil, wantDir: guide, wantName: "a.md", wantInside: true},
		{name: "a directory root", roots: []string{docs}, wantDir: docs,
			wantName: filepath.Join("guide", "a.md"), wantInside: true},
		{name: "the first root that holds the file", roots: []string{dir, guide}, wantDir: dir,
			wantName: filepath.Join("docs", "guide", "a.md"), wantInside: true},
		{name: "a repeated root", roots: []string{guide, dir, guide}, wantDir: guide, wantName: "a.md", wantInside: true},
		{name: "the file as its root", roots: []string{file, dir}, wantDir: guide, wantName: "a.md", wantInside: true},
		{name: "a link to a directory", roots: []string{link}, wantDir: docs,
			wantName: filepath.Join("guide", "a.md"), wantInside: true},
		{name: "the top directory", roots: []string{top}, wantDir: top, wantName: relFromTop, wantInside: true},
		{name: "a sibling that shares a prefix", roots: []string{filepath.Join(dir, "docs2")}, wantDir: guide, wantName: "a.md"},
		{name: "a root that does not exist", roots: []string{filepath.Join(dir, "other")}, wantDir: guide, wantName: "a.md"},
		{name: "a missing root around the file", roots: []string{filepath.Join(guide, "a.md", "b")}, wantDir: guide,
			wantName: "a.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rootDir, name, inside := newWriteRoots(tt.roots, false).find(file)
			assert.Equal(t, tt.wantDir, rootDir)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantInside, inside)
		})
	}
}

func TestWriteRoots_FindFoldsCaseWhenAsked(t *testing.T) {
	t.Parallel()

	// Roots that do not exist hold the file without being its directory, so
	// the answer is the same on every system.
	base := resolvePath(t.TempDir())
	target := filepath.Join(base, "docs", "a.md")
	roots := []string{filepath.Join(base, "Other"), filepath.Join(base, "DOCS")}

	_, _, inside := newWriteRoots(roots, true).find(target)
	assert.True(t, inside, "DOCS holds docs/a.md when case is folded")
	_, _, inside = newWriteRoots(roots, false).find(target)
	assert.False(t, inside, "DOCS does not hold docs/a.md when case counts")
}

func TestWriteRoots_FindUnderAUNCShareRoot(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("UNC paths exist only on Windows")
	}

	_, _, inside := newWriteRoots([]string{`\\host\share`}, true).find(`\\host\share\a.md`)
	assert.True(t, inside, `\\host\share holds \\host\share\a.md`)
}

func TestRootKey_MatchesWhatFilepathRelComparesOnWindows(t *testing.T) {
	t.Parallel()

	names := []string{"docs", "DOCS", "Docs", "dOcS", "doc", "kelvin", "\u212aelvin", "KELVIN",
		"ſtraße", "STRASSE", "Straße", "ǅ", "ǆ", "Ǆ", "\xff", "\xfe", "\ufffd"}
	for _, a := range names {
		for _, b := range names {
			assert.Equal(t, strings.EqualFold(a, b), rootKey(a, true) == rootKey(b, true), "%q and %q", a, b)
			assert.Equal(t, a == b, rootKey(a, false) == rootKey(b, false), "%q and %q", a, b)
		}
	}
}

// TestWriteRoots_FindDoesNotGrowWithTheRoots guards the lookup of a
// file's write root. Every file named on the command line is a root, so a
// lookup that scans the roots makes a run quadratic in the number of paths.
// The test times the same lookups among many roots and among eight times as
// many; a scan takes about eight times as long for the larger set. The
// fastest of several rounds counts, so a busy machine rarely matters, and
// the test does not run in parallel with others.
func TestWriteRoots_FindDoesNotGrowWithTheRoots(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}

	const (
		small    = 500
		factor   = 8
		lookups  = 20000
		rounds   = 5
		maxRatio = 3
	)
	base := resolvePath(t.TempDir())
	paths := func(n int) []string {
		roots := make([]string, n)
		for i := range roots {
			roots[i] = filepath.Join(base, "docs", "f"+strconv.Itoa(i)+".md")
		}
		return roots
	}
	timeFinds := func(set *WriteRoots, targets []string) time.Duration {
		start := time.Now()
		for i := range lookups {
			if _, _, inside := set.find(targets[i%len(targets)]); !inside {
				t.Fatalf("%s must lie inside its own root", targets[i%len(targets)])
			}
		}
		return time.Since(start)
	}

	few, many := paths(small), paths(factor*small)
	fewSet, manySet := newWriteRoots(few, false), newWriteRoots(many, false)
	targets := many[factor*small-small:]
	bestFew, bestMany := time.Duration(1<<63-1), time.Duration(1<<63-1)
	for range rounds {
		bestFew = min(bestFew, timeFinds(fewSet, few))
		bestMany = min(bestMany, timeFinds(manySet, targets))
	}
	ratio := float64(bestMany) / float64(bestFew)
	t.Logf("%d lookups among %d roots in %v, among %d in %v, ratio %.2f",
		lookups, small, bestFew, factor*small, bestMany, ratio)
	require.Less(t, ratio, float64(maxRatio), "finding a write root grows with the number of roots")
}
