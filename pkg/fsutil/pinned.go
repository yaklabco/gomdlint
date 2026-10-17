package fsutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// maxTempAttempts bounds the attempts to find an unused temp file name.
const maxTempAttempts = 16

// PinnedFile is a regular file read for fixing. It keeps the directory the
// file was opened through as an os.Root, which confines every later
// operation to that directory and refuses symbolic links that lead out of
// it, even if a directory on the way is replaced meanwhile. It also keeps
// the file it read open, so that file's identity cannot be reused by
// another file: Check and Write fail with ErrFileChanged when the name no
// longer refers to that same file.
//
// A PinnedFile must be closed.
type PinnedFile struct {
	root    *os.Root
	held    *os.File
	dir     string
	name    string
	info    os.FileInfo
	content []byte
	hash    [sha256.Size]byte
}

// Pin opens the regular file name, relative to the directory dir, and reads
// it. Symbolic links in name are followed only while they stay inside dir.
func Pin(dir, name string) (*PinnedFile, error) {
	path := filepath.Join(dir, name)

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, describeOpenError(path, err)
	}

	file, err := root.Open(name)
	if err != nil {
		_ = root.Close()
		return nil, describeOpenError(path, err)
	}
	fail := func(err error) (*PinnedFile, error) {
		_ = file.Close()
		_ = root.Close()
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		return fail(fmt.Errorf("stat %s: %w", path, err))
	}
	if info.IsDir() {
		return fail(fmt.Errorf("%w: %s", ErrIsDirectory, path))
	}
	if !info.Mode().IsRegular() {
		return fail(fmt.Errorf("%s is not a regular file", path))
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return fail(describeOpenError(path, err))
	}

	return &PinnedFile{
		root:    root,
		held:    file,
		dir:     dir,
		name:    name,
		info:    info,
		content: content,
		hash:    sha256.Sum256(content),
	}, nil
}

// describeOpenError wraps an error from opening or reading path with the
// matching sentinel error.
func describeOpenError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%w: %s: %w", ErrNotFound, path, err)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w: %s: %w", ErrPermissionDenied, path, err)
	default:
		return fmt.Errorf("open %s: %w", path, err)
	}
}

// Content returns the content that was read.
func (p *PinnedFile) Content() []byte {
	return p.content
}

// Info describes the file as it was read.
func (p *PinnedFile) Info() *FileInfo {
	return &FileInfo{
		Path:    filepath.Join(p.dir, p.name),
		Mode:    p.info.Mode(),
		ModTime: p.info.ModTime(),
		Size:    p.info.Size(),
		Hash:    p.hash,
	}
}

// Close releases the file and directory handles.
func (p *PinnedFile) Close() error {
	p.release()
	if err := p.root.Close(); err != nil {
		return fmt.Errorf("close %s: %w", p.dir, err)
	}
	return nil
}

// release closes the file handle held since Pin. Write calls it right
// before renaming over the file, which Windows refuses while it is open.
func (p *PinnedFile) release() {
	if p.held != nil {
		_ = p.held.Close()
		p.held = nil
	}
}

// Check reports ErrFileChanged when the name no longer refers to the file
// that was read, or when its size or modification time changed, or, with
// strict, when its content changed.
func (p *PinnedFile) Check(strict bool) error {
	file, info, err := p.openSame(os.O_RDONLY)
	if err != nil {
		return err
	}
	defer file.Close()

	if info.Size() != p.info.Size() || !info.ModTime().Equal(p.info.ModTime()) {
		return fmt.Errorf("%w: %s was modified", ErrFileChanged, p.name)
	}
	if !strict {
		return nil
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", p.name, err)
	}
	if sha256.Sum256(content) != p.hash {
		return fmt.Errorf("%w: %s was modified", ErrFileChanged, p.name)
	}
	return nil
}

// openSame opens the name through the root and checks that it still refers
// to the file that was read. Any failure to open it means the file is gone,
// was replaced or now lies outside the root, so it is reported as
// ErrFileChanged.
func (p *PinnedFile) openSame(flag int) (*os.File, os.FileInfo, error) {
	file, err := p.root.OpenFile(p.name, flag, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: open %s: %w", ErrFileChanged, p.name, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("stat %s: %w", p.name, err)
	}
	if !os.SameFile(info, p.info) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: %s now refers to another file", ErrFileChanged, p.name)
	}
	return file, info, nil
}

// Backup saves the content that was read next to the file, as the name plus
// BackupSuffix, unless mode is BackupModeNone or a backup already exists. It
// reports whether it created one. Like CreateBackup, it never overwrites an
// existing backup, so repeated runs keep the oldest original.
func (p *PinnedFile) Backup(mode BackupMode) (bool, error) {
	if mode == BackupModeNone {
		return false, nil
	}

	backupName := p.name + BackupSuffix
	if _, err := p.root.Lstat(backupName); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("stat backup %s: %w", backupName, err)
	}

	tmpName, tmp, err := p.createTemp(filepath.Base(backupName))
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = p.root.Remove(tmpName) //nolint:errcheck // Best effort: cleaning up a temp file after a failed write.
		}
	}()

	if _, err := tmp.Write(p.content); err != nil {
		return false, fmt.Errorf("write backup: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return false, fmt.Errorf("sync backup: %w", err)
	}
	if err := tmp.Chmod(p.info.Mode()); err != nil {
		return false, fmt.Errorf("chmod backup: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("close backup: %w", err)
	}
	if err := p.root.Rename(tmpName, backupName); err != nil {
		return false, fmt.Errorf("rename backup into place: %w", err)
	}
	committed = true
	p.syncDir()
	return true, nil
}

// Write replaces the file's content and keeps its mode, and its owner where
// possible. At commit time the name must still refer to the file that was
// read; otherwise nothing is written and the error wraps ErrFileChanged.
//
// A file with more than one hard link is rewritten in place, through a
// handle that was opened and checked for the purpose, so every link sees
// the new content; the bytes for a rollback are read through that same
// handle. Otherwise the content goes to a temp file in the same directory,
// which gets the original owner and mode and is renamed over the name. When
// the owner cannot be restored, a writable file is rewritten in place
// instead, so its owner does not change.
func (p *PinnedFile) Write(content []byte) error {
	file, _, err := p.openSame(os.O_RDONLY)
	if err != nil {
		return err
	}
	multiple := hasMultipleLinks(file)
	_ = file.Close()

	if multiple {
		return p.rewriteInPlace(content)
	}
	return p.replace(content)
}

// rewriteInPlace overwrites the file through a checked handle, which keeps
// its inode, hard links and owner.
func (p *PinnedFile) rewriteInPlace(content []byte) error {
	file, info, err := p.openSame(os.O_RDWR)
	if err != nil {
		return err
	}

	original := make([]byte, info.Size())
	if _, err := file.ReadAt(original, 0); err != nil && !errors.Is(err, io.EOF) {
		_ = file.Close()
		return fmt.Errorf("read %s before rewriting it in place: %w", p.name, err)
	}

	if err := overwriteWithRollback(file, content, original); err != nil {
		_ = file.Close()
		return fmt.Errorf("rewrite %s in place: %w", p.name, err)
	}

	mode := p.info.Mode()
	if info.Mode()&chmodBits != mode&chmodBits {
		if err := file.Chmod(mode); err != nil {
			_ = file.Close()
			return fmt.Errorf("chmod %s after rewriting it in place (the new content is written): %w", p.name, err)
		}
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", p.name, err)
	}
	return nil
}

// replace writes content to a temp file next to the file and renames it
// over the name, after checking that the name still refers to the file.
func (p *PinnedFile) replace(content []byte) error {
	tmpName, tmp, err := p.createTemp(filepath.Base(p.name))
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = p.root.Remove(tmpName) //nolint:errcheck // Best effort: cleaning up a temp file after a failed write.
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	// Restore the owner before the mode: changing the owner can clear the
	// set-user-ID and set-group-ID bits.
	restored, err := restoreOwner(tmp, p.info)
	if err != nil {
		return err
	}
	if !restored {
		// Keep the owner by rewriting the file in place, if it is writable;
		// otherwise replace it as before, leaving it to the current user.
		err := p.rewriteInPlace(content)
		if err == nil || !errors.Is(err, fs.ErrPermission) {
			return err
		}
	}

	if err := tmp.Chmod(p.info.Mode()); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Commit only if the name still refers to the file that was read.
	current, err := p.root.Lstat(p.name)
	if err != nil {
		return fmt.Errorf("%w: stat %s: %w", ErrFileChanged, p.name, err)
	}
	if !os.SameFile(current, p.info) {
		return fmt.Errorf("%w: %s now refers to another file", ErrFileChanged, p.name)
	}
	p.release()
	if err := p.root.Rename(tmpName, p.name); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	committed = true
	p.syncDir()
	return nil
}

// createTemp creates a new file with a random name based on base in the
// file's directory, inside the root.
func (p *PinnedFile) createTemp(base string) (string, *os.File, error) {
	dir := filepath.Dir(p.name)
	for range maxTempAttempts {
		var suffix [6]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", nil, fmt.Errorf("create temp file name: %w", err)
		}
		name := filepath.Join(dir, "."+base+".tmp."+hex.EncodeToString(suffix[:]))
		file, err := p.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return name, file, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, fmt.Errorf("create temp file: %w", err)
		}
	}
	return "", nil, fmt.Errorf("create temp file in %s: no unused name found", filepath.Join(p.dir, dir))
}

// syncDir makes a rename in the file's directory durable, on a best-effort
// basis.
func (p *PinnedFile) syncDir() {
	if !syncDirectories {
		return
	}
	dir, err := p.root.Open(filepath.Dir(p.name))
	if err != nil {
		return
	}
	_ = dir.Sync() //nolint:errcheck // Best effort: the rename stands whether or not the directory can be synced.
	_ = dir.Close()
}
