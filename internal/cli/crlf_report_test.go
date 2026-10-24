package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// crlfFixDocument has findings for each kind of fix, all fixed in one pass
// without conflicts: a blank line inserted below the heading, trailing
// spaces deleted, two extra blank lines deleted by a fix each, an emphasis
// style rewritten with two edits, which both of its findings carry, and
// the final newline added.
const crlfFixDocument = "# Title\r\nSome text   \r\n\r\n\r\n\r\nText *one* and _two_.\r\nEnd"

// distinctFixes collects the edits of reported fixes, each fix once.
// Findings that one change fixes, such as both markers of an emphasis,
// carry the same fix, which a consumer applies once.
type distinctFixes struct {
	edits []fix.TextEdit
	seen  map[string]bool
}

// add collects the edits of one finding's fix, unless an earlier finding
// carried the same fix.
func (d *distinctFixes) add(edits []fix.TextEdit) {
	if len(edits) == 0 {
		return
	}
	key := fmt.Sprint(edits)
	if d.seen[key] {
		return
	}
	if d.seen == nil {
		d.seen = make(map[string]bool)
	}
	d.seen[key] = true
	d.edits = append(d.edits, edits...)
}

// jsonReportedEdits returns the edits of the distinct fixes that the
// diagnostics in a JSON report carry.
func jsonReportedEdits(t *testing.T, report string) []fix.TextEdit {
	t.Helper()

	var parsed struct {
		Files []struct {
			Diagnostics []struct {
				Fixes []struct {
					StartOffset int    `json:"startOffset"`
					EndOffset   int    `json:"endOffset"`
					NewText     string `json:"newText"`
				} `json:"fixes"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	var fixes distinctFixes
	for _, file := range parsed.Files {
		for _, diag := range file.Diagnostics {
			var edits []fix.TextEdit
			for _, edit := range diag.Fixes {
				edits = append(edits, fix.TextEdit{StartOffset: edit.StartOffset, EndOffset: edit.EndOffset, NewText: edit.NewText})
			}
			fixes.add(edits)
		}
	}
	return fixes.edits
}

// sarifReportedEdits returns the replacements of the distinct fixes in a
// SARIF log as byte edits of content. Columns count UTF-16 code units, and
// content is ASCII here, so a column is a byte offset into its line, line
// terminator included.
func sarifReportedEdits(t *testing.T, log string, content []byte) []fix.TextEdit {
	t.Helper()

	var parsed struct {
		Runs []struct {
			ColumnKind string `json:"columnKind"`
			Results    []struct {
				Fixes []struct {
					ArtifactChanges []struct {
						Replacements []struct {
							DeletedRegion struct {
								StartLine   int `json:"startLine"`
								StartColumn int `json:"startColumn"`
								EndLine     int `json:"endLine"`
								EndColumn   int `json:"endColumn"`
							} `json:"deletedRegion"`
							InsertedContent struct {
								Text string `json:"text"`
							} `json:"insertedContent"`
						} `json:"replacements"`
					} `json:"artifactChanges"`
				} `json:"fixes"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal([]byte(log), &parsed), log)
	require.Len(t, parsed.Runs, 1)
	assert.Equal(t, "utf16CodeUnits", parsed.Runs[0].ColumnKind)

	lines := mdast.BuildLines(content)
	offset := func(line, column int) int {
		require.Positive(t, line)
		require.LessOrEqual(t, line, len(lines))
		require.Positive(t, column)
		return lines[line-1].StartOffset + column - 1
	}

	var fixes distinctFixes
	for _, result := range parsed.Runs[0].Results {
		for _, proposed := range result.Fixes {
			var edits []fix.TextEdit
			for _, change := range proposed.ArtifactChanges {
				for _, replacement := range change.Replacements {
					region := replacement.DeletedRegion
					edits = append(edits, fix.TextEdit{
						StartOffset: offset(region.StartLine, region.StartColumn),
						EndOffset:   offset(region.EndLine, region.EndColumn),
						NewText:     replacement.InsertedContent.Text,
					})
				}
			}
			fixes.add(edits)
		}
	}
	return fixes.edits
}

// applyReported applies reported edits to content, as a consumer of the
// report would.
func applyReported(t *testing.T, content []byte, edits []fix.TextEdit) string {
	t.Helper()

	prepared, err := fix.PrepareEdits(edits, len(content))
	require.NoError(t, err)
	return string(fix.ApplyEdits(content, prepared))
}

// TestFix_JSONAndSARIFFixesReproduceTheBytesWrittenToCRLFFiles checks the
// fixes that JSON and SARIF report for a CRLF file: applied to the file,
// each reproduces exactly the bytes --fix writes, CRLF line endings
// included.
func TestFix_JSONAndSARIFFixesReproduceTheBytesWrittenToCRLFFiles(t *testing.T) {
	t.Parallel()

	original := []byte(crlfFixDocument)
	path := filepath.Join(t.TempDir(), "doc.md")
	require.NoError(t, os.WriteFile(path, original, 0o644))

	jsonEdits := jsonReportedEdits(t, runLint(t, "--format", "json", path))
	sarifEdits := sarifReportedEdits(t, runLint(t, "--format", "sarif", path), original)
	runLint(t, "--fix", path)
	written := readBytes(t, path)

	require.NotEqual(t, crlfFixDocument, string(written), "the document must have fixes")
	require.Len(t, jsonEdits, 7, "one edit for each fix, two for the emphasis style")
	assert.Zero(t, bareLineFeeds(written), "--fix keeps the CRLF line endings")

	assert.Equal(t, string(written), applyReported(t, original, jsonEdits),
		"the JSON fixes must produce exactly the bytes --fix writes")
	assert.Equal(t, string(written), applyReported(t, original, sarifEdits),
		"the SARIF fixes must produce exactly the bytes --fix writes")

	byOffset := func(a, b fix.TextEdit) int { return a.StartOffset - b.StartOffset }
	slices.SortFunc(jsonEdits, byOffset)
	slices.SortFunc(sarifEdits, byOffset)
	assert.Equal(t, jsonEdits, sarifEdits, "JSON and SARIF report the same edits")
}
