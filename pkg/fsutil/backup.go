package fsutil

import (
	"context"
	"fmt"
	"os"
)

// BackupMode specifies how backups are stored.
type BackupMode string

const (
	// BackupModeSidecar stores backups alongside the original file with a .gomdlint.bak suffix.
	BackupModeSidecar BackupMode = "sidecar"

	// BackupModeNone disables backups.
	BackupModeNone BackupMode = "none"
)

// BackupSuffix is the suffix used for sidecar backup files.
const BackupSuffix = ".gomdlint.bak"

// BackupConfig controls backup behavior.
type BackupConfig struct {
	// Enabled indicates whether backups should be created.
	Enabled bool

	// Mode specifies how backups are stored.
	Mode BackupMode
}

// DefaultBackupConfig returns sensible backup defaults.
// Backups are disabled by default.
func DefaultBackupConfig() BackupConfig {
	return BackupConfig{
		Enabled: false,
		Mode:    BackupModeSidecar,
	}
}

// BackupPath returns the backup path for the given file based on the mode.
func BackupPath(path string, mode BackupMode) string {
	switch mode {
	case BackupModeSidecar:
		return path + BackupSuffix
	case BackupModeNone:
		return ""
	default:
		// Default to sidecar mode for unknown modes.
		return path + BackupSuffix
	}
}

// CreateBackup creates a backup of the file at path if one does not already exist.
// Returns true if a backup was created, false if it already existed or backups are disabled.
//
// Backup creation is idempotent: if a backup already exists, it is not overwritten.
// This ensures that repeated runs do not lose the original file content.
func CreateBackup(ctx context.Context, path string, cfg BackupConfig) (bool, error) {
	if !cfg.Enabled || cfg.Mode == BackupModeNone {
		return false, nil
	}

	select {
	case <-ctx.Done():
		return false, fmt.Errorf("create backup: %w", ctx.Err())
	default:
	}

	backupPath := BackupPath(path, cfg.Mode)
	if backupPath == "" {
		return false, nil
	}

	// Check if backup already exists.
	if _, err := os.Stat(backupPath); err == nil {
		// Backup exists, do not overwrite.
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat backup path: %w", err)
	}

	// Read original content.
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Original file doesn't exist, nothing to backup.
			return false, nil
		}
		return false, fmt.Errorf("read original for backup: %w", err)
	}

	// Get original mode.
	stat, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat original for backup: %w", err)
	}

	// Write backup atomically.
	if err := WriteAtomic(ctx, backupPath, content, stat.Mode()); err != nil {
		return false, fmt.Errorf("write backup: %w", err)
	}

	return true, nil
}

// RestoreBackup restores a file from its backup.
// Returns true if the file was restored, false if no backup exists.
func RestoreBackup(ctx context.Context, path string, mode BackupMode) (bool, error) {
	select {
	case <-ctx.Done():
		return false, fmt.Errorf("restore backup: %w", ctx.Err())
	default:
	}

	backupPath := BackupPath(path, mode)
	if backupPath == "" {
		return false, nil
	}

	// Read backup content.
	content, err := os.ReadFile(backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read backup: %w", err)
	}

	// Get backup mode.
	stat, err := os.Stat(backupPath)
	if err != nil {
		return false, fmt.Errorf("stat backup: %w", err)
	}

	// Write original file atomically.
	if err := WriteAtomic(ctx, path, content, stat.Mode()); err != nil {
		return false, fmt.Errorf("restore from backup: %w", err)
	}

	return true, nil
}

// RemoveBackup removes the backup file for the given path if it exists.
// Returns true if a backup was removed, false if none existed.
func RemoveBackup(path string, mode BackupMode) (bool, error) {
	backupPath := BackupPath(path, mode)
	if backupPath == "" {
		return false, nil
	}

	err := os.Remove(backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("remove backup: %w", err)
	}

	return true, nil
}

// BackupExists checks if a backup file exists for the given path.
func BackupExists(path string, mode BackupMode) bool {
	backupPath := BackupPath(path, mode)
	if backupPath == "" {
		return false
	}
	_, err := os.Stat(backupPath)
	return err == nil
}
