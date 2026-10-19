package ruledocs

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// The sentences of a page that text and Markdown pages share. Each takes
// the functions that write the page's words: literal for a word that a
// configuration file or a command line holds, such as an option's name or
// value, and text for prose from the catalog. Text pages write both as
// they are, and Markdown pages write literals as code spans and escape
// prose.

// pageWords are the functions that write the words of a page.
type pageWords struct {
	// literal writes a word that a configuration file or a command line
	// holds.
	literal func(string) string

	// text writes prose from the catalog.
	text func(string) string
}

// unbrokenSpace stands for a space of a literal on a text page until the
// page is wrapped, so that wrapping keeps the literal on one line. It is a
// private-use character, which no catalog text holds, and textPage writes
// it as a space.
const unbrokenSpace = "\uE000"

// plainWords writes the words of a text page as they are.
//
//nolint:gochecknoglobals // Read-only table of functions.
var plainWords = pageWords{
	literal: func(word string) string { return strings.ReplaceAll(word, " ", unbrokenSpace) },
	text:    func(text string) string { return text },
}

// markdownWords writes the words of a Markdown page.
//
//nolint:gochecknoglobals // Read-only table of functions.
var markdownWords = pageWords{literal: markdownCode, text: markdownText}

// defaultStateText describes a rule's built-in state and whether it fixes
// what it finds, in a line such as "On by default, severity warning.
// Fixable.", which a text page shows under its header.
func defaultStateText(entry Entry) string {
	state := "On by default"
	if !entry.Default.Enabled {
		state = "Off by default"
	}
	fixable := "Fixable."
	if !entry.Fixable {
		fixable = "Not fixable."
	}
	return fmt.Sprintf("%s, severity %s. %s", state, entry.Default.Severity, fixable)
}

// configuredStateText describes how the configuration changes the rule's
// state from its built-in state, or returns "" when it does not. The
// severity of a rule that is off either way does not count.
func configuredStateText(entry Entry) string {
	now, before := entry.State, entry.Default
	switch {
	case !now.Enabled && !before.Enabled:
		return ""
	case !now.Enabled:
		return "Your configuration turns it off."
	case !before.Enabled && now.Severity == before.Severity:
		return "Your configuration turns it on."
	case !before.Enabled:
		return fmt.Sprintf("Your configuration turns it on, at severity %s.", now.Severity)
	case now.Severity != before.Severity:
		return fmt.Sprintf("Your configuration sets its severity to %s.", now.Severity)
	}
	return ""
}

// aliasTexts says which other names the rule has. asked is the word the
// rule was asked for by: when it is one of the aliases, the first sentence
// says whose name it is. The catalog gives a rule that markdownlint has no
// aliases other than markdownlint's names for it, with the first of them
// as MarkdownlintName when that is not the rule's own name.
func aliasTexts(entry Entry, asked string, words pageWords) []string {
	var texts, others []string
	key := strings.TrimSpace(asked)
	for _, alias := range entry.Aliases {
		if !strings.EqualFold(alias, key) {
			others = append(others, words.literal(alias))
			continue
		}
		whose := "another name"
		switch {
		case entry.inMarkdownlint() && alias == entry.MarkdownlintName:
			whose = "markdownlint's name"
		case entry.inMarkdownlint():
			whose = "one of markdownlint's names"
		}
		texts = append(texts, words.literal(alias)+" is "+whose+" for this rule.")
	}
	if len(others) > 0 {
		texts = append(texts, "Also named "+joinWords(others, "and")+markdownlintNames(entry)+".")
	}
	return texts
}

// markdownlintNames ends a list of the rule's aliases with ", as in
// markdownlint" for a rule that markdownlint has, whose aliases are
// markdownlint's names for it.
func markdownlintNames(entry Entry) string {
	if entry.inMarkdownlint() {
		return ", as in markdownlint"
	}
	return ""
}

// optionFacts describes the type and default of an option, as in "integer,
// default 2", "integer, defaults to the value of max" or "integer, unset
// by default". Options that markdownlint lacks say so, unless the whole
// rule is gomdlint's own.
func optionFacts(entry Entry, spec lint.OptionSpec, words pageWords) string {
	facts := []string{words.literal(string(spec.Type))}
	switch {
	case spec.DefaultFrom != "":
		facts = append(facts, "defaults to the value of "+words.literal(spec.DefaultFrom))
	case spec.Default == nil:
		facts = append(facts, "unset by default")
	default:
		facts = append(facts, "default "+words.literal(yamlValue(spec.Default)))
	}
	if spec.GomdlintOnly && entry.inMarkdownlint() {
		facts = append(facts, "gomdlint only")
	}
	return strings.Join(facts, ", ")
}

// optionDetails describes what an option controls, the values it allows
// and its other spellings.
func optionDetails(spec lint.OptionSpec, words pageWords) string {
	var sentences []string
	if spec.Description != "" {
		sentences = append(sentences, words.text(spec.Description))
	}
	if len(spec.Values) > 0 {
		values := make([]string, len(spec.Values))
		for idx, value := range spec.Values {
			values[idx] = words.literal(value)
		}
		sentences = append(sentences, "Values: "+strings.Join(values, ", ")+".")
	}
	for _, alias := range spec.Aliases {
		sentence := "Also accepted as " + words.literal(alias.Name)
		if alias.Inverted {
			sentence += ", with the opposite value"
		}
		sentences = append(sentences, sentence+".")
	}
	return strings.Join(sentences, " ")
}

// exampleOption returns the option that a page's configuration example
// sets, to its default: the first that has a default of its own. An option
// whose default is another option's value is left out, because setting it
// would stop it following that option. It reports false when no option
// qualifies.
func exampleOption(entry Entry) (lint.OptionSpec, bool) {
	for _, spec := range entry.Options {
		if spec.Default != nil && spec.DefaultFrom == "" {
			return spec, true
		}
	}
	return lint.OptionSpec{}, false
}

// exampleConfig returns the lines of a configuration that sets the rule's
// example option to its default, or none when no option qualifies (see
// exampleOption).
func exampleConfig(entry Entry) []string {
	spec, ok := exampleOption(entry)
	if !ok {
		return nil
	}
	return []string{
		"rules:",
		"  " + entry.Name + ":",
		"    options:",
		"      " + spec.Name + ": " + yamlValue(spec.Default),
	}
}

// switchText says how a configuration turns the rule off, or on, and
// changes its severity, and how a flag turns it off or on for one run.
func switchText(entry Entry, words pageWords) string {
	name := entry.Name
	other := config.SeverityError
	if entry.Default.Severity == config.SeverityError {
		other = config.SeverityWarning
	}
	if !entry.Default.Enabled {
		return fmt.Sprintf("Under %s, %s turns the rule on and %s turns it on with severity %s. "+
			"The flag %s turns it on for one run.",
			words.literal("rules"), words.literal(name+": true"), words.literal(name+": "+string(other)), other,
			words.literal("--enable "+name))
	}
	return fmt.Sprintf("Under %s, %s turns the rule off and %s reports its findings as %ss. "+
		"The flag %s turns it off for one run.",
		words.literal("rules"), words.literal(name+": false"), words.literal(name+": "+string(other)), other,
		words.literal("--disable "+name))
}

// directive returns the markdownlint comment that silences the rule on the
// line after it.
func directive(entry Entry) string {
	return "<!-- markdownlint-disable-next-line " + entry.directiveName() + " -->"
}

// yamlValue writes value as a configuration file holds it, in YAML, with
// lists on one line, such as [x].
func yamlValue(value any) string {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Sprint(value)
	}
	flowStyle(&node)
	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return fmt.Sprint(value)
	}
	return strings.TrimSuffix(string(encoded), "\n")
}

// flowStyle writes the lists and mappings of node, and of its children, on
// one line.
func flowStyle(node *yaml.Node) {
	if node.Kind == yaml.SequenceNode || node.Kind == yaml.MappingNode {
		node.Style |= yaml.FlowStyle
	}
	for _, child := range node.Content {
		flowStyle(child)
	}
}

// joinWords joins words into a list in prose, with conjunction before the
// last word: "a", "a and b", "a, b and c".
func joinWords(words []string, conjunction string) string {
	last := len(words) - 1
	if last < 1 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:last], ", ") + " " + conjunction + " " + words[last]
}
