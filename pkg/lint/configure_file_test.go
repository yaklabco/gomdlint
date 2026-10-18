package lint_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
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
