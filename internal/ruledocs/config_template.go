package ruledocs

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TemplateOptions selects the configuration template that ConfigTemplate
// writes, as gomdlint init's flags do.
type TemplateOptions struct {
	// Full lists every rule with its documentation. Without it, the
	// template sets the flavor and shows the other settings in comments.
	Full bool

	// JSON writes the template as JSON, which has no comments, so it
	// holds the settings that the YAML template sets and nothing else.
	JSON bool
}

// configHeader starts every configuration template: a title, how rules are
// keyed, and where to read more.
const configHeader = "# gomdlint configuration\n" +
	"# Rule keys are the names that 'gomdlint rules' lists. IDs such as MD013 also work.\n" +
	"# See: https://github.com/yaklabco/gomdlint\n"

// schemaModeline names the JSON Schema of the configuration for editors
// such as yaml-language-server, which then complete and check the file.
const schemaModeline = "# yaml-language-server: $schema=" + SchemaURL + "\n"

// minimalSettings are the settings of the minimal template, after the
// header: the flavor, and the others in comments, with an example of rule
// settings keyed by rule name.
const minimalSettings = `#
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

# Rule settings, keyed by rule name. 'gomdlint explain <rule>' describes a
# rule and its options. false turns a rule off, and a severity turns it on.
# rules:
#   heading-increment:
#     severity: error
#   line-length:
#     options:
#       max: 100
#       ignore_tables: true
#   no-inline-html: false
`

// fullSettings are the settings of the full template, after the header and
// before its rules.
const fullSettings = `#
# This template lists every rule with its built-in settings. A rule's
# severity and options are comments, so the file changes nothing until you
# uncomment and edit them. 'gomdlint explain <rule>' describes a rule and
# its options.
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

# Rule settings, keyed by rule name, by category. A severity set here
# overrides severity_default for that rule.
`

// ConfigTemplate returns the configuration file that gomdlint init writes
// for opts. The full template documents the rules of entries.
func ConfigTemplate(entries []Entry, opts TemplateOptions) ([]byte, error) {
	var buf strings.Builder
	buf.WriteString(schemaModeline)
	buf.WriteString(configHeader)
	if opts.Full {
		buf.WriteString(fullSettings)
		writeTemplateRules(&buf, entries)
	} else {
		buf.WriteString(minimalSettings)
	}

	if opts.JSON {
		content, err := config.YAMLToJSON([]byte(buf.String()))
		if err != nil {
			return nil, fmt.Errorf("write the configuration template as JSON: %w", err)
		}
		return content, nil
	}
	return []byte(buf.String()), nil
}

// writeTemplateRules writes the rules of entries under the rules key of
// the full template, by category in the order that rules lists them. Each
// rule is keyed by its name, with its ID in a comment, after its
// description. Its built-in state is a setting, and its severity and
// options are comments: a severity would override severity_default, and
// options would replace those of lower configuration layers.
func writeTemplateRules(buf *strings.Builder, entries []Entry) {
	if len(entries) == 0 {
		buf.WriteString("rules: {}\n")
		return
	}

	buf.WriteString("rules:\n")
	for idx, group := range groups(entries, config.RuleFormatName) {
		if idx > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString("  # " + group.title() + "\n")
		keyWidth := 0
		for _, entry := range group.entries {
			keyWidth = max(keyWidth, utf8.RuneCountInString(entry.Name)+len(":"))
		}
		for _, entry := range group.entries {
			writeTemplateRule(buf, entry, keyWidth)
		}
	}
}

// writeTemplateRule writes the settings of entry's rule in the full
// template, its key padded to keyWidth before the comment with its ID.
func writeTemplateRule(buf *strings.Builder, entry Entry, keyWidth int) {
	buf.WriteString("\n")
	if entry.Description != "" {
		for _, line := range wrap(entry.Description, lineWidth, "  # ", "  # ") {
			buf.WriteString(line + "\n")
		}
	}
	key := entry.Name + ":"
	buf.WriteString("  " + key + strings.Repeat(" ", keyWidth-utf8.RuneCountInString(key)) + "  # " + entry.ID + "\n")
	buf.WriteString("    enabled: " + strconv.FormatBool(entry.Default.Enabled) + "\n")
	buf.WriteString("    # severity: " + string(entry.Default.Severity) + "\n")
	if len(entry.Options) == 0 {
		return
	}
	buf.WriteString("    # options:\n")
	for _, spec := range entry.Options {
		buf.WriteString("    #   " + templateOption(spec) + "\n")
	}
}

// templateOption writes spec, an option, as a commented line of the full
// template shows it: its name and default, with the values it allows, or
// where its default comes from, in a trailing comment.
func templateOption(spec lint.OptionSpec) string {
	line := spec.Name + ":"
	if spec.Default != nil {
		line += " " + yamlValue(spec.Default)
	}
	switch {
	case spec.DefaultFrom != "":
		line += "  # defaults to the value of " + spec.DefaultFrom
	case len(spec.Values) > 0:
		line += "  # " + strings.Join(spec.Values, ", ")
	case spec.Default == nil:
		line += "  # unset by default"
	}
	return line
}
