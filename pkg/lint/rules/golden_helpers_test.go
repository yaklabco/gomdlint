package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// GoldenTestCase represents a single golden file test case.
type GoldenTestCase struct {
	// Name is the test case name derived from the file path.
	Name string

	// InputPath is the absolute path to the input markdown file.
	InputPath string

	// GoldenPath is the absolute path to the expected output after fixes.
	GoldenPath string

	// DiagsJSONPath is the path to the expected diagnostics JSON file.
	DiagsJSONPath string

	// DiagsTxtPath is the path to the expected diagnostics text file.
	DiagsTxtPath string

	// ConfigPath is the path to the optional sidecar configuration file.
	// The file need not exist; without it the case uses the product defaults.
	ConfigPath string

	// RuleID is the rule to test (empty means run all rules).
	RuleID string

	// IsRealWorld indicates this is a real-world test (all rules).
	IsRealWorld bool
}

// DiagExpectation represents an expected diagnostic in JSON format.
type DiagExpectation struct {
	Rule     string `json:"rule"`
	Name     string `json:"name"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Fixable  bool   `json:"fixable"`
}

// diagFromLint converts a lint.Diagnostic to a DiagExpectation.
func diagFromLint(diag lint.Diagnostic) DiagExpectation {
	return DiagExpectation{
		Rule:     diag.RuleID,
		Name:     diag.RuleName,
		Line:     diag.StartLine,
		Column:   diag.StartColumn,
		Message:  diag.Message,
		Severity: string(diag.Severity),
		Fixable:  diag.HasFix(),
	}
}

// discoverTestCases walks the testdata directory and discovers all test cases.
// Per-rule directories (like MD001, MD031) run only that specific rule.
// The real-world directory runs all enabled rules.
func discoverTestCases(t *testing.T, baseDir string) []GoldenTestCase {
	t.Helper()

	cases := make([]GoldenTestCase, 0)

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return cases
		}
		t.Fatalf("failed to read testdata directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		dirPath := filepath.Join(baseDir, dirName)

		// Determine if this is a rule-specific directory or real-world
		isRealWorld := dirName == "real-world"
		ruleID := ""
		if !isRealWorld && isRuleID(dirName) {
			ruleID = dirName
		}

		// Find all *.input.md files in this directory
		inputFiles, err := filepath.Glob(filepath.Join(dirPath, "*.input.md"))
		if err != nil {
			t.Fatalf("failed to glob input files in %s: %v", dirPath, err)
		}

		for _, inputPath := range inputFiles {
			baseName := strings.TrimSuffix(filepath.Base(inputPath), ".input.md")

			tc := GoldenTestCase{
				Name:          filepath.Join(dirName, baseName),
				InputPath:     inputPath,
				GoldenPath:    filepath.Join(dirPath, baseName+".golden.md"),
				DiagsJSONPath: filepath.Join(dirPath, baseName+".diags.json"),
				DiagsTxtPath:  filepath.Join(dirPath, baseName+".diags.txt"),
				ConfigPath:    filepath.Join(dirPath, baseName+goldenConfigSuffix),
				RuleID:        ruleID,
				IsRealWorld:   isRealWorld,
			}
			cases = append(cases, tc)
		}
	}

	return cases
}

// isRuleID checks if a string looks like a rule ID (MD001, MD031, etc.).
func isRuleID(ruleStr string) bool {
	if len(ruleStr) < 3 {
		return false
	}
	// Check for MD### or MDL### pattern
	if strings.HasPrefix(ruleStr, "MD") {
		rest := strings.TrimPrefix(ruleStr[2:], "L")
		for _, char := range rest {
			if char < '0' || char > '9' {
				return false
			}
		}
		return len(rest) > 0
	}
	return false
}

// loadExpectedDiags loads the expected diagnostics from a JSON file.
// Returns nil if the file doesn't exist.
func loadExpectedDiags(t *testing.T, path string) []DiagExpectation {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("failed to read diagnostics file %s: %v", path, err)
	}

	// Handle empty files
	if len(bytes.TrimSpace(data)) == 0 {
		return []DiagExpectation{}
	}

	var diags []DiagExpectation
	if err := json.Unmarshal(data, &diags); err != nil {
		t.Fatalf("failed to parse diagnostics JSON %s: %v", path, err)
	}

	return diags
}

// loadGoldenFile loads the expected output from a golden file.
// Returns nil if the file doesn't exist.
func loadGoldenFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("failed to read golden file %s: %v", path, err)
	}

	return data
}

// writeDiagsJSON writes diagnostics to a JSON file.
func writeDiagsJSON(t *testing.T, path string, diags []lint.Diagnostic) {
	t.Helper()

	expectations := make([]DiagExpectation, len(diags))
	for i, d := range diags {
		expectations[i] = diagFromLint(d)
	}

	data, err := json.MarshalIndent(expectations, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal diagnostics to JSON: %v", err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create directory for %s: %v", path, err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("failed to write diagnostics JSON %s: %v", path, err)
	}

	t.Logf("Updated golden file: %s", path)
}

// formatDiagLine renders one diagnostic as a line of a .diags.txt file:
//
//	basic.input.md:3:30 warning Trailing whitespace (MD009/no-trailing-spaces) [fixable]
func formatDiagLine(filename string, diag lint.Diagnostic) string {
	fixable := ""
	if diag.HasFix() {
		fixable = " [fixable]"
	}

	return fmt.Sprintf("%s:%d:%d %s %s (%s/%s)%s",
		filename, diag.StartLine, diag.StartColumn, diag.Severity, diag.Message, diag.RuleID, diag.RuleName, fixable)
}

// renderDiagsTxt renders the contents of a .diags.txt file, one line per diagnostic.
func renderDiagsTxt(diags []lint.Diagnostic, filename string) []byte {
	var buf bytes.Buffer
	for _, diag := range diags {
		buf.WriteString(formatDiagLine(filename, diag))
		buf.WriteByte('\n')
	}

	return buf.Bytes()
}

// writeGoldenFile writes content to a golden file.
func writeGoldenFile(t *testing.T, path string, content []byte) {
	t.Helper()

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create directory for %s: %v", path, err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("failed to write golden file %s: %v", path, err)
	}

	t.Logf("Updated golden file: %s", path)
}

// compareWithGolden compares actual bytes with the golden file.
// If update is true, it updates the golden file instead of comparing.
func compareWithGolden(t *testing.T, actualBytes []byte, goldenPath string, update bool) {
	t.Helper()

	if update {
		writeGoldenFile(t, goldenPath, actualBytes)
		return
	}

	expected := loadGoldenFile(t, goldenPath)
	if expected == nil {
		t.Errorf("Golden file does not exist: %s\nRun with -update flag to create it.", goldenPath)
		t.Logf("Actual content:\n%s", string(actualBytes))
		return
	}

	if !bytes.Equal(actualBytes, expected) {
		t.Errorf("Output does not match golden file: %s", goldenPath)
		t.Logf("Expected:\n%s", string(expected))
		t.Logf("Actual:\n%s", string(actualBytes))

		// Show diff for easier debugging
		showDiff(t, expected, actualBytes)
	}
}

// compareDiags compares actual diagnostics with expected diagnostics.
// If update is true, it updates the golden files instead of comparing.
func compareDiags(t *testing.T, actual []lint.Diagnostic, tc GoldenTestCase, update bool) {
	t.Helper()

	inputFilename := filepath.Base(tc.InputPath)
	actualTxt := renderDiagsTxt(actual, inputFilename)

	if update {
		writeDiagsJSON(t, tc.DiagsJSONPath, actual)
		writeGoldenFile(t, tc.DiagsTxtPath, actualTxt)
		return
	}

	// The text file renders the same diagnostics; a stale one fails too.
	compareWithGolden(t, actualTxt, tc.DiagsTxtPath, false)

	expected := loadExpectedDiags(t, tc.DiagsJSONPath)
	if expected == nil {
		t.Errorf("Diagnostics JSON file does not exist: %s\nRun with -update flag to create it.", tc.DiagsJSONPath)
		t.Logf("Actual diagnostics: %d", len(actual))
		for _, d := range actual {
			t.Logf("  %s", formatDiagLine(inputFilename, d))
		}
		return
	}

	// Compare counts first
	if len(actual) != len(expected) {
		t.Errorf("Diagnostic count mismatch: got %d, want %d", len(actual), len(expected))
		t.Logf("Expected diagnostics:")
		for _, d := range expected {
			t.Logf("  %s:%d:%d %s %s (%s/%s)", inputFilename, d.Line, d.Column, d.Severity, d.Message, d.Rule, d.Name)
		}
		t.Logf("Actual diagnostics:")
		for _, d := range actual {
			t.Logf("  %s", formatDiagLine(inputFilename, d))
		}
		return
	}

	// Compare each diagnostic
	for idx := range actual {
		got := diagFromLint(actual[idx])
		want := expected[idx]

		assert.Equal(t, want.Rule, got.Rule, "diagnostic %d: rule mismatch", idx)
		assert.Equal(t, want.Name, got.Name, "diagnostic %d: name mismatch", idx)
		assert.Equal(t, want.Line, got.Line, "diagnostic %d: line mismatch", idx)
		assert.Equal(t, want.Column, got.Column, "diagnostic %d: column mismatch", idx)
		assert.Equal(t, want.Message, got.Message, "diagnostic %d: message mismatch", idx)
		assert.Equal(t, want.Severity, got.Severity, "diagnostic %d: severity mismatch", idx)
		assert.Equal(t, want.Fixable, got.Fixable, "diagnostic %d: fixable mismatch", idx)
	}
}

// showDiff displays a simple diff between expected and actual content.
func showDiff(t *testing.T, expected, actual []byte) {
	t.Helper()

	expectedLines := bytes.Split(expected, []byte("\n"))
	actualLines := bytes.Split(actual, []byte("\n"))

	maxLines := len(expectedLines)
	if len(actualLines) > maxLines {
		maxLines = len(actualLines)
	}

	var diffBuf bytes.Buffer
	for lineNum := range maxLines {
		var expLine, actLine string
		if lineNum < len(expectedLines) {
			expLine = string(expectedLines[lineNum])
		}
		if lineNum < len(actualLines) {
			actLine = string(actualLines[lineNum])
		}

		if expLine != actLine {
			if expLine != "" {
				diffBuf.WriteString("- ")
				diffBuf.WriteString(expLine)
				diffBuf.WriteString("\n")
			}
			if actLine != "" {
				diffBuf.WriteString("+ ")
				diffBuf.WriteString(actLine)
				diffBuf.WriteString("\n")
			}
		}
	}

	if diffBuf.Len() > 0 {
		t.Logf("Diff (- expected, + actual):\n%s", diffBuf.String())
	}
}

// getRuleByID gets a rule by its ID from the default registry.
func getRuleByID(t *testing.T, ruleID string) lint.Rule {
	t.Helper()

	rule, ok := lint.DefaultRegistry.GetByID(ruleID)
	if !ok {
		t.Fatalf("rule %s not found in registry", ruleID)
	}

	return rule
}

// getEnabledRules returns all default-enabled rules from the registry.
func getEnabledRules(t *testing.T) []lint.Rule {
	t.Helper()

	var enabled []lint.Rule
	for _, rule := range lint.DefaultRegistry.Rules() {
		if rule.DefaultEnabled() {
			enabled = append(enabled, rule)
		}
	}

	return enabled
}

// filterFixableDiags returns only diagnostics that have fixes.
func filterFixableDiags(diags []lint.Diagnostic) []lint.Diagnostic {
	var fixable []lint.Diagnostic
	for _, d := range diags {
		if d.HasFix() {
			fixable = append(fixable, d)
		}
	}
	return fixable
}

// countFixableDiags returns the number of diagnostics with fixes.
func countFixableDiags(diags []lint.Diagnostic) int {
	count := 0
	for _, diag := range diags {
		if diag.HasFix() {
			count++
		}
	}
	return count
}
