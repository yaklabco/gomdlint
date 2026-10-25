package configloader

import (
	"bytes"
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// GenerateMigrationHeader returns the header of a migrated config in YAML:
// the line that points editors at the configuration's JSON Schema, the
// source file, and how rules are keyed.
func GenerateMigrationHeader(sourcePath string) string {
	return "# yaml-language-server: $schema=" + ruledocs.SchemaURL + "\n" +
		"# gomdlint configuration\n" +
		"# Migrated from: " + filepath.Base(sourcePath) + "\n" +
		"# Rule keys are the names that 'gomdlint rules' lists. IDs such as MD013 also work.\n" +
		"# See: https://github.com/yaklabco/gomdlint\n"
}

// FileContent returns the content of a gomdlint config file at path that
// holds the converted settings: JSON for a name that ends in .json, which
// lint reads as JSON, and otherwise YAML under a header that names the
// source file (see GenerateMigrationHeader). The file holds the rule
// settings alone, under rules and rule_defaults, so a user or system config
// still sets everything else, such as ignore patterns and the flavor.
//
// Rules are keyed by name, and options are written under options. The YAML
// gives each rule's ID in a comment after its key, and says which rules a
// markdownlint tag set and which rules are gomdlint's own. Settings that are
// unset, and options that are empty, are left out.
func (r *MigrationResult) FileContent(path string) ([]byte, error) {
	content, err := r.ruleSettingsYAML()
	if err != nil {
		return nil, fmt.Errorf("serialize %s: %w", path, err)
	}
	if HoldsJSON(path) {
		content, err = config.YAMLToJSON(content)
		if err != nil {
			return nil, fmt.Errorf("serialize %s: %w", path, err)
		}
		return content, nil
	}
	return append([]byte(GenerateMigrationHeader(r.SourcePath)+"\n"), content...), nil
}

// writtenRule is one rule in a section of a migrated config.
type writtenRule struct {
	id, name string

	// comment is the text of the comment above the rule, or "".
	comment string

	// gap puts a blank line above the rule, which starts a group of rules.
	gap bool

	settings config.RuleConfig
}

// writtenSettings are the settings of a rule as a migrated config writes
// them, without the unset ones.
type writtenSettings struct {
	Enabled      *bool          `yaml:"enabled,omitempty"`
	Severity     *string        `yaml:"severity,omitempty"`
	AutoFix      *bool          `yaml:"auto_fix,omitempty"`
	ResetOptions bool           `yaml:"reset_options,omitempty"`
	Options      map[string]any `yaml:"options,omitempty"`
}

// ruleSettingsYAML writes the rule settings of the conversion as YAML,
// without a header: the rules that the source file sets, under rules, in
// the order of the keys that set them, then the baseline of every rule,
// under rule_defaults, in ID order.
func (r *MigrationResult) ruleSettingsYAML() ([]byte, error) {
	var buf bytes.Buffer
	if rules := r.sourceRules(); len(rules) > 0 {
		if err := writeRuleSection(&buf, "rules", rules); err != nil {
			return nil, err
		}
	}
	if defaults := r.defaultRules(); len(defaults) > 0 {
		if buf.Len() > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString("# The baseline of every rule, as markdownlint gives it. The settings\n" +
			"# under rules, in this file and in every other, override it.\n")
		if err := writeRuleSection(&buf, "rule_defaults", defaults); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// sourceRules returns the rules under Config.Rules in the order of the
// keys of the source file that set them, and by name for one key. The
// first rule that a markdownlint tag set says so in its comment, and blank
// lines set the tag's rules apart.
func (r *MigrationResult) sourceRules() []writtenRule {
	ids := make([]string, 0, len(r.Config.Rules))
	for id := range r.Config.Rules {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(left, right string) int {
		return cmp.Or(cmp.Compare(r.sources[left].order, r.sources[right].order),
			cmp.Compare(r.ruleName(left), r.ruleName(right)), cmp.Compare(left, right))
	})

	written := make([]writtenRule, 0, len(ids))
	for idx, id := range ids {
		rule := writtenRule{id: id, name: r.ruleName(id), settings: r.Config.Rules[id]}
		source := r.sources[id]
		if idx > 0 && r.sources[ids[idx-1]].order != source.order {
			rule.gap = source.tag || r.sources[ids[idx-1]].tag
		}
		if source.tag && (idx == 0 || r.sources[ids[idx-1]].order != source.order) {
			rule.comment = strconv.Quote(source.key) + ", a markdownlint tag, sets these rules:"
		}
		written = append(written, rule)
	}
	return written
}

// defaultRules returns the rules under Config.RuleDefaults in ID order.
// The first of gomdlint's own rules says so in its comment.
func (r *MigrationResult) defaultRules() []writtenRule {
	ids := make([]string, 0, len(r.Config.RuleDefaults))
	for id := range r.Config.RuleDefaults {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	written := make([]writtenRule, 0, len(ids))
	ownRules := false
	for _, id := range ids {
		rule := writtenRule{id: id, name: r.ruleName(id), settings: r.Config.RuleDefaults[id]}
		if lint.IsGomdlintOnlyRule(id) && !ownRules {
			rule.comment = "gomdlint's own rules, which markdownlint does not have:"
			rule.gap = len(written) > 0
			ownRules = true
		}
		written = append(written, rule)
	}
	return written
}

// ruleName returns the name of the rule with the given ID, or the ID of a
// rule that the conversion does not know.
func (r *MigrationResult) ruleName(id string) string {
	if r.registry != nil {
		if rule, ok := r.registry.GetByID(id); ok {
			return rule.Name()
		}
	}
	return id
}

// writeRuleSection writes rules under the key section, each keyed by its
// name, with its ID in a comment after the key, aligned across the
// section.
func writeRuleSection(buf *bytes.Buffer, section string, rules []writtenRule) error {
	keyWidth := 0
	for _, rule := range rules {
		keyWidth = max(keyWidth, utf8.RuneCountInString(rule.name)+len(":"))
	}

	buf.WriteString(section + ":\n")
	for _, rule := range rules {
		if rule.gap {
			buf.WriteString("\n")
		}
		if rule.comment != "" {
			buf.WriteString("  # " + rule.comment + "\n")
		}
		key := rule.name + ":"
		buf.WriteString("  " + key + strings.Repeat(" ", keyWidth-utf8.RuneCountInString(key)) + "  # " + rule.id + "\n")
		settings, err := encodeRuleSettings(rule.settings)
		if err != nil {
			return fmt.Errorf("write %s: %w", rule.name, err)
		}
		for line := range strings.Lines(settings) {
			buf.WriteString("    " + line)
		}
	}
	return nil
}

// encodeRuleSettings writes the settings of one rule as a YAML mapping,
// without the unset ones.
func encodeRuleSettings(settings config.RuleConfig) (string, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(config.YAMLIndent())
	err := encoder.Encode(writtenSettings{
		Enabled: settings.Enabled, Severity: settings.Severity, AutoFix: settings.AutoFix,
		ResetOptions: settings.ResetOptions, Options: settings.Options,
	})
	if err != nil {
		return "", fmt.Errorf("encode settings: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("encode settings: %w", err)
	}
	return buf.String(), nil
}
