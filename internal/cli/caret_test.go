package cli_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// caretDocument has findings with known columns after text that is not one
// byte per display cell: a byte order mark, two-byte letters, wide CJK
// characters, an emoji, a combining mark and tabs.
const caretDocument = "\ufeff# Title \n" + // 1: MD009 after a byte order mark
	"\n" +
	"ÄÖÜ trail   \n" + // 3: MD009 after two-byte letters
	"\n" +
	"中文 trail   \n" + // 5: MD009 after wide characters
	"\n" +
	"a\tb   \n" + // 7: MD010 at the tab, MD009 after it
	"\n" +
	"😀 emoji   \n" + // 9: MD009 after a wide emoji
	"\n" +
	"e\u0301 trail   \n" + // 11: MD009 after a combining mark
	"\n" +
	"中\t文   \n" + // 13: MD010 after a wide character, MD009 after the tab
	"\n" +
	"中 https://example.com\n" // 15: MD034 after a wide character

// contextLines returns the source line and caret line printed under the
// finding at location, such as "doc.md:3:10", without their indent.
func contextLines(t *testing.T, output, location string) (string, string) {
	t.Helper()

	const indent = "        "
	lines := strings.Split(output, "\n")
	for idx, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), location+" ") {
			continue
		}
		require.Less(t, idx+2, len(lines), "no context after %s", location)
		source, caret := lines[idx+1], lines[idx+2]
		require.True(t, strings.HasPrefix(source, indent), "source line of %s: %q", location, source)
		require.True(t, strings.HasPrefix(caret, indent), "caret line of %s: %q", location, caret)
		return strings.TrimPrefix(source, indent), strings.TrimPrefix(caret, indent)
	}
	t.Fatalf("no finding at %s in:\n%s", location, output)
	return "", ""
}

// TestLint_CaretLinesUpUnderTheColumn checks, end to end, that the caret
// under each source line sits below the character at the reported column.
// The reported columns count characters; the caret is padded by the display
// cells of the text before the column, with tabs expanded to the next
// multiple of four.
func TestLint_CaretLinesUpUnderTheColumn(t *testing.T) {
	inProject(t, map[string]string{"doc.md": caretDocument})

	got := execute(t, "lint", "--color", "never", "doc.md")

	tests := []struct {
		location string
		source   string
		caret    string
	}{
		{location: "doc.md:1:8", source: "# Title ", caret: "       ^"},
		{location: "doc.md:3:10", source: "ÄÖÜ trail   ", caret: "         ^"},
		{location: "doc.md:5:9", source: "中文 trail   ", caret: "          ^"},
		{location: "doc.md:7:2", source: "a   b   ", caret: " ^"},
		{location: "doc.md:7:4", source: "a   b   ", caret: "     ^"},
		{location: "doc.md:9:8", source: "😀 emoji   ", caret: "        ^"},
		{location: "doc.md:11:9", source: "e\u0301 trail   ", caret: "       ^"},
		{location: "doc.md:13:2", source: "中  文   ", caret: "  ^"},
		{location: "doc.md:13:4", source: "中  文   ", caret: "      ^"},
		{location: "doc.md:15:3", source: "中 https://example.com", caret: "   ^"},
	}

	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			source, caret := contextLines(t, got.stdout, tt.location)
			assert.Equal(t, tt.source, source, "source line")
			assert.Equal(t, tt.caret, caret, "caret line")
		})
	}
}
