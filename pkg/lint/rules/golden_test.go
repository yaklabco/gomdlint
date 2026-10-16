package rules

import (
	"context"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
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

	// Load the sidecar configuration and parse with its flavor
	env := newGoldenEnv(t, tc)
	rule := getRuleByID(t, tc.RuleID)
	env.requireRulesRun(t, tc, []lint.Rule{rule})
	snapshot := env.parse(t, filepath.Base(tc.InputPath), input)

	// Run the rule
	diags := env.applyRule(t, rule, snapshot)

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

	// Load the sidecar configuration and get all enabled rules
	env := newGoldenEnv(t, tc)
	rules := getEnabledRules(t)
	env.requireRulesRun(t, tc, rules)

	// Run all rules and collect diagnostics
	allDiags := make([]lint.Diagnostic, 0, len(rules))
	for _, rule := range rules {
		// Re-parse for each rule to avoid state issues
		snapshot := env.parse(t, filepath.Base(tc.InputPath), input)
		allDiags = append(allDiags, env.applyRule(t, rule, snapshot)...)
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

	// Load the sidecar configuration and parse with its flavor
	env := newGoldenEnv(t, tc)
	snapshot := env.parse(t, filepath.Base(tc.InputPath), input)

	// Determine which rules to run
	var rules []lint.Rule
	if tc.RuleID != "" {
		rules = []lint.Rule{getRuleByID(t, tc.RuleID)}
	} else {
		rules = getEnabledRules(t)
	}
	env.requireRulesRun(t, tc, rules)

	// Run all rules and collect diagnostics
	allDiags := make([]lint.Diagnostic, 0, len(rules))
	for _, rule := range rules {
		allDiags = append(allDiags, env.applyRule(t, rule, snapshot)...)
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
	snapshot2 := env.parse(t, filepath.Base(tc.InputPath), fixedContent)

	var secondPassDiags []lint.Diagnostic
	for _, rule := range rules {
		secondPassDiags = append(secondPassDiags, env.applyRule(t, rule, snapshot2)...)
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
			{"real-world", false},
			{"test", false},
			{"", false},
			{"MD", false},
			{"MDabc", false},
			{"md001", false}, // lowercase not valid
		}

		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				got := isRuleID(tt.input)
				assert.Equal(t, tt.want, got)
			})
		}
	})

	t.Run("formatDiagLine", func(t *testing.T) {
		diag := lint.Diagnostic{
			RuleID:      "MD009",
			RuleName:    "no-trailing-spaces",
			Message:     "Trailing whitespace",
			Severity:    config.SeverityWarning,
			StartLine:   3,
			StartColumn: 30,
		}
		assert.Equal(t, "basic.input.md:3:30 warning Trailing whitespace (MD009/no-trailing-spaces)",
			formatDiagLine("basic.input.md", diag))

		diag.FixEdits = []fix.TextEdit{{StartOffset: 1, EndOffset: 2}}
		assert.Equal(t, "basic.input.md:3:30 warning Trailing whitespace (MD009/no-trailing-spaces) [fixable]",
			formatDiagLine("basic.input.md", diag))
	})

	t.Run("diagFromLint", func(t *testing.T) {
		got := diagFromLint(lint.Diagnostic{RuleID: "MD009", RuleName: "no-trailing-spaces"})
		assert.Equal(t, "MD009", got.Rule)
		assert.Equal(t, "no-trailing-spaces", got.Name)
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

	t.Run("discoverTestCases_sidecar", func(t *testing.T) {
		baseDir := t.TempDir()
		ruleDir := filepath.Join(baseDir, "MD043")
		require.NoError(t, os.MkdirAll(ruleDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(ruleDir, "required.input.md"), []byte("# Title\n"), 0o644))

		cases := discoverTestCases(t, baseDir)
		require.Len(t, cases, 1)
		assert.Equal(t, filepath.Join(ruleDir, "required"+goldenConfigSuffix), cases[0].ConfigPath)
	})
}

// writeSidecar writes sidecar content to a fresh temporary file and returns its path.
func writeSidecar(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "case"+goldenConfigSuffix)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}

// TestGoldenSidecarConfig verifies loading of per-fixture sidecar configuration.
func TestGoldenSidecarConfig(t *testing.T) {
	t.Run("missing sidecar uses product defaults", func(t *testing.T) {
		cfg, err := loadGoldenConfig(filepath.Join(t.TempDir(), "absent"+goldenConfigSuffix))
		require.NoError(t, err)

		assert.Equal(t, config.NewConfig().Flavor, cfg.Flavor)
		assert.Empty(t, cfg.Rules)
	})

	t.Run("flavor and rule options", func(t *testing.T) {
		path := writeSidecar(t, `flavor: gfm
rules:
  MD043:
    options:
      headings:
        - "# Title"
        - "*"
      match_case: true
  MD013:
    options:
      line_length: 100
`)

		cfg, err := loadGoldenConfig(path)
		require.NoError(t, err)

		assert.Equal(t, config.FlavorGFM, cfg.Flavor)
		// Option values keep the types the product's YAML loader produces.
		assert.Equal(t, map[string]any{"headings": []any{"# Title", "*"}, "match_case": true}, cfg.Rules["MD043"].Options)
		assert.Equal(t, map[string]any{"line_length": 100}, cfg.Rules["MD013"].Options)
		assert.Len(t, cfg.Rules, 2)
	})

	t.Run("explicit commonmark flavor", func(t *testing.T) {
		cfg, err := loadGoldenConfig(writeSidecar(t, "flavor: commonmark\n"))
		require.NoError(t, err)

		assert.Equal(t, config.FlavorCommonMark, cfg.Flavor)
	})

	invalid := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "empty file", content: "", wantErr: "empty"},
		{name: "comment only", content: "# nothing configured\n", wantErr: "empty"},
		{name: "misspelled key", content: "flavour: gfm\n", wantErr: "flavour"},
		{name: "unsupported config key", content: "severity_default: error\n", wantErr: "severity_default"},
		{name: "unsupported rule key", content: "rules:\n  MD009:\n    severity: error\n", wantErr: "severity"},
		{name: "invalid flavor", content: "flavor: markdown\n", wantErr: `"markdown"`},
		{name: "unknown rule", content: "rules:\n  MD999:\n    options:\n      style: x\n", wantErr: "MD999"},
		{name: "rule name instead of ID", content: "rules:\n  proper-names:\n    options:\n      names: [GitHub]\n", wantErr: "MD044"},
		{name: "rule without options", content: "rules:\n  MD009: {}\n", wantErr: "MD009"},
		{name: "second document", content: "flavor: gfm\n---\nflavor: commonmark\n", wantErr: "document"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadGoldenConfig(writeSidecar(t, tt.content))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestGoldenEnv verifies that the sidecar drives the parser and rule options.
func TestGoldenEnv(t *testing.T) {
	table := []byte("| a | b |\n| - | - |\n| 1 | 2 |\n")

	t.Run("default flavor follows the product default", func(t *testing.T) {
		env := newGoldenEnv(t, GoldenTestCase{ConfigPath: filepath.Join(t.TempDir(), "absent"+goldenConfigSuffix)})

		assert.Equal(t, config.NewConfig().Flavor, env.cfg.Flavor)
		assert.Equal(t, string(config.NewConfig().Flavor), env.parser.Flavor())
	})

	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run("sidecar flavor "+string(flavor), func(t *testing.T) {
			env := newGoldenEnv(t, GoldenTestCase{ConfigPath: writeSidecar(t, "flavor: "+string(flavor)+"\n")})
			snapshot := env.parse(t, "table.input.md", table)

			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, env.cfg, nil)
			wantTables := 0
			if flavor == config.FlavorGFM {
				wantTables = 1
			}
			assert.Len(t, ruleCtx.Tables(), wantTables)
			assert.Equal(t, flavor, env.cfg.Flavor)
		})
	}

	t.Run("rule options reach the rule", func(t *testing.T) {
		env := newGoldenEnv(t, GoldenTestCase{ConfigPath: writeSidecar(t, "rules:\n  MD044:\n    options:\n      names: [GitHub]\n")})
		snapshot := env.parse(t, "names.input.md", []byte("# Title\n\nUse github here.\n"))

		diags := env.applyRule(t, getRuleByID(t, "MD044"), snapshot)
		require.Len(t, diags, 1)
		assert.Equal(t, 3, diags[0].StartLine)
	})

	t.Run("idle rule configuration", func(t *testing.T) {
		env := newGoldenEnv(t, GoldenTestCase{ConfigPath: writeSidecar(t, `rules:
  MD043:
    options:
      headings: ["# Title"]
  MD044:
    options:
      names: [GitHub]
`)})

		assert.Equal(t, []string{"MD043"}, env.idleRuleConfigs([]lint.Rule{getRuleByID(t, "MD044")}))
		assert.Empty(t, env.idleRuleConfigs([]lint.Rule{getRuleByID(t, "MD043"), getRuleByID(t, "MD044")}))
	})
}

// stubRule is a lint.Rule that returns a fixed set of diagnostics.
type stubRule struct {
	lint.BaseRule

	diags []lint.Diagnostic
}

// Apply returns a copy of the stub's diagnostics.
func (r *stubRule) Apply(_ *lint.RuleContext) ([]lint.Diagnostic, error) {
	return slices.Clone(r.diags), nil
}

// TestGoldenDiagnosticIdentity verifies that golden cases record diagnostics
// the way the engine reports them: with the rule's ID, name and severity.
func TestGoldenDiagnosticIdentity(t *testing.T) {
	newStub := func(diags ...lint.Diagnostic) *stubRule {
		return &stubRule{BaseRule: lint.NewBaseRule("MD999", "stub-rule", "Stub rule", nil, false), diags: diags}
	}

	t.Run("name and severity are filled like the engine", func(t *testing.T) {
		env := newGoldenEnv(t, GoldenTestCase{ConfigPath: filepath.Join(t.TempDir(), "absent"+goldenConfigSuffix)})
		snapshot := env.parse(t, "stub.input.md", []byte("# Title\n"))

		diags := env.applyRule(t, newStub(lint.Diagnostic{RuleID: "MD999", Message: "stub", StartLine: 1, StartColumn: 1}), snapshot)
		require.Len(t, diags, 1)
		assert.Equal(t, "stub-rule", diags[0].RuleName)
		assert.Equal(t, config.SeverityWarning, diags[0].Severity)
	})

	t.Run("real rule records its ID and name", func(t *testing.T) {
		env := newGoldenEnv(t, GoldenTestCase{ConfigPath: filepath.Join(t.TempDir(), "absent"+goldenConfigSuffix)})
		snapshot := env.parse(t, "spaces.input.md", []byte("# Title\n\nTrailing   \n"))

		diags := env.applyRule(t, getRuleByID(t, "MD009"), snapshot)
		require.NotEmpty(t, diags)
		assert.Equal(t, "MD009", diags[0].RuleID)
		assert.Equal(t, "no-trailing-spaces", diags[0].RuleName)
	})

	t.Run("foreign identity is rejected", func(t *testing.T) {
		rule := newStub()

		require.NoError(t, checkDiagIdentity(rule, []lint.Diagnostic{{RuleID: "MD999", RuleName: "stub-rule"}}))
		require.ErrorContains(t, checkDiagIdentity(rule, []lint.Diagnostic{{RuleID: "MD001", RuleName: "stub-rule"}}), "MD001")
		require.ErrorContains(t, checkDiagIdentity(rule, []lint.Diagnostic{{RuleID: "MD999", RuleName: "other-name"}}), "other-name")
	})
}

// TestGoldenSidecarFiles verifies that every sidecar in testdata is picked up.
// A misnamed or orphaned sidecar would otherwise be ignored silently.
func TestGoldenSidecarFiles(t *testing.T) {
	baseDir := testdataDir(t)

	err := filepath.WalkDir(baseDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		switch {
		case entry.IsDir():
			return nil
		case strings.HasSuffix(name, ".config.yaml"):
			t.Errorf("sidecar %s must use the %s suffix", path, goldenConfigSuffix)
		case strings.HasSuffix(name, goldenConfigSuffix):
			inputPath := strings.TrimSuffix(path, goldenConfigSuffix) + ".input.md"
			if _, statErr := os.Stat(inputPath); statErr != nil {
				t.Errorf("sidecar %s has no matching input %s", path, inputPath)
			}
		}
		return nil
	})
	require.NoError(t, err)
}
