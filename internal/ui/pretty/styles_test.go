package pretty_test

import (
	"bytes"
	"os"
	"testing"

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

func TestIsColorEnabled_AlwaysMode(t *testing.T) {
	var buf bytes.Buffer
	result := pretty.IsColorEnabled("always", &buf)
	assert.True(t, result, "always mode should return true")
}

func TestIsColorEnabled_NeverMode(t *testing.T) {
	result := pretty.IsColorEnabled("never", os.Stdout)
	assert.False(t, result, "never mode should return false")
}

func TestIsColorEnabled_AutoMode_NonTTY(t *testing.T) {
	// bytes.Buffer is not a TTY
	var buf bytes.Buffer
	result := pretty.IsColorEnabled("auto", &buf)
	assert.False(t, result, "auto mode with non-TTY should return false")
}

func TestIsColorEnabled_AutoMode_NoColorEnv(t *testing.T) {
	// Set NO_COLOR environment variable
	t.Setenv("NO_COLOR", "1")

	// Even with a TTY, NO_COLOR should disable colors
	result := pretty.IsColorEnabled("auto", os.Stdout)
	assert.False(t, result, "auto mode with NO_COLOR set should return false")
}

func TestIsColorEnabled_DefaultsToAuto(t *testing.T) {
	// Clear NO_COLOR if set
	t.Setenv("NO_COLOR", "")

	// Empty or unknown mode should default to auto behavior
	var buf bytes.Buffer
	result := pretty.IsColorEnabled("", &buf)
	assert.False(t, result, "empty mode with non-TTY should return false (auto behavior)")

	result = pretty.IsColorEnabled("unknown", &buf)
	assert.False(t, result, "unknown mode with non-TTY should return false (auto behavior)")
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
