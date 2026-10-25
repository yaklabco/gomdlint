// Package ruledocs documents the lint rules for people, from the rule
// catalog. It lists rules by category, as a text table, as Markdown tables
// or as JSON, and writes the page of one rule, as text or Markdown: what
// the rule checks and why, its options with their defaults, what its fix
// does, and how to configure it and silence it. The rules and explain
// commands print its output.
package ruledocs

import (
	"cmp"
	"fmt"
	"io"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// markdownlintVersion is the version of markdownlint whose rules gomdlint
// implements. Pages link to its documentation of each rule.
const markdownlintVersion = "0.41.1"

// State is whether a rule runs, and the severity of its findings. A rule
// that does not run keeps the severity it would report at once turned on.
type State struct {
	Enabled  bool
	Severity config.Severity
}

// Entry documents one registered rule: its catalog entry, with the rule's
// own ID, name, fix flag and, where the entry has none, description, and
// the rule's state.
type Entry struct {
	lint.RuleMeta

	// Default is the rule's built-in state.
	Default State

	// State is the rule's state under the configuration that Entries was
	// given, or its built-in state without one.
	State State

	// ConfigKey is the key under which generated configurations name the
	// rule: its name, or its ID when another rule of the registry shares
	// the name (see lint.Registry.ConfigKey).
	ConfigKey string
}

// Entries documents every rule of registry, in rule ID order, with its
// state under cfg, the configuration that lint would run with. A nil cfg
// gives each rule its built-in state. A rule without a catalog entry that
// declares its options gets the options that it declares itself (see
// lint.OptionDeclarer).
func Entries(registry *lint.Registry, cfg *config.Config) []Entry {
	resolved := lint.ResolveAllRules(registry, cfg)
	entries := make([]Entry, 0, len(resolved))
	for _, rr := range resolved {
		entries = append(entries, newEntry(registry, rr))
	}
	return entries
}

// newEntry documents the rule of rr.
func newEntry(registry *lint.Registry, rr lint.ResolvedRule) Entry {
	rule := rr.Rule
	meta, _ := registry.Meta(rule.ID()) // A rule without an entry gets the zero entry.
	meta.ID = rule.ID()
	meta.Name = rule.Name()
	meta.Fixable = rule.CanFix()
	meta.Description = lint.RuleDescription(rule, meta)
	if declarer, declares := rule.(lint.OptionDeclarer); declares && !meta.OptionsDeclared {
		meta.Options, meta.OptionsDeclared = declarer.DeclaredOptions(), true
	}

	return Entry{
		RuleMeta: meta,
		Default:  State{Enabled: rule.DefaultEnabled(), Severity: rule.DefaultSeverity()},
		State:    State{Enabled: rr.Enabled, Severity: rr.Severity},

		ConfigKey: registry.ConfigKey(rule.ID()),
	}
}

// configKey returns the key under which generated configurations name the
// rule: its ConfigKey, or its ID for an entry without one.
func (e *Entry) configKey() string {
	return cmp.Or(e.ConfigKey, e.ID)
}

// inMarkdownlint reports whether markdownlint has the rule: whether its ID
// is one of markdownlint's rules (see lint.IsMarkdownlintRule).
func (e *Entry) inMarkdownlint() bool {
	return lint.IsMarkdownlintRule(e.ID)
}

// markdownlintRuleName returns the name that markdownlint gives the rule
// first, or "" for a rule that markdownlint does not have.
func (e *Entry) markdownlintRuleName() string {
	if !e.inMarkdownlint() {
		return ""
	}
	return cmp.Or(e.MarkdownlintName, e.Name)
}

// directiveName returns the name to write for the rule in a markdownlint
// comment: markdownlint's name for it, which both tools read, or the rule's
// own name for a rule that markdownlint does not have.
func (e *Entry) directiveName() string {
	return cmp.Or(e.markdownlintRuleName(), e.Name)
}

// markdownlintURL returns the address of markdownlint's documentation of
// the rule, or "" for a rule that markdownlint does not have.
func (e *Entry) markdownlintURL() string {
	if !e.inMarkdownlint() {
		return ""
	}
	return "https://github.com/DavidAnson/markdownlint/blob/v" + markdownlintVersion + "/doc/" +
		strings.ToLower(e.ID) + ".md"
}

// statusText is the STATUS of the rule in a listing: off, or the severity
// of its findings.
func (e *Entry) statusText() string {
	if !e.State.Enabled {
		return "off"
	}
	return string(e.State.Severity)
}

// yesNo writes a flag as listings show it.
func yesNo(flag bool) string {
	if flag {
		return "yes"
	}
	return "no"
}

// write writes text to writer.
func write(writer io.Writer, text string) error {
	if _, err := io.WriteString(writer, text); err != nil {
		return fmt.Errorf("write rule documentation: %w", err)
	}
	return nil
}
