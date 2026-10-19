package lint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestEngineConfigureFileNoticesInvalidOptionValues covers option values in
// a markdownlint-configure-file setting that gomdlint cannot use: a value
// of the wrong type, or a list with an item of the wrong type, which it
// ignores, and a string that the option does not allow, which it keeps, as
// the rule treats such a value as markdownlint does. Config files warn
// about each, and so does the comment now, with a notice that names the
// rule as markdownlint names it. They used to change the file silently.
func TestEngineConfigureFileNoticesInvalidOptionValues(t *testing.T) {
	tests := []struct {
		name, parameter string
		want            []lint.Notice
	}{
		{
			name: "wrong type", parameter: `{"MD009": {"strict": "yes"}}`,
			want: []lint.Notice{{
				Name: "strict", Rules: []string{"no-trailing-spaces"},
				Message: `no-trailing-spaces option "strict" must be true or false, not "yes"; it is ignored`,
			}},
		},
		{
			name: "list item", parameter: `{"MD033": {"allowed_elements": ["br", 1]}}`,
			want: []lint.Notice{{
				Name: "allowed_elements", Rules: []string{"no-inline-html"},
				Message: `no-inline-html option "allowed_elements" item 2 must be a string, not 1; the option is ignored`,
			}},
		},
		{
			name: "not allowed", parameter: `{"MD004": {"style": "dashes"}}`,
			want: []lint.Notice{{
				Name: "style", Rules: []string{"ul-style"}, Suggestions: []string{"dash"},
				Message: `ul-style option "style" must be one of "consistent", "asterisk", "plus", "dash" or "sublist", ` +
					`not "dashes" (did you mean "dash"?)`,
			}},
		},
		{
			name: "a tag", parameter: `{"whitespace": {"br_spaces": "x"}}`,
			want: []lint.Notice{{
				Name: "br_spaces", Rules: []string{"no-trailing-spaces"},
				Message: `no-trailing-spaces option "br_spaces" must be an integer, not "x"; it is ignored`,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lintDefault(t, configuredDoc(tt.parameter))

			want := make([]lint.Notice, len(tt.want))
			for idx, notice := range tt.want {
				notice.Kind = lint.NoticeInvalidOption
				notice.Line = 1
				notice.Directive = "markdownlint-configure-file"
				notice.Message = "markdownlint-configure-file: " + notice.Message
				want[idx] = notice
			}
			assert.Equal(t, want, result.Notices)
		})
	}
}

// TestEngineConfigureFileReadsNumericStrings covers a numeric string for a
// numeric option in a configure-file comment, which markdownlint 0.41.1
// reads as its number, so markdownlint-cli2 reports the comment's line and
// the paragraph as longer than 20 characters, without a notice.
func TestEngineConfigureFileReadsNumericStrings(t *testing.T) {
	const paragraph = "This paragraph line is long enough to pass twenty.\n"
	for _, length := range []string{`"20"`, `" 0x14 "`, `"2e1"`} {
		t.Run(length, func(t *testing.T) {
			input := "<!-- markdownlint-configure-file {\"MD013\": {\"line_length\": " + length + "}} -->\n\n# Title\n\n" + paragraph

			assert.Equal(t, []int{1, 5}, reportedLines(t, input, "MD013"))
			assert.Empty(t, lintDefault(t, input).Notices)
		})
	}
}
