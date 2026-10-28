package sightmap

import (
	"fmt"
	"slices"
	"strings"
)

// ComponentRead is a component property resolved statically against a corpus's
// definitions: which element the value is read from, how it is read there, and
// the refinements applied to it. It is for consumers that cannot match a
// component tree at extraction time (for example a rule engine that extracts
// from a DOM by selector) and compile properties ahead of time; a consumer
// holding a matched tree resolves properties with match.Matcher instead.
type ComponentRead struct {
	// Target is the definition the value is read from: the declaring component
	// for a dom.* source, else the component its PATH addresses, following
	// chained reads to the last.
	Target ComponentDef
	// Selectors select Target's elements within the declaring component's
	// subtree: Target's selector alternatives relative to the declaring
	// component, every definition between them included. Nil when Target is the
	// declaring component.
	//
	// A multi-valued read collects every element Selectors match, exactly as
	// the matcher does. A single-valued read takes the first in document order.
	// That is the matcher's value whenever it resolves one; with more than one
	// PATH step, the matcher takes the first match at each step, so when the
	// first match of an earlier step contains none of the later step, the
	// matcher omits the property while Selectors can still find a later match.
	Selectors []string
	// Source is the read performed at Target: a dom.* extract (From and Path,
	// never Pattern or Join), or From: component.exists.
	Source Extract
	// Patterns refine the raw value, in order: the innermost read's pattern
	// first, then each enclosing read's. Apply each as Extract.Refine does; a
	// value a pattern does not match is omitted.
	Patterns []string
	// Multi reports a PATH with a Name[] segment, which reads every matching
	// element rather than the first in document order.
	Multi bool
	// Join collapses a multi-valued read's values into one: each value is
	// refined, empty ones are dropped, and the rest are joined with Join. Set
	// exactly when Multi is.
	Join string

	plainSteps int // PATH steps without [], across chained reads
}

// maxComponentReadDepth bounds chained reads. The spec has references descend
// only, so a chain is acyclic and as deep as the component tree; the bound is a
// backstop against a malformed corpus, not a limit a valid one approaches.
const maxComponentReadDepth = 32

// ResolveComponentRead resolves the extract e, declared by the component owner,
// against c's definitions (SEP-0010). owner must be a definition of c, as
// AllComponents, a view's Components or GlobalComponents return it; it is
// located by address (ParentChain and Name) and selectors.
//
// A PATH segment names a descendant at any depth beneath the previous segment's
// definition, as the spec resolves it over a matched tree. At match time a plain
// segment takes the first match in document order; statically that is only
// well-defined when the segment names exactly one definition, so a segment
// naming none is reported as component-path-unresolved and one naming several as
// component-path-ambiguous. A component read whose target property is itself a
// component read is followed to the leaf.
//
// A multi-valued read is resolvable only when every PATH step on the chain is
// written Name[]. A plain step takes the first match within each branch the
// steps before it produced, so `Row[].Item.text` reads one Item per Row, and
// one list of selectors cannot express that per-branch first match.
//
// The returned error is a ValidationError: an invalid extract carries the code
// Validate reports for it, and resolution failures carry
// component-path-unresolved, component-path-ambiguous,
// component-property-unresolved, component-read-cycle,
// component-read-owner-unknown, or component-read-unsupported. The last covers
// a multi-valued read with a plain step (including a chained read beneath
// it), a chained read of a property that is itself joined, and a corpus whose
// nested selectors cannot be recovered (see ComponentDef.LocalSelectors).
func (c *Corpus) ResolveComponentRead(owner ComponentDef, e Extract) (ComponentRead, error) {
	scope := c.scopeOf(owner)
	if scope == nil {
		return ComponentRead{}, readError(owner, "component-read-owner-unknown",
			fmt.Sprintf("component %q is not a definition of this corpus", owner.Name))
	}
	r := ComponentRead{Target: owner}
	if err := resolveRead(scope, owner, e, &r, nil); err != nil {
		return ComponentRead{}, err
	}
	if r.Multi && r.plainSteps > 0 {
		return ComponentRead{}, readError(owner, "component-read-unsupported",
			fmt.Sprintf("path %q mixes Name[] with plain steps (across chained reads), which take the first match per branch; write every step as Name[] or read a single value", e.Path))
	}
	if defAddress(r.Target) != defAddress(owner) {
		sels, err := relativeSelectors(scope, owner, r.Target)
		if err != nil {
			return ComponentRead{}, err
		}
		r.Selectors = sels
	}
	return r, nil
}

// resolveRead resolves e declared by owner into r, appending patterns as it
// follows chained reads. seen holds the (component address, property) reads on
// the current chain.
func resolveRead(scope []ComponentDef, owner ComponentDef, e Extract, r *ComponentRead, seen []string) error {
	if code, msg := checkComponentExtract(e); msg != "" {
		return readError(owner, code, msg)
	}
	switch e.From {
	case FromComponent:
		dot := strings.LastIndex(e.Path, ".")
		path, prop := e.Path[:dot], e.Path[dot+1:]
		target, multi, err := resolvePathDefs(scope, owner, path, r)
		if err != nil {
			return err
		}
		key := defAddress(target) + "\x00." + prop
		if slices.Contains(seen, key) || len(seen) >= maxComponentReadDepth {
			return readError(owner, "component-read-cycle",
				fmt.Sprintf("property read %q cycles through %s.%s", e.Path, target.Name, prop))
		}
		i := slices.IndexFunc(target.Properties, func(p ComponentPropertyDef) bool { return p.Name == prop })
		if i < 0 {
			return readError(owner, "component-property-unresolved",
				fmt.Sprintf("path %q: component %q declares no property %q", e.Path, target.Name, prop))
		}
		inner := target.Properties[i].Extract
		if inner.Join != "" {
			return readError(owner, "component-read-unsupported",
				fmt.Sprintf("path %q reads %s.%s, which is itself joined", e.Path, target.Name, prop))
		}
		r.Target = target
		if err := resolveRead(scope, target, inner, r, append(seen, key)); err != nil {
			return err
		}
		if multi {
			r.Multi, r.Join = true, e.Join
		}
	case FromComponentExists:
		target, _, err := resolvePathDefs(scope, owner, e.Path, r)
		if err != nil {
			return err
		}
		r.Target = target
		r.Source = Extract{From: FromComponentExists}
	default:
		r.Source = Extract{From: e.From, Path: e.Path}
	}
	if e.Pattern != "" {
		r.Patterns = append(r.Patterns, e.Pattern)
	}
	return nil
}

// resolvePathDefs resolves a component PATH beneath owner to the definition its
// last segment names, and reports whether any segment was Name[]. It counts
// the plain segments into r.
func resolvePathDefs(scope []ComponentDef, owner ComponentDef, path string, r *ComponentRead) (ComponentDef, bool, error) {
	cur := owner
	var multi bool
	for _, seg := range ParseComponentPath(path) {
		var found []ComponentDef
		prefix := addressOf(cur)
		for _, d := range scope {
			if d.Name == seg.Name && len(d.ParentChain) >= len(prefix) && slices.Equal(d.ParentChain[:len(prefix)], prefix) {
				found = append(found, d)
			}
		}
		switch len(found) {
		case 0:
			return ComponentDef{}, false, readError(owner, "component-path-unresolved",
				fmt.Sprintf("path %q: %q declares no descendant %q", path, cur.Name, seg.Name))
		case 1:
		default:
			return ComponentDef{}, false, readError(owner, "component-path-ambiguous",
				fmt.Sprintf("path %q: %q declares %d descendants named %q", path, cur.Name, len(found), seg.Name))
		}
		cur = found[0]
		multi = multi || seg.Multi
		if !seg.Multi {
			r.plainSteps++
		}
	}
	return cur, multi, nil
}

// relativeSelectors returns target's selector alternatives relative to its
// ancestor owner: the cross product of the local selectors of every definition
// from owner (exclusive) down to target (inclusive), joined by a descendant
// combinator. That is target's flattened Selectors with owner's prefix removed.
func relativeSelectors(scope []ComponentDef, owner, target ComponentDef) ([]string, error) {
	out := []string{""}
	parent := owner
	step := func(d ComponentDef) error {
		local, ok := localSelectors(parent, d)
		if !ok {
			return readError(owner, "component-read-unsupported",
				fmt.Sprintf("cannot recover %q's own selectors from its flattened ones; re-export the corpus with a current sightmap", d.Name))
		}
		out = crossSelectors(out, local)
		parent = d
		return nil
	}
	// target.ParentChain[:len(owner.ParentChain)+1] is owner's own address, so the
	// definitions strictly between owner and target start one level deeper.
	for depth := len(owner.ParentChain) + 2; depth <= len(target.ParentChain); depth++ {
		chain := target.ParentChain[:depth]
		i := slices.IndexFunc(scope, func(d ComponentDef) bool { return slices.Equal(addressOf(d), chain) })
		if i < 0 {
			return nil, readError(owner, "component-path-unresolved",
				fmt.Sprintf("no definition at %q between %q and %q", strings.Join(chain, "."), owner.Name, target.Name))
		}
		if err := step(scope[i]); err != nil {
			return nil, err
		}
	}
	if err := step(target); err != nil {
		return nil, err
	}
	return out, nil
}

// localSelectors returns d's own selector alternatives, as authored. The loader
// records them. For a corpus built without them (an older exporter's wire) they
// are recovered from the loader's flattening order: d's Selectors are every
// parent alternative, in order, each followed by every local one, in order. The
// recovery is checked against every flattened entry and reports false when it
// does not hold, rather than guessing which parent alternative produced an
// entry (one parent alternative can prefix another).
func localSelectors(parent, d ComponentDef) ([]string, bool) {
	if len(d.LocalSelectors) > 0 {
		return d.LocalSelectors, true
	}
	p := len(parent.Selectors)
	if p == 0 || len(d.Selectors) == 0 || len(d.Selectors)%p != 0 {
		return nil, false
	}
	n := len(d.Selectors) / p
	local := make([]string, n)
	for j := range n {
		rel, ok := strings.CutPrefix(d.Selectors[j], parent.Selectors[0]+" ")
		if !ok || rel == "" {
			return nil, false
		}
		local[j] = rel
	}
	for i, ps := range parent.Selectors {
		for j, ls := range local {
			if d.Selectors[i*n+j] != ps+" "+ls {
				return nil, false
			}
		}
	}
	return local, true
}

// crossSelectors joins every outer alternative with every inner one by a
// descendant combinator; an empty outer alternative contributes the inner one
// alone.
func crossSelectors(outer, inner []string) []string {
	out := make([]string, 0, len(outer)*len(inner))
	for _, o := range outer {
		for _, i := range inner {
			if o == "" {
				out = append(out, i)
			} else {
				out = append(out, o+" "+i)
			}
		}
	}
	return out
}

// scopeOf returns the definition list that declares owner: the globals, the
// file-root definitions, or a view's components. A name is unique only within
// its parent, and a view repeats $ref-expanded globals under the same address,
// so a PATH is resolved within the one list its declaring component came from.
func (c *Corpus) scopeOf(owner ComponentDef) []ComponentDef {
	is := func(d ComponentDef) bool {
		return d.Name == owner.Name && slices.Equal(d.ParentChain, owner.ParentChain) && slices.Equal(d.Selectors, owner.Selectors)
	}
	lists := [][]ComponentDef{c.GlobalComponents, c.Definitions}
	for _, v := range c.Views {
		lists = append(lists, v.Components)
	}
	for _, l := range lists {
		if slices.ContainsFunc(l, is) {
			return l
		}
	}
	return nil
}

func addressOf(d ComponentDef) []string {
	return append(slices.Clone(d.ParentChain), d.Name)
}

func defAddress(d ComponentDef) string {
	return strings.Join(addressOf(d), "\x00")
}

func readError(owner ComponentDef, code, msg string) ValidationError {
	return ValidationError{Component: owner.Name, Code: code, Severity: SeverityError, Message: msg}
}
