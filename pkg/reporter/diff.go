package reporter

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	colorEnabled := pretty.IsColorEnabled(opts.Color, opts.Writer)
	return &DiffReporter{
		opts:   opts,
		styles: pretty.NewStyles(colorEnabled),
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
				r.styles.FilePath.Render(file.Path),
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

	// Write summary if there were any diffs.
	if filesWithDiffs > 0 && r.opts.ShowSummary {
		r.writeSummary(filesWithDiffs, totalAdditions, totalDeletions)
	}

	return filesWithDiffs, nil
}

// writeDiff outputs a single file's diff with formatting.
func (r *DiffReporter) writeDiff(diff *fix.Diff) {
	// Use relative path for display if possible.
	displayPath := relativePath(diff.Path)

	// Git-style header: "diff --git a/file b/file"
	header := fmt.Sprintf("diff --git a/%s b/%s", displayPath, displayPath)
	fmt.Fprintln(r.out, r.styles.DiffHeader.Render(header))

	// Write --- and +++ headers with relative path.
	fmt.Fprintln(r.out, r.styles.DiffRemove.Render("--- a/"+displayPath))
	fmt.Fprintln(r.out, r.styles.DiffAdd.Render("+++ b/"+displayPath))

	// Parse and colorize the hunk content (skip the --- and +++ lines from String()).
	lines := strings.Split(diff.String(), "\n")
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}
		r.writeDiffLine(line)
	}

	fmt.Fprintln(r.out) // Blank line between files
}

// relativePath converts an absolute path to a relative path from the current directory.
// If the relative path would require too many "../" traversals, use the basename instead.
func relativePath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Base(path)
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return filepath.Base(path)
	}
	// If relative path has too many parent traversals, just use basename.
	if strings.Count(rel, "..") > 2 {
		return filepath.Base(path)
	}
	return rel
}

// writeDiffLine formats a single diff line with color.
func (r *DiffReporter) writeDiffLine(line string) {
	var styled string

	switch {
	case strings.HasPrefix(line, "@@"):
		styled = r.styles.DiffHunk.Render(line)
	case strings.HasPrefix(line, "+++"):
		styled = r.styles.DiffAdd.Render(line)
	case strings.HasPrefix(line, "---"):
		styled = r.styles.DiffRemove.Render(line)
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
func (r *DiffReporter) writeSummary(files, additions, deletions int) {
	var parts []string

	// Files changed.
	fileWord := "files"
	if files == 1 {
		fileWord = "file"
	}
	parts = append(parts, fmt.Sprintf("%d %s changed", files, fileWord))

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

	fmt.Fprintln(r.out, strings.Join(parts, ", "))
}
