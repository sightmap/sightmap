package device

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sightmap/sightmap/go/coverage"
	"github.com/sightmap/sightmap/go/sightmap"
)

// RenderOpts controls Render.
type RenderOpts struct {
	// IncludeOffscreen lists elements whose frame is off the screen too.
	IncludeOffscreen bool
	// UDID and AccessibilityOn tailor the advice for an empty tree.
	UDID            string
	AccessibilityOn bool
}

func quote(s string) string { return strconv.Quote(s) }

// Describe returns one line for n in the snapshot's own notation, without the
// on-screen markers.
func (s *Snapshot) Describe(n *sightmap.ComponentNode) string {
	return n.Id + " " + s.body(n)
}

func (s *Snapshot) body(n *sightmap.ComponentNode) string {
	if name := s.Component(n); name != "" {
		props := s.Props[n.Id]
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{name}
		labelShown := false
		for _, k := range keys {
			parts = append(parts, k+"="+quote(props[k]))
			labelShown = labelShown || props[k] == n.Name
		}
		if n.Name != "" && !labelShown {
			parts = append(parts, quote(n.Name))
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	b := n.Role
	if id := Identifier(n); id != "" {
		b += "[identifier=" + quote(id) + "]"
	}
	if n.Name != "" {
		b += " " + quote(n.Name)
	}
	if n.Value != "" {
		b += " value=" + quote(n.Value)
	}
	return b
}

func (s *Snapshot) line(n *sightmap.ComponentNode) string {
	l := s.Describe(n)
	if !Enabled(n) {
		l += " (disabled)"
	}
	if n.State["selected"] == "true" {
		l += " (selected)"
	}
	if !n.IsVisible {
		l += " (off-screen)"
	} else if !s.OnScreen(n) {
		l += " (partly off-screen)"
	}
	return l
}

// Render formats the snapshot in the shape of `sightmap snapshot`: the view,
// notes about what covers or hides the app, the guide and memory, the component
// tree with per-snapshot ids, conflicts and coverage.
func (s *Snapshot) Render(opts RenderOpts) string {
	var b strings.Builder
	switch {
	case s.View != nil && s.ViewInferred:
		fmt.Fprintf(&b, "[View: %s] (inferred)\n", s.View.Name)
	case s.View != nil:
		fmt.Fprintf(&b, "[View: %s]\n", s.View.Name)
	case s.CorpusApplied:
		b.WriteString("[No view matched]\n")
	}
	b.WriteString("\n")

	if s.Overlay {
		owner := ""
		if s.Root.Name != "" {
			owner = " (" + quote(s.Root.Name) + ")"
		}
		fmt.Fprintf(&b, "[Overlay]\nA system sheet from another process%s is in front of the app; the tree below is that sheet.\n", owner)
		b.WriteString("Dismiss it with one of its buttons, e.g. `click 'button[text=\"Not Now\"]'`, then snapshot again.\n\n")
	}
	if note := s.sparseNote(opts.UDID, opts.AccessibilityOn); note != nil {
		b.WriteString("[Mostly empty]\n" + strings.Join(note, "\n") + "\n\n")
	}
	if lb := s.logBoxToast(); lb != nil {
		fmt.Fprintf(&b, "[LogBox]\nA React Native LogBox toast (#%s) covers the bottom of the screen; tapping it opens the LogBox inspector.\n", lb.Node.Id)
		fmt.Fprintf(&b, "Dismiss it with `tap-xy %d %d` (its ✕), or call LogBox.ignoreAllLogs() in the app.\n\n", lb.DismissX, lb.DismissY)
	}
	s.writeGuide(&b)
	if len(s.Memory) > 0 {
		b.WriteString("[Memory]\n")
		for _, m := range s.Memory {
			if m.Scope != "" {
				fmt.Fprintf(&b, "- %s: %s\n", m.Scope, m.Text)
			} else {
				fmt.Fprintf(&b, "- %s\n", m.Text)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("--- component tree ---\n\n")
	hidden, hiddenInteractive := 0, 0
	var walk func(n *sightmap.ComponentNode, depth int)
	walk = func(n *sightmap.ComponentNode, depth int) {
		childDepth := depth
		switch {
		case !n.IsVisible && !opts.IncludeOffscreen:
			hidden++
			if n.IsInteractive {
				hiddenInteractive++
			}
		case n.IsIgnored && s.Component(n) == "":
		default:
			b.WriteString(strings.Repeat("  ", depth) + s.line(n) + "\n")
			childDepth = depth + 1
		}
		for _, c := range n.Children {
			walk(c, childDepth)
		}
	}
	walk(s.Root, 0)

	if len(s.Conflicts) > 0 {
		b.WriteString("\n[Conflicts]\n")
		for _, c := range s.Conflicts {
			fmt.Fprintf(&b, "⚠ #%s %s matched by %d components (%s) — only %s applied\n",
				c.Node.Id, c.Node.Role+nameSuffix(c.Node), len(c.Names), strings.Join(c.Names, ", "), s.Component(c.Node))
		}
	}
	if hidden > 0 {
		fmt.Fprintf(&b, "\n[Off-screen]\n%d off-screen node(s) not shown (%d interactive); scroll to reveal them. Queries still resolve them.\n", hidden, hiddenInteractive)
	}
	cov := s.Coverage
	b.WriteString("\n[Coverage] (visible only)\n")
	if cov.Total == 0 {
		b.WriteString("0 interactive ∅")
	} else {
		fmt.Fprintf(&b, "%d interactive · %d direct T1 (%d%%) · %d scoped T2 (%d%%) · %d orphaned T3 %s",
			cov.Total, cov.T1, coverage.Pct(cov.T1, cov.Total), cov.T2, coverage.Pct(cov.T2, cov.Total), cov.T3, cov.Mark())
	}
	return b.String()
}

func nameSuffix(n *sightmap.ComponentNode) string {
	if n.Name == "" {
		return ""
	}
	return " " + quote(n.Name)
}

func (s *Snapshot) writeGuide(b *strings.Builder) {
	counts := map[string]int{}
	for _, cm := range s.Matches {
		counts[cm.Name]++
	}
	if len(counts) == 0 {
		return
	}
	names := make([]string, 0, len(counts))
	width := 0
	for name := range counts {
		names = append(names, name)
		width = max(width, len(name))
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	b.WriteString("[Guide]\n")
	for _, name := range names {
		fmt.Fprintf(b, "%-*s  %d\n", width, name, counts[name])
	}
	b.WriteString("\n")
}

// AnnotatedTree returns the tree in the shape of `sightmap snapshot --json`:
// each matched node carries its component and props, so offline tools and
// scripts read device snapshots like browser ones.
func (s *Snapshot) AnnotatedTree() *AnnotatedNode {
	var convert func(n *sightmap.ComponentNode) *AnnotatedNode
	convert = func(n *sightmap.ComponentNode) *AnnotatedNode {
		out := &AnnotatedNode{ComponentNode: n}
		if name := s.Component(n); name != "" {
			out.Component = name
			if p := s.Props[n.Id]; len(p) > 0 {
				out.Props = p
			}
		}
		for _, c := range n.Children {
			out.Children = append(out.Children, convert(c))
		}
		return out
	}
	return convert(s.Root)
}

// AnnotatedNode is a ComponentNode with its sightmap identity.
type AnnotatedNode struct {
	*sightmap.ComponentNode
	Component string            `json:"component,omitempty"`
	Props     map[string]string `json:"props,omitempty"`
	Children  []*AnnotatedNode  `json:"children,omitempty"`
}

// UnmarshalJSON decodes the node's own ComponentNode fields and its identity.
// It allocates the embedded node itself: a decoder that calls the embedded
// pointer's UnmarshalJSON (encoding/json/v2 does) would otherwise call it on nil.
func (a *AnnotatedNode) UnmarshalJSON(data []byte) error {
	node := &sightmap.ComponentNode{}
	if err := json.Unmarshal(data, node); err != nil {
		return err
	}
	var identity struct {
		Component string            `json:"component"`
		Props     map[string]string `json:"props"`
		Children  []*AnnotatedNode  `json:"children"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return err
	}
	node.Children = node.Children[:0]
	for _, c := range identity.Children {
		node.Children = append(node.Children, c.ComponentNode)
	}
	*a = AnnotatedNode{ComponentNode: node, Component: identity.Component, Props: identity.Props, Children: identity.Children}
	return nil
}
