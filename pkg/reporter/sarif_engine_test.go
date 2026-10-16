package reporter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	goldmarkparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// engineFixtures are files whose findings carry fixes next to text where byte,
// character and UTF-16 positions differ.
var engineFixtures = map[string]string{
	"umlaut.md":   umlautContent,
	"emoji.md":    "# Title\n\n\U0001F600 trailing   \nSecond\n## Next\nBody\n",
	"bom.md":      "\xef\xbb\xbf# Tïtle   \n\nText   \n",
	"crlf.md":     "# Title\r\n\r\nÄÖÜ text   \r\n* item\r\n",
	"noeol.md":    "# Title\n\nNo final newline",
	"trailing.md": "# Title\n\nText\n\n\n\n",
	"lists.md":    "# Title\n\n* one\n+ two\n- three\n",
}

// sarifByteOffset maps a SARIF line and UTF-16 column to a byte offset of
// content using SARIF's text model, independently of the reporter: a line
// ends after its newline, columns start at 1, and a leading byte order mark
// takes no column.
func sarifByteOffset(t *testing.T, content []byte, line, column int) int {
	t.Helper()

	start := 0
	for current := 1; current < line; current++ {
		newline := bytes.IndexByte(content[start:], '\n')
		require.GreaterOrEqual(t, newline, 0, "line %d is past the end of the file", line)
		start += newline + 1
	}
	if line == 1 && bytes.HasPrefix(content, []byte("\xef\xbb\xbf")) {
		start += len("\xef\xbb\xbf")
	}

	offset := start
	for units := 1; units < column; {
		require.Less(t, offset, len(content), "column %d is past the end of line %d", column, line)
		char, size := utf8.DecodeRune(content[offset:])
		units++
		if char > 0xFFFF {
			units++ // a surrogate pair takes a second UTF-16 code unit
		}
		offset += size
	}

	return offset
}

// applySARIFFix applies the replacements of a fix to content using only the
// SARIF regions.
func applySARIFFix(t *testing.T, content []byte, change reporter.SARIFArtifactChange) []byte {
	t.Helper()

	edits := make([]fix.TextEdit, 0, len(change.Replacements))
	for _, replacement := range change.Replacements {
		region := replacement.DeletedRegion
		edit := fix.TextEdit{
			StartOffset: sarifByteOffset(t, content, region.StartLine, region.StartColumn),
			EndOffset:   sarifByteOffset(t, content, region.EndLine, region.EndColumn),
		}
		if replacement.InsertedContent != nil {
			edit.NewText = replacement.InsertedContent.Text
		}
		edits = append(edits, edit)
	}

	return fix.ApplyEdits(content, sortedEdits(edits))
}

func sortedEdits(edits []fix.TextEdit) []fix.TextEdit {
	sorted := slices.Clone(edits)
	slices.SortStableFunc(sorted, func(left, right fix.TextEdit) int {
		return left.StartOffset - right.StartOffset
	})
	return sorted
}

func TestSARIFReporter_EngineFixesRoundTrip(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	cfg := config.NewConfig()
	pipeline := lint.NewPipeline(lint.NewEngine(goldmarkparser.New(string(cfg.Flavor)), registry))

	names := make([]string, 0, len(engineFixtures))
	for name := range engineFixtures {
		names = append(names, name)
	}
	slices.Sort(names)

	result := &runner.Result{}
	for _, name := range names {
		path := filepath.Join(root, name)
		require.NoError(t, os.WriteFile(path, []byte(engineFixtures[name]), 0o644))

		processed, err := pipeline.ProcessFile(context.Background(), path, cfg, lint.DefaultPipelineOptions())
		require.NoError(t, err)
		result.Files = append(result.Files, runner.FileOutcome{Path: path, Result: processed})
	}

	var log reporter.SARIFOutput
	require.NoError(t, json.Unmarshal(reportSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result), &log))
	sarifResults := log.Runs[0].Results

	checked := make(map[string]int, len(engineFixtures))
	bomFirstLine := false
	next := 0
	for _, file := range result.Files {
		name := filepath.Base(file.Path)
		content := []byte(engineFixtures[name])
		for _, diag := range file.Result.Diagnostics {
			require.Less(t, next, len(sarifResults))
			sarifResult := sarifResults[next]
			next++
			require.Equal(t, diag.RuleID, sarifResult.RuleID)

			if len(sarifResult.Fixes) == 0 {
				continue
			}
			require.Len(t, sarifResult.Fixes, 1)
			require.Len(t, sarifResult.Fixes[0].ArtifactChanges, 1)

			want := fix.ApplyEdits(content, sortedEdits(diag.FixEdits))
			got := applySARIFFix(t, content, sarifResult.Fixes[0].ArtifactChanges[0])
			assert.Equal(t, string(want), string(got), "%s %s at %d:%d", name, diag.RuleID, diag.StartLine, diag.StartColumn)
			checked[name]++
			if name == "bom.md" && diag.StartLine == 1 {
				bomFirstLine = true
			}
		}
	}

	require.Equal(t, len(sarifResults), next, "one SARIF result per diagnostic")
	for _, name := range names {
		assert.Positive(t, checked[name], "%s has at least one fix to check", name)
	}
	assert.True(t, bomFirstLine, "a fix on the line that starts with the byte order mark is checked")
}
