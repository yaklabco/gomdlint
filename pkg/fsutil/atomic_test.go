package fsutil_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/fsutil"
)

func TestWriteAtomic(t *testing.T) {
	t.Parallel()

	t.Run("writes new file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		ctx := context.Background()
		err := fsutil.WriteAtomic(ctx, path, content, 0644)

		if err != nil {
			t.Fatalf("WriteAtomic() error = %v", err)
		}

		// Verify content.
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("content = %q, want %q", got, content)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		// Create initial file.
		if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		content := []byte("new content")
		ctx := context.Background()
		err := fsutil.WriteAtomic(ctx, path, content, 0644)

		if err != nil {
			t.Fatalf("WriteAtomic() error = %v", err)
		}

		// Verify content.
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("content = %q, want %q", got, content)
		}
	})

	t.Run("preserves specified mode", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		ctx := context.Background()
		err := fsutil.WriteAtomic(ctx, path, content, 0600)

		if err != nil {
			t.Fatalf("WriteAtomic() error = %v", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}

		// Check mode (mask off type bits).
		gotMode := stat.Mode().Perm()
		if gotMode != 0600 {
			t.Errorf("mode = %o, want %o", gotMode, 0600)
		}
	})

	t.Run("uses default mode when zero", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		ctx := context.Background()
		err := fsutil.WriteAtomic(ctx, path, content, 0)

		if err != nil {
			t.Fatalf("WriteAtomic() error = %v", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}

		gotMode := stat.Mode().Perm()
		if gotMode != fsutil.DefaultFileMode {
			t.Errorf("mode = %o, want %o", gotMode, fsutil.DefaultFileMode)
		}
	})

	t.Run("writes empty content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte{}

		ctx := context.Background()
		err := fsutil.WriteAtomic(ctx, path, content, 0644)

		if err != nil {
			t.Fatalf("WriteAtomic() error = %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}

		if len(got) != 0 {
			t.Errorf("expected empty content, got %d bytes", len(got))
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := fsutil.WriteAtomic(ctx, path, []byte("content"), 0644)

		if err == nil {
			t.Fatal("expected error for cancelled context")
		}

		// File should not exist.
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("file should not have been created")
		}
	})

	t.Run("cleans up temp file on error", func(t *testing.T) {
		t.Parallel()

		// Write to a path where we can't rename (non-existent parent directory).
		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent", "test.txt")

		ctx := context.Background()
		err := fsutil.WriteAtomic(ctx, path, []byte("content"), 0644)

		if err == nil {
			t.Fatal("expected error for invalid path")
		}

		// Verify no temp files left behind.
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("readdir: %v", err)
		}

		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".tmp" {
				t.Errorf("temp file left behind: %s", entry.Name())
			}
		}
	})
}

func TestWriteAtomicIfChanged(t *testing.T) {
	t.Parallel()

	t.Run("writes new file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		ctx := context.Background()
		changed, err := fsutil.WriteAtomicIfChanged(ctx, path, content, 0644)

		if err != nil {
			t.Fatalf("WriteAtomicIfChanged() error = %v", err)
		}

		if !changed {
			t.Error("expected changed = true for new file")
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("content = %q, want %q", got, content)
		}
	})

	t.Run("skips unchanged content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		// Create initial file.
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		ctx := context.Background()
		changed, err := fsutil.WriteAtomicIfChanged(ctx, path, content, 0644)

		if err != nil {
			t.Fatalf("WriteAtomicIfChanged() error = %v", err)
		}

		if changed {
			t.Error("expected changed = false for unchanged content")
		}
	})

	t.Run("writes changed content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		// Create initial file.
		if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		newContent := []byte("new content")
		ctx := context.Background()
		changed, err := fsutil.WriteAtomicIfChanged(ctx, path, newContent, 0644)

		if err != nil {
			t.Fatalf("WriteAtomicIfChanged() error = %v", err)
		}

		if !changed {
			t.Error("expected changed = true for different content")
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}

		if string(got) != string(newContent) {
			t.Errorf("content = %q, want %q", got, newContent)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := fsutil.WriteAtomicIfChanged(ctx, path, []byte("content"), 0644)

		if err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})
}
