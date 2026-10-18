// Package jsonc reads JSON with comments and trailing commas as
// markdownlint-cli2 reads its JSONC configuration files.
//
// markdownlint-cli2 0.23.3 parses .markdownlint.jsonc and .markdownlint.json
// files, and markdownlint-configure-file comments, with jsonc-parser 3.3.1,
// passing the option allowTrailingComma (parsers/jsonc-parse.mjs), and it
// rejects the text when the parser reports any error. That dialect is JSON
// with two additions. Comments may go wherever whitespace may, and run from
// // to the next line feed or carriage return, or from /* to the next */.
// One comma may follow the last member of an object or the last item of an
// array. Everything else is JSON's. Only spaces, tabs, line feeds and
// carriage returns are whitespace, so a byte order mark is an error, and
// strings, numbers and literals are written as JSON writes them.
package jsonc

import "bytes"

// Strip returns text with every comment and every trailing comma replaced
// by spaces, keeping the line feeds and carriage returns within comments.
// Each byte keeps its offset and each token its line, so an error or a
// position found in the result holds for text.
//
// The result is valid JSON exactly when jsonc-parser 3.3.1 with
// allowTrailingComma reads text without an error, and it then holds the
// value that the parser reads. Strip only removes what the dialect allows,
// and nothing that text holds in error, such as a comment without its
// closing */, a lone slash or a comma that follows no value, so that JSON
// rejects such text too.
//
// Strip returns text itself, without copying it, when it holds nothing to
// replace, and as soon as it holds a word that starts no JSON value, such
// as a stray <!--, which no blanking could make valid. So it reads text
// only as far as such an error, and a caller must not change the result.
func Strip(text []byte) []byte {
	// blanks are the comments and trailing commas to replace, in order.
	var blanks [][2]int

	// valueEnded reports whether the last token ended a value: a string, a
	// number, a literal, or a closing brace or bracket.
	valueEnded := false
	// comma is the offset of a comma that follows a value until the next
	// token shows whether it is a trailing comma, and -1 otherwise.
	comma := -1

	for pos := 0; pos < len(text); {
		switch char := text[pos]; {
		case char == ' ' || char == '\t' || char == '\n' || char == '\r':
			pos++
			continue
		case char == '/' && pos+1 < len(text) && text[pos+1] == '/':
			end := lineCommentEnd(text, pos)
			blanks = append(blanks, [2]int{pos, end})
			pos = end
			continue
		case char == '/' && pos+1 < len(text) && text[pos+1] == '*':
			end := bytes.Index(text[pos+2:], []byte("*/"))
			if end < 0 {
				// A comment without its end is an error, which the
				// comment, left as it is, makes an error in JSON too.
				return text
			}
			end += pos + len("/**/")
			blanks = append(blanks, [2]int{pos, end})
			pos = end
			continue
		}

		// Every other byte starts a token, which settles a pending comma:
		// it trails when the token closes an object or an array.
		char := text[pos]
		if comma >= 0 && (char == '}' || char == ']') {
			blanks = append(blanks, [2]int{comma, comma + 1})
		}
		comma = -1

		switch char {
		case '"':
			pos = stringEnd(text, pos)
			valueEnded = true
		case ',':
			if valueEnded {
				comma = pos
			}
			valueEnded = false
			pos++
		case '}', ']':
			valueEnded = true
			pos++
		case '{', '[', ':', '/':
			// A lone slash is an error, which it stays in JSON.
			valueEnded = false
			pos++
		default:
			end := wordEnd(text, pos)
			if !mayBeValue(text[pos:end]) {
				return text // An error in jsonc-parser and in JSON alike.
			}
			pos = end
			valueEnded = true
		}
	}

	if len(blanks) == 0 {
		return text
	}
	stripped := bytes.Clone(text)
	for _, span := range blanks {
		blank(stripped[span[0]:span[1]])
	}
	return stripped
}

// mayBeValue reports whether word, a word as wordEnd reads it, may be a
// JSON value: a number, which starts with a minus sign or a digit and which
// JSON checks in full, or one of the literals true, false and null. Every
// other word is an error in jsonc-parser, which reads it as an unknown
// symbol, and in JSON.
func mayBeValue(word []byte) bool {
	switch string(word) {
	case "true", "false", "null":
		return true
	}
	return word[0] == '-' || '0' <= word[0] && word[0] <= '9'
}

// lineCommentEnd returns the offset where the line comment that starts at
// start ends: at the next line feed or carriage return, which the comment
// leaves to the next line, or at the end of text.
func lineCommentEnd(text []byte, start int) int {
	end := bytes.IndexAny(text[start:], "\n\r")
	if end < 0 {
		return len(text)
	}
	return start + end
}

// stringEnd returns the offset right after the string that starts with the
// quote at start, as jsonc-parser scans it: a backslash escapes the byte
// after it, and an unescaped line feed or carriage return, which is an
// error, ends the string before it. A string without its closing quote
// runs to the end of text.
func stringEnd(text []byte, start int) int {
	for pos := start + 1; pos < len(text); pos++ {
		switch text[pos] {
		case '"':
			return pos + 1
		case '\\':
			pos++ // The escaped byte, whatever it is, belongs to the string.
		case '\n', '\r':
			return pos
		}
	}
	return len(text)
}

// wordEnd returns the offset right after the word that starts at start: a
// number, a literal such as true, or any other run of bytes that are not
// whitespace, a quote, a slash or a structural character, as jsonc-parser
// reads a word. A word is never empty, since start holds none of those.
func wordEnd(text []byte, start int) int {
	pos := start + 1
	for pos < len(text) {
		switch text[pos] {
		case ' ', '\t', '\n', '\r', '{', '}', '[', ']', '"', ':', ',', '/':
			return pos
		}
		pos++
	}
	return pos
}

// blank replaces each byte of comment with a space, except line feeds and
// carriage returns, so that the lines after the comment keep their numbers.
func blank(comment []byte) {
	for idx, char := range comment {
		if char != '\n' && char != '\r' {
			comment[idx] = ' '
		}
	}
}
