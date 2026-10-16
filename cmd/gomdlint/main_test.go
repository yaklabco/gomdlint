package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cleanMarkdown    = "# Title\n\nSome text.\n"
	trailingMarkdown = "# Title \n\nSome text.\n"
)

// isolate runs the test from an empty directory with no user configuration
// and no GOMDLINT_* settings, so machine state cannot change the outcome.
func isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".xdg"))
	for _, entry := range os.Environ() {
		if name, _, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(name, "GOMDLINT_") {
			t.Setenv(name, "")
		}
	}
	t.Chdir(dir)

	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// TestRunExitCodes drives the real entry point once per documented exit code.
func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		args       []string
		cancel     bool
		unreadable string
		want       int
		wantStderr string
	}{
		{
			name:  "clean file exits 0",
			files: map[string]string{"ok.md": cleanMarkdown},
			args:  []string{"lint", "ok.md"},
			want:  0,
		},
		{
			name:  "warnings without strict exit 0",
			files: map[string]string{"warn.md": trailingMarkdown},
			args:  []string{"lint", "warn.md"},
			want:  0,
		},
		{
			name: "error findings exit 1",
			files: map[string]string{
				"warn.md":       trailingMarkdown,
				".gomdlint.yml": "rules:\n  MD009:\n    severity: error\n",
			},
			args: []string{"lint", "warn.md"},
			want: 1,
		},
		{
			name:  "warnings with strict exit 2",
			files: map[string]string{"warn.md": trailingMarkdown},
			args:  []string{"lint", "--strict", "warn.md"},
			want:  2,
		},
		{
			name:       "unknown flag exits 64",
			files:      map[string]string{"ok.md": cleanMarkdown},
			args:       []string{"lint", "--bogus", "ok.md"},
			want:       64,
			wantStderr: "Run 'gomdlint lint --help' for usage.",
		},
		{
			name:       "invalid format value exits 64",
			files:      map[string]string{"ok.md": cleanMarkdown},
			args:       []string{"lint", "--format", "bogus", "ok.md"},
			want:       64,
			wantStderr: `"bogus"`,
		},
		{
			name: "invalid configuration exits 65",
			files: map[string]string{
				"ok.md":         cleanMarkdown,
				".gomdlint.yml": "flavor: bogus\n",
			},
			args:       []string{"lint", "ok.md"},
			want:       65,
			wantStderr: "invalid flavor",
		},
		{
			name:   "interrupted run exits 70",
			files:  map[string]string{"ok.md": cleanMarkdown},
			args:   []string{"lint", "ok.md"},
			cancel: true,
			want:   70,
		},
		{
			name:       "unreadable file exits 74",
			files:      map[string]string{"ok.md": cleanMarkdown, "locked.md": cleanMarkdown},
			args:       []string{"lint", "ok.md", "locked.md"},
			unreadable: "locked.md",
			want:       74,
		},
		{
			name:       "missing path exits 74",
			args:       []string{"lint", "missing.md"},
			want:       74,
			wantStderr: "missing.md",
		},
		{
			name: "file errors outrank error findings",
			files: map[string]string{
				"warn.md":       trailingMarkdown,
				"locked.md":     cleanMarkdown,
				".gomdlint.yml": "rules:\n  MD009:\n    severity: error\n",
			},
			args:       []string{"lint", "warn.md", "locked.md"},
			unreadable: "locked.md",
			want:       74,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unreadable != "" && (os.Geteuid() == 0 || runtime.GOOS == "windows") {
				t.Skip("permission bits do not stop root or Windows from reading files")
			}

			dir := isolate(t)
			for name, content := range tt.files {
				writeFile(t, filepath.Join(dir, name), content)
			}
			if tt.unreadable != "" {
				locked := filepath.Join(dir, tt.unreadable)
				require.NoError(t, os.Chmod(locked, 0))
				t.Cleanup(func() { assert.NoError(t, os.Chmod(locked, 0o600)) })
			}

			ctx := context.Background()
			if tt.cancel {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}

			var stdout, stderr bytes.Buffer
			got := run(ctx, tt.args, &stdout, &stderr)

			assert.Equal(t, tt.want, got, "stdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
			if tt.wantStderr != "" {
				assert.Contains(t, stderr.String(), tt.wantStderr)
			}
		})
	}
}

// TestRunFindingsAreNotReportedAsErrors keeps lint findings out of the error
// stream: the report already describes them.
func TestRunFindingsAreNotReportedAsErrors(t *testing.T) {
	dir := isolate(t)
	writeFile(t, filepath.Join(dir, "warn.md"), trailingMarkdown)

	var stdout, stderr bytes.Buffer
	got := run(context.Background(), []string{"lint", "--strict", "--color", "never", "warn.md"}, &stdout, &stderr)

	assert.Equal(t, 2, got)
	assert.Contains(t, stdout.String(), "no-trailing-spaces")
	assert.NotContains(t, stderr.String(), "Error")
}
