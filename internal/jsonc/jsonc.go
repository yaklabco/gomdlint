// Package jsonc reads JSON with comments and trailing commas as
// markdownlint-cli2 reads its JSONC configuration files.
//
// markdownlint-cli2 0.23.3 parses .markdownlint.jsonc and .markdownlint.json
// files, and markdownlint-configure-file comments, with jsonc-parser 3.3.1,
// passing the option allowTrailingComma (parsers/jsonc-parse.mjs), and it
// rejects the text when the parser reports any error. That dialect is JSON
// with two additions: comments, which run from // to the next line feed or
// carriage return or from /* to the next */, wherever whitespace may go;
// and one comma after the last member of an object or the last item of an
// array. Everything else is JSON's: only spaces, tabs, line feeds and
// carriage returns are whitespace, so a byte order mark is an error, and
// strings, numbers and literals are written as JSON writes them.
package jsonc

import "bytes"

// Strip returns a copy of text in which every comment and every trailing
// comma is replaced by spaces, keeping the line feeds and carriage returns
// within comments. Each byte keeps its offset and each token its line, so
// an error or a position found in the result holds for text.
//
// The result is valid JSON exactly when jsonc-parser 3.3.1 with
// allowTrailingComma reads text without an error, and it then holds the
// value that the parser reads. Strip only removes what the dialect allows,
// and nothing that text holds in error, such as a comment without its
// closing */, a lone slash or a comma that follows no value, so that JSON
// rejects such text too.
func Strip(text []byte) []byte {
	stripped := bytes.Clone(text)

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
			blank(stripped[pos:end])
			pos = end
			continue
		case char == '/' && pos+1 < len(text) && text[pos+1] == '*':
			end := bytes.Index(text[pos+2:], []byte("*/"))
			if end < 0 {
				// A comment without its end is an error, which the
				// comment, left as it is, makes an error in JSON too.
				return stripped
			}
			end += pos + len("/**/")
			blank(stripped[pos:end])
			pos = end
			continue
		}

		// Every other byte starts a token, which settles a pending comma:
		// it trails when the token closes an object or an array.
		char := text[pos]
		if comma >= 0 && (char == '}' || char == ']') {
			stripped[comma] = ' '
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
			pos = wordEnd(text, pos)
			valueEnded = true
		}
	}
	return stripped
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
