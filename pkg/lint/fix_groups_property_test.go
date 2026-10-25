package lint_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// randomGroupContent returns short text of letters and line endings of
// every kind.
func randomGroupContent(rng *rand.Rand) string {
	pieces := []string{"a", "b", "z", "\n", "\r\n", "\r"}
	var text strings.Builder
	for range 3 + rng.Intn(10) {
		text.WriteString(pieces[rng.Intn(len(pieces))])
	}
	return text.String()
}

// splitsCRLF reports whether offset lies between the "\r" and "\n" of
// content, where no valid edit may start or end.
func splitsCRLF(content string, offset int) bool {
	return offset > 0 && offset < len(content) && content[offset-1] == '\r' && content[offset] == '\n'
}

// randomGroupEdits returns two to four edits of content that do not
// overlap, insertions at one offset among them, with texts that hold line
// endings of every kind.
func randomGroupEdits(rng *rand.Rand, content string) []fix.TextEdit {
	texts := []string{"", "X", "\n", "\r\n", "\r", "Y\n", "\nB", "Q\r"}
	var offsets []int
	for offset := 0; offset <= len(content); offset++ {
		if !splitsCRLF(content, offset) {
			offsets = append(offsets, offset)
		}
	}
	var edits []fix.TextEdit
	pos := 0
	for range 2 + rng.Intn(3) {
		var candidates []int
		for _, offset := range offsets {
			if offset >= pos {
				candidates = append(candidates, offset)
			}
		}
		if len(candidates) == 0 {
			break
		}
		start := candidates[rng.Intn(len(candidates))]
		end := start
		if rng.Intn(2) == 0 {
			var ends []int
			for _, offset := range candidates {
				if offset >= start {
					ends = append(ends, offset)
				}
			}
			end = ends[rng.Intn(len(ends))]
		}
		edit := fix.TextEdit{StartOffset: start, EndOffset: end, NewText: texts[rng.Intn(len(texts))]}
		if edit.StartOffset == edit.EndOffset && edit.NewText == "" {
			edit.NewText = "X"
		}
		edits = append(edits, edit)
		pos = end
	}
	return edits
}

// hasDuplicateEdit reports whether two of edits are the same.
func hasDuplicateEdit(edits []fix.TextEdit) bool {
	seen := make(map[fix.TextEdit]bool)
	for _, edit := range edits {
		if seen[edit] {
			return true
		}
		seen[edit] = true
	}
	return false
}

// TestEngine_FixGroupsReportWhatTheyApply checks, on random groups of
// edits in text with mixed line endings, that the engine fixes a group
// exactly when it fixes the one edit the group makes, offered as a single
// fix, that it applies that same edit, and that every edit the group's
// members report, an identical one once, gives what it applies.
func TestEngine_FixGroupsReportWhatTheyApply(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(88))
	checked, fixed := 0, 0
	for range 4000 {
		content := randomGroupContent(rng)
		edits := randomGroupEdits(rng, content)
		// Members that carry the same edit share it, which the group makes
		// once, so the edits differ here.
		if len(edits) < 2 || hasDuplicateEdit(edits) {
			continue
		}
		combined, ok := fix.CombineEdits([]byte(content), edits)
		if !ok {
			continue
		}
		var members []lint.Diagnostic
		for _, edit := range edits {
			members = append(members, groupMember("TEST001", 1, 1, edit))
		}
		grouped := lintMarkdown(t, content, groupRule("TEST001", members...))
		single := lintMarkdown(t, content, groupRule("TEST001", fixDiag("TEST001", combined)))
		checked++

		require.Equal(t, single.AcceptedFixes > 0, grouped.AcceptedFixes > 0,
			"content %q edits %+v: the group is fixed exactly when its one edit is", content, edits)
		require.Equal(t, single.Edits, grouped.Edits, "content %q edits %+v", content, edits)
		if grouped.AcceptedFixes == 0 {
			continue
		}
		fixed++
		require.Equal(t, len(edits), grouped.AcceptedFixes)
		applied := string(fix.ApplyEdits([]byte(content), grouped.Edits))
		require.Equal(t, applied, applyReported(t, content, grouped),
			"content %q edits %+v: the reported edits give what --fix writes", content, edits)
	}
	t.Logf("checked %d groups, %d fixed", checked, fixed)
	require.Greater(t, fixed, 1000)
}
