package refs

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Characters with lowercase mappings that unicode.ToLower does not give.
const (
	capitalIWithDot = '\u0130' // lowercases to "i" and U+0307 COMBINING DOT ABOVE
	capitalSigma    = '\u03a3'
	smallSigma      = '\u03c3'
	finalSmallSigma = '\u03c2'
)

// jsToLower lowercases text the way JavaScript's String.prototype.toLowerCase
// does, which github-slugger and markdownlint use: the full Unicode case
// mapping without a locale. It differs from strings.ToLower in two places:
// U+0130 becomes "i" followed by U+0307, and a capital sigma that ends a word
// becomes the final sigma.
func jsToLower(text string) string {
	ascii := true
	for idx := range len(text) {
		if text[idx] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(text)
	}
	runes := []rune(text)
	var buf strings.Builder
	buf.Grow(len(text))
	for idx, char := range runes {
		switch char {
		case capitalIWithDot:
			buf.WriteString("i\u0307")
		case capitalSigma:
			if isFinalSigma(runes, idx) {
				buf.WriteRune(finalSmallSigma)
			} else {
				buf.WriteRune(smallSigma)
			}
		default:
			buf.WriteRune(unicode.ToLower(char))
		}
	}
	return buf.String()
}

// isFinalSigma reports whether runes[idx] meets the Unicode Final_Sigma
// condition: a cased letter comes before it, with only case-ignorable
// characters between, and no cased letter follows it after case-ignorable
// characters.
func isFinalSigma(runes []rune, idx int) bool {
	before := idx - 1
	for before >= 0 && isCaseIgnorable(runes[before]) {
		before--
	}
	if before < 0 || !isCased(runes[before]) {
		return false
	}
	after := idx + 1
	for after < len(runes) && isCaseIgnorable(runes[after]) {
		after++
	}
	return after >= len(runes) || !isCased(runes[after])
}

// isCased reports whether char has the Unicode Cased property.
func isCased(char rune) bool {
	return unicode.In(char, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// isCaseIgnorable reports whether char has the Unicode Case_Ignorable property:
// a nonspacing, enclosing, format, modifier letter, or modifier symbol
// character, or one of the word-internal punctuation marks the Word_Break
// property calls MidLetter, MidNumLet, or Single_Quote.
func isCaseIgnorable(char rune) bool {
	switch char {
	case '\'', '.', ':', '\u00b7', '\u0387', '\u055f', '\u05f4', '\u2018', '\u2019', '\u2024', '\u2027',
		'\ufe13', '\ufe52', '\ufe55', '\uff07', '\uff0e', '\uff1a':
		return true
	}
	return unicode.In(char, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}
