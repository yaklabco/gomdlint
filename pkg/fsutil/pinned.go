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

// errExchangeUnsupported reports that two names cannot be exchanged
// atomically on this platform or file system.
var errExchangeUnsupported = errors.New("atomic exchange not supported")

// PinnedFile is a regular file read for fixing, which is written back only
// if nothing changed it meanwhile.
//
// Pin opens the write root as an os.Root, opens the file's directory
// through it as its own os.Root, and opens and reads the file through that
// directory. It keeps the file open, so its identity (device and inode)
// cannot be reused by another file. Every later operation goes through the
// pinned directory: a directory on the path that is moved, or replaced by a
// link, afterwards cannot redirect it, and links that would lead out of the
// directory are refused. Before a backup or a write, the pinned file checks
// that the directory is still where it was pinned, that the name in it
// still refers to the file that was read, and that the file still holds the
// content that was read (re-read through a handle on that file). Otherwise
// it fails with ErrFileChanged.
//
// How a write commits, and what can still race with other programs:
//   - A file with more than one hard link is rewritten in place, through a
//     handle that was opened and checked for the purpose. A program writing
//     into the file between that check and the rewrite (a few system calls)
//     can interleave with it; file systems offer no way to prevent that
//     short of mandatory locking.
//   - Otherwise the new content goes to a hidden temp file in the same
//     directory. On Linux the temp file and the name are then exchanged
//     atomically (renameat2 with RENAME_EXCHANGE), and the file swapped out
//     must be the pinned file, with a single link and the content that was
//     read; if it is not, the exchange is undone and the write fails with
//     ErrFileChanged. Only a program that keeps the old file open and
//     writes to it after the exchange loses that write, as with any save
//     that replaces a file.
//   - Where names cannot be exchanged (macOS, BSD, Windows, other
//     platforms, and Linux file systems without RENAME_EXCHANGE), the temp
//     file is renamed over the name right after the checks. A file replaced
//     or saved into, or a hard link created, between those checks and the
//     rename (a few system calls) is not detected.
//
// A PinnedFile must be closed.
type PinnedFile struct {
	root       *os.Root    // the write root
	parent     *os.Root    // the file's directory, opened through root
	exchange   *os.File    // the same directory, for exchanging names (Linux)
	parentRel  string      // the directory's path relative to root
	parentInfo os.FileInfo // the directory's identity
	held       *os.File    // the file as read, kept open
	dir        string      // the write root's path
	name       string      // the file's path relative to root
	base       string      // the file's name in parent
	info       os.FileInfo
	content    []byte
	hash       [sha256.Size]byte

	// beforeCommit, when set, runs after the final checks and right before
	// a backup or a write is committed. Tests use it to race the commit.
	beforeCommit func()

	// noExchange forces the rename commit. Tests use it to cover the
	// platforms without an atomic exchange.
	noExchange bool
}

// Pin opens the regular file name, relative to the directory dir, and reads
// it. Symbolic links in name are followed only while they stay inside dir.
func Pin(dir, name string) (*PinnedFile, error) {
	pinned := &PinnedFile{
		dir:       dir,
		name:      name,
		parentRel: filepath.Dir(name),
		base:      filepath.Base(name),
	}
	if err := pinned.open(); err != nil {
		_ = pinned.Close()
		return nil, err
	}
	return pinned, nil
}

// open opens the handles of a new PinnedFile and reads the file.
func (p *PinnedFile) open() error {
	path := filepath.Join(p.dir, p.name)

	var err error
	if p.root, err = os.OpenRoot(p.dir); err != nil {
		return describeOpenError(path, err)
	}
	if p.parent, err = p.root.OpenRoot(p.parentRel); err != nil {
		return describeOpenError(path, err)
	}
	if p.parentInfo, err = p.parent.Stat("."); err != nil {
		return fmt.Errorf("stat the directory of %s: %w", path, err)
	}
	if p.exchange, err = openExchangeDir(p.parent); err != nil {
		return fmt.Errorf("open the directory of %s: %w", path, err)
	}
	if p.held, err = p.parent.Open(p.base); err != nil {
		return describeOpenError(path, err)
	}
	if p.info, err = p.held.Stat(); err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if p.info.IsDir() {
		return fmt.Errorf("%w: %s", ErrIsDirectory, path)
	}
	if !p.info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if p.content, err = io.ReadAll(p.held); err != nil {
		return describeOpenError(path, err)
	}
	p.hash = sha256.Sum256(p.content)
	return nil
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
	var errs []error
	if p.exchange != nil {
		errs = append(errs, p.exchange.Close())
		p.exchange = nil
	}
	if p.parent != nil {
		errs = append(errs, p.parent.Close())
		p.parent = nil
	}
	if p.root != nil {
		errs = append(errs, p.root.Close())
		p.root = nil
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("close %s: %w", p.dir, err)
	}
	return nil
}

// release closes the file handle held since Pin. The rename commit calls it
// first, since Windows refuses to rename over a file that is open.
func (p *PinnedFile) release() {
	if p.held != nil {
		_ = p.held.Close()
		p.held = nil
	}
}

// Check reports ErrFileChanged when the directory moved, the name no longer
// refers to the file that was read, or the file changed: its size or
// modification time, or, with strict, its content.
func (p *PinnedFile) Check(strict bool) error {
	file, _, err := p.verify(os.O_RDONLY, strict)
	if err != nil {
		return err
	}
	defer file.Close()
	if strict {
		return nil
	}

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", p.name, err)
	}
	if info.Size() != p.info.Size() || !info.ModTime().Equal(p.info.ModTime()) {
		return fmt.Errorf("%w: %s was modified", ErrFileChanged, p.name)
	}
	return nil
}

// verify checks that the directory is still where it was pinned and that
// the name in it still refers to the pinned file, and returns a handle to
// that file opened with flag. With content set, it also reads the file
// through that handle and checks that it holds the content that was read,
// and returns those bytes. Every mismatch is reported as ErrFileChanged.
func (p *PinnedFile) verify(flag int, content bool) (*os.File, []byte, error) {
	current, err := p.root.Lstat(p.parentRel)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: the directory of %s: %w", ErrFileChanged, p.name, err)
	}
	if !os.SameFile(current, p.parentInfo) {
		return nil, nil, fmt.Errorf("%w: the directory of %s was moved or replaced", ErrFileChanged, p.name)
	}

	file, err := p.parent.OpenFile(p.base, flag, 0)
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
	if !content {
		return file, nil, nil
	}

	data, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("read %s: %w", p.name, err)
	}
	if sha256.Sum256(data) != p.hash {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: %s was modified", ErrFileChanged, p.name)
	}
	return file, data, nil
}

// Backup saves the content that was read next to the file, as the name plus
// BackupSuffix, unless mode is BackupModeNone or a backup already exists. It
// reports whether it created one. It first checks that the file is
// unchanged, so it never saves a backup for a write that will be refused,
// and it never overwrites an existing backup, so repeated runs keep the
// oldest original.
func (p *PinnedFile) Backup(mode BackupMode) (bool, error) {
	if mode == BackupModeNone {
		return false, nil
	}

	file, _, err := p.verify(os.O_RDONLY, true)
	if err != nil {
		return false, err
	}
	_ = file.Close()

	backupName := p.base + BackupSuffix
	tmpName, tmp, err := p.createTemp(backupName)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = tmp.Close()
		_ = p.parent.Remove(tmpName) //nolint:errcheck // Best effort: the temp name is removed once the backup is linked or abandoned.
	}()

	if err := writeAndClose(tmp, p.content, p.info.Mode()); err != nil {
		return false, fmt.Errorf("write backup: %w", err)
	}

	if p.beforeCommit != nil {
		p.beforeCommit()
	}

	// Link the temp file into place: unlike a rename, a link never
	// replaces a backup that exists already.
	err = p.parent.Link(tmpName, backupName)
	switch {
	case err == nil:
		p.syncDir()
		return true, nil
	case errors.Is(err, fs.ErrExist):
		return false, nil
	}

	// Hard links are not available here: create the backup exclusively.
	return p.writeBackupExclusively(backupName)
}

// writeBackupExclusively creates the backup with O_EXCL, so it never
// replaces an existing one either. Unlike linking, a crash part way leaves
// a partial backup.
func (p *PinnedFile) writeBackupExclusively(name string) (bool, error) {
	file, err := p.parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create backup %s: %w", name, err)
	}
	if err := writeAndClose(file, p.content, p.info.Mode()); err != nil {
		_ = p.parent.Remove(name) //nolint:errcheck // Best effort: a partial backup is worse than none.
		return false, fmt.Errorf("write backup %s: %w", name, err)
	}
	p.syncDir()
	return true, nil
}

// writeAndClose writes content to a new file, syncs it, gives it mode and
// closes it.
func writeAndClose(file *os.File, content []byte, mode os.FileMode) error {
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync: %w", err)
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("chmod: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// Write replaces the file's content and keeps its mode, and its owner where
// possible, as described on PinnedFile. If the file changed after it was
// read, nothing is written and the error wraps ErrFileChanged.
func (p *PinnedFile) Write(content []byte) error {
	file, _, err := p.verify(os.O_RDONLY, true)
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

// rewriteInPlace overwrites the file through a handle that was checked for
// identity and content just before, which keeps its inode, hard links and
// owner. The bytes for a rollback are the ones read through that handle.
func (p *PinnedFile) rewriteInPlace(content []byte) error {
	file, original, err := p.verify(os.O_RDWR, true)
	if err != nil {
		return err
	}

	if err := overwriteWithRollback(file, content, original); err != nil {
		_ = file.Close()
		return fmt.Errorf("rewrite %s in place: %w", p.name, err)
	}

	mode := p.info.Mode()
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat %s after rewriting it in place: %w", p.name, err)
	}
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

// replace writes content to a temp file next to the file and commits it as
// described on PinnedFile.
func (p *PinnedFile) replace(content []byte) error {
	tmpName, tmp, err := p.createTemp(p.base)
	if err != nil {
		return err
	}
	tmpLeft := true
	defer func() {
		if tmpLeft {
			_ = tmp.Close()
			_ = p.parent.Remove(tmpName) //nolint:errcheck // Best effort: cleaning up a temp file after a failed write.
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

	// The name must still refer to the pinned file, unchanged.
	file, _, err := p.verify(os.O_RDONLY, true)
	if err != nil {
		return err
	}
	_ = file.Close()

	if p.beforeCommit != nil {
		p.beforeCommit()
	}

	if !p.noExchange {
		err := exchangeFiles(p.exchange, tmpName, p.base)
		if err == nil {
			tmpLeft = false
			return p.settleExchange(tmpName)
		}
		if !errors.Is(err, errExchangeUnsupported) {
			return err
		}
	}

	// No atomic exchange here: rename over the name. A change between the
	// check above and this rename is not detected.
	p.release()
	if err := p.parent.Rename(tmpName, p.base); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	tmpLeft = false
	p.syncDir()
	return nil
}

// settleExchange runs after the temp file and the name were exchanged: the
// name now holds the new content and tmpName the file that had the name. If
// that file is the pinned file, with a single link and the content that was
// read, it is removed and the write is done. Otherwise the exchange is
// undone and the write fails with ErrFileChanged.
func (p *PinnedFile) settleExchange(tmpName string) error {
	reason := p.swappedOutChange(tmpName)
	if reason == "" {
		_ = p.parent.Remove(tmpName) //nolint:errcheck // Best effort: the write is done; only the replaced file is left over.
		p.syncDir()
		return nil
	}

	if err := exchangeFiles(p.exchange, tmpName, p.base); err != nil {
		return fmt.Errorf("%s %s during the write, and putting it back failed; its content is in %s: %w",
			p.name, reason, filepath.Join(p.dir, p.parentRel, tmpName), err)
	}
	_ = p.parent.Remove(tmpName) //nolint:errcheck // Best effort: removing the unused new content.
	return fmt.Errorf("%w: %s %s", ErrFileChanged, p.name, reason)
}

// swappedOutChange describes how the file now at name differs from the
// pinned file, or returns an empty string when it is the pinned file, with a
// single link and the content that was read.
func (p *PinnedFile) swappedOutChange(name string) string {
	file, err := p.parent.Open(name)
	if err != nil {
		return "could not be checked"
	}
	defer file.Close()

	info, err := file.Stat()
	switch {
	case err != nil:
		return "could not be checked"
	case !os.SameFile(info, p.info):
		return "was replaced"
	case hasMultipleLinks(file):
		return "got another hard link"
	}
	data, err := io.ReadAll(file)
	if err != nil || sha256.Sum256(data) != p.hash {
		return "was modified"
	}
	return ""
}

// createTemp creates a new hidden file, named after base with a random
// suffix, in the file's directory.
func (p *PinnedFile) createTemp(base string) (string, *os.File, error) {
	for range maxTempAttempts {
		var suffix [6]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", nil, fmt.Errorf("create temp file name: %w", err)
		}
		name := "." + base + ".tmp." + hex.EncodeToString(suffix[:])
		file, err := p.parent.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return name, file, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, fmt.Errorf("create temp file: %w", err)
		}
	}
	return "", nil, fmt.Errorf("create temp file in %s: no unused name found", filepath.Join(p.dir, p.parentRel))
}

// syncDir makes a change in the file's directory durable, on a best-effort
// basis.
func (p *PinnedFile) syncDir() {
	if !syncDirectories {
		return
	}
	dir, err := p.parent.Open(".")
	if err != nil {
		return
	}
	_ = dir.Sync() //nolint:errcheck // Best effort: the change stands whether or not the directory can be synced.
	_ = dir.Close()
}
