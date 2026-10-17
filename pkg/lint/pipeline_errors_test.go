package lint_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fsutil"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestProcessFileErrorNamesCategoryOnce covers errors from reading a file:
// they match both the pipeline and fsutil categories, but the message
// names the category once.
func TestProcessFileErrorNamesCategoryOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.md")
	locked := filepath.Join(dir, "locked.md")
	if err := os.WriteFile(locked, []byte("# Title\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatalf("setup: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		category  error
		fsutilErr error
		skipRoot  bool
	}{
		{name: "missing", path: missing, category: lint.ErrFileNotFound, fsutilErr: fsutil.ErrNotFound},
		{name: "unreadable", path: locked, category: lint.ErrPermissionDenied, fsutilErr: fsutil.ErrPermissionDenied, skipRoot: true},
	}

	pipeline := lint.NewPipeline(lint.NewEngine(nil, lint.NewRegistry()))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipRoot && (os.Geteuid() == 0 || runtime.GOOS == "windows") {
				t.Skip("permission bits do not stop root or Windows from reading files")
			}

			_, err := pipeline.ProcessFile(context.Background(), tt.path, config.NewConfig(), lint.DefaultPipelineOptions())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, tt.category) || !errors.Is(err, tt.fsutilErr) || !lint.IsPipelineError(err) {
				t.Errorf("error %q lost its category", err)
			}
			if doubled := tt.category.Error() + ": " + tt.category.Error(); strings.Contains(err.Error(), doubled) {
				t.Errorf("error %q repeats %q", err, tt.category)
			}
			if !strings.Contains(err.Error(), tt.path) {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}
