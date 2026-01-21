package mdast

// WalkFunc is the function signature for Walk callbacks.
// Return a non-nil error to stop the walk.
type WalkFunc func(n *Node) error

// Walk performs a pre-order traversal of the AST starting at root.
// The callback walkFunc is called for each node. If walkFunc returns a non-nil error,
// the walk stops immediately and returns that error.
func Walk(root *Node, walkFunc WalkFunc) error {
	if root == nil {
		return nil
	}

	// Visit the current node.
	if err := walkFunc(root); err != nil {
		return err
	}

	// Visit children.
	for child := root.FirstChild; child != nil; child = child.Next {
		if err := Walk(child, walkFunc); err != nil {
			return err
		}
	}

	return nil
}

// WalkContextFunc is the function signature for WalkWithContext callbacks.
// The enter callback is called before visiting children.
// The leave callback is called after visiting children.
// Return a non-nil error from either to stop the walk.
type WalkContextFunc func(n *Node) error

// WalkWithContext performs a traversal with enter and leave callbacks.
// Enter is called before visiting children, leave is called after.
// Either callback may be nil.
func WalkWithContext(root *Node, enter, leave WalkContextFunc) error {
	if root == nil {
		return nil
	}

	// Enter the current node.
	if enter != nil {
		if err := enter(root); err != nil {
			return err
		}
	}

	// Visit children.
	for child := root.FirstChild; child != nil; child = child.Next {
		if err := WalkWithContext(child, enter, leave); err != nil {
			return err
		}
	}

	// Leave the current node.
	if leave != nil {
		if err := leave(root); err != nil {
			return err
		}
	}

	return nil
}

// WalkBlocks walks only block-level nodes.
func WalkBlocks(root *Node, fn WalkFunc) error {
	return Walk(root, func(n *Node) error {
		if n.IsBlock() {
			return fn(n)
		}
		return nil
	})
}

// WalkInlines walks only inline-level nodes.
func WalkInlines(root *Node, fn WalkFunc) error {
	return Walk(root, func(n *Node) error {
		if n.IsInline() {
			return fn(n)
		}
		return nil
	})
}

// FindAll returns all nodes matching the predicate.
func FindAll(root *Node, predicate func(n *Node) bool) []*Node {
	var result []*Node

	//nolint:errcheck,revive // Walk only returns nil errors in this usage
	Walk(root, func(node *Node) error {
		if predicate(node) {
			result = append(result, node)
		}
		return nil
	})

	return result
}

// FindFirst returns the first node matching the predicate, or nil if none found.
func FindFirst(root *Node, predicate func(n *Node) bool) *Node {
	var found *Node

	//nolint:errcheck,revive // errStopWalk is expected and intentionally ignored
	Walk(root, func(node *Node) error {
		if predicate(node) {
			found = node
			return errStopWalk
		}
		return nil
	})

	return found
}

// FindByKind returns all nodes of the specified kind.
func FindByKind(root *Node, kind NodeKind) []*Node {
	return FindAll(root, func(n *Node) bool {
		return n.Kind == kind
	})
}

// errStopWalk is a sentinel error used to stop walking early.
var errStopWalk = &stopWalkError{}

type stopWalkError struct{}

func (e *stopWalkError) Error() string {
	return "stop walk"
}
