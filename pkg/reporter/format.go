package reporter

import "fmt"

// Format represents an output format.
type Format string

// Output formats supported by the reporter.
const (
	FormatText    Format = "text"
	FormatTable   Format = "table"
	FormatJSON    Format = "json"
	FormatSARIF   Format = "sarif"
	FormatDiff    Format = "diff"
	FormatSummary Format = "summary"
)

// ParseFormat parses a format string, returning an error for unknown formats.
func ParseFormat(formatStr string) (Format, error) {
	switch formatStr {
	case "text", "":
		return FormatText, nil
	case "table":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	case "sarif":
		return FormatSARIF, nil
	case "diff":
		return FormatDiff, nil
	case "summary":
		return FormatSummary, nil
	default:
		return "", fmt.Errorf("unknown format %q; valid formats: text, table, json, sarif, diff, summary", formatStr)
	}
}

// String returns the string representation of the format.
func (f Format) String() string {
	return string(f)
}

// IsValid returns true if the format is a known valid format.
func (f Format) IsValid() bool {
	switch f {
	case FormatText, FormatTable, FormatJSON, FormatSARIF, FormatDiff, FormatSummary:
		return true
	default:
		return false
	}
}
