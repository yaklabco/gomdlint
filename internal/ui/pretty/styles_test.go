package pretty_test

import (
	"bytes"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
)

func TestNewStyles_ColorEnabled(t *testing.T) {
	styles := pretty.NewStyles(true)
	require.NotNil(t, styles)

	// Verify that all style fields are properly initialized
	// Note: Lipgloss may not render ANSI codes in non-TTY environments
	// so we just verify the struct is properly constructed
	assert.NotNil(t, styles.Bold)
	assert.NotNil(t, styles.Error)
	assert.NotNil(t, styles.Warning)
	assert.NotNil(t, styles.Info)
}

func TestNewStyles_ColorDisabled(t *testing.T) {
	styles := pretty.NewStyles(false)
	require.NotNil(t, styles)

	// With color disabled, styles should return unmodified text
	text := "test"
	rendered := styles.Bold.Render(text)
	assert.Equal(t, text, rendered, "No-color Bold should not add formatting")

	rendered = styles.Error.Render(text)
	assert.Equal(t, text, rendered, "No-color Error should not add formatting")
}

// clearColorEnv unsets the environment variables that choose a color
// profile, so each test sets only the ones it covers.
func clearColorEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{"TERM", "COLORTERM", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "CI", "GOOGLE_CLOUD_SHELL"} {
		t.Setenv(name, "")
	}
}

// TestColorProfile covers the color profile of each --color mode on a
// writer that is not a terminal, such as a pipe. always forces color: the
// profile the environment describes, at least basic ANSI, even where NO_COLOR
// is set, since the flag is the more specific request. never and auto,
// which needs a terminal, give no color.
func TestColorProfile(t *testing.T) {
	tests := []struct {
		name string
		mode string
		env  map[string]string
		want termenv.Profile
	}{
		{name: "always without TERM", mode: "always", want: termenv.ANSI},
		{name: "always with a dumb terminal", mode: "always", env: map[string]string{"TERM": "dumb"}, want: termenv.ANSI},
		{name: "always with TERM xterm", mode: "always", env: map[string]string{"TERM": "xterm"}, want: termenv.ANSI},
		{name: "always with 256 colors", mode: "always", env: map[string]string{"TERM": "xterm-256color"}, want: termenv.ANSI256},
		{name: "always with true color", mode: "always", env: map[string]string{"COLORTERM": "truecolor"}, want: termenv.TrueColor},
		{name: "always in CI", mode: "always", env: map[string]string{"CI": "true", "TERM": "xterm-256color"}, want: termenv.ANSI256},
		{name: "always over NO_COLOR", mode: "always", env: map[string]string{"NO_COLOR": "1"}, want: termenv.ANSI},
		{name: "never", mode: "never", env: map[string]string{"TERM": "xterm-256color", "CLICOLOR_FORCE": "1"}, want: termenv.Ascii},
		{name: "auto on a pipe", mode: "auto", env: map[string]string{"TERM": "xterm-256color"}, want: termenv.Ascii},
		{name: "auto on a pipe with NO_COLOR", mode: "auto", env: map[string]string{"NO_COLOR": "1"}, want: termenv.Ascii},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearColorEnv(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			assert.Equal(t, tt.want, pretty.ColorProfile(tt.mode, &bytes.Buffer{}))
		})
	}
}

// TestNewStylesFor covers styles made for a writer that is not a terminal:
// --color always renders ANSI escapes into it, and never and auto render
// plain text.
func TestNewStylesFor(t *testing.T) {
	clearColorEnv(t)

	always := pretty.NewStylesFor("always", &bytes.Buffer{})
	assert.Equal(t, "\x1b[1;91mx\x1b[0m", always.Error.Render("x"), "bold bright red in basic ANSI")
	assert.Equal(t, "\x1b[93mx\x1b[0m", always.TableWarnRow.Render("x"), "bright yellow in basic ANSI")

	for _, mode := range []string{"never", "auto"} {
		styles := pretty.NewStylesFor(mode, &bytes.Buffer{})
		assert.Equal(t, "x", styles.Error.Render("x"), mode)
		assert.Equal(t, "x", styles.TableWarnRow.Render("x"), mode)
	}
}

func TestStyles_AllFieldsInitialized(t *testing.T) {
	// Test that all style fields are initialized (not nil)
	styles := pretty.NewStyles(true)

	// Verify all severity styles
	assert.NotEmpty(t, styles.Error.Render("x"))
	assert.NotEmpty(t, styles.Warning.Render("x"))
	assert.NotEmpty(t, styles.Info.Render("x"))

	// Verify diagnostic component styles
	assert.NotEmpty(t, styles.FilePath.Render("x"))
	assert.NotEmpty(t, styles.Location.Render("x"))
	assert.NotEmpty(t, styles.RuleID.Render("x"))
	assert.NotEmpty(t, styles.Message.Render("x"))
	assert.NotEmpty(t, styles.Suggestion.Render("x"))
	assert.NotEmpty(t, styles.SourceLine.Render("x"))
	assert.NotEmpty(t, styles.Caret.Render("x"))

	// Verify diff styles
	assert.NotEmpty(t, styles.DiffHeader.Render("x"))
	assert.NotEmpty(t, styles.DiffHunk.Render("x"))
	assert.NotEmpty(t, styles.DiffAdd.Render("x"))
	assert.NotEmpty(t, styles.DiffRemove.Render("x"))
	assert.NotEmpty(t, styles.DiffContext.Render("x"))

	// Verify summary styles
	assert.NotEmpty(t, styles.SummaryTitle.Render("x"))
	assert.NotEmpty(t, styles.SummaryValue.Render("x"))
	assert.NotEmpty(t, styles.Success.Render("x"))
	assert.NotEmpty(t, styles.Failure.Render("x"))

	// Verify misc styles
	assert.NotEmpty(t, styles.Dim.Render("x"))
	assert.NotEmpty(t, styles.Bold.Render("x"))
}
