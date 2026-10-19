package ruledocs

import (
	"strings"
	"unicode/utf8"
)

// lineWidth is the width in characters at which text and Markdown output
// wrap prose. The repository's own lint configuration asks for it too.
const lineWidth = 80

// wrap breaks text into lines at spaces, so that each line, which starts
// with first for the first line and with rest for the others, has at most
// width characters. A word that does not fit on a line of its own gets one
// anyway. A Markdown code span between single backticks counts as one
// word: spaces between backticks that no backslash escapes do not break.
// Lines carry no trailing spaces, and an empty text gives one line of
// first without them.
func wrap(text string, width int, first, rest string) []string {
	var lines []string
	line, empty := first, true
	for _, word := range splitWords(text) {
		if !empty && utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > width {
			lines = append(lines, line)
			line, empty = rest, true
		}
		if !empty {
			line += " "
		}
		line += word
		empty = false
	}
	return append(lines, strings.TrimRight(line, " "))
}

// splitWords splits text at spaces, except inside code spans, where a
// backslash escapes nothing.
func splitWords(text string) []string {
	var split []string
	var word strings.Builder
	inCode, escaped := false, false
	for _, char := range text {
		switch {
		case inCode:
			inCode = char != '`'
		case escaped:
			escaped = false
		case char == '\\':
			escaped = true
		case char == '`':
			inCode = true
		case char == ' ':
			if word.Len() > 0 {
				split = append(split, word.String())
				word.Reset()
			}
			continue
		}
		word.WriteRune(char)
	}
	if word.Len() > 0 {
		split = append(split, word.String())
	}
	return split
}
