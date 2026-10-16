package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/yaklabco/gomdlint/internal/display"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// SARIF version used by this reporter.
const sarifVersion = "2.1.0"

// sarifSchemaURI is the OASIS location of the SARIF 2.1.0 (errata 01) schema.
const sarifSchemaURI = "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"

const (
	sarifToolName       = "gomdlint"
	sarifInformationURI = "https://github.com/yaklabco/gomdlint"

	// sarifSourceRootID is the URI base ID that relative artifact URIs use. It
	// names the root of the source tree, as SARIF conventionally does.
	sarifSourceRootID = "%SRCROOT%"

	// sarifColumnKind declares the unit of every column in the log, matching
	// the display.UTF16CodeUnits conversion the regions use. SARIF requires it
	// whenever a run reports results on text files.
	sarifColumnKind = "utf16CodeUnits"

	// devVersion is the version string of builds made without release ldflags.
	devVersion = "dev"

	// develBuildVersion is the main module version Go records when it knows no
	// better, for example under go run.
	develBuildVersion = "(devel)"
)

// SARIFOutput represents the root SARIF document.
type SARIFOutput struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

// SARIFRun represents a single analysis run.
type SARIFRun struct {
	Tool SARIFTool `json:"tool"`

	// Invocations records whether the run succeeded. A file that could not be
	// linted makes the run unsuccessful and is reported as a notification.
	Invocations []SARIFInvocation `json:"invocations,omitempty"`

	// OriginalURIBaseIDs maps each URI base ID used by relative artifact URIs
	// (%SRCROOT%) to the absolute file URI it stood for when the log was
	// written. It is empty when the base is unknown.
	OriginalURIBaseIDs map[string]SARIFArtifactLocation `json:"originalUriBaseIds,omitempty"`

	Results []SARIFResult `json:"results"`

	// ColumnKind names the unit of every column in the run: utf16CodeUnits.
	ColumnKind string `json:"columnKind,omitempty"`
}

// SARIFInvocation describes how the run went. ExecutionSuccessful is false
// when a file could not be linted; findings alone do not make it false.
type SARIFInvocation struct {
	ExecutionSuccessful        bool                `json:"executionSuccessful"`
	ToolExecutionNotifications []SARIFNotification `json:"toolExecutionNotifications,omitempty"`
}

// SARIFNotification reports a problem the tool met while running, such as a
// file it could not read, as opposed to a finding in a file.
type SARIFNotification struct {
	Level     string          `json:"level"`
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations,omitempty"`
}

// SARIFTool describes the analysis tool.
type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

// SARIFDriver contains tool metadata and rules.
type SARIFDriver struct {
	Name string `json:"name"`

	// Version is the gomdlint version, omitted when the build does not know it.
	Version string `json:"version,omitempty"`

	InformationURI string      `json:"informationUri"`
	Rules          []SARIFRule `json:"rules"`
}

// SARIFRule describes a rule (linter check). Descriptions, the default
// configuration and tags come from the rule registry; they are absent for a
// rule the registry does not know.
type SARIFRule struct {
	ID               string                `json:"id"`
	Name             string                `json:"name,omitempty"`
	ShortDescription *SARIFMultiformatText `json:"shortDescription,omitempty"`
	FullDescription  *SARIFMultiformatText `json:"fullDescription,omitempty"`
	DefaultConfig    *SARIFRuleConfig      `json:"defaultConfiguration,omitempty"`
	Properties       map[string]any        `json:"properties,omitempty"`
}

// SARIFMultiformatText contains text in multiple formats.
type SARIFMultiformatText struct {
	Text string `json:"text"`
}

// SARIFRuleConfig contains rule configuration.
type SARIFRuleConfig struct {
	Level string `json:"level"`
}

// SARIFResult represents a single diagnostic result.
type SARIFResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   SARIFMessage    `json:"message"`
	Locations []SARIFLocation `json:"locations"`
	Fixes     []SARIFFix      `json:"fixes,omitempty"`
}

// SARIFMessage contains the result message.
type SARIFMessage struct {
	Text string `json:"text"`
}

// SARIFLocation describes a code location.
type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

// SARIFPhysicalLocation contains file path and region.
type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`

	// Region is nil for a result that has no position in the file.
	Region *SARIFRegion `json:"region,omitempty"`
}

// SARIFArtifactLocation identifies a file. When URIBaseID is set, URI is a
// relative reference resolved against that base (see
// SARIFRun.OriginalURIBaseIDs); otherwise URI is an absolute file URI.
type SARIFArtifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

// SARIFRegion describes a text region. Lines are 1-based. Columns are 1-based
// and count UTF-16 code units, as SARIFRun.ColumnKind declares; EndColumn is
// exclusive. Zero fields are omitted: an absent StartColumn means column 1,
// and an absent end means the end of the start line.
type SARIFRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIFFix represents a proposed fix.
type SARIFFix struct {
	// Description is the diagnostic's suggestion, when it has one.
	Description     *SARIFMessage         `json:"description,omitempty"`
	ArtifactChanges []SARIFArtifactChange `json:"artifactChanges"`
}

// SARIFArtifactChange describes changes to a file.
type SARIFArtifactChange struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Replacements     []SARIFReplacement    `json:"replacements"`
}

// SARIFReplacement describes a text replacement. DeletedRegion locates the
// replaced text in the unmodified file; an empty region is an insertion point.
type SARIFReplacement struct {
	DeletedRegion   SARIFRegion           `json:"deletedRegion"`
	InsertedContent *SARIFInsertedContent `json:"insertedContent,omitempty"`
}

// SARIFInsertedContent contains the replacement text.
type SARIFInsertedContent struct {
	Text string `json:"text"`
}

// SARIFReporter formats results as SARIF.
type SARIFReporter struct {
	opts     Options
	out      io.Writer
	registry *lint.Registry
}

// NewSARIFReporter creates a new SARIF reporter.
func NewSARIFReporter(opts Options) *SARIFReporter {
	registry := opts.Registry
	if registry == nil {
		registry = lint.DefaultRegistry
	}

	return &SARIFReporter{
		opts:     opts,
		out:      opts.Writer,
		registry: registry,
	}
}

// Report implements Reporter.
func (r *SARIFReporter) Report(_ context.Context, result *runner.Result) (int, error) {
	output := r.buildOutput(result)

	encoder := json.NewEncoder(r.out)
	if !r.opts.Compact {
		encoder.SetIndent("", "  ")
	}

	if err := encoder.Encode(output); err != nil {
		return 0, fmt.Errorf("encode SARIF: %w", err)
	}

	return len(output.Runs[0].Results), nil
}

func (r *SARIFReporter) buildOutput(result *runner.Result) *SARIFOutput {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		buildInfo = nil
	}

	run := SARIFRun{
		Tool: SARIFTool{
			Driver: SARIFDriver{
				Name:           sarifToolName,
				Version:        toolVersion(r.opts.ToolVersion, buildInfo),
				InformationURI: sarifInformationURI,
				Rules:          make([]SARIFRule, 0),
			},
		},
		Results:    make([]SARIFResult, 0),
		ColumnKind: sarifColumnKind,
	}

	locator := newArtifactLocator(r.opts.WorkingDir)
	if base := locator.baseURI(); base != "" {
		run.OriginalURIBaseIDs = map[string]SARIFArtifactLocation{sarifSourceRootID: {URI: base}}
	}

	if result != nil {
		builder := &sarifRunBuilder{
			run:       &run,
			registry:  r.registry,
			locator:   locator,
			described: make(map[string]bool),
		}
		for _, file := range result.Files {
			if file.Error != nil {
				builder.addFailure(file.Error, file.Path)
				continue
			}
			if file.Result == nil || file.Result.FileResult == nil {
				continue
			}
			for idx := range file.Result.Diagnostics {
				builder.addResult(file, &file.Result.Diagnostics[idx])
			}
		}
		for _, err := range result.Errors {
			if err != nil {
				builder.addFailure(err, "")
			}
		}

		invocation := builder.invocation
		invocation.ExecutionSuccessful = len(invocation.ToolExecutionNotifications) == 0
		run.Invocations = []SARIFInvocation{invocation}
	}

	return &SARIFOutput{
		Schema:  sarifSchemaURI,
		Version: sarifVersion,
		Runs:    []SARIFRun{run},
	}
}

// toolVersion returns the version to report as tool.driver.version: explicit,
// unless it is empty or the "dev" placeholder of builds without release
// ldflags, and otherwise the main module version recorded in the build
// information (set by go install and by builds of a tagged checkout). It
// returns "" when neither names a version.
func toolVersion(explicit string, buildInfo *debug.BuildInfo) string {
	if explicit != "" && explicit != devVersion {
		return explicit
	}

	if buildInfo != nil && buildInfo.Main.Version != "" && buildInfo.Main.Version != develBuildVersion {
		return buildInfo.Main.Version
	}

	return ""
}

// sarifRunBuilder accumulates the rules, results and failures of a run.
type sarifRunBuilder struct {
	run        *SARIFRun
	registry   *lint.Registry
	locator    *artifactLocator
	described  map[string]bool
	invocation SARIFInvocation
}

// addFailure records an error the run met as an error notification, located
// at path when it concerns a file.
func (b *sarifRunBuilder) addFailure(err error, path string) {
	notification := SARIFNotification{Level: "error", Message: SARIFMessage{Text: err.Error()}}
	if path != "" {
		notification.Locations = []SARIFLocation{{
			PhysicalLocation: SARIFPhysicalLocation{ArtifactLocation: b.locator.locate(path)},
		}}
	}

	b.invocation.ToolExecutionNotifications = append(b.invocation.ToolExecutionNotifications, notification)
}

// addResult appends the result for diag, describing its rule on first use.
func (b *sarifRunBuilder) addResult(file runner.FileOutcome, diag *lint.Diagnostic) {
	if !b.described[diag.RuleID] {
		b.described[diag.RuleID] = true
		b.run.Tool.Driver.Rules = append(b.run.Tool.Driver.Rules, b.describeRule(diag))
	}

	path := diag.FilePath
	if path == "" {
		path = file.Path
	}
	location := b.locator.locate(path)
	snapshot := file.Result.Snapshot

	result := SARIFResult{
		RuleID:  diag.RuleID,
		Level:   severityToSARIFLevel(diag.Severity),
		Message: SARIFMessage{Text: diag.Message},
		Locations: []SARIFLocation{{
			PhysicalLocation: SARIFPhysicalLocation{
				ArtifactLocation: location,
				Region:           diagnosticRegion(snapshot, diag),
			},
		}},
	}

	if fix, ok := buildFix(location, snapshot, diag); ok {
		result.Fixes = []SARIFFix{fix}
	}

	b.run.Results = append(b.run.Results, result)
}

// describeRule returns the SARIF description of the rule that produced diag,
// taking its metadata from the registry.
func (b *sarifRunBuilder) describeRule(diag *lint.Diagnostic) SARIFRule {
	rule := SARIFRule{ID: diag.RuleID, Name: diag.RuleName}

	registered, ok := b.registry.GetByID(diag.RuleID)
	if !ok {
		return rule
	}

	rule.Name = registered.Name()
	if description := registered.Description(); description != "" {
		rule.ShortDescription = &SARIFMultiformatText{Text: description}
		rule.FullDescription = &SARIFMultiformatText{Text: description}
	}
	rule.DefaultConfig = &SARIFRuleConfig{Level: severityToSARIFLevel(registered.DefaultSeverity())}
	if tags := uniqueTags(registered.Tags()); len(tags) > 0 {
		rule.Properties = map[string]any{"tags": tags}
	}

	return rule
}

// uniqueTags returns the non-empty tags in order without duplicates, as SARIF
// requires of a tags property.
func uniqueTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	unique := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		unique = append(unique, tag)
	}
	return unique
}

// sarifPoint is a 1-based line and UTF-16 column in SARIF's text model. A
// zero column is absent.
type sarifPoint struct {
	line   int
	column int
}

// endsBefore reports whether a region ending at end would end before it
// starts at start. An absent end column means the end of the line, and an
// absent start column means column 1.
func endsBefore(end, start sarifPoint) bool {
	if end.line != start.line {
		return end.line < start.line
	}
	return end.column != 0 && end.column < max(start.column, 1)
}

// diagnosticRegion converts the byte-based position of diag into a SARIF text
// region. It returns nil when the diagnostic has no line, so the result
// refers to the whole file. An end before the start is collapsed onto the
// start.
func diagnosticRegion(snapshot *mdast.FileSnapshot, diag *lint.Diagnostic) *SARIFRegion {
	if diag.StartLine < 1 {
		return nil
	}

	start := toSARIFPoint(snapshot, diag.StartLine, diag.StartColumn)
	region := &SARIFRegion{StartLine: start.line, StartColumn: start.column}
	if diag.EndLine < 1 {
		return region
	}

	end := toSARIFPoint(snapshot, diag.EndLine, diag.EndColumn)
	if endsBefore(end, start) {
		end = sarifPoint{line: start.line, column: max(start.column, 1)}
	}
	region.EndLine = end.line
	region.EndColumn = end.column

	return region
}

// toSARIFPoint converts a 1-based line and byte column to SARIF's text model,
// counting columns in UTF-16 code units. SARIF has no empty line after a
// final newline, so a position on that line moves to the end of the last
// line, past its newline.
func toSARIFPoint(snapshot *mdast.FileSnapshot, line, byteColumn int) sarifPoint {
	if isTrailingEmptyLine(snapshot, line) {
		last := line - 1
		info := snapshot.Lines[last-1]
		pastNewline := info.EndOffset - info.StartOffset + 1
		return sarifPoint{line: last, column: display.Column(snapshot, mdast.Position{Line: last, Column: pastNewline}, display.UTF16CodeUnits)}
	}

	return sarifPoint{line: line, column: display.Column(snapshot, mdast.Position{Line: line, Column: byteColumn}, display.UTF16CodeUnits)}
}

// isTrailingEmptyLine reports whether line is the empty line that follows the
// final newline of snapshot.
func isTrailingEmptyLine(snapshot *mdast.FileSnapshot, line int) bool {
	if snapshot == nil {
		return false
	}

	count := len(snapshot.Lines)
	return line == count && count > 1 && snapshot.Lines[count-1].StartOffset == len(snapshot.Content)
}

// buildFix returns the SARIF fix for the edits of diag. SARIF requires each
// change in a fix to name a distinct file, so every edit becomes a
// replacement in one change, in the order the rule produced them. Edits that
// change nothing are dropped. There is no fix without a snapshot to locate
// the edits in, or when any edit falls outside the file.
func buildFix(location SARIFArtifactLocation, snapshot *mdast.FileSnapshot, diag *lint.Diagnostic) (SARIFFix, bool) {
	if snapshot == nil || len(diag.FixEdits) == 0 {
		return SARIFFix{}, false
	}

	replacements := make([]SARIFReplacement, 0, len(diag.FixEdits))
	for _, edit := range diag.FixEdits {
		if edit.StartOffset < 0 || edit.EndOffset < edit.StartOffset || edit.EndOffset > len(snapshot.Content) {
			return SARIFFix{}, false
		}
		if edit.StartOffset == edit.EndOffset && edit.NewText == "" {
			continue
		}

		replacements = append(replacements, SARIFReplacement{
			DeletedRegion:   offsetRegion(snapshot, edit.StartOffset, edit.EndOffset),
			InsertedContent: &SARIFInsertedContent{Text: edit.NewText},
		})
	}

	if len(replacements) == 0 {
		return SARIFFix{}, false
	}

	fix := SARIFFix{
		ArtifactChanges: []SARIFArtifactChange{{ArtifactLocation: location, Replacements: replacements}},
	}
	if diag.Suggestion != "" {
		fix.Description = &SARIFMessage{Text: diag.Suggestion}
	}

	return fix, true
}

// offsetRegion returns the SARIF text region for the byte range [start, end)
// of snapshot.
func offsetRegion(snapshot *mdast.FileSnapshot, start, end int) SARIFRegion {
	from := offsetPoint(snapshot, start)
	to := offsetPoint(snapshot, end)

	return SARIFRegion{StartLine: from.line, StartColumn: from.column, EndLine: to.line, EndColumn: to.column}
}

// offsetPoint converts a byte offset of snapshot to a SARIF point. An empty
// file has a single insertion point, at line 1, column 1.
func offsetPoint(snapshot *mdast.FileSnapshot, offset int) sarifPoint {
	line, column := snapshot.LineAt(offset)
	if line < 1 {
		return sarifPoint{line: 1, column: 1}
	}

	return toSARIFPoint(snapshot, line, column)
}

// artifactLocator turns file paths into SARIF artifact locations. Files under
// the source root get URIs relative to %SRCROOT%; other files get absolute
// file URIs.
type artifactLocator struct {
	// workDir is the absolute working directory, or "" when unknown.
	workDir string

	// root is the absolute source root, or "" when unknown.
	root string

	cache map[string]SARIFArtifactLocation
}

func newArtifactLocator(workDir string) *artifactLocator {
	locator := &artifactLocator{cache: make(map[string]SARIFArtifactLocation)}
	if workDir == "" {
		return locator
	}

	absDir, err := filepath.Abs(workDir)
	if err != nil {
		return locator
	}

	locator.workDir = absDir
	locator.root = sourceRoot(absDir)

	return locator
}

// baseURI returns the absolute URI that %SRCROOT% stands for, ending in a
// slash as SARIF requires, or "" when the source root is unknown.
func (l *artifactLocator) baseURI() string {
	if l.root == "" {
		return ""
	}

	uri := fileURI(l.root)
	if !strings.HasSuffix(uri, "/") {
		uri += "/"
	}

	return uri
}

// locate returns the artifact location for path.
func (l *artifactLocator) locate(path string) SARIFArtifactLocation {
	if location, ok := l.cache[path]; ok {
		return location
	}

	location := l.computeLocation(path)
	l.cache[path] = location

	return location
}

func (l *artifactLocator) computeLocation(path string) SARIFArtifactLocation {
	absPath := path
	if !filepath.IsAbs(absPath) {
		if l.workDir == "" {
			// The base of a relative path is unknown; keep it relative.
			return SARIFArtifactLocation{URI: relativeURI(path), URIBaseID: sarifSourceRootID}
		}
		absPath = filepath.Join(l.workDir, path)
	}

	if rel, ok := l.relativeToRoot(absPath); ok {
		return SARIFArtifactLocation{URI: relativeURI(rel), URIBaseID: sarifSourceRootID}
	}

	return SARIFArtifactLocation{URI: fileURI(absPath)}
}

// relativeToRoot returns absPath relative to the source root when the file
// lies inside it. GitHub code scanning cannot display a result whose path
// goes through a symbolic link, so a file reached through one is located by
// its real path when that path is inside the root too.
func (l *artifactLocator) relativeToRoot(absPath string) (string, bool) {
	if l.root == "" {
		return "", false
	}

	if realPath, err := filepath.EvalSymlinks(absPath); err == nil && realPath != absPath {
		if rel := display.Path(realPath, l.root); !filepath.IsAbs(rel) {
			return rel, true
		}
	}

	if rel := display.Path(absPath, l.root); !filepath.IsAbs(rel) {
		return rel, true
	}

	return "", false
}

// sourceRoot returns the directory that artifact URIs are relative to: the
// root of the git work tree containing dir, or dir itself outside one. GitHub
// code scanning resolves relative URIs against the repository root, so this
// keeps results attached to the right files when gomdlint runs in a
// subdirectory. A work tree root holds a .git directory, or a .git file for a
// linked worktree or submodule.
func sourceRoot(dir string) string {
	for candidate := dir; ; {
		if _, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
			return candidate
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			return dir
		}
		candidate = parent
	}
}

// relativeURI converts a relative file path to a relative URI reference,
// percent-encoding what a URI path cannot contain.
func relativeURI(path string) string {
	return (&url.URL{Path: filepath.ToSlash(filepath.Clean(path))}).String()
}

// fileURI converts an absolute file path to a file URI, for example
// /home/me/a b.md to file:///home/me/a%20b.md and C:\docs\a.md to
// file:///C:/docs/a.md.
func fileURI(path string) string {
	return fileURIFromSlash(filepath.ToSlash(path))
}

// fileURIFromSlash converts an absolute slash-separated path, including the
// Windows forms C:/dir/file and //server/share/file, to a file URI.
func fileURIFromSlash(path string) string {
	uri := url.URL{Scheme: "file"}

	switch {
	case strings.HasPrefix(path, "//"):
		host, rest, _ := strings.Cut(strings.TrimPrefix(path, "//"), "/")
		uri.Host = host
		uri.Path = "/" + rest
	case strings.HasPrefix(path, "/"):
		uri.Path = path
	default:
		uri.Path = "/" + path
	}

	return uri.String()
}

// severityToSARIFLevel converts gomdlint severity to SARIF level.
func severityToSARIFLevel(severity config.Severity) string {
	switch severity {
	case config.SeverityError:
		return "error"
	case config.SeverityWarning:
		return "warning"
	case config.SeverityInfo:
		return "note"
	default:
		return "warning"
	}
}
