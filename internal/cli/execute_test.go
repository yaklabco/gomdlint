package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// execution is the observable outcome of one command line.
type execution struct {
	code   int
	stdout string
	stderr string
}

func execute(t *testing.T, args ...string) execution {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.Execute(context.Background(), cli.BuildInfo{Version: "test"}, args, &stdout, &stderr)
	return execution{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: cli.ExitSuccess},
		{name: "unclassified errors come from argument parsing", err: errors.New("unknown command"), want: cli.ExitInvalidUsage},
		{name: "classified", err: &cli.ExitError{Code: cli.ExitConfigError, Err: errors.New("bad")}, want: cli.ExitConfigError},
		{
			name: "classified and wrapped",
			err:  fmt.Errorf("context: %w", &cli.ExitError{Code: cli.ExitIOError, Err: errors.New("read")}),
			want: cli.ExitIOError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cli.ExitCode(tt.err))
		})
	}
}

func TestExitCodeFromResult(t *testing.T) {
	t.Parallel()

	stats := func(errored, errs, warnings int) *runner.Result {
		return &runner.Result{Stats: runner.Stats{
			FilesErrored:          errored,
			DiagnosticsBySeverity: map[string]int{"error": errs, "warning": warnings},
		}}
	}

	tests := []struct {
		name   string
		result *runner.Result
		strict bool
		want   int
	}{
		{name: "nil result", result: nil, want: cli.ExitSuccess},
		{name: "clean", result: stats(0, 0, 0), want: cli.ExitSuccess},
		{name: "warnings pass without strict", result: stats(0, 0, 3), want: cli.ExitSuccess},
		{name: "warnings fail with strict", result: stats(0, 0, 3), strict: true, want: cli.ExitLintWarnings},
		{name: "errors", result: stats(0, 2, 3), want: cli.ExitLintErrors},
		{name: "errors outrank strict warnings", result: stats(0, 2, 3), strict: true, want: cli.ExitLintErrors},
		{name: "file errors outrank findings", result: stats(1, 2, 3), strict: true, want: cli.ExitIOError},
		{name: "file errors alone", result: stats(1, 0, 0), want: cli.ExitIOError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, cli.ExitCodeFromResult(tt.result, tt.strict))
		})
	}
}

// TestExecuteUsageErrors covers invalid command lines that never reach a
// file or configuration.
func TestExecuteUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "unknown command", args: []string{"bogus"}, wantStderr: `unknown command "bogus"`},
		{name: "unknown flag", args: []string{"lint", "--bogus"}, wantStderr: "unknown flag: --bogus"},
		{name: "invalid flavor", args: []string{"lint", "--flavor", "bogus"}, wantStderr: `invalid flavor "bogus"`},
		{name: "invalid rule format", args: []string{"lint", "--rule-format", "bogus"}, wantStderr: `invalid rule format "bogus"`},
		{name: "invalid summary order", args: []string{"lint", "--summary-order", "bogus"}, wantStderr: `invalid summary order "bogus"`},
		{name: "negative jobs", args: []string{"lint", "--jobs", "-1"}, wantStderr: "invalid jobs value -1"},
		{name: "malformed ignore pattern", args: []string{"lint", "--ignore", "docs/[a-"}, wantStderr: `"docs/[a-"`},
		{name: "invalid color", args: []string{"lint", "--color", "bogus"}, wantStderr: `invalid color mode "bogus"`},
		{name: "rules with invalid rule format", args: []string{"rules", "--rule-format", "bogus"}, wantStderr: `invalid rule format "bogus"`},
		{name: "migrate with two inputs", args: []string{"migrate", "a.json", "b.json"}, wantStderr: "accepts at most 1 arg"},
		{name: "init with invalid format", args: []string{"init", "--format", "toml"}, wantStderr: `invalid format "toml"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := execute(t, tt.args...)

			assert.Equal(t, cli.ExitInvalidUsage, got.code, "stderr: %s", got.stderr)
			assert.Contains(t, got.stderr, tt.wantStderr)
			assert.Contains(t, got.stderr, "--help' for usage.")
		})
	}
}

// TestExecuteFileCommandErrors covers init and migrate failures, which work
// on files in the current directory.
func TestExecuteFileCommandErrors(t *testing.T) {
	t.Run("init refuses to overwrite without force", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".gomdlint.yml"), []byte("flavor: gfm\n"), 0o644))

		got := execute(t, "init")

		assert.Equal(t, cli.ExitInvalidUsage, got.code)
		assert.Contains(t, got.stderr, "already exists")
	})

	t.Run("migrate with a missing input", func(t *testing.T) {
		t.Chdir(t.TempDir())

		got := execute(t, "migrate", "missing.json")

		assert.Equal(t, cli.ExitIOError, got.code)
		assert.Contains(t, got.stderr, "missing.json")
	})

	t.Run("migrate with an unconvertible input", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".markdownlint.cjs"), []byte("module.exports = {};\n"), 0o644))

		got := execute(t, "migrate", ".markdownlint.cjs")

		assert.Equal(t, cli.ExitConfigError, got.code)
		assert.Contains(t, got.stderr, "JavaScript")
	})

	t.Run("migrate with malformed input", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".markdownlint.json"), []byte("{not json"), 0o644))

		got := execute(t, "migrate", ".markdownlint.json")

		assert.Equal(t, cli.ExitConfigError, got.code)
	})
}
