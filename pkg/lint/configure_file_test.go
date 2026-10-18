package lint_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// The tests in this file cover markdownlint-configure-file comments that
// markdownlint 0.41.1, run by markdownlint-cli2 0.23.3, reads leniently. It
// parses a comment's parameter with markdownlint-cli2's parsers, JSONC,
// then TOML, then YAML, and spreads what the first that succeeds reads over
// its configuration (configureFile in lib/markdownlint.mjs). A parameter
// that no parser reads changes nothing, a value other than an object reads
// as an object without keys, and a key that names no rule configures
// nothing. None of them is an error. Each expected value is what
// markdownlint-cli2 0.23.3 reports for the document.

// configuredDoc is a document that starts with a configure-file comment
// with parameter, and ends with a trailing space on line 5, which
// no-trailing-spaces (MD009) reports unless the comment turns it off.
func configuredDoc(parameter string) string {
	return "<!-- markdownlint-configure-file " + parameter + " -->\n\n# Title\n\ntext \n"
}

// TestEngineConfigureFileReadsJSONCAndYAML covers parameters that
// markdownlint-cli2's JSONC parser reads, with comments or a trailing
// comma, and parameters that only its YAML parser reads. Each turns MD009
// off without a notice.
func TestEngineConfigureFileReadsJSONCAndYAML(t *testing.T) {
	for _, parameter := range []string{
		`{"MD009": false,}`,
		`{ "MD009": false /* off */ }`,
		`{"MD009": false} // off`,
		`{ MD009: false }`,
		`MD009: false`,
	} {
		t.Run(parameter, func(t *testing.T) {
			result := lintDefault(t, configuredDoc(parameter))

			assert.Empty(t, reportedLines(t, configuredDoc(parameter), "MD009"))
			assert.Empty(t, result.Notices)
		})
	}
}

// TestEngineConfigureFileIgnoresWhatNoParserReads covers parameters that
// neither JSON with comments nor YAML reads. The comment changes nothing,
// so MD009 reports, and it gets a notice with the JSON error.
func TestEngineConfigureFileIgnoresWhatNoParserReads(t *testing.T) {
	tests := []struct {
		parameter, problem string
	}{
		{parameter: `{"MD009": false`, problem: "unexpected end of JSON input"},
		{parameter: `{"MD009": false} trailing`, problem: "invalid character 't' after top-level value"},
		{parameter: `{"MD009": false,,}`, problem: "invalid character ',' looking for beginning of object key string"},
	}

	for _, tt := range tests {
		t.Run(tt.parameter, func(t *testing.T) {
			result := lintDefault(t, configuredDoc(tt.parameter))

			assert.Equal(t, []int{5}, reportedLines(t, configuredDoc(tt.parameter), "MD009"))
			assert.Equal(t, []lint.Notice{{
				Kind:      lint.NoticeInvalidConfiguration,
				Line:      1,
				Directive: "markdownlint-configure-file",
				Message:   "markdownlint-configure-file: invalid JSON (" + tt.problem + "); it is ignored",
			}}, result.Notices)
		})
	}
}

// TestEngineConfigureFileIgnoresValuesOtherThanAnObject covers parameters
// that hold no object. Each changes nothing, so MD009 reports. A value of
// another type gets a notice, and null, nothing and an empty object get
// none.
func TestEngineConfigureFileIgnoresValuesOtherThanAnObject(t *testing.T) {
	tests := []struct {
		parameter, held string
	}{
		{parameter: `[1]`, held: "a list"},
		{parameter: `"MD009"`, held: "a string"},
		{parameter: `42`, held: "a number"},
		{parameter: `false`, held: "a boolean"},
		{parameter: `- MD009`, held: "a list"},
		{parameter: `null`},
		{parameter: ``},
		{parameter: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.parameter, func(t *testing.T) {
			result := lintDefault(t, configuredDoc(tt.parameter))

			assert.Equal(t, []int{5}, reportedLines(t, configuredDoc(tt.parameter), "MD009"))
			if tt.held == "" {
				assert.Empty(t, result.Notices)
				return
			}
			assert.Equal(t, []lint.Notice{{
				Kind:      lint.NoticeInvalidConfiguration,
				Line:      1,
				Directive: "markdownlint-configure-file",
				Message:   "markdownlint-configure-file: " + tt.held + " instead of an object of settings; it is ignored",
			}}, result.Notices)
		})
	}
}

// TestEngineConfigureFileIgnoresUnknownKeys covers keys that name no rule
// or tag. markdownlint ignores each and applies the other keys, so MD009
// is off. Each unknown key gets a notice that offers the closest rules.
func TestEngineConfigureFileIgnoresUnknownKeys(t *testing.T) {
	parameter := `{"MD999": false, "no-trailling-spaces": true, "MD009": false}`

	result := lintDefault(t, configuredDoc(parameter))

	assert.Empty(t, reportedLines(t, configuredDoc(parameter), "MD009"))
	require.Len(t, result.Notices, 2)
	assert.Equal(t, lint.Notice{
		Kind:      lint.NoticeUnknownRule,
		Line:      1,
		Directive: "markdownlint-configure-file",
		Name:      "MD999",
		Message:   `markdownlint-configure-file: unknown rule "MD999"; it is ignored`,
	}, result.Notices[0])
	assert.Equal(t, lint.Notice{
		Kind:        lint.NoticeUnknownRule,
		Line:        1,
		Directive:   "markdownlint-configure-file",
		Name:        "no-trailling-spaces",
		Suggestions: []string{"no-trailing-spaces"},
		Message:     `markdownlint-configure-file: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
	}, result.Notices[1])
}

// TestEngineConfigureFileNoticesUnknownOptions covers options that the rule
// a configure-file key names does not read, which markdownlint ignores, as
// gomdlint does. Each gets a notice that names the rule as markdownlint
// names it and offers the closest option names. MD009 still reports.
func TestEngineConfigureFileNoticesUnknownOptions(t *testing.T) {
	tests := []struct {
		name, parameter string
		want            lint.Notice
	}{
		{
			name: "typo", parameter: `{"MD009": {"br_spcaes": 0}}`,
			want: lint.Notice{
				Name: "br_spcaes", Suggestions: []string{"br_spaces"}, Rules: []string{"no-trailing-spaces"},
				Message: `unknown option "br_spcaes" for no-trailing-spaces (did you mean "br_spaces"?); it is ignored`,
			},
		},
		{
			name: "markdownlint's name of the rule", parameter: `{"MD009": true, "unordered-list-style": {"styel": "dash"}}`,
			want: lint.Notice{
				Name: "styel", Suggestions: []string{"style"}, Rules: []string{"ul-style"},
				Message: `unknown option "styel" for ul-style (did you mean "style"?); it is ignored`,
			},
		},
		{
			name: "a rule without options", parameter: `{"MD028": {"strict": true}}`,
			want: lint.Notice{
				Name: "strict", Rules: []string{"no-blanks-blockquote"},
				Message: `unknown option "strict" for no-blanks-blockquote (it has no options); it is ignored`,
			},
		},
		{
			name: "no rule of a tag reads it", parameter: `{"whitespace": {"br_spaces": 3, "br_spcaes": 0}}`,
			want: lint.Notice{
				Name: "br_spcaes",
				Rules: []string{
					"no-trailing-spaces", "no-hard-tabs", "no-multiple-blanks", "no-multiple-space-blockquote",
					"no-blanks-blockquote", "list-marker-space", "no-space-in-emphasis", "no-space-in-code",
					"no-space-in-links",
				},
				Message: `unknown option "br_spcaes" for the rules of the tag "whitespace"; it is ignored`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lintDefault(t, configuredDoc(tt.parameter))

			want := tt.want
			want.Kind = lint.NoticeUnknownOption
			want.Line = 1
			want.Directive = "markdownlint-configure-file"
			want.Message = "markdownlint-configure-file: " + want.Message
			assert.Equal(t, []lint.Notice{want}, result.Notices)
		})
	}
}

// TestEngineConfigureFileUnknownOptionsChangeNothing covers a setting with
// a known option and an unknown one: markdownlint applies the known one and
// ignores the other, so the findings are those of the setting without it.
// markdownlint reports the line of 60 characters with a line length of 40.
func TestEngineConfigureFileUnknownOptionsChangeNothing(t *testing.T) {
	document := func(parameter string) string {
		return "<!-- markdownlint-configure-file " + parameter + " -->\n\n# Title\n\n" +
			"This line has many words and it keeps going well past forty.\n"
	}
	noticed := document(`{"MD013": {"line_length": 40, "lin_length": 100}}`)
	quiet := document(`{"MD013": {"line_length": 40}}`)

	assert.Contains(t, reportedLines(t, noticed, "MD013"), 5)
	assert.Equal(t, diagnosticSummary(lintDefault(t, quiet).Diagnostics), diagnosticSummary(lintDefault(t, noticed).Diagnostics))
	require.Len(t, lintDefault(t, noticed).Notices, 1)
	assert.Empty(t, lintDefault(t, quiet).Notices)
}

// TestEngineConfigureFileReadsMarkdownlintsSpelling covers a setting that
// writes an option in markdownlint's spelling and in gomdlint's, which a
// config file rejects. markdownlint knows its own spelling only, so it
// reads that and ignores the other, and gomdlint does the same, with a
// notice. gomdlint used to fail the file. markdownlint reports the long
// lines with a line length of 20, and the table row only in the first
// document, where gomdlint's ignore_tables is ignored in favour of
// markdownlint's inverted tables.
func TestEngineConfigureFileReadsMarkdownlintsSpelling(t *testing.T) {
	document := func(parameter string) string {
		return "<!-- markdownlint-configure-file " + parameter + " -->\n\n# Title\n\n" +
			"| a | b |\n| - | - |\n| one table row that is long enough | x |\n\n" +
			"This paragraph line is long enough to pass twenty.\n"
	}
	tests := []struct {
		parameter, ignored, message string
		lines                       []int
	}{
		{
			parameter: `{"MD013": {"line_length": 20, "max": 100}}`, ignored: "max", lines: []int{1, 7, 9},
			message: `"line_length" and "max" both set line-length option "max"; ` +
				`markdownlint reads "line_length", so "max" is ignored`,
		},
		{
			parameter: `{"MD013": {"line_length": 20, "tables": false, "ignore_tables": false}}`,
			ignored:   "ignore_tables", lines: []int{1, 9},
			message: `"ignore_tables" and "tables" both set line-length option "ignore_tables"; ` +
				`markdownlint reads "tables", so "ignore_tables" is ignored`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.parameter, func(t *testing.T) {
			result := lintDefault(t, document(tt.parameter))

			assert.Equal(t, tt.lines, reportedLines(t, document(tt.parameter), "MD013"))
			assert.Equal(t, []lint.Notice{{
				Kind:      lint.NoticeOptionConflict,
				Line:      1,
				Directive: "markdownlint-configure-file",
				Name:      tt.ignored,
				Rules:     []string{"line-length"},
				Message:   "markdownlint-configure-file: " + tt.message,
			}}, result.Notices)
		})
	}
}

// TestEngineConfigureFileAppliesTheCommentsItReads covers a file with two
// configure-file comments, of which markdownlint ignores the first, which
// it cannot read, and applies the second, which turns MD009 off.
func TestEngineConfigureFileAppliesTheCommentsItReads(t *testing.T) {
	input := "<!-- markdownlint-configure-file {\"MD013\": false -->\n" +
		"<!-- markdownlint-configure-file {\"MD009\": false} -->\n\n# Title\n\n" +
		"This line has many words and it keeps going well past the eighty character limit of the rule here.\n\ntext \n"

	result := lintDefault(t, input)

	assert.Equal(t, []int{6}, reportedLines(t, input, "MD013"))
	assert.Empty(t, reportedLines(t, input, "MD009"))
	require.Len(t, result.Notices, 1)
	assert.Equal(t, lint.NoticeInvalidConfiguration, result.Notices[0].Kind)
	assert.Equal(t, 1, result.Notices[0].Line)
}

// TestEngineConfigureFileNoticesChangeNoFindings covers every notice of a
// configure-file comment: the findings, and their fixes, are those of the
// same document with the comment's ignored parts left out.
func TestEngineConfigureFileNoticesChangeNoFindings(t *testing.T) {
	for _, tt := range []struct{ noticed, quiet string }{
		{noticed: `{"MD009": false`, quiet: `{}`},
		{noticed: `[1]`, quiet: `{}`},
		{noticed: `{"MD999": false, "MD013": false}`, quiet: `{"MD013": false}`},
	} {
		t.Run(tt.noticed, func(t *testing.T) {
			noticed := lintDefault(t, configuredDoc(tt.noticed))
			quiet := lintDefault(t, configuredDoc(tt.quiet))

			require.NotEmpty(t, noticed.Notices)
			assert.Empty(t, quiet.Notices)
			assert.Equal(t, diagnosticSummary(quiet.Diagnostics), diagnosticSummary(noticed.Diagnostics))
		})
	}
}

// TestEngineConfigureFileCommentsWithoutTheirEndLintInLinearTime covers
// configure-file comments that lack their closing "-->", so that each one's
// parameter runs to the one "-->" at the end of the file and holds every
// later comment. markdownlint reads each such parameter in full, but none
// is valid JSON, since the next comment's "<!--" stands outside any string,
// and reading stops there. gomdlint used to fail the file at the first
// comment. Copying or scanning each parameter whole takes time that grows
// with the square of the number of comments: about 11 seconds for 10,000
// and 43 for these 20,000. The race detector slows everything down several
// times, so it gets fewer comments and more time.
func TestEngineConfigureFileCommentsWithoutTheirEndLintInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("long document")
	}
	comments, limit := 20000, 5*time.Second
	if raceEnabled {
		comments, limit = 5000, 30*time.Second
	}
	var input strings.Builder
	input.WriteString("# Title\n\n")
	for idx := range comments {
		fmt.Fprintf(&input, "<!-- markdownlint-configure-file {\"MD0%02d\": false}\n\nText %d\n\n", idx%60, idx)
	}
	input.WriteString("-->\n")

	start := time.Now()
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), lint.DefaultRegistry).LintFile(
		context.Background(), "doc.md", []byte(input.String()), config.NewConfig())
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Len(t, result.Notices, comments, "each comment that markdownlint cannot read gets a notice")
	assert.Less(t, elapsed, limit, "configure-file comments must take time in proportion to the file")
}

// diagnosticSummary describes each diagnostic by its rule, its position
// and whether it has a fix, which documents of different lengths on line 1
// share.
func diagnosticSummary(diagnostics []lint.Diagnostic) []string {
	var summary []string
	for _, diag := range diagnostics {
		if diag.StartLine == 1 {
			continue // The comment's own line differs in length.
		}
		summary = append(summary, fmt.Sprintf("%s line %d fixes %d", diag.RuleID, diag.StartLine, len(diag.FixEdits)))
	}
	return summary
}
