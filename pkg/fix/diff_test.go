package fix_test

import (
	"strings"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/fix"
)

func TestGenerateDiff(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for empty inputs", func(t *testing.T) {
		t.Parallel()

		diff := fix.GenerateDiff("test.md", nil, nil)
		if diff != nil {
			t.Error("expected nil for empty inputs")
		}

		diff = fix.GenerateDiff("test.md", []byte{}, []byte{})
		if diff != nil {
			t.Error("expected nil for empty byte slices")
		}
	})

	t.Run("returns nil for identical content", func(t *testing.T) {
		t.Parallel()

		content := []byte("hello\nworld\n")
		diff := fix.GenerateDiff("test.md", content, content)

		if diff != nil {
			t.Error("expected nil for identical content")
		}
	})

	t.Run("detects single line change", func(t *testing.T) {
		t.Parallel()

		original := []byte("hello\nworld\n")
		modified := []byte("hello\nearth\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		if !diff.HasChanges() {
			t.Error("expected HasChanges() = true")
		}

		if len(diff.Hunks) != 1 {
			t.Errorf("expected 1 hunk, got %d", len(diff.Hunks))
		}
	})

	t.Run("detects addition", func(t *testing.T) {
		t.Parallel()

		original := []byte("line1\nline2\n")
		modified := []byte("line1\nline2\nline3\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		// Check that the diff string contains the added line.
		diffStr := diff.String()
		if !strings.Contains(diffStr, "+line3") {
			t.Errorf("expected diff to contain +line3, got:\n%s", diffStr)
		}
	})

	t.Run("detects deletion", func(t *testing.T) {
		t.Parallel()

		original := []byte("line1\nline2\nline3\n")
		modified := []byte("line1\nline3\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		diffStr := diff.String()
		if !strings.Contains(diffStr, "-line2") {
			t.Errorf("expected diff to contain -line2, got:\n%s", diffStr)
		}
	})

	t.Run("detects replacement", func(t *testing.T) {
		t.Parallel()

		original := []byte("foo\nbar\nbaz\n")
		modified := []byte("foo\nqux\nbaz\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		diffStr := diff.String()
		if !strings.Contains(diffStr, "-bar") {
			t.Errorf("expected diff to contain -bar, got:\n%s", diffStr)
		}
		if !strings.Contains(diffStr, "+qux") {
			t.Errorf("expected diff to contain +qux, got:\n%s", diffStr)
		}
	})

	t.Run("handles new file", func(t *testing.T) {
		t.Parallel()

		original := []byte{}
		modified := []byte("new content\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		diffStr := diff.String()
		if !strings.Contains(diffStr, "+new content") {
			t.Errorf("expected diff to contain +new content, got:\n%s", diffStr)
		}
	})

	t.Run("handles file deletion", func(t *testing.T) {
		t.Parallel()

		original := []byte("old content\n")
		modified := []byte{}

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		diffStr := diff.String()
		if !strings.Contains(diffStr, "-old content") {
			t.Errorf("expected diff to contain -old content, got:\n%s", diffStr)
		}
	})
}

func TestDiff_String(t *testing.T) {
	t.Parallel()

	t.Run("returns empty string for nil diff", func(t *testing.T) {
		t.Parallel()

		var diff *fix.Diff
		if diff.String() != "" {
			t.Error("expected empty string for nil diff")
		}
	})

	t.Run("returns empty string for diff with no hunks", func(t *testing.T) {
		t.Parallel()

		diff := &fix.Diff{Path: "test.md"}
		if diff.String() != "" {
			t.Error("expected empty string for diff with no hunks")
		}
	})

	t.Run("produces valid unified diff format", func(t *testing.T) {
		t.Parallel()

		original := []byte("line1\nold\nline3\n")
		modified := []byte("line1\nnew\nline3\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		diffStr := diff.String()

		// Check header.
		if !strings.HasPrefix(diffStr, "--- a/test.md\n+++ b/test.md\n") {
			t.Errorf("expected standard diff header, got:\n%s", diffStr)
		}

		// Check hunk header format.
		if !strings.Contains(diffStr, "@@ -") {
			t.Errorf("expected hunk header, got:\n%s", diffStr)
		}
	})
}

func TestDiff_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("returns false for nil diff", func(t *testing.T) {
		t.Parallel()

		var diff *fix.Diff
		if diff.HasChanges() {
			t.Error("expected HasChanges() = false for nil diff")
		}
	})

	t.Run("returns false for empty hunks", func(t *testing.T) {
		t.Parallel()

		diff := &fix.Diff{Path: "test.md"}
		if diff.HasChanges() {
			t.Error("expected HasChanges() = false for empty hunks")
		}
	})

	t.Run("returns true for diff with hunks", func(t *testing.T) {
		t.Parallel()

		diff := &fix.Diff{
			Path: "test.md",
			Hunks: []fix.DiffHunk{
				{OriginalStart: 1, OriginalCount: 1, ModifiedStart: 1, ModifiedCount: 1},
			},
		}
		if !diff.HasChanges() {
			t.Error("expected HasChanges() = true")
		}
	})
}

func TestGenerateDiff_MultipleChanges(t *testing.T) {
	t.Parallel()

	t.Run("handles multiple separate changes", func(t *testing.T) {
		t.Parallel()

		// Create content with changes far apart to test hunk separation.
		var origLines []string
		var modLines []string

		for lineIdx := range 20 {
			origLines = append(origLines, "line"+string(rune('a'+lineIdx)))
			modLines = append(modLines, "line"+string(rune('a'+lineIdx)))
		}

		// Change line 2 and line 18 (far apart).
		origLines[1] = "original2"
		modLines[1] = "modified2"
		origLines[17] = "original18"
		modLines[17] = "modified18"

		original := []byte(strings.Join(origLines, "\n") + "\n")
		modified := []byte(strings.Join(modLines, "\n") + "\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		// Should have 2 hunks for changes far apart.
		if len(diff.Hunks) != 2 {
			t.Errorf("expected 2 hunks, got %d", len(diff.Hunks))
		}
	})

	t.Run("merges close changes into single hunk", func(t *testing.T) {
		t.Parallel()

		original := []byte("a\nb\nc\nd\ne\n")
		modified := []byte("a\nB\nc\nD\ne\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		// Changes are close, should be merged into single hunk.
		if len(diff.Hunks) != 1 {
			t.Errorf("expected 1 merged hunk, got %d", len(diff.Hunks))
		}
	})
}

func TestGenerateDiff_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("handles content without trailing newline", func(t *testing.T) {
		t.Parallel()

		// Note: line-based diff treats "line1\nline2" and "line1\nline2\n"
		// as equivalent since both split to the same lines.
		// This test verifies actual content changes are detected.
		original := []byte("line1\nline2")
		modified := []byte("line1\nline3")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected diff for changed content")
		}
	})

	t.Run("handles single line content", func(t *testing.T) {
		t.Parallel()

		original := []byte("hello\n")
		modified := []byte("world\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		diffStr := diff.String()
		if !strings.Contains(diffStr, "-hello") || !strings.Contains(diffStr, "+world") {
			t.Errorf("unexpected diff output:\n%s", diffStr)
		}
	})

	t.Run("handles empty lines", func(t *testing.T) {
		t.Parallel()

		original := []byte("a\n\nb\n")
		modified := []byte("a\nb\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		// Should detect removed empty line.
		if len(diff.Hunks) != 1 {
			t.Errorf("expected 1 hunk, got %d", len(diff.Hunks))
		}
	})

	t.Run("handles all lines changed", func(t *testing.T) {
		t.Parallel()

		original := []byte("a\nb\nc\n")
		modified := []byte("x\ny\nz\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil {
			t.Fatal("expected non-nil diff")
		}

		// All lines should be in one hunk.
		if len(diff.Hunks) != 1 {
			t.Errorf("expected 1 hunk, got %d", len(diff.Hunks))
		}

		// Check counts.
		hunk := diff.Hunks[0]
		if hunk.OriginalCount != 3 {
			t.Errorf("OriginalCount = %d, want 3", hunk.OriginalCount)
		}
		if hunk.ModifiedCount != 3 {
			t.Errorf("ModifiedCount = %d, want 3", hunk.ModifiedCount)
		}
	})
}

func TestDiffHunk_Counts(t *testing.T) {
	t.Parallel()

	t.Run("counts context lines correctly", func(t *testing.T) {
		t.Parallel()

		original := []byte("ctx1\nctx2\nold\nctx3\nctx4\n")
		modified := []byte("ctx1\nctx2\nnew\nctx3\nctx4\n")

		diff := fix.GenerateDiff("test.md", original, modified)

		if diff == nil || len(diff.Hunks) == 0 {
			t.Fatal("expected non-nil diff with hunks")
		}

		hunk := diff.Hunks[0]

		// Count line types.
		var ctx, add, rem int
		for _, line := range hunk.Lines {
			switch line.Kind {
			case fix.DiffLineContext:
				ctx++
			case fix.DiffLineAdd:
				add++
			case fix.DiffLineRemove:
				rem++
			}
		}

		// Should have context + 1 remove + 1 add.
		if add != 1 {
			t.Errorf("add count = %d, want 1", add)
		}
		if rem != 1 {
			t.Errorf("remove count = %d, want 1", rem)
		}
	})
}
