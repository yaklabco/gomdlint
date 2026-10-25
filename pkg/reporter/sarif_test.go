package reporter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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

// canonicalTempDir returns a new temporary directory with symbolic links in
// its path resolved, as on macOS where /var links to /private/var.
func canonicalTempDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	return dir
}

// newGitWorkTree returns a temporary directory marked as the root of a git
// work tree, with symbolic links in its path resolved. A linked worktree or a
// submodule marks its root with a .git file rather than a directory, so both
// forms are supported.
func newGitWorkTree(t *testing.T, gitFile bool) string {
	t.Helper()

	root := canonicalTempDir(t)
	marker := filepath.Join(root, ".git")
	if gitFile {
		require.NoError(t, os.WriteFile(marker, []byte("gitdir: /elsewhere/.git/worktrees/x\n"), 0o644))
	} else {
		require.NoError(t, os.Mkdir(marker, 0o755))
	}

	return root
}

// fileURIPrefix returns the file URI of an absolute local path, with three
// slashes before a Windows drive letter as before a Unix root.
func fileURIPrefix(path string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
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

func TestSARIFReporter_SourceRootThroughSymlinkedWorkingDir(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("x\n"), 0o644))

	shortcutParent := t.TempDir()
	skipInsideGitWorkTree(t, shortcutParent)
	shortcut := filepath.Join(shortcutParent, "shortcut")
	if err := os.Symlink(filepath.Join(root, "docs"), shortcut); err != nil {
		t.Skip("cannot create symlinks:", err)
	}

	realRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(shortcut, "a.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: shortcut}, result).Runs[0]

	assert.Equal(t, fileURIPrefix(realRoot)+"/", run.OriginalURIBaseIDs["%SRCROOT%"].URI,
		"a working directory reached through a symlink still finds its repository")
	assert.Equal(t, []reporter.SARIFArtifactLocation{{URI: "docs/a.md", URIBaseID: "%SRCROOT%"}}, resultLocations(run))
}

// skipInsideGitWorkTree skips the test when dir already lies inside a git work
// tree, where the source root would be that tree instead.
func skipInsideGitWorkTree(t *testing.T, dir string) {
	t.Helper()

	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			t.Skipf("temporary directory %s is inside a git work tree", dir)
		}
		if filepath.Dir(current) == current {
			return
		}
	}
}

func TestSARIFReporter_SourceRootFollowsTheResolvedWorkingDir(t *testing.T) {
	t.Parallel()

	// outer/link points into a different repository, target.
	outer := newGitWorkTree(t, false)
	target := newGitWorkTree(t, false)
	require.NoError(t, os.MkdirAll(filepath.Join(target, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "sub", "a.md"), []byte("x\n"), 0o644))
	link := filepath.Join(outer, "link")
	if err := os.Symlink(filepath.Join(target, "sub"), link); err != nil {
		t.Skip("cannot create symlinks:", err)
	}

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(link, "a.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: link}, result).Runs[0]

	assert.Equal(t, fileURIPrefix(target)+"/", run.OriginalURIBaseIDs["%SRCROOT%"].URI,
		"the files belong to the repository the link resolves into")
	assert.Equal(t, []reporter.SARIFArtifactLocation{{URI: "sub/a.md", URIBaseID: "%SRCROOT%"}}, resultLocations(run))
}

func TestSARIFReporter_SourceRootFallsBackToTheRepositoryOfTheLinkedPath(t *testing.T) {
	t.Parallel()

	// outer/link points at a directory outside any repository.
	outer := newGitWorkTree(t, false)
	plain := canonicalTempDir(t)
	skipInsideGitWorkTree(t, plain)
	require.NoError(t, os.WriteFile(filepath.Join(plain, "a.md"), []byte("x\n"), 0o644))
	link := filepath.Join(outer, "link")
	if err := os.Symlink(plain, link); err != nil {
		t.Skip("cannot create symlinks:", err)
	}

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(link, "a.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: link}, result).Runs[0]

	assert.Equal(t, fileURIPrefix(outer)+"/", run.OriginalURIBaseIDs["%SRCROOT%"].URI,
		"with no repository behind the link, the repository of the path as written is used")
	assert.Equal(t, []reporter.SARIFArtifactLocation{{URI: "link/a.md", URIBaseID: "%SRCROOT%"}}, resultLocations(run))
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
	skipInsideGitWorkTree(t, workDir)

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
			// Columns below 1 are unknown, so they are left out.
			lint.Diagnostic{RuleID: "MD001", StartLine: 1, StartColumn: -2},
			lint.Diagnostic{RuleID: "MD003", StartLine: 1, StartColumn: 1, EndLine: 2, EndColumn: -5},
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
		{StartLine: 1},
		{StartLine: 1, StartColumn: 1, EndLine: 2},
		{StartLine: 1, StartColumn: 8, EndLine: 1, EndColumn: 11},
	}, regions)
}

func TestSARIFReporter_RegionsWithoutSourceOmitColumns(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	file := sarifFile(filepath.Join(root, "nosnap.md"), umlautContent,
		lint.Diagnostic{RuleID: "MD009", StartLine: 3, StartColumn: 13, EndLine: 3, EndColumn: 16},
	)
	file.Result.Snapshot = nil

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, &runner.Result{Files: []runner.FileOutcome{file}}).Runs[0]

	require.Len(t, run.Results, 1)
	assert.Equal(t, &reporter.SARIFRegion{StartLine: 3, EndLine: 3}, run.Results[0].Locations[0].PhysicalLocation.Region,
		"byte columns cannot be converted to UTF-16 without the source, so only lines are reported")
}

func TestSARIFReporter_CleansAbsolutePaths(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	outside := filepath.Join(t.TempDir(), "x.md")
	diag := lint.Diagnostic{RuleID: "MD041", StartLine: 1}

	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile("/"+outside, "x\n", diag),
		sarifFile(root+"/./docs/../a.md", "x\n", diag),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]

	assert.Equal(t, []reporter.SARIFArtifactLocation{
		{URI: fileURIPrefix(outside)},
		{URI: "a.md", URIBaseID: "%SRCROOT%"},
	}, resultLocations(run), "a doubled leading slash must not become a URI host")
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

func TestSARIFReporter_OmitsFixesThatTouchTheByteOrderMark(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	edits := [][]fix.TextEdit{
		{{StartOffset: 0, EndOffset: 3}},                // removes the byte order mark
		{{StartOffset: 1, EndOffset: 5}},                // starts inside it
		{{StartOffset: 0, EndOffset: 0, NewText: "x"}},  // inserts before it
		{{StartOffset: 3, EndOffset: 3, NewText: "x"}},  // inserts after it: representable
		{{StartOffset: 3, EndOffset: 5, NewText: "##"}}, // replaces text after it: representable
	}

	diags := make([]lint.Diagnostic, 0, len(edits))
	for _, edit := range edits {
		diags = append(diags, lint.Diagnostic{RuleID: "MD018", StartLine: 1, FixEdits: edit})
	}
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "bom.md"), "\xef\xbb\xbf# Title\n", diags...),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]
	require.Len(t, run.Results, len(edits))

	// SARIF text regions ignore the byte order mark, so an edit of its bytes
	// cannot be expressed and a consumer would apply a different change.
	for idx := range 3 {
		assert.Empty(t, run.Results[idx].Fixes, "edit %d touches the byte order mark", idx)
	}
	for idx := 3; idx < len(edits); idx++ {
		require.Len(t, run.Results[idx].Fixes, 1, "edit %d", idx)
	}
	assert.Equal(t, reporter.SARIFRegion{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 3},
		run.Results[4].Fixes[0].ArtifactChanges[0].Replacements[0].DeletedRegion)
}

func TestSARIFReporter_DescribesRulesFromRegistry(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	md009, ok := registry.GetByID("MD009")
	require.True(t, ok)
	md009Meta, ok := registry.Meta("MD009")
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
	assert.Equal(t, md009Meta.Description, described.ShortDescription.Text, "the catalog description, not a message")
	require.NotNil(t, described.FullDescription)
	assert.Equal(t, md009Meta.Description+" "+md009Meta.Rationale+" "+md009Meta.Fix, described.FullDescription.Text,
		"the catalog description, rationale and fix")
	require.NotNil(t, described.DefaultConfig)
	assert.Equal(t, "warning", described.DefaultConfig.Level, "the default severity of the rule, not the configured one")
	assert.Equal(t, []any{"layout", "whitespace"}, described.Properties["tags"], "the category, then markdownlint's tags")

	assert.Equal(t, reporter.SARIFRule{ID: "XX999", Name: "custom-rule"}, driver.Rules[1],
		"an unregistered rule is identified without invented metadata")

	assert.Equal(t, "error", log.Runs[0].Results[0].Level, "results keep their configured severity")
}

// sarifDescriptionLimit is the number of characters of a rule description
// that GitHub code scanning keeps.
const sarifDescriptionLimit = 1024

func TestSARIFReporter_DescribesEveryRuleFromTheCatalog(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	rules.RegisterAll(registry)
	catalog := rules.Catalog()

	diags := make([]lint.Diagnostic, 0, len(catalog))
	for _, meta := range catalog {
		diags = append(diags, lint.Diagnostic{RuleID: meta.ID, RuleName: meta.Name, StartLine: 1})
	}
	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{sarifFile(filepath.Join(root, "a.md"), "x\n", diags...)}}

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result).Runs[0].Tool.Driver.Rules

	require.Len(t, descriptors, len(catalog))
	for idx, meta := range catalog {
		descriptor := descriptors[idx]
		require.Equal(t, meta.ID, descriptor.ID)
		assert.Equal(t, meta.Name, descriptor.Name)
		if assert.NotNil(t, descriptor.ShortDescription, meta.ID) {
			assert.Equal(t, meta.Description, descriptor.ShortDescription.Text, "%s: the one-line description", meta.ID)
		}
		if assert.NotNil(t, descriptor.FullDescription, meta.ID) {
			full := descriptor.FullDescription.Text
			assert.Equal(t, meta.Description+" "+meta.Rationale+" "+meta.Fix, full,
				"%s: what correct Markdown looks like, why, and what the fix does", meta.ID)
			assert.LessOrEqual(t, utf8.RuneCountInString(full), sarifDescriptionLimit, "%s: GitHub keeps the whole text", meta.ID)
		}
	}
}

// TestSARIFReporter_FallsBackToTheRuleDescription covers rules whose catalog
// entry cannot describe them: a rule without an entry, and a rule whose
// entry has no description, which keep the rule's own description in both
// fields. An entry with a description but no rationale or fix gives that
// description alone.
func TestSARIFReporter_FallsBackToTheRuleDescription(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	plain := lint.NewBaseRule("ZZ001", "plain-rule", "Described by the rule itself", nil, false)
	partial := lint.NewBaseRule("ZZ002", "partial-rule", "The rule's own description", nil, false)
	blank := lint.NewBaseRule("ZZ003", "blank-entry-rule", "The entry leaves this to the rule", nil, false)
	registry.Register(&plain)
	registry.Register(&partial)
	registry.Register(&blank)
	registry.Describe(lint.RuleMeta{ID: "ZZ002", Name: "partial-rule", Description: "The catalog's one line."})
	registry.Describe(lint.RuleMeta{
		ID: "ZZ003", Name: "blank-entry-rule",
		Rationale: "A reason without a description is not used.", Fix: "There is no automatic fix.",
	})

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), "x\n",
			lint.Diagnostic{RuleID: "ZZ001", StartLine: 1},
			lint.Diagnostic{RuleID: "ZZ002", StartLine: 1},
			lint.Diagnostic{RuleID: "ZZ003", StartLine: 1},
		),
	}}

	descriptors := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: registry}, result).Runs[0].Tool.Driver.Rules

	text := func(value string) *reporter.SARIFMultiformatText {
		return &reporter.SARIFMultiformatText{Text: value}
	}
	require.Len(t, descriptors, 3)
	assert.Equal(t, text("Described by the rule itself"), descriptors[0].ShortDescription, "a rule without an entry")
	assert.Equal(t, text("Described by the rule itself"), descriptors[0].FullDescription, "a rule without an entry")
	assert.Equal(t, text("The catalog's one line."), descriptors[1].ShortDescription, "the entry's description")
	assert.Equal(t, text("The catalog's one line."), descriptors[1].FullDescription, "an entry without a rationale or a fix")
	assert.Equal(t, text("The entry leaves this to the rule"), descriptors[2].ShortDescription, "an entry without a description")
	assert.Equal(t, text("The entry leaves this to the rule"), descriptors[2].FullDescription, "an entry without a description")
}

func TestSARIFReporter_RecordsFilesThatFailed(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	result := &runner.Result{
		Files: []runner.FileOutcome{
			sarifFile(filepath.Join(root, "ok.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
			{Path: filepath.Join(root, "docs", "locked.md"), Error: errors.New("permission denied")},
		},
		Errors: []error{errors.New("walk interrupted")},
	}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]

	require.Len(t, run.Invocations, 1)
	invocation := run.Invocations[0]
	assert.False(t, invocation.ExecutionSuccessful, "a file that could not be linted is a failed run")
	assert.Equal(t, []reporter.SARIFNotification{
		{
			Level:   "error",
			Message: reporter.SARIFMessage{Text: "permission denied"},
			Locations: []reporter.SARIFLocation{{PhysicalLocation: reporter.SARIFPhysicalLocation{
				ArtifactLocation: reporter.SARIFArtifactLocation{URI: "docs/locked.md", URIBaseID: "%SRCROOT%"},
			}}},
		},
		{Level: "error", Message: reporter.SARIFMessage{Text: "walk interrupted"}},
	}, invocation.ToolExecutionNotifications)
	assert.Len(t, run.Results, 1, "findings in other files are still reported")
}

func TestSARIFReporter_RecordsRuleFailures(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	file := sarifFile(filepath.Join(root, "a.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1})
	file.Result.RuleErrors = map[string]error{
		"MD013": errors.New("line length exploded"),
		"MD009": errors.New("whitespace exploded"),
		"MD010": nil,
	}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, &runner.Result{Files: []runner.FileOutcome{file}}).Runs[0]

	require.Len(t, run.Invocations, 1)
	assert.False(t, run.Invocations[0].ExecutionSuccessful, "a rule that failed did not analyze the file")

	location := []reporter.SARIFLocation{{PhysicalLocation: reporter.SARIFPhysicalLocation{
		ArtifactLocation: reporter.SARIFArtifactLocation{URI: "a.md", URIBaseID: "%SRCROOT%"},
	}}}
	assert.Equal(t, []reporter.SARIFNotification{
		{
			Level:          "error",
			Message:        reporter.SARIFMessage{Text: "rule MD009 failed: whitespace exploded"},
			Locations:      location,
			AssociatedRule: &reporter.SARIFRuleReference{ID: "MD009", Index: intPtr(1)},
		},
		{
			Level:          "error",
			Message:        reporter.SARIFMessage{Text: "rule MD013 failed: line length exploded"},
			Locations:      location,
			AssociatedRule: &reporter.SARIFRuleReference{ID: "MD013", Index: intPtr(2)},
		},
	}, run.Invocations[0].ToolExecutionNotifications, "one notification per failed rule, in rule order")
	assert.Len(t, run.Results, 1, "findings of the rules that ran are still reported")

	// A failed rule is described too, so its reference can carry an index.
	descriptors := run.Tool.Driver.Rules
	require.Len(t, descriptors, 3)
	assert.Equal(t, []string{"MD041", "MD009", "MD013"}, []string{descriptors[0].ID, descriptors[1].ID, descriptors[2].ID})
	assert.NotNil(t, descriptors[2].ShortDescription, "the failed rule carries its registry metadata")
}

// intPtr returns a pointer to value, for optional integers in SARIF types.
func intPtr(value int) *int {
	return &value
}

func TestSARIFReporter_SuccessfulRun(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	result := &runner.Result{
		Files: []runner.FileOutcome{
			sarifFile(filepath.Join(root, "ok.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
		},
		Errors: []error{nil},
	}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]

	require.Len(t, run.Invocations, 1)
	assert.True(t, run.Invocations[0].ExecutionSuccessful, "findings do not make a run fail")
	assert.Empty(t, run.Invocations[0].ToolExecutionNotifications)
}

// ruleLookup is a minimal rule catalogue for tests.
type ruleLookup map[string]lint.Rule

func (l ruleLookup) GetByID(id string) (lint.Rule, bool) {
	rule, ok := l[id]
	return rule, ok
}

// Meta reports that no rule has a catalog entry, so rules describe
// themselves.
func (l ruleLookup) Meta(string) (lint.RuleMeta, bool) {
	return lint.RuleMeta{}, false
}

func TestSARIFReporter_AcceptsAnyRuleLookup(t *testing.T) {
	t.Parallel()

	custom := lint.NewBaseRule("ZZ001", "custom-rule", "Custom rules are described too", []string{"custom", "custom"}, false)
	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), "x\n", lint.Diagnostic{RuleID: "ZZ001", StartLine: 1}),
	}}

	driver := decodeSARIF(t, reporter.Options{WorkingDir: root, Registry: ruleLookup{"ZZ001": &custom}}, result).Runs[0].Tool.Driver

	require.Len(t, driver.Rules, 1)
	require.NotNil(t, driver.Rules[0].ShortDescription)
	assert.Equal(t, "Custom rules are described too", driver.Rules[0].ShortDescription.Text)
	assert.Equal(t, "custom-rule", driver.Rules[0].Name)
	assert.Equal(t, []any{"custom"}, driver.Rules[0].Properties["tags"], "tags are unique")
}

func TestSARIFReporter_TypedNilRegistryFallsBackToDefault(t *testing.T) {
	t.Parallel()

	var registry *lint.Registry
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(newGitWorkTree(t, false), "a.md"), umlautContent, lint.Diagnostic{RuleID: "MD009", StartLine: 3}),
	}}

	driver := decodeSARIF(t, reporter.Options{Registry: registry}, result).Runs[0].Tool.Driver

	require.Len(t, driver.Rules, 1)
	assert.NotNil(t, driver.Rules[0].ShortDescription, "the default registry describes MD009")
}

func TestSARIFReporter_ResultsReferenceTheirRuleByIndex(t *testing.T) {
	t.Parallel()

	root := newGitWorkTree(t, false)
	result := &runner.Result{Files: []runner.FileOutcome{
		sarifFile(filepath.Join(root, "a.md"), umlautContent,
			lint.Diagnostic{RuleID: "MD009", RuleName: "no-trailing-spaces", StartLine: 3, StartColumn: 13},
			lint.Diagnostic{RuleID: "XX999", RuleName: "custom-rule", StartLine: 1},
			lint.Diagnostic{RuleID: "MD009", RuleName: "no-trailing-spaces", StartLine: 3, StartColumn: 14},
		),
		sarifFile(filepath.Join(root, "b.md"), "x\n", lint.Diagnostic{RuleID: "MD041", StartLine: 1}),
	}}

	run := decodeSARIF(t, reporter.Options{WorkingDir: root}, result).Runs[0]
	descriptors := run.Tool.Driver.Rules

	want := []int{0, 1, 0, 2}
	require.Len(t, run.Results, len(want))
	for idx, res := range run.Results {
		require.NotNil(t, res.RuleIndex, "result %d must reference its rule descriptor, even at index 0", idx)
		assert.Equal(t, want[idx], *res.RuleIndex, "result %d", idx)
		require.Less(t, *res.RuleIndex, len(descriptors))
		assert.Equal(t, res.RuleID, descriptors[*res.RuleIndex].ID, "result %d points at the descriptor of its own rule", idx)
	}
}

func TestSARIFReporter_ToolMetadata(t *testing.T) {
	t.Parallel()

	log := decodeSARIF(t, reporter.Options{ToolVersion: "1.2.3"}, &runner.Result{})

	assert.Equal(t, "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json", log.Schema)
	assert.Equal(t, "2.1.0", log.Version)
	assert.Equal(t, "gomdlint", log.Runs[0].Tool.Driver.Name)
	assert.Equal(t, "1.2.3", log.Runs[0].Tool.Driver.Version)
}
