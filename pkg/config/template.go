package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// commentWrapWidth is the maximum width for wrapped comments in templates.
const commentWrapWidth = 70

// TemplateOptions controls configuration template generation.
type TemplateOptions struct {
	// Full includes all rules with their documentation.
	// If false, generates a minimal template.
	Full bool

	// Format is the output format: "yaml" or "json".
	Format string

	// IncludeRules is a list of rule IDs to include.
	// If empty, all rules are included.
	IncludeRules []string

	// IncludeDefaults includes fields that match the default values.
	IncludeDefaults bool
}

// RuleInfo contains rule metadata for template generation.
type RuleInfo struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	Severity    Severity
	Tags        []string
	CanFix      bool
}

// RuleInfoProvider is a function that returns rule information.
// This allows decoupling from the lint package to avoid circular imports.
type RuleInfoProvider func() []RuleInfo

// DefaultRuleInfoProvider is set by the lint package during init.
//
//nolint:gochecknoglobals // Intentional extension point for rule info.
var DefaultRuleInfoProvider RuleInfoProvider

// GenerateTemplate creates a configuration file template.
func GenerateTemplate(opts TemplateOptions) ([]byte, error) {
	if opts.Full {
		return generateFullTemplate(opts)
	}
	return generateMinimalTemplate(opts)
}

// generateMinimalTemplate creates a minimal commented template.
func generateMinimalTemplate(opts TemplateOptions) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(`# gomdlint configuration
# See: https://github.com/yaklabco/gomdlint
#
# Run-time settings such as --fix, --dry-run, --jobs and --format are
# command-line flags and cannot be set in this file.

# Markdown flavor: commonmark or gfm
flavor: gfm

# Severity for every rule that does not set one: error, warning, or info.
# Leave it unset to keep each rule's built-in severity.
# severity_default: error

# File patterns to ignore (glob patterns, relative to this file). Files that
# .gitignore ignores and node_modules and vendor directories are skipped by
# default; see the gitignore and default_ignores settings.
# ignore:
#   - "docs/drafts/**"
#   - "CHANGELOG.md"

# Rule-specific configuration
# rules:
#   MD001:
#     enabled: true
#     severity: error
#   MD013:
#     enabled: true
#     options:
#       max: 80
#       ignore_tables: true
`)

	if opts.Format == "json" {
		return templateToJSON(buf.Bytes())
	}

	return buf.Bytes(), nil
}

// generateFullTemplate creates a full template with all rules documented.
func generateFullTemplate(opts TemplateOptions) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(`# gomdlint configuration - Full Template
# See: https://github.com/yaklabco/gomdlint
#
# This template includes all available rules with their default settings.
# Uncomment and modify settings as needed.
#
# Run-time settings are command-line flags and cannot be set in this file:
# --fix, --dry-run, --jobs, --format, --enable, --disable and --fix-rules.

# Markdown flavor: commonmark or gfm
flavor: gfm

# Severity for every rule that does not set one: error, warning, or info.
# Leave it unset to keep each rule's built-in severity.
# severity_default: error

# Backup configuration for auto-fix
backups:
  enabled: true
  mode: sidecar

# Skip files that .gitignore files ignore when walking directories
gitignore: true

# Skip node_modules and vendor directories when walking directories
default_ignores: true

# File patterns to ignore (glob patterns, relative to this file)
# ignore:
#   - "docs/drafts/**"
#   - "CHANGELOG.md"

# Rule-specific configuration
rules:
`)

	// Get rule information
	rules := getRuleInfos()

	// Filter by IncludeRules if specified
	if len(opts.IncludeRules) > 0 {
		includeSet := make(map[string]bool)
		for _, id := range opts.IncludeRules {
			includeSet[id] = true
		}
		filtered := make([]RuleInfo, 0)
		for _, r := range rules {
			if includeSet[r.ID] {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	// Sort by ID
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].ID < rules[j].ID
	})

	// Write each rule
	for _, rule := range rules {
		buf.WriteString("\n  # " + rule.ID + ": " + rule.Name + "\n")
		buf.WriteString("  # " + wrapComment(rule.Description, commentWrapWidth) + "\n")
		if len(rule.Tags) > 0 {
			buf.WriteString("  # Tags: " + strings.Join(rule.Tags, ", ") + "\n")
		}
		if rule.CanFix {
			buf.WriteString("  # Auto-fix: yes\n")
		}
		buf.WriteString("  " + rule.ID + ":\n")
		buf.WriteString("    enabled: " + strconv.FormatBool(rule.Enabled) + "\n")
		buf.WriteString("    severity: " + string(rule.Severity) + "\n")
		buf.WriteString("    # options:\n")
		buf.WriteString("    #   key: value\n")
	}

	if opts.Format == "json" {
		return templateToJSON(buf.Bytes())
	}

	return buf.Bytes(), nil
}

// getRuleInfos returns information about all registered rules.
func getRuleInfos() []RuleInfo {
	if DefaultRuleInfoProvider != nil {
		return DefaultRuleInfoProvider()
	}

	// Fallback to a static list of known rules
	return []RuleInfo{
		{
			ID: "MD001", Name: "heading-increment", Enabled: true, Severity: SeverityError,
			Description: "Heading levels should only increment by one level at a time",
			Tags:        []string{"headings"},
		},
		{
			ID: "MD003", Name: "heading-style", Enabled: true, Severity: SeverityError,
			Description: "Enforce a consistent heading style (atx, atx_closed, or setext)",
			Tags:        []string{"headings"},
		},
		{
			ID: "MD004", Name: "ul-style", Enabled: true, Severity: SeverityError,
			Description: "Enforce a consistent unordered list style",
			Tags:        []string{"bullet", "ul"},
		},
		{
			ID: "MD005", Name: "list-indent", Enabled: true, Severity: SeverityError,
			Description: "Inconsistent indentation for list items at the same level",
			Tags:        []string{"bullet", "ul", "indentation"},
		},
		{
			ID: "MD009", Name: "no-trailing-spaces", Enabled: true, Severity: SeverityError,
			Description: "Trailing spaces at the end of lines",
			Tags:        []string{"whitespace"}, CanFix: true,
		},
		{
			ID: "MD010", Name: "no-hard-tabs", Enabled: true, Severity: SeverityError,
			Description: "Hard tabs in the file",
			Tags:        []string{"whitespace", "hard_tab"}, CanFix: true,
		},
		{
			ID: "MD012", Name: "no-multiple-blanks", Enabled: true, Severity: SeverityError,
			Description: "Multiple consecutive blank lines",
			Tags:        []string{"whitespace", "blank_lines"}, CanFix: true,
		},
		{
			ID: "MD013", Name: "line-length", Enabled: true, Severity: SeverityError,
			Description: "Line length exceeds the configured maximum",
			Tags:        []string{"line_length"},
		},
		{
			ID: "MD025", Name: "single-h1", Enabled: true, Severity: SeverityError,
			Description: "Multiple top-level headings in the same document",
			Tags:        []string{"headings"},
		},
		{
			ID: "MD029", Name: "ol-prefix", Enabled: true, Severity: SeverityError,
			Description: "Ordered list item prefix style",
			Tags:        []string{"ol"},
		},
		{
			ID: "MD047", Name: "single-trailing-newline", Enabled: true, Severity: SeverityError,
			Description: "Files should end with a single newline character",
			Tags:        []string{"blank_lines"}, CanFix: true,
		},
	}
}

// wrapComment wraps a comment to fit within maxWidth characters.
func wrapComment(text string, maxWidth int) string {
	if len(text) <= maxWidth {
		return text
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		switch {
		case currentLine == "":
			currentLine = word
		case len(currentLine)+1+len(word) <= maxWidth:
			currentLine += " " + word
		default:
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return strings.Join(lines, "\n  # ")
}

// templateToJSON converts a YAML template to JSON format.
func templateToJSON(yamlContent []byte) ([]byte, error) {
	// Parse the YAML (skipping comments)
	lines := strings.Split(string(yamlContent), "\n")
	var jsonLines []string

	// Build a simple config for JSON. Run-time settings such as fix and
	// format are command-line flags, so the file does not carry them.
	cfg := map[string]any{
		"flavor": "gfm",
		"backups": map[string]any{
			"enabled": true,
			"mode":    "sidecar",
		},
		"gitignore":       true,
		"default_ignores": true,
		"ignore":          []string{},
		"rules":           map[string]any{},
	}

	// Parse rules from YAML content (simplified)
	rules := getRuleInfos()
	rulesMap := make(map[string]any)
	for _, r := range rules {
		rulesMap[r.ID] = map[string]any{
			"enabled":  r.Enabled,
			"severity": string(r.Severity),
		}
	}
	cfg["rules"] = rulesMap

	jsonBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal JSON: %w", err)
	}

	// Keep the lines slice usage for future expansion
	_ = jsonLines
	_ = lines

	return jsonBytes, nil
}

// DefaultTemplateHeader returns the default header for generated configs.
func DefaultTemplateHeader() string {
	return `# gomdlint configuration
# See: https://github.com/yaklabco/gomdlint`
}
