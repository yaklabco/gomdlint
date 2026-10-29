package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileKeyOf_TellsFilesOfOneSizeAndTimeApart covers files that share a
// size and a modification time, as files extracted from an archive often
// do. Where the system gives each file an identity, only the paths of one
// file share a key, so grouping compares a file with no other.
func TestFileKeyOf_TellsFilesOfOneSizeAndTimeApart(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("os.Stat gives no file identity here")
	}

	dir := t.TempDir()
	first, second, link := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md"), filepath.Join(dir, "c.md")
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, path := range []string{first, second} {
		require.NoError(t, os.WriteFile(path, []byte("# Same\n"), 0o644))
		require.NoError(t, os.Chtimes(path, stamp, stamp))
	}
	if err := os.Link(first, link); err != nil {
		t.Skipf("hard links not supported here: %v", err)
	}

	key := func(path string) fileKey {
		info, err := os.Stat(path)
		require.NoError(t, err)
		return fileKeyOf(info)
	}
	assert.NotEqual(t, key(first), key(second), "two files of one size and time")
	assert.Equal(t, key(first), key(link), "two hard links to one file")
	assert.Equal(t, [][]string{{first, link}, {second}}, groupFiles([]string{first, second, link}))
}
