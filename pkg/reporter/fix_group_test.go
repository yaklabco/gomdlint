package reporter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// fixGroupContent is a list whose nested bullets MD004 replaces together.
const fixGroupContent = "- a\n  * b\n  * c\n- d\n"

// fixGroupDiagnostics returns the two MD004 findings of fixGroupContent, in
// fix group 1, and an MD009 finding with a fix of its own.
func fixGroupDiagnostics() []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			RuleID: "MD004", StartLine: 2, StartColumn: 3, FixGroup: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 6, EndOffset: 7, NewText: "-"}},
		},
		{
			RuleID: "MD004", StartLine: 3, StartColumn: 3, FixGroup: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 12, EndOffset: 13, NewText: "-"}},
		},
		{
			RuleID: "MD009", StartLine: 4, StartColumn: 4,
			FixEdits: []fix.TextEdit{{StartOffset: 17, EndOffset: 18, NewText: "D"}},
		},
	}
}

// TestJSONReporter_ShowsFixGroups covers the fix group of a finding whose
// fix --fix applies together with those of other findings of its rule, so
// that a consumer can apply them together too.
func TestJSONReporter_ShowsFixGroups(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	_, err := reporter.NewJSONReporter(reporter.Options{Writer: &buf}).Report(context.Background(),
		&runner.Result{Files: []runner.FileOutcome{sarifFile("a.md", fixGroupContent, fixGroupDiagnostics()...)}})
	require.NoError(t, err)

	var output struct {
		Files []struct {
			Diagnostics []map[string]json.RawMessage `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))
	require.Len(t, output.Files, 1)
	diags := output.Files[0].Diagnostics
	require.Len(t, diags, 3)
	assert.JSONEq(t, "1", string(diags[0]["fixGroup"]))
	assert.JSONEq(t, "1", string(diags[1]["fixGroup"]))
	assert.NotContains(t, diags[2], "fixGroup", "a fix of its own has no group")
}

// TestSARIFReporter_ShowsFixGroups covers the fix group of a result, in its
// property bag, next to its fix.
func TestSARIFReporter_ShowsFixGroups(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), fixGroupContent, fixGroupDiagnostics()...),
	}}

	var log struct {
		Runs []struct {
			Results []struct {
				Fixes      []json.RawMessage `json:"fixes"`
				Properties map[string]any    `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(reportSARIF(t, reporter.Options{WorkingDir: root}, result), &log))
	require.Len(t, log.Runs, 1)
	results := log.Runs[0].Results
	require.Len(t, results, 3)
	for idx := range 2 {
		require.Len(t, results[idx].Fixes, 1, "result %d", idx)
		assert.Equal(t, map[string]any{"fixGroup": float64(1)}, results[idx].Properties, "result %d", idx)
	}
	assert.Nil(t, results[2].Properties, "a fix of its own has no group")
}
