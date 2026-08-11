package sightmap

// Predicate tests whether a node should be included in a filtered tree.
// Returns (include bool, descend bool):
//
//	include=true  → keep this node in the output
//	descend=true  → recurse into children
//
// Both can be true, false, or any combination.
type Predicate func(node *ComponentNode, depth int) (include, descend bool)

// IsInteractive returns a Predicate that matches nodes where IsInteractive is true.
func IsInteractive() Predicate {
	return func(node *ComponentNode, depth int) (bool, bool) {
		return node.IsInteractive, true
	}
}

// InViewport returns a Predicate that matches nodes where InViewport is true.
func InViewport() Predicate {
	return func(node *ComponentNode, depth int) (bool, bool) {
		return node.InViewport, true
	}
}

// IsVisible returns a Predicate that matches nodes where IsVisible is true.
func IsVisible() Predicate {
	return func(node *ComponentNode, depth int) (bool, bool) {
		return node.IsVisible, true
	}
}

// HasBounds returns a Predicate that matches nodes with a non-nil Bounds
// where both Width and Height are positive.
func HasBounds() Predicate {
	return func(node *ComponentNode, depth int) (bool, bool) {
		if node.Bounds == nil {
			return false, true
		}
		return node.Bounds.Width > 0 && node.Bounds.Height > 0, true
	}
}

// And returns a Predicate that is true only when all of the given predicates
// match. include is true iff every predicate returns include=true; descend is
// true only if every predicate returns descend=true.
func And(preds ...Predicate) Predicate {
	return func(node *ComponentNode, depth int) (bool, bool) {
		include := true
		descend := true
		for _, p := range preds {
			inc, desc := p(node, depth)
			if !inc {
				include = false
			}
			if !desc {
				descend = false
			}
		}
		return include, descend
	}
}

// Or returns a Predicate that is true when any of the given predicates match.
// include is true if any predicate returns include=true; descend is true if
// any predicate returns descend=true.
func Or(preds ...Predicate) Predicate {
	return func(node *ComponentNode, depth int) (bool, bool) {
		include := false
		descend := false
		for _, p := range preds {
			inc, desc := p(node, depth)
			if inc {
				include = true
			}
			if desc {
				descend = true
			}
		}
		return include, descend
	}
}

// Not returns a Predicate that negates the include result of p while
// preserving its descend value.
func Not(p Predicate) Predicate {
	return func(node *ComponentNode, depth int) (bool, bool) {
		inc, desc := p(node, depth)
		return !inc, desc
	}
}

// Walk visits every node depth-first (pre-order). fn receives the node and
// its depth (root = 0). Return false from fn to skip descending into
// that node's children.
func Walk(root *ComponentNode, fn func(*ComponentNode, int) bool) {
	walkNode(root, 0, fn)
}

func walkNode(node *ComponentNode, depth int, fn func(*ComponentNode, int) bool) {
	if !fn(node, depth) {
		return
	}
	for _, child := range node.Children {
		walkNode(child, depth+1, fn)
	}
}

// Collect returns a flat slice of all nodes for which pred returns include=true,
// in DFS pre-order. When pred returns descend=false for a node, its children
// are not visited.
func Collect(root *ComponentNode, pred Predicate) []*ComponentNode {
	var result []*ComponentNode
	Walk(root, func(node *ComponentNode, depth int) bool {
		inc, desc := pred(node, depth)
		if inc {
			result = append(result, node)
		}
		return desc
	})
	return result
}

// Filter returns a pruned shallow copy of the tree keeping only nodes for
// which pred returns include=true plus any structural connector ancestors
// required to attach them to the root. Returns nil if no nodes match.
//
// Rules:
//   - A node is included if pred returns include=true, OR if it has at least
//     one included descendant (structural connector).
//   - If pred returns descend=false for a node, its children are not traversed
//     or included.
//   - Included nodes are shallow copies (all fields copied, Children replaced
//     by the filtered child list).
func Filter(root *ComponentNode, pred Predicate) *ComponentNode {
	node, ok := filterNode(root, 0, pred)
	if !ok {
		return nil
	}
	return node
}

// filterNode recursively filters the subtree rooted at node.
// Returns the (possibly trimmed) copy and whether it should be included in
// the parent's child list.
func filterNode(node *ComponentNode, depth int, pred Predicate) (*ComponentNode, bool) {
	inc, desc := pred(node, depth)

	var filteredChildren []*ComponentNode
	if desc {
		for _, child := range node.Children {
			if filtered, ok := filterNode(child, depth+1, pred); ok {
				filteredChildren = append(filteredChildren, filtered)
			}
		}
	}

	// Include if directly matched, or as a structural connector.
	if !inc && len(filteredChildren) == 0 {
		return nil, false
	}

	// Shallow copy with replaced Children.
	out := *node
	out.Children = filteredChildren
	return &out, true
}
