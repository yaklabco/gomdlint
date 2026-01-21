package goldmark

import (
	"sort"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark/ast"
)

// assignTokenRanges assigns FirstToken and LastToken indices to all nodes in the tree.
// It uses the goldmark AST for byte offset information and maps those to token indices.
func assignTokenRanges(root *mdast.Node, gmRoot ast.Node, tokens []mdast.Token, content []byte) {
	if root == nil || len(tokens) == 0 {
		return
	}

	// Build a parallel walk of both trees.
	assignTokenRangesRecursive(root, gmRoot, tokens, content)
}

// assignTokenRangesRecursive recursively assigns token ranges to nodes.
func assignTokenRangesRecursive(node *mdast.Node, gmNode ast.Node, tokens []mdast.Token, content []byte) {
	if node == nil {
		return
	}

	// Get byte range from goldmark node.
	start, end := getNodeByteRange(gmNode, content)

	if start >= 0 && end >= 0 && start <= end {
		// Find token indices for this byte range.
		firstToken := findTokenAtOffset(tokens, start)
		lastToken := findLastTokenAtOffset(tokens, end)

		if firstToken >= 0 && lastToken >= 0 {
			mdast.SetTokenRange(node, firstToken, lastToken)
		}
	}

	// Process children in parallel.
	mdChild := node.FirstChild
	gmChild := gmNode.FirstChild()

	for mdChild != nil && gmChild != nil {
		assignTokenRangesRecursive(mdChild, gmChild, tokens, content)
		mdChild = mdChild.Next
		gmChild = gmChild.NextSibling()
	}
}

// findTokenAtOffset finds the token index that contains or starts at the given byte offset.
// Returns -1 if no token is found.
func findTokenAtOffset(tokens []mdast.Token, offset int) int {
	if len(tokens) == 0 || offset < 0 {
		return -1
	}

	// Binary search for the token containing this offset.
	idx := sort.Search(len(tokens), func(i int) bool {
		return tokens[i].EndOffset > offset
	})

	if idx < len(tokens) && tokens[idx].StartOffset <= offset {
		return idx
	}

	// If exact match not found, return the first token at or after offset.
	if idx < len(tokens) {
		return idx
	}

	return -1
}

// findLastTokenAtOffset finds the last token index that ends at or before the given byte offset.
// Returns -1 if no token is found.
func findLastTokenAtOffset(tokens []mdast.Token, offset int) int {
	if len(tokens) == 0 || offset < 0 {
		return -1
	}

	// Binary search for the token that ends at or just before this offset.
	idx := sort.Search(len(tokens), func(i int) bool {
		return tokens[i].EndOffset >= offset
	})

	// Adjust to get the token that ends at or just before offset.
	if idx < len(tokens) && tokens[idx].EndOffset == offset {
		return idx
	}

	if idx > 0 {
		return idx - 1
	}

	return 0
}

// computeDocumentTokenRange computes the token range for the document root.
// The document spans all tokens.
func computeDocumentTokenRange(root *mdast.Node, tokens []mdast.Token) {
	if root == nil || len(tokens) == 0 {
		return
	}

	if root.Kind == mdast.NodeDocument {
		mdast.SetTokenRange(root, 0, len(tokens)-1)
	}
}

// propagateTokenRanges ensures all nodes have valid token ranges by propagating
// from children to parents where needed.
func propagateTokenRanges(root *mdast.Node) {
	if root == nil {
		return
	}

	// Post-order traversal: process children first.
	for child := root.FirstChild; child != nil; child = child.Next {
		propagateTokenRanges(child)
	}

	// If this node has no token range but has children with ranges, compute from children.
	if root.FirstToken < 0 || root.LastToken < 0 {
		first := -1
		last := -1

		for child := root.FirstChild; child != nil; child = child.Next {
			if child.FirstToken >= 0 && (first < 0 || child.FirstToken < first) {
				first = child.FirstToken
			}
			if child.LastToken >= 0 && (last < 0 || child.LastToken > last) {
				last = child.LastToken
			}
		}

		if first >= 0 && last >= 0 {
			mdast.SetTokenRange(root, first, last)
		}
	}
}

// TokenRangeAssigner handles the assignment of token ranges to AST nodes.
type TokenRangeAssigner struct {
	tokens  []mdast.Token
	content []byte
}

// NewTokenRangeAssigner creates a new TokenRangeAssigner.
func NewTokenRangeAssigner(tokens []mdast.Token, content []byte) *TokenRangeAssigner {
	return &TokenRangeAssigner{
		tokens:  tokens,
		content: content,
	}
}

// AssignRanges assigns token ranges to all nodes in the mdast tree using the goldmark AST.
func (a *TokenRangeAssigner) AssignRanges(root *mdast.Node, gmRoot ast.Node) {
	if root == nil || len(a.tokens) == 0 {
		return
	}

	// First, assign ranges based on goldmark node positions.
	assignTokenRanges(root, gmRoot, a.tokens, a.content)

	// Set document range to cover all tokens.
	computeDocumentTokenRange(root, a.tokens)

	// Propagate ranges from children to parents where needed.
	propagateTokenRanges(root)
}

// FindTokensInRange returns all token indices within the given byte range.
func (a *TokenRangeAssigner) FindTokensInRange(start, end int) (int, int) {
	first := findTokenAtOffset(a.tokens, start)
	last := findLastTokenAtOffset(a.tokens, end)
	return first, last
}
