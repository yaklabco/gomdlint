// Package fsutil provides file system utilities and safety primitives for gomdlint.
// It handles atomic writes, content hashing, modification detection, and backups.
package fsutil

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"time"
)

// FileInfo captures the state of a file at a point in time.
// It is used for modification detection during the safety pipeline.
type FileInfo struct {
	// Path is the absolute or relative path to the file.
	Path string

	// Mode is the file's permission and mode bits.
	Mode os.FileMode

	// ModTime is the file's modification time.
	ModTime time.Time

	// Size is the file size in bytes.
	Size int64

	// Hash is the SHA-256 hash of the file content.
	Hash [32]byte
}

// ReadFile reads a file and returns its content along with metadata.
// The returned FileInfo can be used for modification detection.
func ReadFile(ctx context.Context, path string) ([]byte, *FileInfo, error) {
	select {
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("read file: %w", ctx.Err())
	default:
	}

	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("%w: %s: %w", ErrNotFound, path, err)
		}
		if os.IsPermission(err) {
			return nil, nil, fmt.Errorf("%w: %s: %w", ErrPermissionDenied, path, err)
		}
		return nil, nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if stat.IsDir() {
		return nil, nil, fmt.Errorf("%w: %s", ErrIsDirectory, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, nil, fmt.Errorf("%w: %s: %w", ErrPermissionDenied, path, err)
		}
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	info := &FileInfo{
		Path:    path,
		Mode:    stat.Mode(),
		ModTime: stat.ModTime(),
		Size:    stat.Size(),
		Hash:    sha256.Sum256(content),
	}

	return content, info, nil
}

// CheckModified returns true if the file has been modified since the given FileInfo.
// This is used to detect concurrent external modifications.
//
// The check uses a two-tier approach:
//  1. Quick check: compare mod time and size (fast, catches most cases)
//  2. Hash check: re-read and hash content (paranoid, catches all changes)
//
// Sentinel errors for error categorization via errors.Is.
var (
	// ErrNilFileInfo is returned when a nil FileInfo is passed.
	ErrNilFileInfo = errors.New("nil FileInfo")

	// ErrNotFound indicates the file does not exist.
	ErrNotFound = errors.New("file not found")

	// ErrPermissionDenied indicates a permission error.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrIsDirectory indicates the path is a directory, not a file.
	ErrIsDirectory = errors.New("path is a directory")
)

func CheckModified(ctx context.Context, info *FileInfo) (bool, error) {
	if info == nil {
		return false, ErrNilFileInfo
	}

	select {
	case <-ctx.Done():
		return false, fmt.Errorf("check modified: %w", ctx.Err())
	default:
	}

	stat, err := os.Stat(info.Path)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted - that's a modification.
			return true, nil
		}
		return false, fmt.Errorf("stat %s: %w", info.Path, err)
	}

	// Quick check: mod time and size.
	if !stat.ModTime().Equal(info.ModTime) || stat.Size() != info.Size {
		return true, nil
	}

	// Paranoid check: re-hash the content.
	content, err := os.ReadFile(info.Path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", info.Path, err)
	}

	currentHash := sha256.Sum256(content)
	return currentHash != info.Hash, nil
}

// CheckModifiedQuick performs only the quick modification check (mod time + size).
// Use this when hash comparison is too expensive and false negatives are acceptable.
func CheckModifiedQuick(ctx context.Context, info *FileInfo) (bool, error) {
	if info == nil {
		return false, ErrNilFileInfo
	}

	select {
	case <-ctx.Done():
		return false, fmt.Errorf("check modified: %w", ctx.Err())
	default:
	}

	stat, err := os.Stat(info.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("stat %s: %w", info.Path, err)
	}

	return !stat.ModTime().Equal(info.ModTime) || stat.Size() != info.Size, nil
}
