// Package pretty provides Lipgloss-based styled output utilities.
package pretty

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
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

// NewStyles creates a new Styles with the given color mode.
func NewStyles(colorEnabled bool) *Styles {
	if !colorEnabled {
		return newNoColorStyles()
	}
	return newColorStyles()
}

// newColorStyles creates styles with ANSI 256 colors.
func newColorStyles() *Styles {
	return &Styles{
		// Severity colors
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
		Info:    lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),

		// Diagnostic components
		FilePath:   lipgloss.NewStyle().Bold(true),
		Location:   lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		RuleID:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Message:    lipgloss.NewStyle(),
		Suggestion: lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Italic(true),
		SourceLine: lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Caret:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),

		// Diff styles
		DiffHeader:  lipgloss.NewStyle().Bold(true),
		DiffHunk:    lipgloss.NewStyle().Foreground(lipgloss.Color("14")),
		DiffAdd:     lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		DiffRemove:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		DiffContext: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),

		// Summary styles
		SummaryTitle: lipgloss.NewStyle().Bold(true),
		SummaryValue: lipgloss.NewStyle(),
		Success:      lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),
		Failure:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),

		// Table styles - using foreground colors for severity
		TableHeader:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("7")),
		TableBorder:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		TableErrorRow:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")),  // Red text
		TableWarnRow:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")), // Yellow text
		TableInfoRow:   lipgloss.NewStyle().Foreground(lipgloss.Color("12")), // Blue text
		TableFixable:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")), // Green
		TableLegend:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true),
		TableSeparator: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),

		// Misc
		Dim:  lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Bold: lipgloss.NewStyle().Bold(true),
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

// IsColorEnabled determines if color should be enabled based on mode and writer.
// Mode values: "auto" (default), "always", "never".
// In auto mode, color is enabled only if the writer is a TTY and NO_COLOR is not set.
func IsColorEnabled(mode string, writer io.Writer) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		// Check NO_COLOR environment variable (https://no-color.org/)
		if os.Getenv("NO_COLOR") != "" {
			return false
		}
		// Check if output is a TTY
		if f, ok := writer.(*os.File); ok {
			return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
		}
		return false
	}
}
