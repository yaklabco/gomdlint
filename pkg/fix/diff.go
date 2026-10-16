package fix

import (
	"bytes"
	"fmt"
	"strings"
)

// Diff represents a unified diff between original and modified content.
type Diff struct {
	// Path is the file path for the diff header.
	Path string

	// Original is the original file content.
	Original []byte

	// Modified is the modified file content.
	Modified []byte

	// Hunks contains the diff hunks.
	Hunks []DiffHunk

	// Additions is the number of lines added.
	Additions int

	// Deletions is the number of lines deleted.
	Deletions int
}

// DiffHunk represents a single hunk in a unified diff.
type DiffHunk struct {
	// OriginalStart is the 1-based line number where the hunk starts in the original.
	// For a hunk with no original lines it is the line the hunk would start at.
	OriginalStart int

	// OriginalCount is the number of lines from the original in this hunk.
	OriginalCount int

	// ModifiedStart is the 1-based line number where the hunk starts in the modified.
	// For a hunk with no modified lines it is the line the hunk would start at.
	ModifiedStart int

	// ModifiedCount is the number of lines from the modified in this hunk.
	ModifiedCount int

	// Lines contains the diff lines in this hunk.
	Lines []DiffLine
}

// DiffLine represents a single line in a diff hunk.
type DiffLine struct {
	// Kind indicates whether this is a context, add, or remove line.
	Kind DiffLineKind

	// Content is the line content without the diff prefix and without the
	// trailing newline. A carriage return before the newline is kept.
	Content string

	// NoNewline is true when this is the last line of its file and the file
	// does not end with a newline. In unified diff output such a line is
	// followed by the "\ No newline at end of file" marker.
	NoNewline bool
}

// DiffLineKind indicates the type of diff line.
type DiffLineKind int

const (
	// DiffLineContext is an unchanged context line.
	DiffLineContext DiffLineKind = iota

	// DiffLineAdd is a line added in the modified version.
	DiffLineAdd

	// DiffLineRemove is a line removed from the original version.
	DiffLineRemove
)

// contextLines is the number of context lines to show around changes.
const contextLines = 3

// NoNewlineMarker is the unified diff line that follows a line which ends
// its file without a trailing newline.
const NoNewlineMarker = `\ No newline at end of file`

// GenerateDiff creates a unified diff between original and modified content.
// Returns nil if there are no changes.
//
// Lines are compared together with their line terminators, so a change that
// only adds or removes the final newline still produces a hunk. The edit
// script is a shortest one, computed with Myers' algorithm in linear space;
// very large rewrites may fall back to replacing whole regions to bound the
// running time.
func GenerateDiff(path string, original, modified []byte) *Diff {
	if bytes.Equal(original, modified) {
		return nil
	}

	ops := diffLines(splitLines(original), splitLines(modified))
	hunks := groupIntoHunks(ops)
	if len(hunks) == 0 {
		return nil
	}

	// Count additions and deletions.
	var additions, deletions int
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			switch line.Kind {
			case DiffLineAdd:
				additions++
			case DiffLineRemove:
				deletions++
			case DiffLineContext:
			}
		}
	}

	return &Diff{
		Path:      path,
		Original:  original,
		Modified:  modified,
		Hunks:     hunks,
		Additions: additions,
		Deletions: deletions,
	}
}

// GitHeader returns the "diff --git" header line.
func (d *Diff) GitHeader() string {
	if d == nil {
		return ""
	}
	path := strings.TrimPrefix(d.Path, "/")
	return fmt.Sprintf("diff --git a/%s b/%s", path, path)
}

// String returns the diff in unified diff format (without the git header).
func (d *Diff) String() string {
	if d == nil || len(d.Hunks) == 0 {
		return ""
	}

	path := strings.TrimPrefix(d.Path, "/")

	var builder strings.Builder
	fmt.Fprintf(&builder, "--- a/%s\n", path)
	fmt.Fprintf(&builder, "+++ b/%s\n", path)

	for _, hunk := range d.Hunks {
		fmt.Fprintf(&builder, "@@ -%s +%s @@\n",
			hunkRange(hunk.OriginalStart, hunk.OriginalCount),
			hunkRange(hunk.ModifiedStart, hunk.ModifiedCount))

		for _, line := range hunk.Lines {
			switch line.Kind {
			case DiffLineContext:
				builder.WriteByte(' ')
			case DiffLineAdd:
				builder.WriteByte('+')
			case DiffLineRemove:
				builder.WriteByte('-')
			}
			builder.WriteString(line.Content)
			builder.WriteByte('\n')
			if line.NoNewline {
				builder.WriteString(NoNewlineMarker)
				builder.WriteByte('\n')
			}
		}
	}

	return builder.String()
}

// hunkRange formats one side of a hunk header. An empty range names the line
// before it (0 at the start of the file), as GNU diff and git do.
func hunkRange(start, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", start-1)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

// FullString returns the complete diff including the git header.
func (d *Diff) FullString() string {
	if d == nil || len(d.Hunks) == 0 {
		return ""
	}
	return d.GitHeader() + "\n" + d.String()
}

// HasChanges returns true if the diff contains any changes.
func (d *Diff) HasChanges() bool {
	return d != nil && len(d.Hunks) > 0
}

// splitLines splits content into lines that keep their trailing newline.
// The last line has no newline when content does not end with one.
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	text := string(content)
	lines := make([]string, 0, strings.Count(text, "\n")+1)
	for text != "" {
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			lines = append(lines, text)
			break
		}
		lines = append(lines, text[:idx+1])
		text = text[idx+1:]
	}
	return lines
}

// diffOp represents a single diff operation.
type diffOp struct {
	kind DiffLineKind
	// line is the full line, including its newline if it has one.
	line string
}

// diffLines returns the operations that turn orig into mod. Within each
// changed region, removals come before additions.
func diffLines(orig, mod []string) []diffOp {
	origIDs, modIDs := internLines(orig, mod)
	removed, added := diffLineIDs(origIDs, modIDs)

	ops := make([]diffOp, 0, max(len(orig), len(mod)))
	origIdx, modIdx := 0, 0
	for origIdx < len(orig) || modIdx < len(mod) {
		switch {
		case origIdx < len(orig) && (removed[origIdx] || modIdx == len(mod)):
			ops = append(ops, diffOp{kind: DiffLineRemove, line: orig[origIdx]})
			origIdx++
		case modIdx < len(mod) && (added[modIdx] || origIdx == len(orig)):
			ops = append(ops, diffOp{kind: DiffLineAdd, line: mod[modIdx]})
			modIdx++
		default:
			ops = append(ops, diffOp{kind: DiffLineContext, line: orig[origIdx]})
			origIdx++
			modIdx++
		}
	}
	return ops
}

// internLines maps each distinct line to a small integer so the diff
// algorithm compares integers instead of strings.
func internLines(orig, mod []string) ([]int, []int) {
	ids := make(map[string]int, len(orig)+len(mod))
	toIDs := func(lines []string) []int {
		out := make([]int, len(lines))
		for idx, line := range lines {
			id, ok := ids[line]
			if !ok {
				id = len(ids)
				ids[line] = id
			}
			out[idx] = id
		}
		return out
	}
	return toIDs(orig), toIDs(mod)
}

// groupIntoHunks groups diff operations into hunks with context lines.
func groupIntoHunks(ops []diffOp) []DiffHunk {
	// Find ranges of changes (non-context lines).
	type changeRange struct {
		start, end int // Indices into ops.
	}

	var ranges []changeRange
	inChange := false
	rangeStart := 0

	for opIdx, op := range ops {
		isChange := op.kind != DiffLineContext
		if isChange && !inChange {
			rangeStart = opIdx
			inChange = true
		} else if !isChange && inChange {
			ranges = append(ranges, changeRange{rangeStart, opIdx})
			inChange = false
		}
	}
	if inChange {
		ranges = append(ranges, changeRange{rangeStart, len(ops)})
	}

	// Merge ranges that are close together and build hunks.
	var hunks []DiffHunk
	for rangeIdx := 0; rangeIdx < len(ranges); {
		// Find contiguous ranges to merge.
		mergeEnd := rangeIdx + 1
		for mergeEnd < len(ranges) {
			gap := ranges[mergeEnd].start - ranges[mergeEnd-1].end
			if gap > contextLines*2 {
				break
			}
			mergeEnd++
		}

		// Build hunk from ranges[rangeIdx] to ranges[mergeEnd-1].
		hunks = append(hunks, buildHunk(ops, ranges[rangeIdx].start, ranges[mergeEnd-1].end))
		rangeIdx = mergeEnd
	}

	return hunks
}

// buildHunk builds a single hunk from a range of operations.
func buildHunk(ops []diffOp, changeStart, changeEnd int) DiffHunk {
	// Expand to include context lines.
	start := max(changeStart-contextLines, 0)
	end := min(changeEnd+contextLines, len(ops))

	// Find original and modified start positions.
	hunk := DiffHunk{OriginalStart: 1, ModifiedStart: 1}
	for _, op := range ops[:start] {
		if op.kind != DiffLineAdd {
			hunk.OriginalStart++
		}
		if op.kind != DiffLineRemove {
			hunk.ModifiedStart++
		}
	}

	// Build lines and count.
	hunk.Lines = make([]DiffLine, 0, end-start)
	for _, op := range ops[start:end] {
		content, hasNewline := strings.CutSuffix(op.line, "\n")
		hunk.Lines = append(hunk.Lines, DiffLine{
			Kind:      op.kind,
			Content:   content,
			NoNewline: !hasNewline,
		})

		switch op.kind {
		case DiffLineContext:
			hunk.OriginalCount++
			hunk.ModifiedCount++
		case DiffLineRemove:
			hunk.OriginalCount++
		case DiffLineAdd:
			hunk.ModifiedCount++
		}
	}

	return hunk
}
