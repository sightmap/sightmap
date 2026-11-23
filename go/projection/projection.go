// Package projection reduces a native mobile component tree to the nodes a
// person or an agent can perceive or operate, so that two producers that see
// the same screen differently (the iOS Simulator's accessibility tree through
// `sightmap device`, and a captured session's layer tree through lidar) publish
// the same tree for it.
//
// The rule it implements: a node is a component iff it can be perceived or
// operated as its own thing. That is a control, a node with text of its own, a
// node that carries an app identifier, or a named structural container (a tab
// list, a navigation bar). Everything else is scaffolding, and its children
// move up to its parent. A producer may add nodes the other cannot see (a
// decoration the accessibility tree hides), but never invents a role.
//
// Producers adapt their tree to Node first: Kind is one of the roles below,
// and ID must be an identifier the app set, not one the platform derived.
// Project returns a new tree and leaves its input alone. A node's ID and Text
// in the result may come from a node the rules folded into it; its Ref stays
// the survivor's own.
package projection

// Kinds the rules distinguish. A producer may use others; they are treated as
// opaque containers or leaves.
const (
	KindRoot       = "root"
	KindGroup      = "group"
	KindText       = "text"
	KindHeading    = "heading"
	KindImage      = "img"
	KindButton     = "button"
	KindLink       = "link"
	KindTab        = "tab"
	KindTabList    = "tablist"
	KindTextbox    = "textbox"
	KindSwitch     = "switch"
	KindSlider     = "slider"
	KindCheckbox   = "checkbox"
	KindRadio      = "radio"
	KindList       = "list"
	KindSection    = "section"
	KindNavigation = "navigation"
)

// Node is a component in the neutral shape both producers convert to.
type Node struct {
	Kind string  `json:"kind"`
	ID   string  `json:"id"`
	Text string  `json:"text"`
	Kids []*Node `json:"kids"`
	// Ref is the producer's own element, carried through untouched: a node
	// that survives keeps its Ref, and a node the rules build from another
	// (a group that becomes its button) takes the Ref of the one it stands for.
	// The package never reads it.
	Ref any `json:"-"`
}

// isControl reports whether kind is something a person operates.
func isControl(kind string) bool {
	switch kind {
	case KindButton, KindTab, KindLink, KindTextbox, KindSwitch, KindSlider, KindCheckbox, KindRadio:
		return true
	}
	return false
}

// Project applies the rules to root and returns the projected tree.
func Project(root *Node) *Node {
	if root == nil {
		return nil
	}
	p := fold(clone(root))
	if p.Kind == splice {
		return &Node{Kind: KindRoot, Kids: p.Kids}
	}
	return p
}

func clone(n *Node) *Node {
	c := *n
	c.Kids = make([]*Node, len(n.Kids))
	for i, k := range n.Kids {
		c.Kids[i] = clone(k)
	}
	return &c
}

// splice marks a node that dissolves into its parent, leaving its children.
const splice = "\x00splice"

func isLeaf(n *Node) bool { return len(n.Kids) == 0 }

// fold applies the rules bottom-up. Each rule is local: it looks at a node, its
// children and, for the hand-offs, the one child involved.
func fold(n *Node) *Node {
	var kids []*Node
	for _, k := range n.Kids {
		k = fold(k)
		if k.Kind == splice {
			kids = append(kids, k.Kids...)
		} else {
			kids = append(kids, k)
		}
	}
	n.Kids = kids

	// A control's icon is part of the control, not a component.
	if isControl(n.Kind) {
		n.Kids = keep(n.Kids, func(k *Node) bool { return !(k.Kind == KindImage && k.ID == "" && isLeaf(k)) })
	}

	// A leaf that repeats an earlier sibling exactly adds nothing.
	type key struct{ kind, id, text string }
	seen := map[key]bool{}
	var uniq []*Node
	for _, k := range n.Kids {
		kk := key{k.Kind, k.ID, k.Text}
		if seen[kk] && isLeaf(k) {
			continue
		}
		seen[kk] = true
		uniq = append(uniq, k)
	}
	n.Kids = uniq

	// An anonymous text or same-kind leaf that only says what its parent says.
	if n.Kind != KindRoot {
		n.Kids = keep(n.Kids, func(k *Node) bool {
			return !((k.Kind == KindText || k.Kind == n.Kind) && isLeaf(k) && k.ID == "" && k.Text == n.Text)
		})
	}

	// A control whose only content is one anonymous text takes the text.
	if isControl(n.Kind) && n.Text == "" && len(n.Kids) == 1 {
		if k := n.Kids[0]; k.Kind == KindText && isLeaf(k) && k.ID == "" {
			n.Text, n.Kids = k.Text, nil
		}
	}

	// An id-only group over one anonymous node hands its id to it.
	if n.Kind == KindGroup && n.ID != "" && n.Text == "" && len(n.Kids) == 1 && n.Kids[0].ID == "" {
		c := n.Kids[0]
		c.ID = n.ID
		return c
	}

	// A parent and its only child with the same id are one node.
	if n.ID != "" && len(n.Kids) == 1 && n.Kids[0].ID == n.ID {
		c := n.Kids[0]
		if c.Text == "" {
			c.Text = n.Text
		}
		return c
	}

	// A parent and its only child of the same kind with the same (or no) text
	// are one node, unless both carry different ids.
	if len(n.Kids) == 1 {
		c := n.Kids[0]
		if c.Kind == n.Kind && (c.Text == "" || c.Text == n.Text) && !(n.ID != "" && c.ID != "" && n.ID != c.ID) {
			if c.ID == "" {
				c.ID = n.ID
			}
			if c.Text == "" {
				c.Text = n.Text
			}
			return c
		}
	}

	// An unnamed group is scaffolding.
	if n.Kind == KindGroup && n.ID == "" && n.Text == "" && len(n.Kids) > 0 {
		return &Node{Kind: splice, Kids: n.Kids}
	}

	// A navigation bar is named by its heading, and drops a nested copy of itself.
	if n.Kind == KindNavigation {
		if n.Text == "" {
			for i, k := range n.Kids {
				if k.Kind == KindHeading {
					n.Text = k.Text
					n.Kids = append(append([]*Node{}, n.Kids[:i]...), n.Kids[i+1:]...)
					break
				}
			}
		}
		n.Kids = keep(n.Kids, func(k *Node) bool { return !(k.Kind == KindNavigation && k.Text == n.Text) })
	}

	// A leaf group with an id and text is a text node that carries the id.
	if n.Kind == KindGroup && n.ID != "" && n.Text != "" && isLeaf(n) {
		n.Kind = KindText
	}

	// An id group of identical (or unnamed) buttons is that one button.
	if n.Kind == KindGroup && n.ID != "" && len(n.Kids) > 0 {
		all, text := true, ""
		names := map[string]bool{}
		for _, k := range n.Kids {
			if k.Kind != KindButton || !isLeaf(k) {
				all = false
				break
			}
			if k.Text != "" {
				names[k.Text] = true
				text = k.Text
			}
		}
		if all && len(names) <= 1 {
			if text == "" {
				text = n.Text
			}
			return &Node{Kind: KindButton, ID: n.ID, Text: text, Ref: n.Ref}
		}
	}

	// An unnamed list or section is scaffolding; a lone text keeps its text.
	if (n.Kind == KindList || n.Kind == KindSection) && n.ID == "" {
		switch {
		case len(n.Kids) > 0:
			return &Node{Kind: splice, Kids: n.Kids}
		case n.Text != "":
			return &Node{Kind: KindText, Text: n.Text, Ref: n.Ref}
		default:
			return &Node{Kind: splice}
		}
	}

	// An id group over one control with the same label hands its id to the control.
	if n.Kind == KindGroup && n.ID != "" && len(n.Kids) == 1 && isControl(n.Kids[0].Kind) && n.Kids[0].Text == n.Text {
		c := n.Kids[0]
		c.ID = n.ID
		return c
	}

	// An anonymous control wrapping one control is that control.
	if isControl(n.Kind) && n.ID == "" && n.Text == "" && len(n.Kids) == 1 && isControl(n.Kids[0].Kind) {
		return n.Kids[0]
	}

	// An anonymous node over one child of its own kind is that child.
	if n.ID == "" && n.Text == "" && len(n.Kids) == 1 && n.Kids[0].Kind == n.Kind {
		return n.Kids[0]
	}

	// A bare group over one child is that child.
	if n.Kind == KindGroup && n.ID == "" && n.Text == "" && len(n.Kids) == 1 {
		return n.Kids[0]
	}
	return n
}

func keep(nodes []*Node, f func(*Node) bool) []*Node {
	var out []*Node
	for _, n := range nodes {
		if f(n) {
			out = append(out, n)
		}
	}
	return out
}
