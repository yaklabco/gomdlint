package mermaid_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules/mermaid"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// TestMermaidRulesIntegration tests all mermaid rules together on a complex document.
func TestMermaidRulesIntegration(t *testing.T) {
	t.Parallel()

	// Document with multiple issues across different rules:
	// - MM001: Parse error (invalid syntax)
	// - MM002: Undefined branch reference
	// - MM003: Duplicate node ID (tested separately due to parser behavior)
	// - MM004: Invalid direction
	//nolint:dupword // test data contains intentional repeated words in mermaid syntax
	md := `# Complex Document

` + "```mermaid" + `
flowchart TD
    A[Start] --> B
    B --> C[End]
` + "```" + `

` + "```mermaid" + `
this is not valid mermaid syntax
` + "```" + `

` + "```mermaid" + `
gitGraph
    commit
    branch develop
    checkout undefined-branch
` + "```" + `

` + "```mermaid" + `
flowchart INVALID
    A --> B
` + "```" + `
`

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	// Create all rules
	rules := []lint.Rule{
		mermaid.NewSyntaxRule(),             // MM001
		mermaid.NewUndefinedReferenceRule(), // MM002
		mermaid.NewDuplicateIDRule(),        // MM003
		mermaid.NewInvalidDirectionRule(),   // MM004
		mermaid.NewTypeCheckRule(),          // MM005
	}

	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	// Collect all diagnostics from all rules
	var allDiags []lint.Diagnostic
	for _, rule := range rules {
		diags, err := rule.Apply(ctx)
		require.NoError(t, err, "rule %s should not return error", rule.ID())
		allDiags = append(allDiags, diags...)
	}

	// Verify we got expected diagnostics
	diagsByRule := make(map[string][]lint.Diagnostic)
	for _, d := range allDiags {
		diagsByRule[d.RuleID] = append(diagsByRule[d.RuleID], d)
	}

	// MM001 should report the invalid syntax block
	assert.Len(t, diagsByRule["MM001"], 1, "MM001 should report one syntax error")
	if len(diagsByRule["MM001"]) > 0 {
		assert.Contains(t, diagsByRule["MM001"][0].Message, "Invalid mermaid syntax")
	}

	// MM002 should report the undefined branch
	assert.Len(t, diagsByRule["MM002"], 1, "MM002 should report one undefined reference")
	if len(diagsByRule["MM002"]) > 0 {
		assert.Contains(t, diagsByRule["MM002"][0].Message, "undefined-branch")
	}

	// MM004 should report the invalid direction
	assert.Len(t, diagsByRule["MM004"], 1, "MM004 should report one invalid direction")
	if len(diagsByRule["MM004"]) > 0 {
		assert.Contains(t, diagsByRule["MM004"][0].Message, "Invalid flowchart direction")
	}
}

// TestMermaidRulesNoOverlap verifies rules don't report the same issue twice.
func TestMermaidRulesNoOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		md              string
		expectedRuleIDs []string // Only these rules should report
		excludedRuleIDs []string // These rules should NOT report
	}{
		{
			name:            "direction error only reported by MM004",
			md:              "```mermaid\nflowchart INVALID\n    A --> B\n```\n",
			expectedRuleIDs: []string{"MM004"},
			excludedRuleIDs: []string{"MM001"}, // MM001 should skip direction errors
		},
		{
			name:            "parse error only reported by MM001",
			md:              "```mermaid\nnot valid mermaid at all\n```\n",
			expectedRuleIDs: []string{"MM001"},
			excludedRuleIDs: []string{"MM002", "MM003", "MM004", "MM005"},
		},
		{
			name:            "undefined reference only reported by MM002",
			md:              "```mermaid\ngitGraph\n    commit\n    checkout nonexistent\n```\n",
			expectedRuleIDs: []string{"MM002"},
			excludedRuleIDs: []string{"MM003", "MM005"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parser := goldmark.New(goldmark.FlavorGFM)
			file, err := parser.Parse(context.Background(), "test.md", []byte(tc.md))
			require.NoError(t, err)

			rules := []lint.Rule{
				mermaid.NewSyntaxRule(),
				mermaid.NewUndefinedReferenceRule(),
				mermaid.NewDuplicateIDRule(),
				mermaid.NewInvalidDirectionRule(),
				mermaid.NewTypeCheckRule(),
			}

			ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

			diagsByRule := make(map[string][]lint.Diagnostic)
			for _, rule := range rules {
				diags, err := rule.Apply(ctx)
				require.NoError(t, err)
				if len(diags) > 0 {
					diagsByRule[rule.ID()] = diags
				}
			}

			// Verify expected rules reported
			for _, ruleID := range tc.expectedRuleIDs {
				assert.NotEmpty(t, diagsByRule[ruleID], "rule %s should report diagnostics", ruleID)
			}

			// Verify excluded rules did NOT report
			for _, ruleID := range tc.excludedRuleIDs {
				assert.Empty(t, diagsByRule[ruleID], "rule %s should NOT report diagnostics", ruleID)
			}
		})
	}
}

// TestMermaidRulesStrictOption tests that strict option affects validation.
func TestMermaidRulesStrictOption(t *testing.T) {
	t.Parallel()

	// This diagram has a reference that only triggers in strict mode
	// In go-mermaid, strict mode enables additional validation rules
	//nolint:dupword // test data contains intentional repeated words in mermaid syntax
	md := "```mermaid\nstateDiagram-v2\n    [*] --> Active\n    Active --> Inactive\n    Inactive --> [*]\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	// Test without strict mode (default)
	ctxDefault := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)
	rule := mermaid.NewUndefinedReferenceRule()
	diagsDefault, err := rule.Apply(ctxDefault)
	require.NoError(t, err)

	// Test with strict mode enabled
	ruleCfg := &config.RuleConfig{
		Options: map[string]any{
			"strict": true,
		},
	}
	ctxStrict := lint.NewRuleContext(context.Background(), file, config.NewConfig(), ruleCfg)
	diagsStrict, err := rule.Apply(ctxStrict)
	require.NoError(t, err)

	// Strict mode should report at least as many issues as non-strict
	// (it may report more depending on go-mermaid's strict validators)
	assert.GreaterOrEqual(t, len(diagsStrict), len(diagsDefault),
		"strict mode should report at least as many issues as default")

	// Both modes should report the undefined state references
	assert.NotEmpty(t, diagsDefault, "even non-strict mode should catch undefined state references")
}

// TestMermaidRulesCaching verifies that ExtractMermaidBlocks doesn't cause issues
// when called multiple times with the same context.
func TestMermaidRulesCaching(t *testing.T) {
	t.Parallel()

	md := "```mermaid\nflowchart TD\n    A --> B\n```\n\n```mermaid\ngitGraph\n    commit\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	// Run all rules - each will call ExtractMermaidBlocks
	rules := []lint.Rule{
		mermaid.NewSyntaxRule(),
		mermaid.NewUndefinedReferenceRule(),
		mermaid.NewDuplicateIDRule(),
		mermaid.NewInvalidDirectionRule(),
		mermaid.NewTypeCheckRule(),
	}

	// Run twice to ensure no state corruption
	for iteration := range 2 {
		for _, rule := range rules {
			diags, err := rule.Apply(ctx)
			require.NoError(t, err, "iteration %d: rule %s should not error", iteration, rule.ID())
			// Valid diagrams should not produce diagnostics
			assert.Empty(t, diags, "iteration %d: rule %s should not produce diagnostics for valid diagrams",
				iteration, rule.ID())
		}
	}
}

// TestMermaidRulesValidDocument ensures no false positives on a valid complex document.
func TestMermaidRulesValidDocument(t *testing.T) {
	t.Parallel()

	md := `# Valid Document

## Flowchart

` + "```mermaid" + `
flowchart TD
    A[Start] --> B{Decision}
    B -->|Yes| C[Process]
    B -->|No| D[End]
    C --> D
` + "```" + `

## Sequence Diagram

` + "```mermaid" + `
sequenceDiagram
    participant A as Alice
    participant B as Bob
    A->>B: Hello Bob
    B->>A: Hi Alice
` + "```" + `

## Git Graph

` + "```mermaid" + `
gitGraph
    commit
    branch develop
    checkout develop
    commit
    checkout main
    merge develop
` + "```" + `

## Class Diagram

` + "```mermaid" + `
classDiagram
    class Animal {
        +String name
        +eat()
    }
    class Dog {
        +bark()
    }
    Animal <|-- Dog
` + "```" + `
`

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rules := []lint.Rule{
		mermaid.NewSyntaxRule(),
		mermaid.NewUndefinedReferenceRule(),
		mermaid.NewDuplicateIDRule(),
		mermaid.NewInvalidDirectionRule(),
		mermaid.NewTypeCheckRule(),
	}

	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	var allDiags []lint.Diagnostic
	for _, rule := range rules {
		diags, err := rule.Apply(ctx)
		require.NoError(t, err, "rule %s should not error", rule.ID())
		allDiags = append(allDiags, diags...)
	}

	assert.Empty(t, allDiags, "valid document should produce no diagnostics")
}

// TestMermaidRulesEmptyDocument ensures rules handle documents with no mermaid blocks.
func TestMermaidRulesEmptyDocument(t *testing.T) {
	t.Parallel()

	md := `# Document Without Mermaid

This document has no mermaid diagrams.

` + "```go" + `
package main

func main() {}
` + "```" + `
`

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rules := []lint.Rule{
		mermaid.NewSyntaxRule(),
		mermaid.NewUndefinedReferenceRule(),
		mermaid.NewDuplicateIDRule(),
		mermaid.NewInvalidDirectionRule(),
		mermaid.NewTypeCheckRule(),
	}

	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	for _, rule := range rules {
		diags, err := rule.Apply(ctx)
		require.NoError(t, err, "rule %s should not error on empty document", rule.ID())
		assert.Empty(t, diags, "rule %s should produce no diagnostics for non-mermaid document", rule.ID())
	}
}

// TestMermaidRulesMultipleErrorsPerBlock verifies rules can report multiple issues in one block.
func TestMermaidRulesMultipleErrorsPerBlock(t *testing.T) {
	t.Parallel()

	// GitGraph with multiple undefined branch references
	md := "```mermaid\ngitGraph\n    commit\n    checkout branch1\n    commit\n    checkout branch2\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)

	// Should report both undefined branches
	assert.GreaterOrEqual(t, len(diags), 2, "should report multiple undefined references in one block")

	// Verify distinct errors
	messages := make([]string, len(diags))
	for i, d := range diags {
		messages[i] = d.Message
	}
	foundBranch1 := false
	foundBranch2 := false
	for _, msg := range messages {
		if contains(msg, "branch1") {
			foundBranch1 = true
		}
		if contains(msg, "branch2") {
			foundBranch2 = true
		}
	}
	assert.True(t, foundBranch1, "should report branch1 as undefined")
	assert.True(t, foundBranch2, "should report branch2 as undefined")
}

// contains is a helper for checking if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestMermaidRulesDiagnosticLineNumbers verifies correct line number calculation.
func TestMermaidRulesDiagnosticLineNumbers(t *testing.T) {
	t.Parallel()

	// Header (line 1) + blank (line 2) + fence (line 3) + gitGraph (line 4) + commit (line 5) + checkout (line 6)
	md := "# Test\n\n```mermaid\ngitGraph\n    commit\n    checkout undefined\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	require.NoError(t, err)

	rule := mermaid.NewUndefinedReferenceRule()
	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)

	diags, err := rule.Apply(ctx)
	require.NoError(t, err)
	require.Len(t, diags, 1)

	// The checkout undefined is at line 6 in the document
	assert.Equal(t, 6, diags[0].StartLine, "diagnostic should report correct document line")
}
