package fsutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultFileMode is the default permission mode for newly created files.
const DefaultFileMode os.FileMode = 0644

// WriteAtomic writes content to path atomically using a temp file and rename.
// The original file's mode is preserved. If mode is 0, DefaultFileMode (0644) is used.
//
// The atomic write pattern:
//  1. Create a temp file in the same directory as the target.
//  2. Write all content to the temp file.
//  3. Sync the temp file to ensure durability.
//  4. Set the file mode.
//  5. Rename the temp file to the target path (atomic on POSIX).
//
// On error, the temp file is cleaned up and the original file remains untouched.
func WriteAtomic(ctx context.Context, path string, content []byte, mode os.FileMode) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("write atomic: %w", ctx.Err())
	default:
	}

	if mode == 0 {
		mode = DefaultFileMode
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Create temp file in same directory for atomic rename.
	tmp, err := os.CreateTemp(dir, base+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Ensure cleanup on error.
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// Write content.
	if _, err := tmp.Write(content); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Sync to ensure durability.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Set mode before rename.
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	success = true
	return nil
}

// WriteAtomicIfChanged writes content to path atomically only if the content differs.
// Returns true if the file was written, false if it was unchanged.
func WriteAtomicIfChanged(ctx context.Context, path string, content []byte, mode os.FileMode) (bool, error) {
	select {
	case <-ctx.Done():
		return false, fmt.Errorf("write atomic: %w", ctx.Err())
	default:
	}

	// Read existing content.
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, write it.
			if err := WriteAtomic(ctx, path, content, mode); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, fmt.Errorf("read existing: %w", err)
	}

	// Compare content.
	if bytes.Equal(existing, content) {
		return false, nil
	}

	// Content differs, write atomically.
	if err := WriteAtomic(ctx, path, content, mode); err != nil {
		return false, err
	}
	return true, nil
}
