package configloader

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/internal/ruledocs"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// migrationHeader returns the header of a migrated config in YAML, which
// names the source file (see ruledocs.ConfigHeader).
func migrationHeader(sourcePath string) string {
	return ruledocs.ConfigHeader("Migrated from: " + filepath.Base(sourcePath))
}

// FileContent returns the content of a gomdlint config file at path that
// holds the converted settings: JSON for a name that ends in .json, which
// lint reads as JSON, and otherwise YAML under a header that names the
// source file (see migrationHeader). The file holds the rule
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
	return append([]byte(migrationHeader(r.SourcePath)+"\n"), content...), nil
}

// writtenRule is one rule in a section of a migrated config.
type writtenRule struct {
	// id is the rule's ID, and key the key that names it (see ruleKey).
	id, key string

	// comment is the text of the comment above the rule, or "".
	comment string

	// gap puts a blank line above the rule, which starts a group of rules.
	gap bool

	settings config.RuleConfig
}

// writtenSettings are the settings of a rule as a migrated config writes
// them, without the unset ones. The options are the node of the options
// (see ruledocs.YAMLNode), so that the encoder writes strings that a line
// of YAML cannot hold as they are, such as one with a line break, in
// double quotes.
type writtenSettings struct {
	Enabled      *bool      `yaml:"enabled,omitempty"`
	Severity     *string    `yaml:"severity,omitempty"`
	AutoFix      *bool      `yaml:"auto_fix,omitempty"`
	ResetOptions bool       `yaml:"reset_options,omitempty"`
	Options      *yaml.Node `yaml:"options,omitempty"`
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
// lines set the tag's rules apart. A rule that no key of the source file
// set, which a caller may add to Config.Rules, comes last.
func (r *MigrationResult) sourceRules() []writtenRule {
	ids := make([]string, 0, len(r.Config.Rules))
	for id := range r.Config.Rules {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(left, right string) int {
		return cmp.Or(cmp.Compare(r.sourceOf(left).order, r.sourceOf(right).order),
			cmp.Compare(r.ruleKey(left), r.ruleKey(right)), cmp.Compare(left, right))
	})

	written := make([]writtenRule, 0, len(ids))
	var previous ruleSource
	for idx, id := range ids {
		rule := writtenRule{id: id, key: r.ruleKey(id), settings: r.Config.Rules[id]}
		source := r.sourceOf(id)
		startsGroup := idx == 0 || previous.order != source.order
		rule.gap = idx > 0 && startsGroup && (source.tag || previous.tag)
		if source.tag && startsGroup {
			rule.comment = strconv.Quote(source.key) + ", a markdownlint tag, sets these rules:"
		}
		written = append(written, rule)
		previous = source
	}
	return written
}

// sourceOf returns the key of the source file that set the rule with the
// given ID under Config.Rules, or, for a rule that no key set, a source
// after every key.
func (r *MigrationResult) sourceOf(id string) ruleSource {
	if source, ok := r.sources[id]; ok {
		return source
	}
	return ruleSource{order: math.MaxInt}
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
		rule := writtenRule{id: id, key: r.ruleKey(id), settings: r.Config.RuleDefaults[id]}
		if lint.IsGomdlintOnlyRule(id) && !ownRules {
			rule.comment = "gomdlint's own rules, which markdownlint does not have:"
			rule.gap = len(written) > 0
			ownRules = true
		}
		written = append(written, rule)
	}
	return written
}

// ruleKey returns the key under which the file that FileContent writes
// names the rule with the given ID: its name in the registry that loads the
// file, or its ID when another rule there shares the name or the registry
// does not hold the rule (see lint.Registry.ConfigKey).
func (r *MigrationResult) ruleKey(id string) string {
	if r.keyRegistry == nil {
		return id
	}
	return ruledocs.ConfigKey(r.keyRegistry, id)
}

// writeRuleSection writes rules under the key section, each keyed by its
// key, with its ID in a comment after the key, aligned across the section.
func writeRuleSection(buf *bytes.Buffer, section string, rules []writtenRule) error {
	names := make([]string, len(rules))
	for idx, rule := range rules {
		names[idx] = rule.key
	}
	keyWidth := ruledocs.RuleKeyWidth(names)

	buf.WriteString(section + ":\n")
	for _, rule := range rules {
		if rule.gap {
			buf.WriteString("\n")
		}
		if rule.comment != "" {
			buf.WriteString("  # " + rule.comment + "\n")
		}
		buf.WriteString(ruledocs.RuleKeyLine(rule.key, rule.id, keyWidth))
		settings, err := encodeRuleSettings(rule.settings)
		if err != nil {
			return fmt.Errorf("write %s: %w", rule.key, err)
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
	written := writtenSettings{
		Enabled: settings.Enabled, Severity: settings.Severity, AutoFix: settings.AutoFix,
		ResetOptions: settings.ResetOptions,
	}
	if len(settings.Options) > 0 {
		options, err := ruledocs.YAMLNode(settings.Options, false)
		if err != nil {
			return "", fmt.Errorf("encode options: %w", err)
		}
		written.Options = options
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(config.YAMLIndent())
	if err := encoder.Encode(written); err != nil {
		return "", fmt.Errorf("encode settings: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("encode settings: %w", err)
	}
	return buf.String(), nil
}
