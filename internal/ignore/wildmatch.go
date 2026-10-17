package ignore

import (
	"errors"
	"fmt"
	"strings"
)

// This file ports the parts of Git's wildmatch (wildmatch.c) that apply
// within one path segment, so patterns match byte for byte as Git matches
// them: "?" matches one byte, not one character, and character classes are
// ASCII only.

var (
	errUnterminatedBracket = errors.New("unterminated bracket expression")
	errTrailingBackslash   = errors.New("trailing backslash")
)

// asciiDelete is DEL, the last ASCII byte and a control character.
const asciiDelete = 0x7f

// wildResult is the outcome of matching a pattern against a segment.
type wildResult int

const (
	wildMatch wildResult = iota
	wildNoMatch
	// wildAbort means no match, and also that no "*" earlier in the pattern
	// can produce one by matching more bytes, so callers stop trying.
	wildAbort
)

// wildmatch reports whether name, one path segment, matches pattern, one
// pattern segment.
func wildmatch(pattern, name string) bool {
	return doWild(pattern, name) == wildMatch
}

// doWild matches text against pat as Git's dowild does for text without
// slashes.
func doWild(pat, text string) wildResult {
	pIdx, tIdx := 0, 0
	for ; pIdx < len(pat); pIdx, tIdx = pIdx+1, tIdx+1 {
		pChar := pat[pIdx]
		if tIdx == len(text) && pChar != '*' {
			return wildAbort
		}
		switch pChar {
		case '\\':
			// Match the next byte literally. A trailing backslash matches
			// nothing.
			pIdx++
			if pIdx == len(pat) || text[tIdx] != pat[pIdx] {
				return wildNoMatch
			}
		case '?':
			// Any one byte.
		case '*':
			// A run of stars matches like one star within a segment.
			for pIdx+1 < len(pat) && pat[pIdx+1] == '*' {
				pIdx++
			}
			if pIdx+1 == len(pat) {
				return wildMatch
			}
			for ; tIdx < len(text); tIdx++ {
				if result := doWild(pat[pIdx+1:], text[tIdx:]); result != wildNoMatch {
					return result
				}
			}
			return wildAbort
		case '[':
			matched, end, err := matchBracket(pat, pIdx, text[tIdx])
			if err != nil {
				return wildAbort
			}
			if !matched {
				return wildNoMatch
			}
			pIdx = end
		default:
			if text[tIdx] != pChar {
				return wildNoMatch
			}
		}
	}
	if tIdx < len(text) {
		return wildNoMatch
	}
	return wildMatch
}

// matchBracket parses the bracket expression that opens at pat[open] and
// reports whether it matches char. It returns the index of the closing "]".
//
// As in Git, "!" or "^" first negates, a "]" first (after any negation) is
// a member, "-" between two members makes a range, "[:name:]" is a POSIX
// class, and a backslash makes the next byte a member. A range or class
// cannot start a range, so "-" after one is a member. An unterminated
// expression or an unknown class is an error: the pattern matches nothing.
func matchBracket(pat string, open int, char byte) (bool, int, error) {
	pIdx := open + 1
	negated := pIdx < len(pat) && (pat[pIdx] == '!' || pat[pIdx] == '^')
	if negated {
		pIdx++
	}

	matched := false
	prev := -1 // the previous member, while it may start a range
	for first := true; ; first = false {
		if pIdx == len(pat) {
			return false, 0, errUnterminatedBracket
		}
		member := int(pat[pIdx])
		switch {
		case pat[pIdx] == ']' && !first:
			return matched != negated, pIdx, nil
		case pat[pIdx] == '\\':
			pIdx++
			if pIdx == len(pat) {
				return false, 0, errUnterminatedBracket
			}
			member = int(pat[pIdx])
			matched = matched || char == pat[pIdx]
		case pat[pIdx] == '-' && prev >= 0 && pIdx+1 < len(pat) && pat[pIdx+1] != ']':
			pIdx++
			if pat[pIdx] == '\\' {
				pIdx++
				if pIdx == len(pat) {
					return false, 0, errUnterminatedBracket
				}
			}
			matched = matched || (int(char) >= prev && char <= pat[pIdx])
			member = -1
		case pat[pIdx] == '[' && pIdx+1 < len(pat) && pat[pIdx+1] == ':':
			nameStart := pIdx + 2
			closing := strings.IndexByte(pat[nameStart:], ']')
			if closing < 0 {
				return false, 0, errUnterminatedBracket
			}
			closing += nameStart
			if closing == nameStart || pat[closing-1] != ':' {
				// No ":]", so the "[" is an ordinary member.
				matched = matched || char == '['
				break
			}
			inClass, known := charClass(pat[nameStart:closing-1], char)
			if !known {
				return false, 0, fmt.Errorf("unknown character class %q", pat[pIdx:closing+1])
			}
			matched = matched || inClass
			pIdx = closing
			member = -1
		default:
			matched = matched || char == pat[pIdx]
		}
		prev = member
		pIdx++
	}
}

// charClass reports whether char belongs to the named POSIX class, as Git's
// own ASCII-only ctype defines the classes, and whether the class exists.
func charClass(name string, char byte) (bool, bool) {
	alpha := (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
	digit := char >= '0' && char <= '9'
	graph := char > ' ' && char < asciiDelete

	switch name {
	case "alnum":
		return alpha || digit, true
	case "alpha":
		return alpha, true
	case "blank":
		return char == ' ' || char == '\t', true
	case "cntrl":
		return char < ' ' || char == asciiDelete, true
	case "digit":
		return digit, true
	case "graph":
		return graph, true
	case "lower":
		return char >= 'a' && char <= 'z', true
	case "print":
		return graph || char == ' ', true
	case "punct":
		return graph && !alpha && !digit, true
	case "space":
		// Git leaves out vertical tab and form feed.
		return char == ' ' || char == '\t' || char == '\n' || char == '\r', true
	case "upper":
		return char >= 'A' && char <= 'Z', true
	case "xdigit":
		return digit || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F'), true
	default:
		return false, false
	}
}

// validateSegment reports a construct that stops a pattern segment from
// matching anything: an unterminated bracket expression, an unknown
// character class or a trailing backslash.
func validateSegment(segment string) error {
	for idx := 0; idx < len(segment); idx++ {
		switch segment[idx] {
		case '\\':
			if idx+1 == len(segment) {
				return errTrailingBackslash
			}
			idx++
		case '[':
			_, end, err := matchBracket(segment, idx, 0)
			if err != nil {
				return err
			}
			idx = end
		}
	}
	return nil
}

// isDoubleStar reports whether a pattern segment is a run of two or more
// stars, which matches any number of segments.
func isDoubleStar(segment string) bool {
	return len(segment) >= len(doubleStar) && strings.Trim(segment, "*") == ""
}

// splitPattern splits a pattern into segments at the slashes that separate
// path segments. As in Git, an escaped slash separates segments too, while
// a slash inside a bracket expression does not. Before an escaped slash a
// run of stars matches at least one segment, so it becomes "*" and "**".
func splitPattern(text string) []string {
	var segments []string
	start := 0
	for idx := 0; idx < len(text); idx++ {
		switch text[idx] {
		case '/':
			segments = append(segments, text[start:idx])
			start = idx + 1
		case '\\':
			if idx+1 < len(text) && text[idx+1] == '/' {
				if segment := text[start:idx]; isDoubleStar(segment) {
					segments = append(segments, "*", doubleStar)
				} else {
					segments = append(segments, segment)
				}
				start = idx + 2
			}
			idx++
		case '[':
			if _, end, err := matchBracket(text, idx, 0); err == nil {
				idx = end
			}
		}
	}
	return append(segments, text[start:])
}
