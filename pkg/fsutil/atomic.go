package fsutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultFileMode is the default permission mode for newly created files.
const DefaultFileMode os.FileMode = 0644

// chmodBits are the mode bits that os.Chmod applies.
const chmodBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// WriteAtomic replaces the content of the file at path, or creates it.
// If mode is 0, DefaultFileMode (0644) is used.
//
// The write keeps the identity of an existing file:
//   - If path is a symbolic link, the link is resolved and its target is
//     written. The link itself is left alone. A dangling link is an error.
//   - If the file has more than one hard link, it is rewritten in place so
//     every link sees the new content. See below for what that costs.
//   - Otherwise the content goes to a temp file in the target's directory,
//     which is synced, given the original owner (where the platform has
//     owners) and mode, and renamed over the target; then the directory is
//     synced so the rename is durable. The rename is atomic on POSIX: on
//     error the original file is untouched.
//   - If the original owner cannot be restored on the temp file (a
//     non-root user fixing another user's group-writable file), the file is
//     rewritten in place instead, so its owner does not change. If it cannot
//     be written in place either, it is replaced as above and ends up owned
//     by the current user, as with sed -i.
//
// An in-place rewrite is not atomic. If it fails part way, the original
// content is written back on a best-effort basis and the error says whether
// that worked.
func WriteAtomic(ctx context.Context, path string, content []byte, mode os.FileMode) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("write atomic: %w", ctx.Err())
	default:
	}

	if mode == 0 {
		mode = DefaultFileMode
	}

	target, err := resolveWriteTarget(path)
	if err != nil {
		return err
	}

	existing, err := os.Stat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		existing = nil
	case err != nil:
		return fmt.Errorf("stat %s: %w", target, err)
	case existing.IsDir():
		return fmt.Errorf("%w: %s", ErrIsDirectory, target)
	}

	if existing != nil && pathHasMultipleLinks(target) {
		return rewriteInPlace(target, content, mode)
	}
	return replaceFile(target, content, mode, existing)
}

// pathHasMultipleLinks reports whether the file at path has more than one
// hard link. When that cannot be determined it reports true, so the file is
// rewritten in place and keeps its identity.
func pathHasMultipleLinks(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()
	return hasMultipleLinks(file)
}

// syncDir makes a rename in dir durable, on a best-effort basis: a
// directory that cannot be opened (no read permission) or synced (some file
// systems do not support it) is left as it is.
func syncDir(dir string) {
	if !syncDirectories {
		return
	}
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = handle.Sync() //nolint:errcheck // Best effort: the rename stands whether or not the directory can be synced.
	_ = handle.Close()
}

// resolveWriteTarget returns the file that a write to path must change:
// path itself, or the final target when path is a symbolic link.
func resolveWriteTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return path, nil
	}

	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve symlink %s: %w", path, err)
	}
	return target, nil
}

// replaceFile writes content to a temp file next to target and renames it
// over target. existing describes the current target, or is nil when the
// target does not exist yet.
func replaceFile(target string, content []byte, mode os.FileMode, existing os.FileInfo) error {
	dir := filepath.Dir(target)

	// Create temp file in same directory for atomic rename.
	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".tmp.*")
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

	if _, err := tmp.Write(content); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Sync to ensure durability.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	// Restore the owner before the mode: changing the owner can clear the
	// set-user-ID and set-group-ID bits.
	if existing != nil {
		restored, err := restoreOwner(tmp, existing)
		if err != nil {
			return err
		}
		if !restored {
			// Keep the owner by rewriting the original file in place, if we
			// may write it. The deferred cleanup removes the temp file.
			file, err := os.OpenFile(target, os.O_WRONLY|openNoFollow, 0)
			switch {
			case err == nil:
				return rewriteOpenFile(target, file, content, mode)
			case !errors.Is(err, fs.ErrPermission):
				return fmt.Errorf("open %s for an in-place rewrite: %w", target, err)
			}
			// The file is not writable either: replace it as before, which
			// leaves it owned by the current user.
		}
	}

	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	success = true

	// Make the rename itself durable, on a best-effort basis: the new content
	// is already in place, so a directory that cannot be synced is not a
	// failed write.
	syncDir(dir)
	return nil
}

// ownerFile is the part of *os.File that restoring an owner needs.
type ownerFile interface {
	Stat() (os.FileInfo, error)
	Chown(uid, gid int) error
}

// contentWriter is the part of *os.File that rewriting in place needs.
type contentWriter interface {
	WriteAt(p []byte, off int64) (int, error)
	Truncate(size int64) error
	Sync() error
}

// rewriteInPlace overwrites the existing file at target without replacing
// it, which keeps its inode, hard links and owner. On failure it tries to
// put the original content back.
func rewriteInPlace(target string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(target, os.O_WRONLY|openNoFollow, 0)
	if err != nil {
		return fmt.Errorf("open %s for an in-place rewrite: %w", target, err)
	}
	return rewriteOpenFile(target, file, content, mode)
}

// rewriteOpenFile overwrites the content of target through file, which is
// open for writing, and closes it. On failure it tries to put the original
// content back.
func rewriteOpenFile(target string, file *os.File, content []byte, mode os.FileMode) error {
	original, err := os.ReadFile(target)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("read %s before rewriting it in place: %w", target, err)
	}

	if err := overwriteWithRollback(file, content, original); err != nil {
		_ = file.Close()
		return fmt.Errorf("rewrite %s in place: %w", target, err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat %s after rewriting it in place: %w", target, err)
	}
	if info.Mode()&chmodBits != mode&chmodBits {
		if err := file.Chmod(mode); err != nil {
			_ = file.Close()
			return fmt.Errorf("chmod %s after rewriting it in place (the new content is written): %w", target, err)
		}
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", target, err)
	}
	return nil
}

// overwriteWithRollback replaces the content behind file. If that fails, it
// tries to put original back, and the error says whether that worked.
func overwriteWithRollback(file contentWriter, content, original []byte) error {
	err := overwriteFile(file, content)
	if err == nil {
		return nil
	}
	if restoreErr := overwriteFile(file, original); restoreErr != nil {
		return errors.Join(err, fmt.Errorf("restore original content: %w", restoreErr))
	}
	return fmt.Errorf("%w (original content restored)", err)
}

// overwriteFile replaces the whole content of an open file and syncs it.
func overwriteFile(file contentWriter, content []byte) error {
	if _, err := file.WriteAt(content, 0); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := file.Truncate(int64(len(content))); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
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
