package lint

import "github.com/yaklabco/gomdlint/pkg/mdast"

// Source structure helpers built on the node span contract documented in
// package mdast.

// CodeContentLines returns the 1-based numbers of the lines that hold code:
// every line of an indented code block, and the lines between the fences of
// a fenced code block. The fence lines themselves are not included; use the
// full span of a code block (or RuleContext.IsLineInCodeBlock) to cover them.
func CodeContentLines(blocks []*mdast.Node) map[int]bool {
	lines := make(map[int]bool)
	for _, block := range blocks {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		first, last := pos.StartLine, pos.EndLine
		if IsFencedCodeBlock(block) {
			first++
			if block.Block.CodeBlock.Closed {
				last--
			}
		}
		for line := first; line <= last; line++ {
			lines[line] = true
		}
	}
	return lines
}
