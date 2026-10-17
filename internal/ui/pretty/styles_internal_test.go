package pretty

import (
	"bytes"
	"io"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

// TestColorProfileOnTerminal covers the color profile of output to a
// terminal, which tests can only fake: auto, the default and any unknown
// mode, gives the profile the environment describes unless NO_COLOR or
// CLICOLOR=0 rules color out. A terminal in CI still gets color, and so
// does a Cygwin or MSYS terminal, which plain isatty does not recognize.
func TestColorProfileOnTerminal(t *testing.T) {
	terminal := func(io.Writer) bool { return true }

	tests := []struct {
		name string
		mode string
		env  map[string]string
		want termenv.Profile
	}{
		{name: "auto with 256 colors", mode: "auto", env: map[string]string{"TERM": "xterm-256color"}, want: termenv.ANSI256},
		{name: "auto with true color", mode: "auto", env: map[string]string{"COLORTERM": "truecolor"}, want: termenv.TrueColor},
		{name: "auto with TERM xterm", mode: "auto", env: map[string]string{"TERM": "xterm"}, want: termenv.ANSI},
		{name: "auto on a dumb terminal", mode: "auto", env: map[string]string{"TERM": "dumb"}, want: termenv.Ascii},
		{name: "auto with NO_COLOR", mode: "auto", env: map[string]string{"TERM": "xterm-256color", "NO_COLOR": "1"}, want: termenv.Ascii},
		{name: "auto with CLICOLOR=0", mode: "auto", env: map[string]string{"TERM": "xterm-256color", "CLICOLOR": "0"}, want: termenv.Ascii},
		{name: "auto in CI", mode: "auto", env: map[string]string{"TERM": "xterm-256color", "CI": "true"}, want: termenv.ANSI256},
		{name: "default mode", mode: "", env: map[string]string{"TERM": "xterm-256color"}, want: termenv.ANSI256},
		{name: "unknown mode", mode: "sometimes", env: map[string]string{"TERM": "xterm-256color"}, want: termenv.ANSI256},
		{name: "never", mode: "never", env: map[string]string{"TERM": "xterm-256color"}, want: termenv.Ascii},
		{name: "always without TERM", mode: "always", want: termenv.ANSI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, name := range []string{"TERM", "COLORTERM", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "CI", "GOOGLE_CLOUD_SHELL"} {
				t.Setenv(name, "")
			}
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			assert.Equal(t, tt.want, colorProfile(tt.mode, &bytes.Buffer{}, terminal))
		})
	}
}
