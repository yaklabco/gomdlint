package mdscan

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// LiteralKind identifies a GFM autolink literal.
type LiteralKind int

// Kinds of GFM autolink literal that MatchAutolinkLiteral recognizes.
const (
	// LiteralNone means no autolink literal starts at the position.
	LiteralNone LiteralKind = iota
	// LiteralURL is an http or https URL.
	LiteralURL
	// LiteralEmail is an email address.
	LiteralEmail
)

// maxProtocolLength is the length of "https", the longest protocol accepted.
const maxProtocolLength = 5

// trailPunctuation are the characters that end an autolink literal when only
// more of them, or whitespace, follow.
const trailPunctuation = "!\"')*,.:;?_~"

// pathPunctuation are the path characters checked for being a trail.
const pathPunctuation = "!\"&')*,.:;<?]_~"

// MatchAutolinkLiteral matches a GFM autolink literal (an http or https URL,
// or an email address) that starts at text[pos], following the rules of
// micromark-extension-gfm-autolink-literal, which markdownlint uses. prev is
// the character before pos, or 0 at the start of a line. It returns the
// offset just after the literal: trailing punctuation, an unbalanced closing
// parenthesis, and an entity reference at the end are not part of it.
// "www." literals are not matched.
func MatchAutolinkLiteral(text []byte, pos int, prev byte) (int, LiteralKind) {
	if pos < 0 || pos >= len(text) {
		return pos, LiteralNone
	}
	if text[pos] == 'h' || text[pos] == 'H' {
		if end, ok := matchProtocolLiteral(text, pos, prev); ok {
			return end, LiteralURL
		}
	}
	if end, ok := matchEmailLiteral(text, pos, prev); ok {
		return end, LiteralEmail
	}
	return pos, LiteralNone
}

// matchProtocolLiteral matches "http://" or "https://", a domain, and a path.
func matchProtocolLiteral(text []byte, pos int, prev byte) (int, bool) {
	if isASCIILetter(prev) {
		return pos, false
	}
	idx := pos
	for idx < len(text) && isASCIILetter(text[idx]) && idx-pos < maxProtocolLength {
		idx++
	}
	protocol := strings.ToLower(string(text[pos:idx]))
	if (protocol != "http" && protocol != "https") || !hasPrefixAt(text, idx, "://") {
		return pos, false
	}
	idx += len("://")
	if idx >= len(text) {
		return pos, false
	}
	if first, _ := utf8.DecodeRune(text[idx:]); unicode.IsControl(first) || unicode.IsSpace(first) || isUnicodePunct(first) {
		return pos, false
	}
	end, ok := matchLiteralDomain(text, idx)
	if !ok {
		return pos, false
	}
	return matchLiteralPath(text, end), true
}

// matchLiteralDomain matches a domain: anything but whitespace and
// punctuation other than '-', where '.' and '_' are allowed unless they start
// the trail. Underscores are not allowed in the last two segments.
func matchLiteralDomain(text []byte, idx int) (int, bool) {
	seen := false
	underscoreInLast := false
	underscoreInSecondLast := false
	for idx < len(text) {
		ch := text[idx]
		if ch == '.' || ch == '_' {
			if isLiteralTrail(text, idx) {
				break
			}
			if ch == '_' {
				underscoreInLast = true
			} else {
				underscoreInSecondLast = underscoreInLast
				underscoreInLast = false
			}
			idx++
			continue
		}
		r, size := utf8.DecodeRune(text[idx:])
		if unicode.IsSpace(r) || (ch != '-' && isUnicodePunct(r)) {
			break
		}
		seen = true
		idx += size
	}
	return idx, seen && !underscoreInLast && !underscoreInSecondLast
}

// matchLiteralPath matches the path after a domain up to whitespace or the
// trail, keeping closing parentheses only while they balance opening ones.
func matchLiteralPath(text []byte, idx int) int {
	opened, closed := 0, 0
	for idx < len(text) {
		ch := text[idx]
		switch {
		case ch == '(':
			opened++
			idx++
			continue
		case ch == ')' && closed < opened:
			closed++
			idx++
			continue
		case strings.IndexByte(pathPunctuation, ch) >= 0:
			if isLiteralTrail(text, idx) {
				return idx
			}
			if ch == ')' {
				closed++
			}
			idx++
			continue
		}
		r, size := utf8.DecodeRune(text[idx:])
		if unicode.IsSpace(r) {
			return idx
		}
		idx += size
	}
	return idx
}

// isLiteralTrail reports whether text[idx:] starts with trailing punctuation
// that ends the literal: trail characters, entity references, and ']' not
// followed by '(' or '[', up to whitespace, '<', or the end of the text.
func isLiteralTrail(text []byte, idx int) bool {
	for idx < len(text) {
		ch := text[idx]
		switch {
		case strings.IndexByte(trailPunctuation, ch) >= 0:
			idx++
			continue
		case ch == '&':
			end := idx + 1
			for end < len(text) && isASCIILetter(text[end]) {
				end++
			}
			if end == idx+1 || end >= len(text) || text[end] != ';' {
				return false
			}
			idx = end + 1
			continue
		case ch == ']':
			idx++
			if idx >= len(text) || text[idx] == '(' || text[idx] == '[' || isSpaceAt(text, idx) {
				return true
			}
			continue
		case ch == '<':
			return true
		}
		return isSpaceAt(text, idx)
	}
	return true
}

// matchEmailLiteral matches local@domain where the domain contains a dot and
// ends in a letter. A trailing dot is not part of the address.
func matchEmailLiteral(text []byte, pos int, prev byte) (int, bool) {
	if prev == '/' || isGFMAtext(prev) || !isGFMAtext(text[pos]) {
		return pos, false
	}
	idx := pos
	for idx < len(text) && isGFMAtext(text[idx]) {
		idx++
	}
	if idx >= len(text) || text[idx] != '@' {
		return pos, false
	}
	idx++
	data, dot := false, false
	var last byte
	for idx < len(text) {
		ch := text[idx]
		if ch == '.' {
			if idx+1 >= len(text) || !isASCIIAlphanumeric(text[idx+1]) {
				break
			}
			dot = true
		} else if ch == '-' || ch == '_' || isASCIIAlphanumeric(ch) {
			data = true
		} else {
			break
		}
		last = ch
		idx++
	}
	if !data || !dot || !isASCIILetter(last) {
		return pos, false
	}
	return idx, true
}

func isGFMAtext(ch byte) bool {
	return ch == '+' || ch == '-' || ch == '.' || ch == '_' || isASCIIAlphanumeric(ch)
}

func isASCIIAlphanumeric(ch byte) bool {
	return isASCIILetter(ch) || isASCIIDigit(ch)
}

func isUnicodePunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func isSpaceAt(text []byte, idx int) bool {
	r, _ := utf8.DecodeRune(text[idx:])
	return unicode.IsSpace(r)
}

func hasPrefixAt(text []byte, idx int, prefix string) bool {
	return idx+len(prefix) <= len(text) && string(text[idx:idx+len(prefix)]) == prefix
}
