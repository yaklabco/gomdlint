package lint

import (
	"context"

	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// Parser parses Markdown content into a FileSnapshot.
//
// The lint package defines this interface to follow the gobible principle
// of defining interfaces in the consumer package. Implementations (e.g.,
// parser/goldmark) provide the concrete parsing logic.
//
// Implementations must be:
//   - deterministic for a given (flavor, path, content) tuple,
//   - safe for concurrent use by multiple goroutines, if documented as such,
//   - side-effect free (no I/O, no global state mutation).
type Parser interface {
	// Parse converts raw Markdown bytes into a fully-populated FileSnapshot.
	//
	// Parameters:
	//   - ctx: context for cancellation and timeout propagation.
	//   - path: logical file path (for diagnostics; must not be used for I/O).
	//   - content: raw Markdown bytes (must not be mutated by the implementation).
	//
	// Returns:
	//   - On success: a fully-populated FileSnapshot with valid tokens and AST.
	//   - On error: nil and a descriptive error; no partial snapshot is returned.
	//
	// The returned FileSnapshot must satisfy:
	//   - snapshot.Path == path
	//   - bytes.Equal(snapshot.Content, content)
	//   - mdast.ValidateTokens(snapshot.Tokens, len(snapshot.Content)) == true
	//   - snapshot.Root != nil && snapshot.Root.Kind == mdast.NodeDocument
	//   - All nodes have node.File == snapshot
	Parse(ctx context.Context, path string, content []byte) (*mdast.FileSnapshot, error)
}
