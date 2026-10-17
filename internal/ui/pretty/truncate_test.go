package pretty_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/clipperhouse/displaywidth"
	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
)

// mixedTexts mixes ASCII with characters that take two cells (CJK, emoji,
// flags), with grapheme clusters of several code points (combining marks,
// emoji joined by zero-width joiners) and with two-byte letters, so a cut
// that counts bytes or code points instead of cells and grapheme clusters
// splits a character or misjudges a width.
func mixedTexts() []string {
	return []string{
		"short",
		"plain ASCII text that is longer than most columns",
		"docs/开发使用指南.md",
		"/srv/文档/résumé.md",
		"Unused reference definition [äöüäöüäöüäöüäöüäöüäöüäöü]",
		"été café résumé naïve",
		"family 👨‍👩‍👧 and 👨‍👩‍👧‍👦 again",
		"flags 🇩🇪🇫🇷🇯🇵🇰🇷 and more",
		"中文 mixed ascii 和 emoji 😀 with ä ö ü",
	}
}

func TestTruncateEnd(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "short", pretty.TruncateEnd("short", 10, "..."), "text that fits is unchanged")
	assert.Equal(t, "abcde...", pretty.TruncateEnd("abcdefghij", 8, "..."))
	assert.Equal(t, "开发...", pretty.TruncateEnd("开发使用指南", 7, "..."), "wide characters take two cells")
	assert.Equal(t, "开发...", pretty.TruncateEnd("开发使用指南", 8, "..."), "a wide character that does not fit is dropped whole")
	assert.Equal(t, "é...", pretty.TruncateEnd("ééééé", 4, "..."), "a combining mark stays with its letter")
	assert.Equal(t, "👨‍👩‍👧...", pretty.TruncateEnd(strings.Repeat("👨‍👩‍👧", 3), 5, "..."),
		"an emoji sequence is one character")
	assert.Equal(t, "ab", pretty.TruncateEnd("abcdef", 2, "..."), "no marker when it does not fit")
	assert.Empty(t, pretty.TruncateEnd("abcdef", 0, "..."))

	for _, text := range mixedTexts() {
		for width := range 40 {
			got := pretty.TruncateEnd(text, width, "...")
			assert.LessOrEqual(t, displaywidth.String(got), width, "%q at width %d gave %q", text, width, got)
			assert.True(t, utf8.ValidString(got), "%q at width %d gave %q", text, width, got)
			assert.True(t, strings.HasPrefix(text, strings.TrimSuffix(got, "...")),
				"%q at width %d gave %q, which is not its start", text, width, got)
		}
	}
}

func TestTruncateStart(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "short.md", pretty.TruncateStart("short.md", 10, "…"), "text that fits is unchanged")
	assert.Equal(t, "…cdef.md", pretty.TruncateStart("abcdef.md", 8, "…"))
	assert.Equal(t, "…文档.md", pretty.TruncateStart("长长文档.md", 8, "…"), "wide characters take two cells")
	assert.Equal(t, "…档.md", pretty.TruncateStart("长长文档.md", 7, "…"), "a wide character that does not fit is dropped whole")
	assert.Equal(t, "...使用指南.md", pretty.TruncateStart("docs/开发使用指南.md", 15, "..."))
	assert.Equal(t, "md", pretty.TruncateStart("doc.md", 2, "..."), "no marker when it does not fit")
	assert.Empty(t, pretty.TruncateStart("doc.md", 0, "..."))

	for _, text := range mixedTexts() {
		for width := range 40 {
			got := pretty.TruncateStart(text, width, "...")
			assert.LessOrEqual(t, displaywidth.String(got), width, "%q at width %d gave %q", text, width, got)
			assert.True(t, utf8.ValidString(got), "%q at width %d gave %q", text, width, got)
			assert.True(t, strings.HasSuffix(text, strings.TrimPrefix(got, "...")),
				"%q at width %d gave %q, which is not its end", text, width, got)
		}
	}
}
