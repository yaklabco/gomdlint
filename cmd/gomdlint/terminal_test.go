package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	helperEnv     = "GOMDLINT_TEST_HELPER_ARGS"
	helperArgsSep = "\x1f"
)

// TestHelperProcess runs gomdlint with the arguments in GOMDLINT_TEST_HELPER_ARGS.
// It does nothing in a normal test run; TestNoTerminalColorQueries starts the
// test binary with it under a pseudo-terminal.
func TestHelperProcess(t *testing.T) {
	args, ok := os.LookupEnv(helperEnv)
	if !ok {
		t.Skip("helper process for TestNoTerminalColorQueries")
	}
	//nolint:revive // The helper process exits with gomdlint's exit code.
	os.Exit(run(context.Background(), strings.Split(args, helperArgsSep), os.Stdout, os.Stderr))
}

// TestNoTerminalColorQueries runs gomdlint on a pseudo-terminal and checks
// that it never queries the terminal for its colors. The queries (OSC 10
// and 11, fenced by a cursor position report) ignore NO_COLOR and stall
// terminals that do not answer them.
func TestNoTerminalColorQueries(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs the util-linux script command")
	}
	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script is not installed")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Title \n\nText.\n"), 0o644))

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"version"}, want: "gomdlint dev"},
		{args: []string{"rules"}, want: "no-trailing-spaces"},
		{args: []string{"lint", "doc.md"}, want: "Trailing whitespace"},
	}

	for _, tt := range tests {
		args := tt.args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			helper := shellQuote(os.Args[0]) + " -test.run=^TestHelperProcess$"
			cmd := exec.CommandContext(ctx, script, "-q", "-e", "-c", helper, "/dev/null")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(),
				helperEnv+"="+strings.Join(args, helperArgsSep),
				"NO_COLOR=1",
				"SHELL=/bin/sh",
				"TERM=xterm-256color", // Terminal multiplexers would suppress the queries.
				"CI=",                 // So would CI.
				"XDG_CONFIG_HOME="+filepath.Join(dir, ".xdg"),
			)

			start := time.Now()
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(start)
			require.NoError(t, ctx.Err(), "gomdlint hung on the terminal:\n%q", out)
			if err != nil {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr, "output:\n%q", out)
			}

			output := string(out)
			for _, query := range []struct{ name, sequence string }{
				{name: "foreground color query", sequence: "\x1b]10;?"},
				{name: "background color query", sequence: "\x1b]11;?"},
				{name: "cursor position report", sequence: "\x1b[6n"},
			} {
				if strings.Contains(output, query.sequence) {
					t.Errorf("sent a %s: %q", query.name, head(output))
				}
			}
			assert.Less(t, elapsed, 8*time.Second, "each unanswered query stalls for 5 seconds")
			if !strings.Contains(output, tt.want) {
				t.Errorf("the command did not run: %q", head(output))
			}
		})
	}
}

// head shortens output for failure messages.
func head(output string) string {
	const limit = 200
	if len(output) > limit {
		return output[:limit] + "..."
	}
	return output
}

// shellQuote quotes path for /bin/sh.
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
