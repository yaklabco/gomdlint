package mdscan

import "regexp"

// maxLinkLabelLength is the CommonMark limit on characters inside a link label's brackets.
const maxLinkLabelLength = 999

// CommonMark autolink grammar: a URI with a 2 to 32 character scheme, or an email address.
var (
	uriAutolinkPattern   = regexp.MustCompile(`^<[A-Za-z][A-Za-z0-9+.-]{1,31}:[^\x00-\x20\x7f<>]*>`)
	emailAutolinkPattern = regexp.MustCompile("^<[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@" +
		`[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*>`)
)

// IsASCIIPunct reports whether ch is ASCII punctuation, the set of characters
// a backslash can escape.
func IsASCIIPunct(ch byte) bool {
	return (ch >= '!' && ch <= '/') || (ch >= ':' && ch <= '@') || (ch >= '[' && ch <= '`') || (ch >= '{' && ch <= '~')
}

func isASCIILetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isASCIIDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// IsEscaped reports whether src[pos] is escaped by an odd number of backslashes before it.
func IsEscaped(src []byte, pos int) bool {
	backslashes := 0
	for idx := pos - 1; idx >= 0 && src[idx] == '\\'; idx-- {
		backslashes++
	}
	return backslashes%2 == 1
}

// BacktickRun returns the length of the run of backticks starting at src[pos].
func BacktickRun(src []byte, pos int) int {
	end := pos
	for end < len(src) && src[end] == '`' {
		end++
	}
	return end - pos
}

// CodeSpanEnd returns the offset just after the code span whose opening
// backtick string starts at src[pos]. It reports false when no backtick
// string of the same length follows in src, in which case the opening
// backticks are literal text. Callers bound src to the enclosing paragraph or
// link so a closer in unrelated text is never used.
func CodeSpanEnd(src []byte, pos int) (int, bool) {
	opener := BacktickRun(src, pos)
	if opener == 0 {
		return pos, false
	}
	for idx := pos + opener; idx < len(src); {
		if src[idx] != '`' {
			idx++
			continue
		}
		run := BacktickRun(src, idx)
		if run == opener {
			return idx + run, true
		}
		idx += run
	}
	return pos, false
}

// MatchAutolink returns the offset just after a CommonMark autolink
// ("<scheme:...>" or "<user@host>") that starts at src[pos].
func MatchAutolink(src []byte, pos int) (int, bool) {
	if pos < 0 || pos >= len(src) || src[pos] != '<' {
		return pos, false
	}
	if loc := uriAutolinkPattern.FindIndex(src[pos:]); loc != nil {
		return pos + loc[1], true
	}
	if loc := emailAutolinkPattern.FindIndex(src[pos:]); loc != nil {
		return pos + loc[1], true
	}
	return pos, false
}

// SkipInlineConstruct returns the offset just after a backslash escape, code
// span, autolink, or raw HTML construct that starts at src[pos]. Brackets
// inside these constructs never delimit link text. It reports false when no
// such construct starts at pos.
func SkipInlineConstruct(src []byte, pos int) (int, bool) {
	switch src[pos] {
	case '\\':
		if pos+1 < len(src) && IsASCIIPunct(src[pos+1]) {
			return pos + 2, true
		}
	case '`':
		if end, ok := CodeSpanEnd(src, pos); ok {
			return end, true
		}
		return pos + BacktickRun(src, pos), true
	case '<':
		if end, ok := MatchAutolink(src, pos); ok {
			return end, true
		}
		if construct, ok := MatchHTML(src, pos); ok {
			return construct.End, true
		}
	}
	return pos, false
}

// LinkTextEnd returns the offset of the ']' that closes the link text whose
// '[' is at src[open]. Link text may contain balanced brackets; brackets in
// escapes, code spans, autolinks, and raw HTML do not count. Bound src to the
// end of the link so constructs outside it are ignored.
func LinkTextEnd(src []byte, open int) (int, bool) {
	if open < 0 || open >= len(src) || src[open] != '[' {
		return 0, false
	}
	depth := 0
	for idx := open + 1; idx < len(src); {
		if next, skipped := SkipInlineConstruct(src, idx); skipped {
			idx = next
			continue
		}
		switch src[idx] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return idx, true
			}
			depth--
		}
		idx++
	}
	return 0, false
}

// LinkLabelEnd returns the offset of the ']' that closes the link label whose
// '[' is at src[open]. A link label contains no unescaped brackets, at most
// 999 characters, and at least one character that is not whitespace.
func LinkLabelEnd(src []byte, open int) (int, bool) {
	if open < 0 || open >= len(src) || src[open] != '[' {
		return 0, false
	}
	blank := true
	for idx := open + 1; idx < len(src) && idx-open-1 <= maxLinkLabelLength; idx++ {
		switch ch := src[idx]; ch {
		case '\\':
			blank = false
			if idx+1 < len(src) && IsASCIIPunct(src[idx+1]) {
				idx++
			}
		case '[':
			return 0, false
		case ']':
			if blank {
				return 0, false
			}
			return idx, true
		case ' ', '\t', '\n', '\r':
		default:
			blank = false
		}
	}
	return 0, false
}
