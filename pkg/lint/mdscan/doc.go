// Package mdscan provides byte-level scanners for CommonMark inline syntax:
// raw HTML constructs, autolinks, code spans, backslash escapes, and the
// brackets that delimit link text and link labels.
//
// Rules use these scanners on source text the parser has already classified,
// for example to find every element inside an HTML block, to locate the
// closing bracket of a link's text, or to tell whether a URL sits inside a
// code span. The scanners never allocate an AST and work on absolute byte
// offsets so callers can map results back to file positions.
package mdscan
