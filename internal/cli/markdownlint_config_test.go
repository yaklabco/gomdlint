package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/internal/cli"
)

// trailingSpaceDoc has one finding, a trailing space (MD009), whatever
// rules a markdownlint config leaves on.
const trailingSpaceDoc = "# Title\n\ntext \n"

// reportedFiles returns, for each file that a JSON report lists, the IDs of
// the rules that reported it, in order, or nil for a file without findings.
// Paths are written with slashes.
func reportedFiles(t *testing.T, report string) map[string][]string {
	t.Helper()

	var parsed struct {
		Files []struct {
			Path        string `json:"path"`
			Diagnostics []struct {
				RuleID string `json:"ruleId"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	files := make(map[string][]string)
	for _, file := range parsed.Files {
		var ids []string
		for _, diag := range file.Diagnostics {
			ids = append(ids, diag.RuleID)
		}
		slices.Sort(ids)
		files[filepath.ToSlash(file.Path)] = slices.Compact(ids)
	}
	return files
}

// markdownlintModes are the two ways a markdownlint config takes effect:
// translated for each run, or migrated to a gomdlint config first.
var markdownlintModes = []string{"applied for the run", "migrated"}

// lintWithMarkdownlintConfig lints doc.md, which holds trailingSpaceDoc,
// under the markdownlint config name, which holds settings, as mode says:
// translated for the run, or migrated with gomdlint migrate, after which
// the markdownlint config goes, as migrate advises. It returns the rules
// that each file reports and the warnings: the run's for a translated
// config, and migrate's for a migrated one.
func lintWithMarkdownlintConfig(t *testing.T, mode, name, settings string) (map[string][]string, string) {
	t.Helper()

	report, warnings := markdownlintConfigReport(t, mode, name, settings)
	return reportedFiles(t, report), warnings
}

// markdownlintConfigReport is lintWithMarkdownlintConfig, which returns the
// JSON report itself.
func markdownlintConfigReport(t *testing.T, mode, name, settings string) (string, string) {
	t.Helper()

	dir := inProject(t, map[string]string{"doc.md": trailingSpaceDoc, name: settings})
	var warnings string
	if mode == "migrated" {
		migrated := execute(t, "migrate", name, "--output", ".gomdlint.yml")
		require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
		warnings = migrated.stdout
		require.NoError(t, os.Remove(filepath.Join(dir, name)))
	}

	got := execute(t, "lint", "--format", "json", "doc.md")
	require.NotEqual(t, cli.ExitConfigError, got.code, got.stderr)
	if mode != "migrated" {
		warnings = got.stderr
	}
	return got.stdout, warnings
}

// nestedYAMLBlockItems returns a YAML block sequence nested levels deep,
// each item indented two spaces further than the one before, whose
// innermost item is empty.
func nestedYAMLBlockItems(levels int) string {
	var text strings.Builder
	for idx := range levels {
		text.WriteString(strings.Repeat("  ", idx) + "-\n")
	}
	return text.String()
}

// reportedSeverities returns the severities of the findings that a JSON
// report lists for doc.md, in order.
func reportedSeverities(t *testing.T, report string) []string {
	t.Helper()

	var parsed struct {
		Files []struct {
			Diagnostics []struct {
				Severity string `json:"severity"`
			} `json:"diagnostics"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed), report)

	var severities []string
	for _, file := range parsed.Files {
		for _, diag := range file.Diagnostics {
			severities = append(severities, diag.Severity)
		}
	}
	return severities
}

// TestMarkdownlintConfig_KeysAsMarkdownlintReadsThem covers keys beyond
// ASCII in a markdownlint config. markdownlint 0.41.1 compares keys by their
// JavaScript upper case, so the long s of no-trailing-ſpaces reads as S and
// the key names no-trailing-spaces (MD009), while the dotted capital I of
// NO-TRAİLING-SPACES stays as it is, and that key names nothing, though it
// lower-cases to the rule's name. markdownlint reports trailingSpaceDoc's
// trailing space with the second config only.
func TestMarkdownlintConfig_KeysAsMarkdownlintReadsThem(t *testing.T) {
	tests := []struct {
		name, settings string
		want           []string
		warning        string
	}{
		{name: "long s", settings: `{"no-trailing-` + "ſ" + `paces": false}`},
		{
			name: "dotted capital I", settings: `{"NO-TRA` + "İ" + `LING-SPACES": false}`,
			want: []string{"MD009"}, warning: `unknown rule "NO-TRA` + "İ" + `LING-SPACES"`,
		},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, warnings := lintWithMarkdownlintConfig(t, mode, ".markdownlint.json", tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": tt.want}, got)
				if tt.warning == "" {
					assert.NotContains(t, warnings, "unknown rule")
				} else {
					assert.Contains(t, warnings, tt.warning)
				}
			})
		}
	}
}

// TestMarkdownlintConfig_TrailingCommas covers markdownlint configs with
// comments and a comma after their last member, which markdownlint-cli2
// 0.23.3 reads with jsonc-parser and its allowTrailingComma option, in
// .markdownlint.json and .markdownlint.jsonc alike. gomdlint used to stop
// with a config error. markdownlint-cli2 reports trailingSpaceDoc's
// trailing space with neither config.
func TestMarkdownlintConfig_TrailingCommas(t *testing.T) {
	tests := []struct {
		name, file, settings string
	}{
		{name: "JSON", file: ".markdownlint.json", settings: `{"MD009": false,}`},
		{name: "JSONC", file: ".markdownlint.jsonc", settings: "{\n  // Trailing spaces are fine.\n  \"MD009\": false,\n}\n"},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, warnings := lintWithMarkdownlintConfig(t, mode, tt.file, tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": nil}, got)
				assert.NotContains(t, warnings, "WARN")
			})
		}
	}
}

// TestMarkdownlintConfig_ByteOrderMark covers markdownlint configs that
// start with a UTF-8 byte order mark, which markdownlint-cli2 0.23.3 loads
// in every format. gomdlint used to stop with a config error on a JSON
// one. markdownlint-cli2 reports trailingSpaceDoc's trailing space with
// none of them.
func TestMarkdownlintConfig_ByteOrderMark(t *testing.T) {
	tests := []struct {
		name, file, settings string
	}{
		{name: "JSON", file: ".markdownlint.json", settings: "\ufeff{\"MD009\": false}\n"},
		{name: "JSONC", file: ".markdownlint.jsonc", settings: "\ufeff{\"MD009\": false,}\n"},
		{name: "YAML", file: ".markdownlint.yaml", settings: "\ufeffMD009: false\n"},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, warnings := lintWithMarkdownlintConfig(t, mode, tt.file, tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": nil}, got)
				assert.NotContains(t, warnings, "WARN")
			})
		}
	}
}

// TestMarkdownlintConfig_ReadsEveryNameAsMarkdownlintCLI2Does covers
// markdownlint configs whose content does not match their name, which
// markdownlint-cli2 0.23.3 reads whatever the name: with its JSONC parser
// first, then as YAML. A .markdownlint.yaml that holds JSON with a key
// written twice keeps the key's first place and takes its last value, so
// in the second config the whitespace tag, which follows MD009's first
// place, turns no-trailing-spaces (MD009) back on. gomdlint used to reject
// both JSON configs in YAML files, as YAML rejects a key written twice, and
// the YAML in a JSON file. markdownlint-cli2 reports trailingSpaceDoc's
// trailing space with the second config only.
func TestMarkdownlintConfig_ReadsEveryNameAsMarkdownlintCLI2Does(t *testing.T) {
	tests := []struct {
		name, file, settings string
		want                 []string
	}{
		{name: "JSON in YAML", file: ".markdownlint.yaml", settings: `{"MD009": true, "MD009": false}`},
		{
			name: "JSON in YAML first place", file: ".markdownlint.yaml",
			settings: `{"MD009": false, "whitespace": true, "MD009": false}`, want: []string{"MD009"},
		},
		{name: "YAML in JSON", file: ".markdownlint.json", settings: "MD009: false\n"},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, warnings := lintWithMarkdownlintConfig(t, mode, tt.file, tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": tt.want}, got)
				assert.NotContains(t, warnings, "WARN")
			})
		}
	}
}

// TestMarkdownlintConfig_ValuesOtherThanAnObject covers markdownlint
// configs that hold no object: markdownlint-cli2 0.23.3 loads them as an
// object without keys, so every rule runs with its defaults, where
// gomdlint used to stop with a config error. gomdlint warns about a value
// other than an object, but not about an empty file. markdownlint-cli2
// reports trailingSpaceDoc's trailing space with each config.
func TestMarkdownlintConfig_ValuesOtherThanAnObject(t *testing.T) {
	tests := []struct {
		name, file, settings, warning string
	}{
		{name: "list", file: ".markdownlint.json", settings: `["MD009"]`, warning: "a list instead of an object of settings"},
		{name: "empty", file: ".markdownlint.json", settings: ""},
		{name: "YAML list", file: ".markdownlint.yaml", settings: "- MD009\n", warning: "a list instead of an object of settings"},
		{name: "YAML string", file: ".markdownlint.yaml", settings: "MD009\n", warning: "a string instead of an object of settings"},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, warnings := lintWithMarkdownlintConfig(t, mode, tt.file, tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": {"MD009"}}, got)
				if tt.warning == "" {
					assert.NotContains(t, warnings, "WARN")
				} else {
					assert.Contains(t, warnings, tt.warning)
				}
			})
		}
	}
}

// TestMarkdownlintConfig_YAMLAsJSYAMLReadsIt covers YAML that
// markdownlint-cli2 0.23.3 reads with js-yaml 5.4.1's core schema, where
// gomdlint used to decode YAML 1.1 with yaml.v3. 0b0 is a string, which
// turns no-trailing-spaces (MD009) on, and 0o0 is the number 0, which turns
// it off. false after the non-specific tag !, which may follow the anchor
// on the next line, is a string, which turns MD009 on. A numeric key is a
// string, so an option named 0 is one that MD009 does not read, with a
// warning, and the rest of MD009's object applies, where gomdlint used to
// lose the whole object. A %YAML directive of version 1.2 loads, where
// gomdlint used to stop with a config error, and so does a directive whose
// name js-yaml does not know, such as #foo, which it ignores, as it ignores
// a byte order mark at the start of a line after the end marker "...". A
// config nested 100 levels deep loads, as it does on gomdlint's main line.
// A warning names the line of its option, which a key spelled 0x0 and a
// %YAML 1.2 directive used to lose. markdownlint-cli2 reports
// trailingSpaceDoc's trailing space with the configs that turn MD009 on.
func TestMarkdownlintConfig_YAMLAsJSYAMLReadsIt(t *testing.T) {
	tests := []struct {
		name, settings, warning string
		want                    []string
	}{
		{name: "0b0", settings: "MD009: 0b0\n", want: []string{"MD009"}},
		{name: "0o0", settings: "MD009: 0o0\n"},
		{name: "non-specific tag after a line break", settings: "MD009: &a\n  ! false\n", want: []string{"MD009"}},
		{name: "byte order mark after the end marker", settings: "MD009: false\n...\n\ufeff\n"},
		{name: "directive named #foo", settings: "%#foo\n---\nMD009: false\n"},
		{name: "directive name with a no-break space", settings: "%YAML\u00a01.2\n---\nMD009: false\n"},
		{
			name: "block items 100 deep", settings: "MD009: false\nignored:\n" + nestedYAMLBlockItems(99),
			warning: `unknown rule "ignored"`,
		},
		{name: "numeric key", settings: "MD009: {enabled: false, 0: x}\n", warning: `unknown option "0" for no-trailing-spaces`},
		{
			name: "numeric key with its line", settings: "MD009:\n  enabled: false\n  0x0: x\n",
			warning: `.markdownlint.yaml:3: unknown option "0" for no-trailing-spaces`,
		},
		{name: "YAML 1.2", settings: "%YAML 1.2\n---\nMD009: false\n"},
		{
			name: "YAML 1.2 with a warning line", settings: "%YAML 1.2\n---\nMD009:\n  enabled: false\n  br_space: 2\n",
			warning: `.markdownlint.yaml:5: unknown option "br_space" for no-trailing-spaces`,
		},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, warnings := lintWithMarkdownlintConfig(t, mode, ".markdownlint.yaml", tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": tt.want}, got)
				if tt.warning == "" {
					assert.NotContains(t, warnings, "WARN")
				} else {
					assert.Contains(t, warnings, tt.warning)
				}
			})
		}
	}
}

// TestMarkdownlintConfig_YAMLThatJSYAMLRejects covers YAML that js-yaml
// 5.4.1 rejects, so markdownlint-cli2 0.23.3 stops with an error: an unknown
// tag, which gomdlint used to ignore, a tag that the core schema lacks,
// keys that are duplicates once they are strings, a key written twice in a
// list, which gomdlint used to load from a JSON file name as a list that
// holds no settings, and a second document after a byte order mark, which
// js-yaml skips only at the start of a line, as a document starts.
// gomdlint stops with a config error.
func TestMarkdownlintConfig_YAMLThatJSYAMLRejects(t *testing.T) {
	tests := []struct {
		name, file, settings, want string
	}{
		{name: "unknown tag", file: ".markdownlint.yaml", settings: "!unknown {MD009: false}\n", want: "unknown mapping tag !unknown"},
		{name: "binary", file: ".markdownlint.yaml", settings: "MD009: !!binary aGk=\n", want: "unknown scalar tag !!binary"},
		{
			name: "numeric duplicate", file: ".markdownlint.yaml", settings: "0x10: a\n16: b\nMD009: false\n",
			want: `line 2: duplicated mapping key "16"`,
		},
		{name: "duplicate in a list", file: ".markdownlint.json", settings: "- MD009: true\n  MD009: false\n", want: ".markdownlint.json"},
		{
			name: "second document after a byte order mark", file: ".markdownlint.yaml",
			settings: "MD009: false\n...\n\ufeffMD010: false\n", want: ".markdownlint.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inProject(t, map[string]string{"doc.md": trailingSpaceDoc, tt.file: tt.settings})

			got := execute(t, "lint", "doc.md")

			assert.Equal(t, cli.ExitConfigError, got.code)
			assert.Contains(t, got.stderr, tt.want)
		})
	}
}

// TestMarkdownlintConfig_SeverityAsMarkdownlintReadsIt covers the severity
// key of a rule's object in a markdownlint config, which markdownlint
// 0.41.1 reads as "warning" when it is exactly that, and as "error"
// otherwise, so gomdlint's info and a misspelling such as bogus or Warning
// make errors. gomdlint used to keep info, which never fails a run, and to
// reject bogus and Warning with a config error. markdownlint-cli2 reports
// trailingSpaceDoc's trailing space with each config, as a warning with the
// fourth only.
func TestMarkdownlintConfig_SeverityAsMarkdownlintReadsIt(t *testing.T) {
	tests := []struct {
		name, file, settings, want string
	}{
		{name: "info", file: ".markdownlint.json", settings: `{"MD009": {"severity": "info"}}`, want: "error"},
		{name: "bogus", file: ".markdownlint.json", settings: `{"MD009": {"severity": "bogus"}}`, want: "error"},
		{name: "Warning", file: ".markdownlint.json", settings: `{"MD009": {"severity": "Warning"}}`, want: "error"},
		{name: "warning", file: ".markdownlint.json", settings: `{"MD009": {"severity": "warning"}}`, want: "warning"},
		{name: "YAML info", file: ".markdownlint.yaml", settings: "MD009:\n  severity: info\n", want: "error"},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				report, _ := markdownlintConfigReport(t, mode, tt.file, tt.settings)

				assert.Equal(t, []string{tt.want}, reportedSeverities(t, report))
			})
		}
	}

	t.Run("in markdownlint-configure-file", func(t *testing.T) {
		doc := "<!-- markdownlint-configure-file {\"MD009\": {\"severity\": \"info\"}} -->\n\n" + trailingSpaceDoc
		inProject(t, map[string]string{"doc.md": doc})

		got := execute(t, "lint", "--format", "json", "doc.md")

		assert.Equal(t, []string{"error"}, reportedSeverities(t, got.stdout))
	})
}

// TestMarkdownlintConfig_SeverityValueAsMarkdownlintReadsIt covers a rule
// whose value in a markdownlint config is a string, which markdownlint
// 0.41.1 reads as the rule turned on, with warnings when the string is
// exactly "warning" and errors otherwise, so gomdlint's info and a
// misspelling such as bogus or Warning make errors. gomdlint's own config
// files read such a string as a severity of gomdlint's (see
// TestLint_OwnConfigKeepsItsSeverities), which leaves markdownlint configs
// as they are.
func TestMarkdownlintConfig_SeverityValueAsMarkdownlintReadsIt(t *testing.T) {
	tests := []struct {
		name, file, settings, want string
	}{
		{name: "info", file: ".markdownlint.json", settings: `{"MD009": "info"}`, want: "error"},
		{name: "bogus", file: ".markdownlint.json", settings: `{"MD009": "bogus"}`, want: "error"},
		{name: "Warning", file: ".markdownlint.json", settings: `{"MD009": "Warning"}`, want: "error"},
		{name: "error", file: ".markdownlint.json", settings: `{"MD009": "error"}`, want: "error"},
		{name: "warning", file: ".markdownlint.json", settings: `{"no-trailing-spaces": "warning"}`, want: "warning"},
		{name: "YAML info", file: ".markdownlint.yaml", settings: "MD009: info\n", want: "error"},
		{name: "YAML off", file: ".markdownlint.yaml", settings: "MD009: off\n", want: "error"},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				report, _ := markdownlintConfigReport(t, mode, tt.file, tt.settings)

				assert.Equal(t, []string{tt.want}, reportedSeverities(t, report))
			})
		}
	}

	t.Run("in markdownlint-configure-file", func(t *testing.T) {
		for value, want := range map[string]string{"info": "error", "bogus": "error", "warning": "warning"} {
			doc := "<!-- markdownlint-configure-file {\"MD009\": \"" + value + "\"} -->\n\n" + trailingSpaceDoc
			inProject(t, map[string]string{"doc.md": doc})

			got := execute(t, "lint", "--format", "json", "doc.md")

			assert.Equal(t, []string{want}, reportedSeverities(t, got.stdout), value)
		}
	})
}

// TestLint_OwnConfigKeepsItsSeverities covers gomdlint's own config files,
// whose severities are gomdlint's: info stays info, and an unknown
// severity is a config error, under severity or as the rule's value.
func TestLint_OwnConfigKeepsItsSeverities(t *testing.T) {
	for _, settings := range []string{"rules:\n  MD009:\n    severity: info\n", "rules:\n  MD009: info\n"} {
		inProject(t, map[string]string{"doc.md": trailingSpaceDoc, ".gomdlint.yml": settings})
		got := execute(t, "lint", "--format", "json", "doc.md")
		assert.Equal(t, []string{"info"}, reportedSeverities(t, got.stdout), settings)
	}

	for _, settings := range []string{"rules:\n  MD009:\n    severity: bogus\n", "rules:\n  MD009: bogus\n"} {
		inProject(t, map[string]string{"doc.md": trailingSpaceDoc, ".gomdlint.yml": settings})
		got := execute(t, "lint", "--format", "json", "doc.md")
		assert.Equal(t, cli.ExitConfigError, got.code, settings)
		assert.Contains(t, got.stderr, `invalid severity "bogus"`, settings)
	}
}

// TestMarkdownlintConfig_DefaultKeyInAnyCase covers the default key of a
// markdownlint config, which markdownlint 0.41.1 reads in any case, taking
// the first one written. gomdlint used to read only "default", so it warned
// about an unknown rule "Default" and left the rules on. markdownlint
// reports trailingSpaceDoc's trailing space with the last config only.
func TestMarkdownlintConfig_DefaultKeyInAnyCase(t *testing.T) {
	tests := []struct {
		name, file, settings string
		want                 []string
	}{
		{name: "JSON", file: ".markdownlint.json", settings: `{"Default": false}`},
		{name: "YAML", file: ".markdownlint.yaml", settings: "DEFAULT: false\n"},
		{
			name: "the first of two", file: ".markdownlint.json",
			settings: `{"default": true, "Default": false}`, want: []string{"MD009"},
		},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, warnings := lintWithMarkdownlintConfig(t, mode, tt.file, tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": tt.want}, got)
				assert.NotContains(t, warnings, "unknown rule")
			})
		}
	}
}

// TestMarkdownlintConfig_FalsyRuleValues covers rule values that are falsy
// in JavaScript, which markdownlint 0.41.1 reads as turning the rule off,
// in a markdownlint config and in a markdownlint-configure-file comment.
// markdownlint reports trailingSpaceDoc's trailing space with none of them.
func TestMarkdownlintConfig_FalsyRuleValues(t *testing.T) {
	for _, settings := range []string{`{"MD009": 0}`, `{"MD009": ""}`, `{"MD009": {"enabled": null}}`} {
		for _, mode := range markdownlintModes {
			t.Run(settings+" "+mode, func(t *testing.T) {
				got, _ := lintWithMarkdownlintConfig(t, mode, ".markdownlint.json", settings)

				assert.Equal(t, map[string][]string{"doc.md": nil}, got)
			})
		}

		t.Run(settings+" in markdownlint-configure-file", func(t *testing.T) {
			doc := "<!-- markdownlint-configure-file " + settings + " -->\n\n" + trailingSpaceDoc
			inProject(t, map[string]string{"doc.md": doc})

			got := execute(t, "lint", "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, map[string][]string{"doc.md": nil}, reportedFiles(t, got.stdout))
		})
	}
}

// TestMarkdownlintConfig_AppliesKeysInWrittenOrder covers the order in which
// a markdownlint config's keys apply, in JSON and in YAML: the order they
// are written, as in markdownlint 0.41.1, so a later key overrides an
// earlier one for the rules both name. markdownlint reports
// trailingSpaceDoc's trailing space with the second config of each pair
// only, where the rule's key follows the whitespace tag's.
func TestMarkdownlintConfig_AppliesKeysInWrittenOrder(t *testing.T) {
	tests := []struct {
		name, file, settings string
		want                 []string
	}{
		{name: "JSON tag last", file: ".markdownlint.json", settings: `{"MD009": true, "whitespace": false}`},
		{
			name: "JSON rule last", file: ".markdownlint.json",
			settings: `{"whitespace": false, "MD009": true}`, want: []string{"MD009"},
		},
		{name: "YAML tag last", file: ".markdownlint.yaml", settings: "MD009: true\nwhitespace: false\n"},
		{
			name: "YAML rule last", file: ".markdownlint.yaml",
			settings: "whitespace: false\nMD009: true\n", want: []string{"MD009"},
		},
	}

	for _, tt := range tests {
		for _, mode := range markdownlintModes {
			t.Run(tt.name+" "+mode, func(t *testing.T) {
				got, _ := lintWithMarkdownlintConfig(t, mode, tt.file, tt.settings)

				assert.Equal(t, map[string][]string{"doc.md": tt.want}, got)
			})
		}
	}
}

// TestLint_ConfigureFileAppliesKeysInWrittenOrder covers the same order in
// markdownlint-configure-file comments, which act as one object, as in
// markdownlint 0.41.1: a key written again in a later comment keeps its
// first place. markdownlint reports the trailing space in the second and
// third documents only.
func TestLint_ConfigureFileAppliesKeysInWrittenOrder(t *testing.T) {
	tests := []struct {
		name     string
		comments []string
		want     []string
	}{
		{name: "tag last", comments: []string{`{"MD009": true, "whitespace": false}`}},
		{name: "rule last", comments: []string{`{"whitespace": false, "MD009": true}`}, want: []string{"MD009"}},
		{
			name:     "tag written again in a later comment",
			comments: []string{`{"whitespace": false, "MD009": true}`, `{"whitespace": false}`}, want: []string{"MD009"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc strings.Builder
			for _, comment := range tt.comments {
				doc.WriteString("<!-- markdownlint-configure-file " + comment + " -->\n\n")
			}
			doc.WriteString(trailingSpaceDoc)
			inProject(t, map[string]string{"doc.md": doc.String()})

			got := execute(t, "lint", "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, map[string][]string{"doc.md": tt.want}, reportedFiles(t, got.stdout))
		})
	}
}

// TestLint_ConfigureFileReadsAsMarkdownlintCLI2Does covers configure-file
// parameters with a trailing comma, with a comment, or in YAML. markdownlint
// 0.41.1 reads a parameter with the parsers that markdownlint-cli2 0.23.3
// gives it, JSONC first and YAML last, so it applies each, and reports
// trailingSpaceDoc's trailing space with none of them. gomdlint used to
// fail the file.
func TestLint_ConfigureFileReadsAsMarkdownlintCLI2Does(t *testing.T) {
	for _, parameter := range []string{`{"MD009": false,}`, `{"MD009": false /* off */}`, `{ MD009: false }`} {
		t.Run(parameter, func(t *testing.T) {
			doc := "<!-- markdownlint-configure-file " + parameter + " -->\n\n" + trailingSpaceDoc
			inProject(t, map[string]string{"doc.md": doc})

			got := execute(t, "lint", "--format", "json", "doc.md")

			require.Empty(t, got.stderr)
			assert.Equal(t, map[string][]string{"doc.md": nil}, reportedFiles(t, got.stdout))
		})
	}
}

// TestLint_ConfigureFileReadsMarkdownlintsSpelling covers a configure-file
// setting that writes an option in markdownlint's spelling and gomdlint's.
// markdownlint reads its own, line_length, and knows no other, so
// markdownlint-cli2 reports the comment's own line as longer than 10
// characters, and the trailing space. gomdlint used to fail the file. It
// now reads line_length too, with a notice that max is an option that
// markdownlint does not read, which offers markdownlint's spelling.
func TestLint_ConfigureFileReadsMarkdownlintsSpelling(t *testing.T) {
	doc := "<!-- markdownlint-configure-file {\"MD013\": {\"line_length\": 10, \"max\": 100}} -->\n\n" + trailingSpaceDoc
	inProject(t, map[string]string{"doc.md": doc})

	got := execute(t, "lint", "--format", "json", "doc.md")

	assert.Equal(t, map[string][]string{"doc.md": {"MD009", "MD013"}}, reportedFiles(t, got.stdout))
	assert.Equal(t, `WARN doc.md:1: markdownlint-configure-file: unknown option "max" for line-length `+
		`(did you mean "line_length"?); it is ignored`+"\n", got.stderr)
}

// TestMarkdownlintConfig_MarkdownlintCLI2OptionsFile covers a project whose
// markdownlint configuration is the config key of a markdownlint-cli2
// options file, as markdownlint-cli2 0.23.3 reads it. Its config turns
// MD009 off, so markdownlint-cli2 reports nothing in doc.md, and its globs
// configure markdownlint-cli2 itself, which gomdlint does not read and
// warns about, both when it applies the file for a run and when it
// migrates it. gomdlint used to ignore such a file.
func TestMarkdownlintConfig_MarkdownlintCLI2OptionsFile(t *testing.T) {
	const options = "{\n  \"globs\": [\"*.md\"],\n  \"config\": {\"MD009\": false}\n}\n"
	for _, mode := range markdownlintModes {
		t.Run(mode, func(t *testing.T) {
			reported, warnings := lintWithMarkdownlintConfig(t, mode, ".markdownlint-cli2.jsonc", options)

			assert.Equal(t, map[string][]string{"doc.md": nil}, reported)
			assert.Contains(t, warnings,
				`.markdownlint-cli2.jsonc:2: unsupported markdownlint-cli2 option "globs" (name the files on the command line instead); it is ignored`)
		})
	}
}

// TestMigrate_FindsMarkdownlintCLI2OptionsFile covers migrate without an
// input file in a directory whose only markdownlint configuration is an
// options file: migrate finds it and converts its config key. Its other
// keys still configure markdownlint-cli2, so migrate advises deleting the
// config key, not the file.
func TestMigrate_FindsMarkdownlintCLI2OptionsFile(t *testing.T) {
	inProject(t, map[string]string{".markdownlint-cli2.yaml": "config:\n  MD013: false\n"})

	got := execute(t, "migrate")

	require.Equal(t, cli.ExitSuccess, got.code, got.stdout+got.stderr)
	assert.Contains(t, got.stdout, "found markdownlint config")
	assert.Contains(t, got.stdout, ".markdownlint-cli2.yaml")
	assert.Contains(t, got.stdout, "you can now delete the config key of the markdownlint-cli2 options file")
	assert.NotContains(t, got.stdout, "delete the old markdownlint configuration file")
	written, err := os.ReadFile(".gomdlint.yml")
	require.NoError(t, err)
	assert.Contains(t, string(written), "line-length:  # MD013\n    enabled: false")
}

// TestMigrate_OptionsFileWithoutConfig covers migrate in a directory whose
// only markdownlint file is an options file without the config key. It
// holds no markdownlint configuration, so migrate writes one with
// markdownlint's defaults, and has no config key to advise deleting.
func TestMigrate_OptionsFileWithoutConfig(t *testing.T) {
	inProject(t, map[string]string{".markdownlint-cli2.jsonc": `{"globs": ["**/*.md"]}`})

	got := execute(t, "migrate")

	require.Equal(t, cli.ExitSuccess, got.code, got.stdout+got.stderr)
	assert.NotContains(t, got.stdout, "delete")
	assert.Contains(t, got.stdout, "the markdownlint-cli2 options file sets no config key")
}

// TestMigrate_KeepsInheritedSettings covers the settings that a user config
// sets and a markdownlint config cannot: its ignore patterns, flavor,
// gitignore and default_ignores settings and backups. While gomdlint
// translates the markdownlint config for each run they apply, and they
// must still apply once the markdownlint config is migrated. A migrated
// file used to write each of them with its default value, such as
// ignore: [], which hid the user's.
func TestMigrate_KeepsInheritedSettings(t *testing.T) {
	files := map[string]string{
		".xdg/gomdlint/config.yaml": "ignore:\n  - \"drafts/**\"\n" +
			"flavor: commonmark\n" +
			"gitignore: false\n" +
			"default_ignores: false\n" +
			"backups:\n  enabled: false\n",
		".markdownlint.json": `{"MD013": false}` + "\n",
		// The user's pattern skips drafts.
		"drafts/draft.md": trailingSpaceDoc,
		// gitignore: false lints a file that .gitignore lists.
		".gitignore": "ignored.md\n",
		"ignored.md": trailingSpaceDoc,
		// default_ignores: false lints node_modules.
		"node_modules/dep.md": trailingSpaceDoc,
		// Under gfm, blanks-around-tables (MD058) reports the table that
		// follows the paragraph; under commonmark there is no table.
		"table.md": "# Title\n\nText\n| a | b |\n| - | - |\n| 1 | 2 |\n",
	}
	want := map[string][]string{
		"ignored.md":          {"MD009"},
		"node_modules/dep.md": {"MD009"},
		"table.md":            nil,
	}

	dir := inProject(t, files)

	translated := execute(t, "lint", "--format", "json", ".")
	require.Contains(t, translated.stderr, "translated to gomdlint settings for this run")
	require.Equal(t, want, reportedFiles(t, translated.stdout), "the markdownlint config translated for the run")

	migrated := execute(t, "migrate", ".markdownlint.json", "--output", ".gomdlint.yml")
	require.Equal(t, cli.ExitSuccess, migrated.code, migrated.stderr)
	// As migrate advises, the markdownlint config goes.
	require.NoError(t, os.Remove(filepath.Join(dir, ".markdownlint.json")))

	got := execute(t, "lint", "--format", "json", ".")
	require.Empty(t, got.stderr)
	assert.Equal(t, want, reportedFiles(t, got.stdout), "the migrated config")

	fixed := execute(t, "lint", "--fix", "ignored.md")
	require.Equal(t, cli.ExitSuccess, fixed.code, fixed.stderr+fixed.stdout)
	assert.Empty(t, backupFiles(t, dir), "the user config turns backups off")
}
