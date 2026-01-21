package fix_test

import (
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/fix"
)

func FuzzGenerateDiff(f *testing.F) {
	// Add seed corpus.
	f.Add([]byte(""), []byte(""))
	f.Add([]byte("hello"), []byte("hello"))
	f.Add([]byte("hello"), []byte("world"))
	f.Add([]byte("hello\n"), []byte("hello\n"))
	f.Add([]byte("a\nb\nc\n"), []byte("a\nx\nc\n"))
	f.Add([]byte("line1\nline2\n"), []byte("line1\nline2\nline3\n"))
	f.Add([]byte("line1\nline2\nline3\n"), []byte("line1\nline3\n"))
	f.Add([]byte("a\nb\nc\nd\ne\n"), []byte("a\nB\nc\nD\ne\n"))

	f.Fuzz(func(t *testing.T, original, modified []byte) {
		// GenerateDiff should not panic.
		diff := fix.GenerateDiff("test.md", original, modified)

		// If diff is nil, content should be considered equivalent.
		if diff == nil {
			return
		}

		// Diff should have valid structure.
		if diff.Path != "test.md" {
			t.Errorf("Path = %q, want test.md", diff.Path)
		}

		// String() should not panic.
		_ = diff.String()

		// HasChanges should be consistent.
		if !diff.HasChanges() && len(diff.Hunks) > 0 {
			t.Error("HasChanges() inconsistent with Hunks")
		}

		// Verify hunk structure.
		for hunkIdx, hunk := range diff.Hunks {
			if hunk.OriginalStart < 1 {
				t.Errorf("hunk %d: OriginalStart = %d, want >= 1", hunkIdx, hunk.OriginalStart)
			}
			if hunk.ModifiedStart < 1 {
				t.Errorf("hunk %d: ModifiedStart = %d, want >= 1", hunkIdx, hunk.ModifiedStart)
			}
			if hunk.OriginalCount < 0 {
				t.Errorf("hunk %d: OriginalCount = %d, want >= 0", hunkIdx, hunk.OriginalCount)
			}
			if hunk.ModifiedCount < 0 {
				t.Errorf("hunk %d: ModifiedCount = %d, want >= 0", hunkIdx, hunk.ModifiedCount)
			}

			// Count line types.
			var ctxCount, addCount, remCount int
			for _, line := range hunk.Lines {
				switch line.Kind {
				case fix.DiffLineContext:
					ctxCount++
				case fix.DiffLineAdd:
					addCount++
				case fix.DiffLineRemove:
					remCount++
				}
			}

			// Counts should be consistent.
			if ctxCount+remCount != hunk.OriginalCount {
				t.Errorf("hunk %d: context(%d) + remove(%d) != OriginalCount(%d)",
					hunkIdx, ctxCount, remCount, hunk.OriginalCount)
			}
			if ctxCount+addCount != hunk.ModifiedCount {
				t.Errorf("hunk %d: context(%d) + add(%d) != ModifiedCount(%d)",
					hunkIdx, ctxCount, addCount, hunk.ModifiedCount)
			}
		}
	})
}

func FuzzApplyEdits(f *testing.F) {
	// Add seed corpus.
	f.Add([]byte("hello"), 0, 5, "world")
	f.Add([]byte("hello world"), 5, 5, " beautiful")
	f.Add([]byte("abcdef"), 0, 0, "prefix")
	f.Add([]byte("abcdef"), 6, 6, "suffix")
	f.Add([]byte("abcdef"), 2, 4, "")

	f.Fuzz(func(t *testing.T, content []byte, start, end int, newText string) {
		// Validate edit range.
		if start < 0 || end < start || end > len(content) {
			return // Invalid edit, skip.
		}

		edits := []fix.TextEdit{
			{StartOffset: start, EndOffset: end, NewText: newText},
		}

		// ApplyEdits should not panic.
		result := fix.ApplyEdits(content, edits)

		// Result should have expected length.
		expectedLen := len(content) - (end - start) + len(newText)
		if len(result) != expectedLen {
			t.Errorf("result length = %d, want %d", len(result), expectedLen)
		}

		// Verify content before edit is preserved.
		for i := range start {
			if result[i] != content[i] {
				t.Errorf("byte %d modified before edit: got %d, want %d", i, result[i], content[i])
				break
			}
		}

		// Verify new text is inserted.
		for i := range len(newText) {
			if result[start+i] != newText[i] {
				t.Errorf("new text byte %d wrong: got %d, want %d", i, result[start+i], newText[i])
				break
			}
		}

		// Verify content after edit is preserved.
		afterEditStart := start + len(newText)
		for i := end; i < len(content); i++ {
			resultIdx := afterEditStart + (i - end)
			if result[resultIdx] != content[i] {
				t.Errorf("byte %d modified after edit: got %d, want %d", i, result[resultIdx], content[i])
				break
			}
		}
	})
}
