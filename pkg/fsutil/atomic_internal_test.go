package fsutil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDiskFull = errors.New("disk full")

// flakyFile is an in-memory contentWriter whose first failWrites calls to
// WriteAt write only half of their data and then fail.
type flakyFile struct {
	data       []byte
	failWrites int
}

func (f *flakyFile) WriteAt(chunk []byte, off int64) (int, error) {
	if f.failWrites > 0 {
		f.failWrites--
		half := len(chunk) / 2
		f.write(chunk[:half], int(off))
		return half, errDiskFull
	}
	f.write(chunk, int(off))
	return len(chunk), nil
}

func (f *flakyFile) write(p []byte, off int) {
	if end := off + len(p); end > len(f.data) {
		f.data = append(f.data, make([]byte, end-len(f.data))...)
	}
	copy(f.data[off:], p)
}

func (f *flakyFile) Truncate(size int64) error {
	if int(size) <= len(f.data) {
		f.data = f.data[:size]
		return nil
	}
	f.data = append(f.data, make([]byte, int(size)-len(f.data))...)
	return nil
}

func (f *flakyFile) Sync() error { return nil }

func TestOverwriteWithRollback_WritesNewContent(t *testing.T) {
	t.Parallel()

	file := &flakyFile{data: []byte("a much longer original\n")}
	require.NoError(t, overwriteWithRollback(file, []byte("new\n"), file.data))
	assert.Equal(t, "new\n", string(file.data))
}

func TestOverwriteWithRollback_RestoresOriginalAfterAFailedWrite(t *testing.T) {
	t.Parallel()

	original := []byte("original content\n")
	file := &flakyFile{data: append([]byte(nil), original...), failWrites: 1}

	err := overwriteWithRollback(file, []byte("replacement that fails part way\n"), original)

	require.ErrorIs(t, err, errDiskFull)
	assert.Contains(t, err.Error(), "original content restored")
	assert.Equal(t, string(original), string(file.data), "a failed rewrite must put the original back")
}

func TestOverwriteWithRollback_ReportsAFailedRestore(t *testing.T) {
	t.Parallel()

	original := []byte("original content\n")
	file := &flakyFile{data: append([]byte(nil), original...), failWrites: 2}

	err := overwriteWithRollback(file, []byte("replacement\n"), original)

	require.ErrorIs(t, err, errDiskFull)
	assert.Contains(t, err.Error(), "restore original content")
	assert.NotContains(t, err.Error(), "original content restored")
}
