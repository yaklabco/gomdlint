package fsutil_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/fsutil"
)

func FuzzWriteAtomic(f *testing.F) {
	// Add seed corpus.
	f.Add([]byte(""))
	f.Add([]byte("hello"))
	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte("line with trailing space  \n"))
	f.Add([]byte("\x00\x01\x02\x03"))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, content []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		ctx := context.Background()
		err := fsutil.WriteAtomic(ctx, path, content, 0644)

		if err != nil {
			// WriteAtomic should not fail for valid paths and content.
			t.Fatalf("WriteAtomic failed: %v", err)
		}

		// Read back and verify.
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		if len(got) != len(content) {
			t.Errorf("content length mismatch: got %d, want %d", len(got), len(content))
		}

		for i := range got {
			if got[i] != content[i] {
				t.Errorf("content mismatch at byte %d: got %d, want %d", i, got[i], content[i])
				break
			}
		}
	})
}

func FuzzReadFileCheckModified(f *testing.F) {
	// Add seed corpus.
	f.Add([]byte("hello"))
	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte(""))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, content []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		// Write initial content.
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		ctx := context.Background()

		// Read file.
		got, info, err := fsutil.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		// Verify content.
		if len(got) != len(content) {
			t.Errorf("content length mismatch: got %d, want %d", len(got), len(content))
		}

		// Check should report not modified.
		modified, err := fsutil.CheckModified(ctx, info)
		if err != nil {
			t.Fatalf("CheckModified failed: %v", err)
		}

		if modified {
			t.Error("file should not be reported as modified")
		}
	})
}
