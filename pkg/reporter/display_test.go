package reporter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	goldmarkparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// contextIndent is the indentation the text reporter puts before source
// context lines and their caret.
const contextIndent = "        "

// umlautContent is the MD009 repro from the review: the trailing spaces start
// at byte column 13 but character column 10.
const umlautContent = "# Title\n\nÄÖÜ trail   \n"

// umlautResult returns a result for umlautContent at path with the diagnostic
// MD009 produces for it, using byte columns and byte offsets as rules do.
func umlautResult(path string) *runner.Result {
	return &runner.Result{
		Files: []runner.FileOutcome{{
			Path: path,
			Result: &lint.PipelineResult{
				Path: path,
				FileResult: &lint.FileResult{
					Snapshot: mdast.NewFileSnapshot(path, []byte(umlautContent)),
					Diagnostics: []lint.Diagnostic{{
						RuleID:      "MD009",
						RuleName:    "no-trailing-spaces",
						Message:     "Trailing whitespace",
						Severity:    config.SeverityWarning,
						FilePath:    path,
						StartLine:   3,
						StartColumn: 13,
						EndLine:     3,
						EndColumn:   16,
						Suggestion:  "Remove trailing whitespace",
						FixEdits:    []fix.TextEdit{{StartOffset: 21, EndOffset: 24, NewText: ""}},
					}},
				},
			},
		}},
		Stats: runner.Stats{
			FilesDiscovered:       1,
			FilesProcessed:        1,
			FilesWithIssues:       1,
			DiagnosticsTotal:      1,
			DiagnosticsFixable:    1,
			DiagnosticsBySeverity: map[string]int{"warning": 1},
		},
	}
}

// caretLine is the context caret the text reporter prints for a character column.
func caretLine(column int) string {
	return contextIndent + strings.Repeat(" ", column-1) + "^"
}

func TestTextReporter_CharacterColumnsAndRelativePath(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	path := filepath.Join(workDir, "docs", "umlaut.md")

	var buf bytes.Buffer
	rep := reporter.NewTextReporter(reporter.Options{
		Writer:      &buf,
		Color:       "never",
		ShowContext: true,
		GroupByFile: true,
		WorkingDir:  workDir,
	})

	_, err := rep.Report(context.Background(), umlautResult(path))
	require.NoError(t, err)

	output := buf.String()
	shown := filepath.Join("docs", "umlaut.md")
	assert.Contains(t, output, shown+" (1 issues)", "file header uses the relative path")
	assert.Contains(t, output, shown+":3:10 ", "location uses the relative path and character column")
	assert.NotContains(t, output, workDir, "no absolute paths for files inside the working directory")
	assert.Contains(t, output, contextIndent+"ÄÖÜ trail   \n"+caretLine(10)+"\n",
		"caret sits under the first trailing space")
}

func TestTextReporter_FlatModeUsesDisplayPaths(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	path := filepath.Join(workDir, "umlaut.md")

	var buf bytes.Buffer
	rep := reporter.NewTextReporter(reporter.Options{
		Writer:      &buf,
		Color:       "never",
		GroupByFile: false,
		WorkingDir:  workDir,
	})

	_, err := rep.Report(context.Background(), umlautResult(path))
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "  umlaut.md:3:10 ")
	assert.NotContains(t, buf.String(), workDir)
}

func TestTextReporter_ContextLineOmitsByteOrderMark(t *testing.T) {
	t.Parallel()

	// '#' follows a three-byte byte order mark; the trailing spaces start at
	// byte column 12 but character column 8.
	content := "\xef\xbb\xbf# Tïtle   \n"
	path := "bom.md"

	result := &runner.Result{
		Files: []runner.FileOutcome{{
			Path: path,
			Result: &lint.PipelineResult{
				FileResult: &lint.FileResult{
					Snapshot: mdast.NewFileSnapshot(path, []byte(content)),
					Diagnostics: []lint.Diagnostic{{
						RuleID:      "MD009",
						RuleName:    "no-trailing-spaces",
						Message:     "Trailing whitespace",
						Severity:    config.SeverityWarning,
						FilePath:    path,
						StartLine:   1,
						StartColumn: 12,
						EndLine:     1,
						EndColumn:   15,
					}},
				},
			},
		}},
	}

	var buf bytes.Buffer
	rep := reporter.NewTextReporter(reporter.Options{
		Writer:      &buf,
		Color:       "never",
		ShowContext: true,
		GroupByFile: true,
	})

	_, err := rep.Report(context.Background(), result)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "bom.md:1:8 ")
	assert.Contains(t, output, contextIndent+"# Tïtle   \n"+caretLine(8)+"\n")
	assert.NotContains(t, output, "\xef\xbb\xbf", "byte order mark is not printed")
}

func TestTextReporter_FileErrorUsesDisplayPath(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	path := filepath.Join(workDir, "docs", "locked.md")

	result := &runner.Result{
		Files: []runner.FileOutcome{{Path: path, Error: errors.New("permission denied")}},
	}

	for _, grouped := range []bool{true, false} {
		var buf bytes.Buffer
		rep := reporter.NewTextReporter(reporter.Options{
			Writer:      &buf,
			Color:       "never",
			GroupByFile: grouped,
			WorkingDir:  workDir,
		})

		_, err := rep.Report(context.Background(), result)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), filepath.Join("docs", "locked.md")+": error: permission denied")
		assert.NotContains(t, buf.String(), workDir)
	}
}

func TestJSONReporter_CharacterColumnsAndRelativePath(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	path := filepath.Join(workDir, "docs", "umlaut.md")

	var buf bytes.Buffer
	rep := reporter.NewJSONReporter(reporter.Options{Writer: &buf, WorkingDir: workDir})

	_, err := rep.Report(context.Background(), umlautResult(path))
	require.NoError(t, err)

	var output reporter.JSONOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))
	require.Len(t, output.Files, 1)
	assert.Equal(t, filepath.Join("docs", "umlaut.md"), output.Files[0].Path)

	require.Len(t, output.Files[0].Diagnostics, 1)
	diag := output.Files[0].Diagnostics[0]
	assert.Equal(t, 3, diag.StartLine)
	assert.Equal(t, 10, diag.StartColumn)
	assert.Equal(t, 3, diag.EndLine)
	assert.Equal(t, 13, diag.EndColumn)

	require.Len(t, diag.Fixes, 1)
	assert.Equal(t, 21, diag.Fixes[0].StartOffset, "fix offsets stay byte offsets")
	assert.Equal(t, 24, diag.Fixes[0].EndOffset)
}

func TestJSONReporter_OutsideWorkingDirStaysAbsolute(t *testing.T) {
	t.Parallel()

	outside := filepath.Join(t.TempDir(), "elsewhere.md")

	var buf bytes.Buffer
	rep := reporter.NewJSONReporter(reporter.Options{Writer: &buf, WorkingDir: t.TempDir()})

	_, err := rep.Report(context.Background(), umlautResult(outside))
	require.NoError(t, err)

	var output reporter.JSONOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))
	require.Len(t, output.Files, 1)
	assert.Equal(t, outside, output.Files[0].Path)
}

func TestTableReporter_CharacterColumnsAndRelativePath(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	path := filepath.Join(workDir, "docs", "umlaut.md")

	for _, perFile := range []bool{false, true} {
		var buf bytes.Buffer
		rep := reporter.NewTableReporter(reporter.Options{
			Writer:      &buf,
			Color:       "never",
			ShowSummary: true,
			PerFile:     perFile,
			WorkingDir:  workDir,
		})

		_, err := rep.Report(context.Background(), umlautResult(path))
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, filepath.Join("docs", "umlaut.md"), "perFile=%v", perFile)
		assert.Contains(t, output, " 3:10 ", "perFile=%v", perFile)
		assert.NotContains(t, output, workDir, "perFile=%v", perFile)
	}
}

func TestDiffReporter_DirectoryNamedWithDotsIsInsideWorkingDir(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	path := filepath.Join(workDir, "..notes", "file.md")

	diff := fix.GenerateDiff(path, []byte("old\n"), []byte("new\n"))
	require.NotNil(t, diff)

	var buf bytes.Buffer
	rep := reporter.NewDiffReporter(reporter.Options{Writer: &buf, Color: "never", WorkingDir: workDir})

	_, err := rep.Report(context.Background(), &runner.Result{
		Files: []runner.FileOutcome{{
			Path:   path,
			Result: &lint.PipelineResult{FileResult: &lint.FileResult{}, Diff: diff},
		}},
	})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "diff --git a/..notes/file.md b/..notes/file.md")
}

func TestReporters_EngineDiagnosticsUseCharacterColumns(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	path := filepath.Join(workDir, "umlaut.md")
	require.NoError(t, os.WriteFile(path, []byte(umlautContent), 0o644))

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	pipeline := lint.NewPipeline(lint.NewEngine(goldmarkparser.New(string(config.FlavorCommonMark)), registry))

	processed, err := pipeline.ProcessFile(context.Background(), path, config.NewConfig(), lint.DefaultPipelineOptions())
	require.NoError(t, err)

	result := &runner.Result{Files: []runner.FileOutcome{{Path: path, Result: processed}}}

	var buf bytes.Buffer
	rep := reporter.NewJSONReporter(reporter.Options{Writer: &buf, WorkingDir: workDir})
	_, err = rep.Report(context.Background(), result)
	require.NoError(t, err)

	var output reporter.JSONOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &output))
	require.Len(t, output.Files, 1)
	assert.Equal(t, "umlaut.md", output.Files[0].Path)

	var found bool
	for _, diag := range output.Files[0].Diagnostics {
		if diag.RuleID != "MD009" {
			continue
		}
		found = true
		assert.Equal(t, 3, diag.StartLine)
		assert.Equal(t, 10, diag.StartColumn, "markdownlint reports this at character column 10")
	}
	assert.True(t, found, "MD009 reports the trailing spaces")
}
