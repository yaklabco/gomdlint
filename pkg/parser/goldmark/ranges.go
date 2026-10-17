package goldmark

import (
	"sort"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// assignTokenRanges sets the token range of every node to the tokens that
// overlap its source span. Spans come from goldmark, never from the token
// stream, so a token boundary can never move a node. FirstToken never
// exceeds LastToken; an empty span maps to the token at its offset.
func assignTokenRanges(root *mdast.Node, tokens []mdast.Token) {
	if root == nil || len(tokens) == 0 {
		return
	}
	//nolint:errcheck // The visitor never returns an error.
	mdast.Walk(root, func(node *mdast.Node) error {
		if node.SourceSpan != nil {
			first, last := tokenRangeFor(tokens, *node.SourceSpan)
			mdast.SetTokenRange(node, first, last)
		}
		return nil
	})
}

// tokenRangeFor returns the indices of the first and last tokens that
// overlap span.
func tokenRangeFor(tokens []mdast.Token, span mdast.SourceRange) (int, int) {
	first := tokenAt(tokens, span.StartOffset)
	last := first
	if span.EndOffset > span.StartOffset {
		last = max(first, tokenAt(tokens, span.EndOffset-1))
	}
	return first, last
}

// tokenAt returns the index of the token containing offset. Offsets at or
// past the end of the content map to the last token.
func tokenAt(tokens []mdast.Token, offset int) int {
	idx := sort.Search(len(tokens), func(i int) bool {
		return tokens[i].EndOffset > offset
	})
	return min(idx, len(tokens)-1)
}
