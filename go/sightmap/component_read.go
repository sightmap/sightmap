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
// The returned error is a ValidationError: an invalid extract carries the code
// Validate reports for it, and resolution failures carry
// component-path-unresolved, component-path-ambiguous,
// component-property-unresolved, component-read-cycle, or
// component-read-unsupported (a chained read whose inner read is itself
// joined, which no single read expresses).
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
	if defAddress(r.Target) != defAddress(owner) {
		r.Selectors = relativeSelectors(scope, owner, r.Target)
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
		target, multi, err := resolvePathDefs(scope, owner, path)
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
		if inner.Join != "" || (multi && inner.From == FromComponentExists) {
			return readError(owner, "component-read-unsupported",
				fmt.Sprintf("path %q reads %s.%s, which cannot be collected by a multi-valued read", e.Path, target.Name, prop))
		}
		r.Target = target
		if err := resolveRead(scope, target, inner, r, append(seen, key)); err != nil {
			return err
		}
		if multi {
			r.Multi, r.Join = true, e.Join
		}
	case FromComponentExists:
		target, _, err := resolvePathDefs(scope, owner, e.Path)
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
// last segment names, and reports whether any segment was Name[].
func resolvePathDefs(scope []ComponentDef, owner ComponentDef, path string) (ComponentDef, bool, error) {
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
	}
	return cur, multi, nil
}

// relativeSelectors returns target's selector alternatives relative to its
// ancestor owner: the cross product of the local selectors of every definition
// from owner (exclusive) down to target (inclusive), joined by a descendant
// combinator. That is target's flattened Selectors with owner's prefix removed.
func relativeSelectors(scope []ComponentDef, owner, target ComponentDef) []string {
	out := []string{""}
	parent := owner
	// target.ParentChain[:len(owner.ParentChain)+1] is owner's own address, so the
	// definitions strictly between owner and target start one level deeper.
	for depth := len(owner.ParentChain) + 2; depth <= len(target.ParentChain); depth++ {
		chain := target.ParentChain[:depth]
		var step ComponentDef
		for _, d := range scope {
			if slices.Equal(addressOf(d), chain) {
				step = d
				break
			}
		}
		out = crossSelectors(out, localSelectors(parent, step))
		parent = step
	}
	return crossSelectors(out, localSelectors(parent, target))
}

// localSelectors returns d's own selector alternatives, as authored. The loader
// records them; for a corpus built without them (an older exporter's wire) they
// are recovered from d's flattened Selectors by removing parent's.
func localSelectors(parent, d ComponentDef) []string {
	if len(d.LocalSelectors) > 0 {
		return d.LocalSelectors
	}
	var out []string
	for _, s := range d.Selectors {
		for _, ps := range parent.Selectors {
			if rel, ok := strings.CutPrefix(s, ps+" "); ok {
				if !slices.Contains(out, rel) {
					out = append(out, rel)
				}
				break
			}
		}
	}
	return out
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
