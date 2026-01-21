package fix

import (
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
	OriginalStart int

	// OriginalCount is the number of lines from the original in this hunk.
	OriginalCount int

	// ModifiedStart is the 1-based line number where the hunk starts in the modified.
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

	// Content is the line content (without the diff prefix).
	Content string
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

// GenerateDiff creates a unified diff between original and modified content.
// Returns nil if there are no changes.
func GenerateDiff(path string, original, modified []byte) *Diff {
	if len(original) == 0 && len(modified) == 0 {
		return nil
	}

	// Split into lines.
	origLines := splitLines(original)
	modLines := splitLines(modified)

	// Check if content is identical.
	if linesEqual(origLines, modLines) {
		return nil
	}

	// Compute diff hunks.
	hunks := computeHunks(origLines, modLines)
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
		fmt.Fprintf(&builder, "@@ -%d,%d +%d,%d @@\n",
			hunk.OriginalStart, hunk.OriginalCount,
			hunk.ModifiedStart, hunk.ModifiedCount)

		for _, line := range hunk.Lines {
			switch line.Kind {
			case DiffLineContext:
				fmt.Fprintf(&builder, " %s\n", line.Content)
			case DiffLineAdd:
				fmt.Fprintf(&builder, "+%s\n", line.Content)
			case DiffLineRemove:
				fmt.Fprintf(&builder, "-%s\n", line.Content)
			}
		}
	}

	return builder.String()
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

// splitLines splits content into lines, removing the trailing newline if present.
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	s := string(content)
	lines := strings.Split(s, "\n")

	// Remove trailing empty string if content ends with newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// linesEqual compares two string slices for equality.
func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// computeHunks computes diff hunks using LCS-based algorithm.
func computeHunks(orig, mod []string) []DiffHunk {
	// Compute longest common subsequence.
	lcs := longestCommonSubsequence(orig, mod)

	// Build raw diff operations.
	ops := buildDiffOps(orig, mod, lcs)
	if len(ops) == 0 {
		return nil
	}

	// Group operations into hunks with context.
	return groupIntoHunks(ops, len(orig), len(mod))
}

// diffOp represents a single diff operation.
type diffOp struct {
	kind    DiffLineKind
	content string
	origIdx int // Original line index (-1 for adds).
	modIdx  int // Modified line index (-1 for removes).
}

// buildDiffOps builds a sequence of diff operations from original, modified, and LCS.
func buildDiffOps(orig, mod []string, lcs []string) []diffOp {
	var ops []diffOp
	origIdx, modIdx, lcsIdx := 0, 0, 0

	for origIdx < len(orig) || modIdx < len(mod) {
		// If both match the LCS, it's a context line.
		if lcsIdx < len(lcs) &&
			origIdx < len(orig) && mod != nil && modIdx < len(mod) &&
			orig[origIdx] == lcs[lcsIdx] && mod[modIdx] == lcs[lcsIdx] {
			ops = append(ops, diffOp{
				kind:    DiffLineContext,
				content: orig[origIdx],
				origIdx: origIdx,
				modIdx:  modIdx,
			})
			origIdx++
			modIdx++
			lcsIdx++
			continue
		}

		// Remove lines from original that aren't in LCS.
		for origIdx < len(orig) && (lcsIdx >= len(lcs) || orig[origIdx] != lcs[lcsIdx]) {
			ops = append(ops, diffOp{
				kind:    DiffLineRemove,
				content: orig[origIdx],
				origIdx: origIdx,
				modIdx:  -1,
			})
			origIdx++
		}

		// Add lines from modified that aren't in LCS.
		for modIdx < len(mod) && (lcsIdx >= len(lcs) || mod[modIdx] != lcs[lcsIdx]) {
			ops = append(ops, diffOp{
				kind:    DiffLineAdd,
				content: mod[modIdx],
				origIdx: -1,
				modIdx:  modIdx,
			})
			modIdx++
		}
	}

	return ops
}

// groupIntoHunks groups diff operations into hunks with context lines.
func groupIntoHunks(ops []diffOp, _, _ int) []DiffHunk {
	if len(ops) == 0 {
		return nil
	}

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

	if len(ranges) == 0 {
		return nil
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
		hunk := buildHunk(ops, ranges[rangeIdx].start, ranges[mergeEnd-1].end, len(ops))
		if len(hunk.Lines) > 0 {
			hunks = append(hunks, hunk)
		}

		rangeIdx = mergeEnd
	}

	return hunks
}

// buildHunk builds a single hunk from a range of operations.
func buildHunk(ops []diffOp, changeStart, changeEnd, opsLen int) DiffHunk {
	// Expand to include context lines.
	start := changeStart - contextLines
	if start < 0 {
		start = 0
	}
	end := changeEnd + contextLines
	if end > opsLen {
		end = opsLen
	}

	hunk := DiffHunk{}

	// Find original and modified start positions.
	origStart := 1
	modStart := 1
	for opIdx := range start {
		if ops[opIdx].kind != DiffLineAdd {
			origStart++
		}
		if ops[opIdx].kind != DiffLineRemove {
			modStart++
		}
	}
	hunk.OriginalStart = origStart
	hunk.ModifiedStart = modStart

	// Build lines and count.
	for i := start; i < end; i++ {
		op := ops[i]
		hunk.Lines = append(hunk.Lines, DiffLine{
			Kind:    op.kind,
			Content: op.content,
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

// longestCommonSubsequence computes the LCS of two string slices.
func longestCommonSubsequence(orig, mod []string) []string {
	origLen, modLen := len(orig), len(mod)
	if origLen == 0 || modLen == 0 {
		return nil
	}

	// Build DP table.
	dp := make([][]int, origLen+1)
	for idx := range dp {
		dp[idx] = make([]int, modLen+1)
	}

	for row := 1; row <= origLen; row++ {
		for col := 1; col <= modLen; col++ {
			if orig[row-1] == mod[col-1] {
				dp[row][col] = dp[row-1][col-1] + 1
			} else {
				dp[row][col] = max(dp[row-1][col], dp[row][col-1])
			}
		}
	}

	// Backtrack to find LCS.
	lcsLen := dp[origLen][modLen]
	if lcsLen == 0 {
		return nil
	}

	lcs := make([]string, lcsLen)
	row, col, idx := origLen, modLen, lcsLen-1
	for row > 0 && col > 0 {
		switch {
		case orig[row-1] == mod[col-1]:
			lcs[idx] = orig[row-1]
			row--
			col--
			idx--
		case dp[row-1][col] > dp[row][col-1]:
			row--
		default:
			col--
		}
	}

	return lcs
}
