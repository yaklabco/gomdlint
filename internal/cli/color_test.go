package cli_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ansiEscape matches the escape sequences that color output.
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// inColorProject is inProject for a document with findings and fixes,
// with the environment variables that choose a color profile cleared, so
// each test sets only the ones it covers.
func inColorProject(t *testing.T) {
	t.Helper()

	inProject(t, map[string]string{"doc.md": trailingDoc})
	for _, name := range []string{"TERM", "COLORTERM", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "CI"} {
		t.Setenv(name, "")
	}
}

// TestLintColorAlways covers --color always on output that is not a
// terminal, such as a pipe: every human format colors its output, whatever
// TERM says, and even where NO_COLOR is set, since the flag asks for color
// for this run. Without the escapes, the output is what --color never
// prints.
func TestLintColorAlways(t *testing.T) {
	for _, format := range []string{"text", "summary", "diff"} {
		for name, env := range map[string]map[string]string{
			"without TERM":      nil,
			"with 256 colors":   {"TERM": "xterm-256color"},
			"over NO_COLOR":     {"NO_COLOR": "1"},
			"on a dumb console": {"TERM": "dumb"},
		} {
			t.Run(format+" "+name, func(t *testing.T) {
				inColorProject(t)
				for key, value := range env {
					t.Setenv(key, value)
				}

				colored := execute(t, "lint", "--color", "always", "--format", format, "doc.md")
				plain := execute(t, "lint", "--color", "never", "--format", format, "doc.md")

				assert.Regexp(t, ansiEscape, colored.stdout)
				assert.NotRegexp(t, ansiEscape, plain.stdout)
				assert.Equal(t, plain.stdout, ansiEscape.ReplaceAllString(colored.stdout, ""))
				assert.Equal(t, plain.code, colored.code)
			})
		}
	}

	t.Run("table", func(t *testing.T) {
		inColorProject(t)

		got := execute(t, "lint", "--color", "always", "--format", "table", "doc.md")

		assert.Regexp(t, ansiEscape, got.stdout)
	})
}

// TestLintColorOff covers output without color: --color never, auto on
// output that is not a terminal, and the machine formats, which never
// color, even under --color always.
func TestLintColorOff(t *testing.T) {
	for _, format := range []string{"text", "table", "summary", "diff"} {
		for _, args := range [][]string{{"--color", "never"}, {"--color", "auto"}, {}} {
			t.Run(format+" "+argsName(args), func(t *testing.T) {
				inColorProject(t)
				t.Setenv("TERM", "xterm-256color")
				t.Setenv("CLICOLOR_FORCE", "1")

				got := execute(t, append(append([]string{"lint"}, args...), "--format", format, "doc.md")...)

				assert.NotEmpty(t, got.stdout)
				assert.NotRegexp(t, ansiEscape, got.stdout)
			})
		}
	}

	for _, format := range []string{"json", "sarif"} {
		t.Run(format+" with --color always", func(t *testing.T) {
			inColorProject(t)

			got := execute(t, "lint", "--color", "always", "--format", format, "doc.md")

			require.NotEmpty(t, got.stdout)
			assert.NotContains(t, got.stdout, "\x1b")
		})
	}
}

// TestHelpColor covers help output, which follows --color like lint output:
// help used to choose its colors before the flags were parsed.
func TestHelpColor(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"lint", "--help"}, {"help", "lint"}} {
		t.Run(argsName(args), func(t *testing.T) {
			inColorProject(t)

			always := execute(t, append([]string{"--color", "always"}, args...)...)
			never := execute(t, append([]string{"--color", "never"}, args...)...)
			auto := execute(t, args...)

			assert.Regexp(t, ansiEscape, always.stdout)
			assert.NotRegexp(t, ansiEscape, never.stdout)
			assert.NotRegexp(t, ansiEscape, auto.stdout, "auto colors only a terminal")
			assert.Equal(t, never.stdout, ansiEscape.ReplaceAllString(always.stdout, ""))
		})
	}
}

// argsName names a subtest after command-line arguments.
func argsName(args []string) string {
	if len(args) == 0 {
		return "default"
	}
	return strings.Join(args, " ")
}
