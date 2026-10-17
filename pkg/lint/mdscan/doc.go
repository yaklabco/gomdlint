// Package mdscan provides byte-level scanners for Markdown inline syntax that
// the parser does not expose as nodes: the individual constructs inside an
// HTML block, and GFM autolink literals in text parsed as CommonMark.
//
// Rules use these scanners on source text the parser has already classified,
// for example to find every element inside an HTML block or to find bare URLs
// in a text node. The scanners never allocate an AST and work on absolute
// byte offsets so callers can map results back to file positions.
package mdscan
