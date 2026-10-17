package mdast_test

import (
	"testing"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func TestNode_ContentRange(t *testing.T) {
	t.Parallel()

	content := []byte("# Title\n")
	file := mdast.NewFileSnapshot("test.md", content)

	heading := mdast.NewNode(mdast.NodeHeading)
	heading.File = file
	heading.SourceSpan = &mdast.SourceRange{StartOffset: 0, EndOffset: 7}
	heading.ContentSpan = &mdast.SourceRange{StartOffset: 2, EndOffset: 7}

	if got := string(heading.Text()); got != "# Title" {
		t.Errorf("Text() = %q, want %q", got, "# Title")
	}
	if got := heading.ContentRange(); got != (mdast.SourceRange{StartOffset: 2, EndOffset: 7}) {
		t.Errorf("ContentRange() = %+v, want [2,7)", got)
	}
	if got := string(heading.ContentText()); got != "Title" {
		t.Errorf("ContentText() = %q, want %q", got, "Title")
	}
	pos := heading.ContentPosition()
	if pos.StartLine != 1 || pos.StartColumn != 3 || pos.EndLine != 1 || pos.EndColumn != 8 {
		t.Errorf("ContentPosition() = %+v, want 1:3-1:8", pos)
	}
}

func TestNode_ContentRangeDefaultsToSourceRange(t *testing.T) {
	t.Parallel()

	content := []byte("***\n")
	file := mdast.NewFileSnapshot("test.md", content)

	rule := mdast.NewNode(mdast.NodeThematicBreak)
	rule.File = file
	rule.SourceSpan = &mdast.SourceRange{StartOffset: 0, EndOffset: 3}

	if got := rule.ContentRange(); got != rule.SourceRange() {
		t.Errorf("ContentRange() = %+v, want the source range %+v", got, rule.SourceRange())
	}
	if got := string(rule.ContentText()); got != "***" {
		t.Errorf("ContentText() = %q, want %q", got, "***")
	}
}

func TestNode_SourcePositionOfEmptySpan(t *testing.T) {
	t.Parallel()

	content := []byte("-\n")
	file := mdast.NewFileSnapshot("test.md", content)

	item := mdast.NewNode(mdast.NodeListItem)
	item.File = file
	item.SourceSpan = &mdast.SourceRange{StartOffset: 0, EndOffset: 1}
	item.ContentSpan = &mdast.SourceRange{StartOffset: 1, EndOffset: 1}

	pos := item.ContentPosition()
	if pos.StartLine != 1 || pos.StartColumn != 2 || pos.EndLine != 1 || pos.EndColumn != 2 {
		t.Errorf("ContentPosition() = %+v, want 1:2-1:2", pos)
	}
	if got := item.ContentText(); len(got) != 0 {
		t.Errorf("ContentText() = %q, want empty", got)
	}
}
