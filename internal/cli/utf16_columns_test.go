package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// utf16ColumnsDocument puts findings after two-byte letters, emoji (each two
// UTF-16 code units) and a combining mark.
const utf16ColumnsDocument = "# Title\n" +
	"\n" +
	"ÄÖÜ trail   \n" +
	"\n" +
	"\U0001F600 smile   \n" +
	"\n" +
	"\U0001F600\U0001F600 tab\there\n" +
	"\n" +
	"é combining   \n" +
	"\n" +
	"\U0001F600 see https://example.com now\n" +
	"\n" +
	"\U0001F600 (link)[https://example.com]\n" +
	"\n" +
	"\U0001F600 some * spaced * text\n" +
	"\n" +
	"\U0001F600 *a* and _b_ styles\n"

// markdownlintUTF16Findings are the findings markdownlint 0.41.1 reports for
// utf16ColumnsDocument with LF, CRLF and CR line endings alike, as
// "rule line:column-endColumn". markdownlint reports a 1-based column and a
// length counted in UTF-16 code units, and the end column is the column plus
// the length.
var markdownlintUTF16Findings = []string{
	"MD009 3:10-13",
	"MD009 5:9-12",
	"MD010 7:9-10",
	"MD009 9:13-16",
	"MD034 11:8-27",
	"MD011 13:4-31",
	"MD037 15:10-11",
	"MD037 15:17-18",
	"MD049 17:12-13",
	"MD049 17:14-15",
}

// invalidUTF8Document puts findings after ill-formed UTF-8: a truncated
// three-byte character, an encoded surrogate and a truncated emoji. Node,
// and so markdownlint, decodes each maximal ill-formed sequence to one
// U+FFFD.
const invalidUTF8Document = "# Title\n" +
	"\n" +
	"\xe2\x82 trail   \n" +
	"\n" +
	"\xed\xa0\x80 x\there\n" +
	"\n" +
	"\xf0\x9f\x98 y   \n"

// markdownlintInvalidUTF8Findings are the findings markdownlint 0.41.1
// reports for invalidUTF8Document, in the form of markdownlintUTF16Findings.
var markdownlintInvalidUTF8Findings = []string{
	"MD009 3:8-11",
	"MD010 5:6-7",
	"MD009 7:4-7",
}

// TestLint_ColumnsCountUTF16CodeUnitsAsMarkdownlintDoes checks the JSON,
// SARIF, text and table columns of findings after multibyte text, emoji,
// combining marks and ill-formed UTF-8 against markdownlint's, for each line
// ending.
func TestLint_ColumnsCountUTF16CodeUnitsAsMarkdownlintDoes(t *testing.T) {
	t.Parallel()

	documents := map[string]struct {
		content  string
		findings []string
	}{
		"multibyte": {content: utf16ColumnsDocument, findings: markdownlintUTF16Findings},
		"invalid":   {content: invalidUTF8Document, findings: markdownlintInvalidUTF8Findings},
	}
	endings := map[string]string{"LF": "\n", "CRLF": "\r\n", "CR": "\r"}
	for docName, doc := range documents {
		for name, ending := range endings {
			t.Run(docName+" "+name, func(t *testing.T) {
				t.Parallel()

				content := strings.ReplaceAll(doc.content, "\n", ending)
				path := filepath.Join(t.TempDir(), "doc.md")
				require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

				assert.Equal(t, doc.findings, jsonFindings(t, runLint(t, "--format", "json", path)),
					"JSON columns match markdownlint")
				assert.Equal(t, doc.findings, sarifFindings(t, runLint(t, "--format", "sarif", path)),
					"SARIF columns match markdownlint")
				assert.Equal(t, findingStarts(doc.findings),
					textFindings(runLint(t, "--format", "text", "--rule-format", "id", path)),
					"text columns match markdownlint")
				assert.Equal(t, findingStarts(doc.findings),
					textFindings(runLint(t, "--format", "table", "--rule-format", "id", path)),
					"table columns match markdownlint")
			})
		}
	}
}

// comparedRules are the rules whose findings the UTF-16 column test compares.
var comparedRules = []string{"MD009", "MD010", "MD011", "MD034", "MD037", "MD049"}

// jsonFindings returns the findings of the compared rules in a JSON report.
func jsonFindings(t *testing.T, report string) []string {
	t.Helper()

	var parsed struct {
		Files []struct {
			Diagnostics []struct {
				RuleID      string `json:"ruleId"`
				StartLine   int    `json:"startLine"`
				StartColumn int    `json:"startColumn"`
				EndLine     int    `json:"endLine"`
				EndColumn   int    `json:"endColumn"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	var found []string
	for _, file := range parsed.Files {
		for _, diag := range file.Diagnostics {
			if !slices.Contains(comparedRules, diag.RuleID) {
				continue
			}
			assert.Equal(t, diag.StartLine, diag.EndLine, "%s ends on its line", diag.RuleID)
			found = append(found, fmt.Sprintf("%s %d:%d-%d", diag.RuleID, diag.StartLine, diag.StartColumn, diag.EndColumn))
		}
	}
	return found
}

// sarifFindings returns the findings of the compared rules in a SARIF log,
// whose run must declare UTF-16 columns.
func sarifFindings(t *testing.T, log string) []string {
	t.Helper()

	var parsed struct {
		Runs []struct {
			ColumnKind string `json:"columnKind"`
			Results    []struct {
				RuleID    string `json:"ruleId"`
				Locations []struct {
					PhysicalLocation struct {
						Region struct {
							StartLine   int `json:"startLine"`
							StartColumn int `json:"startColumn"`
							EndLine     int `json:"endLine"`
							EndColumn   int `json:"endColumn"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal([]byte(log), &parsed), log)
	require.Len(t, parsed.Runs, 1)
	assert.Equal(t, "utf16CodeUnits", parsed.Runs[0].ColumnKind)

	var found []string
	for _, result := range parsed.Runs[0].Results {
		if !slices.Contains(comparedRules, result.RuleID) {
			continue
		}
		require.Len(t, result.Locations, 1)
		region := result.Locations[0].PhysicalLocation.Region
		assert.Equal(t, region.StartLine, region.EndLine, "%s ends on its line", result.RuleID)
		found = append(found, fmt.Sprintf("%s %d:%d-%d", result.RuleID, region.StartLine, region.StartColumn, region.EndColumn))
	}
	return found
}

// findingStarts returns findings without their end columns, as
// "rule line:column".
func findingStarts(findings []string) []string {
	starts := make([]string, len(findings))
	for idx, finding := range findings {
		starts[idx], _, _ = strings.Cut(finding, "-")
	}
	return starts
}

// textFindings returns the findings of the compared rules in a text or table
// report printed with --rule-format id, as "rule line:column". Both formats
// show only where a finding starts.
func textFindings(report string) []string {
	var found []string
	for line := range strings.SplitSeq(report, "\n") {
		var rule, location string
		for _, field := range strings.Fields(line) {
			if slices.Contains(comparedRules, field) {
				rule = field
			}
			if parts := strings.Split(field, ":"); len(parts) >= 2 && location == "" {
				location = strings.Join(parts[len(parts)-2:], ":")
			}
		}
		if rule != "" && location != "" {
			found = append(found, rule+" "+location)
		}
	}

	return found
}
