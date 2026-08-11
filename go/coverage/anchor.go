package coverage

import (
	"fmt"
	"strings"

	"github.com/sightmap/sightmap/go/comps"
)

// The anchor helpers locate the nearest stable (data-attr-bearing) ancestor of a
// node and format it. They back three surfaces that must agree: the coverage
// "Unlabeled clusters" trace, the `gap` command, and the view-set novelty gate's
// orphan-slot identity. Changing the anchoring heuristic here changes all three.

// NearestDataAttrAncestor walks up parentMap to the nearest ancestor whose
// selector carries a data-testid or data-component attribute, or nil if none.
func NearestDataAttrAncestor(node *comps.ComponentNode, parentMap ParentMap) *comps.ComponentNode {
	curr := parentMap[node]
	for curr != nil {
		if curr.Element != nil {
			a := curr.Element.Attrs
			if a["data-testid"] != "" || a["data-component"] != "" {
				return curr
			}
		}
		curr = parentMap[curr]
	}
	return nil
}

// DataAttrSelector formats an ancestor node for an "inside:" display line.
// Prefers data-testid, then data-component, then falls back to tag#id.cls.
func DataAttrSelector(node *comps.ComponentNode) string {
	if node.Element == nil {
		return node.Role
	}
	tag := node.Element.Tag
	if tag == "" {
		tag = node.Role
	}
	a := node.Element.Attrs
	if dt := a["data-testid"]; dt != "" {
		return fmt.Sprintf(`%s[data-testid="%s"]`, tag, dt)
	}
	if dc := a["data-component"]; dc != "" {
		return fmt.Sprintf(`%s[data-component^="%s"]`, tag, dc)
	}
	return SelectorString(node.Element)
}

// OrphanSlotKey is the cross-capture STRUCTURAL identity of an uncovered
// interactive node, used by the view-set novelty gate to decide whether a
// capture introduces a new "slot". It is the node's role plus the selector of
// its nearest data-attr ancestor — deliberately WITHOUT the node's instance text,
// so different instances of the same widget (e.g. each product link in a
// carousel) collapse to one slot and don't read as endless novelty.
func OrphanSlotKey(node *comps.ComponentNode, parentMap ParentMap) string {
	role := node.Role
	if role == "" {
		role = "?"
	}
	inside := "(no stable ancestor)"
	if anc := NearestDataAttrAncestor(node, parentMap); anc != nil {
		inside = DataAttrSelector(anc)
	}
	return role + " @ " + inside
}

// ArrowHint builds the "→ selector" suggestion for a T3 node: the nearest
// data-attr ancestor as the scope plus the node's own tag as the leaf. Returns
// "" when there is no stable ancestor to anchor on.
func ArrowHint(node *comps.ComponentNode, parentMap ParentMap) string {
	nodeTag := ""
	if node.Element != nil {
		nodeTag = node.Element.Tag
	}
	if nodeTag == "" {
		nodeTag = node.Role
		if nodeTag == "" {
			nodeTag = "?"
		}
	}

	anc := NearestDataAttrAncestor(node, parentMap)
	if anc == nil || anc.Element == nil {
		return ""
	}
	a := anc.Element.Attrs
	if dt := a["data-testid"]; dt != "" {
		return fmt.Sprintf(`[data-testid="%s"] %s`, dt, nodeTag)
	}
	if dc := a["data-component"]; dc != "" {
		return fmt.Sprintf(`[data-component^="%s"] %s`, dc, nodeTag)
	}
	return SelectorString(anc.Element) + " " + nodeTag
}

// SelectorString formats an observed Element as tag[#id][.cls1.cls2]. Attributes
// are not included; they appear only in the trace/anchor displays above.
func SelectorString(s *comps.Element) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(s.Tag)
	if s.Id != "" {
		b.WriteByte('#')
		b.WriteString(s.Id)
	}
	for _, c := range s.Classes {
		b.WriteByte('.')
		b.WriteString(c)
	}
	return b.String()
}
