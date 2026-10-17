package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// contentRule is a test rule whose behaviour is a function of the content.
type contentRule struct {
	lint.BaseRule

	apply func(content []byte) ([]lint.Diagnostic, error)
}

func (r *contentRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return r.apply(ctx.File.Content)
}

func newContentRule(id string, apply func(content []byte) ([]lint.Diagnostic, error)) *contentRule {
	return &contentRule{BaseRule: lint.NewBaseRule(id, "rule-"+id, "", nil, true), apply: apply}
}

// failingRegistry holds rules that fail on content holding a marker word:
// TEST001 panics on "panic", TEST002 returns an error on "boom" and TEST003
// proposes an invalid fix on "invalid". TEST004 fixes "x" to "y", and
// TEST005 panics on "y", so it only fails once fixes have been applied.
func failingRegistry() *lint.Registry {
	whenContains := func(word string, fail func() ([]lint.Diagnostic, error)) func([]byte) ([]lint.Diagnostic, error) {
		return func(content []byte) ([]lint.Diagnostic, error) {
			if !bytes.Contains(content, []byte(word)) {
				return nil, nil
			}
			return fail()
		}
	}

	registry := lint.NewRegistry()
	registry.Register(newContentRule("TEST001", whenContains("panic", func() ([]lint.Diagnostic, error) {
		panic("cannot lint this")
	})))
	registry.Register(newContentRule("TEST002", whenContains("boom", func() ([]lint.Diagnostic, error) {
		return nil, errors.New("frobnicator missing")
	})))
	registry.Register(newContentRule("TEST003", whenContains("invalid", func() ([]lint.Diagnostic, error) {
		return []lint.Diagnostic{{RuleID: "TEST003", Message: "overlap", StartLine: 1, StartColumn: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 3, NewText: "a"}, {StartOffset: 1, EndOffset: 4, NewText: "b"}}}}, nil
	})))
	registry.Register(newContentRule("TEST004", func(content []byte) ([]lint.Diagnostic, error) {
		idx := bytes.IndexByte(content, 'x')
		if idx < 0 {
			return nil, nil
		}
		return []lint.Diagnostic{{RuleID: "TEST004", Message: "x", StartLine: 1, StartColumn: idx + 1,
			FixEdits: []fix.TextEdit{{StartOffset: idx, EndOffset: idx + 1, NewText: "y"}}}}, nil
	}))
	registry.Register(newContentRule("TEST005", whenContains("y", func() ([]lint.Diagnostic, error) {
		panic("cannot lint fixed content")
	})))
	return registry
}

// skipWithoutFilePermissions skips a test that needs a file it cannot read:
// permission bits do not stop root or Windows from reading files.
func skipWithoutFilePermissions(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("permission bits do not stop root or Windows from reading files")
	}
}

func executeWithFailingRules(t *testing.T, args ...string) execution {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.ExecuteWithRegistry(context.Background(), failingRegistry(), args, &stdout, &stderr)
	return execution{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestLint_RuleFailureIsAnInternalError(t *testing.T) {
	tests := []struct {
		name    string
		content string
		cause   string
	}{
		{name: "panic", content: "panic\n", cause: "rule TEST001 failed on a.md: rule panicked: cannot lint this"},
		{name: "returned error", content: "boom\n", cause: "rule TEST002 failed on a.md: frobnicator missing"},
		{name: "invalid fix", content: "invalid\n", cause: "rule TEST003 failed on a.md: invalid fix: fix for line 1:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inProject(t, map[string]string{"a.md": tt.content})

			got := executeWithFailingRules(t, "lint", "a.md")

			assert.Equal(t, cli.ExitInternalError, got.code, "stderr: %s", got.stderr)
			assert.Contains(t, got.stderr, tt.cause)
			assert.Contains(t, got.stderr, "Error: 1 rule failure on 1 file")
			assert.NotContains(t, got.stderr, "could not be linted", "the file was read")
			assert.Contains(t, got.stdout, "1 rule failure", "the summary names the failure")
		})
	}
}

func TestLint_UnreadableFileOutranksRuleFailure(t *testing.T) {
	skipWithoutFilePermissions(t)

	dir := inProject(t, map[string]string{"a.md": "panic\n", "locked.md": "# Locked\n"})
	require.NoError(t, os.Chmod(filepath.Join(dir, "locked.md"), 0))

	got := executeWithFailingRules(t, "lint", "a.md", "locked.md")

	assert.Equal(t, cli.ExitIOError, got.code, "stderr: %s", got.stderr)
	assert.Contains(t, got.stderr, "1 file could not be linted")
	assert.Contains(t, got.stderr, "rule TEST001 failed on a.md: rule panicked: cannot lint this",
		"the rule failure is still reported")
}

func TestLint_DiffPreviewReportsRuleFailuresOfTheFixedContent(t *testing.T) {
	for _, args := range [][]string{
		{"lint", "--format", "diff", "a.md"},
		{"lint", "--fix", "--dry-run", "a.md"},
	} {
		t.Run(args[2], func(t *testing.T) {
			inProject(t, map[string]string{"a.md": "x\n"})

			got := executeWithFailingRules(t, args...)

			assert.Equal(t, cli.ExitInternalError, got.code, "stderr: %s", got.stderr)
			assert.Contains(t, got.stderr, "rule TEST005 failed on a.md: rule panicked: cannot lint fixed content")
		})
	}
}

func TestLint_RulePanicStackOnlyWithDebug(t *testing.T) {
	inProject(t, map[string]string{"a.md": "panic\n"})

	quiet := executeWithFailingRules(t, "lint", "a.md")
	assert.NotContains(t, quiet.stderr, "goroutine ", "the stack is debug output")

	verbose := executeWithFailingRules(t, "--debug", "lint", "a.md")
	assert.Contains(t, verbose.stderr, "goroutine ")
	assert.Equal(t, cli.ExitInternalError, verbose.code)
}

func TestLint_MachineReadableReportsCountRuleFailuresSeparately(t *testing.T) {
	inProject(t, map[string]string{"a.md": "panic\n"})

	jsonRun := executeWithFailingRules(t, "lint", "--format", "json", "a.md")
	require.Equal(t, cli.ExitInternalError, jsonRun.code, "stderr: %s", jsonRun.stderr)

	var report struct {
		Files []struct {
			Path       string `json:"path"`
			RuleErrors []struct {
				RuleID   string `json:"ruleId"`
				RuleName string `json:"ruleName"`
				Message  string `json:"message"`
			} `json:"ruleErrors"`
		} `json:"files"`
		Summary struct {
			FilesErrored int `json:"filesErrored"`
			RuleErrors   int `json:"ruleErrors"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonRun.stdout), &report))
	assert.Zero(t, report.Summary.FilesErrored, "filesErrored counts files that could not be linted")
	assert.Equal(t, 1, report.Summary.RuleErrors)
	require.Len(t, report.Files, 1)
	require.Len(t, report.Files[0].RuleErrors, 1)
	assert.Equal(t, "TEST001", report.Files[0].RuleErrors[0].RuleID)
	assert.Equal(t, "rule-TEST001", report.Files[0].RuleErrors[0].RuleName)
	assert.Equal(t, "rule panicked: cannot lint this", report.Files[0].RuleErrors[0].Message)

	sarifRun := executeWithFailingRules(t, "lint", "--format", "sarif", "a.md")
	require.Equal(t, cli.ExitInternalError, sarifRun.code, "stderr: %s", sarifRun.stderr)

	var sarif struct {
		Runs []struct {
			Invocations []struct {
				ExecutionSuccessful        bool `json:"executionSuccessful"`
				ToolExecutionNotifications []struct {
					Message struct {
						Text string `json:"text"`
					} `json:"message"`
				} `json:"toolExecutionNotifications"`
			} `json:"invocations"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal([]byte(sarifRun.stdout), &sarif))
	require.Len(t, sarif.Runs, 1)
	require.Len(t, sarif.Runs[0].Invocations, 1)
	invocation := sarif.Runs[0].Invocations[0]
	assert.False(t, invocation.ExecutionSuccessful)
	require.Len(t, invocation.ToolExecutionNotifications, 1)
	assert.Equal(t, "rule TEST001 failed: rule panicked: cannot lint this", invocation.ToolExecutionNotifications[0].Message.Text)
}

// successClaims are the messages of a run with nothing to report.
var successClaims = []string{"All files passed", "No issues found", "No issues would remain"}

func TestLint_EveryFormatReportsARuleFailureWithoutFindings(t *testing.T) {
	tests := []struct {
		name    string
		content string
		args    []string
	}{
		{name: "text", content: "panic\n", args: []string{"lint", "a.md"}},
		{name: "table", content: "panic\n", args: []string{"lint", "--format", "table", "a.md"}},
		{name: "table per file", content: "panic\n", args: []string{"lint", "--format", "table", "--per-file", "a.md"}},
		{name: "summary", content: "panic\n", args: []string{"lint", "--format", "summary", "a.md"}},
		{name: "diff", content: "panic\n", args: []string{"lint", "--format", "diff", "a.md"}},
		// The dry run fixes "x" to "y", on which TEST005 fails: no finding
		// remains, but the rule failed on the fixed content.
		{name: "text dry run", content: "x\n", args: []string{"lint", "--fix", "--dry-run", "a.md"}},
		{name: "table dry run", content: "x\n", args: []string{"lint", "--fix", "--dry-run", "--format", "table", "a.md"}},
		{name: "table per file dry run", content: "x\n",
			args: []string{"lint", "--fix", "--dry-run", "--format", "table", "--per-file", "a.md"}},
		{name: "summary dry run", content: "x\n", args: []string{"lint", "--fix", "--dry-run", "--format", "summary", "a.md"}},
		{name: "diff dry run", content: "x\n", args: []string{"lint", "--fix", "--dry-run", "--format", "diff", "a.md"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inProject(t, map[string]string{"a.md": tt.content})

			got := executeWithFailingRules(t, tt.args...)

			assert.Equal(t, cli.ExitInternalError, got.code, "stderr: %s", got.stderr)
			assert.Contains(t, got.stdout, "1 rule failure")
			for _, claim := range successClaims {
				assert.NotContains(t, got.stdout, claim)
			}
		})
	}
}

func TestLint_EveryFormatReportsAnUnreadableFileWithoutFindings(t *testing.T) {
	skipWithoutFilePermissions(t)

	for _, format := range []string{"text", "table", "summary", "diff"} {
		t.Run(format, func(t *testing.T) {
			dir := inProject(t, map[string]string{"locked.md": "# Locked\n"})
			require.NoError(t, os.Chmod(filepath.Join(dir, "locked.md"), 0))

			got := executeWithFailingRules(t, "lint", "--format", format, "locked.md")

			assert.Equal(t, cli.ExitIOError, got.code, "stderr: %s", got.stderr)
			assert.Contains(t, got.stdout, "could not be linted")
			for _, claim := range successClaims {
				assert.NotContains(t, got.stdout, claim)
			}
		})
	}
}
