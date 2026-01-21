package fsutil_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesainslie/gomdlint/pkg/fsutil"
)

func TestReadFile(t *testing.T) {
	t.Parallel()

	t.Run("reads file content and metadata", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		got, info, err := fsutil.ReadFile(ctx, path)

		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("content = %q, want %q", got, content)
		}

		if info.Path != path {
			t.Errorf("Path = %q, want %q", info.Path, path)
		}

		if info.Size != int64(len(content)) {
			t.Errorf("Size = %d, want %d", info.Size, len(content))
		}

		if info.Mode != 0644 {
			t.Errorf("Mode = %o, want %o", info.Mode, 0644)
		}

		// Hash should be non-zero.
		var zeroHash [32]byte
		if info.Hash == zeroHash {
			t.Error("Hash should not be zero")
		}
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		_, _, err := fsutil.ReadFile(ctx, "/nonexistent/path/file.txt")

		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
	})

	t.Run("returns error for directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		ctx := context.Background()
		_, _, err := fsutil.ReadFile(ctx, dir)

		if err == nil {
			t.Fatal("expected error for directory")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := fsutil.ReadFile(ctx, "anypath")

		if err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})
}

func TestCheckModified(t *testing.T) {
	t.Parallel()

	t.Run("returns false for unmodified file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		_, info, err := fsutil.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		modified, err := fsutil.CheckModified(ctx, info)
		if err != nil {
			t.Fatalf("CheckModified() error = %v", err)
		}

		if modified {
			t.Error("expected file to be unmodified")
		}
	})

	t.Run("returns true for content change", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		_, info, err := fsutil.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		// Modify the file.
		newContent := []byte("hello modified")
		if err := os.WriteFile(path, newContent, 0644); err != nil {
			t.Fatalf("modify: %v", err)
		}

		modified, err := fsutil.CheckModified(ctx, info)
		if err != nil {
			t.Fatalf("CheckModified() error = %v", err)
		}

		if !modified {
			t.Error("expected file to be modified")
		}
	})

	t.Run("returns true for deleted file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		_, info, err := fsutil.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		// Delete the file.
		if err := os.Remove(path); err != nil {
			t.Fatalf("delete: %v", err)
		}

		modified, err := fsutil.CheckModified(ctx, info)
		if err != nil {
			t.Fatalf("CheckModified() error = %v", err)
		}

		if !modified {
			t.Error("expected deleted file to be reported as modified")
		}
	})

	t.Run("returns error for nil FileInfo", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		_, err := fsutil.CheckModified(ctx, nil)

		if err == nil {
			t.Fatal("expected error for nil FileInfo")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()

		info := &fsutil.FileInfo{Path: "anypath"}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := fsutil.CheckModified(ctx, info)

		if err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})
}

func TestCheckModifiedQuick(t *testing.T) {
	t.Parallel()

	t.Run("returns false for unmodified file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		_, info, err := fsutil.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		modified, err := fsutil.CheckModifiedQuick(ctx, info)
		if err != nil {
			t.Fatalf("CheckModifiedQuick() error = %v", err)
		}

		if modified {
			t.Error("expected file to be unmodified")
		}
	})

	t.Run("returns true for size change", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		_, info, err := fsutil.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		// Modify file with different size.
		newContent := []byte("hello world extended content")
		if err := os.WriteFile(path, newContent, 0644); err != nil {
			t.Fatalf("modify: %v", err)
		}

		modified, err := fsutil.CheckModifiedQuick(ctx, info)
		if err != nil {
			t.Fatalf("CheckModifiedQuick() error = %v", err)
		}

		if !modified {
			t.Error("expected file to be modified")
		}
	})

	t.Run("returns true for mod time change", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		_, info, err := fsutil.ReadFile(ctx, path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}

		// Change mod time only.
		newTime := info.ModTime.Add(time.Hour)
		if err := os.Chtimes(path, newTime, newTime); err != nil {
			t.Fatalf("chtimes: %v", err)
		}

		modified, err := fsutil.CheckModifiedQuick(ctx, info)
		if err != nil {
			t.Fatalf("CheckModifiedQuick() error = %v", err)
		}

		if !modified {
			t.Error("expected file to be modified")
		}
	})
}
