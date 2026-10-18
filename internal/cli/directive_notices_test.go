package cli_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// The markdownlint comments of noticeDoc, which get notices, and the
// comments of quietDoc, which take their places and get none.
const (
	// typoComment names a rule with a typo, so it silences nothing.
	typoComment = "<!-- markdownlint-disable no-trailling-spaces -->"

	// plainComment is a comment of typoComment's length that configures
	// nothing, so the findings keep their offsets.
	plainComment = "<!-- notdirective-disable no-trailling-spaces -->"

	// formerTagComment names gomdlint's former tag lists, which still
	// silences the rule it covers.
	formerTagComment = "<!-- markdownlint-disable lists -->"

	// ruleComment names the rule that lists covers by markdownlint's name
	// for it, as its notice advises.
	ruleComment = "<!-- markdownlint-disable ul-style -->"
)

// noticeDocument is a document with a trailing space on line 5, which a
// comment on line 3 may silence, and a list on line 11 whose bullet style
// a comment on line 7 may silence.
func noticeDocument(line3, line7 string) string {
	return "# Notices\n\n" + line3 + "\n\nTrailing space \n\n" + line7 + "\n\n- one\n\n* two\n"
}

var (
	noticeDoc = noticeDocument(typoComment, formerTagComment)
	quietDoc  = noticeDocument(plainComment, ruleComment)

	// asNoticeDoc rewrites output about quietDoc, such as a diff that shows
	// its comments, into the output expected for noticeDoc.
	asNoticeDoc = strings.NewReplacer(plainComment, typoComment, ruleComment, formerTagComment).Replace
)

// wantNoticeWarnings are the warnings that linting noticeDoc as doc.md writes
// to stderr.
var wantNoticeWarnings = []string{
	`WARN doc.md:3: markdownlint-disable: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
	`WARN doc.md:7: markdownlint-disable: deprecated tag "lists" (write its rules instead: ul-style); ` +
		`it will stop working in a later release`,
}

// TestLint_DirectiveNoticesChangeNoFindings lints a document whose
// markdownlint comments get notices, and the same document with comments
// that get none in their places, in every human format and under --strict.
// The notices go to stderr. Findings, the rest of the output and the exit
// code are the same for both documents: a word that names no rule does
// nothing, as in markdownlint, and a former tag still silences the rule
// that its notice names.
func TestLint_DirectiveNoticesChangeNoFindings(t *testing.T) {
	for _, flags := range [][]string{
		{"--format", "text"},
		{"--strict"},
		{"--format", "table"},
		{"--format", "summary"},
		{"--format", "diff"},
		{"--fix", "--dry-run"},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			args := append([]string{"lint", "--color", "never"}, append(flags, "doc.md")...)

			inProject(t, map[string]string{"doc.md": quietDoc})
			want := execute(t, args...)
			require.Empty(t, want.stderr)
			require.NotEmpty(t, want.stdout)

			inProject(t, map[string]string{"doc.md": noticeDoc})
			got := execute(t, args...)

			assert.Equal(t, want.code, got.code, "notices never change the exit code")
			assert.Equal(t, asNoticeDoc(want.stdout), got.stdout, "notices never change the findings")
			assert.Equal(t, strings.Join(wantNoticeWarnings, "\n")+"\n", got.stderr)
		})
	}
}

// TestLint_DirectiveNoticesPassStrict covers --strict, which fails a run on
// warning findings. A notice is not a finding, so a file whose only problem
// is a notice still passes.
func TestLint_DirectiveNoticesPassStrict(t *testing.T) {
	inProject(t, map[string]string{"clean.md": "# Clean\n\n<!-- markdownlint-disable frobnicate -->\n"})

	got := execute(t, "lint", "--color", "never", "--strict", "clean.md")

	assert.Equal(t, cli.ExitSuccess, got.code, "stderr: %s", got.stderr)
	assert.Contains(t, got.stdout, "No issues found")
	assert.Equal(t, `WARN clean.md:3: markdownlint-disable: unknown rule "frobnicate"; it is ignored`+"\n", got.stderr)
}

// jsonNotice is a notice in JSON output.
type jsonNotice struct {
	Line        int      `json:"line"`
	Kind        string   `json:"kind"`
	Directive   string   `json:"directive"`
	Name        string   `json:"name"`
	Suggestions []string `json:"suggestions"`
	Rules       []string `json:"rules"`
	Message     string   `json:"message"`
}

// TestLint_JSONCarriesDirectiveNotices covers JSON output: each file lists
// its notices, and its findings and the summary are those of the document
// whose comments get none. The notices also go to stderr, as in every
// format.
func TestLint_JSONCarriesDirectiveNotices(t *testing.T) {
	type report struct {
		Files []struct {
			Path        string          `json:"path"`
			Diagnostics json.RawMessage `json:"diagnostics"`
			Notices     []jsonNotice    `json:"notices"`
		} `json:"files"`
		Summary json.RawMessage `json:"summary"`
	}
	decode := func(t *testing.T, stdout string) report {
		t.Helper()
		var decoded report
		require.NoError(t, json.Unmarshal([]byte(stdout), &decoded), stdout)
		require.Len(t, decoded.Files, 1)
		return decoded
	}

	inProject(t, map[string]string{"doc.md": quietDoc})
	quiet := execute(t, "lint", "--format", "json", "doc.md")
	want := decode(t, quiet.stdout)
	assert.NotContains(t, quiet.stdout, `"notices"`, "a file without notices has no notices key")

	inProject(t, map[string]string{"doc.md": noticeDoc})
	run := execute(t, "lint", "--format", "json", "doc.md")
	got := decode(t, run.stdout)

	assert.Equal(t, quiet.code, run.code)
	assert.JSONEq(t, string(want.Files[0].Diagnostics), string(got.Files[0].Diagnostics))
	assert.JSONEq(t, string(want.Summary), string(got.Summary))
	assert.Equal(t, []jsonNotice{
		{
			Line: 3, Kind: "unknown-rule", Directive: "markdownlint-disable", Name: "no-trailling-spaces",
			Suggestions: []string{"no-trailing-spaces"},
			Message:     `markdownlint-disable: unknown rule "no-trailling-spaces" (did you mean "no-trailing-spaces"?); it is ignored`,
		},
		{
			Line: 7, Kind: "deprecated-tag", Directive: "markdownlint-disable", Name: "lists",
			Rules: []string{"ul-style"},
			Message: `markdownlint-disable: deprecated tag "lists" (write its rules instead: ul-style); ` +
				`it will stop working in a later release`,
		},
	}, got.Files[0].Notices)
	assert.Equal(t, strings.Join(wantNoticeWarnings, "\n")+"\n", run.stderr)
}

// configureFileNoticeDoc holds three markdownlint-configure-file comments
// that markdownlint 0.41.1, run by markdownlint-cli2 0.23.3, ignores, and a
// trailing space on line 7, which it reports: invalid JSON, a list, and a
// key that names no rule. quietConfigureFileDoc holds plain comments of
// the same length in their places.
var (
	configureFileNoticeDoc = "# Notices\n\n" +
		"<!-- markdownlint-configure-file {\"MD009\": false -->\n" +
		"<!-- markdownlint-configure-file [1] -->\n" +
		"<!-- markdownlint-configure-file {\"MD999\": false} -->\n\n" +
		"Trailing space \n"
	quietConfigureFileDoc = strings.ReplaceAll(configureFileNoticeDoc, "markdownlint-", "notdirective-")
)

// wantConfigureFileWarnings are the warnings that linting
// configureFileNoticeDoc as doc.md writes to stderr.
var wantConfigureFileWarnings = []string{
	`WARN doc.md:3: markdownlint-configure-file: invalid JSON (unexpected end of JSON input); it is ignored`,
	`WARN doc.md:4: markdownlint-configure-file: a list instead of an object of settings; it is ignored`,
	`WARN doc.md:5: markdownlint-configure-file: unknown rule "MD999"; it is ignored`,
}

// TestLint_ConfigureFileNoticesChangeNoFindings lints a document whose
// configure-file comments markdownlint ignores, and the same document with
// plain comments in their places, in every human format and under
// --strict. gomdlint used to fail the file. Now the findings, the rest of
// the output and the exit code are the same for both documents, and the
// notices go to stderr.
func TestLint_ConfigureFileNoticesChangeNoFindings(t *testing.T) {
	for _, flags := range [][]string{{"--format", "text"}, {"--strict"}, {"--format", "table"}, {"--format", "summary"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			args := append([]string{"lint", "--color", "never"}, append(flags, "doc.md")...)

			inProject(t, map[string]string{"doc.md": quietConfigureFileDoc})
			want := execute(t, args...)
			require.Empty(t, want.stderr)

			inProject(t, map[string]string{"doc.md": configureFileNoticeDoc})
			got := execute(t, args...)

			assert.Equal(t, want.code, got.code, "notices never change the exit code")
			assert.Equal(t, want.stdout, got.stdout, "notices never change the findings")
			assert.Equal(t, strings.Join(wantConfigureFileWarnings, "\n")+"\n", got.stderr)
		})
	}
}

// TestLint_MachineFormatsCarryConfigureFileNotices covers JSON output, which
// lists the notices of each file under notices, and SARIF output, which
// records them as configuration notifications at their comments' lines.
func TestLint_MachineFormatsCarryConfigureFileNotices(t *testing.T) {
	inProject(t, map[string]string{"doc.md": configureFileNoticeDoc})

	run := execute(t, "lint", "--format", "json", "doc.md")

	var report struct {
		Files []struct {
			Notices []jsonNotice `json:"notices"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(run.stdout), &report), run.stdout)
	require.Len(t, report.Files, 1)
	assert.Equal(t, []jsonNotice{
		{
			Line: 3, Kind: "invalid-configuration", Directive: "markdownlint-configure-file",
			Message: "markdownlint-configure-file: invalid JSON (unexpected end of JSON input); it is ignored",
		},
		{
			Line: 4, Kind: "invalid-configuration", Directive: "markdownlint-configure-file",
			Message: "markdownlint-configure-file: a list instead of an object of settings; it is ignored",
		},
		{
			Line: 5, Kind: "unknown-rule", Directive: "markdownlint-configure-file", Name: "MD999",
			Message: `markdownlint-configure-file: unknown rule "MD999"; it is ignored`,
		},
	}, report.Files[0].Notices)
	assert.NotContains(t, run.stdout, `"name": ""`, "a notice without a name has no name key")

	run = execute(t, "lint", "--format", "sarif", "doc.md")

	var log struct {
		Runs []struct {
			Invocations []struct {
				ToolConfigurationNotifications []struct {
					Message struct {
						Text string `json:"text"`
					} `json:"message"`
					Locations []struct {
						PhysicalLocation struct {
							Region struct {
								StartLine int `json:"startLine"`
							} `json:"region"`
						} `json:"physicalLocation"`
					} `json:"locations"`
				} `json:"toolConfigurationNotifications"`
			} `json:"invocations"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal([]byte(run.stdout), &log), run.stdout)
	require.Len(t, log.Runs, 1)
	require.Len(t, log.Runs[0].Invocations, 1)
	var got []string
	for _, notification := range log.Runs[0].Invocations[0].ToolConfigurationNotifications {
		require.Len(t, notification.Locations, 1)
		got = append(got, fmt.Sprintf("%d: %s", notification.Locations[0].PhysicalLocation.Region.StartLine, notification.Message.Text))
	}
	assert.Equal(t, []string{
		"3: markdownlint-configure-file: invalid JSON (unexpected end of JSON input); it is ignored",
		"4: markdownlint-configure-file: a list instead of an object of settings; it is ignored",
		`5: markdownlint-configure-file: unknown rule "MD999"; it is ignored`,
	}, got)
}
