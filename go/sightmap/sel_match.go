package sightmap

import (
	"strings"
)

// MatchesNode reports whether a tree node satisfies rule, including the
// relational pseudo-classes that need the node's subtree (:has()) or its
// ancestors (a :not() whose argument carries a combinator). It is the
// single-node entry point: the node is treated as the root of its own chain, so
// an ancestor-constrained :not() argument finds no ancestor and therefore does
// not exclude. Callers that hold the node's ancestor chain (the NFA matcher)
// should use MatchesNodeChain so those :not() arguments are evaluated fully.
// Tree-walking callers (sel-check, lint, coverage) use this.
func MatchesNode(node *ComponentNode, rule *SelectorPart) bool {
	return MatchesNodeChain([]*ComponentNode{node}, rule)
}

// MatchesNodeChain reports whether the LAST node of chain (the subject) satisfies
// rule. chain is root-first (chain[0] is the outermost ancestor, chain[len-1] is
// the subject); chain[:len-1] is the subject's ancestor path, used to evaluate a
// :not() argument that carries a combinator (an ancestor constraint on the
// subject). :has() is evaluated against the subject's own subtree. A nil rule
// matches everything.
func MatchesNodeChain(chain []*ComponentNode, rule *SelectorPart) bool {
	if rule == nil {
		return true
	}
	if len(chain) == 0 {
		return matchesNodeChain([]*ComponentNode{{}}, 0, rule)
	}
	return matchesNodeChain(chain, len(chain)-1, rule)
}

// matchesNodeChain reports whether chain[i] satisfies rule, using chain[:i] as
// its ancestors (for combinator-bearing :not() arguments) and chain[i]'s subtree
// (for :has()). It is the recursion core shared by MatchesNodeChain, :is()
// alternatives, and :not() argument subjects, so every nested relational pseudo
// is honored with the correct tree context.
func matchesNodeChain(chain []*ComponentNode, i int, rule *SelectorPart) bool {
	if rule == nil {
		return true
	}
	node := chain[i]
	el := node.Element
	if el == nil {
		el = &Element{}
	}
	// Flat identity: tag, id, classes, attributes.
	if !matchesIdentity(el, rule) {
		return false
	}
	// :is() / :where() — the subject must match at least one alternative.
	// Evaluated node-aware so an alternative may itself carry :has()/:not().
	if len(rule.Is) > 0 {
		anyMatch := false
		for _, alt := range rule.Is {
			if matchesNodeChain(chain, i, alt) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			return false
		}
	}
	// :not() — exclude if the subject matches ANY argument (as that argument's
	// subject). Multiple :not() and comma-lists both flatten into rule.Not.
	for n := range rule.Not {
		arg := rule.Not[n]
		if complexSubjectMatch(chain, i, arg.Parts, arg.Combinators, len(arg.Parts)-1) {
			return false
		}
	}
	// :has() — each entry is AND-ed; within an entry, alternatives are OR-ed.
	for _, h := range rule.Has {
		if !hasMatches(node, h) {
			return false
		}
	}
	return true
}

// complexSubjectMatch reports whether chain[i] matches the complex selector
// parts[:pi+1] (root-first, combinators[k] precedes parts[k], combinators[0]=="")
// with parts[pi] as the subject. parts[pi] is tested against chain[i]; each
// earlier part is matched against an ancestor per its combinator (">" = direct
// parent, " " = any ancestor). Used to evaluate a :not() argument.
func complexSubjectMatch(chain []*ComponentNode, i int, parts []*SelectorPart, combs []string, pi int) bool {
	if pi < 0 || pi >= len(parts) {
		return false
	}
	if !matchesNodeChain(chain, i, parts[pi]) {
		return false
	}
	if pi == 0 {
		return true
	}
	// combs[pi] links parts[pi-1] (ancestor) to parts[pi] (this subject).
	if combs[pi] == ">" {
		return i > 0 && complexSubjectMatch(chain, i-1, parts, combs, pi-1)
	}
	for a := i - 1; a >= 0; a-- {
		if complexSubjectMatch(chain, a, parts, combs, pi-1) {
			return true
		}
	}
	return false
}

// hasMatches reports whether node's subtree satisfies the :has() selector h
// (any of its comma-separated alternatives).
func hasMatches(node *ComponentNode, h *HasSelector) bool {
	for _, alt := range h.Alternatives {
		if relMatches(node, alt.Parts, alt.Combinators, 0) {
			return true
		}
	}
	return false
}

// relMatches reports whether the relative chain parts[idx:] can be satisfied
// within anchor's subtree. combs[idx] links anchor to parts[idx]: ">" means
// parts[idx] must match a direct child of anchor; " " means any descendant.
// MatchesNode is used for each candidate so nested :has() works.
func relMatches(anchor *ComponentNode, parts []*SelectorPart, combs []string, idx int) bool {
	direct := combs[idx] == ">"
	var search func(n *ComponentNode) bool
	search = func(n *ComponentNode) bool {
		for _, child := range n.Children {
			if MatchesNode(child, parts[idx]) {
				if idx+1 == len(parts) {
					return true
				}
				if relMatches(child, parts, combs, idx+1) {
					return true
				}
			}
			if !direct && search(child) {
				return true
			}
		}
		return false
	}
	return search(anchor)
}

// Matches reports whether el satisfies rule using only the element's own
// identity (no tree). It checks tag, id, classes, attribute operators, :is()/
// :where(), and the tree-free part of :not(). It CANNOT evaluate :has() (needs a
// subtree), a :not() argument carrying a combinator (needs ancestors), or a
// :not(:has()) (needs a subtree): those are skipped here, so this is a best-
// effort element-only check — use MatchesNode / MatchesNodeChain for full,
// tree-aware matching. A nil rule matches everything. el is the observed
// element's identity (the subject).
func Matches(el *Element, rule *SelectorPart) bool {
	if rule == nil {
		return true
	}
	if !matchesIdentity(el, rule) {
		return false
	}

	// :is() / :where() — node must match at least one alternative.
	if len(rule.Is) > 0 {
		anyMatch := false
		for _, alt := range rule.Is {
			if Matches(el, alt) {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			return false
		}
	}

	// :not() — element-only: an argument that is a single compound with no :has()
	// can be evaluated against el alone. Complex (ancestor-constrained) or
	// :has()-bearing arguments need tree context and are skipped here.
	for n := range rule.Not {
		arg := rule.Not[n]
		if len(arg.Parts) == 1 && len(arg.Parts[0].Has) == 0 && Matches(el, arg.Parts[0]) {
			return false
		}
	}

	return true
}

// matchesIdentity checks the tree-free identity of el against rule: tag, id,
// classes, and attribute operators. It ignores the logical/relational pseudos
// (:is/:not/:has), which the callers layer on with the appropriate context.
func matchesIdentity(el *Element, rule *SelectorPart) bool {
	// Tag match (case-insensitive for HTML; case-sensitive for synthetic mobile).
	if rule.Tag != "" && !strings.EqualFold(el.Tag, rule.Tag) {
		return false
	}

	// ID match.
	if rule.Id != "" && el.Id != rule.Id {
		return false
	}

	// Class match: every class in rule.Classes must appear in el.Classes.
	if len(rule.Classes) > 0 {
		nodeClasses := sliceToSet(el.Classes)
		for _, cls := range rule.Classes {
			if !nodeClasses[cls] {
				return false
			}
		}
	}

	// Attribute match.
	for key, ruleVal := range rule.Attrs {
		op := "="
		if rule.AttrOps != nil {
			if o, ok := rule.AttrOps[key]; ok {
				op = o
			}
		}

		nodeVal, present := effectiveAttrValue(el, key)
		if !present {
			// Attribute not present on node — only "[]" (presence-only) would
			// logically not care, but absence means presence check fails too.
			return false
		}

		if !attrMatches(op, nodeVal, ruleVal) {
			return false
		}
	}

	return true
}

// effectiveAttrValue resolves an attribute value on an observed Element for
// matching. id and class live in dedicated fields (Id, Classes) — not always in
// Attrs — so attribute selectors like [id^="issue_"] or [class*="card"] must see
// them there to match offline the way the browser matches them live. Attrs is
// consulted first (it wins when populated); id/class then fall back to their
// dedicated fields. All other attributes come straight from Attrs.
func effectiveAttrValue(el *Element, key string) (string, bool) {
	if v, ok := el.Attrs[key]; ok {
		return v, true
	}
	switch key {
	case "id":
		if el.Id != "" {
			return el.Id, true
		}
	case "class":
		if len(el.Classes) > 0 {
			return strings.Join(el.Classes, " "), true
		}
	}
	return "", false
}

// attrMatches returns whether a node attribute value satisfies the operator
// against the rule value.
func attrMatches(op, nodeVal, ruleVal string) bool {
	switch op {
	case "=":
		return nodeVal == ruleVal
	case "[]":
		// Presence-only — attribute exists (we already checked above).
		return true
	case "^=":
		return ruleVal != "" && strings.HasPrefix(nodeVal, ruleVal)
	case "$=":
		return ruleVal != "" && strings.HasSuffix(nodeVal, ruleVal)
	case "*=":
		return ruleVal != "" && strings.Contains(nodeVal, ruleVal)
	case "~=":
		// Whitespace-separated list includes ruleVal exactly.
		return includesWord(nodeVal, ruleVal)
	case "|=":
		// Equals ruleVal or starts with "ruleVal-".
		return nodeVal == ruleVal || strings.HasPrefix(nodeVal, ruleVal+"-")
	default:
		return false
	}
}

// includesWord returns true if s is a whitespace-separated list containing word.
func includesWord(s, word string) bool {
	for s != "" {
		i := strings.IndexAny(s, " \t\r\n\f")
		var token string
		if i < 0 {
			token = s
			s = ""
		} else {
			token = s[:i]
			s = s[i+1:]
		}
		if token == word {
			return true
		}
	}
	return false
}

// sliceToSet converts a string slice to a presence map.
func sliceToSet(ss []string) map[string]bool {
	if len(ss) == 0 {
		return nil
	}
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
