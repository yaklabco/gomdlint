package reporter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/reporter"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// sarifFile builds a file outcome whose diagnostics were computed against
// content, as the lint engine produces them: byte columns and byte offsets.
func sarifFile(path, content string, diags ...lint.Diagnostic) runner.FileOutcome {
	for idx := range diags {
		if diags[idx].FilePath == "" {
			diags[idx].FilePath = path
		}
		if diags[idx].Message == "" {
			diags[idx].Message = "Example finding"
		}
	}

	return runner.FileOutcome{
		Path: path,
		Result: &lint.PipelineResult{
			Path: path,
			FileResult: &lint.FileResult{
				Snapshot:    mdast.NewFileSnapshot(path, []byte(content)),
				Diagnostics: diags,
			},
		},
	}
}

// reportSARIF runs the SARIF reporter and returns the raw log after checking
// it against the vendored SARIF 2.1.0 schema.
func reportSARIF(t *testing.T, opts reporter.Options, result *runner.Result) []byte {
	t.Helper()

	var buf bytes.Buffer
	opts.Writer = &buf
	opts.Format = reporter.FormatSARIF

	_, err := reporter.NewSARIFReporter(opts).Report(context.Background(), result)
	require.NoError(t, err)

	violations := loadSARIFValidator(t).validate(decodeJSON(t, buf.Bytes()))
	require.Empty(t, violations, "SARIF output must satisfy the SARIF 2.1.0 schema")

	return buf.Bytes()
}

// decodeSARIF reports result as SARIF, validates it, and decodes it.
func decodeSARIF(t *testing.T, opts reporter.Options, result *runner.Result) reporter.SARIFOutput {
	t.Helper()

	var log reporter.SARIFOutput
	require.NoError(t, json.Unmarshal(reportSARIF(t, opts, result), &log))
	require.Len(t, log.Runs, 1)

	return log
}

// newGitWorkTree returns a temporary directory marked as the root of a git
// work tree. A linked worktree or a submodule marks its root with a .git file
// rather than a directory, so both forms are supported.
func newGitWorkTree(t *testing.T, gitFile bool) string {
	t.Helper()

	root := t.TempDir()
	marker := filepath.Join(root, ".git")
	if gitFile {
		require.NoError(t, os.WriteFile(marker, []byte("gitdir: /elsewhere/.git/worktrees/x\n"), 0o644))
	} else {
		require.NoError(t, os.Mkdir(marker, 0o755))
	}

	return root
}

// fileURIPrefix returns the file URI of an absolute directory whose name needs
// no percent-encoding, which holds for the temporary directories tests use.
func fileURIPrefix(dir string) string {
	return "file://" + filepath.ToSlash(dir)
}

func resultLocations(run reporter.SARIFRun) []reporter.SARIFArtifactLocation {
	locations := make([]reporter.SARIFArtifactLocation, 0, len(run.Results))
	for _, result := range run.Results {
		locations = append(locations, result.Locations[0].PhysicalLocation.ArtifactLocation)
	}
	return locations
}

func TestSARIFReporter_OutputMatchesSchema(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")

	result := &runner.Result{
		Files: []runner.FileOutcome{
			sarifFile(filepath.Join(workDir, "docs", "umlaut.md"), umlautContent, lint.Diagnostic{
				RuleID: "MD009", RuleName: "no-trailing-spaces", Severity: config.SeverityWarning,
				StartLine: 3, StartColumn: 13, EndLine: 3, EndColumn: 16,
				Suggestion: "Remove trailing whitespace",
				FixEdits:   []fix.TextEdit{{StartOffset: 21, EndOffset: 24}},
			}),
			sarifFile(filepath.Join(workDir, "docs", "emoji \U0001F600 #1.md"), "\U0001F600 x\n",
				lint.Diagnostic{RuleID: "MD047", Severity: config.SeverityError},
				lint.Diagnostic{RuleID: "MD010", Severity: config.SeverityInfo, StartLine: 1, StartColumn: 6, EndLine: 1, EndColumn: 2},
				lint.Diagnostic{
					RuleID: "MD022", StartLine: 1, StartColumn: 1, EndLine: 2, EndColumn: 1,
					Suggestion: "Insert blank lines",
					FixEdits:   []fix.TextEdit{{StartOffset: 0, EndOffset: 0, NewText: "\n"}, {StartOffset: 0, EndOffset: 0, NewText: "\n"}},
				},
			),
			sarifFile(filepath.Join(workDir, "trailing.md"), "a\n\n\n", lint.Diagnostic{
				RuleID: "MD012", StartLine: 2, StartColumn: 1, EndLine: 4, EndColumn: 1,
				Suggestion: "Remove extra blank lines",
				FixEdits:   []fix.TextEdit{{StartOffset: 2, EndOffset: 4}},
			}),
			sarifFile(outside, "# Outside\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 10}),
			{Path: filepath.Join(workDir, "locked.md"), Error: errors.New("permission denied")},
		},
	}

	reportSARIF(t, reporter.Options{WorkingDir: workDir}, result)
}

func TestSARIFReporter_ArtifactURIsRelativeToRepositoryRoot(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	workDir := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	outside := filepath.Join(t.TempDir(), "elsewhere.md")

	diag := lint.Diagnostic{RuleID: "MD041", StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2}
	result := &runner.Result{
		Files: []runner.FileOutcome{
			sarifFile(filepath.Join(workDir, "guide.md"), "x\n", diag),
			sarifFile(filepath.Join(root, "top.md"), "x\n", diag),
			sarifFile(filepath.Join(workDir, "a b#1é.md"), "x\n", diag),
			sarifFile(filepath.Join(workDir, "a:b.md"), "x\n", diag),
			sarifFile(outside, "x\n", diag),
		},
	}

	run := decodeSARIF(t, reporter.Options{WorkingDir: workDir}, result).Runs[0]

	assert.Equal(t, map[string]reporter.SARIFArtifactLocation{
		"%SRCROOT%": {URI: fileURIPrefix(root) + "/"},
	}, run.OriginalURIBaseIDs, "the source root is the enclosing git work tree")

	assert.Equal(t, []reporter.SARIFArtifactLocation{
		{URI: "sub/guide.md", URIBaseID: "%SRCROOT%"},
		{URI: "top.md", URIBaseID: "%SRCROOT%"},
		{URI: "sub/a%20b%231%C3%A9.md", URIBaseID: "%SRCROOT%"},
		{URI: "sub/a:b.md", URIBaseID: "%SRCROOT%"},
		{URI: fileURIPrefix(outside)},
	}, resultLocations(run))
}

func TestSARIFReporter_SymlinkedFilesUseTheirTargetInsideTheRoot(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "real"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "real", "page.md"), []byte("x\n"), 0o644))
	outsideTarget := filepath.Join(t.TempDir(), "external.md")
	require.NoError(t, os.WriteFile(outsideTarget, []byte("x\n"), 0o644))

	links := map[string]string{
		filepath.Join(root, "README.md"):          filepath.Join("docs", "guide.md"),
		filepath.Join(root, "linked"):             filepath.Join("docs", "real"),
		filepath.Join(root, "docs", "outside.md"): outsideTarget,
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Skip("cannot create symlinks:", err)
		}
	}

	diag := lint.Diagnostic{RuleID: "MD041", StartLine: 1}
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "README.md"), "x\n", diag),
		sarifFile(filepath.Join(root, "linked", "page.md"), "x\n", diag),
		sarifFile(filepath.Join(root, "docs", "outside.md"), "x\n", diag),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]

	// GitHub code scanning cannot display a result through a symlink, so a
	// link resolves to its target when the target is in the source tree.
	assert.Equal(t, []reporter.SARIFArtifactLocation{
		{URI: "docs/guide.md", URIBaseID: "%SRCROOT%"},
		{URI: "docs/real/page.md", URIBaseID: "%SRCROOT%"},
		{URI: "docs/outside.md", URIBaseID: "%SRCROOT%"},
	}, resultLocations(run))
}

func TestSARIFReporter_GitFileMarksSourceRoot(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, true)
	workDir := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(workDir, "a.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: workDir}, result).Runs[0]

	assert.Equal(t, fileURIPrefix(root)+"/", run.OriginalURIBaseIDs["%SRCROOT%"].URI)
	assert.Equal(t, []reporter.SARIFArtifactLocation{{URI: "docs/a.md", URIBaseID: "%SRCROOT%"}}, resultLocations(run))
}

func TestSARIFReporter_SourceRootDefaultsToWorkingDir(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	for dir := workDir; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			t.Skipf("temporary directory %s is inside a git work tree", workDir)
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(workDir, "docs", "a.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: workDir}, result).Runs[0]

	assert.Equal(t, fileURIPrefix(workDir)+"/", run.OriginalURIBaseIDs["%SRCROOT%"].URI)
	assert.Equal(t, []reporter.SARIFArtifactLocation{{URI: "docs/a.md", URIBaseID: "%SRCROOT%"}}, resultLocations(run))
}

func TestSARIFReporter_WithoutWorkingDir(t *testing.T) {
	t.Parallel()

	absolute := filepath.Join(t.TempDir(), "abs.md")
	diag := lint.Diagnostic{RuleID: "MD041", StartLine: 1}
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(absolute, "x\n", diag),
		sarifFile(filepath.Join("docs", "rel.md"), "x\n", diag),
	}}

	run := decodeSARIF(t, reporter.Options{}, result).Runs[0]

	assert.Empty(t, run.OriginalURIBaseIDs, "no base URI is known without a working directory")
	assert.Equal(t, []reporter.SARIFArtifactLocation{
		{URI: fileURIPrefix(absolute)},
		{URI: "docs/rel.md", URIBaseID: "%SRCROOT%"},
	}, resultLocations(run))
}

func TestSARIFReporter_RegionsCountUTF16CodeUnits(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	content := "# Title\n\nÄÖÜ trail   \n\U0001F600 x\n"
	bomContent := "\xef\xbb\xbf# Tïtle   \n"

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "regions.md"), content,
			// Multibyte letters: byte columns 13 to 16 are UTF-16 columns 10 to 13.
			lint.Diagnostic{RuleID: "MD009", StartLine: 3, StartColumn: 13, EndLine: 3, EndColumn: 16},
			// An astral character is four bytes but two UTF-16 code units.
			lint.Diagnostic{RuleID: "MD010", StartLine: 4, StartColumn: 6, EndLine: 4, EndColumn: 7},
			// No position at all: the result covers the whole file.
			lint.Diagnostic{RuleID: "MD047"},
			// An end before the start collapses to an insertion point at the start.
			lint.Diagnostic{RuleID: "MD022", StartLine: 3, StartColumn: 5, EndLine: 3, EndColumn: 2},
			// The empty line after the final newline does not exist in SARIF.
			lint.Diagnostic{RuleID: "MD012", StartLine: 5, StartColumn: 1, EndLine: 5, EndColumn: 1},
			// A start without an end keeps the end absent.
			lint.Diagnostic{RuleID: "MD034", StartLine: 3, StartColumn: 13},
		),
		sarifFile(filepath.Join(root, "bom.md"), bomContent,
			lint.Diagnostic{RuleID: "MD009", StartLine: 1, StartColumn: 12, EndLine: 1, EndColumn: 15},
		),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]

	assert.Equal(t, "utf16CodeUnits", run.ColumnKind)

	regions := make([]*reporter.SARIFRegion, 0, len(run.Results))
	for _, res := range run.Results {
		regions = append(regions, res.Locations[0].PhysicalLocation.Region)
	}

	assert.Equal(t, []*reporter.SARIFRegion{
		{StartLine: 3, StartColumn: 10, EndLine: 3, EndColumn: 13},
		{StartLine: 4, StartColumn: 4, EndLine: 4, EndColumn: 5},
		nil,
		{StartLine: 3, StartColumn: 3, EndLine: 3, EndColumn: 3},
		{StartLine: 4, StartColumn: 6, EndLine: 4, EndColumn: 6},
		{StartLine: 3, StartColumn: 10},
		{StartLine: 1, StartColumn: 8, EndLine: 1, EndColumn: 11},
	}, regions)
}

func TestSARIFReporter_FixesCarryFullRegions(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "fixes.md"), umlautContent, lint.Diagnostic{
			RuleID: "MD009", StartLine: 3, StartColumn: 13, EndLine: 3, EndColumn: 16,
			Suggestion: "Remove trailing whitespace",
			FixEdits:   []fix.TextEdit{{StartOffset: 21, EndOffset: 24}},
		}),
		sarifFile(filepath.Join(root, "noeol.md"), "# T\n\ntext", lint.Diagnostic{
			RuleID: "MD047", StartLine: 3, StartColumn: 5,
			FixEdits: []fix.TextEdit{{StartOffset: 9, EndOffset: 9, NewText: "\n"}},
		}),
		sarifFile(filepath.Join(root, "trailing.md"), "a\n\n\n", lint.Diagnostic{
			RuleID: "MD012", StartLine: 3, StartColumn: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 2, EndOffset: 4}},
		}),
		sarifFile(filepath.Join(root, "two.md"), "one two\n", lint.Diagnostic{
			RuleID: "MD044", StartLine: 1, StartColumn: 1,
			FixEdits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 3, NewText: "One"},
				{StartOffset: 3, EndOffset: 3},
				{StartOffset: 4, EndOffset: 7, NewText: "Two"},
			},
		}),
		sarifFile(filepath.Join(root, "empty.md"), "", lint.Diagnostic{
			RuleID: "MD041", StartLine: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 0, NewText: "# Title\n"}},
		}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]
	require.Len(t, run.Results, 5)

	umlautFix := run.Results[0].Fixes
	require.Len(t, umlautFix, 1)
	require.NotNil(t, umlautFix[0].Description)
	assert.Equal(t, "Remove trailing whitespace", umlautFix[0].Description.Text)
	assert.Equal(t, []reporter.SARIFArtifactChange{{
		ArtifactLocation: reporter.SARIFArtifactLocation{URI: "fixes.md", URIBaseID: "%SRCROOT%"},
		Replacements: []reporter.SARIFReplacement{{
			DeletedRegion:   reporter.SARIFRegion{StartLine: 3, StartColumn: 10, EndLine: 3, EndColumn: 13},
			InsertedContent: &reporter.SARIFInsertedContent{Text: ""},
		}},
	}}, umlautFix[0].ArtifactChanges, "the deleted region is the trailing spaces, not the whole line")

	replacements := func(idx int) []reporter.SARIFReplacement {
		fixes := run.Results[idx].Fixes
		require.Len(t, fixes, 1)
		require.Len(t, fixes[0].ArtifactChanges, 1, "one artifact change per file")
		return fixes[0].ArtifactChanges[0].Replacements
	}

	assert.Equal(t, []reporter.SARIFReplacement{{
		DeletedRegion:   reporter.SARIFRegion{StartLine: 3, StartColumn: 5, EndLine: 3, EndColumn: 5},
		InsertedContent: &reporter.SARIFInsertedContent{Text: "\n"},
	}}, replacements(1), "insertion point after the last character")
	assert.Nil(t, run.Results[1].Fixes[0].Description, "no description without a suggestion")

	assert.Equal(t, []reporter.SARIFReplacement{{
		DeletedRegion:   reporter.SARIFRegion{StartLine: 2, StartColumn: 1, EndLine: 3, EndColumn: 2},
		InsertedContent: &reporter.SARIFInsertedContent{Text: ""},
	}}, replacements(2), "end of file maps onto the last line, past its newline")

	assert.Equal(t, []reporter.SARIFReplacement{
		{
			DeletedRegion:   reporter.SARIFRegion{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 4},
			InsertedContent: &reporter.SARIFInsertedContent{Text: "One"},
		},
		{
			DeletedRegion:   reporter.SARIFRegion{StartLine: 1, StartColumn: 5, EndLine: 1, EndColumn: 8},
			InsertedContent: &reporter.SARIFInsertedContent{Text: "Two"},
		},
	}, replacements(3), "every edit of a diagnostic is a replacement in one change, and no-op edits are dropped")

	assert.Equal(t, []reporter.SARIFReplacement{{
		DeletedRegion:   reporter.SARIFRegion{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 1},
		InsertedContent: &reporter.SARIFInsertedContent{Text: "# Title\n"},
	}}, replacements(4), "an empty file has one insertion point")
}

func TestSARIFReporter_OmitsFixesThatCannotBeLocated(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	noSnapshot := sarifFile(filepath.Join(root, "nosnap.md"), "abc\n", lint.Diagnostic{
		RuleID: "MD009", StartLine: 1, StartColumn: 1,
		FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1}},
	})
	noSnapshot.Result.Snapshot = nil

	result := &runner.Result{Files: []runner.FileOutcome{
		noSnapshot,
		sarifFile(filepath.Join(root, "range.md"), "abc\n", lint.Diagnostic{
			RuleID: "MD009", StartLine: 1, StartColumn: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1}, {StartOffset: 2, EndOffset: 99}},
		}),
		sarifFile(filepath.Join(root, "noop.md"), "abc\n", lint.Diagnostic{
			RuleID: "MD009", StartLine: 1, StartColumn: 1,
			FixEdits: []fix.TextEdit{{StartOffset: 2, EndOffset: 2}},
		}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]
	require.Len(t, run.Results, 3)
	for idx, res := range run.Results {
		assert.Empty(t, res.Fixes, "result %d", idx)
	}
}

func TestSARIFReporter_DescribesRulesFromRegistry(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	md009, ok := registry.GetByID("MD009")
	require.True(t, ok)

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), umlautContent,
			lint.Diagnostic{
				RuleID: "MD009", RuleName: "no-trailing-spaces", Severity: config.SeverityError,
				Message: "Trailing whitespace", StartLine: 3, StartColumn: 13,
			},
			lint.Diagnostic{
				RuleID: "MD009", RuleName: "no-trailing-spaces", Severity: config.SeverityError,
				Message: "Different wording for the second finding", StartLine: 3, StartColumn: 14,
			},
			lint.Diagnostic{RuleID: "XX999", RuleName: "custom-rule", Message: "Custom finding", StartLine: 1},
		),
	}}

	log := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result)
	driver := log.Runs[0].Tool.Driver

	require.Len(t, driver.Rules, 2, "each rule is described once")

	described := driver.Rules[0]
	assert.Equal(t, "MD009", described.ID)
	assert.Equal(t, md009.Name(), described.Name)
	require.NotNil(t, described.ShortDescription)
	assert.Equal(t, md009.Description(), described.ShortDescription.Text, "the rule description, not a message")
	require.NotNil(t, described.FullDescription)
	assert.Equal(t, md009.Description(), described.FullDescription.Text)
	require.NotNil(t, described.DefaultConfig)
	assert.Equal(t, "warning", described.DefaultConfig.Level, "the default severity of the rule, not the configured one")
	assert.Equal(t, []any{"whitespace"}, described.Properties["tags"])

	assert.Equal(t, reporter.SARIFRule{ID: "XX999", Name: "custom-rule"}, driver.Rules[1],
		"an unregistered rule is identified without invented metadata")

	assert.Equal(t, "error", log.Runs[0].Results[0].Level, "results keep their configured severity")
}

func TestSARIFReporter_ToolMetadata(t *testing.T) {
	t.Parallel()

	log := decodeSARIF(t, reporter.Options{ToolVersion: "1.2.3"}, &runner.Result{})

	assert.Equal(t, "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json", log.Schema)
	assert.Equal(t, "2.1.0", log.Version)
	assert.Equal(t, "gomdlint", log.Runs[0].Tool.Driver.Name)
	assert.Equal(t, "1.2.3", log.Runs[0].Tool.Driver.Version)
}
