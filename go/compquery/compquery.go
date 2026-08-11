// Package compquery implements the component-query DSL: a CSS-shaped language
// for addressing elements by their sightmap identity (component name +
// extracted properties + descendant chain) rather than by an ephemeral probe
// id. It is a deliberately separate engine from package sel (which matches CSS
// selectors against DOM SelectorParts); the two share concepts but not code,
// because compquery matches against component names and human-facing extracted
// property values, not DOM tokens.
//
// Grammar (descendant combinator only; target is the LAST component):
//
//	query     := component (WS component)* ('#' index)?
//	component := Name ('[' predicate ']')*
//	predicate := Name op value (WS 'i')?
//	op        := '=' | '^=' | '*='        // exact | prefix | substring
//	value     := '"' ... '"' | bareword
//	index     := digits                    // 0-based occurrence selector
//
// Examples:
//
//	FulfillmentTileButton[label=deliveryTile]
//	ProductCard[sku=12345]
//	ProductCard[name^="Webber grill" i]   // case-insensitive prefix
//	LoginForm UserNameInput               // descendant chain; target = UserNameInput
//	FulfillmentTileButton#1               // pick occurrence 1 (0-based)
package compquery

import (
	"errors"
	"fmt"
	"github.com/sightmap/sightmap/go/sightmap"
	"sort"
	"strings"
)

// Query is a parsed component query: a descendant chain of component matchers
// plus an optional occurrence index. The target is the last Part; earlier Parts
// are ancestor scoping under descendant semantics.
type Query struct {
	Parts []Part
	Index int // 0-based occurrence to select among candidates; -1 when unset
}

// Part matches a single component: a name plus zero or more property predicates,
// all of which must hold.
type Part struct {
	Name  string
	Preds []Predicate
}

// Predicate is a single property constraint on a component.
type Predicate struct {
	Prop string
	Op   string // "=", "^=", or "*="
	Val  string
	CI   bool // case-insensitive comparison
}

// Match reports whether props satisfies the predicate. A missing property never
// matches.
func (pr Predicate) Match(props map[string]string) bool {
	v, ok := props[pr.Prop]
	if !ok {
		return false
	}
	a, b := v, pr.Val
	if pr.CI {
		a = strings.ToLower(a)
		b = strings.ToLower(b)
	}
	switch pr.Op {
	case "=":
		return a == b
	case "^=":
		return strings.HasPrefix(a, b)
	case "*=":
		return strings.Contains(a, b)
	default:
		return false
	}
}

// String renders the query in canonical form (round-trips through ParseQuery
// up to whitespace and quoting). Used in error messages.
func (q *Query) String() string {
	var b strings.Builder
	for i, p := range q.Parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(p.Name)
		for _, pr := range p.Preds {
			fmt.Fprintf(&b, "[%s%s%q", pr.Prop, pr.Op, pr.Val)
			if pr.CI {
				b.WriteString(" i")
			}
			b.WriteByte(']')
		}
	}
	if q.Index >= 0 {
		fmt.Fprintf(&b, "#%d", q.Index)
	}
	return b.String()
}

// Resolve finds the single component node matching q. matches maps tree nodes to
// their sightmap identity; props maps node id -> extracted property values
// (keyed by sightmap.ComponentNode.Id). It returns a descriptive error when nothing
// matches, or when several components match and no occurrence index (#N)
// disambiguates.
func Resolve(
	root *sightmap.ComponentNode,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	props map[string]map[string]string,
	q *Query,
) (*sightmap.ComponentNode, error) {
	cands := FindCandidates(root, matches, props, q)
	if len(cands) == 0 {
		return nil, fmt.Errorf("no component matches query %s", q)
	}
	if q.Index >= 0 {
		if q.Index >= len(cands) {
			return nil, fmt.Errorf("query %s: index %d out of range (%d match(es))", q, q.Index, len(cands))
		}
		return cands[q.Index], nil
	}
	if len(cands) == 1 {
		return cands[0], nil
	}
	return nil, ambiguityError(q, cands, matches, props)
}

// FindCandidates returns every node satisfying q's target part whose matched
// ancestors satisfy the preceding chain parts as an ordered subsequence
// (descendant semantics). Results are in document (pre-order) order.
func FindCandidates(
	root *sightmap.ComponentNode,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	props map[string]map[string]string,
	q *Query,
) []*sightmap.ComponentNode {
	if root == nil || q == nil || len(q.Parts) == 0 {
		return nil
	}
	target := q.Parts[len(q.Parts)-1]
	prefix := q.Parts[:len(q.Parts)-1]

	var out []*sightmap.ComponentNode
	var walk func(node *sightmap.ComponentNode, ancestors []*sightmap.ComponentNode)
	walk = func(node *sightmap.ComponentNode, ancestors []*sightmap.ComponentNode) {
		if satisfies(node, target, matches, props) && subsequence(prefix, ancestors, matches, props) {
			out = append(out, node)
		}
		next := ancestors
		if matches[node] != nil {
			// Only matched (named) nodes can satisfy chain parts, so only they
			// extend the ancestor scope.
			next = append(append([]*sightmap.ComponentNode(nil), ancestors...), node)
		}
		for _, ch := range node.Children {
			walk(ch, next)
		}
	}
	walk(root, nil)
	return out
}

// satisfies reports whether node is matched as part.Name and all of part's
// predicates hold against the node's extracted properties.
func satisfies(
	node *sightmap.ComponentNode,
	part Part,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	props map[string]map[string]string,
) bool {
	m := matches[node]
	if m == nil || m.Name != part.Name {
		return false
	}
	if len(part.Preds) == 0 {
		return true
	}
	np := props[node.Id]
	for _, pr := range part.Preds {
		if !pr.Match(np) {
			return false
		}
	}
	return true
}

// subsequence reports whether prefix parts appear, in order, among the matched
// ancestors (each ancestor consumed at most once). An empty prefix is satisfied.
func subsequence(
	prefix []Part,
	ancestors []*sightmap.ComponentNode,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	props map[string]map[string]string,
) bool {
	i := 0
	for _, a := range ancestors {
		if i >= len(prefix) {
			break
		}
		if satisfies(a, prefix[i], matches, props) {
			i++
		}
	}
	return i == len(prefix)
}

func ambiguityError(
	q *Query,
	cands []*sightmap.ComponentNode,
	matches map[*sightmap.ComponentNode]*sightmap.ComponentMatch,
	props map[string]map[string]string,
) error {
	var b strings.Builder
	fmt.Fprintf(&b,
		"query %s is ambiguous: %d components match. Add a property predicate or an occurrence index (#N):",
		q, len(cands))
	for i, c := range cands {
		name := ""
		if m := matches[c]; m != nil {
			name = m.Name
		}
		fmt.Fprintf(&b, "\n  #%d  id=%s  %s", i, c.Id, name)
		for _, k := range sortedKeys(props[c.Id]) {
			fmt.Fprintf(&b, " %s=%q", k, props[c.Id][k])
		}
	}
	return errors.New(b.String())
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
