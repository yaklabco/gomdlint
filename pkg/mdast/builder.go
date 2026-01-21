package mdast

// NewNode creates a new node of the specified kind.
// The node has no parent, children, or token associations.
func NewNode(kind NodeKind) *Node {
	return &Node{
		Kind:       kind,
		FirstToken: -1,
		LastToken:  -1,
	}
}

// NewDocument creates a new document root node.
func NewDocument() *Node {
	return NewNode(NodeDocument)
}

// AppendChild appends a child node to a parent.
// It maintains the parent/child/sibling relationships correctly.
func AppendChild(parent, child *Node) {
	if parent == nil || child == nil {
		return
	}

	// Remove from previous parent if any.
	if child.Parent != nil {
		RemoveChild(child.Parent, child)
	}

	child.Parent = parent
	child.Prev = parent.LastChild
	child.Next = nil

	if parent.LastChild != nil {
		parent.LastChild.Next = child
	} else {
		parent.FirstChild = child
	}

	parent.LastChild = child
}

// PrependChild prepends a child node to a parent.
func PrependChild(parent, child *Node) {
	if parent == nil || child == nil {
		return
	}

	// Remove from previous parent if any.
	if child.Parent != nil {
		RemoveChild(child.Parent, child)
	}

	child.Parent = parent
	child.Prev = nil
	child.Next = parent.FirstChild

	if parent.FirstChild != nil {
		parent.FirstChild.Prev = child
	} else {
		parent.LastChild = child
	}

	parent.FirstChild = child
}

// InsertBefore inserts newNode before sibling.
// sibling must have a parent.
func InsertBefore(sibling, newNode *Node) {
	if sibling == nil || newNode == nil || sibling.Parent == nil {
		return
	}

	parent := sibling.Parent

	// Remove newNode from its current parent if any.
	if newNode.Parent != nil {
		RemoveChild(newNode.Parent, newNode)
	}

	newNode.Parent = parent
	newNode.Prev = sibling.Prev
	newNode.Next = sibling

	if sibling.Prev != nil {
		sibling.Prev.Next = newNode
	} else {
		parent.FirstChild = newNode
	}

	sibling.Prev = newNode
}

// InsertAfter inserts newNode after sibling.
// sibling must have a parent.
func InsertAfter(sibling, newNode *Node) {
	if sibling == nil || newNode == nil || sibling.Parent == nil {
		return
	}

	parent := sibling.Parent

	// Remove newNode from its current parent if any.
	if newNode.Parent != nil {
		RemoveChild(newNode.Parent, newNode)
	}

	newNode.Parent = parent
	newNode.Prev = sibling
	newNode.Next = sibling.Next

	if sibling.Next != nil {
		sibling.Next.Prev = newNode
	} else {
		parent.LastChild = newNode
	}

	sibling.Next = newNode
}

// RemoveChild removes a child from its parent.
func RemoveChild(parent, child *Node) {
	if parent == nil || child == nil || child.Parent != parent {
		return
	}

	if child.Prev != nil {
		child.Prev.Next = child.Next
	} else {
		parent.FirstChild = child.Next
	}

	if child.Next != nil {
		child.Next.Prev = child.Prev
	} else {
		parent.LastChild = child.Prev
	}

	child.Parent = nil
	child.Prev = nil
	child.Next = nil
}

// ReplaceChild replaces oldChild with newChild in the tree.
func ReplaceChild(parent, oldChild, newChild *Node) {
	if parent == nil || oldChild == nil || newChild == nil {
		return
	}

	if oldChild.Parent != parent {
		return
	}

	// Remove newChild from its current parent if any.
	if newChild.Parent != nil {
		RemoveChild(newChild.Parent, newChild)
	}

	newChild.Parent = parent
	newChild.Prev = oldChild.Prev
	newChild.Next = oldChild.Next

	if oldChild.Prev != nil {
		oldChild.Prev.Next = newChild
	} else {
		parent.FirstChild = newChild
	}

	if oldChild.Next != nil {
		oldChild.Next.Prev = newChild
	} else {
		parent.LastChild = newChild
	}

	oldChild.Parent = nil
	oldChild.Prev = nil
	oldChild.Next = nil
}

// SetTokenRange sets the token range for a node.
func SetTokenRange(n *Node, first, last int) {
	if n == nil {
		return
	}
	n.FirstToken = first
	n.LastToken = last
}

// SetFile sets the file reference for a node and all its descendants.
func SetFile(node *Node, file *FileSnapshot) {
	if node == nil {
		return
	}

	//nolint:errcheck,revive // Walk only returns nil errors in this usage
	Walk(node, func(child *Node) error {
		child.File = file
		return nil
	})
}
