// Package cli provides the Cobra command structure for gomdlint.
package cli

import (
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
)

// HelpStyles contains Lipgloss styles for command help formatting.
type HelpStyles struct {
	// Command name/usage styling
	Command lipgloss.Style

	// Section headers (Usage, Available Commands, Flags, etc.)
	Heading lipgloss.Style

	// Subcommand names
	Subcommand lipgloss.Style

	// Flag names (--flag, -f)
	Flag lipgloss.Style

	// Flag/command descriptions
	Description lipgloss.Style

	// Examples section
	Example lipgloss.Style

	// Aliases
	Alias lipgloss.Style

	// Dim text (secondary info)
	Dim lipgloss.Style
}

// NewHelpStyles creates help styles based on color mode.
func NewHelpStyles(colorEnabled bool) *HelpStyles {
	if !colorEnabled {
		return newNoColorHelpStyles()
	}
	return newColorHelpStyles()
}

func newColorHelpStyles() *HelpStyles {
	return &HelpStyles{
		Command:     lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true),
		Heading:     lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
		Subcommand:  lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		Flag:        lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
		Description: lipgloss.NewStyle(),
		Example:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Alias:       lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Dim:         lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	}
}

func newNoColorHelpStyles() *HelpStyles {
	plain := lipgloss.NewStyle()
	return &HelpStyles{
		Command:     plain,
		Heading:     plain,
		Subcommand:  plain,
		Flag:        plain,
		Description: plain,
		Example:     plain,
		Alias:       plain,
		Dim:         plain,
	}
}

// HelpFormatter provides styled help output for Cobra commands.
type HelpFormatter struct {
	styles       *HelpStyles
	colorEnabled bool
}

// NewHelpFormatter creates a new help formatter with the given color mode.
func NewHelpFormatter(colorMode string, writer io.Writer) *HelpFormatter {
	colorEnabled := pretty.IsColorEnabled(colorMode, writer)
	return &HelpFormatter{
		styles:       NewHelpStyles(colorEnabled),
		colorEnabled: colorEnabled,
	}
}

// templateFuncs returns template functions for styled help rendering.
func (h *HelpFormatter) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"styleCommand":            h.styles.Command.Render,
		"styleHeading":            h.styles.Heading.Render,
		"styleSubcommand":         h.styles.Subcommand.Render,
		"styleFlag":               h.styles.Flag.Render,
		"styleDescription":        h.styles.Description.Render,
		"styleExample":            h.styles.Example.Render,
		"styleAlias":              h.styles.Alias.Render,
		"styleDim":                h.styles.Dim.Render,
		"rpad":                    rpad,
		"trimTrailingWhitespaces": trimTrailingWhitespaces,
	}
}

// usageTemplate returns the styled usage template.
func (h *HelpFormatter) usageTemplate() string {
	return `{{ styleHeading "Usage:" }}
  {{if .Runnable}}{{ styleCommand .UseLine }}{{end}}
  {{if .HasAvailableSubCommands}}{{ styleCommand .CommandPath }} [command]{{end}}

{{- if gt (len .Aliases) 0}}

{{ styleHeading "Aliases:" }}
  {{ styleAlias (join .Aliases ", ") }}
{{- end}}

{{- if .HasExample}}

{{ styleHeading "Examples:" }}
{{ styleExample .Example }}
{{- end}}

{{- if .HasAvailableSubCommands}}

{{ styleHeading "Available Commands:" }}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{ styleSubcommand (rpad .Name .NamePadding) }} {{ styleDescription .Short }}{{end}}{{end}}
{{- end}}

{{- if .HasAvailableLocalFlags}}

{{ styleHeading "Flags:" }}
{{ styleFlagsUsage .LocalFlags }}
{{- end}}

{{- if .HasAvailableInheritedFlags}}

{{ styleHeading "Global Flags:" }}
{{ styleFlagsUsage .InheritedFlags }}
{{- end}}

{{- if .HasHelpSubCommands}}

{{ styleHeading "Additional help topics:" }}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{ styleSubcommand (rpad .CommandPath .CommandPathPadding) }} {{ styleDescription .Short }}{{end}}{{end}}
{{- end}}

{{- if .HasAvailableSubCommands}}

Use "{{ styleCommand (print .CommandPath " [command] --help") }}" for more information about a command.
{{- end}}
`
}

// helpTemplate returns the styled help template.
func (h *HelpFormatter) helpTemplate() string {
	return `{{if or .Runnable .HasSubCommands}}{{ styleCommand .CommandPath }}{{if .Version}} {{ styleDim .Version }}{{end}}

{{end}}{{with (or .Long .Short)}}{{ . | trimTrailingWhitespaces }}

{{end}}` + h.usageTemplate()
}

// styleFlagsUsage formats flag usage with styling.
func (h *HelpFormatter) styleFlagsUsage(flags interface{}) string {
	// Get the FlagUsages string from pflags
	flagUsages, ok := flags.(interface{ FlagUsages() string })
	if !ok {
		return ""
	}

	usages := flagUsages.FlagUsages()
	if usages == "" {
		return ""
	}

	var result strings.Builder
	lines := strings.Split(strings.TrimSuffix(usages, "\n"), "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Parse flag line: typically "  -f, --flag type   description"
		styled := h.styleFlagLine(line)
		result.WriteString(styled)
	}

	return result.String()
}

// styleFlagLine applies styling to a single flag usage line.
func (h *HelpFormatter) styleFlagLine(line string) string {
	if strings.TrimSpace(line) == "" {
		return line
	}

	// Find where the description starts (after multiple spaces)
	// Typical format: "  -f, --flag type   Description here"
	trimmed := strings.TrimLeft(line, " ")
	leadingSpaces := len(line) - len(trimmed)

	// Find the boundary between flag+type and description
	// Look for 2+ consecutive spaces after the flag definition
	parts := splitFlagLine(trimmed)
	if len(parts) != 2 {
		return line
	}

	flagPart := parts[0]
	descPart := parts[1]

	// Style the flag part
	styledFlag := h.styleFlagPart(flagPart)

	// Rebuild with original spacing structure
	prefix := strings.Repeat(" ", leadingSpaces)
	spacing := "   " // Standard spacing between flag and description

	return prefix + styledFlag + spacing + h.styles.Description.Render(descPart)
}

// splitFlagLine splits a flag line into [flagPart, description].
func splitFlagLine(line string) []string {
	// Find the first occurrence of 2+ spaces followed by non-space
	inSpaces := false
	spaceStart := -1
	minSpaceGap := 2 // Minimum consecutive spaces to identify boundary

	for idx, char := range line {
		if char == ' ' {
			if !inSpaces {
				inSpaces = true
				spaceStart = idx
			}
		} else {
			if inSpaces && idx-spaceStart >= minSpaceGap {
				// Found the boundary
				return []string{
					strings.TrimRight(line[:spaceStart], " "),
					line[idx:],
				}
			}
			inSpaces = false
		}
	}

	return []string{line}
}

// styleFlagPart styles the flag portion of a flag line.
func (h *HelpFormatter) styleFlagPart(flagPart string) string {
	// Split into tokens and style each flag (-f, --flag) in color
	var result strings.Builder
	tokens := strings.Fields(flagPart)

	for i, token := range tokens {
		if i > 0 {
			result.WriteString(" ")
		}

		if strings.HasPrefix(token, "-") {
			// Remove trailing comma for styling, add back after
			hasComma := strings.HasSuffix(token, ",")
			clean := strings.TrimSuffix(token, ",")
			result.WriteString(h.styles.Flag.Render(clean))
			if hasComma {
				result.WriteString(",")
			}
		} else {
			// Type indicator (string, int, etc.) - dim it
			result.WriteString(h.styles.Dim.Render(token))
		}
	}

	return result.String()
}

// ApplyToCommand applies styled help templates to a Cobra command and all subcommands.
func (h *HelpFormatter) ApplyToCommand(cmd *cobra.Command) {
	// Create template funcs with flag styling
	funcs := h.templateFuncs()
	funcs["styleFlagsUsage"] = h.styleFlagsUsage
	funcs["join"] = strings.Join

	// Set the custom templates
	cmd.SetUsageTemplate(h.usageTemplate())
	cmd.SetHelpTemplate(h.helpTemplate())

	// Apply funcs by setting them on the usage/help functions
	cmd.SetUsageFunc(func(command *cobra.Command) error {
		usageTmpl := template.New("usage").Funcs(funcs)
		usageTmpl, err := usageTmpl.Parse(h.usageTemplate())
		if err != nil {
			return fmt.Errorf("parse usage template: %w", err)
		}
		return usageTmpl.Execute(command.OutOrStdout(), command)
	})

	cmd.SetHelpFunc(func(command *cobra.Command, _ []string) {
		helpTmpl := template.New("help").Funcs(funcs)
		helpTmpl, err := helpTmpl.Parse(h.helpTemplate())
		if err != nil {
			command.PrintErrln(err)
			return
		}
		if err := helpTmpl.Execute(command.OutOrStdout(), command); err != nil {
			command.PrintErrln(err)
		}
	})
}

// rpad adds padding to the right of a string.
func rpad(str string, padding int) string {
	if len(str) >= padding {
		return str
	}
	return str + strings.Repeat(" ", padding-len(str))
}

// trimTrailingWhitespaces removes trailing whitespace from lines.
func trimTrailingWhitespaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}
