package jsonc_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/internal/jsonc"
)

// stripCases are inputs with what markdownlint-cli2 0.23.3 reads from each:
// the value that its JSONC parser, jsonc-parser 3.3.1 called with
// allowTrailingComma as its parsers/jsonc-parse.mjs calls it, returns, as
// JSON.stringify writes it, or rejected when the parser reports an error.
// Each comment names the errors it reports.
var stripCases = []struct {
	input    string
	want     string
	rejected bool
}{
	{input: "{}", want: "{}"},
	{input: "{\"a\": 1}", want: "{\"a\":1}"},
	{input: "{\"a\": 1,}", want: "{\"a\":1}"},
	{input: "{\"a\": 1 ,}", want: "{\"a\":1}"},
	{input: "{\"a\": 1,\n}", want: "{\"a\":1}"},
	{input: "[1,]", want: "[1]"},
	{input: "[1, 2, ]", want: "[1,2]"},
	{input: "{\"a\": [1, 2,],}", want: "{\"a\":[1,2]}"},
	{input: "{\"a\": {\"b\": true,},}", want: "{\"a\":{\"b\":true}}"},
	{input: "[[1,],[2,],]", want: "[[1],[2]]"},
	{input: "[{},]", want: "[{}]"},
	{input: "{\"a\": 1,,}", rejected: true},  // PropertyNameExpected,ValueExpected
	{input: "{,}", rejected: true},           // ValueExpected
	{input: "[,]", rejected: true},           // ValueExpected
	{input: "[1,,2]", rejected: true},        // ValueExpected
	{input: "[1,,]", rejected: true},         // ValueExpected
	{input: "{\"a\": 1}\n,", rejected: true}, // EndOfFileExpected
	{input: "{\"a\": 1},", rejected: true},   // EndOfFileExpected
	{input: "{\"a\",}", rejected: true},      // ColonExpected
	{input: "{\"a\":,}", rejected: true},     // ValueExpected
	{input: "{\"a\" , : 1}", rejected: true}, // ColonExpected,PropertyNameExpected,ValueExpected
	{input: "[1,}", rejected: true},          // ValueExpected,CloseBracketExpected
	{input: "{\"a\": 1,]", rejected: true},   // PropertyNameExpected,ValueExpected,CloseBraceExpected
	{input: "// c\n{\"a\": 1}", want: "{\"a\":1}"},
	{input: "{\"a\": 1} // c", want: "{\"a\":1}"},
	{input: "{\"a\": 1} // c\n", want: "{\"a\":1}"},
	{input: "{\"a\": 1}//c", want: "{\"a\":1}"},
	{input: "{\"a\"://c\n1}", want: "{\"a\":1}"},
	{input: "{\"a\": 1/*c*/}", want: "{\"a\":1}"},
	{input: "{\"a\": 1 /* c */ , /* d */ }", want: "{\"a\":1}"},
	{input: "{\"a\": 1, // c\n}", want: "{\"a\":1}"},
	{input: "/* multi\nline */ {\"a\": 1}", want: "{\"a\":1}"},
	{input: "{\"a\": /* x\r\ny */ 1}", want: "{\"a\":1}"},
	{input: "{\"a\": 1}\r// c\r", want: "{\"a\":1}"},
	{input: "{\"a\": 1}\n/**/", want: "{\"a\":1}"},
	{input: "{\"a\": 1}\n/***/", want: "{\"a\":1}"},
	{input: "{\"a\": 1}\n/*/", rejected: true},               // UnexpectedEndOfComment
	{input: "{\"a\": 1}\n/* unterminated", rejected: true},   // UnexpectedEndOfComment
	{input: "{\"a\": 1}\n/* unterminated *", rejected: true}, // UnexpectedEndOfComment
	{input: "{\"a\": 1}/", rejected: true},                   // InvalidSymbol
	{input: "{\"a\": 1} / ", rejected: true},                 // InvalidSymbol
	{input: "{\"a\": 1}\n//", want: "{\"a\":1}"},
	{input: "{\"a//b\": \"/*c*/\"}", want: "{\"a//b\":\"/*c*/\"}"},
	{input: "{\"a\\\"//\": 1}", want: "{\"a\\\"//\":1}"},
	{input: "{\"a\\\\\": 1} // c", want: "{\"a\\\\\":1}"},
	{input: "{\"a\": \"x\\\\\"}", want: "{\"a\":\"x\\\\\"}"},
	{input: "\"a // b\"", want: "\"a // b\""},
	{input: "true//c", want: "true"},
	{input: "true/*c*/", want: "true"},
	{input: "tr/**/ue", rejected: true}, // InvalidSymbol,InvalidSymbol,ValueExpected
	{input: "1//c", want: "1"},
	{input: "12/*c*/3", rejected: true}, // EndOfFileExpected
	{input: "-1", want: "-1"},
	{input: "-", rejected: true},   // InvalidSymbol,ValueExpected
	{input: "- 1", rejected: true}, // InvalidSymbol
	{input: "+1", rejected: true},  // InvalidSymbol,ValueExpected
	{input: "01", rejected: true},  // EndOfFileExpected
	{input: "0", want: "0"},
	{input: "0.5", want: "0.5"},
	{input: ".5", rejected: true},   // InvalidSymbol,ValueExpected
	{input: "1.", rejected: true},   // UnexpectedEndOfNumber
	{input: "1.e5", rejected: true}, // UnexpectedEndOfNumber,InvalidSymbol
	{input: "1e5", want: "100000"},
	{input: "1E+5", want: "100000"},
	{input: "1e-5", want: "0.00001"},
	{input: "1e", rejected: true},  // UnexpectedEndOfNumber
	{input: "1e+", rejected: true}, // UnexpectedEndOfNumber
	{input: "-0", want: "0"},
	{input: "0x10", rejected: true},      // InvalidSymbol
	{input: "NaN", rejected: true},       // InvalidSymbol,ValueExpected
	{input: "Infinity", rejected: true},  // InvalidSymbol,ValueExpected
	{input: "-Infinity", rejected: true}, // InvalidSymbol,InvalidSymbol,ValueExpected
	{input: "True", rejected: true},      // InvalidSymbol,ValueExpected
	{input: "null", want: "null"},
	{input: "nul", rejected: true},   // InvalidSymbol,ValueExpected
	{input: "nullx", rejected: true}, // InvalidSymbol,ValueExpected
	{input: "\"a\"", want: "\"a\""},
	{input: "'a'", rejected: true},                  // InvalidSymbol,ValueExpected
	{input: "{'a': 1}", rejected: true},             // InvalidSymbol,PropertyNameExpected,ValueExpected
	{input: "{a: 1}", rejected: true},               // InvalidSymbol,PropertyNameExpected,ValueExpected
	{input: "{\"a\": 1 \"b\": 2}", rejected: true},  // CommaExpected
	{input: "{\"a\": 1}{\"b\": 2}", rejected: true}, // EndOfFileExpected
	{input: "{\"a\": 1} x", rejected: true},         // InvalidSymbol
	{input: "", rejected: true},                     // ValueExpected
	{input: " ", rejected: true},                    // ValueExpected
	{input: "\n\n", rejected: true},                 // ValueExpected
	{input: "// only", rejected: true},              // ValueExpected
	{input: "/* only */", rejected: true},           // ValueExpected
	{input: "\ufeff{\"a\": 1}", rejected: true},     // InvalidSymbol
	{input: "{\"a\": 1}\ufeff", rejected: true},     // InvalidSymbol
	{input: "{\u00a0\"a\": 1}", rejected: true},     // InvalidSymbol
	{input: "{\u000b\"a\": 1}", rejected: true},     // InvalidSymbol
	{input: "{\f\"a\": 1}", rejected: true},         // InvalidSymbol
	{input: "{\u0000\"a\": 1}", rejected: true},     // InvalidSymbol
	{input: "{\"a\":\t1}", want: "{\"a\":1}"},
	{input: "{\r\n\"a\": 1\r\n}", want: "{\"a\":1}"},
	{input: "{\"a\": \"line\nbreak\"}", rejected: true}, // UnexpectedEndOfString,InvalidSymbol,UnexpectedEndOfString,CommaExpected,ColonExpected,CloseBraceExpected
	{input: "{\"a\": \"tab\there\"}", rejected: true},   // InvalidCharacter
	{input: "{\"a\": \"\\t\"}", want: "{\"a\":\"\\t\"}"},
	{input: "{\"a\": \"\\x41\"}", rejected: true},  // InvalidEscapeCharacter
	{input: "{\"a\": \"\\u004\"}", rejected: true}, // InvalidUnicode
	{input: "{\"a\": \"\\u0041\"}", want: "{\"a\":\"A\"}"},
	{input: "{\"a\": \"\\ud83d\\ude00\"}", want: "{\"a\":\"😀\"}"},
	{input: "{\"a\": \"\\/\"}", want: "{\"a\":\"/\"}"},
	{input: "{\"a\": \"\\'\"}", rejected: true},        // InvalidEscapeCharacter
	{input: "{\"a\": \"unterminated}", rejected: true}, // UnexpectedEndOfString,CloseBraceExpected
	{input: "{\"a\": \"unterminated", rejected: true},  // UnexpectedEndOfString,CloseBraceExpected
	{input: "{\"a\": \"esc\\\n\"}", rejected: true},    // InvalidEscapeCharacter
	{input: "{\"a\": \"é中😀\"}", want: "{\"a\":\"é中😀\"}"},
	{input: "{\"a\": 1, \"a\": 2}", want: "{\"a\":2}"},
	{input: "{\"a\": {\"b\": 1}, \"a\": {\"c\": 2},}", want: "{\"a\":{\"c\":2}}"},
	{input: "[\"// not\", \"/* comment */\",]", want: "[\"// not\",\"/* comment */\"]"},
	{input: "{\"MD013\": {\"line_length\": 100,}, // keep\n\"MD009\": false, /* end */}", want: "{\"MD013\":{\"line_length\":100},\"MD009\":false}"},
	{input: "{\n  // The tag comes last.\n  \"MD013\": {\"line_length\": 90}, /* then */ \"whitespace\": false\n}\n", want: "{\"MD013\":{\"line_length\":90},\"whitespace\":false}"},
}

// TestStripReadsJSONCAsMarkdownlintCLI2Does checks that the JSON text that
// Strip returns is valid exactly when jsonc-parser accepts the input, and
// then holds the value that it reads.
func TestStripReadsJSONCAsMarkdownlintCLI2Does(t *testing.T) {
	t.Parallel()

	for _, tt := range stripCases {
		stripped := jsonc.Strip([]byte(tt.input))
		if tt.rejected {
			assert.False(t, json.Valid(stripped), "%q must be rejected, as %q", tt.input, stripped)
			continue
		}
		assert.JSONEq(t, tt.want, string(stripped), "%q", tt.input)
	}
}

// TestStripKeepsOffsets checks that Strip only turns bytes into spaces, and
// never a line break, so that every byte keeps its offset and every token
// its line, and that it leaves its input as it is.
func TestStripKeepsOffsets(t *testing.T) {
	t.Parallel()

	for _, tt := range stripCases {
		input := []byte(tt.input)
		stripped := jsonc.Strip(input)

		assert.Equal(t, tt.input, string(input), "the input stays as it is")
		if !assert.Len(t, stripped, len(input), "%q", tt.input) {
			continue
		}
		for idx := range input {
			if stripped[idx] == input[idx] {
				continue
			}
			assert.Equal(t, byte(' '), stripped[idx], "%q byte %d", tt.input, idx)
			assert.NotContains(t, "\r\n", string(input[idx]), "%q keeps its line break at %d", tt.input, idx)
		}
	}
}
