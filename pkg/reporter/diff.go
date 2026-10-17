package reporter

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/yaklabco/gomdlint/internal/display"
	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// DiffReporter formats results as unified diffs in GitHub style.
type DiffReporter struct {
	opts   Options
	styles *pretty.Styles
	out    io.Writer
}

// NewDiffReporter creates a new diff reporter.
func NewDiffReporter(opts Options) *DiffReporter {
	return &DiffReporter{
		opts:   opts,
		styles: pretty.NewStylesFor(opts.Color, opts.Writer),
		out:    opts.Writer,
	}
}

// Report implements Reporter.
func (r *DiffReporter) Report(_ context.Context, result *runner.Result) (int, error) {
	if result == nil {
		return 0, nil
	}

	var filesWithDiffs int
	var totalAdditions, totalDeletions int

	for _, file := range result.Files {
		if file.Error != nil {
			fmt.Fprintf(r.out, "%s: %s\n",
				r.styles.FilePath.Render(display.Path(file.Path, r.opts.WorkingDir)),
				r.styles.Error.Render(fmt.Sprintf("error: %v", file.Error)),
			)
			continue
		}

		if file.Result == nil || file.Result.Diff == nil || !file.Result.Diff.HasChanges() {
			continue
		}

		filesWithDiffs++
		totalAdditions += file.Result.Diff.Additions
		totalDeletions += file.Result.Diff.Deletions
		r.writeDiff(file.Result.Diff)
	}

	// Write a summary of the diffs and of any failures: without the
	// failures, a run that changed nothing would look complete.
	failures := r.styles.FormatFailures(result.Stats.FilesErrored, result.Stats.RuleErrors)
	if r.opts.ShowSummary && (filesWithDiffs > 0 || len(failures) > 0) {
		r.writeSummary(filesWithDiffs, totalAdditions, totalDeletions, failures)
	}

	return filesWithDiffs, nil
}

// writeDiff outputs a single file's diff with formatting.
func (r *DiffReporter) writeDiff(diff *fix.Diff) {
	// Use relative path for display if possible.
	displayPath := r.displayPath(diff.Path)

	// Git-style header: "diff --git a/file b/file"
	header := fmt.Sprintf("diff --git a/%s b/%s", displayPath, displayPath)
	fmt.Fprintln(r.out, r.styles.DiffHeader.Render(header))

	// Write --- and +++ headers with relative path.
	fmt.Fprintln(r.out, r.styles.DiffRemove.Render("--- a/"+displayPath))
	fmt.Fprintln(r.out, r.styles.DiffAdd.Render("+++ b/"+displayPath))

	// Colorize the hunks. Every line after the file headers of String() is
	// hunk content, even one that starts with "---" or "+++".
	for _, line := range hunkLines(diff.String()) {
		r.writeDiffLine(line)
	}

	fmt.Fprintln(r.out) // Blank line between files
}

// hunkLines returns the lines of a unified diff that follow its "--- " and
// "+++ " file header lines, without the final newline.
func hunkLines(unified string) []string {
	if unified == "" {
		return nil
	}

	lines := strings.Split(strings.TrimSuffix(unified, "\n"), "\n")
	if len(lines) >= 2 && strings.HasPrefix(lines[0], "--- ") && strings.HasPrefix(lines[1], "+++ ") {
		lines = lines[2:]
	}

	return lines
}

// displayPath returns the path shown in the diff headers: relative to the
// working directory when the file lies inside it (see display.Path),
// as every other reporter shows it. A file outside the working directory keeps
// its full path, not just its basename, so the patch still applies
// unambiguously; the leading slash is dropped so the a/ and b/ prefixes do not
// produce a//path.
func (r *DiffReporter) displayPath(path string) string {
	shown := filepath.ToSlash(display.Path(path, r.opts.WorkingDir))
	return strings.TrimPrefix(shown, "/")
}

// writeDiffLine formats a single diff line with color.
func (r *DiffReporter) writeDiffLine(line string) {
	var styled string

	switch {
	case strings.HasPrefix(line, "@@"):
		styled = r.styles.DiffHunk.Render(line)
	case strings.HasPrefix(line, "+"):
		styled = r.styles.DiffAdd.Render(line)
	case strings.HasPrefix(line, "-"):
		styled = r.styles.DiffRemove.Render(line)
	default:
		styled = r.styles.DiffContext.Render(line)
	}

	fmt.Fprintln(r.out, styled)
}

// writeSummary writes a summary line at the end.
func (r *DiffReporter) writeSummary(files, additions, deletions int, failures []string) {
	var parts []string

	// Files changed.
	if files > 0 {
		fileWord := "files"
		if files == 1 {
			fileWord = "file"
		}
		parts = append(parts, fmt.Sprintf("%d %s changed", files, fileWord))
	}

	// Additions.
	if additions > 0 {
		insertionWord := "insertions"
		if additions == 1 {
			insertionWord = "insertion"
		}
		parts = append(parts, r.styles.DiffAdd.Render(fmt.Sprintf("%d %s(+)", additions, insertionWord)))
	}

	// Deletions.
	if deletions > 0 {
		deletionWord := "deletions"
		if deletions == 1 {
			deletionWord = "deletion"
		}
		parts = append(parts, r.styles.DiffRemove.Render(fmt.Sprintf("%d %s(-)", deletions, deletionWord)))
	}

	parts = append(parts, failures...)
	fmt.Fprintln(r.out, strings.Join(parts, ", "))
}
