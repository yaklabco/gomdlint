package fsutil_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/fsutil"
)

func TestBackupPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		mode fsutil.BackupMode
		want string
	}{
		{
			name: "sidecar mode",
			path: "/path/to/file.md",
			mode: fsutil.BackupModeSidecar,
			want: "/path/to/file.md.gomdlint.bak",
		},
		{
			name: "none mode returns empty",
			path: "/path/to/file.md",
			mode: fsutil.BackupModeNone,
			want: "",
		},
		{
			name: "unknown mode defaults to sidecar",
			path: "/path/to/file.md",
			mode: fsutil.BackupMode("unknown"),
			want: "/path/to/file.md.gomdlint.bak",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := fsutil.BackupPath(tt.path, tt.mode)
			if got != tt.want {
				t.Errorf("BackupPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultBackupConfig(t *testing.T) {
	t.Parallel()

	cfg := fsutil.DefaultBackupConfig()

	if cfg.Enabled {
		t.Error("expected Enabled = false by default")
	}

	if cfg.Mode != fsutil.BackupModeSidecar {
		t.Errorf("Mode = %q, want %q", cfg.Mode, fsutil.BackupModeSidecar)
	}
}

//nolint:gocognit,maintidx // Test function with many subtests
func TestCreateBackup(t *testing.T) {
	t.Parallel()

	t.Run("creates backup for existing file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		content := []byte("hello world")

		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		cfg := fsutil.BackupConfig{
			Enabled: true,
			Mode:    fsutil.BackupModeSidecar,
		}

		ctx := context.Background()
		created, err := fsutil.CreateBackup(ctx, path, cfg)

		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}

		if !created {
			t.Error("expected created = true")
		}

		// Verify backup exists with correct content.
		backupPath := fsutil.BackupPath(path, cfg.Mode)
		got, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatalf("read backup: %v", err)
		}

		if string(got) != string(content) {
			t.Errorf("backup content = %q, want %q", got, content)
		}
	})

	t.Run("does not overwrite existing backup", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		originalContent := []byte("original content")
		backupContent := []byte("existing backup")

		if err := os.WriteFile(path, originalContent, 0644); err != nil {
			t.Fatalf("setup original: %v", err)
		}

		cfg := fsutil.BackupConfig{
			Enabled: true,
			Mode:    fsutil.BackupModeSidecar,
		}

		backupPath := fsutil.BackupPath(path, cfg.Mode)
		if err := os.WriteFile(backupPath, backupContent, 0644); err != nil {
			t.Fatalf("setup backup: %v", err)
		}

		ctx := context.Background()
		created, err := fsutil.CreateBackup(ctx, path, cfg)

		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}

		if created {
			t.Error("expected created = false for existing backup")
		}

		// Verify backup content unchanged.
		got, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatalf("read backup: %v", err)
		}

		if string(got) != string(backupContent) {
			t.Errorf("backup content = %q, want %q", got, backupContent)
		}
	})

	t.Run("returns false when disabled", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		cfg := fsutil.BackupConfig{
			Enabled: false,
			Mode:    fsutil.BackupModeSidecar,
		}

		ctx := context.Background()
		created, err := fsutil.CreateBackup(ctx, path, cfg)

		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}

		if created {
			t.Error("expected created = false when disabled")
		}

		// Verify no backup created.
		backupPath := fsutil.BackupPath(path, cfg.Mode)
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Error("backup should not exist when disabled")
		}
	})

	t.Run("returns false when mode is none", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		cfg := fsutil.BackupConfig{
			Enabled: true,
			Mode:    fsutil.BackupModeNone,
		}

		ctx := context.Background()
		created, err := fsutil.CreateBackup(ctx, path, cfg)

		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}

		if created {
			t.Error("expected created = false when mode is none")
		}
	})

	t.Run("returns false for non-existent file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.txt")

		cfg := fsutil.BackupConfig{
			Enabled: true,
			Mode:    fsutil.BackupModeSidecar,
		}

		ctx := context.Background()
		created, err := fsutil.CreateBackup(ctx, path, cfg)

		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}

		if created {
			t.Error("expected created = false for non-existent file")
		}
	})

	t.Run("preserves file mode in backup", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
			t.Fatalf("setup: %v", err)
		}

		cfg := fsutil.BackupConfig{
			Enabled: true,
			Mode:    fsutil.BackupModeSidecar,
		}

		ctx := context.Background()
		_, err := fsutil.CreateBackup(ctx, path, cfg)

		if err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}

		backupPath := fsutil.BackupPath(path, cfg.Mode)
		stat, err := os.Stat(backupPath)
		if err != nil {
			t.Fatalf("stat backup: %v", err)
		}

		if stat.Mode().Perm() != 0600 {
			t.Errorf("backup mode = %o, want %o", stat.Mode().Perm(), 0600)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		cfg := fsutil.BackupConfig{
			Enabled: true,
			Mode:    fsutil.BackupModeSidecar,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := fsutil.CreateBackup(ctx, path, cfg)

		if err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})
}

func TestRestoreBackup(t *testing.T) {
	t.Parallel()

	t.Run("restores from backup", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		backupContent := []byte("backup content")
		currentContent := []byte("current content")

		// Create current file.
		if err := os.WriteFile(path, currentContent, 0644); err != nil {
			t.Fatalf("setup current: %v", err)
		}

		// Create backup.
		backupPath := fsutil.BackupPath(path, fsutil.BackupModeSidecar)
		if err := os.WriteFile(backupPath, backupContent, 0644); err != nil {
			t.Fatalf("setup backup: %v", err)
		}

		ctx := context.Background()
		restored, err := fsutil.RestoreBackup(ctx, path, fsutil.BackupModeSidecar)

		if err != nil {
			t.Fatalf("RestoreBackup() error = %v", err)
		}

		if !restored {
			t.Error("expected restored = true")
		}

		// Verify content restored.
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		if string(got) != string(backupContent) {
			t.Errorf("content = %q, want %q", got, backupContent)
		}
	})

	t.Run("returns false when no backup exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		ctx := context.Background()
		restored, err := fsutil.RestoreBackup(ctx, path, fsutil.BackupModeSidecar)

		if err != nil {
			t.Fatalf("RestoreBackup() error = %v", err)
		}

		if restored {
			t.Error("expected restored = false when no backup exists")
		}
	})

	t.Run("returns false for none mode", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		ctx := context.Background()
		restored, err := fsutil.RestoreBackup(ctx, path, fsutil.BackupModeNone)

		if err != nil {
			t.Fatalf("RestoreBackup() error = %v", err)
		}

		if restored {
			t.Error("expected restored = false for none mode")
		}
	})
}

func TestRemoveBackup(t *testing.T) {
	t.Parallel()

	t.Run("removes existing backup", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		backupPath := fsutil.BackupPath(path, fsutil.BackupModeSidecar)

		// Create backup file.
		if err := os.WriteFile(backupPath, []byte("backup"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		removed, err := fsutil.RemoveBackup(path, fsutil.BackupModeSidecar)

		if err != nil {
			t.Fatalf("RemoveBackup() error = %v", err)
		}

		if !removed {
			t.Error("expected removed = true")
		}

		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Error("backup should not exist after removal")
		}
	})

	t.Run("returns false when no backup exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.txt")

		removed, err := fsutil.RemoveBackup(path, fsutil.BackupModeSidecar)

		if err != nil {
			t.Fatalf("RemoveBackup() error = %v", err)
		}

		if removed {
			t.Error("expected removed = false when no backup exists")
		}
	})

	t.Run("returns false for none mode", func(t *testing.T) {
		t.Parallel()

		removed, err := fsutil.RemoveBackup("/any/path", fsutil.BackupModeNone)

		if err != nil {
			t.Fatalf("RemoveBackup() error = %v", err)
		}

		if removed {
			t.Error("expected removed = false for none mode")
		}
	})
}

func TestBackupExists(t *testing.T) {
	t.Parallel()

	t.Run("returns true when backup exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		backupPath := fsutil.BackupPath(path, fsutil.BackupModeSidecar)

		if err := os.WriteFile(backupPath, []byte("backup"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		if !fsutil.BackupExists(path, fsutil.BackupModeSidecar) {
			t.Error("expected BackupExists = true")
		}
	})

	t.Run("returns false when backup does not exist", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		if fsutil.BackupExists(path, fsutil.BackupModeSidecar) {
			t.Error("expected BackupExists = false")
		}
	})

	t.Run("returns false for none mode", func(t *testing.T) {
		t.Parallel()

		if fsutil.BackupExists("/any/path", fsutil.BackupModeNone) {
			t.Error("expected BackupExists = false for none mode")
		}
	})
}
