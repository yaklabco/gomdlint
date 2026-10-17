package pretty

import (
	"github.com/clipperhouse/displaywidth"
)

// TruncateEnd shortens text to at most width terminal cells by replacing its
// end with marker, so its start stays visible. It measures cells as a
// terminal shows them, two for a wide character and none for a combining
// mark, and cuts only between grapheme clusters, so it never splits a
// character. Text that fits is returned unchanged. When width cannot hold
// the marker, the text is cut without one.
func TruncateEnd(text string, width int, marker string) string {
	if displaywidth.String(text) <= width {
		return text
	}
	if displaywidth.String(marker) > width {
		marker = ""
	}
	return displaywidth.TruncateString(text, width, marker)
}

// TruncateStart shortens text to at most width terminal cells by replacing
// its start with marker, so its end, such as the name of a file at the end
// of a path, stays visible. It measures and cuts as TruncateEnd does.
func TruncateStart(text string, width int, marker string) string {
	if displaywidth.String(text) <= width {
		return text
	}
	markerWidth := displaywidth.String(marker)
	if markerWidth > width {
		marker, markerWidth = "", 0
	}

	var starts, widths []int
	graphemes := displaywidth.StringGraphemes(text)
	for offset := 0; graphemes.Next(); offset += len(graphemes.Value()) {
		starts = append(starts, offset)
		widths = append(widths, graphemes.Width())
	}

	budget := width - markerWidth
	keepFrom := len(text)
	for idx := len(starts) - 1; idx >= 0 && widths[idx] <= budget; idx-- {
		budget -= widths[idx]
		keepFrom = starts[idx]
	}

	return marker + text[keepFrom:]
}
