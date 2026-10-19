package rules

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// update is a flag to update golden files instead of comparing:
//
//	go test ./pkg/lint/rules/ -run 'TestGoldenPerRule/<ID>' -update
//
// The flag goes after the package path: go test stops reading package paths
// at the first flag it does not know, so -update before the path tests the
// repository root instead.
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
//  2. Apply all fixes as --fix does, pass by pass, re-parsing the fixed
//     content and re-running the rules after each pass
//  3. Assert that the passes converge and zero fixable diagnostics remain.
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

	// Lint with the case's configuration and only its rule enabled
	run := mustGoldenRun(t, tc)
	result := run.mustLintFile(t, filepath.Base(tc.InputPath), input)

	// Compare diagnostics and the output after applying the fixes
	compareDiags(t, result.Diagnostics, tc, updateGolden)
	compareWithGolden(t, fix.ApplyEdits(input, result.Edits), tc.GoldenPath, updateGolden)
}

// runRealWorldGoldenTest executes a single real-world golden test.
func runRealWorldGoldenTest(t *testing.T, tc GoldenTestCase, updateGolden bool) {
	t.Helper()

	// Read input file
	input, err := os.ReadFile(tc.InputPath)
	require.NoError(t, err, "failed to read input file: %s", tc.InputPath)

	// Lint with the case's configuration and the default-enabled rules
	run := mustGoldenRun(t, tc)
	name := filepath.Base(tc.InputPath)
	result := run.mustLintFile(t, name, input)

	// Sort diagnostics by line, column for consistent output
	diags := result.Diagnostics
	sortDiagnostics(diags)

	// Compare diagnostics and the output of --fix, which applies the fixes
	// of several rules over several passes
	compareDiags(t, diags, tc, updateGolden)
	fixed, _ := run.mustFixFile(t, name, input)
	compareWithGolden(t, fixed, tc.GoldenPath, updateGolden)
}

// runRoundTripTest verifies that applying fixes results in zero fixable diagnostics.
func runRoundTripTest(t *testing.T, tc GoldenTestCase) {
	t.Helper()

	// Read input file
	input, err := os.ReadFile(tc.InputPath)
	require.NoError(t, err, "failed to read input file: %s", tc.InputPath)

	// Lint with the case's configuration and rules
	run := mustGoldenRun(t, tc)
	name := filepath.Base(tc.InputPath)
	first := run.mustLintFile(t, name, input)

	// Check if there are any fixable diagnostics
	if countFixableDiags(first.Diagnostics) == 0 {
		// No fixable diagnostics, round-trip test passes trivially
		return
	}

	// Fix the content as --fix does, linting it again after each pass
	fixedContent, second := run.mustFixFile(t, name, input)
	assert.False(t, second.Exhausted, "the fixes did not converge within the pass limit")
	assert.False(t, second.Oscillated, "the fixes recreate earlier content")

	// Assert zero fixable diagnostics remain
	remainingFixable := filterFixableDiags(second.Diagnostics)
	if len(remainingFixable) > 0 {
		t.Errorf("Round-trip test failed: %d fixable diagnostics remain after applying fixes", len(remainingFixable))
		for _, d := range remainingFixable {
			t.Logf("  %s", formatDiagLine(name, diagFromLint(d)))
		}
		t.Logf("Fixed content:\n%s", string(fixedContent))
	}
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

	t.Run("renderDiagsTxt", func(t *testing.T) {
		diag := lint.Diagnostic{
			RuleID:      "MD009",
			RuleName:    "no-trailing-spaces",
			Message:     "Trailing whitespace",
			Severity:    config.SeverityWarning,
			StartLine:   3,
			StartColumn: 30,
		}
		fixable := diag
		fixable.StartLine = 5
		fixable.FixEdits = []fix.TextEdit{{StartOffset: 1, EndOffset: 2}}

		assert.Equal(t, "basic.input.md:3:30 warning Trailing whitespace (MD009/no-trailing-spaces)\n"+
			"basic.input.md:5:30 warning Trailing whitespace (MD009/no-trailing-spaces) [fixable]\n",
			string(renderDiagsTxt("basic.input.md", []lint.Diagnostic{diag, fixable})))
		assert.Empty(t, renderDiagsTxt("basic.input.md", nil))
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

	t.Run("discoverTestCases_unsupported_directory", func(t *testing.T) {
		baseDir := t.TempDir()
		for _, rel := range []string{"MD043/a.input.md", "real-world/b.input.md", "MD05S/c.input.md", "notes/d.input.md"} {
			path := filepath.Join(baseDir, rel)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("# Title\n"), 0o644))
		}

		var names []string
		for _, tc := range discoverTestCases(t, baseDir) {
			names = append(names, tc.Name)
		}
		assert.ElementsMatch(t, []string{filepath.Join("MD043", "a"), filepath.Join("real-world", "b")}, names)
	})
}

// writeSidecar writes sidecar content to a fresh temporary file and returns its path.
func writeSidecar(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "case"+goldenConfigSuffix)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	return path
}

// absentSidecar returns the path of a sidecar that does not exist.
func absentSidecar(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "absent"+goldenConfigSuffix)
}

// TestGoldenSidecarConfig verifies loading of per-fixture sidecar configuration.
func TestGoldenSidecarConfig(t *testing.T) {
	t.Run("missing sidecar uses product defaults", func(t *testing.T) {
		cfg, err := loadGoldenConfig(absentSidecar(t))
		require.NoError(t, err)

		assert.Equal(t, config.NewConfig(), cfg)
	})

	t.Run("flavor and rule options", func(t *testing.T) {
		content := `flavor: gfm
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
`
		cfg, err := loadGoldenConfig(writeSidecar(t, content))
		require.NoError(t, err)

		assert.Equal(t, config.FlavorGFM, cfg.Flavor)
		assert.Equal(t, map[string]any{"headings": []any{"# Title", "*"}, "match_case": true}, cfg.Rules["MD043"].Options)
		assert.Equal(t, map[string]any{"line_length": 100}, cfg.Rules["MD013"].Options)
		assert.Len(t, cfg.Rules, 2)

		// Option values have the types that product config loading produces.
		product, err := config.FromYAML([]byte(content))
		require.NoError(t, err)
		require.Len(t, product.Rules, 2)
		for ruleID, ruleCfg := range product.Rules {
			assert.Equal(t, ruleCfg.Options, cfg.Rules[ruleID].Options, "options of %s", ruleID)
		}
	})

	t.Run("explicit commonmark flavor", func(t *testing.T) {
		cfg, err := loadGoldenConfig(writeSidecar(t, "flavor: commonmark\n"))
		require.NoError(t, err)

		assert.Equal(t, config.FlavorCommonMark, cfg.Flavor)
	})

	// A sidecar writes options in the shapes a config file may use:
	// directly under the rule ID, as markdownlint writes them, under
	// options, or both, with gomdlint's names or markdownlint's.
	t.Run("flat rule options", func(t *testing.T) {
		content := `rules:
  MD043:
    headings:
      - "# Title"
    options:
      match_case: true
  MD013:
    line_length: 100
    code_blocks: false
`
		cfg, err := loadGoldenConfig(writeSidecar(t, content))
		require.NoError(t, err)

		assert.Equal(t, map[string]any{"headings": []any{"# Title"}, "match_case": true}, cfg.Rules["MD043"].Options)
		assert.Equal(t, map[string]any{"line_length": 100, "code_blocks": false}, cfg.Rules["MD013"].Options)
		assert.Len(t, cfg.Rules, 2)

		product, err := config.FromYAML([]byte(content))
		require.NoError(t, err)
		for ruleID, ruleCfg := range product.Rules {
			assert.Equal(t, ruleCfg.Options, cfg.Rules[ruleID].Options, "options of %s", ruleID)
		}
	})

	// Rules read a value that an option does not allow as markdownlint
	// does, so a scenario may check how they treat one.
	t.Run("value the option does not allow", func(t *testing.T) {
		cfg, err := loadGoldenConfig(writeSidecar(t, "rules:\n  MD004:\n    style: dashes\n"))
		require.NoError(t, err)

		assert.Equal(t, map[string]any{"style": "dashes"}, cfg.Rules["MD004"].Options)
	})

	t.Run("errors name the first bad rule in ID order", func(t *testing.T) {
		path := writeSidecar(t, "rules:\n  MD999:\n    options:\n      a: 1\n  MD998:\n    options:\n      a: 1\n")
		for range 20 {
			_, err := loadGoldenConfig(path)
			require.ErrorContains(t, err, "rules.MD998:")
		}
	})

	invalid := []struct {
		name    string
		content string
		wantIs  error
		wantErr string
	}{
		{name: "empty file", content: "", wantIs: errEmptySidecar},
		{name: "comment only", content: "# nothing configured\n", wantIs: errEmptySidecar},
		{name: "flavor without a value", content: "flavor:\n", wantIs: errEmptySidecar},
		{name: "second document", content: "flavor: gfm\n---\nflavor: commonmark\n", wantIs: errMultipleDocuments},
		{name: "misspelled key", content: "flavour: gfm\n", wantErr: "flavour"},
		{name: "unsupported config key", content: "severity_default: error\n", wantErr: "severity_default"},
		{name: "unsupported rule key", content: "rules:\n  MD009:\n    severity: error\n", wantErr: "severity"},
		{name: "invalid flavor", content: "flavor: markdown\n", wantErr: `"markdown"`},
		{name: "unknown rule", content: "rules:\n  MD999:\n    options:\n      style: x\n", wantErr: "rules.MD999: unknown rule"},
		{name: "rule name instead of ID", content: "rules:\n  proper-names:\n    options:\n      names: [GitHub]\n", wantErr: "use the rule ID MD044"},
		{name: "rule without options", content: "rules:\n  MD009: {}\n", wantErr: "rules.MD009 sets no options"},
		{name: "rule turned on", content: "rules:\n  MD044:\n    enabled: true\n    names: [GitHub]\n", wantErr: "rules.MD044.enabled"},
		{name: "rule shorthand", content: "rules:\n  MD044: true\n", wantErr: "rules.MD044.enabled"},
		{name: "rule severity shorthand", content: "rules:\n  MD044: warning\n", wantErr: "rules.MD044.severity"},
		{
			name:    "rule severity shorthand through an alias",
			content: "flavor: &level gfm\nrules:\n  MD044: *level\n",
			wantErr: "rules.MD044.severity",
		},
		{name: "rule auto-fix", content: "rules:\n  MD044:\n    auto_fix: false\n    names: [GitHub]\n", wantErr: "rules.MD044.auto_fix"},
		{name: "null severity", content: "rules:\n  MD013:\n    max: 100\n    severity: null\n", wantErr: "rules.MD013.severity"},
		{name: "null enabled", content: "rules:\n  MD013:\n    max: 100\n    enabled:\n", wantErr: "rules.MD013.enabled"},
		{name: "null auto-fix", content: "rules:\n  MD013:\n    max: 100\n    auto_fix: ~\n", wantErr: "rules.MD013.auto_fix"},
		{name: "merged severity", content: "rules:\n  MD013:\n    <<: {severity: null}\n    max: 100\n", wantErr: "rules.MD013.severity"},
		{
			name:    "unknown option",
			content: "rules:\n  MD044:\n    options:\n      nmes: [GitHub]\n",
			wantErr: `rules.MD044: unknown option "nmes" for proper-names (did you mean "names"?)`,
		},
		{
			name:    "unknown flat option",
			content: "rules:\n  MD044:\n    nmes: [GitHub]\n",
			wantErr: `rules.MD044: unknown option "nmes" for proper-names (did you mean "names"?)`,
		},
		{
			name:    "option of the wrong type",
			content: "rules:\n  MD013:\n    line_length: long\n",
			wantErr: `rules.MD013: line-length option "line_length" must be an integer, not "long"`,
		},
		{
			name:    "list item of the wrong type",
			content: "rules:\n  MD044:\n    names: [GitHub, 42]\n",
			wantErr: `rules.MD044: proper-names option "names" item 2 must be a string, not 42`,
		},
		{
			name:    "two spellings of an option",
			content: "rules:\n  MD013:\n    line_length: 100\n    options:\n      max: 90\n",
			wantErr: `rules.MD013: "line_length" and "max" both set line-length option "max"`,
		},
		{
			name:    "option set flat and under options",
			content: "rules:\n  MD013:\n    max: 100\n    options:\n      max: 90\n",
			wantErr: `"max" and "options.max" both set option "max"`,
		},
		{
			name:    "option set twice through an alias",
			content: "rules:\n  MD013:\n    &opt line_length: 100\n    *opt : 20\n",
			wantErr: `line 4: mapping key "line_length" already defined at line 3`,
		},
		{
			name:    "option set twice through an alias under options",
			content: "rules:\n  MD013:\n    options:\n      &opt max: 100\n      *opt : 20\n",
			wantErr: `line 5: mapping key "max" already defined at line 4`,
		},
		{
			name:    "rule set twice through an alias",
			content: "rules:\n  &id MD013:\n    line_length: 100\n  *id :\n    line_length: 90\n",
			wantErr: `line 4: mapping key "MD013" already defined at line 2`,
		},
		{
			// A sidecar without alias keys gets every error of its decode.
			name:    "type error beside a misspelled key",
			content: "rules:\n  MD013:\n    enabled: 3\nflavour: gfm\n",
			wantErr: "field flavour not found",
		},
		{
			// The rule keys are checked before the sidecar is decoded.
			name:    "rule set twice through an alias beside a misspelled key",
			content: "rules:\n  &id MD013:\n    line_length: 100\n  *id :\n    line_length: 90\nflavour: gfm\n",
			wantErr: `line 4: mapping key "MD013" already defined at line 2`,
		},
		{
			// yaml.v3 panics on such a key beside a merge key.
			name:    "sequence key beside a merge key",
			content: "rules:\n  <<: {MD013: {line_length: 100}}\n  ? [MD044]\n  : {names: [GitHub]}\n",
			wantErr: "line 3: mapping key must be a scalar",
		},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() { _, err = loadGoldenConfig(writeSidecar(t, tt.content)) })
			if tt.wantIs != nil {
				require.ErrorIs(t, err, tt.wantIs)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestGoldenRun verifies how a case is prepared from its sidecar and directory.
func TestGoldenRun(t *testing.T) {
	table := []byte("| a | b |\n| - | - |\n| 1 | 2 |\n")

	t.Run("parser follows the product default flavor", func(t *testing.T) {
		run := mustGoldenRun(t, GoldenTestCase{ConfigPath: absentSidecar(t), RuleID: "MD056"})

		assert.Equal(t, string(config.NewConfig().Flavor), run.parser.Flavor())
	})

	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run("sidecar flavor "+string(flavor), func(t *testing.T) {
			run := mustGoldenRun(t, GoldenTestCase{ConfigPath: writeSidecar(t, "flavor: "+string(flavor)+"\n"), RuleID: "MD056"})

			snapshot, err := run.parser.Parse(context.Background(), "table.input.md", table)
			require.NoError(t, err)
			wantTables := 0
			if flavor == config.FlavorGFM {
				wantTables = 1
			}
			assert.Len(t, lint.NewRuleContext(context.Background(), snapshot, run.cfg, nil).Tables(), wantTables)
			assert.Equal(t, flavor, run.cfg.Flavor)
		})
	}

	t.Run("per-rule case runs its rule with the sidecar's options", func(t *testing.T) {
		sidecar := writeSidecar(t, "rules:\n  MD044:\n    options:\n      names: [GitHub]\n")
		run := mustGoldenRun(t, GoldenTestCase{ConfigPath: sidecar, RuleID: "MD044"})

		result := run.mustLintFile(t, "names.input.md", []byte("# Title\n\nUse github here.\n"))
		require.Len(t, result.Diagnostics, 1)
		assert.Equal(t, 3, result.Diagnostics[0].StartLine)
	})

	t.Run("per-rule case runs no other rule", func(t *testing.T) {
		run := mustGoldenRun(t, GoldenTestCase{ConfigPath: absentSidecar(t), RuleID: "MD047"})

		// MD009 would report the trailing spaces.
		result := run.mustLintFile(t, "spaces.input.md", []byte("# Title\n\nTrailing   \n"))
		assert.Empty(t, result.Diagnostics)
	})

	t.Run("real-world case runs the default-enabled rules", func(t *testing.T) {
		run := mustGoldenRun(t, GoldenTestCase{ConfigPath: absentSidecar(t), IsRealWorld: true})

		assert.Equal(t, defaultEnabledRules(lint.DefaultRegistry), run.rules)
	})

	t.Run("options for rules the case does not run are rejected", func(t *testing.T) {
		sidecar := writeSidecar(t, `rules:
  MD043:
    options:
      headings: ["# Title"]
  MD044:
    options:
      names: [GitHub]
`)

		_, err := newGoldenRun(GoldenTestCase{ConfigPath: sidecar, RuleID: "MD044"})
		require.ErrorContains(t, err, "configures rules the case does not run: MD043")
	})

	// Every rule runs by default, so a real-world case runs MD043 and MD044
	// too, and its sidecar can give them the options they need to check
	// anything.
	t.Run("real-world case takes options for every rule", func(t *testing.T) {
		sidecar := writeSidecar(t, "rules:\n  MD044:\n    options:\n      names: [GitHub]\n")
		run := mustGoldenRun(t, GoldenTestCase{ConfigPath: sidecar, IsRealWorld: true})

		result := run.mustLintFile(t, "names.input.md", []byte("# Title\n\nUse github here.\n"))
		var lines []int
		for _, diag := range result.Diagnostics {
			if diag.RuleID == "MD044" {
				lines = append(lines, diag.StartLine)
			}
		}
		assert.Equal(t, []int{3}, lines)
	})

	t.Run("invalid sidecar is rejected", func(t *testing.T) {
		_, err := newGoldenRun(GoldenTestCase{ConfigPath: writeSidecar(t, "flavour: gfm\n"), RuleID: "MD009"})
		require.ErrorContains(t, err, "flavour")
	})

	t.Run("case that is neither per-rule nor real-world is rejected", func(t *testing.T) {
		_, err := newGoldenRun(GoldenTestCase{Name: "MD05S/case", ConfigPath: absentSidecar(t)})
		require.ErrorContains(t, err, "MD05S/case")
	})
}

// stubRule is a lint.Rule that returns a fixed result.
type stubRule struct {
	lint.BaseRule

	diags []lint.Diagnostic
	err   error
}

// Apply returns a copy of the stub's diagnostics and its error.
func (r *stubRule) Apply(_ *lint.RuleContext) ([]lint.Diagnostic, error) {
	return slices.Clone(r.diags), r.err
}

// newStubRule returns a stub rule MD999 named stub-rule.
func newStubRule(diags ...lint.Diagnostic) *stubRule {
	return &stubRule{BaseRule: lint.NewBaseRule("MD999", "stub-rule", "Stub rule", nil, false), diags: diags}
}

// stubRun returns a run whose registry holds only the given rule.
func stubRun(rule *stubRule) goldenRun {
	registry := lint.NewRegistry()
	registry.Register(rule)
	cfg := config.NewConfig()

	return goldenRun{cfg: cfg, parser: goldmark.New(string(cfg.Flavor)), registry: registry, rules: []lint.Rule{rule}}
}

// TestGoldenLintFile verifies that golden cases record what the engine reports.
func TestGoldenLintFile(t *testing.T) {
	content := []byte("# Title\n")

	t.Run("engine fills the rule name and resolves the severity", func(t *testing.T) {
		rule := newStubRule(lint.Diagnostic{RuleID: "MD999", Message: "stub", Severity: config.SeverityError, StartLine: 1, StartColumn: 1})

		result, err := stubRun(rule).lintFile("stub.input.md", content)
		require.NoError(t, err)
		require.Len(t, result.Diagnostics, 1)
		assert.Equal(t, "stub-rule", result.Diagnostics[0].RuleName)
		assert.Equal(t, config.SeverityWarning, result.Diagnostics[0].Severity)
	})

	t.Run("inline directives apply", func(t *testing.T) {
		run := mustGoldenRun(t, GoldenTestCase{ConfigPath: absentSidecar(t), RuleID: "MD009"})

		result := run.mustLintFile(t, "directive.input.md", []byte("# Title\n\n<!-- markdownlint-disable MD009 -->\nTrailing   \n"))
		assert.Empty(t, result.Diagnostics)
	})

	t.Run("bare enable directive runs no other rule", func(t *testing.T) {
		run := mustGoldenRun(t, GoldenTestCase{ConfigPath: absentSidecar(t), RuleID: "MD047"})

		// The bare enable turns every rule on, and MD009 would report the trailing spaces.
		enableAll := []byte("# Title\n\n<!-- markdownlint-disable -->\n<!-- markdownlint-enable -->\nTrailing   \n")
		result := run.mustLintFile(t, "enable.input.md", enableAll)
		assert.Empty(t, result.Diagnostics)
	})

	directives := []struct {
		name    string
		ruleID  string
		key     string
		content string
	}{
		{name: "legacy alias", ruleID: "MD022", key: "blanks-around-headings", content: "# Title\nText right below the heading.\n"},
		{name: "rule name", ruleID: "MD009", key: "no-trailing-spaces", content: "# Title\n\nTrailing   \n"},
		{name: "tag", ruleID: "MD009", key: "whitespace", content: "# Title\n\nTrailing   \n"},
	}
	for _, tt := range directives {
		t.Run("directive by "+tt.name, func(t *testing.T) {
			run := mustGoldenRun(t, GoldenTestCase{ConfigPath: absentSidecar(t), RuleID: tt.ruleID})

			require.NotEmpty(t, run.mustLintFile(t, "plain.input.md", []byte(tt.content)).Diagnostics)
			disabled := "<!-- markdownlint-disable " + tt.key + " -->\n" + tt.content
			assert.Empty(t, run.mustLintFile(t, "directive.input.md", []byte(disabled)).Diagnostics)
		})
	}

	t.Run("another rule's ID is rejected", func(t *testing.T) {
		rule := newStubRule(lint.Diagnostic{RuleID: "MD001", Message: "stub", StartLine: 1, StartColumn: 1})

		_, err := stubRun(rule).lintFile("stub.input.md", content)
		require.ErrorContains(t, err, "labelled MD001/stub-rule")
	})

	t.Run("another rule's name is rejected", func(t *testing.T) {
		rule := newStubRule(lint.Diagnostic{RuleID: "MD999", RuleName: "other-name", Message: "stub", StartLine: 1, StartColumn: 1})

		_, err := stubRun(rule).lintFile("stub.input.md", content)
		require.ErrorContains(t, err, "labelled MD999/other-name")
	})

	t.Run("rule errors are rejected", func(t *testing.T) {
		rule := newStubRule()
		rule.err = errors.New("stub failure")

		_, err := stubRun(rule).lintFile("stub.input.md", content)
		require.ErrorContains(t, err, "stub failure")
	})
}

// firstMarkRule is a fixable rule MD999 that reports the first "x" of the
// content with a fix to "y", so fixing every "x" takes one pass each.
type firstMarkRule struct {
	lint.BaseRule
}

// Apply reports the first "x".
func (r *firstMarkRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	idx := bytes.IndexByte(ctx.File.Content, 'x')
	if idx < 0 {
		return nil, nil
	}
	line, column := ctx.File.LineAt(idx)
	return []lint.Diagnostic{{
		RuleID: r.ID(), Message: "x", StartLine: line, StartColumn: column,
		FixEdits: []fix.TextEdit{{StartOffset: idx, EndOffset: idx + 1, NewText: "y"}},
	}}, nil
}

// TestGoldenFixFile verifies that golden cases fix content as --fix does.
func TestGoldenFixFile(t *testing.T) {
	rule := &firstMarkRule{BaseRule: lint.NewBaseRule("MD999", "stub-rule", "Stub rule", nil, true)}
	registry := lint.NewRegistry()
	registry.Register(rule)
	cfg := config.NewConfig()
	run := goldenRun{cfg: cfg, parser: goldmark.New(string(cfg.Flavor)), registry: registry, rules: []lint.Rule{rule}}

	fixed, result := run.mustFixFile(t, "passes.input.md", []byte("# Title\n\nax bx cx\n"))

	assert.Equal(t, "# Title\n\nay by cy\n", string(fixed), "the fixes are applied pass by pass")
	assert.Equal(t, 3, result.FixPasses)
	assert.Empty(t, result.Diagnostics, "the result describes the fixed content")
}

// strayTestdataFiles returns the files under baseDir that belong to no
// discovered case, so the harness never reads them. Hidden files are ignored.
func strayTestdataFiles(t *testing.T, baseDir string) []string {
	t.Helper()

	known := make(map[string]bool)
	for _, tc := range discoverTestCases(t, baseDir) {
		for _, path := range []string{tc.InputPath, tc.GoldenPath, tc.DiagsJSONPath, tc.DiagsTxtPath, tc.ConfigPath} {
			known[path] = true
		}
	}

	var stray []string
	err := filepath.WalkDir(baseDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && !known[path] {
			stray = append(stray, path)
		}
		return nil
	})
	require.NoError(t, err)

	return stray
}

// TestGoldenTestdataFiles verifies that every file under testdata belongs to a
// case. A misnamed or orphaned sidecar, a file in a nested directory, or a
// golden left behind by a renamed input would otherwise be ignored.
func TestGoldenTestdataFiles(t *testing.T) {
	t.Run("testdata", func(t *testing.T) {
		assert.Empty(t, strayTestdataFiles(t, testdataDir(t)))
	})

	t.Run("stray files are found", func(t *testing.T) {
		baseDir := t.TempDir()
		write := func(rel string) string {
			path := filepath.Join(baseDir, rel)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("# Title\n"), 0o644))
			return path
		}

		write("MD043/case.input.md")
		write("MD043/case.config.yml")
		write("MD043/case.golden.md")
		write("MD043/.DS_Store")
		want := []string{
			write("MD043/case.config.yaml"),
			write("MD043/orphan.config.yml"),
			write("MD043/renamed.golden.md"),
			write("MD043/nested/deep.input.md"),
			write("top.config.yml"),
			// No runner compares cases in a directory that is neither a rule ID nor real-world.
			write("MD05S/case.input.md"),
			write("MD05S/case.golden.md"),
			write("MD05S/case.diags.json"),
		}

		assert.ElementsMatch(t, want, strayTestdataFiles(t, baseDir))
	})
}
