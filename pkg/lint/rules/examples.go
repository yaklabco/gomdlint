package rules

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
)

// exampleFiles holds the example scenarios of the rules: the golden test
// scenarios named example and example_good in each rule's testdata
// directory. The golden tests generate their fixed documents and findings,
// so the examples that the rule reference and gomdlint explain show are
// tested output.
//
//go:embed testdata/*/example*
var exampleFiles embed.FS

// Example shows what a rule reports and what correct Markdown looks like
// under it, from the rule's example scenarios.
type Example struct {
	// Config is the configuration that the examples run with: YAML that
	// sets the flavor or the rule's options under rules.<ID>.options, as a
	// golden sidecar does, or "" for the defaults.
	Config string

	// Input is a document that breaks the rule.
	Input string

	// Findings are what the rule reports in Input, in order.
	Findings []ExampleFinding

	// Fixed is Input after one pass of the rule's fixes. It is Input
	// itself for a rule that cannot fix what it finds.
	Fixed string

	// Good is a document that follows the rule, or "" when the example has
	// none. Every rule that cannot fix what it finds has one.
	Good string
}

// ExampleFinding is one finding of a rule in its example.
type ExampleFinding struct {
	// Line and Column are where the finding starts, counted from 1.
	Line   int `json:"line"`
	Column int `json:"column"`

	// Message is what the rule says about it.
	Message string `json:"message"`

	// Fixable reports that the rule fixes it.
	Fixable bool `json:"fixable"`
}

// The names of the example scenarios in a rule's testdata directory.
const (
	exampleScenario     = "example"
	goodExampleScenario = "example_good"
)

// RuleExample returns the example of the built-in rule with the given ID,
// such as MD001. It reports false when the rule has none, and an error
// when the example's files cannot be read.
func RuleExample(id string) (Example, bool, error) {
	if !fs.ValidPath(id) || id == "." || path.Base(id) != id {
		return Example{}, false, nil
	}
	dir := path.Join("testdata", id)

	input, err := readExampleFile(dir, exampleScenario+".input.md")
	if errors.Is(err, fs.ErrNotExist) {
		return Example{}, false, nil
	}
	if err != nil {
		return Example{}, false, err
	}

	example := Example{Input: input}
	if example.Fixed, err = readExampleFile(dir, exampleScenario+".golden.md"); err != nil {
		return Example{}, false, err
	}
	findings, err := readExampleFile(dir, exampleScenario+".diags.json")
	if err != nil {
		return Example{}, false, err
	}
	if err := json.Unmarshal([]byte(findings), &example.Findings); err != nil {
		return Example{}, false, fmt.Errorf("read the findings of the example of %s: %w", id, err)
	}
	if example.Config, err = readOptionalExampleFile(dir, exampleScenario+".config.yml"); err != nil {
		return Example{}, false, err
	}
	if example.Good, err = readOptionalExampleFile(dir, goodExampleScenario+".input.md"); err != nil {
		return Example{}, false, err
	}

	return example, true, nil
}

// readExampleFile returns the content of the example file name in dir.
func readExampleFile(dir, name string) (string, error) {
	data, err := exampleFiles.ReadFile(path.Join(dir, name))
	if err != nil {
		return "", fmt.Errorf("read example: %w", err)
	}
	return string(data), nil
}

// readOptionalExampleFile returns the content of the example file name in
// dir, or "" when there is none.
func readOptionalExampleFile(dir, name string) (string, error) {
	content, err := readExampleFile(dir, name)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return content, err
}
