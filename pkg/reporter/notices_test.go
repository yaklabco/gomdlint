package reporter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// typoNotice and formerTagNotice are the notices of two markdownlint
// comments: one that names a rule with a typo and one that names a former
// gomdlint tag.
var (
	typoNotice = lint.Notice{
		Kind:        lint.NoticeUnknownRule,
		Line:        3,
		Directive:   "markdownlint-disable",
		Name:        "no-trailling-spaces",
		Suggestions: []string{"no-trailing-spaces"},
		Message:     `markdownlint-disable: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
	}
	formerTagNotice = lint.Notice{
		Kind:      lint.NoticeDeprecatedTag,
		Line:      7,
		Directive: "markdownlint-disable-next-line",
		Name:      "lists",
		Rules:     []string{"ul-style"},
		Message: `markdownlint-disable-next-line: deprecated tag "lists" ` +
			`(write its rules instead: ul-style); it will stop working in a later release`,
	}
)

// noticeResult is a run over two files: a.md, with a finding and the two
// notices, and b.md, with neither.
func noticeResult(dir string) *runner.Result {
	noticed := sarifFile(filepath.Join(dir, "a.md"), "# A\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1})
	noticed.Notices = []lint.Notice{typoNotice, formerTagNotice}
	quiet := sarifFile(filepath.Join(dir, "b.md"), "# B\n")
	return &runner.Result{Files: []runner.FileOutcome{noticed, quiet}}
}

func TestJSONReporter_ListsNoticesPerFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var buf bytes.Buffer
	_, err := reporter.NewJSONReporter(reporter.Options{Writer: &buf, WorkingDir: dir}).
		Report(context.Background(), noticeResult(dir))
	require.NoError(t, err)

	var output struct {
		Files   []map[string]json.RawMessage `json:"files"`
		Summary map[string]json.RawMessage   `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))
	require.Len(t, output.Files, 2)

	assert.JSONEq(t, `[
		{
			"line": 3,
			"kind": "unknown-rule",
			"directive": "markdownlint-disable",
			"name": "no-trailling-spaces",
			"suggestions": ["no-trailing-spaces"],
			"message": "markdownlint-disable: unknown rule \"no-trailling-spaces\" (did you mean \"no-trailing-spaces\"?); it is ignored"
		},
		{
			"line": 7,
			"kind": "deprecated-tag",
			"directive": "markdownlint-disable-next-line",
			"name": "lists",
			"rules": ["ul-style"],
			"message": "markdownlint-disable-next-line: deprecated tag \"lists\" (write its rules instead: ul-style); it will stop working in a later release"
		}
	]`, string(output.Files[0]["notices"]))
	assert.NotContains(t, output.Files[1], "notices", "a file without notices has no notices key")

	assert.JSONEq(t, "1", string(output.Summary["totalIssues"]), "notices are not issues")
}

func TestJSONReporter_OmitsEmptySuggestions(t *testing.T) {
	t.Parallel()

	file := sarifFile("a.md", "# A\n")
	file.Notices = []lint.Notice{{
		Kind: lint.NoticeUnknownRule, Line: 1, Directive: "markdownlint-disable", Name: "MD002",
		Message: `markdownlint-disable: unknown rule "MD002"; it is ignored`,
	}}
	var buf bytes.Buffer
	_, err := reporter.NewJSONReporter(reporter.Options{Writer: &buf}).
		Report(context.Background(), &runner.Result{Files: []runner.FileOutcome{file}})
	require.NoError(t, err)

	var output struct {
		Files []struct {
			Notices []map[string]json.RawMessage `json:"notices"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))
	require.Len(t, output.Files, 1)
	require.Len(t, output.Files[0].Notices, 1)
	assert.NotContains(t, output.Files[0].Notices[0], "suggestions")
	assert.NotContains(t, output.Files[0].Notices[0], "rules", "an unknown word covers no rules")
}

func TestSARIFReporter_RecordsNoticesAsConfigurationNotifications(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, noticeResult(root)).Runs[0]

	require.Len(t, run.Invocations, 1)
	invocation := run.Invocations[0]
	assert.True(t, invocation.ExecutionSuccessful, "a notice does not make the run fail")
	assert.Empty(t, invocation.ToolExecutionNotifications)

	location := func(line int) []reporter.SARIFLocation {
		return []reporter.SARIFLocation{{PhysicalLocation: reporter.SARIFPhysicalLocation{
			ArtifactLocation: reporter.SARIFArtifactLocation{URI: "a.md", URIBaseID: "%SRCROOT%"},
			Region:           &reporter.SARIFRegion{StartLine: line},
		}}}
	}
	assert.Equal(t, []reporter.SARIFNotification{
		{Level: "warning", Message: reporter.SARIFMessage{Text: typoNotice.Message}, Locations: location(3)},
		{Level: "warning", Message: reporter.SARIFMessage{Text: formerTagNotice.Message}, Locations: location(7)},
	}, invocation.ToolConfigurationNotifications)
	assert.Len(t, run.Results, 1, "notices are not results")
}

func TestSARIFReporter_LocatesNoticesWithoutALineAtTheirFile(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	file := sarifFile(filepath.Join(root, "a.md"), "# A\n")
	file.Notices = []lint.Notice{{Kind: lint.NoticeUnknownRule, Message: "markdownlint-disable: unknown rule \"x\"; it is ignored"}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, &runner.Result{Files: []runner.FileOutcome{file}}).Runs[0]

	require.Len(t, run.Invocations, 1)
	require.Len(t, run.Invocations[0].ToolConfigurationNotifications, 1)
	assert.Equal(t, []reporter.SARIFLocation{{PhysicalLocation: reporter.SARIFPhysicalLocation{
		ArtifactLocation: reporter.SARIFArtifactLocation{URI: "a.md", URIBaseID: "%SRCROOT%"},
	}}}, run.Invocations[0].ToolConfigurationNotifications[0].Locations, "SARIF lines start at 1, so there is no region")
}
