package ruledocs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// jsonVersion is the version of the shape of the JSON output, as lint's
// JSON output has one.
const jsonVersion = "1.0.0"

// jsonOutput is the JSON output of the rules and explain commands.
type jsonOutput struct {
	Version string     `json:"version"`
	Rules   []jsonRule `json:"rules"`
}

// jsonRule describes one rule in JSON output. The ID is the rule's stable
// key. A rule that markdownlint does not have has no markdownlintName and
// no markdownlintUrl.
type jsonRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Aliases          []string     `json:"aliases"`
	MarkdownlintName string       `json:"markdownlintName,omitempty"`
	Category         string       `json:"category"`
	Tags             []string     `json:"tags"`
	Description      string       `json:"description"`
	Rationale        string       `json:"rationale"`
	Fix              string       `json:"fix"`
	Fixable          bool         `json:"fixable"`
	Enabled          bool         `json:"enabled"`
	Severity         string       `json:"severity"`
	DefaultEnabled   bool         `json:"defaultEnabled"`
	DefaultSeverity  string       `json:"defaultSeverity"`
	Options          []jsonOption `json:"options"`
	MarkdownlintURL  string       `json:"markdownlintUrl,omitempty"`
}

// jsonOption describes one option of a rule in JSON output. An option that
// takes any value of its type has no values, and only an option whose
// default is another option's value has a defaultFrom. A default of null
// leaves the option unset.
type jsonOption struct {
	Name         string            `json:"name"`
	Aliases      []jsonOptionAlias `json:"aliases"`
	Type         string            `json:"type"`
	Default      any               `json:"default"`
	DefaultFrom  string            `json:"defaultFrom,omitempty"`
	Values       []string          `json:"values,omitempty"`
	GomdlintOnly bool              `json:"gomdlintOnly"`
	Description  string            `json:"description"`
}

// jsonOptionAlias is another spelling of an option in JSON output. An
// inverted alias holds the opposite boolean value.
type jsonOptionAlias struct {
	Name     string `json:"name"`
	Inverted bool   `json:"inverted"`
}

// WriteJSON writes entries as JSON, as rules --format json and explain
// --format json print them: an object with a version and the rules, in
// listing order, each with its identifiers, classification, texts, state
// by default and under the configuration, and options.
func WriteJSON(writer io.Writer, entries []Entry) error {
	output := jsonOutput{Version: jsonVersion, Rules: make([]jsonRule, 0, len(entries))}
	for _, entry := range ordered(entries) {
		output.Rules = append(output.Rules, newJSONRule(entry))
	}

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("encode rules as JSON: %w", err)
	}
	return write(writer, encoded.String())
}

// newJSONRule describes entry's rule for JSON output.
func newJSONRule(entry Entry) jsonRule {
	rule := jsonRule{
		ID:               entry.ID,
		Name:             entry.Name,
		Aliases:          append([]string{}, entry.Aliases...),
		MarkdownlintName: entry.markdownlintRuleName(),
		Category:         string(entry.Category),
		Tags:             append([]string{}, entry.Tags...),
		Description:      entry.Description,
		Rationale:        entry.Rationale,
		Fix:              entry.Fix,
		Fixable:          entry.Fixable,
		Enabled:          entry.State.Enabled,
		Severity:         string(entry.State.Severity),
		DefaultEnabled:   entry.Default.Enabled,
		DefaultSeverity:  string(entry.Default.Severity),
		Options:          make([]jsonOption, 0, len(entry.Options)),
		MarkdownlintURL:  entry.markdownlintURL(),
	}
	for _, spec := range entry.Options {
		option := jsonOption{
			Name:         spec.Name,
			Aliases:      make([]jsonOptionAlias, 0, len(spec.Aliases)),
			Type:         string(spec.Type),
			Default:      spec.Default,
			DefaultFrom:  spec.DefaultFrom,
			Values:       spec.Values,
			GomdlintOnly: spec.GomdlintOnly,
			Description:  spec.Description,
		}
		for _, alias := range spec.Aliases {
			option.Aliases = append(option.Aliases, jsonOptionAlias{Name: alias.Name, Inverted: alias.Inverted})
		}
		rule.Options = append(rule.Options, option)
	}
	return rule
}
