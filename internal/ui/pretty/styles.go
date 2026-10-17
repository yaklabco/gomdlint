// Package pretty provides Lipgloss-based styled output utilities.
package pretty

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// The values of the --color flag.
const (
	colorAlways = "always"
	colorNever  = "never"
)

// Styles contains all styled renderers for CLI output.
type Styles struct {
	// Severity styles
	Error   lipgloss.Style
	Warning lipgloss.Style
	Info    lipgloss.Style

	// Diagnostic components
	FilePath   lipgloss.Style
	Location   lipgloss.Style
	RuleID     lipgloss.Style
	Message    lipgloss.Style
	Suggestion lipgloss.Style
	SourceLine lipgloss.Style
	Caret      lipgloss.Style

	// Diff styles
	DiffHeader  lipgloss.Style
	DiffHunk    lipgloss.Style
	DiffAdd     lipgloss.Style
	DiffRemove  lipgloss.Style
	DiffContext lipgloss.Style

	// Summary styles
	SummaryTitle lipgloss.Style
	SummaryValue lipgloss.Style
	Success      lipgloss.Style
	Failure      lipgloss.Style

	// Table styles
	TableHeader    lipgloss.Style
	TableBorder    lipgloss.Style
	TableErrorRow  lipgloss.Style
	TableWarnRow   lipgloss.Style
	TableInfoRow   lipgloss.Style
	TableFixable   lipgloss.Style
	TableLegend    lipgloss.Style
	TableSeparator lipgloss.Style

	// Misc
	Dim  lipgloss.Style
	Bold lipgloss.Style
}

// NewStyles creates styles that render color, in basic ANSI on any
// writer, when colorEnabled, and plain text otherwise. Reporters use
// NewStylesFor, which follows the --color flag and the writer.
func NewStyles(colorEnabled bool) *Styles {
	if !colorEnabled {
		return newNoColorStyles()
	}
	return newColorStyles(NewRenderer(io.Discard, termenv.ANSI))
}

// NewStylesFor returns the styles for output to writer under the --color
// mode: "always", "never" or "auto". They render in the color profile that
// ColorProfile chooses, and render plain text where it allows no color.
func NewStylesFor(mode string, writer io.Writer) *Styles {
	profile := ColorProfile(mode, writer)
	if profile == termenv.Ascii {
		return newNoColorStyles()
	}
	return newColorStyles(NewRenderer(writer, profile))
}

// NewRenderer returns a renderer for writer that renders in profile,
// whatever writer is. Styles made with lipgloss.NewStyle render for the
// process's standard output instead, in the profile detected for it, which
// has no color when standard output is not a terminal.
func NewRenderer(writer io.Writer, profile termenv.Profile) *lipgloss.Renderer {
	renderer := lipgloss.NewRenderer(writer)
	renderer.SetColorProfile(profile)
	return renderer
}

// ColorProfile returns the color profile of output to writer under the
// --color mode:
//
//   - "never" gives no color (termenv.Ascii).
//   - "always" gives color whatever writer is, so piped output keeps its
//     color: the profile that TERM and COLORTERM describe, or basic ANSI,
//     the 16 colors every color terminal shows, where they describe none.
//     The styles use only those 16 colors, so a richer profile renders them
//     the same. It also gives color where NO_COLOR is set, since an
//     explicit flag asks for color for this run, as https://no-color.org
//     recommends.
//   - "auto", the default and any other value, gives color only on a
//     terminal where NO_COLOR is not set: the profile its environment
//     describes, which may be none, for example for TERM=dumb.
func ColorProfile(mode string, writer io.Writer) termenv.Profile {
	return colorProfile(mode, writer, isTerminal)
}

// colorProfile is ColorProfile with terminal deciding whether a writer is a
// terminal, which tests fake.
func colorProfile(mode string, writer io.Writer, terminal func(io.Writer) bool) termenv.Profile {
	switch mode {
	case colorNever:
		return termenv.Ascii
	case colorAlways:
		// Assuming a terminal makes the environment choose the profile.
		// An explicit profile skips the NO_COLOR check of the default one.
		described := termenv.NewOutput(writer, termenv.WithTTY(true), termenv.WithProfile(termenv.Ascii)).ColorProfile()
		// Profiles with more colors have lower values.
		return min(described, termenv.ANSI)
	default:
		if os.Getenv("NO_COLOR") != "" || !terminal(writer) {
			return termenv.Ascii
		}
		// writer is a terminal: termenv must not decide otherwise, as its
		// plain isatty check does for Cygwin and MSYS terminals, and its
		// CI check does for any terminal in CI.
		return termenv.NewOutput(writer, termenv.WithTTY(true)).EnvColorProfile()
	}
}

// isTerminal reports whether writer is a file attached to a terminal.
func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}

// newColorStyles creates styles that render with renderer. They use only
// the 16 basic ANSI colors.
func newColorStyles(renderer *lipgloss.Renderer) *Styles {
	style := renderer.NewStyle
	return &Styles{
		// Severity colors
		Error:   style().Foreground(lipgloss.Color("9")).Bold(true),
		Warning: style().Foreground(lipgloss.Color("11")).Bold(true),
		Info:    style().Foreground(lipgloss.Color("12")).Bold(true),

		// Diagnostic components
		FilePath:   style().Bold(true),
		Location:   style().Foreground(lipgloss.Color("8")),
		RuleID:     style().Foreground(lipgloss.Color("8")),
		Message:    style(),
		Suggestion: style().Foreground(lipgloss.Color("10")).Italic(true),
		SourceLine: style().Foreground(lipgloss.Color("7")),
		Caret:      style().Foreground(lipgloss.Color("9")),

		// Diff styles
		DiffHeader:  style().Bold(true),
		DiffHunk:    style().Foreground(lipgloss.Color("14")),
		DiffAdd:     style().Foreground(lipgloss.Color("10")),
		DiffRemove:  style().Foreground(lipgloss.Color("9")),
		DiffContext: style().Foreground(lipgloss.Color("8")),

		// Summary styles
		SummaryTitle: style().Bold(true),
		SummaryValue: style(),
		Success:      style().Foreground(lipgloss.Color("10")).Bold(true),
		Failure:      style().Foreground(lipgloss.Color("9")).Bold(true),

		// Table styles - using foreground colors for severity
		TableHeader:    style().Bold(true).Foreground(lipgloss.Color("7")),
		TableBorder:    style().Foreground(lipgloss.Color("8")),
		TableErrorRow:  style().Foreground(lipgloss.Color("9")),  // Red text
		TableWarnRow:   style().Foreground(lipgloss.Color("11")), // Yellow text
		TableInfoRow:   style().Foreground(lipgloss.Color("12")), // Blue text
		TableFixable:   style().Foreground(lipgloss.Color("10")), // Green
		TableLegend:    style().Foreground(lipgloss.Color("8")).Italic(true),
		TableSeparator: style().Foreground(lipgloss.Color("8")),

		// Misc
		Dim:  style().Foreground(lipgloss.Color("8")),
		Bold: style().Bold(true),
	}
}

// newNoColorStyles creates styles with no color formatting.
func newNoColorStyles() *Styles {
	plain := lipgloss.NewStyle()
	return &Styles{
		Error:          plain,
		Warning:        plain,
		Info:           plain,
		FilePath:       plain,
		Location:       plain,
		RuleID:         plain,
		Message:        plain,
		Suggestion:     plain,
		SourceLine:     plain,
		Caret:          plain,
		DiffHeader:     plain,
		DiffHunk:       plain,
		DiffAdd:        plain,
		DiffRemove:     plain,
		DiffContext:    plain,
		SummaryTitle:   plain,
		SummaryValue:   plain,
		Success:        plain,
		Failure:        plain,
		TableHeader:    plain,
		TableBorder:    plain,
		TableErrorRow:  plain,
		TableWarnRow:   plain,
		TableInfoRow:   plain,
		TableFixable:   plain,
		TableLegend:    plain,
		TableSeparator: plain,
		Dim:            plain,
		Bold:           plain,
	}
}
