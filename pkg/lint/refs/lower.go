package refs

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Characters with lowercase mappings that unicode.ToLower does not give.
const (
	capitalIWithDot = 'İ' // lowercases to "i" and U+0307 COMBINING DOT ABOVE
	capitalSigma    = 'Σ'
	smallSigma      = 'σ'
	finalSmallSigma = 'ς'
)

// jsToLower lowercases s the way JavaScript's String.prototype.toLowerCase
// does, which github-slugger and markdownlint use: the full Unicode case
// mapping without a locale. It differs from strings.ToLower in two places:
// U+0130 becomes "i" followed by U+0307, and a capital sigma that ends a word
// becomes the final sigma.
func jsToLower(s string) string {
	ascii := true
	for idx := range len(s) {
		if s[idx] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return strings.ToLower(s)
	}
	runes := []rune(s)
	var buf strings.Builder
	buf.Grow(len(s))
	for idx, r := range runes {
		switch r {
		case capitalIWithDot:
			buf.WriteString("i̇")
		case capitalSigma:
			if isFinalSigma(runes, idx) {
				buf.WriteRune(finalSmallSigma)
			} else {
				buf.WriteRune(smallSigma)
			}
		default:
			buf.WriteRune(unicode.ToLower(r))
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

// isCased reports whether r has the Unicode Cased property.
func isCased(r rune) bool {
	return unicode.In(r, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Other_Lowercase, unicode.Other_Uppercase)
}

// isCaseIgnorable reports whether r has the Unicode Case_Ignorable property:
// a nonspacing, enclosing, format, modifier letter, or modifier symbol
// character, or one of the word-internal punctuation marks the Word_Break
// property calls MidLetter, MidNumLet, or Single_Quote.
func isCaseIgnorable(r rune) bool {
	switch r {
	case '\'', '.', ':', '·', '·', '՟', '״', '‘', '’', '․', '‧',
		'︓', '﹒', '﹕', '＇', '．', '：':
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}
