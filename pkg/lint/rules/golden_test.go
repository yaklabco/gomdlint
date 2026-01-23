package rules

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// update is a flag to update golden files instead of comparing.
// Usage: go test -update ./pkg/lint/rules/... -run TestGolden.
var update = flag.Bool("update", false, "update golden files")

// testdataDir returns the absolute path to the testdata directory.
func testdataDir(t *testing.T) string {
	t.Helper()

	// Get the directory of this test file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}

	return filepath.Join(filepath.Dir(filename), "testdata")
}

// TestGoldenPerRule runs golden tests for each rule directory.
// Each subdirectory under testdata/ named after a rule ID (e.g., MD001, MD031)
// contains test cases that run only that specific rule.
func TestGoldenPerRule(t *testing.T) {
	baseDir := testdataDir(t)

	cases := discoverTestCases(t, baseDir)
	if len(cases) == 0 {
		t.Skip("No golden test cases found. Create testdata/<RULE_ID>/*.input.md files to add tests.")
	}

	// Filter to per-rule tests only (not real-world)
	var perRuleCases []GoldenTestCase
	for _, tc := range cases {
		if !tc.IsRealWorld && tc.RuleID != "" {
			perRuleCases = append(perRuleCases, tc)
		}
	}

	if len(perRuleCases) == 0 {
		t.Skip("No per-rule golden test cases found.")
	}

	for _, tc := range perRuleCases {
		t.Run(tc.Name, func(t *testing.T) {
			runPerRuleGoldenTest(t, tc, *update)
		})
	}
}

// TestGoldenRealWorld runs golden tests against real-world markdown files.
// These tests run all enabled rules against files in testdata/real-world/.
func TestGoldenRealWorld(t *testing.T) {
	baseDir := testdataDir(t)

	cases := discoverTestCases(t, baseDir)
	if len(cases) == 0 {
		t.Skip("No golden test cases found.")
	}

	// Filter to real-world tests only
	var realWorldCases []GoldenTestCase
	for _, tc := range cases {
		if tc.IsRealWorld {
			realWorldCases = append(realWorldCases, tc)
		}
	}

	if len(realWorldCases) == 0 {
		t.Skip("No real-world golden test cases found. Create testdata/real-world/*.input.md files to add tests.")
	}

	for _, tc := range realWorldCases {
		t.Run(tc.Name, func(t *testing.T) {
			runRealWorldGoldenTest(t, tc, *update)
		})
	}
}

// TestGoldenRoundTrip verifies that fixes are idempotent.
// For each test case with fixable diagnostics:
//  1. Parse input and run rules
//  2. Apply all fixes
//  3. Re-parse fixed content and re-run rules
//  4. Assert zero fixable diagnostics remain.
func TestGoldenRoundTrip(t *testing.T) {
	baseDir := testdataDir(t)

	cases := discoverTestCases(t, baseDir)
	if len(cases) == 0 {
		t.Skip("No golden test cases found for round-trip testing.")
	}

	for _, tc := range cases {
		t.Run(tc.Name+"_roundtrip", func(t *testing.T) {
			runRoundTripTest(t, tc)
		})
	}
}

// runPerRuleGoldenTest executes a single per-rule golden test.
func runPerRuleGoldenTest(t *testing.T, tc GoldenTestCase, updateGolden bool) {
	t.Helper()

	// Read input file
	input, err := os.ReadFile(tc.InputPath)
	require.NoError(t, err, "failed to read input file: %s", tc.InputPath)

	// Parse markdown
	parser := goldmark.New(string(config.FlavorCommonMark))
	snapshot, err := parser.Parse(context.Background(), filepath.Base(tc.InputPath), input)
	require.NoError(t, err, "failed to parse input file: %s", tc.InputPath)

	// Get the specific rule
	rule := getRuleByID(t, tc.RuleID)

	// Create rule context and run the rule
	cfg := config.NewConfig()
	ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
	ruleCtx.Registry = lint.DefaultRegistry

	diags, err := rule.Apply(ruleCtx)
	require.NoError(t, err, "rule %s failed to apply", tc.RuleID)

	// Compare diagnostics
	compareDiags(t, diags, tc, updateGolden)

	// Apply fixes and compare golden output
	if len(diags) > 0 {
		fixedContent := applyAllFixes(t, input, diags)
		compareWithGolden(t, fixedContent, tc.GoldenPath, updateGolden)
	} else {
		// No diagnostics means no changes expected
		compareWithGolden(t, input, tc.GoldenPath, updateGolden)
	}
}

// runRealWorldGoldenTest executes a single real-world golden test.
func runRealWorldGoldenTest(t *testing.T, tc GoldenTestCase, updateGolden bool) {
	t.Helper()

	// Read input file
	input, err := os.ReadFile(tc.InputPath)
	require.NoError(t, err, "failed to read input file: %s", tc.InputPath)

	// Parse markdown
	parser := goldmark.New(string(config.FlavorCommonMark))

	// Get all enabled rules
	rules := getEnabledRules(t)
	cfg := config.NewConfig()

	// Run all rules and collect diagnostics
	allDiags := make([]lint.Diagnostic, 0, len(rules))
	for _, rule := range rules {
		// Re-parse for each rule to avoid state issues
		snapshot, parseErr := parser.Parse(context.Background(), filepath.Base(tc.InputPath), input)
		require.NoError(t, parseErr)

		ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
		ruleCtx.Registry = lint.DefaultRegistry

		diags, applyErr := rule.Apply(ruleCtx)
		require.NoError(t, applyErr, "rule %s failed to apply", rule.ID())

		allDiags = append(allDiags, diags...)
	}

	// Sort diagnostics by line, column for consistent output
	sortDiagnostics(allDiags)

	// Compare diagnostics
	compareDiags(t, allDiags, tc, updateGolden)

	// Apply fixes and compare golden output
	if len(allDiags) > 0 {
		fixedContent := applyAllFixes(t, input, allDiags)
		compareWithGolden(t, fixedContent, tc.GoldenPath, updateGolden)
	} else {
		// No diagnostics means no changes expected
		compareWithGolden(t, input, tc.GoldenPath, updateGolden)
	}
}

// runRoundTripTest verifies that applying fixes results in zero fixable diagnostics.
func runRoundTripTest(t *testing.T, tc GoldenTestCase) {
	t.Helper()

	// Read input file
	input, err := os.ReadFile(tc.InputPath)
	require.NoError(t, err, "failed to read input file: %s", tc.InputPath)

	// Parse markdown
	parser := goldmark.New(string(config.FlavorCommonMark))
	snapshot, err := parser.Parse(context.Background(), filepath.Base(tc.InputPath), input)
	require.NoError(t, err, "failed to parse input file: %s", tc.InputPath)

	cfg := config.NewConfig()

	// Determine which rules to run
	var rules []lint.Rule
	if tc.RuleID != "" {
		rules = []lint.Rule{getRuleByID(t, tc.RuleID)}
	} else {
		rules = getEnabledRules(t)
	}

	// Run all rules and collect diagnostics
	allDiags := make([]lint.Diagnostic, 0, len(rules))
	for _, rule := range rules {
		ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
		ruleCtx.Registry = lint.DefaultRegistry

		diags, applyErr := rule.Apply(ruleCtx)
		require.NoError(t, applyErr, "rule %s failed to apply", rule.ID())

		allDiags = append(allDiags, diags...)
	}

	// Check if there are any fixable diagnostics
	fixableCount := countFixableDiags(allDiags)
	if fixableCount == 0 {
		// No fixable diagnostics, round-trip test passes trivially
		return
	}

	// Apply all fixes
	fixedContent := applyAllFixes(t, input, allDiags)

	// Re-parse and re-run rules
	snapshot2, err := parser.Parse(context.Background(), filepath.Base(tc.InputPath), fixedContent)
	require.NoError(t, err, "failed to parse fixed content")

	var secondPassDiags []lint.Diagnostic
	for _, rule := range rules {
		ruleCtx := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
		ruleCtx.Registry = lint.DefaultRegistry

		diags, err := rule.Apply(ruleCtx)
		require.NoError(t, err, "rule %s failed to apply on second pass", rule.ID())

		secondPassDiags = append(secondPassDiags, diags...)
	}

	// Assert zero fixable diagnostics remain
	remainingFixable := filterFixableDiags(secondPassDiags)
	if len(remainingFixable) > 0 {
		t.Errorf("Round-trip test failed: %d fixable diagnostics remain after applying fixes", len(remainingFixable))
		for _, d := range remainingFixable {
			t.Logf("  %s:%d:%d %s (%s)", filepath.Base(tc.InputPath), d.StartLine, d.StartColumn, d.Message, d.RuleName)
		}
		t.Logf("Fixed content:\n%s", string(fixedContent))
	}
}

// applyAllFixes applies all fix edits from diagnostics to the input content.
// Uses PrepareEditsFiltered to handle overlapping deletions by merging them.
func applyAllFixes(t *testing.T, input []byte, diags []lint.Diagnostic) []byte {
	t.Helper()

	// Count total edits for preallocation
	totalEdits := 0
	for _, diag := range diags {
		totalEdits += len(diag.FixEdits)
	}

	// Collect all edits
	allEdits := make([]fix.TextEdit, 0, totalEdits)
	for _, diag := range diags {
		allEdits = append(allEdits, diag.FixEdits...)
	}

	if len(allEdits) == 0 {
		return input
	}

	// Prepare and apply edits, merging overlapping deletions
	accepted, skipped, merged, err := fix.PrepareEditsFiltered(allEdits, len(input))
	if err != nil {
		// Validation error (not conflicts - those are filtered)
		t.Logf("Warning: edit validation failed: %v", err)
		return input
	}

	if len(skipped) > 0 {
		t.Logf("Note: %d edits skipped due to conflicts (non-deletions that overlap)", len(skipped))
	}
	if merged > 0 {
		t.Logf("Note: %d overlapping deletions were merged", merged)
	}

	return fix.ApplyEdits(input, accepted)
}

// sortDiagnostics sorts diagnostics by line, then column, then rule ID.
func sortDiagnostics(diags []lint.Diagnostic) {
	for i := 0; i < len(diags); i++ {
		for j := i + 1; j < len(diags); j++ {
			if shouldSwap(diags[i], diags[j]) {
				diags[i], diags[j] = diags[j], diags[i]
			}
		}
	}
}

// shouldSwap returns true if first should come after second in sorted order.
func shouldSwap(first, second lint.Diagnostic) bool {
	if first.StartLine != second.StartLine {
		return first.StartLine > second.StartLine
	}
	if first.StartColumn != second.StartColumn {
		return first.StartColumn > second.StartColumn
	}
	return first.RuleID > second.RuleID
}

// TestGoldenTestInfrastructure verifies the golden test infrastructure itself.
func TestGoldenTestInfrastructure(t *testing.T) {
	t.Run("isRuleID", func(t *testing.T) {
		tests := []struct {
			input string
			want  bool
		}{
			{"MD001", true},
			{"MD031", true},
			{"MD999", true},
			{"MDL001", true},
			{"MDL003", true},
			{"MM001", true}, // mermaid rules
			{"MM999", true}, // mermaid rules
			{"real-world", false},
			{"test", false},
			{"", false},
			{"MD", false},
			{"MM", false},
			{"MDabc", false},
			{"MMabc", false},
			{"md001", false}, // lowercase not valid
			{"mm001", false}, // lowercase not valid
		}

		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				got := isRuleID(tt.input)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("itoa", func(t *testing.T) {
		tests := []struct {
			input int
			want  string
		}{
			{0, "0"},
			{1, "1"},
			{10, "10"},
			{123, "123"},
			{-5, "-5"},
		}

		for _, tt := range tests {
			got := itoa(tt.input)
			assert.Equal(t, tt.want, got)
		}
	})

	t.Run("testdataDir", func(t *testing.T) {
		dir := testdataDir(t)
		assert.Contains(t, dir, "testdata")
		assert.True(t, filepath.IsAbs(dir))
	})

	t.Run("discoverTestCases_empty", func(t *testing.T) {
		// This should not fail even if no test cases exist
		baseDir := testdataDir(t)
		cases := discoverTestCases(t, baseDir)
		// Just verify it returns a slice (possibly empty)
		assert.NotNil(t, cases)
	})
}
