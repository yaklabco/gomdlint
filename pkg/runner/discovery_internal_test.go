package runner

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sameSizeAndTime writes names into a new directory, with equal content and
// modification times, as files extracted from an archive often have.
func sameSizeAndTime(t *testing.T, names ...string) []string {
	t.Helper()

	dir := t.TempDir()
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(paths[i], []byte("# Same\n"), 0o644))
		require.NoError(t, os.Chtimes(paths[i], stamp, stamp))
	}
	return paths
}

// countingSameFile returns os.SameFile and a count of its calls.
func countingSameFile() (func(fs.FileInfo, fs.FileInfo) bool, *int) {
	calls := 0
	return func(a, b fs.FileInfo) bool {
		calls++
		return os.SameFile(a, b)
	}, &calls
}

func TestGroupFiles_GroupsHardLinksAndNothingElse(t *testing.T) {
	t.Parallel()

	paths := sameSizeAndTime(t, "a.md", "b.md")
	link := filepath.Join(filepath.Dir(paths[0]), "c.md")
	if err := os.Link(paths[0], link); err != nil {
		t.Skipf("hard links not supported here: %v", err)
	}

	assert.Equal(t, [][]string{{paths[0], link}, {paths[1]}}, groupFiles([]string{paths[0], paths[1], link}))
}

// TestGroupFiles_ComparesFilesWithAnIdentityOnlyWithTheirOwn covers many
// files of one size and time: where each file has an identity, a file is
// compared with no other file, instead of with every file before it.
func TestGroupFiles_ComparesFilesWithAnIdentityOnlyWithTheirOwn(t *testing.T) {
	t.Parallel()

	names := make([]string, 50)
	for i := range names {
		names[i] = "f" + strconv.Itoa(i) + ".md"
	}
	paths := sameSizeAndTime(t, names...)

	identities := map[string]func(string, fs.FileInfo) (uint64, uint64, bool){
		"identities": func(_ string, info fs.FileInfo) (uint64, uint64, bool) { return fakeID(t, info, paths) },
	}
	if info, err := os.Stat(paths[0]); err != nil {
		t.Fatal(err)
	} else if _, _, ok := fileID(paths[0], info); ok {
		identities["the system's identities"] = fileID
	}
	for name, identify := range identities {
		sameFile, calls := countingSameFile()
		assert.Len(t, groupFilesBy(paths, identify, sameFile), len(paths), name)
		assert.Zero(t, *calls, "%s: distinct files with identities are never compared", name)
	}

	none := func(string, fs.FileInfo) (uint64, uint64, bool) { return 0, 0, false }
	sameFile, calls := countingSameFile()
	assert.Len(t, groupFilesBy(paths, none, sameFile), len(paths))
	assert.Positive(t, *calls, "without identities files are compared")
}

// TestGroupFiles_KeepsAliasesTogetherWhenIdentitiesFail covers paths whose
// identity cannot be read, and files that report one identity though they
// differ: the paths of one file still form one group, so that one worker
// processes them in turn.
func TestGroupFiles_KeepsAliasesTogetherWhenIdentitiesFail(t *testing.T) {
	t.Parallel()

	paths := sameSizeAndTime(t, "a.md", "b.md")
	first, other := paths[0], paths[1]
	link := filepath.Join(filepath.Dir(first), "c.md")
	if err := os.Link(first, link); err != nil {
		t.Skipf("hard links not supported here: %v", err)
	}
	all := []string{first, other, link}

	tests := []struct {
		name     string
		identify func(string, fs.FileInfo) (uint64, uint64, bool)
	}{
		{name: "the first path has none", identify: missingFor(t, all, first)},
		{name: "the other file has none", identify: missingFor(t, all, other)},
		{name: "the later path has none", identify: missingFor(t, all, link)},
		{name: "both paths of one file have none", identify: missingFor(t, all, first, link)},
		{name: "no path has one", identify: missingFor(t, all, all...)},
		{name: "every path has the same one", identify: func(string, fs.FileInfo) (uint64, uint64, bool) {
			return 7, 7, true
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, [][]string{{first, link}, {other}}, groupFilesBy(all, tt.identify, os.SameFile))
		})
	}
}

// missingFor returns an identify function that reads no identity for the
// missing paths and a fakeID among paths for the others.
func missingFor(t *testing.T, paths []string, missing ...string) func(string, fs.FileInfo) (uint64, uint64, bool) {
	t.Helper()

	return func(path string, info fs.FileInfo) (uint64, uint64, bool) {
		for _, name := range missing {
			if path == name {
				return 0, 0, false
			}
		}
		return fakeID(t, info, paths)
	}
}

// fakeID numbers the file that info describes by the first of paths that
// names it, so the paths of one file share an identity on any system.
func fakeID(t *testing.T, info fs.FileInfo, paths []string) (uint64, uint64, bool) {
	t.Helper()

	for i, path := range paths {
		other, err := os.Stat(path)
		require.NoError(t, err)
		if os.SameFile(info, other) {
			return 1, uint64(i), true
		}
	}
	return 0, 0, false
}
