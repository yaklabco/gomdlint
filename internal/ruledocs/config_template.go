package ruledocs

import (
	"cmp"
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

// ruleComment starts a comment line under the rules key of a generated
// configuration.
const ruleComment = "  # "

// ConfigHeader returns the header of a configuration file that gomdlint
// writes in YAML: the line that names the configuration's JSON Schema for
// editors such as yaml-language-server, which then complete and check the
// file, a title, each of lines as a comment, how rules are keyed, and
// where to read more.
func ConfigHeader(lines ...string) string {
	var header strings.Builder
	header.WriteString("# yaml-language-server: $schema=" + SchemaURL + "\n# gomdlint configuration\n")
	for _, line := range lines {
		header.WriteString("# " + commentText(line) + "\n")
	}
	header.WriteString("# Rule keys are the names that 'gomdlint rules' lists. IDs such as MD013 also work.\n" +
		"# See: https://github.com/yaklabco/gomdlint\n")
	return header.String()
}

// yamlKey writes key, a rule key, as a YAML mapping key on one line, as
// the YAML encoder writes it (see yamlValue): as it is, or quoted when YAML
// would read it otherwise, as it would a custom rule's name such as null,
// 123, a: b, or one with a line break or spaces at either end.
func yamlKey(key string) string {
	return yamlValue(key)
}

// commentText writes text on one line of a comment, with each run of
// whitespace, line breaks among them, as one space.
func commentText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// RuleKeyWidth returns the width of the widest of the lines that
// RuleKeyLine writes for keys, up to the comment, so that the comments
// after the keys line up.
func RuleKeyWidth(keys []string) int {
	width := 0
	for _, key := range keys {
		width = max(width, utf8.RuneCountInString(yamlKey(key))+len(":"))
	}
	return width
}

// RuleKeyLine returns the line that keys a rule by key, its name or its ID
// (see lint.Registry.ConfigKey), under rules or rule_defaults, padded to
// width (see RuleKeyWidth), with the rule's ID in a comment after the key.
// A key that YAML would not read as written is quoted.
func RuleKeyLine(key, id string, width int) string {
	written := yamlKey(key) + ":"
	return "  " + written + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(written))) + "  # " + commentText(id) + "\n"
}

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
# rule and its options. false as a rule's value turns it off, and a
# severity as its value, such as line-length: warning, turns it on.
# rules:
`

// exampleRule is a rule of the minimal template's example of rule
// settings: the rule's ID, the value after its key, and the lines under it.
type exampleRule struct {
	id, value string
	lines     []string
}

// minimalExample returns the rules of the minimal template's example of
// rule settings, in order.
func minimalExample() []exampleRule {
	return []exampleRule{
		{id: "MD001", lines: []string{"#     severity: error"}},
		{id: "MD013", lines: []string{"#     options:", "#       max: 100", "#       ignore_tables: true"}},
		{id: "MD033", value: " false"},
	}
}

// writeMinimalExample writes the minimal template's example of rule
// settings, commented, each rule keyed as entries key it (see
// Entry.ConfigKey), or by its ID when entries do not hold it, with its ID
// in a comment after the key.
func writeMinimalExample(buf *strings.Builder, entries []Entry) {
	keys := make(map[string]string, len(entries))
	for _, entry := range entries {
		keys[entry.ID] = entry.configKey()
	}
	example := minimalExample()
	heads := make([]string, len(example))
	width := 0
	for idx, rule := range example {
		heads[idx] = "#   " + yamlKey(cmp.Or(keys[rule.id], rule.id)) + ":" + rule.value
		width = max(width, utf8.RuneCountInString(heads[idx]))
	}
	for idx, rule := range example {
		buf.WriteString(heads[idx] + strings.Repeat(" ", width-utf8.RuneCountInString(heads[idx])) + "  # " + rule.id + "\n")
		for _, line := range rule.lines {
			buf.WriteString(line + "\n")
		}
	}
}

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
	buf.WriteString(ConfigHeader())
	if opts.Full {
		buf.WriteString(fullSettings)
		writeTemplateRules(&buf, entries)
	} else {
		buf.WriteString(minimalSettings)
		writeMinimalExample(&buf, entries)
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
// rule is keyed by its name, or by its ID when another rule shares the
// name (see Entry.ConfigKey), with its ID in a comment, after its
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
		buf.WriteString(ruleComment + commentText(group.title()) + "\n")
		names := make([]string, len(group.entries))
		for idx, entry := range group.entries {
			names[idx] = entry.configKey()
		}
		keyWidth := RuleKeyWidth(names)
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
		for _, line := range wrap(commentText(entry.Description), lineWidth, ruleComment, ruleComment) {
			buf.WriteString(line + "\n")
		}
	}
	buf.WriteString(RuleKeyLine(entry.configKey(), entry.ID, keyWidth))
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
// where its default comes from, in a trailing comment. The name, the
// default and the values are written as the YAML encoder writes them on
// one line (see yamlValue), so that the line, uncommented, sets the option
// to its default and nothing else. An option whose default is another
// option's value shows no value, which would stop it following that option
// once the line is uncommented.
func templateOption(spec lint.OptionSpec) string {
	line := yamlKey(spec.Name) + ":"
	if spec.Default != nil && spec.DefaultFrom == "" {
		line += " " + yamlValue(spec.Default)
	}
	switch {
	case spec.DefaultFrom != "":
		line += "  # defaults to the value of " + yamlValue(spec.DefaultFrom)
	case len(spec.Values) > 0:
		values := make([]string, len(spec.Values))
		for idx, value := range spec.Values {
			values[idx] = yamlValue(value)
		}
		line += "  # " + strings.Join(values, ", ")
	case spec.Default == nil:
		line += "  # unset by default"
	}
	return line
}
