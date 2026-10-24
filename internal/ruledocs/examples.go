package ruledocs

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
)

// The labels of the parts of a rule's example on its pages.
const (
	exampleConfigLabel = "With this configuration:"
	exampleInputLabel  = "This document breaks the rule:"
	exampleReportLabel = "gomdlint reports:"
	exampleFixedLabel  = "The fix changes it to:"
	exampleGoodLabel   = "This document follows the rule:"

	// exampleGutter starts each line of a document on a text page.
	exampleGutter = "  |"

	// minFenceLength is the fewest backticks that a code fence takes.
	minFenceLength = 3
)

// examplePart is one part of a rule's example, as its pages show it: a
// label and a block of text in a language.
type examplePart struct {
	label    string
	language string
	content  string
}

// exampleParts returns the parts of the example of entry's built-in rule,
// in the order that its pages show them: the configuration that the
// example needs, the document that breaks the rule, what the rule reports
// there, the fixed document and a document that follows the rule. It
// returns none for a rule without an example.
func exampleParts(entry Entry) ([]examplePart, error) {
	example, found, err := rules.RuleExample(entry.ID)
	if err != nil {
		return nil, fmt.Errorf("example of %s: %w", entry.ID, err)
	}
	if !found {
		return nil, nil
	}

	var parts []examplePart
	if example.Config != "" {
		configText, err := exampleConfigText(entry, example.Config)
		if err != nil {
			return nil, err
		}
		parts = append(parts, examplePart{label: exampleConfigLabel, language: "yaml", content: configText})
	}
	parts = append(parts,
		examplePart{label: exampleInputLabel, language: "markdown", content: example.Input},
		examplePart{label: exampleReportLabel, language: "text", content: findingsText(entry, example.Findings)})
	if example.Fixed != example.Input {
		parts = append(parts, examplePart{label: exampleFixedLabel, language: "markdown", content: example.Fixed})
	}
	if example.Good != "" {
		parts = append(parts, examplePart{label: exampleGoodLabel, language: "markdown", content: example.Good})
	}
	return parts, nil
}

// findingsText lists findings, one per line, by line and column. A finding
// of a fixing rule that gets no fix says so.
func findingsText(entry Entry, findings []rules.ExampleFinding) string {
	var out strings.Builder
	for _, finding := range findings {
		out.WriteString(strconv.Itoa(finding.Line) + ":" + strconv.Itoa(finding.Column) + " " + finding.Message)
		if entry.Fixable && !finding.Fixable {
			out.WriteString(" (no fix)")
		}
		out.WriteString("\n")
	}
	return out.String()
}

// exampleConfigText returns the configuration of an example, a golden
// sidecar, which keys rules by ID, with entry's rule keyed by its name, as
// the pages name rules everywhere else.
func exampleConfigText(entry Entry, sidecar string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(sidecar), &doc); err != nil {
		return "", fmt.Errorf("read the configuration of the example of %s: %w", entry.ID, err)
	}
	if len(doc.Content) == 1 {
		renameRuleKey(doc.Content[0], entry.ID, entry.Name)
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(config.YAMLIndent())
	if err := encoder.Encode(&doc); err != nil {
		return "", fmt.Errorf("write the configuration of the example of %s: %w", entry.ID, err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("write the configuration of the example of %s: %w", entry.ID, err)
	}
	return out.String(), nil
}

// renameRuleKey renames the key id under the rules key of root, a mapping,
// to name.
func renameRuleKey(root *yaml.Node, id, name string) {
	if root.Kind != yaml.MappingNode {
		return
	}
	for idx := 0; idx+1 < len(root.Content); idx += 2 {
		if root.Content[idx].Value != "rules" || root.Content[idx+1].Kind != yaml.MappingNode {
			continue
		}
		ruleKeys := root.Content[idx+1].Content
		for key := 0; key < len(ruleKeys); key += 2 {
			if ruleKeys[key].Value == id {
				ruleKeys[key].Value = name
			}
		}
	}
}

// markdownExample returns the blocks of a Markdown page that show the
// parts of an example: each label as a paragraph, then its text as a code
// block.
func markdownExample(parts []examplePart) []string {
	blocks := make([]string, 0, 2*len(parts)) // A label and a code block for each part.
	for _, part := range parts {
		blocks = append(blocks, part.label, markdownFence(part.language, part.content))
	}
	return blocks
}

// textExample returns the sections of a text page that show the parts of
// an example: each label, then its text, a document behind a gutter and
// findings indented.
func textExample(parts []examplePart) [][]string {
	sections := make([][]string, 0, len(parts))
	for _, part := range parts {
		lines := []string{part.label}
		for _, line := range strings.Split(strings.TrimSuffix(part.content, "\n"), "\n") {
			switch {
			case part.language == "text":
				lines = append(lines, "  "+line)
			case line == "":
				lines = append(lines, exampleGutter)
			default:
				lines = append(lines, exampleGutter+" "+line)
			}
		}
		sections = append(sections, lines)
	}
	return sections
}

// markdownFence writes content as a fenced code block in language, with a
// fence longer than any run of backticks in content, so that no line of
// content closes the block.
func markdownFence(language, content string) string {
	longest, run := 0, 0
	for _, char := range content {
		if char == '`' {
			run++
			longest = max(longest, run)
			continue
		}
		run = 0
	}
	fence := strings.Repeat("`", max(minFenceLength, longest+1))
	return fence + language + "\n" + strings.TrimSuffix(content, "\n") + "\n" + fence
}
