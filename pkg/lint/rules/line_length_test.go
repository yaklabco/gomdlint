package rules

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// spacedLine returns a line of exactly n bytes made of four-letter words, so
// that markdownlint's allowance for a final unbreakable run does not apply.
func spacedLine(n int) string {
	line := []byte(strings.Repeat("abcd ", n/5+1)[:n])
	if line[n-1] == ' ' {
		line[n-1] = 'x'
	}
	return string(line)
}

// applyMaxLineLength runs MD013 with the given options on a document.
func applyMaxLineLength(t *testing.T, flavor, input string, options map[string]any) []lint.Diagnostic {
	t.Helper()
	snapshot, err := goldmark.New(flavor).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	var ruleCfg *config.RuleConfig
	if options != nil {
		ruleCfg = &config.RuleConfig{Options: engineOptions("MD013", options)}
	}
	diags, err := NewMaxLineLengthRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), ruleCfg))
	require.NoError(t, err)
	return diags
}

func TestMaxLineLengthRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "short lines",
			input:     "Hello world\nSecond line\n",
			wantDiags: 0,
		},
		{
			name:      "line at the default maximum of 80",
			input:     spacedLine(80) + "\n",
			wantDiags: 0,
		},
		{
			name:      "line exceeds the default maximum of 80",
			input:     spacedLine(90) + "\n",
			wantDiags: 1,
		},
		{
			name:      "multiple long lines",
			input:     spacedLine(130) + "\n\n" + spacedLine(125) + "\n",
			wantDiags: 2,
		},
		{
			name:      "custom max length",
			input:     spacedLine(65) + "\n",
			wantDiags: 1,
			config:    map[string]any{"max": 60},
		},
		{
			// Like markdownlint, a final run without spaces may cross the limit.
			name:      "long final word is allowed",
			input:     "Short start " + strings.Repeat("a", 100) + "\n",
			wantDiags: 0,
		},
		{
			name:      "long final word is reported when strict",
			input:     "Short start " + strings.Repeat("a", 100) + "\n",
			wantDiags: 1,
			config:    map[string]any{"strict": true},
		},
		{
			name:      "trailing URL is allowed",
			input:     "Check out this link: https://example.com/very/long/path/that/exceeds/the/maximum/line/length/limit/significantly\n",
			wantDiags: 0,
		},
		{
			name:      "text after a URL is reported by default",
			input:     "See https://example.com/a/very/long/path/that/goes/on/and/on for the details about this topic.\n",
			wantDiags: 1,
		},
		{
			name:      "ignore_urls skips lines with URLs",
			input:     "See https://example.com/a/very/long/path/that/goes/on/and/on for the details about this topic.\n",
			wantDiags: 0,
			config:    map[string]any{"ignore_urls": true},
		},
		{
			name:      "code blocks are checked by default",
			input:     "```\n" + spacedLine(100) + "\n```\n",
			wantDiags: 1,
		},
		{
			name:      "code blocks can be ignored",
			input:     "```\n" + spacedLine(100) + "\n```\n",
			wantDiags: 0,
			config:    map[string]any{"ignore_code_blocks": true},
		},
		{
			name:      "headings are checked by default",
			input:     "# " + spacedLine(100) + "\n",
			wantDiags: 1,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "blank lines",
			input:     "\n\n\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyMaxLineLength(t, goldmark.FlavorCommonMark, tt.input, tt.config)
			assert.Len(t, diags, tt.wantDiags)
			for _, d := range diags {
				assert.Contains(t, d.Message, "exceeds maximum")
			}
		})
	}
}

// TestMaxLineLengthRule_MarkdownlintOptions checks that markdownlint's option
// names configure the rule like their gomdlint equivalents.
func TestMaxLineLengthRule_MarkdownlintOptions(t *testing.T) {
	tests := []struct {
		name      string
		flavor    string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "line_length",
			input:     spacedLine(45) + "\n",
			wantDiags: 1,
			config:    map[string]any{"line_length": 40},
		},
		{
			name:      "max wins over line_length",
			input:     spacedLine(45) + "\n",
			wantDiags: 0,
			config:    map[string]any{"line_length": 40, "max": 50},
		},
		{
			name:      "heading_line_length",
			input:     "# " + spacedLine(60) + "\n",
			wantDiags: 0,
			config:    map[string]any{"line_length": 40, "heading_line_length": 70},
		},
		{
			name:      "code_block_line_length",
			input:     "```\n" + spacedLine(60) + "\n```\n",
			wantDiags: 0,
			config:    map[string]any{"line_length": 40, "code_block_line_length": 70},
		},
		// markdownlint reads each length as "value || default", so 0 means
		// the default: 80 for the line length, and the line length for
		// headings and code blocks.
		{
			name:      "line_length of 0 means 80",
			input:     spacedLine(85) + "\n",
			wantDiags: 1,
			config:    map[string]any{"line_length": 0},
		},
		{
			name:      "max of 0 means 80",
			input:     spacedLine(85) + "\n",
			wantDiags: 1,
			config:    map[string]any{"max": 0},
		},
		{
			name:      "heading_line_length of 0 means the line length",
			input:     "# " + spacedLine(60) + "\n",
			wantDiags: 1,
			config:    map[string]any{"line_length": 40, "heading_line_length": 0},
		},
		{
			name:      "code_block_line_length of 0 means the line length",
			input:     "```\n" + spacedLine(60) + "\n```\n",
			wantDiags: 1,
			config:    map[string]any{"line_length": 40, "code_block_line_length": 0},
		},
		{
			name:      "heading_max of 0 means the line length",
			input:     "# " + spacedLine(60) + "\n",
			wantDiags: 1,
			config:    map[string]any{"max": 40, "heading_max": 0},
		},
		{
			name:      "negative line_length checks nothing",
			input:     spacedLine(85) + "\n",
			wantDiags: 0,
			config:    map[string]any{"line_length": -1},
		},
		{
			name:      "code_blocks false",
			input:     "```\n" + spacedLine(100) + "\n```\n",
			wantDiags: 0,
			config:    map[string]any{"code_blocks": false},
		},
		{
			name:      "headings false",
			input:     "# " + spacedLine(100) + "\n",
			wantDiags: 0,
			config:    map[string]any{"headings": false},
		},
		{
			name:      "tables false",
			flavor:    goldmark.FlavorGFM,
			input:     "| " + spacedLine(100) + " |\n| --- |\n| cell |\n",
			wantDiags: 0,
			config:    map[string]any{"tables": false},
		},
		{
			name:      "tables are checked by default",
			flavor:    goldmark.FlavorGFM,
			input:     "| " + spacedLine(100) + " |\n| --- |\n| cell |\n",
			wantDiags: 1,
		},
		{
			name:      "line holding only an image is not reported",
			input:     "![" + spacedLine(90) + "](image.png)\n",
			wantDiags: 0,
		},
		{
			name:      "line holding only a link is not reported",
			input:     "[" + spacedLine(90) + "](https://example.com)\n",
			wantDiags: 0,
		},
		{
			name:      "line holding an image inside emphasis is not reported",
			input:     "*![" + spacedLine(90) + "](image.png)*\n",
			wantDiags: 0,
		},
		{
			// The space between the links is text, so the line is not
			// a link-only line.
			name:      "line holding two links is reported",
			input:     "[" + spacedLine(90) + "](https://example.com/a) [b](c)\n",
			wantDiags: 1,
		},
		{
			name:      "line holding a link and text is reported",
			input:     "See [" + spacedLine(90) + "](https://example.com/a)\n",
			wantDiags: 1,
		},
		{
			name:      "title line of a reference definition is not reported",
			input:     "[id]: /target\n    \"" + spacedLine(90) + "\"\n\nSee [id].\n",
			wantDiags: 0,
		},
		{
			name:      "destination line of a reference definition is not reported",
			input:     "[id]:\n/" + strings.Repeat("segment/", 12) + "end \"title\"\n\nSee [id].\n",
			wantDiags: 0,
		},
		{
			name:      "label line of a reference definition is not reported",
			input:     "[id\n " + strings.Repeat("long label ", 10) + "]: /url\n",
			wantDiags: 0,
		},
		{
			name:      "label of 999 characters at most makes a definition",
			input:     "[x\n" + strings.Repeat(" "+strings.Repeat("long label ", 10)+"\n", 8) + "]: /url\n",
			wantDiags: 0,
		},
		{
			// A longer label is not a label, so the lines are paragraph text.
			name:      "label of more than 999 characters makes no definition",
			input:     "[x\n" + strings.Repeat(" "+strings.Repeat("long label ", 10)+"\n", 10) + "]: /url\n",
			wantDiags: 10,
		},
		{
			name:      "reference definition in a blockquote is not reported",
			input:     "> [id]: /target\n>     \"" + spacedLine(90) + "\"\n>\n> See [id].\n",
			wantDiags: 0,
		},
		{
			// A definition cannot interrupt a paragraph, so this is text.
			name:      "paragraph line that looks like a definition is reported",
			input:     "Text\n[id]: /target " + spacedLine(90) + "\n",
			wantDiags: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor := tt.flavor
			if flavor == "" {
				flavor = goldmark.FlavorCommonMark
			}
			assert.Len(t, applyMaxLineLength(t, flavor, tt.input, tt.config), tt.wantDiags)
		})
	}
}

// TestMaxLineLengthRule_CharacterLength checks that lines are measured in
// characters (UTF-16 code units, like markdownlint) rather than bytes.
func TestMaxLineLengthRule_CharacterLength(t *testing.T) {
	strict := map[string]any{"strict": true}

	assert.Empty(t, applyMaxLineLength(t, goldmark.FlavorCommonMark, strings.Repeat("中文", 24)+"\n", strict),
		"48 CJK characters fit in 80 columns")

	diags := applyMaxLineLength(t, goldmark.FlavorCommonMark, strings.Repeat("中文", 45)+"\n", strict)
	require.Len(t, diags, 1)
	assert.Equal(t, "Line length 90 exceeds maximum 80", diags[0].Message)
	assert.Equal(t, 81*3-2, diags[0].StartColumn, "column is the byte column of the first character past the limit")

	nonCompatible := map[string]any{"markdownlint_compatible": false}
	assert.Empty(t, applyMaxLineLength(t, goldmark.FlavorCommonMark, strings.Repeat("é", 70)+"\n", nonCompatible),
		"70 two-byte characters fit in 80 columns")

	diags = applyMaxLineLength(t, goldmark.FlavorCommonMark, strings.Repeat("😀 ", 30)+"\n", nonCompatible)
	require.Len(t, diags, 1)
	assert.Equal(t, "Line length 90 exceeds maximum 80", diags[0].Message, "an emoji counts as two UTF-16 code units")
}

// TestMaxLineLengthRule_IllFormedUTF8 measures lines with ill-formed UTF-8
// as markdownlint does: Node decodes each maximal ill-formed sequence to one
// U+FFFD, one UTF-16 code unit.
func TestMaxLineLengthRule_IllFormedUTF8(t *testing.T) {
	// A truncated three-byte character is one code unit, so this line is 80.
	fits := "\xe2\x82" + strings.Repeat("a", 77) + " x"
	assert.Empty(t, applyMaxLineLength(t, goldmark.FlavorCommonMark, fits+"\n", nil),
		"markdownlint measures 80 code units and reports nothing")

	long := "\xe2\x82" + strings.Repeat("a", 79) + " x"
	diags := applyMaxLineLength(t, goldmark.FlavorCommonMark, long+"\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, "Line length 82 exceeds maximum 80", diags[0].Message)
	assert.Equal(t, len("\xe2\x82")+79+1, diags[0].StartColumn,
		"the finding starts at code unit 80 from 0, the space after the a characters")
	assert.Zero(t, diags[0].StartColumnUTF16Offset)
}

// TestMaxLineLengthRule_LimitInsideASurrogatePair covers a limit that falls
// in the middle of an emoji. markdownlint reports the code unit after the
// limit, the second half of the emoji's surrogate pair. A byte column cannot
// point there, so the finding starts at the emoji and records one code unit
// into it.
func TestMaxLineLengthRule_LimitInsideASurrogatePair(t *testing.T) {
	for name, ending := range map[string]string{"LF": "\n", "CRLF": "\r\n", "CR": "\r"} {
		line := strings.Repeat("a", 79) + "\U0001F600 more"
		diags := applyMaxLineLength(t, goldmark.FlavorCommonMark, line+ending, nil)
		require.Len(t, diags, 1, name)
		assert.Equal(t, "Line length 86 exceeds maximum 80", diags[0].Message, name)
		assert.Equal(t, 80, diags[0].StartColumn, "%s: byte column of the emoji", name)
		assert.Equal(t, 1, diags[0].StartColumnUTF16Offset, "%s: one code unit into the emoji", name)
		assert.Equal(t, len(line)+1, diags[0].EndColumn, name)
	}

	// The wrap fix still breaks at whitespace, never inside the emoji.
	wrapped := strings.Repeat("abc ", 19) + "abc\U0001F600 more words here"
	diags := applyMaxLineLength(t, goldmark.FlavorCommonMark, wrapped+"\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].StartColumnUTF16Offset)
	require.NotEmpty(t, diags[0].FixEdits, "the paragraph line wraps")
	for _, edit := range diags[0].FixEdits {
		for _, offset := range []int{edit.StartOffset, edit.EndOffset} {
			assert.True(t, offset == len(wrapped) || utf8.RuneStart(wrapped[offset]),
				"fix offset %d starts a character", offset)
		}
	}

	// The limit at the start of the emoji needs no offset.
	line := strings.Repeat("a", 80) + "\U0001F600 more"
	diags = applyMaxLineLength(t, goldmark.FlavorCommonMark, line+"\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, 81, diags[0].StartColumn)
	assert.Zero(t, diags[0].StartColumnUTF16Offset)
}

// TestMaxLineLengthRule_DefinitionLabelInUTF16 covers the 999 limit on a
// reference definition's label, which micromark, and so markdownlint,
// counts in UTF-16 code units. A label of 500 emoji is 1000 code units, so
// the line is not a definition and markdownlint reports its length. A label
// of 499 emoji is a definition, which markdownlint never reports.
func TestMaxLineLengthRule_DefinitionLabelInUTF16(t *testing.T) {
	tooLong := "[" + strings.Repeat("\U0001F600", 500) + "]: /url\n"
	diags := applyMaxLineLength(t, goldmark.FlavorCommonMark, tooLong, nil)
	require.Len(t, diags, 1, "a 1000 code unit label does not make a definition")

	fits := "[" + strings.Repeat("\U0001F600", 499) + "]: /url\n"
	assert.Empty(t, applyMaxLineLength(t, goldmark.FlavorCommonMark, fits, nil),
		"a 998 code unit label makes a definition, which is exempt")

	illFormed := "[" + strings.Repeat("\xe2\x82", 999) + "]: /url\n"
	assert.Empty(t, applyMaxLineLength(t, goldmark.FlavorCommonMark, illFormed, nil),
		"each truncated character is one code unit, so 999 of them fit")
}

func TestMaxLineLengthRule_Metadata(t *testing.T) {
	rule := NewMaxLineLengthRule()

	assert.Equal(t, "MD013", rule.ID())
	assert.Equal(t, "line-length", rule.Name())
	assert.Contains(t, rule.Tags(), "line_length")
	assert.True(t, rule.CanFix()) // Auto-fixable via line wrapping.
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestMaxLineLengthRule_DiagnosticPosition(t *testing.T) {
	diags := applyMaxLineLength(t, goldmark.FlavorCommonMark, spacedLine(130)+"\n", nil)
	require.Len(t, diags, 1)

	// Position should start at column 81 (the first character past the limit)
	// and, like every span, end exclusively, one past the last character.
	assert.Equal(t, 1, diags[0].StartLine)
	assert.Equal(t, 81, diags[0].StartColumn)
	assert.Equal(t, 1, diags[0].EndLine)
	assert.Equal(t, 131, diags[0].EndColumn)

	// Multibyte text: the end column is the byte column after the line.
	diags = applyMaxLineLength(t, goldmark.FlavorCommonMark, strings.Repeat("é ", 50)+"\n", nil)
	require.Len(t, diags, 1)
	assert.Equal(t, len(strings.Repeat("é ", 50))+1, diags[0].EndColumn)
}

// maxLineLengthAutofixCase is one MD013 wrap fix scenario.
type maxLineLengthAutofixCase struct {
	name     string
	flavor   string
	input    string
	maxLen   int
	options  map[string]any
	wantFix  bool
	wantText string // expected text after applying fix
}

// runMaxLineLengthAutofix applies the MD013 fixes for a scenario and checks
// the fixed text and that no fixable diagnostic is left.
func runMaxLineLengthAutofix(t *testing.T, tests []maxLineLengthAutofixCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor := tt.flavor
			if flavor == "" {
				flavor = goldmark.FlavorCommonMark
			}
			options := map[string]any{"max": tt.maxLen}
			for key, value := range tt.options {
				options[key] = value
			}
			diags := applyMaxLineLength(t, flavor, tt.input, options)

			var edits []fix.TextEdit
			for _, diag := range diags {
				edits = append(edits, diag.FixEdits...)
			}
			assert.Equal(t, tt.wantFix, len(edits) > 0, "fix presence mismatch")

			prepared, err := fix.PrepareEdits(edits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantText, string(fixed), "fixed text mismatch")

			// The fixed text has no fixable line length diagnostics left.
			assert.Empty(t, filterFixableDiags(applyMaxLineLength(t, flavor, string(fixed), options)))
		})
	}
}

func TestMaxLineLengthRule_Autofix(t *testing.T) {
	runMaxLineLengthAutofix(t, []maxLineLengthAutofixCase{
		{
			name:     "paragraph wraps at word boundary",
			input:    "This is a very long line that exceeds the maximum allowed length for a line in this markdown file.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "This is a very long line that exceeds the maximum\nallowed length for a line in this markdown file.\n",
		},
		{
			name:     "single long word is not reported",
			input:    strings.Repeat("a", 60) + "\n",
			maxLen:   50,
			wantFix:  false,
			wantText: strings.Repeat("a", 60) + "\n",
		},
		{
			name:     "heading - skipped",
			input:    "# This is a very long heading that exceeds the maximum allowed length for a line\n",
			maxLen:   50,
			wantFix:  false,
			wantText: "# This is a very long heading that exceeds the maximum allowed length for a line\n",
		},
		{
			name:     "table line - skipped",
			input:    "| Column 1 | Column 2 | Column 3 | Column 4 | Column 5 | Column 6 |\n",
			maxLen:   50,
			wantFix:  false,
			wantText: "| Column 1 | Column 2 | Column 3 | Column 4 | Column 5 | Column 6 |\n",
		},
		{
			name:     "list item wraps with indent",
			input:    "- This is a list item with a very long description that exceeds the line limit.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "- This is a list item with a very long description\n  that exceeds the line limit.\n",
		},
		{
			name:     "numbered list wraps with indent",
			input:    "1. This is a numbered list item that has quite a long description here.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "1. This is a numbered list item that has quite a\n   long description here.\n",
		},
		{
			name:     "blockquote wraps preserving prefix",
			input:    "> This is a blockquote that is too long and needs to be wrapped at a word boundary.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "> This is a blockquote that is too long and needs\n> to be wrapped at a word boundary.\n",
		},
		{
			name:     "nested blockquote and list",
			input:    "> - This is a nested list item in a blockquote that exceeds the limit.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "> - This is a nested list item in a blockquote\n>   that exceeds the limit.\n",
		},
		{
			name:     "list item with wide marker spacing continues at its content column",
			input:    "-   This is a list item with a very long description that exceeds the limit.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "-   This is a list item with a very long\n    description that exceeds the limit.\n",
		},
		{
			name:     "continuation line of a quoted list item keeps the item indentation",
			input:    "> - First line of a quoted item\n>   and a continuation line that is long enough to wrap here.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "> - First line of a quoted item\n>   and a continuation line that is long enough to\n>   wrap here.\n",
		},
		{
			name:     "nested list item wraps at its own content column",
			input:    "- Parent\n  - This nested list item is long enough that it has to be wrapped.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "- Parent\n  - This nested list item is long enough that it\n    has to be wrapped.\n",
		},
		{
			name:     "indented content wraps with same indent",
			input:    "  This is indented content that is very long and needs wrapping.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "  This is indented content that is very long and\n  needs wrapping.\n",
		},
		{
			name:    "long line is wrapped into several lines at once",
			input:   "Plain text runs on" + strings.Repeat(" and on", 21) + ".\n",
			maxLen:  40,
			wantFix: true,
			wantText: "Plain text runs on and on and on and on\n" +
				"and on and on and on and on and on and\n" +
				"on and on and on and on and on and on\n" +
				"and on and on and on and on and on and\n" +
				"on and on.\n",
		},
		{
			name:     "gomdlint measuring mode wraps too",
			input:    "This is a very long line that exceeds the maximum allowed length for a line in this markdown file.\n",
			maxLen:   50,
			options:  map[string]any{"markdownlint_compatible": false},
			wantFix:  true,
			wantText: "This is a very long line that exceeds the maximum\nallowed length for a line in this markdown file.\n",
		},
	})
}

// TestMaxLineLengthRule_AutofixSafety covers the breaks the wrap fix never
// makes and the lines it leaves alone.
func TestMaxLineLengthRule_AutofixSafety(t *testing.T) {
	runMaxLineLengthAutofix(t, []maxLineLengthAutofixCase{
		{
			name:     "never starts a line with a list marker",
			input:    "Plain text runs on until it is clear - yes it is over now.\n",
			maxLen:   36,
			wantFix:  true,
			wantText: "Plain text runs on until it is\nclear - yes it is over now.\n",
		},
		{
			name:     "never starts a line with a heading marker",
			input:    "Plain text runs on until it uses # hashes here and on.\n",
			maxLen:   32,
			wantFix:  true,
			wantText: "Plain text runs on until it\nuses # hashes here and on.\n",
		},
		{
			name:     "never starts a line with an ordered list marker",
			input:    "Plain text runs on until it says 1. not a list here.\n",
			maxLen:   32,
			wantFix:  true,
			wantText: "Plain text runs on until it\nsays 1. not a list here.\n",
		},
		{
			name:     "never starts a line with a blockquote marker",
			input:    "Plain text runs on until it says > quoted here and on.\n",
			maxLen:   32,
			wantFix:  true,
			wantText: "Plain text runs on until it\nsays > quoted here and on.\n",
		},
		{
			name:     "never starts a line with a code fence",
			input:    "Plain text runs on until it has ``` fences here and on.\n",
			maxLen:   31,
			wantFix:  true,
			wantText: "Plain text runs on until it\nhas ``` fences here and on.\n",
		},
		{
			name:     "never starts a line with a thematic break",
			input:    "Plain text runs on until it shows *** stars here.\n",
			maxLen:   33,
			wantFix:  true,
			wantText: "Plain text runs on until it\nshows *** stars here.\n",
		},
		{
			// Without the closing ">" the text is not a tag, but at the
			// start of a line it could begin an HTML block.
			name:     "never starts a line with an HTML block",
			input:    "Plain text runs on until it has <div tags in it here.\n",
			maxLen:   31,
			wantFix:  true,
			wantText: "Plain text runs on until it\nhas <div tags in it here.\n",
		},
		{
			name:     "never starts a line with a pipe",
			input:    "Plain text runs on until it has | pipes in it here.\n",
			maxLen:   31,
			wantFix:  true,
			wantText: "Plain text runs on until it\nhas | pipes in it here.\n",
		},
		{
			name:     "never starts a line with a plus sign",
			input:    "Plain text runs on until it has + plus signs in here.\n",
			maxLen:   31,
			wantFix:  true,
			wantText: "Plain text runs on until it\nhas + plus signs in here.\n",
		},
		{
			name:     "never starts a line with a setext underline",
			input:    "Plain text runs on until it has = equals signs in here.\n",
			maxLen:   31,
			wantFix:  true,
			wantText: "Plain text runs on until it\nhas = equals signs in here.\n",
		},
		{
			name:     "never breaks after a backslash",
			input:    "Plain text runs on until it ends\\ with a backslash here.\n",
			maxLen:   33,
			wantFix:  true,
			wantText: "Plain text runs on until it\nends\\ with a backslash here.\n",
		},
		{
			name:     "never breaks inside a code span",
			input:    "Some words before `code span with spaces` and after it.\n",
			maxLen:   30,
			wantFix:  true,
			wantText: "Some words before\n`code span with spaces` and\nafter it.\n",
		},
		{
			name:     "never breaks inside a link destination or title",
			input:    "Read [docs](https://example.com/a \"a long title here\") now.\n",
			maxLen:   50,
			wantFix:  true,
			wantText: "Read\n[docs](https://example.com/a \"a long title here\")\nnow.\n",
		},
		{
			// A break between the brackets would turn the checkbox into
			// bracket text, and no other break makes the line shorter.
			name:     "never breaks inside a task list checkbox",
			flavor:   goldmark.FlavorGFM,
			input:    "- [ ] task details are long enough to wrap\n",
			maxLen:   4,
			wantFix:  false,
			wantText: "- [ ] task details are long enough to wrap\n",
		},
		{
			// CommonMark reads "[ ]" as text, but GFM renderers show a
			// checkbox.
			name:     "never breaks inside a task list checkbox in CommonMark",
			input:    "- [ ] task details are long enough to wrap\n",
			maxLen:   4,
			wantFix:  false,
			wantText: "- [ ] task details are long enough to wrap\n",
		},
		{
			// The text of a textarea keeps its line breaks.
			name:     "never wraps inside an inline textarea",
			input:    "Before <textarea>alpha beta gamma delta epsilon zeta eta theta iota kappa lambda</textarea> after.\n",
			maxLen:   80,
			wantFix:  false,
			wantText: "Before <textarea>alpha beta gamma delta epsilon zeta eta theta iota kappa lambda</textarea> after.\n",
		},
		{
			name:     "no fix for a paragraph with inline HTML",
			input:    "Some words come first and fill the line <b>bold words here</b> after.\n",
			maxLen:   50,
			wantFix:  false,
			wantText: "Some words come first and fill the line <b>bold words here</b> after.\n",
		},
		{
			name:     "no fix for a textarea that holds literal tags",
			input:    "Before <b><textarea>alpha </b> beta gamma delta epsilon zeta.</textarea></b> after.\n",
			maxLen:   40,
			wantFix:  false,
			wantText: "Before <b><textarea>alpha </b> beta gamma delta epsilon zeta.</textarea></b> after.\n",
		},
		{
			name:     "no fix for a self-closed textarea",
			input:    "Before <textarea/>alpha beta gamma delta epsilon zeta eta theta.</textarea> after.\n",
			maxLen:   40,
			wantFix:  false,
			wantText: "Before <textarea/>alpha beta gamma delta epsilon zeta eta theta.</textarea> after.\n",
		},
		{
			name: "no fix for a paragraph before inline HTML",
			input: "Some words come first and fill the line with plain words after.\n\n" +
				"Some words come first and fill the line <b>bold words here</b> after.\n",
			maxLen:  50,
			wantFix: false,
			wantText: "Some words come first and fill the line with plain words after.\n\n" +
				"Some words come first and fill the line <b>bold words here</b> after.\n",
		},
		{
			// A style sheet can make line breaks show anywhere.
			name:     "no fix before a style element",
			input:    "alpha beta gamma delta epsilon\n\n<style>p { white-space: pre }</style>\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "alpha beta gamma delta epsilon\n\n<style>p { white-space: pre }</style>\n",
		},
		{
			name:     "no fix with a style sheet link",
			input:    "alpha beta gamma delta epsilon\n\n<link rel=stylesheet href=\"a.css\">\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "alpha beta gamma delta epsilon\n\n<link rel=stylesheet href=\"a.css\">\n",
		},
		{
			// An event handler can set styles on the whole document.
			name:     "no fix before an image with an error handler",
			input:    "alpha beta gamma delta epsilon\n\n<img src=\"data:image/png,broken\" onerror=\"document.body.style.whiteSpace='pre'\">\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "alpha beta gamma delta epsilon\n\n<img src=\"data:image/png,broken\" onerror=\"document.body.style.whiteSpace='pre'\">\n",
		},
		{
			name:     "wraps next to a directive comment and an image",
			input:    "<!-- markdownlint-disable MD033 -->\n<img src=\"x.png\" alt=\"a b\">\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  true,
			wantText: "<!-- markdownlint-disable MD033 -->\n<img src=\"x.png\" alt=\"a b\">\n\nalpha beta gamma\ndelta epsilon\n",
		},
		{
			name: "no fix for a paragraph after inline HTML",
			input: "Some words come first and fill the line <b>bold words here</b> after.\n\n" +
				"Some words come first and fill the line with plain words after.\n",
			maxLen:  50,
			wantFix: false,
			wantText: "Some words come first and fill the line <b>bold words here</b> after.\n\n" +
				"Some words come first and fill the line with plain words after.\n",
		},
		{
			name:     "no fix inside a styled div left open",
			input:    "<div style=\"white-space: pre\">\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "<div style=\"white-space: pre\">\n\nalpha beta gamma delta epsilon\n",
		},
		{
			name:     "no fix in a textarea after a lookalike end tag",
			input:    "Open <textarea>here </textareax>\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "Open <textarea>here </textareax>\n\nalpha beta gamma delta epsilon\n",
		},
		{
			name:     "no fix after a pre whose title holds its end tag",
			input:    "<pre title=\"</pre>\">\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "<pre title=\"</pre>\">\n\nalpha beta gamma delta epsilon\n",
		},
		{
			name:     "no fix after a tag in a malformed comment",
			input:    "<!-- a --!> <pre> -->\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "<!-- a --!> <pre> -->\n\nalpha beta gamma delta epsilon\n",
		},
		{
			name:     "wraps after void elements only",
			input:    "Line<br> more\n\n<img src=\"a.png\">\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  true,
			wantText: "Line<br> more\n\n<img src=\"a.png\">\n\nalpha beta gamma\ndelta epsilon\n",
		},
		{
			// GitHub writes the code element after the document, where the
			// browser opens it again around the next footnote.
			name:     "no fix in a footnote after a footnote with raw HTML",
			flavor:   goldmark.FlavorGFM,
			input:    "See [^1] and [^2].\n\n[^1]: <code>\n\n[^2]: alpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "See [^1] and [^2].\n\n[^1]: <code>\n\n[^2]: alpha beta gamma delta epsilon\n",
		},
		{
			name:     "wraps a footnote after a footnote with a line break element",
			flavor:   goldmark.FlavorGFM,
			input:    "See [^1] and [^2].\n\n[^1]: a<br>b\n\n[^2]: alpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  true,
			wantText: "See [^1] and [^2].\n\n[^1]: a<br>b\n\n[^2]: alpha beta\ngamma delta epsilon\n",
		},
		{
			// GitHub renders TOML front matter as Markdown, where the code
			// element holds the rest of the page.
			name:     "no fix after TOML front matter with raw HTML",
			input:    "+++\ntitle = \"<code>open\"\n+++\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "+++\ntitle = \"<code>open\"\n+++\n\nalpha beta gamma delta epsilon\n",
		},
		{
			name:     "wraps after TOML front matter without HTML",
			input:    "+++\ntitle = \"open\"\n+++\n\nalpha beta gamma delta epsilon\n",
			maxLen:   20,
			wantFix:  true,
			wantText: "+++\ntitle = \"open\"\n+++\n\nalpha beta gamma\ndelta epsilon\n",
		},
		{
			name:     "no fix inside code",
			input:    "```\nthis is code that is long and must not wrap at all\n```\n",
			maxLen:   30,
			wantFix:  false,
			wantText: "```\nthis is code that is long and must not wrap at all\n```\n",
		},
		{
			name:     "no fix for a pipe-less table row",
			flavor:   goldmark.FlavorGFM,
			input:    "Col A | Col B with lots of text | Col C and more text\n--- | --- | ---\na | b | c\n",
			maxLen:   30,
			wantFix:  false,
			wantText: "Col A | Col B with lots of text | Col C and more text\n--- | --- | ---\na | b | c\n",
		},
		{
			// In CommonMark the rows are a paragraph, but wrapping them would
			// still break the table for GFM renderers.
			name:     "no fix for table-like paragraphs",
			input:    "Col A | Col B with lots of text | Col C and more text\n--- | --- | ---\na | b | c\n",
			maxLen:   30,
			wantFix:  false,
			wantText: "Col A | Col B with lots of text | Col C and more text\n--- | --- | ---\na | b | c\n",
		},
		{
			name:     "no fix when every break is unsafe",
			input:    "Unbreakable - - - - - - - - - - - - - - - - - - - - - - end.\n",
			maxLen:   20,
			wantFix:  false,
			wantText: "Unbreakable - - - - - - - - - - - - - - - - - - - - - - end.\n",
		},
	})
}

// TestMaxLineLengthRule_ProtectsTaskCheckboxes checks that the wrap fix never
// breaks between the brackets of a task list checkbox, in either flavor.
func TestMaxLineLengthRule_ProtectsTaskCheckboxes(t *testing.T) {
	for _, flavor := range []string{goldmark.FlavorCommonMark, goldmark.FlavorGFM} {
		for _, checkbox := range []string{"[ ]", "[x]", "[X]"} {
			input := "- " + checkbox + " task\n"
			snapshot, err := goldmark.New(flavor).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			ranges := noBreakRanges(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
			assert.Contains(t, ranges[1], offsetRange{start: 2, end: 5}, "%s %q", flavor, checkbox)
		}
	}
}

func TestMaxLineLengthRule_HelperFunctions(t *testing.T) {
	t.Run("linePrefix for plain text", func(t *testing.T) {
		prefix, start := linePrefix("Hello world")
		assert.Empty(t, prefix)
		assert.Equal(t, 0, start)
	})

	t.Run("linePrefix for indented text", func(t *testing.T) {
		prefix, start := linePrefix("  Hello world")
		assert.Equal(t, "  ", prefix)
		assert.Equal(t, 2, start)
	})

	t.Run("linePrefix for list item", func(t *testing.T) {
		prefix, start := linePrefix("- List item")
		assert.Equal(t, "  ", prefix)
		assert.Equal(t, 2, start)
	})

	t.Run("linePrefix for numbered list", func(t *testing.T) {
		prefix, start := linePrefix("1. Numbered item")
		assert.Equal(t, "   ", prefix)
		assert.Equal(t, 3, start)
	})

	t.Run("linePrefix for blockquote", func(t *testing.T) {
		prefix, start := linePrefix("> Quoted text")
		assert.Equal(t, "> ", prefix)
		assert.Equal(t, 2, start)
	})

	t.Run("linePrefix for nested blockquote and list", func(t *testing.T) {
		prefix, start := linePrefix("> - Nested item")
		assert.Equal(t, ">   ", prefix)
		assert.Equal(t, 4, start)
	})

	t.Run("wrapContinuationStartsBlock detects block starts", func(t *testing.T) {
		for _, rest := range []string{
			"# heading", "> quote", "- item", "+ item", "* item", "= underline", "1. item", "12) item",
			"```js", "~~~", "***", "___", "<div>", "</p>", "<!-- c -->", "| cell", ":--- | ---",
		} {
			assert.True(t, wrapContinuationStartsBlock(rest), rest)
		}
		for _, rest := range []string{
			"word", "*emphasis*", "_emphasis_", "1.5 million", "2024 was", "<3 hearts", "[link](x)", "``code``",
		} {
			assert.False(t, wrapContinuationStartsBlock(rest), rest)
		}
	})

	t.Run("isTableLine identifies tables", func(t *testing.T) {
		assert.True(t, isTableLine("| cell |"))
		assert.True(t, isTableLine("  | indented |"))
		assert.False(t, isTableLine("Not a table"))
		assert.False(t, isTableLine(""))
	})
}
